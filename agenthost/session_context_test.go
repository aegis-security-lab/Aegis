package agenthost

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

type authorizationTestModel struct {
	mu    sync.Mutex
	calls int
}

type authorizationTestResolver struct {
	model agentcore.Model
}

func (r authorizationTestResolver) ResolveModel(context.Context, ModelRef) (agentcore.Model, error) {
	return r.model, nil
}

func (m *authorizationTestModel) Stream(_ context.Context, _ agentcore.ModelRequest) (agentcore.ModelStream, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()
	if call == 1 {
		return &authorizationTestStream{chunks: []agentcore.ModelChunk{{
			ToolCallDeltas: []agentcore.ToolCallDelta{{Index: 0, ID: "call-1", Name: "workspace_read", ArgumentsDelta: `{}`}},
			StopReason:     agentcore.StopReasonToolUse,
		}}}, nil
	}
	return &authorizationTestStream{chunks: []agentcore.ModelChunk{{TextDelta: "done", StopReason: agentcore.StopReasonStop}}}, nil
}

type authorizationTestStream struct {
	chunks []agentcore.ModelChunk
}

func (s *authorizationTestStream) Recv() (agentcore.ModelChunk, error) {
	if len(s.chunks) == 0 {
		return agentcore.ModelChunk{}, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (*authorizationTestStream) Close() error { return nil }

func TestSessionRunsPreserveMaterializedToolAuthorization(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *agentcore.SessionSnapshot
		stream   bool
	}{
		{name: "new session prompt"},
		{name: "restored session stream", snapshot: &agentcore.SessionSnapshot{
			SteeringMode: agentcore.DeliveryOne,
			FollowUpMode: agentcore.DeliveryOne,
		}, stream: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &authorizationTestModel{}
			registry := capability.NewRegistry()
			var executed atomic.Bool
			err := registry.Register(capability.KindTool, "workspace", capability.SourceFunc(func(context.Context, capability.ResolveContext, capability.Ref) (capability.Resolved, error) {
				tool := agentcore.FuncTool{
					ToolDefinition: agentcore.ToolDefinition{Name: "workspace_read", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
					ExecuteFunc: func(context.Context, json.RawMessage, agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
						executed.Store(true)
						return agentcore.TextToolResult("ok"), nil
					},
				}
				return capability.Resolved{Tools: []agentcore.Tool{tool}}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			host, err := New(authorizationTestResolver{model: model}, registry)
			if err != nil {
				t.Fatal(err)
			}
			host.AgentDefaults.Hooks.BeforeToolCall = func(ctx context.Context, call agentcore.ToolCallContext) (agentcore.ToolCallDecision, error) {
				if !ToolAuthorizedFromContext(ctx, call.Call.Name) {
					return agentcore.ToolCallDecision{Block: true, Reason: "missing materialized tool authorization"}, nil
				}
				return agentcore.ToolCallDecision{}, nil
			}
			spec := ExecutionSpec{
				ExecutionID: "execution-1", SessionID: "session-1", AgentID: "agent-1",
				Model: ModelRef{Provider: "test", Model: "test"}, Prompt: "inspect",
				Capabilities: []capability.Ref{{Kind: capability.KindTool, Name: "workspace"}},
			}
			session, err := host.NewSession(context.Background(), spec, test.snapshot, agentcore.SessionOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()

			var result agentcore.Result
			if test.stream {
				stream, streamErr := session.Stream(context.Background(), []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "inspect")})
				if streamErr != nil {
					t.Fatal(streamErr)
				}
				result, err = stream.Result()
			} else {
				result, err = session.Prompt(context.Background(), []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "inspect")}, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.StopReason != agentcore.StopReasonStop || !executed.Load() {
				t.Fatalf("result=%+v tool_executed=%t", result, executed.Load())
			}
		})
	}
}
