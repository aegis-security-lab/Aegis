package control

import (
	"errors"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// taskIssueScopeWithDB resolves the durable Task aggregate and every Issue in
// all of its root trees. Task-facing APIs use this scope instead of selecting
// an arbitrary or latest root Issue as the Task representative.
func taskIssueScopeWithDB(db *gorm.DB, taskID string) (Task, []Issue, []Issue, error) {
	if db == nil {
		return Task{}, nil, nil, errors.New("task store is unavailable")
	}
	taskID = strings.TrimSpace(taskID)
	var task Task
	if taskID == "" || db.First(&task, "id = ?", taskID).Error != nil {
		return Task{}, nil, nil, errors.New("task not found")
	}
	var roots []Issue
	if err := db.Where("task_source_id = ? AND parent_id = ''", task.ID).Order("created_at asc, number asc").Find(&roots).Error; err != nil {
		return Task{}, nil, nil, err
	}
	if len(roots) == 0 {
		return task, []Issue{}, []Issue{}, nil
	}
	var allIssues []Issue
	if err := db.Find(&allIssues).Error; err != nil {
		return Task{}, nil, nil, err
	}
	childrenByParent := make(map[string][]Issue)
	for _, issue := range allIssues {
		if issue.ParentID != "" {
			childrenByParent[issue.ParentID] = append(childrenByParent[issue.ParentID], issue)
		}
	}
	for parentID := range childrenByParent {
		sort.Slice(childrenByParent[parentID], func(i, j int) bool {
			return childrenByParent[parentID][i].Number < childrenByParent[parentID][j].Number
		})
	}
	issues := make([]Issue, 0, len(roots))
	seen := make(map[string]bool)
	queue := append([]Issue(nil), roots...)
	for len(queue) > 0 {
		issue := queue[0]
		queue = queue[1:]
		if seen[issue.ID] {
			continue
		}
		seen[issue.ID] = true
		issues = append(issues, issue)
		queue = append(queue, childrenByParent[issue.ID]...)
	}
	return task, roots, issues, nil
}
