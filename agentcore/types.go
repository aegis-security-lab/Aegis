package agentcore

import (
	"context"
	"encoding/json"
)

// Role identifies the author of a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentType identifies a block within a message.
type ContentType string

const (
	ContentText     ContentType = "text"
	ContentThinking ContentType = "thinking"
	ContentToolCall ContentType = "tool_call"
)

// StopReason describes why model generation or the agent run stopped.
type StopReason string

const (
	StopReasonStop       StopReason = "stop"
	StopReasonToolUse    StopReason = "tool_use"
	StopReasonLength     StopReason = "length"
	StopReasonError      StopReason = "error"
	StopReasonAborted    StopReason = "aborted"
	StopReasonMaxTurns   StopReason = "max_turns"
	StopReasonTerminated StopReason = "tool_terminated"
)

// Usage contains provider-reported token accounting for one model response.
type Usage struct {
	InputTokens      int `json:"inputTokens,omitempty"`
	OutputTokens     int `json:"outputTokens,omitempty"`
	CacheReadTokens  int `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int `json:"cacheWriteTokens,omitempty"`
}

// ToolCall is a model-requested tool invocation. Arguments must contain a JSON
// object or another JSON value accepted by the tool.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ContentBlock is one text, thinking, or tool-call block.
type ContentBlock struct {
	Type     ContentType `json:"type"`
	Text     string      `json:"text,omitempty"`
	ToolCall *ToolCall   `json:"toolCall,omitempty"`
}

// Message is the provider-neutral message format retained by the loop.
type Message struct {
	ID           string         `json:"id,omitempty"`
	Role         Role           `json:"role"`
	Content      []ContentBlock `json:"content,omitempty"`
	ToolCallID   string         `json:"toolCallId,omitempty"`
	ToolName     string         `json:"toolName,omitempty"`
	StopReason   StopReason     `json:"stopReason,omitempty"`
	Usage        Usage          `json:"usage,omitempty"`
	Error        string         `json:"error,omitempty"`
	IsError      bool           `json:"isError,omitempty"`
	ProviderData any            `json:"-"`
}

// TextMessage constructs a single-block text message.
func TextMessage(role Role, text string) Message {
	return Message{Role: role, Content: []ContentBlock{{Type: ContentText, Text: text}}}
}

// Text concatenates all text blocks in a message.
func (m Message) Text() string {
	var text string
	for _, block := range m.Content {
		if block.Type == ContentText {
			text += block.Text
		}
	}
	return text
}

// ToolCalls returns tool calls in their original model order.
func (m Message) ToolCalls() []ToolCall {
	calls := make([]ToolCall, 0)
	for _, block := range m.Content {
		if block.Type == ContentToolCall && block.ToolCall != nil {
			calls = append(calls, cloneToolCall(*block.ToolCall))
		}
	}
	return calls
}

// ToolDefinition is the schema advertised to a model.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ModelRequest contains one complete inference request.
type ModelRequest struct {
	SystemPrompt string           `json:"systemPrompt,omitempty"`
	Messages     []Message        `json:"messages"`
	Tools        []ToolDefinition `json:"tools,omitempty"`
}

// ToolCallDelta incrementally builds one tool call. Index is stable within a
// single assistant response.
type ToolCallDelta struct {
	Index          int    `json:"index"`
	ID             string `json:"id,omitempty"`
	Name           string `json:"name,omitempty"`
	ArgumentsDelta string `json:"argumentsDelta,omitempty"`
}

// ModelChunk is one normalized streaming update from a model adapter.
type ModelChunk struct {
	TextDelta      string          `json:"textDelta,omitempty"`
	ThinkingDelta  string          `json:"thinkingDelta,omitempty"`
	ToolCallDeltas []ToolCallDelta `json:"toolCallDeltas,omitempty"`
	StopReason     StopReason      `json:"stopReason,omitempty"`
	Usage          *Usage          `json:"usage,omitempty"`
	ProviderData   any             `json:"-"`
}

// ModelStream yields chunks until io.EOF and must be closed by the loop.
type ModelStream interface {
	Recv() (ModelChunk, error)
	Close() error
}

// Model starts a streaming model request.
type Model interface {
	Stream(context.Context, ModelRequest) (ModelStream, error)
}

// ToolExecutionMode controls execution of a batch of tool calls.
type ToolExecutionMode string

const (
	ToolExecutionParallel   ToolExecutionMode = "parallel"
	ToolExecutionSequential ToolExecutionMode = "sequential"
)

// ToolResult is sent back to the model after a tool invocation.
type ToolResult struct {
	Content   []ContentBlock `json:"content,omitempty"`
	Details   any            `json:"details,omitempty"`
	IsError   bool           `json:"isError,omitempty"`
	Terminate bool           `json:"terminate,omitempty"`
}

// TextToolResult constructs a plain-text tool result.
func TextToolResult(text string) ToolResult {
	return ToolResult{Content: []ContentBlock{{Type: ContentText, Text: text}}}
}

// Text concatenates all text blocks in a tool result.
func (r ToolResult) Text() string {
	var text string
	for _, block := range r.Content {
		if block.Type == ContentText {
			text += block.Text
		}
	}
	return text
}

// ToolUpdateSink reports partial tool output.
type ToolUpdateSink func(ToolResult) error

// Tool is executable model-facing functionality.
type Tool interface {
	Definition() ToolDefinition
	Execute(context.Context, json.RawMessage, ToolUpdateSink) (ToolResult, error)
}

// ToolValidator optionally validates arguments before a tool starts.
type ToolValidator interface {
	Validate(json.RawMessage) error
}

// ToolExecutionModeProvider can force all calls in a batch to run sequentially.
type ToolExecutionModeProvider interface {
	ExecutionMode() ToolExecutionMode
}

// State is the reusable in-memory state of an agent session.
type State struct {
	Messages []Message `json:"messages"`
}

// Result is returned after a complete or interrupted run.
type Result struct {
	State       State      `json:"state"`
	NewMessages []Message  `json:"newMessages"`
	Turns       int        `json:"turns"`
	StopReason  StopReason `json:"stopReason"`
}

// TurnContext is passed to hooks after a model turn and its tools complete.
type TurnContext struct {
	Turn        int
	Message     Message
	ToolResults []Message
	State       *State
}

// ToolCallContext describes a tool invocation to a hook.
type ToolCallContext struct {
	Turn  int
	Call  ToolCall
	State *State
}

// ToolCallDecision can block a call or replace its arguments.
type ToolCallDecision struct {
	Block     bool
	Reason    string
	Arguments json.RawMessage
}

// Hooks customize lifecycle behavior without replacing the loop.
type Hooks struct {
	BeforeModelCall func(context.Context, *ModelRequest) error
	AfterModelCall  func(context.Context, *Message) error
	BeforeToolCall  func(context.Context, ToolCallContext) (ToolCallDecision, error)
	AfterToolCall   func(context.Context, ToolCallContext, *ToolResult) error
	PrepareNextTurn func(context.Context, *TurnContext) error
	ShouldStop      func(context.Context, TurnContext) bool
}

// Config controls one Agent instance.
type Config struct {
	Model            Model
	SystemPrompt     string
	Tools            []Tool
	MaxTurns         int
	ToolExecution    ToolExecutionMode
	TransformContext func(context.Context, []Message) ([]Message, error)
	Hooks            Hooks
}

// EventType identifies an observable agent lifecycle event.
type EventType string

const (
	EventAgentStart          EventType = "agent_start"
	EventAgentEnd            EventType = "agent_end"
	EventTurnStart           EventType = "turn_start"
	EventTurnEnd             EventType = "turn_end"
	EventMessageStart        EventType = "message_start"
	EventMessageUpdate       EventType = "message_update"
	EventMessageEnd          EventType = "message_end"
	EventToolExecutionStart  EventType = "tool_execution_start"
	EventToolExecutionUpdate EventType = "tool_execution_update"
	EventToolExecutionEnd    EventType = "tool_execution_end"
)

// Event mirrors the useful portion of Pi's event stream while remaining a
// typed Go value suitable for callbacks, SSE, WebSockets, or JSONL RPC.
type Event struct {
	Type        EventType       `json:"type"`
	Turn        int             `json:"turn,omitempty"`
	Message     *Message        `json:"message,omitempty"`
	Delta       *ModelChunk     `json:"delta,omitempty"`
	ToolCallID  string          `json:"toolCallId,omitempty"`
	ToolName    string          `json:"toolName,omitempty"`
	Arguments   json.RawMessage `json:"args,omitempty"`
	ToolResult  *ToolResult     `json:"result,omitempty"`
	IsError     bool            `json:"isError,omitempty"`
	Error       string          `json:"error,omitempty"`
	Messages    []Message       `json:"messages,omitempty"`
	ToolResults []Message       `json:"toolResults,omitempty"`
}

// EventSink receives events synchronously and in deterministic order.
type EventSink func(context.Context, Event) error

func cloneToolCall(call ToolCall) ToolCall {
	call.Arguments = append(json.RawMessage(nil), call.Arguments...)
	return call
}
