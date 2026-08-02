package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

type fakeSession struct {
	tools    []agentcore.Tool
	toolsErr error
	closed   bool
}

func (s *fakeSession) Tools(context.Context) ([]agentcore.Tool, error) { return s.tools, s.toolsErr }
func (s *fakeSession) Close() error {
	s.closed = true
	return nil
}

func TestSourceOwnsMCPSessionLifecycle(t *testing.T) {
	session := &fakeSession{tools: []agentcore.Tool{agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{Name: "mcp_lookup"},
		ExecuteFunc: func(context.Context, json.RawMessage, agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			return agentcore.ToolResult{}, nil
		},
	}}}
	connector := ConnectorFunc(func(_ context.Context, request ConnectRequest) (Session, error) {
		if request.Execution.ExecutionID != "exec-1" || request.Name != "docs" || request.Config["secretRef"] != "vault://mcp/docs" {
			t.Fatalf("request = %+v", request)
		}
		request.Config["secretRef"] = "mutated"
		return session, nil
	})
	registry := capability.NewRegistry()
	if err := Register(registry, "docs", connector); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"secretRef": "vault://mcp/docs"}
	bundle, err := registry.Resolve(context.Background(), capability.ResolveContext{ExecutionID: "exec-1"}, []capability.Ref{{Kind: capability.KindMCP, Name: "docs", Config: config}})
	if err != nil {
		t.Fatal(err)
	}
	if config["secretRef"] != "vault://mcp/docs" || len(bundle.Tools) != 1 || bundle.Snapshots[0].Metadata["toolCount"] != "1" {
		t.Fatalf("bundle=%+v config=%v", bundle, config)
	}
	if err := bundle.Close(); err != nil || !session.closed {
		t.Fatalf("close err=%v closed=%v", err, session.closed)
	}
}

func TestSourceClosesSessionWhenToolDiscoveryFails(t *testing.T) {
	session := &fakeSession{toolsErr: errors.New("protocol error")}
	source := Source{Connector: ConnectorFunc(func(context.Context, ConnectRequest) (Session, error) { return session, nil })}
	_, err := source.Resolve(context.Background(), capability.ResolveContext{}, capability.Ref{Kind: capability.KindMCP, Name: "broken"})
	if err == nil || !session.closed {
		t.Fatalf("err=%v closed=%v", err, session.closed)
	}
}
