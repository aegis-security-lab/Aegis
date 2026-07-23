package control

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var activeExecutionStatuses = []string{"queued", "starting", "running", "waiting_approval"}

// cancelTaskTree atomically closes every unfinished Issue and active runtime
// record below a top-level task. Completed history remains immutable.
func (s *Store) cancelTaskTree(taskID, reason string) (TaskCancellationResult, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "任务已由操作员取消"
	}

	var result TaskCancellationResult
	var issueIDs []string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var task Issue
		if err := tx.First(&task, "id = ?", taskID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("task not found")
			}
			return err
		}
		if task.ParentID != "" {
			return errors.New("只能取消顶层任务；子 Issue 请通过所属任务统一取消")
		}
		if task.Status == "done" {
			return errors.New("已完成的任务不能取消")
		}

		var projectIssues []Issue
		if err := tx.Where("project_id = ?", task.ProjectID).Find(&projectIssues).Error; err != nil {
			return err
		}
		children := make(map[string][]string)
		for _, issue := range projectIssues {
			children[issue.ParentID] = append(children[issue.ParentID], issue.ID)
		}
		queue := []string{task.ID}
		seen := make(map[string]bool)
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if seen[id] {
				continue
			}
			seen[id] = true
			issueIDs = append(issueIDs, id)
			queue = append(queue, children[id]...)
		}
		if len(issueIDs) == 0 {
			return errors.New("task tree is empty")
		}

		now := time.Now()
		issueUpdate := tx.Model(&Issue{}).
			Where("id IN ? AND status NOT IN ?", issueIDs, terminalIssueStatuses).
			Updates(map[string]any{
				"status":                "cancelled",
				"execution_phase":       "completed",
				"checkout_execution_id": "",
				"cancelled_at":          &now,
				"updated_at":            now,
			})
		if issueUpdate.Error != nil {
			return issueUpdate.Error
		}

		var activeExecutions []Execution
		if err := tx.Where("issue_id IN ? AND status IN ?", issueIDs, activeExecutionStatuses).Find(&activeExecutions).Error; err != nil {
			return err
		}
		executionIDs := make([]string, 0, len(activeExecutions))
		for _, execution := range activeExecutions {
			executionIDs = append(executionIDs, execution.ID)
		}
		executionUpdate := tx.Model(&Execution{}).
			Where("id IN ?", executionIDs).
			Updates(map[string]any{
				"status":       "cancelled",
				"error":        reason,
				"current_tool": "",
				"finished_at":  &now,
				"pid":          0,
				"updated_at":   now,
			})
		if executionUpdate.Error != nil {
			return executionUpdate.Error
		}

		approvalDelete := tx.
			Where("issue_id IN ? AND status = ?", issueIDs, "pending").
			Delete(&Approval{})
		if approvalDelete.Error != nil {
			return approvalDelete.Error
		}
		wakeupUpdate := tx.Model(&AgentWakeup{}).
			Where("issue_id IN ? AND status IN ?", issueIDs, []string{"queued", "delivered"}).
			Updates(map[string]any{"status": "cancelled", "error": reason, "completed_at": &now})
		if wakeupUpdate.Error != nil {
			return wakeupUpdate.Error
		}
		if err := tx.Model(&Message{}).
			Where("issue_id IN ? AND streaming = ?", issueIDs, true).
			Updates(map[string]any{"streaming": false, "updated_at": now}).Error; err != nil {
			return err
		}

		detail := fmt.Sprintf("%s；任务树共 %d 个 Issues，取消 %d 个未完成 Issues 与 %d 个活跃 Executions。", reason, len(issueIDs), issueUpdate.RowsAffected, len(activeExecutions))
		if err := tx.Create(&ExecutionEvent{
			ID: nextID("event"), IssueID: task.ID, Type: "cancellation",
			Title: "任务树已取消", Detail: detail, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		if err := tx.First(&task, "id = ?", task.ID).Error; err != nil {
			return err
		}
		result = TaskCancellationResult{
			Task: task, TotalIssues: len(issueIDs), CancelledIssues: issueUpdate.RowsAffected,
			CancelledExecutions: int64(len(activeExecutions)), RemovedApprovals: approvalDelete.RowsAffected,
			CancelledWakeups: wakeupUpdate.RowsAffected,
		}
		return nil
	})
	if err != nil {
		return TaskCancellationResult{}, nil, err
	}
	s.changedLocked()
	return result, issueIDs, nil
}

func (s *Store) taskRoot(issue Issue) (Issue, error) {
	current := issue
	seen := map[string]bool{current.ID: true}
	for current.ParentID != "" {
		if seen[current.ParentID] {
			return Issue{}, errors.New("Issue 层级存在循环")
		}
		seen[current.ParentID] = true
		var parent Issue
		if err := s.db.First(&parent, "id = ?", current.ParentID).Error; err != nil {
			return Issue{}, err
		}
		current = parent
	}
	return current, nil
}

func (s *Store) belongsToCancelledTask(issue Issue) bool {
	root, err := s.taskRoot(issue)
	return err == nil && root.Status == "cancelled"
}
