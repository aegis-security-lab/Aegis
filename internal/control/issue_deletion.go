package control

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gorm.io/gorm"
)

// DeleteIssueTree permanently removes one visible Board Issue and every child
// beneath it. Running work must be archived/cancelled first so deleting Board
// data can never leave an untracked Agent process behind.
func (s *Store) DeleteIssueTree(issueID string) (DeleteIssueResult, error) {
	var root Issue
	if err := s.db.First(&root, "id = ? AND hidden = ?", strings.TrimSpace(issueID), false).Error; err != nil {
		return DeleteIssueResult{}, errors.New("Issue 不存在")
	}

	var allIssues []Issue
	if err := s.db.Select("id", "parent_id", "task_source_id").Where("hidden = ?", false).Find(&allIssues).Error; err != nil {
		return DeleteIssueResult{}, err
	}
	issueSet := map[string]bool{root.ID: true}
	for changed := true; changed; {
		changed = false
		for _, issue := range allIssues {
			if issue.ParentID != "" && issueSet[issue.ParentID] && !issueSet[issue.ID] {
				issueSet[issue.ID] = true
				changed = true
			}
		}
	}
	issueIDs := make([]string, 0, len(issueSet))
	for _, issue := range allIssues {
		if issueSet[issue.ID] {
			issueIDs = append(issueIDs, issue.ID)
		}
	}

	var executions []Execution
	if err := s.db.Select("id", "status").Where("issue_id IN ?", issueIDs).Find(&executions).Error; err != nil {
		return DeleteIssueResult{}, err
	}
	executionIDs := make([]string, 0, len(executions))
	for _, execution := range executions {
		if slices.Contains(activeExecutionStatuses, execution.Status) {
			return DeleteIssueResult{}, errors.New("Issue 或其子 Issue 仍在执行，请先取消或归档后再删除")
		}
		executionIDs = append(executionIDs, execution.ID)
	}

	taskIDs := []string{}
	taskAgentRootIDs := []string{}
	if root.ParentID == "" && strings.TrimSpace(root.TaskSourceID) != "" {
		taskAgentRootIDs = append(taskAgentRootIDs, root.ID)
		var remainingRoots int64
		if err := s.db.Model(&Issue{}).Where("task_source_id = ? AND parent_id = '' AND id <> ?", root.TaskSourceID, root.ID).Count(&remainingRoots).Error; err != nil {
			return DeleteIssueResult{}, err
		}
		if remainingRoots == 0 {
			taskIDs = append(taskIDs, root.TaskSourceID)
		}
	}
	var containers []ContainerInstance
	if len(taskIDs) > 0 {
		if err := s.db.Where("task_id IN ?", taskIDs).Find(&containers).Error; err != nil {
			return DeleteIssueResult{}, err
		}
		for _, container := range containers {
			if err := removeContainerRuntime(container); err != nil {
				return DeleteIssueResult{}, err
			}
		}
	}

	storagePaths := []string{}
	var outputAttachments []IssueAttachment
	if err := s.db.Select("storage_path").Where("issue_id IN ?", issueIDs).Find(&outputAttachments).Error; err != nil {
		return DeleteIssueResult{}, err
	}
	for _, attachment := range outputAttachments {
		storagePaths = append(storagePaths, attachment.StoragePath)
	}
	var inputAttachments []InputAttachment
	inputQuery := s.db.Where("issue_id IN ? AND task_id = ''", issueIDs)
	if len(executionIDs) > 0 {
		inputQuery = inputQuery.Or("execution_id IN ?", executionIDs)
	}
	if len(taskIDs) > 0 {
		inputQuery = inputQuery.Or("task_id IN ?", taskIDs)
	}
	if err := inputQuery.Find(&inputAttachments).Error; err != nil {
		return DeleteIssueResult{}, err
	}
	inputAttachmentIDs := make([]string, 0, len(inputAttachments))
	for _, attachment := range inputAttachments {
		inputAttachmentIDs = append(inputAttachmentIDs, attachment.ID)
		storagePaths = append(storagePaths, attachment.StoragePath)
	}

	result := DeleteIssueResult{IssueID: root.ID}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("issue_id IN ? OR related_issue_id IN ?", issueIDs, issueIDs).Delete(&IssueRelation{}).Error; err != nil {
			return err
		}
		for _, model := range []any{&ConciergeConversation{}, &IssueValidation{}, &ExecutionEvent{}, &ExecutionProgress{}, &Message{}, &Approval{}, &IssueComment{}, &IssueAttachment{}, &AgentWakeup{}, &RelayMessage{}} {
			if err := tx.Where("issue_id IN ?", issueIDs).Delete(model).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("parent_issue_id IN ?", issueIDs).Delete(&IssueDecomposition{}).Error; err != nil {
			return err
		}
		if err := tx.Where("parent_issue_id IN ?", issueIDs).Delete(&IssueChildWait{}).Error; err != nil {
			return err
		}
		if len(executionIDs) > 0 {
			for _, model := range []any{&ExecutionEvent{}, &ExecutionProgress{}, &Message{}, &Approval{}, &IssueComment{}, &IssueAttachment{}} {
				if err := tx.Where("execution_id IN ?", executionIDs).Delete(model).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("source_execution_id IN ? OR validation_execution_id IN ?", executionIDs, executionIDs).Delete(&IssueValidation{}).Error; err != nil {
				return err
			}
			if err := tx.Where("source_execution_id IN ?", executionIDs).Delete(&IssueDecomposition{}).Error; err != nil {
				return err
			}
			if err := tx.Where("source_execution_id IN ?", executionIDs).Delete(&IssueChildWait{}).Error; err != nil {
				return err
			}
			executions := tx.Delete(&Execution{}, "id IN ?", executionIDs)
			if executions.Error != nil {
				return executions.Error
			}
			result.DeletedExecutions = executions.RowsAffected
		}
		if len(inputAttachmentIDs) > 0 {
			if err := tx.Delete(&InputAttachment{}, "id IN ?", inputAttachmentIDs).Error; err != nil {
				return err
			}
		}
		if len(taskAgentRootIDs) > 0 {
			if err := tx.Delete(&TaskAgent{}, "task_id IN ?", taskAgentRootIDs).Error; err != nil {
				return err
			}
		}
		if len(taskIDs) > 0 {
			if err := tx.Delete(&ContainerInstance{}, "task_id IN ?", taskIDs).Error; err != nil {
				return err
			}
			tasks := tx.Delete(&Task{}, "id IN ?", taskIDs)
			if tasks.Error != nil {
				return tasks.Error
			}
			result.DeletedTasks = tasks.RowsAffected
		}
		issues := tx.Delete(&Issue{}, "id IN ?", issueIDs)
		if issues.Error != nil {
			return issues.Error
		}
		result.DeletedIssues = issues.RowsAffected
		return nil
	})
	if err != nil {
		return DeleteIssueResult{}, err
	}

	for _, storagePath := range uniqueStrings(storagePaths) {
		storagePath = strings.TrimSpace(storagePath)
		if storagePath == "" {
			continue
		}
		absolute := storagePath
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(s.dataDir, filepath.Clean(absolute))
		}
		absolute, _ = filepath.Abs(absolute)
		dataDir, _ := filepath.Abs(s.dataDir)
		if strings.HasPrefix(absolute, dataDir+string(os.PathSeparator)) {
			_ = os.RemoveAll(filepath.Dir(absolute))
		}
	}
	s.notify()
	return result, nil
}
