package control

import (
	"fmt"
	"strings"
	"time"
)

type issueBudgetUsage struct {
	Tokens   int64
	Cost     float64
	Duration time.Duration
}

func (m *Manager) monitorIssueBudgets() {
	defer close(m.budgetDone)
	for {
		budget := normalizeIssueBudget(m.store.Config().IssueBudget)
		interval := time.Duration(budget.CheckIntervalSeconds) * time.Second
		timer := time.NewTimer(interval)
		select {
		case <-m.budgetStop:
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
			m.checkIssueBudgets(time.Now())
		}
	}
}

func (m *Manager) checkIssueBudgets(now time.Time) {
	config := m.store.Config()
	if !config.Configured {
		return
	}
	budget := normalizeIssueBudget(config.IssueBudget)
	if budget.TokenLimit == nil && budget.CostLimit == nil && budget.TimeLimitMinutes == nil {
		return
	}
	var issues []Issue
	if err := m.store.db.Where("status = ? AND execution_phase NOT IN ? AND abandon_requested_at IS NULL", "in_progress", []string{"waiting_children", "summarizing"}).Find(&issues).Error; err != nil {
		return
	}
	for _, issue := range issues {
		usage, ok := m.issueBudgetUsage(issue.ID, now)
		if !ok {
			continue
		}
		reason := issueBudgetExceededReason(budget, usage)
		if reason == "" {
			continue
		}
		if _, err := m.abandonIssueWithSummary(issue.ID, reason, issueBudgetSummaryInstruction, "Issue 执行预算已耗尽", "issue_budget_exhausted", true); err != nil {
			m.store.addEvent(issue.CurrentExecutionID, issue.ID, "error", "预算耗尽后放弃目标失败", err.Error())
		}
	}
}

func (m *Manager) issueBudgetUsage(issueID string, now time.Time) (issueBudgetUsage, bool) {
	var executions []Execution
	if err := m.store.db.Where("issue_id = ? AND kind <> ?", issueID, "concierge").Find(&executions).Error; err != nil || len(executions) == 0 {
		return issueBudgetUsage{}, false
	}
	usage := issueBudgetUsage{}
	for _, execution := range executions {
		usage.Tokens += execution.Tokens
		usage.Cost += execution.Cost
		end := now
		if execution.FinishedAt != nil {
			end = *execution.FinishedAt
		}
		if end.After(execution.StartedAt) {
			usage.Duration += end.Sub(execution.StartedAt)
		}
	}
	return usage, true
}

func issueBudgetExceededReason(budget IssueBudgetConfig, usage issueBudgetUsage) string {
	reasons := make([]string, 0, 3)
	if budget.TokenLimit != nil && usage.Tokens >= *budget.TokenLimit {
		reasons = append(reasons, fmt.Sprintf("Token 预算已达到：当前累计 %d tokens，配置上限 %d tokens", usage.Tokens, *budget.TokenLimit))
	}
	if budget.CostLimit != nil && usage.Cost >= *budget.CostLimit {
		reasons = append(reasons, fmt.Sprintf("成本预算已达到：当前累计 $%.6f，配置上限 $%.6f", usage.Cost, *budget.CostLimit))
	}
	if budget.TimeLimitMinutes != nil {
		limit := time.Duration(*budget.TimeLimitMinutes) * time.Minute
		if usage.Duration >= limit {
			reasons = append(reasons, fmt.Sprintf("时间预算已达到：当前累计执行 %.1f 分钟，配置上限 %d 分钟", usage.Duration.Minutes(), *budget.TimeLimitMinutes))
		}
	}
	if len(reasons) == 0 {
		return ""
	}
	return "系统预算轮询触发目标放弃。" + strings.Join(reasons, "；") + "。"
}
