package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

const defaultMaxTurns = 64

var messageSequence atomic.Uint64

// Agent owns immutable loop configuration and may be reused concurrently.
type Agent struct {
	config Config
	tools  map[string]Tool
}

// New validates config and creates an Agent.
func New(config Config) (*Agent, error) {
	if config.Model == nil {
		return nil, ErrModelRequired
	}
	if config.MaxTurns <= 0 {
		config.MaxTurns = defaultMaxTurns
	}
	if config.ToolExecution == "" {
		config.ToolExecution = ToolExecutionParallel
	}
	if config.ToolExecution != ToolExecutionParallel && config.ToolExecution != ToolExecutionSequential {
		return nil, fmt.Errorf("agentcore: unsupported tool execution mode %q", config.ToolExecution)
	}

	tools := make(map[string]Tool, len(config.Tools))
	for _, tool := range config.Tools {
		if tool == nil {
			return nil, errors.New("agentcore: nil tool")
		}
		definition := tool.Definition()
		if definition.Name == "" {
			return nil, errors.New("agentcore: tool name is required")
		}
		if _, exists := tools[definition.Name]; exists {
			return nil, fmt.Errorf("agentcore: duplicate tool %q", definition.Name)
		}
		tools[definition.Name] = tool
	}

	config.Tools = append([]Tool(nil), config.Tools...)
	return &Agent{config: config, tools: tools}, nil
}

// Prompt appends prompts to state and runs model/tool turns until the model
// stops, a tool terminates the loop, the limit is reached, or ctx is canceled.
// Events are delivered synchronously through sink. A nil sink discards events.
func (a *Agent) Prompt(ctx context.Context, state State, prompts []Message, sink EventSink) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	emit := newEmitter(ctx, sink)
	current := State{Messages: cloneMessages(state.Messages)}
	newMessages := make([]Message, 0, len(prompts)+4)

	if err := emit.send(Event{Type: EventAgentStart}); err != nil {
		return resultFrom(current, newMessages, 0, StopReasonError), err
	}
	if err := emit.send(Event{Type: EventTurnStart, Turn: 1}); err != nil {
		return resultFrom(current, newMessages, 0, StopReasonError), err
	}
	for _, prompt := range prompts {
		prompt = cloneMessage(prompt)
		ensureMessageID(&prompt)
		current.Messages = append(current.Messages, prompt)
		newMessages = append(newMessages, prompt)
		copy := cloneMessage(prompt)
		if err := emit.send(Event{Type: EventMessageStart, Turn: 1, Message: &copy}); err != nil {
			return resultFrom(current, newMessages, 0, StopReasonError), err
		}
		if err := emit.send(Event{Type: EventMessageEnd, Turn: 1, Message: &copy}); err != nil {
			return resultFrom(current, newMessages, 0, StopReasonError), err
		}
	}

	return a.run(ctx, current, newMessages, emit, true)
}

// Continue resumes a state without adding a prompt. The final existing message
// must not be an assistant message because providers require user or tool input
// before another assistant response.
func (a *Agent) Continue(ctx context.Context, state State, sink EventSink) (Result, error) {
	if len(state.Messages) == 0 {
		return Result{}, errors.New("agentcore: cannot continue empty state")
	}
	if state.Messages[len(state.Messages)-1].Role == RoleAssistant {
		return Result{}, errors.New("agentcore: cannot continue from assistant message")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	emit := newEmitter(ctx, sink)
	current := State{Messages: cloneMessages(state.Messages)}
	if err := emit.send(Event{Type: EventAgentStart}); err != nil {
		return resultFrom(current, nil, 0, StopReasonError), err
	}
	if err := emit.send(Event{Type: EventTurnStart, Turn: 1}); err != nil {
		return resultFrom(current, nil, 0, StopReasonError), err
	}
	return a.run(ctx, current, nil, emit, true)
}

func (a *Agent) run(ctx context.Context, state State, newMessages []Message, emit *eventEmitter, turnAlreadyStarted bool) (Result, error) {
	for turn := 1; turn <= a.config.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return a.finish(state, newMessages, turn-1, StopReasonAborted, err, emit)
		}
		if !turnAlreadyStarted {
			if err := emit.send(Event{Type: EventTurnStart, Turn: turn}); err != nil {
				return resultFrom(state, newMessages, turn-1, StopReasonError), err
			}
		}
		turnAlreadyStarted = false

		assistant, err := a.streamAssistant(ctx, state, turn, emit)
		state.Messages = append(state.Messages, assistant)
		newMessages = append(newMessages, assistant)
		if err != nil {
			stop := StopReasonError
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				stop = StopReasonAborted
			}
			return a.finish(state, newMessages, turn, stop, err, emit)
		}

		calls := assistant.ToolCalls()
		toolMessages := make([]Message, 0, len(calls))
		terminate := false
		if len(calls) > 0 {
			var outcomes []toolOutcome
			if assistant.StopReason == StopReasonLength {
				outcomes, err = a.rejectTruncatedCalls(ctx, calls, turn, emit)
			} else {
				outcomes, err = a.executeToolCalls(ctx, &state, calls, turn, emit)
			}
			if err != nil {
				stop := StopReasonError
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					stop = StopReasonAborted
				}
				return a.finish(state, newMessages, turn, stop, err, emit)
			}
			terminate = outcomesTerminate(outcomes)
			for _, outcome := range outcomes {
				message := toolResultMessage(outcome.call, outcome.result)
				state.Messages = append(state.Messages, message)
				newMessages = append(newMessages, message)
				toolMessages = append(toolMessages, message)
				copy := cloneMessage(message)
				if err := emit.send(Event{Type: EventMessageStart, Turn: turn, Message: &copy}); err != nil {
					return resultFrom(state, newMessages, turn, StopReasonError), err
				}
				if err := emit.send(Event{Type: EventMessageEnd, Turn: turn, Message: &copy}); err != nil {
					return resultFrom(state, newMessages, turn, StopReasonError), err
				}
			}
		}

		turnContext := TurnContext{
			Turn:        turn,
			Message:     cloneMessage(assistant),
			ToolResults: cloneMessages(toolMessages),
			State:       &state,
		}
		if err := emit.send(Event{Type: EventTurnEnd, Turn: turn, Message: &turnContext.Message, ToolResults: cloneMessages(toolMessages)}); err != nil {
			return resultFrom(state, newMessages, turn, StopReasonError), err
		}
		if a.config.Hooks.PrepareNextTurn != nil {
			if err := a.config.Hooks.PrepareNextTurn(ctx, &turnContext); err != nil {
				return a.finish(state, newMessages, turn, StopReasonError, err, emit)
			}
		}
		if a.config.Hooks.ShouldStop != nil && a.config.Hooks.ShouldStop(ctx, turnContext) {
			return a.finish(state, newMessages, turn, StopReasonStop, nil, emit)
		}
		if len(calls) == 0 {
			stop := assistant.StopReason
			if stop == "" {
				stop = StopReasonStop
			}
			return a.finish(state, newMessages, turn, stop, nil, emit)
		}
		if terminate {
			return a.finish(state, newMessages, turn, StopReasonTerminated, nil, emit)
		}
	}

	return a.finish(state, newMessages, a.config.MaxTurns, StopReasonMaxTurns, ErrMaxTurns, emit)
}

func (a *Agent) streamAssistant(ctx context.Context, state State, turn int, emit *eventEmitter) (Message, error) {
	assistant := Message{ID: nextMessageID("assistant"), Role: RoleAssistant}
	fail := func(runErr error, started bool) (Message, error) {
		assistant.StopReason = StopReasonError
		assistant.Error = runErr.Error()
		assistant.IsError = true
		copy := cloneMessage(assistant)
		if !started {
			_ = emit.send(Event{Type: EventMessageStart, Turn: turn, Message: &copy})
		}
		_ = emit.send(Event{Type: EventMessageEnd, Turn: turn, Message: &copy, IsError: true, Error: runErr.Error()})
		return assistant, runErr
	}
	messages := cloneMessages(state.Messages)
	if a.config.TransformContext != nil {
		var err error
		messages, err = a.config.TransformContext(ctx, messages)
		if err != nil {
			return fail(fmt.Errorf("transform context: %w", err), false)
		}
	}
	request := ModelRequest{
		SystemPrompt: a.config.SystemPrompt,
		Messages:     messages,
		Tools:        make([]ToolDefinition, 0, len(a.config.Tools)),
	}
	for _, tool := range a.config.Tools {
		request.Tools = append(request.Tools, cloneToolDefinition(tool.Definition()))
	}
	if a.config.Hooks.BeforeModelCall != nil {
		if err := a.config.Hooks.BeforeModelCall(ctx, &request); err != nil {
			return fail(fmt.Errorf("before model call: %w", err), false)
		}
	}

	stream, err := a.config.Model.Stream(ctx, request)
	if err != nil {
		return fail(fmt.Errorf("start model stream: %w", err), false)
	}
	if stream == nil {
		return fail(errors.New("start model stream: model returned a nil stream"), false)
	}
	defer stream.Close()

	startCopy := cloneMessage(assistant)
	if err := emit.send(Event{Type: EventMessageStart, Turn: turn, Message: &startCopy}); err != nil {
		return assistant, err
	}
	accumulator := responseAccumulator{message: &assistant, toolBlockIndexes: map[int]int{}}
	for {
		chunk, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return fail(fmt.Errorf("receive model stream: %w", recvErr), true)
		}
		accumulator.add(chunk)
		updateCopy := cloneMessage(assistant)
		chunkCopy := cloneModelChunk(chunk)
		if err := emit.send(Event{Type: EventMessageUpdate, Turn: turn, Message: &updateCopy, Delta: &chunkCopy}); err != nil {
			return assistant, err
		}
	}
	if assistant.StopReason == "" {
		if len(assistant.ToolCalls()) > 0 {
			assistant.StopReason = StopReasonToolUse
		} else {
			assistant.StopReason = StopReasonStop
		}
	}
	if a.config.Hooks.AfterModelCall != nil {
		if err := a.config.Hooks.AfterModelCall(ctx, &assistant); err != nil {
			return fail(fmt.Errorf("after model call: %w", err), true)
		}
	}
	endCopy := cloneMessage(assistant)
	if err := emit.send(Event{Type: EventMessageEnd, Turn: turn, Message: &endCopy}); err != nil {
		return assistant, err
	}
	return assistant, nil
}

func (a *Agent) finish(state State, newMessages []Message, turns int, reason StopReason, runErr error, emit *eventEmitter) (Result, error) {
	result := resultFrom(state, newMessages, turns, reason)
	event := Event{Type: EventAgentEnd, Turn: turns, Messages: cloneMessages(newMessages)}
	if runErr != nil {
		event.IsError = true
		event.Error = runErr.Error()
	}
	if err := emit.send(event); err != nil && runErr == nil {
		return result, err
	}
	return result, runErr
}

type responseAccumulator struct {
	message          *Message
	toolBlockIndexes map[int]int
}

func (a *responseAccumulator) add(chunk ModelChunk) {
	if chunk.TextDelta != "" {
		a.appendText(ContentText, chunk.TextDelta)
	}
	if chunk.ThinkingDelta != "" {
		a.appendText(ContentThinking, chunk.ThinkingDelta)
	}
	for _, delta := range chunk.ToolCallDeltas {
		blockIndex, exists := a.toolBlockIndexes[delta.Index]
		if !exists {
			call := ToolCall{ID: delta.ID, Name: delta.Name}
			a.message.Content = append(a.message.Content, ContentBlock{Type: ContentToolCall, ToolCall: &call})
			blockIndex = len(a.message.Content) - 1
			a.toolBlockIndexes[delta.Index] = blockIndex
		}
		call := a.message.Content[blockIndex].ToolCall
		if delta.ID != "" {
			call.ID = delta.ID
		}
		if delta.Name != "" {
			call.Name = delta.Name
		}
		call.Arguments = append(call.Arguments, delta.ArgumentsDelta...)
	}
	if chunk.StopReason != "" {
		a.message.StopReason = chunk.StopReason
	}
	if chunk.Usage != nil {
		a.message.Usage = *chunk.Usage
	}
	if chunk.ProviderData != nil {
		a.message.ProviderData = chunk.ProviderData
	}
}

func (a *responseAccumulator) appendText(kind ContentType, delta string) {
	if len(a.message.Content) > 0 && a.message.Content[len(a.message.Content)-1].Type == kind {
		a.message.Content[len(a.message.Content)-1].Text += delta
		return
	}
	a.message.Content = append(a.message.Content, ContentBlock{Type: kind, Text: delta})
}

type eventEmitter struct {
	ctx  context.Context
	sink EventSink
	mu   sync.Mutex
	err  error
}

func newEmitter(ctx context.Context, sink EventSink) *eventEmitter {
	if sink == nil {
		sink = func(context.Context, Event) error { return nil }
	}
	return &eventEmitter{ctx: ctx, sink: sink}
}

func (e *eventEmitter) send(event Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return e.err
	}
	if err := e.sink(e.ctx, event); err != nil {
		e.err = err
		return err
	}
	return nil
}

func resultFrom(state State, newMessages []Message, turns int, reason StopReason) Result {
	return Result{
		State:       State{Messages: cloneMessages(state.Messages)},
		NewMessages: cloneMessages(newMessages),
		Turns:       turns,
		StopReason:  reason,
	}
}

func ensureMessageID(message *Message) {
	if message.ID == "" {
		message.ID = nextMessageID(string(message.Role))
	}
}

func nextMessageID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, messageSequence.Add(1))
}

func cloneMessages(messages []Message) []Message {
	result := make([]Message, len(messages))
	for i := range messages {
		result[i] = cloneMessage(messages[i])
	}
	return result
}

func cloneMessage(message Message) Message {
	content := message.Content
	message.Content = make([]ContentBlock, len(content))
	for i, block := range content {
		message.Content[i] = block
		if block.ToolCall != nil {
			call := cloneToolCall(*block.ToolCall)
			message.Content[i].ToolCall = &call
		}
	}
	return message
}

func cloneToolDefinition(definition ToolDefinition) ToolDefinition {
	definition.Parameters = append(json.RawMessage(nil), definition.Parameters...)
	return definition
}

func cloneModelChunk(chunk ModelChunk) ModelChunk {
	chunk.ToolCallDeltas = append([]ToolCallDelta(nil), chunk.ToolCallDeltas...)
	if chunk.Usage != nil {
		usage := *chunk.Usage
		chunk.Usage = &usage
	}
	return chunk
}
