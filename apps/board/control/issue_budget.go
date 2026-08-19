package control

import (
	"fmt"
	"time"
)

const taskBudgetCheckInterval = 30 * time.Second

// monitorIssueBudgets now owns only the Task wall-clock budget. Child Issue
// Execution budgets are enforced synchronously inside the AgentCore loop, so
// they cannot overshoot because a polling tick was late.
func (m *Manager) monitorIssueBudgets() {
	defer close(m.budgetDone)
	for {
		timer := time.NewTimer(taskBudgetCheckInterval)
		select {
		case <-m.budgetStop:
			if !timer.Stop() {
				<-timer.C
			}
			return
		case now := <-timer.C:
			m.checkIssueBudgets(now)
		}
	}
}

// checkIssueBudgets measures a root Issue from its creation time. Sleeping,
// waiting and newly-created Executions do not reset this Task-level clock.
func (m *Manager) checkIssueBudgets(now time.Time) {
	if !m.store.Config().Configured {
		return
	}
	var roots []Issue
	if err := m.store.db.Where("parent_id = '' AND hidden = ? AND time_budget_minutes IS NOT NULL AND status NOT IN ?", false, terminalIssueStatuses).Find(&roots).Error; err != nil {
		return
	}
	for _, root := range roots {
		if root.TimeBudgetMinutes == nil || *root.TimeBudgetMinutes <= 0 {
			continue
		}
		elapsed := taskWallClockUsage(root, now)
		limit := time.Duration(*root.TimeBudgetMinutes) * time.Minute
		if elapsed < limit {
			continue
		}
		reason := fmt.Sprintf("Task 总时钟墙预算已耗尽：从任务创建起已过 %.1f 分钟，配置上限 %d 分钟；重新执行子 Issue 不会重置此预算。", elapsed.Minutes(), *root.TimeBudgetMinutes)
		if _, err := m.abandonIssueWithSummary(root.ID, reason, issueBudgetSummaryInstruction, "Task 总时间预算已耗尽", "task_wall_budget_exhausted", true); err != nil {
			m.store.addEvent(root.CurrentExecutionID, root.ID, "error", "Task 总预算耗尽后结束目标失败", err.Error())
		} else {
			m.abortNativeIssue(root.ID)
		}
	}
}

func taskWallClockUsage(root Issue, now time.Time) time.Duration {
	anchor := root.CreatedAt
	if anchor.IsZero() || !now.After(anchor) {
		return 0
	}
	return now.Sub(anchor)
}

func taskWallClockRemaining(root Issue, now time.Time) (limit, elapsed, remaining time.Duration, configured bool) {
	if root.TimeBudgetMinutes == nil || *root.TimeBudgetMinutes <= 0 {
		return 0, 0, 0, false
	}
	limit = time.Duration(*root.TimeBudgetMinutes) * time.Minute
	elapsed = taskWallClockUsage(root, now)
	remaining = limit - elapsed
	if remaining < 0 {
		remaining = 0
	}
	return limit, elapsed, remaining, true
}
