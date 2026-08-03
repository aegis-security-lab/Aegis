package control

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"aegis/agenthost"
	"aegis/capability"
	"aegis/coordination"
	"github.com/z3r2ne/agentcore"
	"gorm.io/gorm"
)

// NativeIssueRunner is the only Issue execution path. Coordination owns when
// it runs; AgentHost owns how the Agent loop and capabilities are materialized.
type NativeIssueRunner struct {
	Manager  *Manager
	Host     agenthost.Runner
	Sessions *NativeSessionDelivery
}

func (r NativeIssueRunner) Run(ctx context.Context, scheduled agenthost.ExecutionSpec, sink agentcore.EventSink) (agenthost.Result, error) {
	if r.Manager == nil || r.Manager.store == nil || r.Host == nil {
		return agenthost.Result{}, errors.New("control native runner: manager and host are required")
	}
	issueID, _ := scheduled.Values[controlIssueIDValue].(string)
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return agenthost.Result{}, errors.New("control native runner: issue ID is required")
	}
	issue, err := r.Manager.store.GetIssue(issueID)
	if err != nil {
		return agenthost.Result{}, err
	}
	r.Manager.scheduleMu.Lock()
	var prepared preparedIssueExecution
	preparedID, _ := scheduled.Values[controlPreparedExecutionIDValue].(string)
	preparedID = strings.TrimSpace(preparedID)
	if preparedID != "" {
		if issue.Status == "todo" && issue.ExecutionPhase == "recovering" && issue.RecoveryExecutionID == preparedID {
			// A Coordination lease survives a service restart, while Store startup
			// deliberately marks the domain Execution disconnected. When the same
			// prepared envelope is reclaimed, resume it through the recovery state
			// machine instead of rejecting the disconnected Execution as terminal.
			var prompt string
			var recoveryExecution Execution
			var recoveryAgent AgentDefinition
			issue, recoveryExecution, recoveryAgent, prompt, err = r.Manager.prepareIssueRecovery(issue.ID)
			prepared = preparedIssueExecution{issue: issue, execution: recoveryExecution, agent: recoveryAgent, prompt: prompt}
		} else {
			prompt, _ := scheduled.Values[controlPreparedPromptValue].(string)
			prepared, err = r.prepareExistingExecution(issue, preparedID, prompt)
		}
	} else if issue.Status == "todo" && issue.ExecutionPhase == "recovering" && issue.RecoveryExecutionID != "" {
		var prompt string
		var recoveryExecution Execution
		var recoveryAgent AgentDefinition
		issue, recoveryExecution, recoveryAgent, prompt, err = r.Manager.prepareIssueRecovery(issue.ID)
		prepared = preparedIssueExecution{issue: issue, execution: recoveryExecution, agent: recoveryAgent, prompt: prompt}
	} else if resume, _ := scheduled.Values["control.resume"].(bool); resume {
		prompt, _ := scheduled.Values["control.resumePrompt"].(string)
		prepared, err = r.prepareIssueResume(issue, prompt)
	} else {
		prepared, err = r.Manager.prepareIssueExecution(issue.ID)
	}
	r.Manager.scheduleMu.Unlock()
	if err != nil {
		if preparedID != "" {
			r.reconcilePreparedExecutionFailure(issue, preparedID, err)
		}
		return agenthost.Result{}, err
	}
	if !slices.Contains([]string{"work", "wakeup", "planning", "rework", "continuation", "recovery", "heartbeat", "validation"}, prepared.execution.Kind) {
		return agenthost.Result{}, fmt.Errorf("control native runner: unsupported Issue execution kind %q", prepared.execution.Kind)
	}
	wakeupID, _ := scheduled.Values[controlWakeupIDValue].(string)
	if strings.TrimSpace(wakeupID) != "" {
		defer r.settlePreparedWakeup(wakeupID, prepared)
	}
	nativeSpec, err := r.executionSpec(ctx, prepared)
	if err != nil {
		r.reconcilePreparedExecutionFailure(prepared.issue, prepared.execution.ID, err)
		return agenthost.Result{}, err
	}
	budgetController := newExecutionBudgetController(r.Manager, prepared.execution)
	if budgetController != nil {
		nativeSpec.Runtime = budgetController
	}
	now := time.Now()
	runtimeID := scheduled.ExecutionID
	if containerRuntimeID, ok := nativeSpec.Values["control.runtimeId"].(string); ok && strings.TrimSpace(containerRuntimeID) != "" {
		runtimeID = containerRuntimeID
	}
	if err := r.Manager.store.updateExecution(prepared.execution.ID, map[string]any{
		"status": "running", "runtime_type": "agentcore", "runtime_id": runtimeID,
		"initial_prompt": nativeSpec.Prompt, "system_prompt": nativeSpec.SystemPrompt, "pid": 0,
	}); err != nil {
		r.reconcilePreparedExecutionFailure(prepared.issue, prepared.execution.ID, err)
		return agenthost.Result{}, err
	}
	if err := r.Manager.store.db.Create(&Message{
		ID: nextID("message"), ExecutionID: prepared.execution.ID, IssueID: prepared.issue.ID,
		Role: "user", Content: nativeSpec.Prompt, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		r.reconcilePreparedExecutionFailure(prepared.issue, prepared.execution.ID, err)
		return agenthost.Result{}, err
	}
	_ = r.Manager.store.db.Model(&Execution{}).Where("id = ?", prepared.execution.ID).UpdateColumn("message_count", gorm.Expr("message_count + 1")).Error
	r.Manager.store.addEvent(prepared.execution.ID, prepared.issue.ID, "runtime", prepared.agent.Name+" 已连接 Go AgentCore", "Coordination Execution "+scheduled.ExecutionID)

	combinedSink := r.eventSink(prepared, sink)
	var result agenthost.Result
	var runErr error
	if sessionHost, ok := r.Host.(*agenthost.Host); ok && r.Sessions != nil {
		result, runErr = r.Sessions.Run(ctx, sessionHost, prepared.issue.ID, nativeSpec, combinedSink)
	} else {
		result, runErr = r.Host.Run(ctx, nativeSpec, combinedSink)
	}
	budgetController.Finish()
	persistErr := r.persistMessages(prepared, result.Core.NewMessages)
	if persistErr != nil {
		runErr = errors.Join(runErr, persistErr)
	}
	resultText := lastAssistantText(result.Core.State.Messages)
	var completedExecution Execution
	if lookupErr := r.Manager.store.db.First(&completedExecution, "id = ?", prepared.execution.ID).Error; lookupErr == nil && completedExecution.FinalResultSubmitted {
		resultText = completedExecution.FinalResult
	}
	finishedAt := time.Now()
	updates := map[string]any{
		"result": resultText, "current_tool": "", "finished_at": finishedAt,
		"capabilities_snapshot": result.Capabilities,
		"input_tokens":          result.Core.Usage.InputTokens, "output_tokens": result.Core.Usage.OutputTokens,
		"cache_read_tokens": result.Core.Usage.CacheReadTokens, "cache_write_tokens": result.Core.Usage.CacheWriteTokens,
		"tokens": result.Core.Usage.InputTokens + result.Core.Usage.OutputTokens + result.Core.Usage.CacheReadTokens + result.Core.Usage.CacheWriteTokens,
		"cost":   calculateModelCost(prepared.execution.Pricing, int64(result.Core.Usage.InputTokens), int64(result.Core.Usage.OutputTokens), int64(result.Core.Usage.CacheReadTokens), int64(result.Core.Usage.CacheWriteTokens)),
	}
	if prepared.execution.Kind == "validation" {
		return result, r.finishValidation(prepared, resultText, runErr, persistErr, updates, finishedAt)
	}
	if outcome := budgetController.Outcome(); outcome.Exceeded {
		// The dedicated summary tool persists its body before terminating the
		// loop. Prefer that durable submission over incidental final assistant
		// prose or a provider-generated tool acknowledgement.
		var summarizedExecution Execution
		if lookupErr := r.Manager.store.db.First(&summarizedExecution, "id = ?", prepared.execution.ID).Error; lookupErr == nil && strings.TrimSpace(summarizedExecution.Result) != "" {
			resultText = summarizedExecution.Result
			updates["result"] = resultText
		}
		if persistErr != nil {
			return result, persistErr
		}
		if runErr != nil {
			resultText = strings.TrimSpace(resultText + "\n\n总结阶段运行异常：" + runErr.Error())
			updates["result"] = resultText
		}
		if err := r.finalizeBudgetExceeded(prepared, resultText, outcome, updates, finishedAt); err != nil {
			return result, err
		}
		return result, nil
	}
	if ctx.Err() != nil {
		// Cancellation inherited from the Coordination worker is an
		// infrastructure interruption (most commonly service shutdown), not an
		// Agent outcome. Persist a recoverable checkpoint and let the durable
		// Coordination lease be reclaimed after restart.
		if current, lookupErr := r.Manager.store.GetIssue(prepared.issue.ID); lookupErr == nil && issueStatusTerminal(current.Status) {
			return result, nil
		}
		if recoveryErr := r.interruptForRecovery(prepared, updates, ctx.Err()); recoveryErr != nil {
			return result, errors.Join(runErr, persistErr, ctx.Err(), recoveryErr)
		}
		return result, errors.Join(runErr, persistErr, ctx.Err())
	}
	if runErr != nil {
		if current, lookupErr := r.Manager.store.GetIssue(prepared.issue.ID); lookupErr == nil && issueStatusTerminal(current.Status) {
			// An operator cancellation or Task wall-clock expiry may abort the
			// native session after the durable Issue was already finalized.
			// Preserve that business outcome instead of rewriting it as a crash.
			return result, nil
		}
		updates["status"], updates["error"] = "failed", runErr.Error()
		_ = r.Manager.store.updateExecution(prepared.execution.ID, updates)
		r.Manager.failExecution(prepared.issue, prepared.execution, runErr)
		return result, runErr
	}
	updates["status"], updates["error"] = "completed", ""
	if err := r.Manager.store.updateExecution(prepared.execution.ID, updates); err != nil {
		r.Manager.failExecution(prepared.issue, prepared.execution, err)
		return result, err
	}
	current, err := r.Manager.store.GetIssue(prepared.issue.ID)
	if err != nil {
		return result, err
	}
	if issueStatusTerminal(current.Status) {
		return result, nil
	}
	if current.ExecutionPhase == "sleeping" || (result.Core.StopReason == agentcore.StopReasonTerminated && !completedExecution.FinalResultSubmitted) {
		r.Manager.store.addEvent(prepared.execution.ID, current.ID, "sleeping", "Agent 已结束当前回合并进入休眠", "当前 AgentCore loop 已释放；后续 timer、heartbeat、Board 或 Relay 消息会创建新的 Coordination Execution。")
		return result, nil
	}
	if !completedExecution.FinalResultSubmitted {
		unfinished, childErr := r.unfinishedChildren(current.ID)
		if childErr != nil {
			return result, childErr
		}
		if unfinished > 0 {
			updated := r.Manager.store.db.Model(&Issue{}).
				Where("id = ? AND status = ? AND current_execution_id = ?", current.ID, "in_progress", prepared.execution.ID).
				Updates(map[string]any{"execution_phase": "waiting_children", "checkout_execution_id": "", "sleep_token": "", "updated_at": finishedAt})
			if updated.Error != nil {
				return result, updated.Error
			}
			if updated.RowsAffected == 1 {
				r.Manager.store.addEvent(prepared.execution.ID, current.ID, "waiting", "Agent 已释放当前回合并等待子 Issue", fmt.Sprintf("仍有 %d 个直属子 Issue 未结束；后续唤醒将创建新的 Execution。", unfinished))
				r.Manager.store.notify()
			}
			return result, nil
		}
	}
	prepared.execution.Result = resultText
	authorID := fallback(prepared.issue.AssigneeTaskAgentID, prepared.agent.ID)
	if current.ValidationDisabled || strings.TrimSpace(current.Objective) == "" {
		r.Manager.addTypedAgentComment(current.ID, authorID, "delivery", resultText, prepared.execution.ID, []commentWakeupTarget{})
		r.Manager.completeIssueWithoutValidation(current, prepared.execution.ID, resultText, finishedAt)
		return result, nil
	}
	if err := r.Manager.publishDeliveryForValidation(current, prepared.execution, authorID, resultText); err != nil {
		r.Manager.blockIssue(current, "发布 Native Runner 验收交付失败", err)
		return result, err
	}
	// Validation is a separate Coordination execution. Release this worker now
	// so a saturated worker pool cannot deadlock while every delivery waits for
	// an acceptance turn that has no free slot.
	return result, nil
}

func (r NativeIssueRunner) interruptForRecovery(prepared preparedIssueExecution, executionUpdates map[string]any, cause error) error {
	if r.Manager == nil || r.Manager.store == nil {
		return errors.New("control native runner: recovery store is unavailable")
	}
	current, err := r.Manager.store.GetIssue(prepared.issue.ID)
	if err != nil {
		return err
	}
	if issueStatusTerminal(current.Status) {
		return nil
	}
	if current.Status != "in_progress" || current.CurrentExecutionID != prepared.execution.ID {
		return errors.New("control native runner: interrupted execution no longer owns the Issue")
	}
	now := time.Now()
	recoveryPhase := current.ExecutionPhase
	if recoveryPhase == "" || recoveryPhase == "scheduled" {
		recoveryPhase = "active"
	}
	updates := make(map[string]any, len(executionUpdates)+7)
	for key, value := range executionUpdates {
		updates[key] = value
	}
	updates["status"] = "disconnected"
	updates["error"] = ""
	updates["current_tool"] = ""
	updates["finished_at"] = now
	updates["pid"] = 0
	updates["checkpoint"] = "Aegis 执行上下文已中断，等待从原 Session 恢复"
	updates["checkpoint_at"] = now

	err = r.Manager.store.db.Transaction(func(tx *gorm.DB) error {
		updatedExecution := tx.Model(&Execution{}).
			Where("id = ? AND issue_id = ? AND status NOT IN ?", prepared.execution.ID, current.ID, []string{"completed", "failed", "cancelled", "budget_exceeded"}).
			Updates(updates)
		if updatedExecution.Error != nil {
			return updatedExecution.Error
		}
		if updatedExecution.RowsAffected != 1 {
			return errors.New("interrupted Execution is no longer recoverable")
		}
		updatedIssue := tx.Model(&Issue{}).
			Where("id = ? AND status = ? AND current_execution_id = ?", current.ID, "in_progress", prepared.execution.ID).
			Updates(map[string]any{
				"status": "todo", "execution_phase": "recovering", "checkout_execution_id": "",
				"recovery_execution_id": prepared.execution.ID, "recovery_phase": recoveryPhase,
				"recovery_requested_at": now, "error": "", "updated_at": now,
			})
		if updatedIssue.Error != nil {
			return updatedIssue.Error
		}
		if updatedIssue.RowsAffected != 1 {
			return errors.New("interrupted Issue is no longer recoverable")
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.Manager.store.addEvent(prepared.execution.ID, current.ID, "recovery", "执行因服务关闭而暂停", fmt.Sprintf("保留原 AgentCore Session 和 Execution；中断原因：%v", cause))
	r.Manager.store.notify()
	return nil
}

func (r NativeIssueRunner) finishValidation(prepared preparedIssueExecution, resultText string, runErr, persistErr error, updates map[string]any, _ time.Time) error {
	if persistErr != nil {
		runErr = errors.Join(runErr, persistErr)
	}
	if runErr != nil {
		r.Manager.store.addEvent(prepared.execution.ID, prepared.issue.ID, "error", "Go AgentCore 验收回合异常", runErr.Error())
		if resultText == "" {
			resultText = runErr.Error()
		}
	}
	// Persist native telemetry before the validation state machine potentially
	// reuses the fixed validation Execution for another attempt.
	delete(updates, "result")
	delete(updates, "current_tool")
	delete(updates, "finished_at")
	updates["runtime_type"] = "agentcore"
	updates["pid"] = 0
	if err := r.Manager.store.updateExecution(prepared.execution.ID, updates); err != nil {
		return err
	}
	// The structured decision is authoritative. The legacy text parser remains
	// only for executions created before the native capability was installed.
	r.Manager.settleValidation(prepared.issue, prepared.execution.ID, resultText)
	// A validation model/tool failure is converted into the existing durable
	// validation retry state machine; returning it here would make Coordination
	// replay the same prepared envelope as a second concurrent validator.
	return nil
}

func (r NativeIssueRunner) finalizeBudgetExceeded(prepared preparedIssueExecution, summary string, outcome executionBudgetOutcome, executionUpdates map[string]any, finishedAt time.Time) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "本次 Execution 已超过预算，但 Agent 未能生成有效总结。请检查该 Execution 的消息、事件和工作区产物后再决定是否继续。"
	}
	detail := fmt.Sprintf("%s。工作轮数上限 %d；总结最多 %d 轮。本次共调用模型 %d 轮，其中总结阶段 %d 轮。", outcome.Reason, prepared.execution.BudgetMaxTurns, prepared.execution.BudgetSummaryTurns, outcome.TurnsUsed, outcome.SummaryUsed)
	executionUpdates["status"] = "budget_exceeded"
	executionUpdates["error"] = ""
	executionUpdates["result"] = summary
	executionUpdates["budget_phase"] = "exceeded"
	executionUpdates["budget_turns_used"] = outcome.TurnsUsed
	executionUpdates["budget_summary_used"] = outcome.SummaryUsed
	executionUpdates["budget_exceeded_reason"] = outcome.Reason
	executionUpdates["budget_exceeded_at"] = outcome.ExceededAt
	if err := r.Manager.store.updateExecution(prepared.execution.ID, executionUpdates); err != nil {
		return err
	}
	now := finishedAt
	updated := r.Manager.store.db.Model(&Issue{}).
		Where("id = ? AND current_execution_id = ? AND status NOT IN ?", prepared.issue.ID, prepared.execution.ID, terminalIssueStatuses).
		Updates(map[string]any{
			"status": "budget_exceeded", "execution_phase": "budget_exceeded", "result": summary,
			"error": outcome.Reason, "checkout_execution_id": "", "completed_at": now, "updated_at": now,
		})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected == 0 {
		return errors.New("control native runner: budget outcome lost Issue ownership")
	}
	if prepared.issue.AssigneeTaskAgentID != "" {
		if err := r.Manager.store.db.Model(&TaskAgent{}).Where("id = ?", prepared.issue.AssigneeTaskAgentID).Updates(map[string]any{"status": "completed", "updated_at": now}).Error; err != nil {
			return err
		}
	}
	authorID := fallback(prepared.issue.AssigneeTaskAgentID, prepared.agent.ID)
	r.Manager.addTypedAgentComment(prepared.issue.ID, authorID, "budget_summary", "## Execution 超出预算\n\n"+detail+"\n\n## Agent 总结\n\n"+summary, prepared.execution.ID, []commentWakeupTarget{})
	r.Manager.store.addEvent(prepared.execution.ID, prepared.issue.ID, "budget", "Execution 已按预算结束", detail)
	r.Manager.store.notify()
	current, err := r.Manager.store.GetIssue(prepared.issue.ID)
	if err != nil {
		return err
	}
	r.Manager.ReconcileIssue(current)
	return nil
}

func (r NativeIssueRunner) prepareIssueResume(issue Issue, wakeMessage string) (preparedIssueExecution, error) {
	if issue.Status != "in_progress" || issue.ExecutionPhase != "resuming" || issue.CheckoutExecutionID != "" {
		return preparedIssueExecution{}, errors.New("control native runner: Issue is not ready to resume")
	}
	agent, err := r.Manager.store.chooseAgent(issue)
	if err != nil {
		return preparedIssueExecution{}, err
	}
	execution, err := r.Manager.store.createExecution(issue, agent.ID, "wakeup")
	if err != nil {
		return preparedIssueExecution{}, err
	}
	now := time.Now()
	updated := r.Manager.store.db.Model(&Issue{}).
		Where("id = ? AND status = ? AND execution_phase = ? AND checkout_execution_id = ''", issue.ID, "in_progress", "resuming").
		Updates(map[string]any{"execution_phase": "active", "sleep_token": "", "checkout_execution_id": execution.ID, "current_execution_id": execution.ID, "error": "", "updated_at": now})
	if updated.Error != nil || updated.RowsAffected != 1 {
		cause := updated.Error
		if cause == nil {
			cause = errors.New("resume checkout conflict")
		}
		_ = r.Manager.store.updateExecution(execution.ID, map[string]any{"status": "failed", "error": cause.Error(), "finished_at": now})
		return preparedIssueExecution{}, cause
	}
	issue, _ = r.Manager.store.GetIssue(issue.ID)
	maxDepth, maxPerRequest, maxDirect := normalizeDecompositionLimits(r.Manager.store.Config().MaxIssueDepth, r.Manager.store.Config().MaxChildrenPerRequest, r.Manager.store.Config().MaxDirectChildren)
	budget := normalizeIssueBudget(r.Manager.store.Config().IssueBudget)
	prompt := fmt.Sprintf("%s\n\n## 本轮唤醒原因\n%s\n\n这是同一 TaskAgent Session 的新回合。先结合已有会话、当前 Board/Relay 和工作区状态判断下一步；不要重复已完成的工作。", workerPrompt(issue, maxDepth, maxPerRequest, maxDirect, budget), fallback(strings.TrimSpace(wakeMessage), "任务内持久化唤醒信号已到达。"))
	if attention := r.terminalChildAttentionPrompt(issue.ID); attention != "" {
		prompt += "\n\n" + attention
	}
	r.Manager.store.notify()
	return preparedIssueExecution{issue: issue, execution: execution, agent: agent, prompt: prompt}, nil
}

func (r NativeIssueRunner) prepareExistingExecution(issue Issue, executionID, prompt string) (preparedIssueExecution, error) {
	executionID, prompt = strings.TrimSpace(executionID), strings.TrimSpace(prompt)
	if executionID == "" || prompt == "" {
		return preparedIssueExecution{}, errors.New("control native runner: prepared Execution and prompt are required")
	}
	var execution Execution
	if err := r.Manager.store.db.First(&execution, "id = ? AND issue_id = ?", executionID, issue.ID).Error; err != nil {
		return preparedIssueExecution{}, errors.New("control native runner: prepared Execution was not found")
	}
	if slices.Contains([]string{"completed", "failed", "cancelled", "budget_exceeded", "disconnected"}, execution.Status) {
		return preparedIssueExecution{}, fmt.Errorf("control native runner: prepared Execution is already %s", execution.Status)
	}
	var agent AgentDefinition
	var err error
	if execution.Kind == "validation" {
		agent, err = r.Manager.store.GetAgent(execution.AgentID)
		if err != nil || !agent.Enabled || !agent.Internal || agent.ID != "acceptance-validator" {
			return preparedIssueExecution{}, errors.New("control native runner: validation Agent is unavailable")
		}
	} else {
		agent, err = r.Manager.store.executionAgent(execution.AgentID)
		if err != nil {
			return preparedIssueExecution{}, err
		}
	}
	return preparedIssueExecution{issue: issue, execution: execution, agent: agent, prompt: prompt}, nil
}

// reconcilePreparedExecutionFailure keeps the durable Issue state aligned with
// the Coordination envelope when materialization fails before AgentCore starts.
// Validation failures must also close the active validation row; otherwise the
// UI and parent Issue would wait forever on an Execution that no longer exists.
func (r NativeIssueRunner) reconcilePreparedExecutionFailure(issue Issue, executionID string, cause error) {
	var execution Execution
	if err := r.Manager.store.db.First(&execution, "id = ? AND issue_id = ?", executionID, issue.ID).Error; err != nil {
		return
	}
	if slices.Contains([]string{"completed", "failed", "cancelled", "budget_exceeded", "disconnected"}, execution.Status) {
		return
	}
	if execution.Kind == "validation" {
		now := time.Now()
		_ = r.Manager.store.db.Model(&IssueValidation{}).
			Where("validation_execution_id = ? AND status = ?", execution.ID, "running").
			Updates(map[string]any{"status": "error", "error": cause.Error(), "completed_at": now}).Error
	}
	current, err := r.Manager.store.GetIssue(issue.ID)
	if err != nil || current.CurrentExecutionID != execution.ID || issueStatusTerminal(current.Status) {
		_ = r.Manager.store.updateExecution(execution.ID, map[string]any{
			"status": "failed", "error": cause.Error(), "finished_at": time.Now(), "pid": 0,
		})
		return
	}
	r.Manager.failExecution(current, execution, cause)
}

func (r NativeIssueRunner) settlePreparedWakeup(wakeupID string, prepared preparedIssueExecution) {
	var execution Execution
	if err := r.Manager.store.db.First(&execution, "id = ?", prepared.execution.ID).Error; err != nil {
		return
	}
	now := time.Now()
	switch execution.Status {
	case "completed", "budget_exceeded":
		_ = r.Manager.store.db.Model(&AgentWakeup{}).Where("id = ? AND status = ?", wakeupID, "delivered").Updates(map[string]any{"status": "completed", "completed_at": now}).Error
	case "failed", "cancelled":
		_ = r.Manager.store.db.Model(&AgentWakeup{}).Where("id = ? AND status = ?", wakeupID, "delivered").Updates(map[string]any{"status": "failed", "error": execution.Error, "completed_at": now}).Error
		r.Manager.restoreIssueAfterWakeup(prepared.issue.ID, prepared.execution.ID)
	default:
		return
	}
	r.Manager.store.notify()
}

func (r NativeIssueRunner) terminalChildAttentionPrompt(parentID string) string {
	if r.Manager == nil || r.Manager.store == nil {
		return ""
	}
	var children []Issue
	if err := r.Manager.store.db.Where("parent_id = ? AND status IN ?", parentID, []string{"failed", "budget_exceeded"}).Order("updated_at desc, number asc").Limit(20).Find(&children).Error; err != nil || len(children) == 0 {
		return ""
	}
	var summary strings.Builder
	summary.WriteString("## 必须处理的直属子 Issue 异常结果\n\n恢复 Execution 排队期间出现了以下失败或超预算结果。不要继续被动等待；逐项明确选择 phone_board_continue_issue、使用 phone_board_delegate 委派替代方向，或接受部分结果并继续父目标：\n")
	for _, child := range children {
		fmt.Fprintf(&summary, "\n- %s · %s [%s]\n  结果：%s\n  错误：%s\n", child.Identifier, child.Title, child.Status, fallback(truncate(strings.TrimSpace(child.Result), 2400), "无结果摘要"), fallback(truncate(strings.TrimSpace(child.Error), 1000), "无"))
	}
	return summary.String()
}

func (r NativeIssueRunner) unfinishedChildren(issueID string) (int64, error) {
	var count int64
	err := r.Manager.store.db.Model(&Issue{}).Where("parent_id = ? AND status NOT IN ?", issueID, terminalIssueStatuses).Count(&count).Error
	return count, err
}

func (r NativeIssueRunner) executionSpec(ctx context.Context, prepared preparedIssueExecution) (agenthost.ExecutionSpec, error) {
	cfg := r.Manager.store.effectiveAgentConfig(prepared.agent)
	if !cfg.Configured {
		return agenthost.ExecutionSpec{}, errors.New("Aegis 尚未配置")
	}
	bases, err := r.Manager.store.knowledgeBasesByIDs(prepared.agent.KnowledgeBaseIDs)
	if err != nil {
		return agenthost.ExecutionSpec{}, err
	}
	runtimeWorkspace := prepared.issue.Workspace
	runtimeID := ""
	var taskContainer *ContainerInstance
	if prepared.issue.ContainerProfileID != "" {
		container, containerErr := r.Manager.store.ensureTaskContainer(prepared.issue)
		if containerErr != nil {
			return agenthost.ExecutionSpec{}, fmt.Errorf("control native runner: prepare Task container: %w", containerErr)
		}
		taskContainer = &container
		runtimeWorkspace = container.WorkspacePath
		runtimeID = container.Name
	}
	if prepared.execution.Kind == "validation" {
		if taskContainer == nil {
			return agenthost.ExecutionSpec{}, errors.New("control native runner: validation requires a Task container")
		}
		validation, validationErr := r.Manager.activeValidationForExecution(prepared.execution.ID)
		if validationErr != nil {
			return agenthost.ExecutionSpec{}, validationErr
		}
		if _, validationErr = r.Manager.materializeValidationAttachments(validation.SourceExecutionID, *taskContainer); validationErr != nil {
			return agenthost.ExecutionSpec{}, validationErr
		}
		systemPrompt := agentKnowledgeSystemPrompt(prepared.agent.SystemPrompt, bases)
		systemPrompt = agentSecurityOutcomeGradeSystemPrompt(systemPrompt, prepared.agent.Category)
		systemPrompt = agentLanguageSystemPrompt(systemPrompt, cfg.Language)
		systemPrompt = agentPermissionSystemPrompt(systemPrompt, runtimeWorkspace, prepared.agent.Permissions)
		options := map[string]any{}
		if thinking := strings.TrimSpace(cfg.Thinking); thinking != "" && thinking != "off" {
			options["reasoning_effort"] = thinking
		}
		return agenthost.ExecutionSpec{
			ExecutionID: prepared.execution.ID, AgentID: prepared.agent.ID, SessionID: prepared.execution.SessionID,
			Workspace:    runtimeWorkspace,
			Model:        agenthost.ModelRef{Provider: cfg.Provider, Model: cfg.Model, Options: options},
			SystemPrompt: systemPrompt, Prompt: rewriteWorkspacePaths(prepared.prompt, runtimeWorkspace, prepared.issue.Workspace, cfg.Workspace),
			Capabilities: []capability.Ref{{Kind: capability.KindTool, Name: "workspace"}, {Kind: capability.KindTool, Name: "validation"}},
			Values:       map[string]any{"control.issueId": prepared.issue.ID, "control.executionId": prepared.execution.ID, "control.runtimeId": runtimeID},
		}, nil
	}
	prompt := rewriteWorkspacePaths(prepared.prompt, runtimeWorkspace, prepared.issue.Workspace, cfg.Workspace)
	attachments, err := r.Manager.store.materializeInputAttachments(prepared.issue, prepared.execution, taskContainer)
	if err != nil {
		return agenthost.ExecutionSpec{}, err
	}
	systemPrompt := agentKnowledgeSystemPrompt(prepared.agent.SystemPrompt, bases)
	systemPrompt = agentSecurityOutcomeGradeSystemPrompt(systemPrompt, prepared.agent.Category)
	systemPrompt = agentLanguageSystemPrompt(systemPrompt, cfg.Language)
	systemPrompt = agentExecutionEfficiencySystemPrompt(systemPrompt)
	var activeExecutions int64
	_ = r.Manager.store.db.Model(&Execution{}).Where("status IN ? AND kind <> ?", activeExecutionStatuses, "validation").Count(&activeExecutions).Error
	systemPrompt = agentOrganizationContextSystemPrompt(systemPrompt, r.Manager.store.Agents(), cfg, activeExecutions)
	systemPrompt = agentGoalPreservingDecompositionSystemPrompt(systemPrompt, prepared.agent, cfg)
	systemPrompt = agentInputAttachmentsSystemPrompt(systemPrompt, attachments)
	systemPrompt = agentPermissionSystemPrompt(systemPrompt, runtimeWorkspace, prepared.agent.Permissions)
	systemPrompt = nativeCoordinationSystemPrompt(systemPrompt, prepared.issue, prepared.agent)
	coordinationID := prepared.issue.ID
	if root, rootErr := r.Manager.store.taskRoot(prepared.issue); rootErr == nil {
		coordinationID = root.ID
	}
	bridge := r.Manager.Coordination()
	if bridge == nil {
		return agenthost.ExecutionSpec{}, errors.New("control native runner: Coordination runtime is required")
	}
	decision, err := bridge.PlanAgentCapabilities(ctx, coordinationID, prepared.agent.ID, prepared.issue.Capabilities, coordination.CapabilitySelection(prepared.issue.CapabilitySelection))
	if err != nil {
		return agenthost.ExecutionSpec{}, err
	}
	options := map[string]any{}
	if thinking := strings.TrimSpace(cfg.Thinking); thinking != "" && thinking != "off" {
		options["reasoning_effort"] = thinking
	}
	return agenthost.ExecutionSpec{
		ExecutionID: prepared.execution.ID, AgentID: prepared.agent.ID, SessionID: prepared.execution.SessionID,
		Workspace: runtimeWorkspace, Model: agenthost.ModelRef{Provider: cfg.Provider, Model: cfg.Model, Options: options},
		SystemPrompt: systemPrompt, Prompt: prompt, Capabilities: decision.Capabilities,
		Values: map[string]any{"control.taskId": coordinationID, "control.taskAgentId": prepared.issue.AssigneeTaskAgentID, "control.issueId": prepared.issue.ID, "control.executionId": prepared.execution.ID, "control.runtimeId": runtimeID, "coordination.capabilityDecision": decision},
	}, nil
}

func rewriteWorkspacePaths(prompt, runtimeWorkspace string, candidates ...string) string {
	runtimeWorkspace = strings.TrimSpace(runtimeWorkspace)
	result := prompt
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == runtimeWorkspace {
			continue
		}
		result = strings.ReplaceAll(result, candidate, runtimeWorkspace)
	}
	return result
}

func nativeCoordinationSystemPrompt(systemPrompt string, issue Issue, agent AgentDefinition) string {
	return fmt.Sprintf(`%s

<aegis_task_identity>
You are one task-local Agent instance, not a global employee. Your role type is %q (agentId=%s), taskAgentId=%s, and current Issue is %s. Your conversation and Phone belong only to this Task and must never be treated as memory for another Task.

The only coordination mode is Board Autonomy:
- Use phone_board_list_issues and phone_board_get_issue to inspect authoritative Issue status, runtime state, priority, assignee name, and Agent type before coordinating work.
- Use phone_board_create_issue to create one child Issue without assigning it, or with an optional assignee. Use phone_board_update_issue to change priority, workflow status, or assignee; use phone_board_delete_issue only for permanently removing stopped work.
- Use the Phone Board shortcut phone_board_delegate to create and assign durable child Issues. Delegation does not pause you; continue any useful parent work.
- When a direct child ends as failed or budget_exceeded, you are woken immediately. Use phone_board_continue_issue only if its evidence justifies another Execution; otherwise use phone_board_delegate for a different direction or accept the partial result and continue. A new Execution resets only its own budget, never the Task wall-clock.
- A one-minute heartbeat wakes this Agent only after it releases the active loop into sleeping or waiting_children; it is never periodic input to active model work. On wake, use it to inspect, guide, stop, or reassign work when needed.
- Board comments and Phone Relay messages identify the sender and may directly steer this running loop or wake it early. A reply is optional; reply only when it advances the task.
- Use the Phone Board shortcut phone_board_sleep only when no valuable work remains until a heartbeat, comment, Relay message, or child update arrives.
- Board and Relay operations, including their shortcuts, always run through the task Phone. Never infer another task's messages, drafts, pages, or state.
</aegis_task_identity>`, strings.TrimSpace(systemPrompt), agent.Name, agent.ID, issue.AssigneeTaskAgentID, issue.Identifier)
}

func (r NativeIssueRunner) waitForIssueOutcome(ctx context.Context, issueID string, sink agentcore.EventSink) (agenthost.Result, error) {
	changes, unsubscribe := r.Manager.store.Subscribe()
	defer unsubscribe()
	for {
		issue, err := r.Manager.store.GetIssue(issueID)
		if err != nil {
			return agenthost.Result{}, err
		}
		if issueStatusTerminal(issue.Status) || issue.Status == "in_review" {
			var execution Execution
			_ = r.Manager.store.db.Where("issue_id = ?", issue.ID).Order("started_at desc").First(&execution).Error
			text := fallback(issue.Result, execution.Result)
			message := agentcore.TextMessage(agentcore.RoleAssistant, text)
			success := issue.Status == "done" || issue.Status == "in_review"
			if sink != nil {
				_ = sink(ctx, agentcore.Event{Type: agentcore.EventAgentEnd, Success: success, Error: issue.Error})
			}
			result := agenthost.Result{Core: agentcore.Result{State: agentcore.State{Messages: []agentcore.Message{message}}, NewMessages: []agentcore.Message{message}, StopReason: agentcore.StopReasonStop}}
			if !success {
				result.Core.StopReason = agentcore.StopReasonError
				return result, fmt.Errorf("control issue %s ended as %s: %s", issue.ID, issue.Status, fallback(issue.Error, execution.Error))
			}
			return result, nil
		}
		select {
		case <-ctx.Done():
			return agenthost.Result{}, ctx.Err()
		case _, ok := <-changes:
			if !ok {
				return agenthost.Result{}, errors.New("control native runner: store subscription closed")
			}
		}
	}
}

func defaultNativeCapabilities(agent AgentDefinition, cfg Config, phoneEnabled bool) []capability.Ref {
	result := make([]capability.Ref, 0, len(agent.SkillIDs)+1)
	for _, id := range uniqueStrings(agent.SkillIDs) {
		result = append(result, capability.Ref{Kind: capability.KindSkill, Name: id})
	}
	if cfg.WebSearch.Enabled {
		result = append(result, capability.Ref{Kind: capability.KindWeb, Name: "default", Optional: true})
	}
	if phoneEnabled {
		result = append(result, capability.Ref{Kind: capability.KindPhone, Name: "default"})
	}
	return result
}

func (r NativeIssueRunner) eventSink(prepared preparedIssueExecution, next agentcore.EventSink) agentcore.EventSink {
	return func(ctx context.Context, event agentcore.Event) error {
		if next != nil {
			if err := next(ctx, event); err != nil {
				return err
			}
		}
		switch event.Type {
		case agentcore.EventToolExecutionStart:
			_ = r.Manager.store.updateExecution(prepared.execution.ID, map[string]any{"status": "running", "current_tool": event.ToolName, "checkpoint": "正在调用 " + event.ToolName, "checkpoint_at": time.Now()})
			r.Manager.store.addEvent(prepared.execution.ID, prepared.issue.ID, "tool", "Go AgentCore 调用 "+event.ToolName, truncate(string(event.Arguments), 1000))
		case agentcore.EventToolExecutionEnd:
			_ = r.Manager.store.updateExecution(prepared.execution.ID, map[string]any{"current_tool": "", "checkpoint": "工具 " + event.ToolName + " 已完成", "checkpoint_at": time.Now()})
		}
		return nil
	}
}

func (r NativeIssueRunner) persistMessages(prepared preparedIssueExecution, messages []agentcore.Message) error {
	now := time.Now()
	records := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == agentcore.RoleUser || message.Role == agentcore.RoleSystem {
			continue
		}
		content := strings.TrimSpace(message.Text())
		if content == "" && message.Role == agentcore.RoleTool {
			content = fmt.Sprintf("[%s] tool result", message.ToolName)
		}
		records = append(records, Message{
			ID: nextID("message"), ExecutionID: prepared.execution.ID, IssueID: prepared.issue.ID,
			Role: string(message.Role), Content: content,
			CreatedAt: now.Add(time.Duration(len(records)) * time.Nanosecond), UpdatedAt: now.Add(time.Duration(len(records)) * time.Nanosecond),
		})
	}
	if len(records) == 0 {
		return nil
	}
	return r.Manager.store.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&records).Error; err != nil {
			return err
		}
		return tx.Model(&Execution{}).Where("id = ?", prepared.execution.ID).UpdateColumn("message_count", gorm.Expr("message_count + ?", len(records))).Error
	})
}

func lastAssistantText(messages []agentcore.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == agentcore.RoleAssistant && strings.TrimSpace(messages[index].Text()) != "" {
			return strings.TrimSpace(messages[index].Text())
		}
	}
	return ""
}

var _ agenthost.Runner = NativeIssueRunner{}
