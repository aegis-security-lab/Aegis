package control

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"aegis/agenthost"
	"github.com/z3r2ne/agentcore"
)

const executionBudgetSummaryPrompt = `## 单次 Execution 预算已耗尽

停止继续扩展、探索或实现新内容。现在只整理这次执行已经获得的信息，并在本轮尽快给出可供父 Issue 判断的结构化总结：
1. 已完成的工作与可以验证的证据；
2. 未完成内容、失败点和风险；
3. 当前工作区/附件/提交等可复用产物；
4. 明确建议父 Issue 选择：为同一 Issue 新开一次 Execution 继续、另开 Issue 探索其他方向，或接受当前部分结果并停止。

总结阶段只允许读取状态、报告进度，并必须调用 aegis_submit_budget_summary 提交本次部分成果。不得创建子 Issue、继续大范围修改或调用 aegis_submit_final_result 提交正常完成结果。`

var executionBudgetSummaryTools = map[string]bool{
	"aegis_report_progress":       true,
	"aegis_submit_budget_summary": true,
	"phone_view":                  true,
	"phone_back":                  true,
	"phone_home":                  true,
}

type executionBudgetOutcome struct {
	Exceeded    bool
	Reason      string
	TurnsUsed   int
	SummaryUsed int
	ExceededAt  time.Time
}

// executionBudgetController is scoped to one concrete Execution. The values
// come from the Execution row rather than live settings, which makes retries,
// exports and later evaluation reproducible.
type executionBudgetController struct {
	manager     *Manager
	executionID string
	issueID     string
	startedAt   time.Time
	maxTurns    int
	maxActive   time.Duration
	maxSummary  int
	summaryTime time.Duration
	now         func() time.Time

	mu            sync.Mutex
	session       *agenthost.Session
	phase         string
	modelCalls    int
	exceededTurn  int
	summaryUsed   int
	reason        string
	exceededAt    time.Time
	summarySystem string
	summaryTools  []agentcore.Tool
}

func newExecutionBudgetController(manager *Manager, execution Execution) *executionBudgetController {
	if execution.BudgetMaxTurns <= 0 || execution.BudgetActiveMinutes <= 0 || execution.BudgetSummaryTurns <= 0 {
		return nil
	}
	return &executionBudgetController{
		manager: manager, executionID: execution.ID, issueID: execution.IssueID,
		startedAt: execution.StartedAt, maxTurns: execution.BudgetMaxTurns,
		maxActive:   time.Duration(execution.BudgetActiveMinutes) * time.Minute,
		maxSummary:  execution.BudgetSummaryTurns,
		summaryTime: time.Duration(execution.BudgetSummaryMinutes) * time.Minute,
		now:         time.Now, phase: "active",
	}
}

func (b *executionBudgetController) ConfigureAgent(_ context.Context, _ agenthost.ExecutionSpec, config *agentcore.Config) error {
	if b == nil || config == nil {
		return nil
	}
	b.mu.Lock()
	b.summarySystem = strings.TrimSpace(config.SystemPrompt) + "\n\n" + executionBudgetSummaryPrompt
	b.summaryTools = filterExecutionBudgetTools(config.Tools)
	b.mu.Unlock()
	config.MaxTurns = b.maxTurns + b.maxSummary
	config.Interceptors = append(config.Interceptors, b)
	return nil
}

func (b *executionBudgetController) BindSession(session *agenthost.Session) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.session = session
	b.mu.Unlock()
}

func (b *executionBudgetController) InterceptorName() string { return "aegis.execution_budget" }

func (b *executionBudgetController) BeforeModelCall(_ context.Context, request *agentcore.ModelRequest) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	b.modelCalls++
	turn := b.modelCalls
	phase := b.phase
	now := b.now()
	b.mu.Unlock()
	if phase == "active" && now.Sub(b.startedAt) >= b.maxActive {
		b.exceed(turn-1, fmt.Sprintf("单次 Execution 活跃时间达到 %d 分钟上限", int(b.maxActive/time.Minute)), false)
	}
	b.mu.Lock()
	if b.phase == "summarizing" {
		request.SystemPrompt = b.summarySystem
		request.Tools = filterExecutionBudgetDefinitions(request.Tools)
		if turn == b.exceededTurn+1 {
			request.Messages = append(request.Messages, agentcore.TextMessage(agentcore.RoleUser, executionBudgetSummaryPrompt+"\n\n触发原因："+b.reason))
		}
	}
	b.mu.Unlock()
	return nil
}

func (b *executionBudgetController) BeforeToolCall(_ context.Context, call agentcore.ToolCallContext) (agentcore.ToolCallDecision, error) {
	if b == nil {
		return agentcore.ToolCallDecision{}, nil
	}
	b.mu.Lock()
	phase := b.phase
	now := b.now()
	calls := b.modelCalls
	b.mu.Unlock()
	if phase == "active" && now.Sub(b.startedAt) >= b.maxActive {
		b.exceed(calls, fmt.Sprintf("单次 Execution 活跃时间达到 %d 分钟上限", int(b.maxActive/time.Minute)), true)
		phase = "summarizing"
	}
	if phase == "summarizing" && !executionBudgetSummaryTools[call.Call.Name] {
		return agentcore.ToolCallDecision{Block: true, Reason: "Execution 已进入预算总结阶段；该工具已禁用，请只整理现有成果"}, nil
	}
	return agentcore.ToolCallDecision{}, nil
}

func (b *executionBudgetController) PrepareNextTurn(_ context.Context, turn *agentcore.TurnContext) error {
	if b == nil || turn == nil {
		return nil
	}
	b.mu.Lock()
	phase := b.phase
	calls := b.modelCalls
	b.mu.Unlock()
	if phase == "active" && calls >= b.maxTurns {
		b.exceed(calls, fmt.Sprintf("单次 Execution 已使用 %d 个模型轮次，达到配置上限", calls), true)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.phase != "summarizing" {
		b.persistUsageLocked(calls)
		return nil
	}
	b.summaryUsed = max(0, b.modelCalls-b.exceededTurn)
	b.persistUsageLocked(b.modelCalls)
	tools := append([]agentcore.Tool(nil), b.summaryTools...)
	system := b.summarySystem
	turn.Next = &agentcore.NextTurnConfig{SystemPrompt: &system, Tools: &tools}
	return nil
}

func (b *executionBudgetController) ShouldStop(_ context.Context, turn agentcore.TurnContext) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.phase != "summarizing" {
		return false
	}
	b.summaryUsed = max(b.summaryUsed, b.modelCalls-b.exceededTurn)
	return b.summaryUsed >= b.maxSummary || (!b.exceededAt.IsZero() && b.now().Sub(b.exceededAt) >= b.summaryTime)
}

func (b *executionBudgetController) Outcome() executionBudgetOutcome {
	if b == nil {
		return executionBudgetOutcome{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return executionBudgetOutcome{Exceeded: b.phase == "summarizing" || b.phase == "exceeded", Reason: b.reason, TurnsUsed: b.modelCalls, SummaryUsed: b.summaryUsed, ExceededAt: b.exceededAt}
}

func (b *executionBudgetController) Finish() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.phase == "summarizing" {
		b.phase = "exceeded"
	}
	b.mu.Unlock()
}

func (b *executionBudgetController) exceed(turn int, reason string, steer bool) {
	b.mu.Lock()
	if b.phase != "active" {
		b.mu.Unlock()
		return
	}
	b.phase = "summarizing"
	b.exceededTurn = turn
	b.reason = reason
	b.exceededAt = b.now()
	session := b.session
	exceededAt := b.exceededAt
	b.mu.Unlock()

	if b.manager != nil && b.manager.store != nil {
		_ = b.manager.store.updateExecution(b.executionID, map[string]any{
			"budget_phase": "summarizing", "budget_exceeded_reason": reason,
			"budget_exceeded_at": exceededAt, "budget_turns_used": turn,
		})
		_ = b.manager.store.db.Model(&Issue{}).
			Where("id = ? AND current_execution_id = ? AND status = ?", b.issueID, b.executionID, "in_progress").
			Updates(map[string]any{"execution_phase": "budget_summarizing", "updated_at": exceededAt}).Error
		b.manager.store.addEvent(b.executionID, b.issueID, "budget", "Execution 预算已耗尽，进入受限总结", reason)
		b.manager.store.notify()
	}
	if steer && session != nil {
		_ = session.Steer(agentcore.TextMessage(agentcore.RoleUser, executionBudgetSummaryPrompt+"\n\n触发原因："+reason))
	}
	if session != nil && b.summaryTime > 0 {
		go func(target *agenthost.Session, anchor time.Time, grace time.Duration) {
			timer := time.NewTimer(grace)
			defer timer.Stop()
			<-timer.C
			b.mu.Lock()
			stillSummarizing := b.phase == "summarizing" && b.exceededAt.Equal(anchor)
			b.mu.Unlock()
			if stillSummarizing {
				_ = target.Abort()
			}
		}(session, exceededAt, b.summaryTime)
	}
}

func (b *executionBudgetController) persistUsageLocked(turn int) {
	if b.manager == nil || b.manager.store == nil {
		return
	}
	_ = b.manager.store.updateExecution(b.executionID, map[string]any{
		"budget_turns_used": turn, "budget_summary_used": b.summaryUsed,
	})
}

func filterExecutionBudgetTools(tools []agentcore.Tool) []agentcore.Tool {
	result := make([]agentcore.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool != nil && executionBudgetSummaryTools[tool.Definition().Name] {
			result = append(result, tool)
		}
	}
	return result
}

func filterExecutionBudgetDefinitions(tools []agentcore.ToolDefinition) []agentcore.ToolDefinition {
	result := make([]agentcore.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		if executionBudgetSummaryTools[tool.Name] {
			result = append(result, tool)
		}
	}
	return result
}

var _ agenthost.AgentConfigurer = (*executionBudgetController)(nil)
var _ agentcore.BeforeModelCallInterceptor = (*executionBudgetController)(nil)
var _ agentcore.BeforeToolCallInterceptor = (*executionBudgetController)(nil)
var _ agentcore.PrepareNextTurnInterceptor = (*executionBudgetController)(nil)
var _ agentcore.ShouldStopInterceptor = (*executionBudgetController)(nil)
