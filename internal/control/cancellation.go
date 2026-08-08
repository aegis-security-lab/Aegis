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
func (s *Store) cancelTaskTree(taskID, reason string) (TaskCancellationResult, []string, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "任务已由操作员取消"
	}

	var result TaskCancellationResult
	var issueIDs []string
	var rootIDs []string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		task, roots, issues, scopeErr := taskIssueScopeWithDB(tx, taskID)
		if scopeErr != nil {
			return scopeErr
		}
		rootIDs = issueIDsOf(roots)
		issueIDs = issueIDsOf(issues)
		if len(issueIDs) == 0 {
			return errors.New("任务没有可取消的 Issue")
		}
		allRootsDone := true
		for _, root := range roots {
			if root.Status != "done" {
				allRootsDone = false
				break
			}
		}
		if allRootsDone {
			return errors.New("已完成的任务不能取消")
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
		for _, rootID := range rootIDs {
			if err := tx.Create(&ExecutionEvent{
				ID: nextID("event"), IssueID: rootID, Type: "cancellation",
				Title: "任务已取消", Detail: detail, CreatedAt: now,
			}).Error; err != nil {
				return err
			}
		}
		result = TaskCancellationResult{
			Task: task, TotalIssues: len(issueIDs), CancelledIssues: issueUpdate.RowsAffected,
			CancelledExecutions: int64(len(activeExecutions)), RemovedApprovals: approvalDelete.RowsAffected,
			CancelledWakeups: wakeupUpdate.RowsAffected,
		}
		return nil
	})
	if err != nil {
		return TaskCancellationResult{}, nil, nil, err
	}
	s.changedLocked()
	return result, issueIDs, rootIDs, nil
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
