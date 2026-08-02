package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aegis/capability"
	webcap "aegis/web"
)

func TestSkillLoaderReadsDetachedRegistryDocument(t *testing.T) {
	store := configuredStore(t)
	loader := SkillLoader{Store: store}
	document, err := loader.LoadSkill(context.Background(), "go-service-engineering", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if document.Name != "go-service-engineering" || document.Version != "1.0.0" || document.Content == "" || document.Metadata["source"] == "" {
		t.Fatalf("document = %+v", document)
	}
	document.Metadata["source"] = "mutated"
	reloaded, err := loader.LoadSkill(context.Background(), "go-service-engineering", "")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Metadata["source"] == "mutated" {
		t.Fatal("loader leaked mutable registry state")
	}
}

func TestWebSearcherKeepsCredentialInsideControlStore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer private-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{"results": []map[string]any{{"title": "Docs", "url": "https://example.test/docs"}}})
	}))
	defer server.Close()
	store := configuredStore(t)
	if _, err := store.SaveWebSearchConfig(WebSearchConfig{Engine: "tavily", BaseURL: server.URL, APIKey: "private-key"}); err != nil {
		t.Fatal(err)
	}
	result, err := (WebSearcher{Store: store}).Search(context.Background(), webcap.SearchRequest{Query: "docs", Topic: "general", Depth: "basic", MaxResults: 1})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	if len(result.Results) != 1 || string(encoded) == "" || contains(string(encoded), "private-key") {
		t.Fatalf("result = %s", encoded)
	}
}

func TestNativeHostSupportsDynamicSkillsAndExternalPluginSources(t *testing.T) {
	store := configuredStore(t)
	mcpSource := capability.SourceFunc(func(_ context.Context, _ capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
		return capability.Resolved{Snapshot: capability.Snapshot{Kind: ref.Kind, Name: ref.Name, Version: ref.Version}}, nil
	})
	host, err := NewNativeAgentHost(store, NativeHostOptions{Sources: []NativeCapabilitySource{{Kind: capability.KindMCP, Name: "github", Source: mcpSource}}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSkill(SaveSkillInput{Name: "late-skill", Description: "Created after AgentHost startup", Version: "1", Content: "---\nname: late-skill\ndescription: Created after AgentHost startup\n---\n\n# Workflow\n\nUse late instructions."})
	if err != nil {
		t.Fatal(err)
	}
	refs := []capability.Ref{{Kind: capability.KindSkill, Name: created.ID}, {Kind: capability.KindMCP, Name: "github", Version: "1"}}
	for _, ref := range refs {
		if !host.Capabilities.Has(ref) {
			t.Fatalf("capability %s/%s is not advertised", ref.Kind, ref.Name)
		}
	}
	bundle, err := host.Capabilities.Resolve(context.Background(), capability.ResolveContext{ExecutionID: "exec-plugin", AgentID: "backend-engineer"}, refs)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	if len(bundle.Instructions) != 1 || len(bundle.Snapshots) != 2 || bundle.Snapshots[1].Name != "github" {
		t.Fatalf("bundle=%+v", bundle)
	}
}

func TestNativeHostRootGuardIsSeparateFromChildExecutionBudget(t *testing.T) {
	store := configuredStore(t)
	host, err := NewNativeAgentHost(store)
	if err != nil {
		t.Fatal(err)
	}
	if host.AgentDefaults.MaxTurns != 1000 {
		t.Fatalf("root emergency guard=%d, want 1000", host.AgentDefaults.MaxTurns)
	}
	budget := normalizeIssueBudget(store.Config().IssueBudget)
	if budget.MaxTurns != 100 || budget.SummaryTurns != 10 {
		t.Fatalf("child budget changed with root guard: %+v", budget)
	}
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
