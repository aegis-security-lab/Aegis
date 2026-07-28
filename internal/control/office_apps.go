package control

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

func (m *Manager) executionActor(executionID, token string) (Execution, *PiSession, error) {
	session := m.getSession(executionID)
	if session == nil || token == "" || !secureEqual(token, session.controlToken) {
		return Execution{}, nil, errors.New("invalid execution control token")
	}
	var execution Execution
	if err := m.store.db.First(&execution, "id = ?", executionID).Error; err != nil {
		return Execution{}, nil, err
	}
	return execution, session, nil
}

// BoardCommandFromExecution is the Agent-facing application boundary for the
// Issue board. Runtime completion never writes comments on the Agent's behalf.
func (m *Manager) BoardCommandFromExecution(executionID, token string, input BoardCommandInput) (BoardCommandResult, error) {
	execution, _, err := m.executionActor(executionID, token)
	if err != nil {
		return BoardCommandResult{}, err
	}
	input.Action = strings.TrimSpace(input.Action)
	result := BoardCommandResult{Action: input.Action, Issues: []Issue{}}
	switch input.Action {
	case "list":
		var issues []Issue
		if listErr := m.store.db.Where("hidden = ?", false).Order("updated_at desc").Find(&issues).Error; listErr != nil {
			return result, listErr
		}
		query := strings.ToLower(strings.TrimSpace(input.Query))
		for _, issue := range issues {
			if issue.Hidden || (len(input.Statuses) > 0 && !containsString(input.Statuses, issue.Status)) {
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(issue.Identifier+" "+issue.Title+" "+issue.Description+" "+issue.Objective), query) {
				continue
			}
			result.Issues = append(result.Issues, issue)
		}
		return result, nil
	case "get":
		detail, getErr := m.store.GetIssueDetail(strings.TrimSpace(input.IssueID))
		if getErr != nil {
			return result, getErr
		}
		result.Detail = &detail
		return result, nil
	case "create":
		if err := m.store.ValidateDelegation(execution.AgentID, input.AssigneeAgentID); err != nil {
			return result, err
		}
		attachmentIDs := m.store.InputAttachmentIDsForExecution(execution.ID)
		issue, createErr := m.CreateIssue(CreateIssueInput{
			ParentID: input.ParentID, Title: input.Title, Description: input.Description,
			Objective: input.Objective, Status: fallback(input.Status, "todo"),
			Priority: fallback(input.Priority, "medium"), WorkMode: "autonomous",
			AssigneeAgentID: input.AssigneeAgentID, AttachmentIDs: attachmentIDs,
			AttachmentSourceExecutionID: execution.ID, CreatedBy: execution.AgentID,
		})
		if createErr != nil {
			return result, createErr
		}
		result.Issue = &issue
		return result, nil
	case "update", "assign":
		issue, getErr := m.store.GetIssue(strings.TrimSpace(input.IssueID))
		if getErr != nil {
			return result, getErr
		}
		update := UpdateIssueInput{}
		if strings.TrimSpace(input.Title) != "" {
			update.Title = &input.Title
		}
		if input.Description != "" {
			update.Description = &input.Description
		}
		if input.Objective != "" {
			update.Objective = &input.Objective
		}
		if input.Priority != "" {
			update.Priority = &input.Priority
		}
		if input.Status != "" {
			update.Status = &input.Status
		}
		if input.AssigneeAgentID != "" || input.Action == "assign" {
			if delegationErr := m.store.ValidateDelegation(execution.AgentID, input.AssigneeAgentID); delegationErr != nil {
				return result, delegationErr
			}
			update.AssigneeAgentID = &input.AssigneeAgentID
		}
		if input.ParentID != "" {
			update.ParentID = &input.ParentID
		}
		updated, updateErr := m.store.UpdateIssue(issue.ID, update)
		if updateErr != nil {
			return result, updateErr
		}
		m.ReconcileIssue(updated)
		result.Issue = &updated
		if updated.AssigneeAgentID != "" && updated.AssigneeAgentID != issue.AssigneeAgentID {
			m.notifyBoardAssignment(execution.AgentID, updated, "Board 更新了 Issue 委派")
		}
		return result, nil
	case "archive":
		archived, archiveErr := m.archiveBoardIssue(strings.TrimSpace(input.IssueID), fallback(strings.TrimSpace(input.Reason), "Agent 通过 Board 归档了 Issue"))
		if archiveErr != nil {
			return result, archiveErr
		}
		result.Issue = &archived
		return result, nil
	case "delete":
		deleted, deleteErr := m.DeleteBoardIssue(strings.TrimSpace(input.IssueID))
		if deleteErr != nil {
			return result, deleteErr
		}
		result.DeletedIssueCount = deleted.DeletedIssues
		return result, nil
	case "comment":
		issue, getErr := m.store.GetIssue(strings.TrimSpace(input.IssueID))
		if getErr != nil {
			return result, getErr
		}
		comment := m.addTypedAgentComment(issue.ID, execution.AgentID, fallback(input.CommentType, "normal"), input.Body, execution.ID, []commentWakeupTarget{})
		if comment == nil {
			return result, errors.New("评论内容不能为空")
		}
		result.Comment = comment
		for _, target := range m.operatorCommentWakeupTargets(issue, comment.Mentions) {
			if target.AgentID != execution.AgentID {
				m.sendBoardRelay(execution.AgentID, target.AgentID, issue, "Board 上有一条新评论", input.Body, true)
			}
		}
		m.store.notify()
		return result, nil
	case "relate":
		relation, relationErr := m.store.AddRelation(strings.TrimSpace(input.IssueID), CreateRelationInput{RelatedIssueID: strings.TrimSpace(input.RelatedIssueID), Type: fallback(input.RelationType, "blocks")})
		if relationErr != nil {
			return result, relationErr
		}
		result.Relation = &relation
		return result, nil
	case "unrelate":
		relationID := strings.TrimSpace(input.RelationID)
		if relationID == "" {
			return result, errors.New("删除依赖关系需要 relationId")
		}
		if deleteErr := m.store.DeleteRelation(relationID); deleteErr != nil {
			return result, deleteErr
		}
		result.DeletedRelationID = relationID
		return result, nil
	default:
		return result, errors.New("Board action 必须是 list、get、create、update、assign、comment、relate、unrelate、archive 或 delete")
	}
}

// archiveBoardIssue immediately closes an unfinished Issue subtree. Unlike an
// ordinary status edit, this also terminates runtimes and clears pending work.
func (m *Manager) archiveBoardIssue(issueID, reason string) (archived Issue, err error) {
	m.scheduleMu.Lock()
	parentID := ""
	defer func() {
		m.scheduleMu.Unlock()
		if err == nil && parentID != "" {
			m.scheduleChildren(parentID)
		}
	}()

	issue, err := m.store.GetIssue(issueID)
	if err != nil {
		return Issue{}, err
	}
	if issueStatusTerminal(issue.Status) {
		return Issue{}, errors.New("已经结束的 Issue 不需要归档")
	}
	descendants, err := m.issueDescendantIDs(issue)
	if err != nil {
		return Issue{}, err
	}
	issueIDs := append([]string{issue.ID}, descendants...)
	now := time.Now()
	var activeExecutions []Execution
	err = m.store.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("issue_id IN ? AND status IN ?", issueIDs, activeExecutionStatuses).Find(&activeExecutions).Error; err != nil {
			return err
		}
		if len(activeExecutions) > 0 {
			executionIDs := make([]string, len(activeExecutions))
			for index := range activeExecutions {
				executionIDs[index] = activeExecutions[index].ID
			}
			if err := tx.Model(&Execution{}).Where("id IN ?", executionIDs).Updates(map[string]any{
				"status": "cancelled", "error": reason, "current_tool": "", "finished_at": now, "pid": 0, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&Issue{}).Where("id IN ? AND status NOT IN ?", issueIDs, terminalIssueStatuses).Updates(map[string]any{
			"status": "cancelled", "execution_phase": "completed", "checkout_execution_id": "",
			"cancelled_at": now, "error": reason, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("issue_id IN ? AND status = ?", issueIDs, "pending").Delete(&Approval{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&AgentWakeup{}).Where("issue_id IN ? AND status IN ?", issueIDs, []string{"queued", "delivered"}).Updates(map[string]any{
			"status": "cancelled", "error": reason, "completed_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&IssueChildWait{}).Where("parent_issue_id IN ? AND status = ?", issueIDs, "waiting").Update("status", "superseded").Error; err != nil {
			return err
		}
		if err := tx.Model(&Message{}).Where("issue_id IN ? AND streaming = ?", issueIDs, true).Updates(map[string]any{"streaming": false, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Create(&ExecutionEvent{
			ID: nextID("event"), IssueID: issue.ID, Type: "cancellation", Title: "Board 已归档 Issue",
			Detail: fmt.Sprintf("%s；同步结束 %d 个后代 Issues 和 %d 个活跃 Executions。", reason, len(descendants), len(activeExecutions)), CreatedAt: now,
		}).Error
	})
	if err != nil {
		return Issue{}, err
	}
	activeIDs := make(map[string]bool, len(activeExecutions))
	for _, execution := range activeExecutions {
		activeIDs[execution.ID] = true
	}
	m.mu.RLock()
	sessions := make([]*PiSession, 0, len(activeIDs))
	for executionID, session := range m.sessions {
		if activeIDs[executionID] {
			sessions = append(sessions, session)
		}
	}
	m.mu.RUnlock()
	for _, session := range sessions {
		session.Close()
	}
	m.store.notify()
	parentID = issue.ParentID
	return m.store.GetIssue(issue.ID)
}

func (m *Manager) UpdateBoardIssue(issueID string, input UpdateIssueInput) (Issue, error) {
	before, err := m.store.GetIssue(issueID)
	if err != nil {
		return Issue{}, err
	}
	updated, err := m.store.UpdateIssue(issueID, input)
	if err != nil {
		return Issue{}, err
	}
	m.ReconcileIssue(updated)
	if updated.AssigneeAgentID != "" && updated.AssigneeAgentID != before.AssigneeAgentID {
		m.notifyBoardAssignment("operator", updated, "Board 更新了 Issue 委派")
	}
	return updated, nil
}

func (m *Manager) DeleteBoardIssue(issueID string) (DeleteIssueResult, error) {
	issue, err := m.store.GetIssue(strings.TrimSpace(issueID))
	if err != nil {
		return DeleteIssueResult{}, err
	}
	result, err := m.store.DeleteIssueTree(issue.ID)
	if err != nil {
		return DeleteIssueResult{}, err
	}
	if issue.ParentID != "" {
		go m.scheduleChildren(issue.ParentID)
	}
	return result, nil
}

func (m *Manager) notifyBoardAssignment(senderID string, issue Issue, subject string) {
	if issue.AssigneeAgentID == "" || issue.AssigneeAgentID == senderID {
		return
	}
	m.sendBoardRelay("board", issue.AssigneeAgentID, issue, subject, fmt.Sprintf("%s · %s\n\n目标：%s", issue.Identifier, issue.Title, fallback(issue.Objective, "未填写")), issue.Status != "todo")
}

func (m *Manager) sendBoardRelay(senderID, recipientID string, issue Issue, subject, body string, wake bool) {
	message, err := m.store.SendRelayMessage("app", senderID, SendRelayMessageInput{RecipientID: recipientID, Body: "**" + subject + "**\n\n" + body, IssueID: issue.ID})
	if err == nil && wake {
		go m.notifyEmployeeRelay(recipientID, message)
	}
}

func (m *Manager) RelayCommandFromExecution(executionID, token string, input RelayCommandInput) (RelayCommandResult, error) {
	execution, _, err := m.executionActor(executionID, token)
	if err != nil {
		return RelayCommandResult{}, err
	}
	result := RelayCommandResult{Action: strings.TrimSpace(input.Action)}
	switch result.Action {
	case "directory":
		for _, agent := range m.store.Agents() {
			if isEmployeeAgent(agent) {
				result.Directory = append(result.Directory, EmployeeDirectoryEntry{ID: agent.ID, Name: agent.Name, Description: agent.Description, DepartmentID: agent.DepartmentID})
			}
		}
	case "inbox":
		result.Inbox, err = m.store.RelayInbox(execution.AgentID)
	case "read":
		conversation, readErr := m.store.RelayConversation(execution.AgentID, strings.TrimSpace(input.ThreadID), true)
		err = readErr
		result.Conversation = &conversation
	case "send":
		message, sendErr := m.store.SendRelayMessage("agent", execution.AgentID, SendRelayMessageInput{RecipientID: input.RecipientID, Body: input.Body, IssueID: input.IssueID})
		err = sendErr
		result.Message = &message
		if sendErr == nil {
			go m.notifyEmployeeRelay(input.RecipientID, message)
		}
	default:
		err = errors.New("Relay action 必须是 directory、inbox、read 或 send")
	}
	return result, err
}

func (m *Manager) SendRelayFromOperator(senderID string, input SendRelayMessageInput) (RelayMessage, error) {
	message, err := m.store.SendRelayMessage("operator", senderID, input)
	if err == nil {
		go m.notifyEmployeeRelay(input.RecipientID, message)
	}
	return message, err
}

func (m *Manager) notifyEmployeeRelay(agentID string, message RelayMessage) {
	prompt := fmt.Sprintf("Relay 收到一条新消息（messageId: %s，sender: %s）。你不需要立即回复；在合适的工作节点调用 aegis_relay 的 inbox/read 查看，随后可继续当前工作。", message.ID, message.SenderID)
	taskIssueID := ""
	if message.IssueID != "" {
		if issue, issueErr := m.store.GetIssue(message.IssueID); issueErr == nil {
			if root, rootErr := m.store.taskRoot(issue); rootErr == nil {
				taskIssueID = root.ID
			}
		}
	}
	m.mu.RLock()
	var active *PiSession
	for _, session := range m.sessions {
		if session.agentID == agentID && !session.closed.Load() {
			activeIssue, issueErr := m.store.GetIssue(session.issueID)
			if issueErr != nil {
				continue
			}
			root, rootErr := m.store.taskRoot(activeIssue)
			activeTaskID := root.ID
			if activeIssue.Hidden {
				activeTaskID = ""
			}
			if rootErr != nil || activeTaskID != taskIssueID {
				continue
			}
			active = session
			break
		}
	}
	m.mu.RUnlock()
	if active != nil {
		_, _ = m.sendSessionPrompt(active, prompt)
		return
	}
	_, _ = m.SendEmployeeTaskMessage(agentID, taskIssueID, prompt)
}

func (m *Manager) SendEmployeeMessage(agentID, message string, attachmentIDs ...string) (Message, error) {
	return m.SendEmployeeTaskMessage(agentID, "", message, attachmentIDs...)
}

func (m *Manager) SendEmployeeTaskMessage(agentID, taskIssueID, message string, attachmentIDs ...string) (Message, error) {
	taskIssueID = strings.TrimSpace(taskIssueID)
	if taskIssueID == "general" {
		taskIssueID = ""
	}
	message = strings.TrimSpace(message)
	attachmentIDs, err := normalizeInputAttachmentIDs(attachmentIDs)
	if err != nil {
		return Message{}, err
	}
	if message == "" && len(attachmentIDs) == 0 {
		return Message{}, errors.New("消息不能为空")
	}
	if message == "" {
		message = "请审计并处理本轮上传的附件。"
	}
	agent, err := m.store.executionAgent(agentID)
	if err != nil {
		return Message{}, err
	}
	m.mu.RLock()
	for _, session := range m.sessions {
		if session.agentID == agent.ID && !session.closed.Load() {
			activeIssue, issueErr := m.store.GetIssue(session.issueID)
			if issueErr != nil {
				continue
			}
			activeRoot, rootErr := m.store.taskRoot(activeIssue)
			activeTaskID := activeRoot.ID
			if activeIssue.Hidden {
				activeTaskID = ""
			}
			if rootErr != nil || activeTaskID != taskIssueID {
				continue
			}
			m.mu.RUnlock()
			if len(attachmentIDs) > 0 {
				return Message{}, errors.New("该员工当前正在执行；请等待本轮结束后再发送附件")
			}
			return m.sendSessionPrompt(session, message)
		}
	}
	m.mu.RUnlock()
	var home Issue
	if taskIssueID == "" {
		home, err = m.employeeHomeIssue(agent)
	} else {
		var candidates []Issue
		if err = m.store.db.Where("assignee_agent_id = ? AND hidden = ?", agent.ID, false).Order("updated_at desc").Find(&candidates).Error; err == nil {
			for _, candidate := range candidates {
				root, rootErr := m.store.taskRoot(candidate)
				if rootErr == nil && root.ID == taskIssueID {
					home = candidate
					break
				}
			}
		}
		if home.ID == "" && err == nil {
			err = errors.New("该员工在指定任务中没有会话")
		}
	}
	if err != nil {
		return Message{}, err
	}
	execution, err := m.store.createExecution(home, agent.ID, "employee_chat")
	if err != nil {
		return Message{}, err
	}
	if err = m.store.BindEmployeeInputAttachments(agent.ID, home.ID, execution.ID, attachmentIDs); err != nil {
		_ = m.store.updateExecution(execution.ID, map[string]any{"status": "failed", "error": err.Error(), "finished_at": time.Now()})
		return Message{}, err
	}
	if _, err = m.startSession(home, execution, agent, message); err != nil {
		m.store.UnbindEmployeeInputAttachments(execution.ID)
		_ = m.store.updateExecution(execution.ID, map[string]any{"status": "failed", "error": err.Error(), "finished_at": time.Now()})
		return Message{}, err
	}
	var sent Message
	err = m.store.db.Where("execution_id = ? AND role = ?", execution.ID, "user").Order("created_at desc, id desc").First(&sent).Error
	return sent, err
}

func (m *Manager) employeeHomeIssue(agent AgentDefinition) (Issue, error) {
	session, err := m.store.ensureEmployeeWorkspace(agent)
	if err != nil {
		return Issue{}, err
	}
	return m.store.GetIssue(session.HomeIssueID)
}
