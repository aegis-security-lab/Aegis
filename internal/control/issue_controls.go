package control

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const abandonSummaryInstruction = `## 操作员已放弃目标

操作员已取消这个 Issue。立即停止继续实施，也不要再创建子 Issue 或进入目标验收。

请只提交一次最终总结，说明：已经完成的工作、已经生成的附件、当前状态以及剩余风险。总结完成后，此 Issue 将直接以“目标已放弃”结束。`

const parentAgentCancelSummaryInstruction = `## 父 Agent 请求取消此子 Issue

父 Agent 已决定不再继续这个子 Issue。立即停止继续实施，也不要再创建子 Issue 或进入目标验收。

请只提交一次最终总结，说明：已经完成的工作、已经生成的附件、当前状态以及剩余风险。总结会保存到 Issue.result，供父 Agent 恢复后整合；总结完成后，此 Issue 将结束。`

const issueBudgetSummaryInstruction = `## Task 总时钟墙预算已耗尽

系统检测到整个 Task 从创建开始计算的总时间预算已经耗尽。立即停止继续实施，也不要再创建子 Issue 或进入目标验收。等待、休眠和重新执行均不会重置这个总预算。

请只提交一次最终总结，说明：预算耗尽前已经完成的工作、已经生成的附件、当前状态以及剩余风险。总结会保存到 Issue.result；总结完成后，此 Task 将以“目标已放弃”结束。`

func (m *Manager) AbandonIssue(id, reason string) (Issue, error) {
	return m.abandonIssueWithSummary(id, reason, abandonSummaryInstruction, "操作员已放弃目标", "operator_abandoned_issue", false)
}

func (m *Manager) abandonIssueWithSummary(id, reason, instruction, eventTitle, eventType string, allowTopLevel bool) (Issue, error) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()

	issue, err := m.store.GetIssue(id)
	if err != nil {
		return Issue{}, err
	}
	if issue.ParentID == "" && !allowTopLevel {
		return Issue{}, errors.New("顶层任务请使用“取消任务”；只有子 Issue 可以放弃目标")
	}
	if issueStatusTerminal(issue.Status) {
		return Issue{}, errors.New("已经结束的 Issue 不能放弃目标")
	}
	if issue.AbandonRequestedAt != nil {
		return issue, nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "操作员手动放弃此目标"
	}

	summarySession := m.summarizingSessionForIssue(issue)
	descendantIDs, err := m.issueDescendantIDs(issue)
	if err != nil {
		return Issue{}, err
	}
	now := time.Now()
	var cancelledExecutionIDs []string
	currentExecutionID := ""
	err = m.store.db.Transaction(func(tx *gorm.DB) error {
		if len(descendantIDs) > 0 {
			if err := tx.Model(&Issue{}).
				Where("id IN ? AND status NOT IN ?", descendantIDs, terminalIssueStatuses).
				Updates(map[string]any{
					"status": "cancelled", "execution_phase": "completed", "checkout_execution_id": "",
					"cancelled_at": now, "error": eventTitle, "updated_at": now,
				}).Error; err != nil {
				return err
			}
		}

		allIDs := append([]string{issue.ID}, descendantIDs...)
		var active []Execution
		if err := tx.Where("issue_id IN ? AND status IN ?", allIDs, activeExecutionStatuses).Find(&active).Error; err != nil {
			return err
		}
		for _, execution := range active {
			if summarySession != nil && execution.ID == summarySession.executionID {
				continue
			}
			cancelledExecutionIDs = append(cancelledExecutionIDs, execution.ID)
		}
		if len(cancelledExecutionIDs) > 0 {
			if err := tx.Model(&Execution{}).Where("id IN ?", cancelledExecutionIDs).Updates(map[string]any{
				"status": "cancelled", "error": reason, "current_tool": "", "finished_at": now, "pid": 0, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&IssueValidation{}).Where("issue_id = ? AND status = ?", issue.ID, "running").Updates(map[string]any{
			"status": "skipped", "feedback": "操作员已放弃目标，本轮验收被终止。", "completed_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("issue_id IN ? AND status = ?", allIDs, "pending").Delete(&Approval{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&AgentWakeup{}).Where("issue_id IN ? AND status IN ?", allIDs, []string{"queued", "delivered"}).Updates(map[string]any{
			"status": "cancelled", "error": reason, "completed_at": now,
		}).Error; err != nil {
			return err
		}

		if summarySession != nil {
			currentExecutionID = summarySession.executionID
			if err := tx.Model(&Execution{}).Where("id = ?", currentExecutionID).Updates(map[string]any{
				"status": "starting", "result": "", "error": "", "current_tool": "", "finished_at": nil, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
			"status": "in_progress", "execution_phase": "summarizing", "checkout_execution_id": currentExecutionID,
			"current_execution_id": currentExecutionID, "validation_disabled": true,
			"abandon_requested_at": now, "abandonment_reason": reason, "error": "", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		commentBody := instruction + "\n\n**取消原因：** " + reason
		if err := createSystemComment(tx, issue.ID, commentBody, now); err != nil {
			return err
		}
		eventDetail := fmt.Sprintf("已取消 %d 个后代 Issues；等待原负责人提交最终总结。", len(descendantIDs))
		if summarySession == nil {
			eventDetail = fmt.Sprintf("已取消 %d 个后代 Issues；没有可继续使用的负责人会话，将直接结束。", len(descendantIDs))
		}
		return tx.Create(&ExecutionEvent{
			ID: nextID("event"), ExecutionID: currentExecutionID, IssueID: issue.ID, Type: "cancellation",
			Title: eventTitle, Detail: eventDetail, CreatedAt: now,
		}).Error
	})
	if err != nil {
		return Issue{}, err
	}

	m.closeAbandonedSessions(issue.ID, descendantIDs, currentExecutionID)
	for _, descendantID := range descendantIDs {
		m.reconcileIssueID(descendantID)
	}
	if summarySession == nil {
		issue, _ = m.store.GetIssue(issue.ID)
		m.finalizeManualAbandon(issue, "", "", "")
	} else {
		prompt := `<system_event type="` + eventType + `">` + "\n" + instruction + "\n\n取消原因：" + reason + "\n</system_event>"
		if _, err = m.sendSessionPrompt(summarySession, prompt); err != nil {
			issue, _ = m.store.GetIssue(issue.ID)
			m.finalizeManualAbandon(issue, summarySession.executionID, summarySession.agentID, "原负责人会话不可用，未能生成额外总结。")
		}
	}
	m.store.notify()
	return m.store.GetIssue(issue.ID)
}

func (m *Manager) SetIssueValidationDisabled(id string, disabled bool) (Issue, error) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()

	issue, err := m.store.GetIssue(id)
	if err != nil {
		return Issue{}, err
	}
	if issueStatusTerminal(issue.Status) {
		return Issue{}, errors.New("已经结束的 Issue 不能修改验收设置")
	}
	if issue.ValidationDisabled == disabled {
		return issue, nil
	}
	now := time.Now()
	var validation IssueValidation
	validationCanBeAdopted := issue.ExecutionPhase == "validating" ||
		(issue.ExecutionPhase == "blocked" && issue.ValidationExecutionID != "" && issue.CurrentExecutionID == issue.ValidationExecutionID)
	hasValidation := validationCanBeAdopted && m.store.db.Where("issue_id = ?", issue.ID).Order("attempt desc").First(&validation).Error == nil
	err = m.store.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"validation_disabled": disabled, "updated_at": now}).Error; err != nil {
			return err
		}
		title := "操作员已恢复目标验收"
		body := "## 操作员已恢复目标验收\n\n此 Issue 后续产出将重新交给验收 Agent 核对目标。"
		if disabled {
			title = "操作员已取消目标验收"
			body = "## 操作员已取消目标验收\n\n此 Issue 后续所有产出都会跳过验收 Agent；Worker 结束后将直接完成。"
			if hasValidation && validation.Status == "running" {
				if err := tx.Model(&IssueValidation{}).Where("id = ?", validation.ID).Updates(map[string]any{
					"status": "skipped", "feedback": "操作员取消验收，本轮验收被终止。", "completed_at": now,
				}).Error; err != nil {
					return err
				}
				if err := tx.Model(&Execution{}).Where("id = ?", validation.ValidationExecutionID).Updates(map[string]any{
					"status": "cancelled", "error": "操作员取消验收", "current_tool": "", "finished_at": now, "pid": 0, "updated_at": now,
				}).Error; err != nil {
					return err
				}
			}
		}
		if err := createSystemComment(tx, issue.ID, body, now); err != nil {
			return err
		}
		return tx.Create(&ExecutionEvent{ID: nextID("event"), IssueID: issue.ID, Type: "validation", Title: title, Detail: body, CreatedAt: now}).Error
	})
	if err != nil {
		return Issue{}, err
	}
	if disabled && hasValidation {
		if session := m.getSession(validation.ValidationExecutionID); session != nil {
			session.Close()
		}
		m.completeIssueWithoutValidation(issue, validation.SourceExecutionID, validation.CandidateResult, now)
	}
	m.store.notify()
	return m.store.GetIssue(issue.ID)
}

func (m *Manager) completeIssueWithoutValidation(issue Issue, sourceExecutionID, result string, now time.Time) {
	status := "done"
	if issue.WorkMode == "guided" {
		status = "in_review"
	}
	updates := map[string]any{
		"status": status, "labels": issueLabelsColumn(withoutIssueLabels(issue, issueLabelBlocked)), "execution_phase": "completed", "result": strings.TrimSpace(result),
		"checkout_execution_id": "", "current_execution_id": sourceExecutionID, "error": "", "completed_at": nil, "updated_at": now,
	}
	if status == "done" {
		updates["completed_at"] = now
	}
	_ = m.store.db.Model(&Issue{}).Where("id = ? AND status NOT IN ?", issue.ID, terminalIssueStatuses).Updates(updates).Error
	title := "已跳过目标验收"
	detail := "此 Issue 已配置为跳过验收，Worker 产出直接进入完成状态。"
	if strings.TrimSpace(issue.Objective) == "" {
		title = "无需目标验收"
		detail = "Issue 未设置目标，Worker 产出直接进入完成状态，未启动验收 Agent。"
	}
	if err := m.publishRootIssueTaskReport(issue, sourceExecutionID, now); err != nil {
		m.store.addEvent(sourceExecutionID, issue.ID, "error", "生成任务最终报告失败", err.Error())
	}
	m.store.addEvent(sourceExecutionID, issue.ID, "validation", title, detail)
	m.store.notify()
	m.reconcileIssueID(issue.ID)
	if issue.ParentID != "" {
		go m.scheduleChildren(issue.ParentID)
	}
}

func (m *Manager) finalizeManualAbandon(issue Issue, executionID, agentID, result string) {
	if issue.AbandonRequestedAt == nil || issue.ObjectiveAbandoned {
		return
	}
	now := time.Now()
	result = strings.TrimSpace(result)
	if executionID != "" {
		_ = m.store.updateExecution(executionID, map[string]any{"status": "completed", "result": result, "current_tool": "", "finished_at": now, "pid": 0})
	}
	if result == "" {
		result = "操作员已放弃目标；当前没有可继续使用的负责人会话，因此未生成额外总结。"
	}
	_ = m.store.db.Model(&Issue{}).Where("id = ? AND status <> ?", issue.ID, "cancelled").Updates(map[string]any{
		"status": "cancelled", "execution_phase": "completed", "checkout_execution_id": "",
		"current_execution_id": executionID, "result": result, "objective_abandoned": true,
		"abandoned_at": now, "cancelled_at": now, "error": "", "updated_at": now,
	}).Error
	m.store.addEvent(executionID, issue.ID, "cancellation", "负责人已完成取消总结", "此目标未经验收，已按操作员指令结束。")
	m.store.notify()
	m.reconcileIssueID(issue.ID)
	if issue.ParentID != "" {
		go m.scheduleChildren(issue.ParentID)
	}
}

func createSystemComment(tx *gorm.DB, issueID, body string, now time.Time) error {
	return tx.Create(&IssueComment{
		ID: nextID("comment"), IssueID: issueID, Type: "system", AuthorType: "system", AuthorID: "aegis-system",
		Body: strings.TrimSpace(body), Mentions: []string{}, CreatedAt: now,
	}).Error
}

func (m *Manager) summarizingSessionForIssue(issue Issue) *PiSession {
	if issue.CurrentExecutionID != "" {
		var current Execution
		if m.store.db.First(&current, "id = ? AND issue_id = ?", issue.CurrentExecutionID, issue.ID).Error == nil &&
			current.Kind != "validation" &&
			(issue.AssigneeAgentID == "" || current.AgentID == issue.AssigneeAgentID) {
			if session := m.getSession(current.ID); session != nil && !session.closed.Load() {
				return session
			}
		}
	}
	var executions []Execution
	query := m.store.db.Where("issue_id = ? AND kind <> ?", issue.ID, "validation")
	if issue.AssigneeAgentID != "" {
		query = query.Where("agent_id = ?", issue.AssigneeAgentID)
	}
	if query.Order("started_at desc").Find(&executions).Error != nil {
		return nil
	}
	for _, execution := range executions {
		if session := m.getSession(execution.ID); session != nil && !session.closed.Load() {
			return session
		}
	}
	return nil
}

func (m *Manager) issueDescendantIDs(root Issue) ([]string, error) {
	var issues []Issue
	if err := m.store.db.Where("project_id = ?", root.ProjectID).Find(&issues).Error; err != nil {
		return nil, err
	}
	children := make(map[string][]string)
	for _, issue := range issues {
		children[issue.ParentID] = append(children[issue.ParentID], issue.ID)
	}
	queue := append([]string(nil), children[root.ID]...)
	result := make([]string, 0, len(queue))
	seen := map[string]bool{root.ID: true}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
		queue = append(queue, children[id]...)
	}
	return result, nil
}

func (m *Manager) closeAbandonedSessions(issueID string, descendantIDs []string, preserveExecutionID string) {
	issueIDs := map[string]bool{issueID: true}
	for _, id := range descendantIDs {
		issueIDs[id] = true
	}
	m.mu.RLock()
	sessions := make([]*PiSession, 0)
	for id, session := range m.sessions {
		if issueIDs[session.issueID] && id != preserveExecutionID {
			sessions = append(sessions, session)
		}
	}
	m.mu.RUnlock()
	for _, session := range sessions {
		session.Close()
	}
}
