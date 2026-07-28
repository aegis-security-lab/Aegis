package control

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrEmployeeBusy = errors.New("员工当前不可指派")

func isEmployeeAgent(agent AgentDefinition) bool {
	return agent.Enabled && !agent.Internal && agent.ID != conciergeAgentID && agent.Category != "concierge"
}

func (s *Store) assignableEmployee(agentID string) (AgentDefinition, error) {
	agent, err := s.GetAgent(strings.TrimSpace(agentID))
	if err != nil || !isEmployeeAgent(agent) {
		return AgentDefinition{}, errors.New("任务负责人必须是已启用的具体员工，不能使用岗位或内部 Agent")
	}
	return agent, nil
}

// employeeAssignmentConflict checks ownership only inside one top-level Task.
// An empty scope means a brand-new Task, where every enabled employee is
// assignable because it will receive an independent Session.
func employeeAssignmentConflict(db *gorm.DB, agentID, scopeIssueID string) (EmployeeAvailability, error) {
	availability := EmployeeAvailability{AgentID: strings.TrimSpace(agentID), Available: true}
	scopeIssueID = strings.TrimSpace(scopeIssueID)
	if scopeIssueID == "" {
		return availability, nil
	}
	var scope Issue
	if err := db.First(&scope, "id = ?", scopeIssueID).Error; err != nil {
		return EmployeeAvailability{}, err
	}
	scopeRoot, err := taskRootWithDB(db, scope)
	if err != nil {
		return EmployeeAvailability{}, err
	}
	query := db.Where("hidden = ? AND assignee_agent_id = ? AND status NOT IN ?", false, availability.AgentID, terminalIssueStatuses)
	query = query.Where("id <> ?", scopeIssueID)
	var issues []Issue
	if err := query.Order("CASE status WHEN 'in_progress' THEN 0 WHEN 'in_review' THEN 1 WHEN 'blocked' THEN 2 WHEN 'todo' THEN 3 ELSE 4 END, created_at asc").Find(&issues).Error; err != nil {
		return EmployeeAvailability{}, err
	}
	for _, issue := range issues {
		root, rootErr := taskRootWithDB(db, issue)
		if rootErr != nil || root.ID != scopeRoot.ID {
			continue
		}
		availability.Available = false
		availability.CurrentIssueID = issue.ID
		availability.CurrentIssueIdentifier = issue.Identifier
		availability.CurrentIssueTitle = issue.Title
		availability.Activity = "issue"
		return availability, nil
	}

	var executions []Execution
	if err := db.Where("agent_id = ? AND status IN ? AND issue_id <> ?", availability.AgentID, activeExecutionStatuses, scopeIssueID).Order("started_at asc").Find(&executions).Error; err != nil {
		return EmployeeAvailability{}, err
	}
	for _, execution := range executions {
		var issue Issue
		if issueErr := db.First(&issue, "id = ?", execution.IssueID).Error; issueErr == nil {
			root, rootErr := taskRootWithDB(db, issue)
			if rootErr != nil || root.ID != scopeRoot.ID {
				continue
			}
			availability.Available = false
			availability.Activity = "session"
			availability.CurrentIssueID = execution.IssueID
			availability.CurrentIssueIdentifier = issue.Identifier
			availability.CurrentIssueTitle = issue.Title
			return availability, nil
		}
	}
	return availability, nil
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

func activeEmployeeExecutionsInTask(db *gorm.DB, agentID, taskIssueID string) (int64, error) {
	var executions []Execution
	if err := db.Where("agent_id = ? AND status IN ?", agentID, activeExecutionStatuses).Find(&executions).Error; err != nil {
		return 0, err
	}
	var count int64
	for _, execution := range executions {
		var issue Issue
		if err := db.First(&issue, "id = ?", execution.IssueID).Error; err != nil {
			continue
		}
		root, err := taskRootWithDB(db, issue)
		if err == nil && root.ID == taskIssueID {
			count++
		}
	}
	return count, nil
}

func (s *Store) validateEmployeeAssignmentLocked(agentID, excludeIssueID string) error {
	return s.validateEmployeeAssignmentWithDBLocked(s.db, agentID, excludeIssueID)
}

func (s *Store) validateEmployeeAssignmentWithDBLocked(db *gorm.DB, agentID, excludeIssueID string) error {
	agent, ok := agentByID(s.agents, strings.TrimSpace(agentID))
	if !ok || !isEmployeeAgent(agent) {
		return errors.New("任务负责人必须是已启用的具体员工，不能使用岗位或内部 Agent")
	}
	availability, err := employeeAssignmentConflict(db, agent.ID, excludeIssueID)
	if err != nil {
		return err
	}
	if availability.Available {
		return nil
	}
	if availability.CurrentIssueIdentifier != "" {
		return fmt.Errorf("%w：%s正在处理 %s · %s，任务结束前不能再指派其他任务", ErrEmployeeBusy, agent.Name, availability.CurrentIssueIdentifier, availability.CurrentIssueTitle)
	}
	return fmt.Errorf("%w：%s的固定 Session 正在工作，结束前不能再指派其他任务", ErrEmployeeBusy, agent.Name)
}

func (s *Store) employeeAvailabilitiesLocked() []EmployeeAvailability {
	items := make([]EmployeeAvailability, 0, len(s.agents))
	indexes := make(map[string]int, len(s.agents))
	for _, agent := range s.agents {
		if !isEmployeeAgent(agent) {
			continue
		}
		indexes[agent.ID] = len(items)
		items = append(items, EmployeeAvailability{AgentID: agent.ID, Available: true})
	}
	var issues []Issue
	if err := s.db.Where("hidden = ? AND assignee_agent_id <> ? AND status NOT IN ?", false, "", terminalIssueStatuses).
		Order("CASE status WHEN 'in_progress' THEN 0 WHEN 'in_review' THEN 1 WHEN 'blocked' THEN 2 WHEN 'todo' THEN 3 ELSE 4 END, created_at asc").Find(&issues).Error; err != nil {
		for index := range items {
			items[index].Available = false
			items[index].Activity = "unknown"
		}
		return items
	}
	for _, issue := range issues {
		index, exists := indexes[issue.AssigneeAgentID]
		if !exists || !items[index].Available {
			continue
		}
		items[index].Available = false
		items[index].CurrentIssueID = issue.ID
		items[index].CurrentIssueIdentifier = issue.Identifier
		items[index].CurrentIssueTitle = issue.Title
		items[index].Activity = "issue"
	}
	var executions []Execution
	if err := s.db.Where("agent_id <> ? AND status IN ?", "", activeExecutionStatuses).Order("started_at asc").Find(&executions).Error; err != nil {
		return items
	}
	for _, execution := range executions {
		index, exists := indexes[execution.AgentID]
		if !exists || !items[index].Available {
			continue
		}
		items[index].Available = false
		items[index].CurrentIssueID = execution.IssueID
		items[index].Activity = "session"
	}
	return items
}

func (s *Store) EmployeeAvailabilities() []EmployeeAvailability {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.employeeAvailabilitiesLocked()
}

func availabilityByAgent(items []EmployeeAvailability) map[string]EmployeeAvailability {
	result := make(map[string]EmployeeAvailability, len(items))
	for _, item := range items {
		result[item.AgentID] = item
	}
	return result
}

// ensureEmployeeSession returns the single durable Pi conversation owned by an
// employee. Issues reuse the same SessionID and remain separate audit records.
func (s *Store) ensureEmployeeSession(agentID string) (EmployeeSession, error) {
	return s.ensureEmployeeSessionForIssue(agentID, "")
}

func (s *Store) ensureEmployeeSessionForIssue(agentID, issueID string) (EmployeeSession, error) {
	agentID = strings.TrimSpace(agentID)
	issueID = strings.TrimSpace(issueID)
	if agentID == "" {
		return EmployeeSession{}, errors.New("Agent 不能为空")
	}
	var session EmployeeSession
	err := s.db.Where("agent_id = ? AND issue_id = ?", agentID, issueID).First(&session).Error
	if err == nil {
		if updateErr := s.db.Model(&session).Updates(map[string]any{"status": "active", "updated_at": time.Now()}).Error; updateErr != nil {
			return EmployeeSession{}, updateErr
		}
		return s.EmployeeSessionForIssue(agentID, issueID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return EmployeeSession{}, err
	}
	now := time.Now()
	session = EmployeeSession{
		ID: nextID("employee-session"), AgentID: agentID, IssueID: issueID,
		SessionID: nextID("pi-session"),
		Status:    "active", CreatedAt: now, UpdatedAt: now,
	}
	if err = s.db.Create(&session).Error; err != nil {
		if lookupErr := s.db.Where("agent_id = ? AND issue_id = ?", agentID, issueID).First(&session).Error; lookupErr == nil {
			return session, nil
		}
		return EmployeeSession{}, err
	}
	return session, nil
}

func (s *Store) ensureEmployeeIssueSession(agent AgentDefinition, issueID string) (EmployeeSession, error) {
	if !isEmployeeAgent(agent) {
		return EmployeeSession{}, errors.New("employee not found")
	}
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return s.ensureEmployeeWorkspace(agent)
	}
	issue, err := s.GetIssue(issueID)
	if err != nil || issue.Hidden {
		return EmployeeSession{}, errors.New("Issue not found")
	}
	return s.ensureEmployeeSessionForIssue(agent.ID, issueID)
}

// ensureEmployeeWorkspace creates the private runtime anchor for an employee.
// It is deliberately hidden from Board, so direct chat and Relay notifications
// never become Issue activity.
func (s *Store) ensureEmployeeWorkspace(agent AgentDefinition) (EmployeeSession, error) {
	if !isEmployeeAgent(agent) {
		return EmployeeSession{}, errors.New("employee not found")
	}
	session, err := s.ensureEmployeeSession(agent.ID)
	if err != nil {
		return EmployeeSession{}, err
	}
	if session.HomeIssueID != "" {
		var existing Issue
		if err = s.db.First(&existing, "id = ? AND hidden = ?", session.HomeIssueID, true).Error; err == nil {
			if strings.TrimSpace(existing.ContainerProfileID) == "" && s.config.Provider != "test" {
				profile, profileErr := s.DefaultContainerProfile()
				if profileErr != nil {
					return EmployeeSession{}, profileErr
				}
				if err = s.db.Model(&Issue{}).Where("id = ?", existing.ID).Updates(map[string]any{
					"container_profile_id": profile.ID, "updated_at": time.Now(),
				}).Error; err != nil {
					return EmployeeSession{}, err
				}
			}
			return session, nil
		}
	}
	var project Project
	if err = s.db.Order("created_at asc").First(&project).Error; err != nil {
		return EmployeeSession{}, err
	}
	now := time.Now()
	containerProfileID := ""
	if s.config.Provider != "test" {
		profile, profileErr := s.DefaultContainerProfile()
		if profileErr != nil {
			return EmployeeSession{}, profileErr
		}
		containerProfileID = profile.ID
	}
	homeID := "employee-home-" + agent.ID
	var home Issue
	if err = s.db.First(&home, "id = ?", homeID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		home = Issue{
			ID: homeID, Number: -now.UnixNano(), Identifier: "EMP-" + strings.ToUpper(agent.ID),
			ProjectID: project.ID, Title: agent.Name + " 工作台", Description: "员工长期会话的内部工作台。",
			Status: "done", Priority: "medium", WorkMode: "autonomous", ExecutionPhase: "completed",
			AssigneeAgentID: agent.ID, ValidationDisabled: true, Workspace: s.config.Workspace,
			ContainerProfileID: containerProfileID,
			CreatedBy:          "system", Hidden: true, CreatedAt: now, UpdatedAt: now,
		}
		if err = s.db.Create(&home).Error; err != nil {
			return EmployeeSession{}, err
		}
	} else if err != nil {
		return EmployeeSession{}, err
	}
	if err = s.db.Model(&EmployeeSession{}).Where("id = ?", session.ID).Updates(map[string]any{"home_issue_id": home.ID, "updated_at": now}).Error; err != nil {
		return EmployeeSession{}, err
	}
	return s.EmployeeSession(agent.ID)
}

func (s *Store) EmployeeSession(agentID string) (EmployeeSession, error) {
	return s.EmployeeSessionForIssue(agentID, "")
}

func (s *Store) EmployeeSessionForIssue(agentID, issueID string) (EmployeeSession, error) {
	var session EmployeeSession
	if err := s.db.Where("agent_id = ? AND issue_id = ?", strings.TrimSpace(agentID), strings.TrimSpace(issueID)).First(&session).Error; err != nil {
		return EmployeeSession{}, errors.New("employee session not found")
	}
	return session, nil
}

func (s *Store) EmployeeWorkspace(agentID string, taskIDs ...string) (EmployeeWorkspace, error) {
	watermark := time.Now()
	agent, err := s.GetAgent(agentID)
	if err != nil || !isEmployeeAgent(agent) {
		return EmployeeWorkspace{}, errors.New("employee not found")
	}
	if _, err = s.ensureEmployeeWorkspace(agent); err != nil {
		return EmployeeWorkspace{}, err
	}
	var sessions []EmployeeSession
	if err = s.db.Where("agent_id = ?", agent.ID).Order("updated_at desc").Find(&sessions).Error; err != nil {
		return EmployeeWorkspace{}, err
	}
	taskID := ""
	explicitGeneral := false
	if len(taskIDs) > 0 {
		taskID = strings.TrimSpace(taskIDs[0])
		if taskID == "general" {
			taskID = ""
			explicitGeneral = true
		}
	}
	var session EmployeeSession
	selectedSessionIDs := []string{}
	if taskID != "" {
		for _, candidate := range sessions {
			if candidate.IssueID == "" {
				continue
			}
			candidateIssue, issueErr := s.GetIssue(candidate.IssueID)
			if issueErr != nil {
				continue
			}
			root, rootErr := s.taskRoot(candidateIssue)
			if rootErr == nil && root.ID == taskID {
				if session.ID == "" {
					session = candidate
				}
				selectedSessionIDs = append(selectedSessionIDs, candidate.ID)
			}
		}
		if session.ID == "" {
			var assigned []Issue
			_ = s.db.Where("assignee_agent_id = ? AND hidden = ?", agent.ID, false).Order("updated_at desc").Find(&assigned).Error
			for _, candidate := range assigned {
				root, rootErr := s.taskRoot(candidate)
				if rootErr == nil && root.ID == taskID {
					session, err = s.ensureEmployeeIssueSession(agent, candidate.ID)
					if err == nil {
						sessions = append([]EmployeeSession{session}, sessions...)
						selectedSessionIDs = append(selectedSessionIDs, session.ID)
					}
					break
				}
			}
		}
	} else if !explicitGeneral {
		for _, candidate := range sessions {
			if candidate.IssueID != "" {
				session = candidate
				selectedSessionIDs = append(selectedSessionIDs, candidate.ID)
				break
			}
		}
		if session.ID == "" {
			session, err = s.EmployeeSession(agent.ID)
		}
	} else {
		session, err = s.EmployeeSession(agent.ID)
		if err == nil {
			selectedSessionIDs = append(selectedSessionIDs, session.ID)
		}
	}
	if err != nil {
		return EmployeeWorkspace{}, err
	}
	var executions []Execution
	if len(selectedSessionIDs) == 0 {
		selectedSessionIDs = append(selectedSessionIDs, session.ID)
	}
	if err = s.db.Where("employee_session_id IN ?", selectedSessionIDs).Order("started_at asc, id asc").Find(&executions).Error; err != nil {
		return EmployeeWorkspace{}, err
	}
	var messages []Message
	if err = s.db.Table("messages").Joins("JOIN executions ON executions.id = messages.execution_id").Where("executions.employee_session_id IN ?", selectedSessionIDs).Order("messages.created_at asc, messages.id asc").Select("messages.*").Find(&messages).Error; err != nil {
		return EmployeeWorkspace{}, err
	}
	var events []ExecutionEvent
	if err = s.db.Table("execution_events").Joins("JOIN executions ON executions.id = execution_events.execution_id").Where("executions.employee_session_id IN ?", selectedSessionIDs).Order("execution_events.created_at asc, execution_events.id asc").Select("execution_events.*").Find(&events).Error; err != nil {
		return EmployeeWorkspace{}, err
	}
	for index := range events {
		events[index] = compactExecutionEvent(events[index])
	}
	var attachments []IssueAttachment
	if err = s.db.Table("issue_attachments").Joins("JOIN executions ON executions.id = issue_attachments.execution_id").Where("executions.employee_session_id IN ?", selectedSessionIDs).Order("issue_attachments.created_at asc, issue_attachments.id asc").Select("issue_attachments.*").Find(&attachments).Error; err != nil {
		return EmployeeWorkspace{}, err
	}
	var issues []Issue
	if err = s.db.Where("assignee_agent_id = ? AND hidden = ?", agent.ID, false).Order("updated_at desc").Find(&issues).Error; err != nil {
		return EmployeeWorkspace{}, err
	}
	relay, err := s.RelayInbox(agent.ID)
	if err != nil {
		return EmployeeWorkspace{}, err
	}
	return EmployeeWorkspace{Agent: agent, Session: session, Sessions: sessions, Executions: executions, Messages: messages, Events: events, Attachments: attachments, Issues: issues, Relay: relay, Watermark: watermark}, nil
}

func (s *Store) EmployeeActivity(agentID string, after time.Time) (EmployeeActivity, error) {
	watermark := time.Now()
	agent, err := s.GetAgent(agentID)
	if err != nil || !isEmployeeAgent(agent) {
		return EmployeeActivity{}, errors.New("employee not found")
	}
	var executions []Execution
	if err = s.db.Where("agent_id = ? AND updated_at > ? AND updated_at <= ?", agent.ID, after, watermark).Order("updated_at asc, id asc").Find(&executions).Error; err != nil {
		return EmployeeActivity{}, err
	}
	for index := range executions {
		executions[index] = compactExecution(executions[index])
	}
	var messages []Message
	if err = s.db.Table("messages").Joins("JOIN executions ON executions.id = messages.execution_id").Where("executions.agent_id = ? AND messages.updated_at > ? AND messages.updated_at <= ?", agent.ID, after, watermark).Order("messages.updated_at asc, messages.id asc").Select("messages.*").Find(&messages).Error; err != nil {
		return EmployeeActivity{}, err
	}
	var events []ExecutionEvent
	if err = s.db.Table("execution_events").Joins("JOIN executions ON executions.id = execution_events.execution_id").Where("executions.agent_id = ? AND execution_events.updated_at > ? AND execution_events.updated_at <= ?", agent.ID, after, watermark).Order("execution_events.updated_at asc, execution_events.id asc").Select("execution_events.*").Find(&events).Error; err != nil {
		return EmployeeActivity{}, err
	}
	for index := range events {
		events[index] = compactExecutionEvent(events[index])
	}
	var attachments []IssueAttachment
	if err = s.db.Table("issue_attachments").Joins("JOIN executions ON executions.id = issue_attachments.execution_id").Where("executions.agent_id = ? AND issue_attachments.created_at > ? AND issue_attachments.created_at <= ?", agent.ID, after, watermark).Order("issue_attachments.created_at asc, issue_attachments.id asc").Select("issue_attachments.*").Find(&attachments).Error; err != nil {
		return EmployeeActivity{}, err
	}
	var issues []Issue
	if err = s.db.Where("assignee_agent_id = ? AND hidden = ? AND updated_at > ? AND updated_at <= ?", agent.ID, false, after, watermark).Order("updated_at asc, id asc").Find(&issues).Error; err != nil {
		return EmployeeActivity{}, err
	}
	return EmployeeActivity{Executions: executions, Messages: messages, Events: events, Attachments: attachments, Issues: issues, Watermark: watermark}, nil
}
