package runtimeapp

import (
	"context"
	"io"
	"sync"
	"testing"

	"aegis/agenthost"
	"aegis/capability"
	"aegis/coordination"
	"aegis/policy"
	"aegis/storage"
	webcap "aegis/web"
	"github.com/z3r2ne/agentcore"
)

type scriptedModel struct {
	mu       sync.Mutex
	requests []agentcore.ModelRequest
}

func (m *scriptedModel) Stream(_ context.Context, request agentcore.ModelRequest) (agentcore.ModelStream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, request)
	if len(m.requests) == 1 {
		return &scriptedStream{chunks: []agentcore.ModelChunk{{ToolCallDeltas: []agentcore.ToolCallDelta{{Index: 0, ID: "search-1", Name: "web_search", ArgumentsDelta: `{"query":"Go agents"}`}}, StopReason: agentcore.StopReasonToolUse}}}, nil
	}
	return &scriptedStream{chunks: []agentcore.ModelChunk{{TextDelta: "finished", StopReason: agentcore.StopReasonStop}}}, nil
}

type scriptedStream struct{ chunks []agentcore.ModelChunk }

func (s *scriptedStream) Recv() (agentcore.ModelChunk, error) {
	if len(s.chunks) == 0 {
		return agentcore.ModelChunk{}, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}
func (*scriptedStream) Close() error { return nil }

func TestRuntimeRunsScheduledExecutionThroughResolvedWebTool(t *testing.T) {
	model := &scriptedModel{}
	repository := coordination.NewMemoryExecutionRepository()
	events := storage.NewMemoryEventStore()
	searchCalls := 0
	policyCalls := 0
	runtime, err := New(Config{
		Models:     agenthost.ModelResolverFunc(func(context.Context, agenthost.ModelRef) (agentcore.Model, error) { return model, nil }),
		Repository: repository, Events: events, WorkerID: "worker-1",
		Authorizer: policy.AuthorizerFunc(func(_ context.Context, request policy.ToolRequest) (policy.Decision, error) {
			policyCalls++
			if request.Execution.ExecutionID != "execution-1" || request.Execution.AgentID != "agent-1" || request.Call.Name != "web_search" {
				t.Fatalf("policy request = %+v", request)
			}
			return policy.Decision{Allow: true}, nil
		}),
		Web: &webcap.Source{Searcher: webcap.SearcherFunc(func(_ context.Context, request webcap.SearchRequest) (webcap.SearchResult, error) {
			searchCalls++
			return webcap.SearchResult{Provider: "test", Query: request.Query, Results: []webcap.SearchItem{{Title: "Docs", URL: "https://example.test"}}}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	execution := coordination.Execution{ID: "execution-1", Spec: agenthost.ExecutionSpec{
		AgentID: "agent-1", Prompt: "research", Model: agenthost.ModelRef{Provider: "test", Model: "model"},
		Capabilities: []capability.Ref{{Kind: capability.KindWeb, Name: "default"}},
	}}
	if err := runtime.Executions.Enqueue(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	outcome, claimed, err := runtime.Worker.RunNext(context.Background())
	if err != nil || !claimed || outcome.Status != coordination.ExecutionSucceeded {
		t.Fatalf("outcome=%+v claimed=%v err=%v", outcome, claimed, err)
	}
	stored, err := runtime.Executions.Get(context.Background(), execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if searchCalls != 1 || policyCalls != 1 || stored.Result == nil || stored.Result.Core.State.Messages[len(stored.Result.Core.State.Messages)-1].Text() != "finished" {
		t.Fatalf("searchCalls=%d policyCalls=%d stored=%+v", searchCalls, policyCalls, stored)
	}
	persistedEvents, err := events.ExecutionEvents(context.Background(), execution.ID, 0, 100)
	if err != nil || len(persistedEvents) == 0 || persistedEvents[0].Event.Type != agentcore.EventAgentStart {
		t.Fatalf("events=%+v err=%v", persistedEvents, err)
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if len(model.requests) != 2 || model.requests[1].Messages[2].Role != agentcore.RoleTool {
		t.Fatalf("requests = %+v", model.requests)
	}
}
