package agenthost

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"aegis/observability"
	"github.com/z3r2ne/agentcore"
)

func observabilityHooks(next agentcore.Hooks) agentcore.Hooks {
	var mu sync.Mutex
	toolStarted := map[string]time.Time{}
	result := next
	result.BeforeModelCall = func(ctx context.Context, request *agentcore.ModelRequest) error {
		if next.BeforeModelCall != nil {
			if err := next.BeforeModelCall(ctx, request); err != nil {
				observability.Default().Error(ctx, "agent.model.before.failed", slog.String("error", err.Error()))
				return err
			}
		}
		observability.Default().Debug(ctx, "agent.model.request", slog.Int("message_count", len(request.Messages)), slog.Int("tool_count", len(request.Tools)))
		observability.DefaultMetrics().AddCounter("agent_model_calls_total", 1, nil)
		return nil
	}
	result.AfterModelCall = func(ctx context.Context, message *agentcore.Message) error {
		var hookErr error
		if next.AfterModelCall != nil {
			hookErr = next.AfterModelCall(ctx, message)
		}
		attrs := []slog.Attr{slog.String("stop_reason", string(message.StopReason)), slog.Int("text_bytes", len(message.Text())), slog.Int("tool_call_count", len(message.ToolCalls()))}
		if hookErr != nil {
			attrs = append(attrs, slog.String("error", hookErr.Error()))
			observability.Default().Error(ctx, "agent.model.response", attrs...)
		} else {
			observability.Default().Info(ctx, "agent.model.response", attrs...)
		}
		return hookErr
	}
	result.BeforeToolCall = func(ctx context.Context, call agentcore.ToolCallContext) (agentcore.ToolCallDecision, error) {
		decision := agentcore.ToolCallDecision{}
		var err error
		if next.BeforeToolCall != nil {
			decision, err = next.BeforeToolCall(ctx, call)
		}
		if err != nil {
			observability.Default().Error(ctx, "agent.tool.before.failed", slog.String("tool_name", call.Call.Name), slog.String("tool_call_id", call.Call.ID), slog.Int("turn", call.Turn), slog.String("error", err.Error()))
			return decision, err
		}
		if decision.Block {
			observability.Default().Warn(ctx, "agent.tool.blocked", slog.String("tool_name", call.Call.Name), slog.String("tool_call_id", call.Call.ID), slog.Int("turn", call.Turn), slog.String("reason", decision.Reason))
			observability.DefaultMetrics().AddCounter("agent_tool_calls_total", 1, observability.Labels{"tool": call.Call.Name, "status": "blocked"})
			return decision, nil
		}
		mu.Lock()
		toolStarted[call.Call.ID] = time.Now()
		mu.Unlock()
		observability.Default().Info(ctx, "agent.tool.start", slog.String("tool_name", call.Call.Name), slog.String("tool_call_id", call.Call.ID), slog.Int("turn", call.Turn))
		return decision, nil
	}
	result.AfterToolCall = func(ctx context.Context, call agentcore.ToolCallContext, toolResult *agentcore.ToolResult) error {
		var hookErr error
		if next.AfterToolCall != nil {
			hookErr = next.AfterToolCall(ctx, call, toolResult)
		}
		mu.Lock()
		started, exists := toolStarted[call.Call.ID]
		delete(toolStarted, call.Call.ID)
		mu.Unlock()
		duration := time.Duration(0)
		if exists {
			duration = time.Since(started)
		}
		status := "ok"
		if toolResult != nil && toolResult.IsError {
			status = "tool_error"
		}
		if hookErr != nil {
			status = "hook_error"
		}
		attrs := []slog.Attr{slog.String("tool_name", call.Call.Name), slog.String("tool_call_id", call.Call.ID), slog.Int("turn", call.Turn), slog.String("status", status), slog.Float64("duration_ms", float64(duration.Microseconds())/1000)}
		if hookErr != nil {
			attrs = append(attrs, slog.String("error", hookErr.Error()))
			observability.Default().Error(ctx, "agent.tool.end", attrs...)
		} else if status != "ok" {
			observability.Default().Warn(ctx, "agent.tool.end", attrs...)
		} else {
			observability.Default().Info(ctx, "agent.tool.end", attrs...)
		}
		labels := observability.Labels{"tool": call.Call.Name, "status": status}
		observability.DefaultMetrics().AddCounter("agent_tool_calls_total", 1, labels)
		observability.DefaultMetrics().ObserveHistogram("agent_tool_duration_ms", float64(duration.Microseconds())/1000, labels)
		return hookErr
	}
	return result
}
