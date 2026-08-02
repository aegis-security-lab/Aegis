package agenthost

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

type sessionTestResolver struct{ model agentcore.Model }

func (r sessionTestResolver) ResolveModel(context.Context, ModelRef) (agentcore.Model, error) {
	return r.model, nil
}

type sessionTestModel struct {
	mu       sync.Mutex
	calls    int
	requests []agentcore.ModelRequest
	started  chan struct{}
	release  chan struct{}
}

func (m *sessionTestModel) Stream(_ context.Context, request agentcore.ModelRequest) (agentcore.ModelStream, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.requests = append(m.requests, request)
	m.mu.Unlock()
	if call == 1 {
		close(m.started)
		return &sessionTestStream{wait: m.release, chunks: []agentcore.ModelChunk{{TextDelta: "first", StopReason: agentcore.StopReasonStop}}}, nil
	}
	return &sessionTestStream{chunks: []agentcore.ModelChunk{{TextDelta: "second", StopReason: agentcore.StopReasonStop}}}, nil
}

type sessionTestStream struct {
	wait   <-chan struct{}
	chunks []agentcore.ModelChunk
}

func (s *sessionTestStream) Recv() (agentcore.ModelChunk, error) {
	if s.wait != nil {
		<-s.wait
		s.wait = nil
	}
	if len(s.chunks) == 0 {
		return agentcore.ModelChunk{}, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (*sessionTestStream) Close() error { return nil }

func TestSessionManagerDeliversFollowUpToRunningNativeAgent(t *testing.T) {
	model := &sessionTestModel{started: make(chan struct{}), release: make(chan struct{})}
	host, err := New(sessionTestResolver{model: model}, capability.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewSessionManager(context.Background(), host, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	spec := ExecutionSpec{ExecutionID: "execution-1", SessionID: "session-1", AgentID: "agent-1", Model: ModelRef{Provider: "test", Model: "test"}, Prompt: "initial"}
	if _, err := manager.Open(context.Background(), spec, nil); err != nil {
		t.Fatal(err)
	}
	result := make(chan ManagedRunResult, 1)
	manager.OnResult = func(value ManagedRunResult) { result <- value }
	if err := manager.Start("session-1", agentcore.TextMessage(agentcore.RoleUser, "initial")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("model did not start")
	}
	if err := manager.Deliver("session-1", DeliveryFollowUp, agentcore.TextMessage(agentcore.RoleUser, "child result")); err != nil {
		t.Fatal(err)
	}
	close(model.release)
	select {
	case completed := <-result:
		if completed.Err != nil || completed.Result.StopReason != agentcore.StopReasonStop {
			t.Fatalf("completed=%+v", completed)
		}
	case <-time.After(time.Second):
		t.Fatal("session did not complete")
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.calls != 2 || len(model.requests) != 2 {
		t.Fatalf("calls=%d requests=%d", model.calls, len(model.requests))
	}
	found := false
	for _, message := range model.requests[1].Messages {
		if message.Text() == "child result" {
			found = true
		}
	}
	if !found {
		t.Fatalf("follow-up not delivered: %+v", model.requests[1].Messages)
	}
}

func TestSessionManagerStartsIdleSessionOnDelivery(t *testing.T) {
	model := &sessionTestModel{started: make(chan struct{}), release: make(chan struct{})}
	close(model.release)
	host, _ := New(sessionTestResolver{model: model}, capability.NewRegistry())
	manager, _ := NewSessionManager(context.Background(), host, nil)
	defer manager.Close()
	spec := ExecutionSpec{ExecutionID: "execution-1", SessionID: "session-1", AgentID: "agent-1", Model: ModelRef{Provider: "test", Model: "test"}, Prompt: "initial"}
	_, _ = manager.Open(context.Background(), spec, nil)
	result := make(chan ManagedRunResult, 1)
	manager.OnResult = func(value ManagedRunResult) { result <- value }
	if err := manager.Deliver("session-1", DeliveryNextRun, agentcore.TextMessage(agentcore.RoleUser, "wake")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("idle delivery did not start a run")
	}
}
