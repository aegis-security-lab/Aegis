package control

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const issueHeartbeatReason = "issue_heartbeat"

func (m *Manager) monitorIssueHeartbeats() {
	defer close(m.heartbeatDone)
	for {
		config := normalizeIssueHeartbeat(m.store.Config().IssueHeartbeat)
		timer := time.NewTimer(time.Duration(config.IntervalSeconds) * time.Second)
		select {
		case <-m.heartbeatStop:
			if !timer.Stop() {
				<-timer.C
			}
			return
		case now := <-timer.C:
			m.checkIssueHeartbeats(now)
		}
	}
}

func (m *Manager) checkIssueHeartbeats(now time.Time) {
	if m.closing.Load() || !m.store.Config().Configured {
		return
	}
	m.scheduleMu.Lock()
	wakeupIDs := m.queueDueIssueHeartbeats(now)
	m.scheduleMu.Unlock()
	for _, wakeupID := range wakeupIDs {
		go m.dispatchWakeup(wakeupID)
	}
}

// queueDueIssueHeartbeats must run while scheduleMu is held. AgentWakeup is
// both the durable clock and the delivery queue, so a restart never resets a
// waiting Issue's heartbeat interval or creates a duplicate live heartbeat.
func (m *Manager) queueDueIssueHeartbeats(now time.Time) []string {
	interval := time.Duration(normalizeIssueHeartbeat(m.store.Config().IssueHeartbeat).IntervalSeconds) * time.Second
	var issues []Issue
	if err := m.store.db.Where(
		"hidden = ? AND assignee_agent_id <> '' AND ((status = ? AND execution_phase = ?) OR status IN ?)",
		false, "in_progress", "waiting_children", []string{"todo", "backlog"},
	).Order("updated_at asc").Find(&issues).Error; err != nil {
		return nil
	}

	queued := make([]string, 0, len(issues))
	for _, issue := range issues {
		eligible, _ := m.issueHeartbeatEligibility(issue)
		if !eligible || m.store.belongsToCancelledTask(issue) {
			continue
		}
		if _, err := m.store.executionAgent(issue.AssigneeAgentID); err != nil {
			continue
		}
		var childWait IssueChildWait
		estimatedCheckDue := false
		if err := m.store.db.Where("parent_issue_id = ? AND status = ?", issue.ID, "waiting").Order("created_at desc").First(&childWait).Error; err == nil && childWait.NextCheckAt != nil {
			if now.Before(*childWait.NextCheckAt) {
				continue
			}
			estimatedCheckDue = true
		}

		var live AgentWakeup
		err := m.store.db.Where("issue_id = ? AND reason = ? AND status IN ?", issue.ID, issueHeartbeatReason, []string{"queued", "delivered"}).Order("created_at desc").First(&live).Error
		if err == nil {
			if live.Status == "queued" {
				queued = append(queued, live.ID)
			}
			continue
		}
		var otherWakeups int64
		if m.store.db.Model(&AgentWakeup{}).Where("issue_id = ? AND reason <> ? AND status IN ?", issue.ID, issueHeartbeatReason, []string{"queued", "delivered"}).Count(&otherWakeups).Error != nil || otherWakeups > 0 {
			continue
		}

		var active int64
		if m.store.db.Model(&Execution{}).Where("issue_id = ? AND status IN ?", issue.ID, activeExecutionStatuses).Count(&active).Error != nil || active > 0 {
			continue
		}

		anchor := issue.UpdatedAt
		var previous AgentWakeup
		if err = m.store.db.Where("issue_id = ? AND reason = ?", issue.ID, issueHeartbeatReason).Order("created_at desc").First(&previous).Error; err == nil {
			anchor = previous.CreatedAt
			if previous.CompletedAt != nil && previous.CompletedAt.After(anchor) {
				anchor = *previous.CompletedAt
			}
		}
		if anchor.IsZero() {
			anchor = issue.CreatedAt
		}
		elapsed := now.Sub(anchor)
		if !estimatedCheckDue && elapsed < interval {
			continue
		}
		if elapsed < 0 {
			elapsed = 0
		}
		wakeup := AgentWakeup{
			ID: nextID("wakeup"), IssueID: issue.ID, AgentID: issue.AssigneeAgentID,
			Reason: issueHeartbeatReason, Status: "queued", HeartbeatElapsedSeconds: int64(elapsed / time.Second), CreatedAt: now,
		}
		if err = m.store.db.Create(&wakeup).Error; err != nil {
			continue
		}
		if estimatedCheckDue {
			_ = m.store.db.Model(&IssueChildWait{}).Where("id = ?", childWait.ID).Update("next_check_at", nil).Error
		}
		queued = append(queued, wakeup.ID)
	}
	return queued
}

func (m *Manager) issueHeartbeatEligibility(issue Issue) (bool, string) {
	if issue.Hidden || strings.TrimSpace(issue.AssigneeAgentID) == "" || issueStatusTerminal(issue.Status) {
		return false, ""
	}
	if issue.Status == "in_progress" && issue.ExecutionPhase == "waiting_children" && issue.CheckoutExecutionID == "" {
		return true, "waiting_children"
	}
	return false, ""
}

func (m *Manager) heartbeatPrompt(issue Issue, wakeup AgentWakeup) (string, error) {
	eligible, _ := m.issueHeartbeatEligibility(issue)
	if !eligible {
		return "", errors.New("Issue 已不再等待子树或依赖")
	}

	var children []Issue
	if err := m.store.db.Where("parent_id = ?", issue.ID).Order("number asc").Find(&children).Error; err != nil {
		return "", err
	}
	waitLabel := "直属子 Issues 尚未结束"
	budgetSummary := "此子 Issue 的单次 Execution 预算会在 AgentCore loop 内独立计数；重新执行会获得新预算，但不会重置 Task 总时钟墙预算"
	if issue.ParentID == "" {
		budgetSummary = "Task 未设置总时钟墙预算（无限制）"
		if limit, elapsed, remaining, configured := taskWallClockRemaining(issue, time.Now()); configured {
			budgetSummary = fmt.Sprintf("Task 总时钟墙预算 %s；从创建起已过 %s；剩余 %s（等待和重新执行均不重置）", formatHeartbeatDuration(limit), formatHeartbeatDuration(elapsed), formatHeartbeatDuration(remaining))
		}
	}

	return fmt.Sprintf(`## Aegis Issue 心跳唤醒

这是一次系统心跳，不是新任务。距离上次心跳或进入等待状态已过 %s。

Issue %s：%s
当前等待原因：%s
描述：%s
目标：%s
工作区：%s
时间预算：%s

在规定时间内尽量完美地完成任务。如果超过规定时间仍未完成，将会有很严重的惩罚；如果在规定时间内高质量完成，则会获得很大的奖励。

本次唤醒用于协调而不是空等：
1. 可以继续对当前任务有价值的工作。
2. 使用 aegis_list_child_issues 查看直属子 Issue，用 aegis_get_issue_progress 根据 currentExecutionId 查看子 Agent 的进度或最近消息。
3. 对运行中或等待中的直属子 Issue，用 aegis_board 记录可见的纠正意见，并用 aegis_relay 异步通知负责人。如果当前方向不安全、完全无关或无法就地纠正，则使用 aegis_cancel_issue 停止子 Issue。
4. 需要等待子 Issue 结果时，使用 aegis_wait_for_child_issues 等待相应 childIssueIds；Relay 消息本身不会阻塞工作。
5. 如果子树仍未满足，完成本轮协调后结束回合；Aegis 会恢复等待状态并在下一次心跳再次唤醒你。

当前直属子 Issues：
%s

请先评估状态，再执行最有价值的协调动作，最后给出简短心跳总结。`,
		formatHeartbeatDuration(time.Duration(wakeup.HeartbeatElapsedSeconds)*time.Second), issue.Identifier, issue.Title,
		waitLabel, fallback(issue.Description, "未填写"), fallback(issue.Objective, "未设置"), issue.Workspace, budgetSummary,
		formatHeartbeatIssues(children)), nil
}

func formatHeartbeatIssues(issues []Issue) string {
	if len(issues) == 0 {
		return "- 无"
	}
	const maxItems = 30
	lines := make([]string, 0, min(len(issues), maxItems)+1)
	for index, issue := range issues {
		if index == maxItems {
			lines = append(lines, fmt.Sprintf("- 其余 %d 个 Issue 未展开", len(issues)-maxItems))
			break
		}
		execution := fallback(issue.CurrentExecutionID, "无")
		lines = append(lines, fmt.Sprintf("- %s [%s/%s] %s；currentExecutionId=%s", issue.Identifier, issue.Status, issue.ExecutionPhase, issue.Title, execution))
	}
	return strings.Join(lines, "\n")
}

func formatHeartbeatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	seconds := int64(duration.Round(time.Second) / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%d 分 %d 秒", minutes, seconds%60)
	}
	return fmt.Sprintf("%d 小时 %d 分", minutes/60, minutes%60)
}
