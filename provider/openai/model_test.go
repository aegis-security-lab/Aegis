package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"aegis/agenthost"
	"aegis/secret"
	"github.com/z3r2ne/agentcore"
)

func TestModelRunsAgentcoreToolLoop(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("path=%s authorization=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "gpt-test" || body["stream"] != true || body["temperature"] != float64(0) {
			t.Fatalf("body = %#v", body)
		}
		messages := body["messages"].([]any)
		if call == 1 {
			if len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" || len(body["tools"].([]any)) != 1 {
				t.Fatalf("first body = %#v", body)
			}
		} else if len(messages) != 4 || messages[2].(map[string]any)["tool_calls"] == nil || messages[3].(map[string]any)["role"] != "tool" {
			t.Fatalf("second messages = %#v", messages)
		}
		response.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := response.(http.Flusher)
		write := func(payload string) {
			_, _ = fmt.Fprintf(response, "data: %s\n\n", payload)
			flusher.Flush()
		}
		if call == 1 {
			write(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"double","arguments":"{\"value\":"}}]},"finish_reason":null}]}`)
			write(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"2}"}}]},"finish_reason":"tool_calls"}]}`)
			write(`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":2}}}`)
		} else {
			write(`{"choices":[{"delta":{"content":"four"},"finish_reason":"stop"}]}`)
			write(`{"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":1}}`)
		}
		write(`[DONE]`)
	}))
	defer server.Close()

	model, err := NewModel(Config{BaseURL: server.URL + "/v1", APIKey: "test-key"}, "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	tool := agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{Name: "double", Parameters: json.RawMessage(`{"type":"object","properties":{"value":{"type":"integer"}},"required":["value"]}`)},
		ExecuteFunc: func(_ context.Context, arguments json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			if string(arguments) != `{"value":2}` {
				t.Fatalf("arguments = %s", arguments)
			}
			return agentcore.TextToolResult("4"), nil
		},
	}
	agent, err := agentcore.New(agentcore.Config{Model: model, SystemPrompt: "system", Tools: []agentcore.Tool{tool}, ModelOptions: map[string]any{"temperature": 0, "model": "ignored"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := agent.Prompt(context.Background(), agentcore.State{}, []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "double two")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || result.StopReason != agentcore.StopReasonStop || result.State.Messages[len(result.State.Messages)-1].Text() != "four" {
		t.Fatalf("calls=%d result=%+v", calls.Load(), result)
	}
	if result.Usage.InputTokens != 22 || result.Usage.OutputTokens != 4 || result.Usage.CacheReadTokens != 2 {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestResolverUsesSecretReferenceWithoutPuttingKeyInModelRef(t *testing.T) {
	resolvedRef := secret.Ref("")
	resolver := Resolver{
		Providers: map[string]ProviderConfig{"openai": {BaseURL: "https://example.test/v1", APIKeyRef: "env://openai"}},
		Secrets: secret.ResolverFunc(func(_ context.Context, ref secret.Ref) (secret.Value, error) {
			resolvedRef = ref
			return secret.NewValue([]byte("key")), nil
		}),
	}
	model, err := resolver.ResolveModel(context.Background(), agenthost.ModelRef{Provider: "OpenAI", Model: "gpt-test", Options: map[string]any{"temperature": 0}})
	if err != nil || model == nil || resolvedRef != "env://openai" {
		t.Fatalf("model=%v ref=%s err=%v", model, resolvedRef, err)
	}
	if strings.Contains(fmt.Sprintf("%+v", agenthost.ModelRef{Provider: "OpenAI", Model: "gpt-test"}), "key") {
		t.Fatal("model ref exposed credential")
	}
}

func TestModelReturnsBoundedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = response.Write([]byte("invalid credential"))
	}))
	defer server.Close()
	model, _ := NewModel(Config{BaseURL: server.URL}, "model")
	_, err := model.Stream(context.Background(), agentcore.ModelRequest{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("err = %v", err)
	}
}
