package agentcoreadapter

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"aegis/agentapp"
	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

type fakePhoneClient struct {
	mu           sync.Mutex
	startCalls   int
	startRequest agentapp.StartSessionRequest
	viewCalls    int
	actRequests  []agentapp.ActionRequest
	failActs     int
}

func (c *fakePhoneClient) Start(_ context.Context, request agentapp.StartSessionRequest) (agentapp.StartSessionResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startCalls++
	c.startRequest = request
	if request.AgentID != "agent-1" {
		return agentapp.StartSessionResponse{}, errors.New("unexpected agent")
	}
	return agentapp.StartSessionResponse{PhoneSessionID: "phone-1", Page: phonePage("rev-1", "Home")}, nil
}

func (c *fakePhoneClient) View(_ context.Context, sessionID string) (agentapp.Page, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.viewCalls++
	if sessionID != "phone-1" {
		return agentapp.Page{}, errors.New("unexpected session")
	}
	return phonePage("rev-restored", "Restored"), nil
}

func (c *fakePhoneClient) Act(_ context.Context, request agentapp.ActionRequest) (agentapp.ActionResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.actRequests = append(c.actRequests, request)
	if len(c.actRequests) <= c.failActs {
		return agentapp.ActionResponse{}, errors.New("temporary network failure")
	}
	return agentapp.ActionResponse{Status: "ok", Effect: "navigated", Page: phonePage("rev-2", "Issue")}, nil
}

func phonePage(revision, title string) agentapp.Page {
	return agentapp.Page{AppID: "aegis.board", PageID: "page", Revision: revision, Title: title, Text: "[PAGE] " + title + "\n@1 [link] Open\n"}
}

type phoneModel struct {
	responses [][]agentcore.ModelChunk
}

func (m *phoneModel) Stream(context.Context, agentcore.ModelRequest) (agentcore.ModelStream, error) {
	response := m.responses[0]
	m.responses = m.responses[1:]
	return &phoneModelStream{chunks: response}, nil
}

type phoneModelStream struct{ chunks []agentcore.ModelChunk }

func (s *phoneModelStream) Recv() (agentcore.ModelChunk, error) {
	if len(s.chunks) == 0 {
		return agentcore.ModelChunk{}, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (*phoneModelStream) Close() error { return nil }

func TestPhoneActionUsesStableCallIDAcrossRetries(t *testing.T) {
	client := &fakePhoneClient{failActs: 1}
	toolset, err := NewToolset(ToolsetConfig{
		Client: client, ExecutionID: "execution-1", AgentID: "agent-1",
		Policy: agentcore.ToolPolicy{MaxAttempts: 2, RetryDelay: time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	tools := toolset.Tools()
	for _, tool := range tools {
		mode, ok := tool.(agentcore.ToolExecutionModeProvider)
		if !ok || mode.ExecutionMode() != agentcore.ToolExecutionSequential {
			t.Fatalf("tool %s is not sequential", tool.Definition().Name)
		}
	}
	model := &phoneModel{responses: [][]agentcore.ModelChunk{
		{{ToolCallDeltas: []agentcore.ToolCallDelta{{Index: 0, ID: "call-1", Name: "phone_action", ArgumentsDelta: `{"action":"click","ref":"@1"}`}}, StopReason: agentcore.StopReasonToolUse}},
		{{TextDelta: "done", StopReason: agentcore.StopReasonStop}},
	}}
	agent, err := agentcore.New(agentcore.Config{Model: model, Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	result, err := agent.Prompt(context.Background(), agentcore.State{}, []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "open")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.actRequests) != 2 {
		t.Fatalf("requests = %+v", client.actRequests)
	}
	first, second := client.actRequests[0], client.actRequests[1]
	if first.IdempotencyKey != "phone:execution-1:call-1" || second.IdempotencyKey != first.IdempotencyKey || first.PageRevision != "rev-1" || first.PhoneSessionID != "phone-1" {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	if result.State.Messages[2].Role != agentcore.RoleTool || !strings.Contains(result.State.Messages[2].Text(), "[PAGE] Issue") {
		t.Fatalf("result = %+v", result)
	}
}

func TestPhoneCapabilitySourceRestoresSessionAndSnapshotsIt(t *testing.T) {
	client := &fakePhoneClient{}
	registry := capability.NewRegistry()
	if err := RegisterDefault(registry, Source{Client: client, InstalledApps: []string{"aegis.board"}}); err != nil {
		t.Fatal(err)
	}
	bundle, err := registry.Resolve(context.Background(), capability.ResolveContext{ExecutionID: "execution-2", AgentID: "agent-1"}, []capability.Ref{{
		Kind: capability.KindPhone, Name: "default", Version: "1", Config: map[string]any{"sessionId": "phone-1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	if client.viewCalls != 1 || client.startCalls != 0 || len(bundle.Tools) != 4 || len(bundle.Instructions) != 1 {
		t.Fatalf("bundle=%+v client=%+v", bundle, client)
	}
	if got := bundle.Snapshots[0].Metadata; got["phoneSessionId"] != "phone-1" || got["pageRevision"] != "rev-restored" {
		t.Fatalf("metadata = %v", got)
	}
}

func TestPhoneCapabilitySourcePropagatesExecutionWorkspaceIdentity(t *testing.T) {
	client := &fakePhoneClient{}
	registry := capability.NewRegistry()
	if err := RegisterDefault(registry, Source{Client: client, InstalledApps: []string{"aegis.board"}}); err != nil {
		t.Fatal(err)
	}
	bundle, err := registry.Resolve(context.Background(), capability.ResolveContext{
		ExecutionID: "execution-2", AgentID: "agent-1", Workspace: "/workspace/repo",
	}, []capability.Ref{{Kind: capability.KindPhone, Name: "default"}})
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	if client.startRequest.ExecutionID != "execution-2" || client.startRequest.WorkspaceID != "/workspace/repo" || client.startRequest.AgentID != "agent-1" {
		t.Fatalf("start request=%+v", client.startRequest)
	}
}

func TestPhoneCapabilitySourceRestrictsAppsPerExecution(t *testing.T) {
	client := &fakePhoneClient{}
	registry := capability.NewRegistry()
	if err := RegisterDefault(registry, Source{Client: client, InstalledApps: []string{"aegis.board", "aegis.relay"}, Scopes: []string{"read", "write"}}); err != nil {
		t.Fatal(err)
	}
	bundle, err := registry.Resolve(context.Background(), capability.ResolveContext{ExecutionID: "execution-3", AgentID: "agent-1"}, []capability.Ref{{
		Kind: capability.KindPhone, Name: "default", Config: map[string]any{"installedApps": []any{"aegis.board"}, "scopes": []any{"read"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	if len(client.startRequest.InstalledApps) != 1 || client.startRequest.InstalledApps[0] != "aegis.board" || len(client.startRequest.Scopes) != 1 || client.startRequest.Scopes[0] != "read" {
		t.Fatalf("start request=%+v", client.startRequest)
	}
	_, err = registry.Resolve(context.Background(), capability.ResolveContext{ExecutionID: "execution-4", AgentID: "agent-1"}, []capability.Ref{{
		Kind: capability.KindPhone, Name: "default", Config: map[string]any{"installedApps": []any{"aegis.admin"}},
	}})
	if err == nil {
		t.Fatal("unapproved app should be rejected")
	}
}
