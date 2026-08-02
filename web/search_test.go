package web

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

func TestWebCapabilityReturnsBoundedUntrustedResults(t *testing.T) {
	searcher := SearcherFunc(func(_ context.Context, request SearchRequest) (SearchResult, error) {
		if request.Query != "agent runtimes" || request.MaxResults != 5 {
			t.Fatalf("request = %+v", request)
		}
		return SearchResult{Provider: "test", Query: request.Query, Results: []SearchItem{{Title: "Result", URL: "https://example.test", Content: strings.Repeat("evidence ", 100)}}}, nil
	})
	registry := capability.NewRegistry()
	if err := RegisterDefault(registry, Source{Searcher: searcher, MaxOutputBytes: 160}); err != nil {
		t.Fatal(err)
	}
	bundle, err := registry.Resolve(context.Background(), capability.ResolveContext{}, []capability.Ref{{Kind: capability.KindWeb, Name: "default"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Tools) != 1 || bundle.Tools[0].Definition().Name != "web_search" || len(bundle.Instructions) != 1 || !strings.Contains(bundle.Instructions[0].Content, "untrusted") {
		t.Fatalf("bundle = %+v", bundle)
	}
	result, err := bundle.Tools[0].Execute(context.Background(), json.RawMessage(`{"query":" agent runtimes "}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if text := result.Content[0].Text; len(text) > 160 || !strings.Contains(text, "truncated") {
		t.Fatalf("text len=%d text=%q", len(text), text)
	}
}

func TestWebToolPolicyIsExposed(t *testing.T) {
	source := Source{Searcher: SearcherFunc(func(context.Context, SearchRequest) (SearchResult, error) { return SearchResult{}, nil }), Policy: agentcore.ToolPolicy{MaxAttempts: 2}}
	resolved, err := source.Resolve(context.Background(), capability.ResolveContext{}, capability.Ref{Name: "default"})
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := resolved.Tools[0].(agentcore.ToolPolicyProvider)
	if !ok || provider.ToolPolicy().MaxAttempts != 2 {
		t.Fatalf("tool = %#v", resolved.Tools[0])
	}
}
