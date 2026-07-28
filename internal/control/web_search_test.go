package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestTavilySearchUsesConfiguredBearerTokenAndNormalizesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer th-secret" {
			t.Fatalf("unexpected authorization header: %q", request.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["query"] != "latest AI agent news" || body["search_depth"] != "basic" || body["max_results"] != float64(5) {
			t.Fatalf("unexpected request body: %#v", body)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"answer":"Summary","response_time":0.42,"results":[{"title":"Result","url":"https://example.com","content":"Details","score":0.9}]}`))
	}))
	defer server.Close()

	result, err := executeWebSearch(context.Background(), WebSearchConfig{Engine: "tavily", BaseURL: server.URL, APIKey: "th-secret", Enabled: true}, WebSearchInput{Query: "latest AI agent news", Topic: "general", SearchDepth: "basic", IncludeAnswer: true, MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "Summary" || len(result.Results) != 1 || result.Results[0].URL != "https://example.com" {
		t.Fatalf("unexpected search result: %+v", result)
	}
}

func TestWebSearchIsRequiredForEveryEmployeeAndSecretIsNotExposed(t *testing.T) {
	s := configuredStore(t)
	for _, agent := range s.Agents() {
		if isEmployeeAgent(agent) && !slices.Contains(agent.Tools, "aegis_web_search") {
			t.Fatalf("employee %s is missing aegis_web_search: %v", agent.ID, agent.Tools)
		}
	}
	config := s.Config()
	config.WebSearch = WebSearchConfig{Engine: "tavily", BaseURL: "https://search.example/api/tavily/search", APIKey: "th-secret", Enabled: true}
	view := configView(config)
	if !view.WebSearch.HasAPIKey || view.WebSearch.BaseURL == "" {
		t.Fatalf("unexpected search config view: %+v", view.WebSearch)
	}
}
