package control

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	maxIssueDepth             = 4
	maxChildrenPerRequest     = 8
	maxDirectChildrenPerIssue = 16
)

// CreateSubIssues is the durable control-plane operation behind the Pi tool.
// The request key makes retries from the same execution idempotent.
func (s *Store) CreateSubIssues(parentID, executionID, actorAgentID string, input DecomposeIssueInput) (DecompositionResult, error) {
	input.RequestKey = strings.TrimSpace(input.RequestKey)
	input.Summary = strings.TrimSpace(input.Summary)
	if input.RequestKey == "" || len(input.RequestKey) > 120 {
		return DecompositionResult{}, errors.New("requestKey 必填且不能超过 120 个字符")
	}
	if len(input.Children) < 2 || len(input.Children) > maxChildrenPerRequest {
		return DecompositionResult{}, fmt.Errorf("每次必须创建 2-%d 个子 Issues", maxChildrenPerRequest)
	}
	for index := range input.Children {
		item := &input.Children[index]
		var err error
		item.Title, err = normalizeIssueTitle(item.Title)
		if err != nil {
			return DecompositionResult{}, fmt.Errorf("子 Issue %d: %w", index+1, err)
		}
		item.Description = strings.TrimSpace(item.Description)
		item.AcceptanceCriteria = strings.TrimSpace(item.AcceptanceCriteria)
		item.AgentID = strings.TrimSpace(item.AgentID)
		if item.AcceptanceCriteria == "" {
			return DecompositionResult{}, fmt.Errorf("子 Issue %d 缺少验收标准", index+1)
		}
		if !slices.Contains([]string{"critical", "high", "medium", "low"}, item.Priority) {
			item.Priority = "medium"
		}
		for _, dependency := range item.DependsOn {
			if dependency < 1 || dependency >= index+1 {
				return DecompositionResult{}, fmt.Errorf("子 Issue %d 的 dependsOn 只能引用更早的 1-based 索引", index+1)
			}
		}
		agent, err := s.chooseAgent(Issue{Title: item.Title, Description: item.Description, AcceptanceCriteria: item.AcceptanceCriteria, AssigneeAgentID: item.AgentID})
		if err != nil {
			return DecompositionResult{}, err
		}
		item.AgentID = agent.ID
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var output DecompositionResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing IssueDecomposition
		if err := tx.Where("parent_issue_id = ? AND request_key = ?", parentID, input.RequestKey).First(&existing).Error; err == nil {
			output.Decomposition = existing
			output.Reused = true
			for _, id := range existing.ChildIDs {
				var child Issue
				if err := tx.First(&child, "id = ?", id).Error; err != nil {
					return err
				}
				output.Children = append(output.Children, child)
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var parent Issue
		if err := tx.First(&parent, "id = ?", parentID).Error; err != nil {
			return errors.New("parent issue not found")
		}
		if parent.RequestDepth >= maxIssueDepth {
			return fmt.Errorf("Issue 已达到最大拆解深度 %d", maxIssueDepth)
		}
		if parent.Status != "in_progress" || parent.AssigneeAgentID != actorAgentID || parent.CheckoutExecutionID != executionID {
			return errors.New("当前 Execution 不持有该 Issue 的 checkout")
		}
		var execution Execution
		if err := tx.First(&execution, "id = ? AND issue_id = ? AND agent_id = ?", executionID, parent.ID, actorAgentID).Error; err != nil {
			return errors.New("execution ownership mismatch")
		}
		if !slices.Contains([]string{"starting", "running", "waiting_approval"}, execution.Status) {
			return errors.New("execution is no longer active")
		}
		var existingCount int64
		if err := tx.Model(&Issue{}).Where("parent_id = ?", parent.ID).Count(&existingCount).Error; err != nil {
			return err
		}
		if existingCount+int64(len(input.Children)) > maxDirectChildrenPerIssue {
			return fmt.Errorf("父 Issue 最多允许 %d 个直属子 Issues", maxDirectChildrenPerIssue)
		}
		var project Project
		if err := tx.First(&project, "id = ?", parent.ProjectID).Error; err != nil {
			return err
		}
		var maxNumber int64
		if err := tx.Model(&Issue{}).Select("coalesce(max(number),0)").Scan(&maxNumber).Error; err != nil {
			return err
		}
		now := time.Now()
		children := make([]Issue, 0, len(input.Children))
		for _, item := range input.Children {
			maxNumber++
			child := Issue{
				ID: nextID("issue"), Number: maxNumber, Identifier: fmt.Sprintf("%s-%04d", project.Key, maxNumber),
				ProjectID: parent.ProjectID, ParentID: parent.ID, Title: item.Title, Description: item.Description,
				AcceptanceCriteria: item.AcceptanceCriteria, Status: "todo", Priority: item.Priority,
				WorkMode: parent.WorkMode, ExecutionPhase: "active", RequestDepth: parent.RequestDepth + 1,
				AssigneeAgentID: item.AgentID, Workspace: parent.Workspace, Context: parent.Context,
				Constraints: parent.Constraints, CreatedBy: actorAgentID, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&child).Error; err != nil {
				return err
			}
			children = append(children, child)
		}
		for index, item := range input.Children {
			for _, dependency := range item.DependsOn {
				relation := IssueRelation{ID: nextID("relation"), IssueID: children[dependency-1].ID, RelatedIssueID: children[index].ID, Type: "blocks", CreatedAt: now, UpdatedAt: now}
				if err := tx.Create(&relation).Error; err != nil {
					return err
				}
			}
			// Every child blocks its parent. This is independent from the hierarchy edge.
			parentBlocker := IssueRelation{ID: nextID("relation"), IssueID: children[index].ID, RelatedIssueID: parent.ID, Type: "blocks", CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&parentBlocker).Error; err != nil {
				return err
			}
		}
		childIDs := make([]string, len(children))
		for index := range children {
			childIDs[index] = children[index].ID
		}
		decomposition := IssueDecomposition{ID: nextID("decomposition"), ParentIssueID: parent.ID, SourceExecutionID: executionID, RequestKey: input.RequestKey, Summary: input.Summary, ChildIDs: childIDs, CreatedAt: now}
		if err := tx.Create(&decomposition).Error; err != nil {
			return err
		}
		if err := tx.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{
			"status": "in_progress", "execution_phase": "waiting_children", "checkout_execution_id": "",
			"current_execution_id": executionID, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		output.Decomposition = decomposition
		output.Children = children
		return nil
	})
	if err != nil {
		return DecompositionResult{}, err
	}
	s.changedLocked()
	return output, nil
}
