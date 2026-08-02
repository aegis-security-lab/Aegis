package control

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// isRunnableAgent distinguishes reusable task roles from internal control
// agents. Runtime people are represented exclusively by TaskAgent.
func isRunnableAgent(agent AgentDefinition) bool {
	return agent.Enabled && !agent.Internal && agent.ID != conciergeAgentID && agent.Category != "concierge"
}

func agentByID(agents []AgentDefinition, id string) (AgentDefinition, bool) {
	for _, agent := range agents {
		if agent.ID == id {
			return agent, true
		}
	}
	return AgentDefinition{}, false
}

func (s *Store) assignableAgentType(agentID string) (AgentDefinition, error) {
	agent, err := s.GetAgent(strings.TrimSpace(agentID))
	if err != nil || !isRunnableAgent(agent) {
		return AgentDefinition{}, errors.New("任务负责人必须是已启用、可执行的 Agent 类型")
	}
	return agent, nil
}

func (s *Store) validateAgentAssignmentWithDBLocked(_ *gorm.DB, agentID string) error {
	agent, ok := agentByID(s.agents, strings.TrimSpace(agentID))
	if !ok || !isRunnableAgent(agent) {
		return errors.New("任务负责人必须是已启用、可执行的 Agent 类型")
	}
	return nil
}

// ValidateDelegation validates role availability only. A task may lease the
// same Agent type more than once; each lease receives a different TaskAgent.
func (s *Store) ValidateDelegation(_, targetAgentID string) error {
	_, err := s.assignableAgentType(targetAgentID)
	return err
}

func taskRootWithDB(db *gorm.DB, issue Issue) (Issue, error) {
	current := issue
	seen := map[string]bool{current.ID: true}
	for current.ParentID != "" {
		if seen[current.ParentID] {
			return Issue{}, errors.New("Issue 层级存在循环")
		}
		seen[current.ParentID] = true
		var parent Issue
		if err := db.First(&parent, "id = ?", current.ParentID).Error; err != nil {
			return Issue{}, err
		}
		current = parent
	}
	return current, nil
}

func activeTaskAgentExecutionsInTask(db *gorm.DB, taskAgentID, taskID string) (int64, error) {
	var executions []Execution
	if err := db.Where("task_agent_id = ? AND status IN ?", taskAgentID, activeExecutionStatuses).Find(&executions).Error; err != nil {
		return 0, err
	}
	var count int64
	for _, execution := range executions {
		var issue Issue
		if err := db.First(&issue, "id = ?", execution.IssueID).Error; err != nil {
			continue
		}
		root, err := taskRootWithDB(db, issue)
		if err == nil && root.ID == taskID {
			count++
		}
	}
	return count, nil
}
