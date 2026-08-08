package control

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
		if strings.TrimSpace(input.AssigneeAgentID) != "" {
			if err := m.store.ValidateDelegation(execution.AgentID, input.AssigneeAgentID); err != nil {
				return result, err
			}
		}
		attachmentIDs := m.store.InputAttachmentIDsForExecution(execution.ID)
		issue, createErr := m.CreateIssue(CreateIssueInput{
			ParentID: input.ParentID, Title: input.Title, Description: input.Description,
			Objective: input.Objective, Status: fallback(input.Status, "todo"),
			Priority: fallback(input.Priority, "middle"), WorkMode: "autonomous",
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
			if strings.TrimSpace(input.AssigneeAgentID) != "" {
				if delegationErr := m.store.ValidateDelegation(execution.AgentID, input.AssigneeAgentID); delegationErr != nil {
					return result, delegationErr
				}
			}
			update.AssigneeAgentID = &input.AssigneeAgentID
		}
		if input.ParentID != "" {
			update.ParentID = &input.ParentID
		}
		updated, updateErr := m.UpdateBoardIssue(issue.ID, update)
		if updateErr != nil {
			return result, updateErr
		}
		result.Issue = &updated
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
	for _, archivedID := range issueIDs {
		m.reconcileIssueID(archivedID)
	}
	parentID = issue.ParentID
	return m.store.GetIssue(issue.ID)
}

func (m *Manager) UpdateBoardIssue(issueID string, input UpdateIssueInput) (Issue, error) {
	before, err := m.store.GetIssue(issueID)
	if err != nil {
		return Issue{}, err
	}
	if input.Status != nil && strings.TrimSpace(*input.Status) == "cancelled" && before.Status != "cancelled" {
		input.Status = nil
		if hasBoardIssueUpdate(input) {
			if _, err = m.store.UpdateIssue(issueID, input); err != nil {
				return Issue{}, err
			}
		}
		return m.archiveBoardIssue(issueID, "通过 Board 取消了 Issue")
	}
	requestedStart := input.Status != nil && strings.TrimSpace(*input.Status) == "in_progress" && before.Status != "in_progress"
	if requestedStart {
		assigneeID := before.AssigneeAgentID
		if input.AssigneeAgentID != nil {
			assigneeID = strings.TrimSpace(*input.AssigneeAgentID)
		}
		if assigneeID == "" {
			return Issue{}, errors.New("in_progress Issue 必须先指定负责人")
		}
		// Queued work remains todo until CheckoutIssue atomically proves that an
		// Execution has actually started. This prevents UI state from claiming a
		// worker is running while it is still waiting for capacity.
		status := "todo"
		input.Status = &status
	}
	if input.Status != nil && strings.TrimSpace(*input.Status) != before.Status {
		target := strings.TrimSpace(*input.Status)
		if target == "done" || target == "in_review" {
			// The operator is settling the Issue now: end any live execution
			// so the Agent stops immediately instead of continuing to burn
			// turns before the terminal status is noticed.
			if err := m.terminateIssueExecution(before, "Board 将状态改为 "+target+"，结束当前执行"); err != nil {
				return Issue{}, err
			}
		} else {
			var active int64
			if err = m.store.db.Model(&Execution{}).Where("issue_id = ? AND status IN ?", before.ID, activeExecutionStatuses).Count(&active).Error; err != nil {
				return Issue{}, err
			}
			if active > 0 {
				return Issue{}, errors.New("Issue 正在执行；只能取消活动工作，不能直接改写为其他状态")
			}
		}
	}
	updated, err := m.store.UpdateIssue(issueID, input)
	if err != nil {
		return Issue{}, err
	}
	m.ReconcileIssue(updated)
	if input.Status != nil && strings.TrimSpace(*input.Status) == "in_review" && before.Status != "in_review" {
		// Switching to review hands the current delivery to the acceptance
		// Agent instead of just relabelling the Issue.
		if err := m.triggerManualReview(updated); err != nil {
			m.store.addEvent(updated.ID, updated.ID, "validation", "手动复核触发失败", err.Error())
		}
	}
	reopenedTerminalOutcome := hasIssueLabel(before, issueLabelFailed, issueLabelBudgetExceeded) && updated.Status == "todo"
	becameSchedulable := before.Status != "todo" && updated.Status == "todo"
	if updated.AssigneeAgentID != "" && (updated.AssigneeAgentID != before.AssigneeAgentID || reopenedTerminalOutcome || becameSchedulable || requestedStart) {
		if bridge := m.Coordination(); bridge != nil {
			if err := bridge.SubmitIssueAssigned(context.Background(), updated); err != nil {
				return Issue{}, err
			}
		} else {
			m.notifyBoardAssignment("operator", updated, "Board 更新了 Issue 委派")
		}
	}
	return updated, nil
}

func hasBoardIssueUpdate(input UpdateIssueInput) bool {
	return input.Title != nil || input.Description != nil || input.Objective != nil || input.Priority != nil || input.Status != nil || input.AssigneeAgentID != nil || input.ParentID != nil || input.TimeBudgetMinutes != nil
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
	if m.Coordination() != nil {
		return
	}
	if issue.AssigneeAgentID == "" || issue.AssigneeAgentID == senderID {
		return
	}
	m.sendBoardRelay("board", issue.AssigneeAgentID, issue, subject, fmt.Sprintf("%s · %s\n\n目标：%s", issue.Identifier, issue.Title, fallback(issue.Objective, "未填写")), issue.Status != "todo")
}

func (m *Manager) sendBoardRelay(senderID, recipientID string, issue Issue, subject, body string, wake bool) {
	message, err := m.store.SendRelayMessage("app", senderID, SendRelayMessageInput{RecipientID: recipientID, Body: "**" + subject + "**\n\n" + body, IssueID: issue.ID})
	if err == nil && wake {
		m.routeRelayMessage(message)
	}
}

func (m *Manager) RelayCommandFromExecution(executionID, token string, input RelayCommandInput) (RelayCommandResult, error) {
	execution, _, err := m.executionActor(executionID, token)
	if err != nil {
		return RelayCommandResult{}, err
	}
	result := RelayCommandResult{Action: strings.TrimSpace(input.Action)}
	issue, err := m.store.GetIssue(execution.IssueID)
	if err != nil {
		return RelayCommandResult{}, err
	}
	root, rootErr := m.store.taskRoot(issue)
	if rootErr != nil {
		return RelayCommandResult{}, rootErr
	}
	routingID := fallback(execution.TaskAgentID, execution.AgentID)
	switch result.Action {
	case "directory":
		identities, listErr := m.store.TaskAgents(root.TaskSourceID)
		if listErr != nil {
			return RelayCommandResult{}, listErr
		}
		for _, identity := range identities {
			agent, _ := m.store.GetAgent(identity.AgentID)
			result.Directory = append(result.Directory, TaskAgentDirectoryEntry{ID: identity.ID, Name: identity.Name, AgentID: identity.AgentID, Role: agent.Name, Description: agent.Description})
		}
	case "inbox":
		result.Inbox, err = m.store.RelayInboxForTask(routingID, root.ID)
	case "read":
		conversation, readErr := m.store.RelayConversationForTask(routingID, strings.TrimSpace(input.ThreadID), root.ID, true)
		err = readErr
		result.Conversation = &conversation
	case "send":
		recipient, recipientErr := m.store.taskAgent(root.ID, input.RecipientID)
		if recipientErr != nil {
			return RelayCommandResult{}, errors.New("Relay 收件人必须是当前任务中的 taskAgentId")
		}
		message, sendErr := m.store.SendRelayMessage("agent", routingID, SendRelayMessageInput{RecipientID: recipient.AgentID, RecipientTaskAgentID: recipient.ID, Body: input.Body, TaskID: root.ID, IssueID: fallback(input.IssueID, issue.ID)})
		err = sendErr
		result.Message = &message
		if sendErr == nil {
			m.routeRelayMessage(message)
		}
	default:
		err = errors.New("Relay action 必须是 directory、inbox、read 或 send")
	}
	return result, err
}

func (m *Manager) SendRelayFromOperator(senderID string, input SendRelayMessageInput) (RelayMessage, error) {
	message, err := m.store.SendRelayMessage("operator", senderID, input)
	if err == nil {
		m.routeRelayMessage(message)
	}
	return message, err
}

func (m *Manager) routeRelayMessage(message RelayMessage) {
	if bridge := m.Coordination(); bridge != nil {
		go func() { _ = bridge.SubmitRelay(context.Background(), message) }()
	}
}

// terminateIssueExecution ends any live execution of the Issue so the Agent
// stops immediately when the operator settles the Issue through the Board
// (done / in_review), instead of continuing until its natural stop.
func (m *Manager) terminateIssueExecution(issue Issue, reason string) error {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	var executions []Execution
	if err := m.store.db.Where("issue_id = ? AND status IN ?", issue.ID, activeExecutionStatuses).Find(&executions).Error; err != nil {
		return err
	}
	if len(executions) == 0 {
		return nil
	}
	now := time.Now()
	executionIDs := make([]string, len(executions))
	for index := range executions {
		executionIDs[index] = executions[index].ID
	}
	if err := m.store.db.Model(&Execution{}).Where("id IN ?", executionIDs).Updates(map[string]any{
		"status": "cancelled", "error": reason, "current_tool": "", "finished_at": now, "pid": 0, "updated_at": now,
	}).Error; err != nil {
		return err
	}
	m.mu.RLock()
	sessions := make([]*PiSession, 0, len(executionIDs))
	for executionID, session := range m.sessions {
		if slices.Contains(executionIDs, executionID) {
			sessions = append(sessions, session)
		}
	}
	m.mu.RUnlock()
	for _, session := range sessions {
		session.Close()
	}
	m.abortNativeIssue(issue.ID)
	_ = m.store.db.Model(&Message{}).Where("issue_id = ? AND streaming = ?", issue.ID, true).Updates(map[string]any{"streaming": false, "updated_at": now}).Error
	_ = m.store.db.Model(&AgentWakeup{}).Where("issue_id = ? AND status IN ?", issue.ID, []string{"queued", "delivered"}).Updates(map[string]any{"status": "cancelled", "error": reason, "completed_at": now}).Error
	m.store.addEvent(issue.ID, issue.ID, "runtime", "Board 修改状态，执行已结束", reason)
	m.store.notify()
	return nil
}

// triggerManualReview hands the Issue's current delivery to the acceptance
// Agent when the operator switches the Issue to in_review.
func (m *Manager) triggerManualReview(issue Issue) error {
	if strings.TrimSpace(issue.Objective) == "" {
		m.store.addEvent(issue.ID, issue.ID, "validation", "手动复核未启动", "Issue 未设置目标，无法启动验收 Agent。")
		m.store.notify()
		return nil
	}
	var source Execution
	if issue.CurrentExecutionID != "" {
		if err := m.store.db.First(&source, "id = ? AND issue_id = ?", issue.CurrentExecutionID, issue.ID).Error; err == nil && source.ID != "" {
			return m.publishManualReview(issue, source)
		}
	}
	if err := m.store.db.Where("issue_id = ?", issue.ID).Order("created_at desc, id desc").First(&source).Error; err != nil || source.ID == "" {
		m.store.addEvent(issue.ID, issue.ID, "validation", "手动复核未启动", "没有可用的执行记录，无法触发验收 Agent。")
		m.store.notify()
		return nil
	}
	return m.publishManualReview(issue, source)
}

func (m *Manager) publishManualReview(issue Issue, source Execution) error {
	body := strings.TrimSpace(issue.Result)
	if body == "" {
		body = fallback(strings.TrimSpace(source.FinalResult), "操作员将 Issue 置为待复核，请求验收 Agent 复核当前交付物。")
	}
	now := time.Now()
	comment := IssueComment{ID: nextID("comment"), IssueID: issue.ID, Type: "delivery", AuthorType: "operator", AuthorID: "operator", ExecutionID: source.ID, Body: body, Mentions: []string{}, CreatedAt: now}
	var wakeups []AgentWakeup
	if err := m.store.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}
		if err := m.store.bindExecutionAttachments(tx, &comment, source.ID); err != nil {
			return err
		}
		var err error
		wakeups, err = createCommentWakeups(tx, comment, []commentWakeupTarget{{AgentID: "acceptance-validator", Reason: "delivery_validation"}})
		return err
	}); err != nil {
		return err
	}
	m.store.addEvent(source.ID, issue.ID, "validation", "操作员请求复核", "Board 将 Issue 置为待复核，系统已请求验收 Agent。")
	for _, wakeup := range wakeups {
		go m.dispatchWakeup(wakeup.ID)
	}
	m.store.notify()
	return nil
}
