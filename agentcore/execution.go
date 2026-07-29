package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
)

type toolOutcome struct {
	call   ToolCall
	result ToolResult
}

type preparedToolCall struct {
	call        ToolCall
	tool        Tool
	callContext ToolCallContext
	immediate   *ToolResult
}

func (a *Agent) rejectTruncatedCalls(ctx context.Context, calls []ToolCall, turn int, emit *eventEmitter) ([]toolOutcome, error) {
	outcomes := make([]toolOutcome, 0, len(calls))
	for _, call := range calls {
		if err := emit.send(toolStartEvent(turn, call)); err != nil {
			return nil, err
		}
		result := errorToolResult(fmt.Sprintf("tool call %q was not executed because model output hit its token limit and arguments may be truncated", call.Name))
		if err := emit.send(toolEndEvent(turn, call, result)); err != nil {
			return nil, err
		}
		outcomes = append(outcomes, toolOutcome{call: call, result: result})
	}
	return outcomes, ctx.Err()
}

func (a *Agent) executeToolCalls(ctx context.Context, state *State, calls []ToolCall, turn int, emit *eventEmitter) ([]toolOutcome, error) {
	sequential := a.config.ToolExecution == ToolExecutionSequential
	if !sequential {
		for _, call := range calls {
			if tool := a.tools[call.Name]; tool != nil {
				if provider, ok := tool.(ToolExecutionModeProvider); ok && provider.ExecutionMode() == ToolExecutionSequential {
					sequential = true
					break
				}
			}
		}
	}
	if sequential {
		return a.executeSequential(ctx, state, calls, turn, emit)
	}
	return a.executeParallel(ctx, state, calls, turn, emit)
}

func (a *Agent) executeSequential(ctx context.Context, state *State, calls []ToolCall, turn int, emit *eventEmitter) ([]toolOutcome, error) {
	outcomes := make([]toolOutcome, 0, len(calls))
	for _, call := range calls {
		if err := emit.send(toolStartEvent(turn, call)); err != nil {
			return outcomes, err
		}
		prepared := a.prepareToolCall(ctx, state, call, turn)
		outcome := a.executePrepared(ctx, prepared, turn, emit)
		outcome = a.finalizeToolCall(ctx, prepared, outcome)
		if err := emit.send(toolEndEvent(turn, outcome.call, outcome.result)); err != nil {
			return outcomes, err
		}
		outcomes = append(outcomes, outcome)
		if err := ctx.Err(); err != nil {
			return outcomes, err
		}
	}
	return outcomes, nil
}

func (a *Agent) executeParallel(ctx context.Context, state *State, calls []ToolCall, turn int, emit *eventEmitter) ([]toolOutcome, error) {
	type indexedOutcome struct {
		index    int
		prepared preparedToolCall
		outcome  toolOutcome
	}
	toolCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	completed := make(chan indexedOutcome, len(calls))
	outcomes := make([]toolOutcome, len(calls))
	pending := make([]indexedOutcome, 0, len(calls))
	for index, call := range calls {
		if err := emit.send(toolStartEvent(turn, call)); err != nil {
			return nil, err
		}
		prepared := a.prepareToolCall(toolCtx, state, call, turn)
		if prepared.immediate != nil {
			outcome := toolOutcome{call: prepared.call, result: cloneToolResult(*prepared.immediate)}
			outcome = a.finalizeToolCall(toolCtx, prepared, outcome)
			outcomes[index] = outcome
			if err := emit.send(toolEndEvent(turn, outcome.call, outcome.result)); err != nil {
				return nil, err
			}
			continue
		}
		pending = append(pending, indexedOutcome{index: index, prepared: prepared})
	}
	for _, entry := range pending {
		go func() {
			completed <- indexedOutcome{
				index:    entry.index,
				prepared: entry.prepared,
				outcome:  a.executePrepared(toolCtx, entry.prepared, turn, emit),
			}
		}()
	}
	for range len(pending) {
		completedOutcome := <-completed
		outcome := a.finalizeToolCall(toolCtx, completedOutcome.prepared, completedOutcome.outcome)
		outcomes[completedOutcome.index] = outcome
		if err := emit.send(toolEndEvent(turn, outcome.call, outcome.result)); err != nil {
			cancel()
			return outcomes, err
		}
	}
	if err := ctx.Err(); err != nil {
		return outcomes, err
	}
	return outcomes, nil
}

func (a *Agent) prepareToolCall(ctx context.Context, state *State, call ToolCall, turn int) preparedToolCall {
	call = cloneToolCall(call)
	prepared := preparedToolCall{call: call}
	tool := a.tools[call.Name]
	if tool == nil {
		result := errorToolResult(fmt.Sprintf("tool %q not found", call.Name))
		prepared.immediate = &result
		return prepared
	}
	prepared.tool = tool
	if len(call.Arguments) == 0 {
		call.Arguments = json.RawMessage("{}")
	}
	if !json.Valid(call.Arguments) {
		result := errorToolResult(fmt.Sprintf("invalid JSON arguments for tool %q", call.Name))
		prepared.immediate = &result
		return prepared
	}
	if validator, ok := tool.(ToolValidator); ok {
		if err := validator.Validate(call.Arguments); err != nil {
			result := errorToolResult(fmt.Sprintf("invalid arguments for tool %q: %v", call.Name, err))
			prepared.immediate = &result
			return prepared
		}
	}
	callContext := ToolCallContext{Turn: turn, Call: cloneToolCall(call), State: state}
	if a.config.Hooks.BeforeToolCall != nil {
		decision, err := a.config.Hooks.BeforeToolCall(ctx, callContext)
		if err != nil {
			result := errorToolResult(fmt.Sprintf("before tool call %q: %v", call.Name, err))
			prepared.immediate = &result
			return prepared
		}
		if decision.Block {
			reason := decision.Reason
			if reason == "" {
				reason = "blocked by hook"
			}
			result := errorToolResult(reason)
			prepared.immediate = &result
			return prepared
		}
		if len(decision.Arguments) > 0 {
			if !json.Valid(decision.Arguments) {
				result := errorToolResult(fmt.Sprintf("hook produced invalid JSON arguments for tool %q", call.Name))
				prepared.immediate = &result
				return prepared
			}
			call.Arguments = append(json.RawMessage(nil), decision.Arguments...)
			callContext.Call = cloneToolCall(call)
		}
	}
	prepared.call = call
	prepared.callContext = callContext
	return prepared
}

func (a *Agent) executePrepared(ctx context.Context, prepared preparedToolCall, turn int, emit *eventEmitter) toolOutcome {
	call := prepared.call
	if prepared.immediate != nil {
		return toolOutcome{call: call, result: cloneToolResult(*prepared.immediate)}
	}
	update := func(partial ToolResult) error {
		partialCopy := cloneToolResult(partial)
		return emit.send(Event{
			Type:       EventToolExecutionUpdate,
			Turn:       turn,
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Arguments:  append(json.RawMessage(nil), call.Arguments...),
			ToolResult: &partialCopy,
			IsError:    partial.IsError,
		})
	}
	result, err := prepared.tool.Execute(ctx, append(json.RawMessage(nil), call.Arguments...), update)
	if err != nil {
		result.IsError = true
		if result.Text() == "" {
			result.Content = []ContentBlock{{Type: ContentText, Text: err.Error()}}
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		result.IsError = true
		if result.Text() == "" {
			result.Content = []ContentBlock{{Type: ContentText, Text: ctxErr.Error()}}
		}
	}
	return toolOutcome{call: call, result: cloneToolResult(result)}
}

func (a *Agent) finalizeToolCall(ctx context.Context, prepared preparedToolCall, outcome toolOutcome) toolOutcome {
	if prepared.immediate == nil && a.config.Hooks.AfterToolCall != nil {
		if hookErr := a.config.Hooks.AfterToolCall(ctx, prepared.callContext, &outcome.result); hookErr != nil {
			outcome.result = errorToolResult(fmt.Sprintf("after tool call %q: %v", outcome.call.Name, hookErr))
		}
	}
	return outcome
}

func toolStartEvent(turn int, call ToolCall) Event {
	return Event{
		Type:       EventToolExecutionStart,
		Turn:       turn,
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Arguments:  append(json.RawMessage(nil), call.Arguments...),
	}
}

func toolEndEvent(turn int, call ToolCall, result ToolResult) Event {
	resultCopy := cloneToolResult(result)
	return Event{
		Type:       EventToolExecutionEnd,
		Turn:       turn,
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Arguments:  append(json.RawMessage(nil), call.Arguments...),
		ToolResult: &resultCopy,
		IsError:    result.IsError,
	}
}

func toolResultMessage(call ToolCall, result ToolResult) Message {
	content := cloneContent(result.Content)
	if len(content) == 0 {
		content = []ContentBlock{{Type: ContentText, Text: ""}}
	}
	return Message{
		ID:         nextMessageID("tool"),
		Role:       RoleTool,
		Content:    content,
		ToolCallID: call.ID,
		ToolName:   call.Name,
		IsError:    result.IsError,
	}
}

func outcomesTerminate(outcomes []toolOutcome) bool {
	if len(outcomes) == 0 {
		return false
	}
	for _, outcome := range outcomes {
		if !outcome.result.Terminate {
			return false
		}
	}
	return true
}

func errorToolResult(message string) ToolResult {
	return ToolResult{Content: []ContentBlock{{Type: ContentText, Text: message}}, IsError: true}
}

func cloneToolResult(result ToolResult) ToolResult {
	result.Content = cloneContent(result.Content)
	return result
}

func cloneContent(content []ContentBlock) []ContentBlock {
	result := make([]ContentBlock, len(content))
	for i, block := range content {
		result[i] = block
		if block.ToolCall != nil {
			call := cloneToolCall(*block.ToolCall)
			result[i].ToolCall = &call
		}
	}
	return result
}
