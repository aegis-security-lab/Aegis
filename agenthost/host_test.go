package agenthost

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

type captureModel struct {
	request agentcore.ModelRequest
}

func (m *captureModel) Stream(_ context.Context, request agentcore.ModelRequest) (agentcore.ModelStream, error) {
	m.request = request
	return &hostStream{}, nil
}

type hostStream struct {
	done bool
}

func (s *hostStream) Recv() (agentcore.ModelChunk, error) {
	if s.done {
		return agentcore.ModelChunk{}, io.EOF
	}
	s.done = true
	return agentcore.ModelChunk{TextDelta: "done", StopReason: agentcore.StopReasonStop}, nil
}

func (*hostStream) Close() error { return nil }

type closeRecorder struct{ closed *bool }

func (c closeRecorder) Close() error { *c.closed = true; return nil }

func TestHostResolvesCapabilitiesAndRunsAgent(t *testing.T) {
	model := &captureModel{}
	registry := capability.NewRegistry()
	closed := false
	err := registry.Register(capability.KindSkill, "review", capability.SourceFunc(func(_ context.Context, execution capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
		if execution.ExecutionID != "execution-1" || execution.AgentID != "agent-1" || execution.Workspace != "/workspace" {
			t.Fatalf("execution = %+v", execution)
		}
		tool := agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{Name: "review_tool"}, ExecuteFunc: func(context.Context, json.RawMessage, agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			return agentcore.TextToolResult("ok"), nil
		}}
		return capability.Resolved{
			Tools:        []agentcore.Tool{tool},
			Instructions: []capability.Instruction{{Source: `review"><bad`, Content: "Review carefully."}},
			Closers:      []io.Closer{closeRecorder{closed: &closed}},
			Snapshot:     capability.Snapshot{Version: "1.2.3"},
		}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	host, err := New(ModelResolverFunc(func(_ context.Context, ref ModelRef) (agentcore.Model, error) {
		if ref.Provider != "test" || ref.Model != "model" {
			t.Fatalf("model ref = %+v", ref)
		}
		return model, nil
	}), registry)
	if err != nil {
		t.Fatal(err)
	}
	spec := ExecutionSpec{
		ExecutionID: "execution-1", AgentID: "agent-1", Workspace: "/workspace",
		Model:        ModelRef{Provider: "test", Model: "model", Options: map[string]any{"thinking": "high"}},
		SystemPrompt: "Base.", Prompt: "Do work.",
		Capabilities: []capability.Ref{{Kind: capability.KindSkill, Name: "review"}},
	}
	result, err := host.Run(context.Background(), spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Core.State.Messages[len(result.Core.State.Messages)-1].Text() != "done" || len(result.Capabilities) != 1 || result.Capabilities[0].Version != "1.2.3" || !closed {
		t.Fatalf("result=%+v closed=%v", result, closed)
	}
	if len(model.request.Tools) != 1 || model.request.Tools[0].Name != "review_tool" || !reflect.DeepEqual(model.request.Options, spec.Model.Options) {
		t.Fatalf("request = %+v", model.request)
	}
	if !strings.Contains(model.request.SystemPrompt, "Review carefully.") || strings.Contains(model.request.SystemPrompt, `source=\"review\"><bad`) {
		t.Fatalf("system prompt = %q", model.request.SystemPrompt)
	}
}
