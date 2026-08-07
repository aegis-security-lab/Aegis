package control

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

const maxIssueDepth = 4 // legacy default retained for existing tests and configurations

// CreateSubIssues is the durable control-plane operation behind the Pi tool.
// The request key makes retries from the same execution idempotent.
func (s *Store) CreateSubIssues(parentID, executionID, actorAgentID string, input DecomposeIssueInput) (DecompositionResult, error) {
	input.RequestKey = strings.TrimSpace(input.RequestKey)
	input.Summary = strings.TrimSpace(input.Summary)
	if input.RequestKey == "" || len(input.RequestKey) > 120 {
		return DecompositionResult{}, errors.New("requestKey 必填且不能超过 120 个字符")
	}
	limits := s.Config()
	maxDepth, maxPerRequest, maxDirect := normalizeDecompositionLimits(limits.MaxIssueDepth, limits.MaxChildrenPerRequest, limits.MaxDirectChildren)
	if len(input.Children) < 2 || len(input.Children) > maxPerRequest {
		return DecompositionResult{}, fmt.Errorf("每次必须创建 2-%d 个子 Issues", maxPerRequest)
	}
	for index := range input.Children {
		item := &input.Children[index]
		var err error
		item.Title, err = normalizeIssueTitle(item.Title)
		if err != nil {
			return DecompositionResult{}, fmt.Errorf("子 Issue %d: %w", index+1, err)
		}
		item.Description = strings.TrimSpace(item.Description)
		item.Objective = strings.TrimSpace(item.Objective)
		item.AgentID = strings.TrimSpace(item.AgentID)
		if item.Objective == "" {
			return DecompositionResult{}, fmt.Errorf("子 Issue %d 缺少目标", index+1)
		}
		item.Priority = normalizeIssuePriority(item.Priority)
		if !slices.Contains([]string{"high", "middle", "low"}, item.Priority) {
			item.Priority = "middle"
		}
		if len(item.DependsOn) > 0 {
			return DecompositionResult{}, fmt.Errorf("子 Issue %d 不能设置 dependsOn；子 Issues 必须彼此独立并可立即调度", index+1)
		}
		agent, err := s.chooseAgent(Issue{Title: item.Title, Description: item.Description, Objective: item.Objective, AssigneeAgentID: item.AgentID})
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
		if parent.RequestDepth >= maxDepth {
			return fmt.Errorf("Issue 已达到最大拆解深度 %d", maxDepth)
		}
		var execution Execution
		if err := tx.First(&execution, "id = ? AND issue_id = ? AND agent_id = ?", executionID, parent.ID, actorAgentID).Error; err != nil {
			return errors.New("execution ownership mismatch")
		}
		if !slices.Contains([]string{"starting", "running", "waiting_approval"}, execution.Status) {
			return errors.New("execution is no longer active")
		}
		ownsCheckout := parent.Status == "in_progress" && parent.AssigneeAgentID == actorAgentID && parent.CheckoutExecutionID == executionID
		// An acceptance "abandoned" decision records the Issue as cancelled, but
		// it is still a completed workflow outcome rather than an operator/task
		// cancellation. A later operator comment may add follow-up deliverables
		// (for example, generating a report from the collected evidence), so the
		// original owner must be able to reopen it from that active continuation.
		canReopenCompleted := (slices.Contains([]string{"done", "in_review"}, parent.Status) ||
			(parent.Status == "cancelled" && parent.ObjectiveAbandoned)) && parent.AssigneeAgentID == actorAgentID
		if !ownsCheckout && !canReopenCompleted {
			return errors.New("当前 Execution 既不持有 Issue checkout，也不能重新打开这个已完成 Issue")
		}
		var existingCount int64
		if err := tx.Model(&Issue{}).Where("parent_id = ?", parent.ID).Count(&existingCount).Error; err != nil {
			return err
		}
		if existingCount+int64(len(input.Children)) > int64(maxDirect) {
			return fmt.Errorf("父 Issue 最多允许 %d 个直属子 Issues", maxDirect)
		}
		var project Project
		if err := tx.First(&project, "id = ?", parent.ProjectID).Error; err != nil {
			return err
		}
		var maxNumber int64
		if err := tx.Model(&Issue{}).Select("coalesce(max(number),0)").Scan(&maxNumber).Error; err != nil {
			return err
		}
		for _, item := range input.Children {
			if err := s.validateAgentAssignmentWithDBLocked(tx, item.AgentID); err != nil {
				return err
			}
		}
		now := time.Now()
		root, rootErr := taskRootWithDB(tx, parent)
		if rootErr != nil {
			return rootErr
		}
		creatorID := actorAgentID
		var actorExecution Execution
		if err := tx.First(&actorExecution, "id = ?", executionID).Error; err == nil && actorExecution.TaskAgentID != "" {
			creatorID = actorExecution.TaskAgentID
		}
		validationMode, maxValidationAttempts := normalizeValidationPolicy(parent.ValidationMode, parent.MaxValidationAttempts)
		validationDisabled := parent.ValidationDisabled || strings.TrimSpace(parent.Objective) == ""
		children := make([]Issue, 0, len(input.Children))
		for _, item := range input.Children {
			maxNumber++
			identity, identityErr := claimTaskAgentTx(tx, root.ID, item.AgentID, "")
			if identityErr != nil {
				return identityErr
			}
			child := Issue{
				ID: nextID("issue"), Number: maxNumber, Identifier: fmt.Sprintf("%s-%04d", project.Key, maxNumber),
				ProjectID: parent.ProjectID, ParentID: parent.ID, Title: item.Title, Description: item.Description,
				Objective: item.Objective, Status: "todo", Priority: item.Priority,
				WorkMode: parent.WorkMode, ExecutionPhase: "active", RequestDepth: parent.RequestDepth + 1,
				ValidationMode: validationMode, MaxValidationAttempts: maxValidationAttempts, ValidationDisabled: validationDisabled,
				AssigneeAgentID: item.AgentID, AssigneeTaskAgentID: identity.ID, Workspace: parent.Workspace, ContainerProfileID: parent.ContainerProfileID, ContainerID: parent.ContainerID,
				Constraints: parent.Constraints, CreatedBy: creatorID, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&child).Error; err != nil {
				return err
			}
			children = append(children, child)
		}
		// Parent/child completion is coordinated by IssueChildWait. Child Issues
		// deliberately have no dependency or blocks edges, so every child can be
		// dispatched independently as soon as a worker is available.
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
			"current_execution_id": executionID, "completed_at": nil, "cancelled_at": nil,
			"objective_abandoned": false, "abandonment_reason": "", "abandoned_at": nil,
			"error": "", "updated_at": now,
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
