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
	agentcoreopenai "github.com/z3r2ne/agentcore/provider/openai"
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

func TestModelReplaysReasoningContentAfterToolCall(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if call == 2 {
			messages := body["messages"].([]any)
			assistant := messages[1].(map[string]any)
			if assistant["reasoning_content"] != "inspect first" {
				t.Fatalf("second request reasoning_content = %#v", assistant["reasoning_content"])
			}
			vendor, ok := assistant["vendor_field"].(map[string]any)
			if !ok || vendor["signature"] != "sig-1" {
				t.Fatalf("second request vendor_field = %#v", assistant["vendor_field"])
			}
		}

		response.Header().Set("Content-Type", "text/event-stream")
		if call == 1 {
			_, _ = fmt.Fprint(response, "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"inspect first\",\"vendor_field\":{\"signature\":\"sig-1\"},\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"inspect\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		} else {
			_, _ = fmt.Fprint(response, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\n")
		}
		_, _ = fmt.Fprint(response, "data: [DONE]\n\n")
	}))
	defer server.Close()

	model, err := NewModel(Config{BaseURL: server.URL}, "reasoning-model")
	if err != nil {
		t.Fatal(err)
	}
	tool := agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{Name: "inspect", Parameters: json.RawMessage(`{"type":"object"}`)},
		ExecuteFunc: func(context.Context, json.RawMessage, agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			return agentcore.TextToolResult("inspected"), nil
		},
	}
	agent, err := agentcore.New(agentcore.Config{Model: model, Tools: []agentcore.Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := agent.Prompt(context.Background(), agentcore.State{}, []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "inspect")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || result.State.Messages[len(result.State.Messages)-1].Text() != "done" {
		t.Fatalf("calls=%d result=%+v", calls.Load(), result)
	}
	assistant := result.State.Messages[1]
	if assistant.ProviderData == nil || assistant.ProviderData.Format != agentcoreopenai.ProviderDataFormat {
		t.Fatalf("provider data = %#v", assistant.ProviderData)
	}
}

func TestSanitizeReplayMessagesRepairsReasoningOnlyProviderData(t *testing.T) {
	originalProviderData := &agentcore.ProviderData{
		Format: agentcoreopenai.ProviderDataFormat,
		Data: json.RawMessage(`{
			"message": {
				"role": "assistant",
				"content": null,
				"reasoning_content": "private reasoning",
				"vendor_field": {"signature": "sig-1"}
			},
			"response": {"id": "response-1"}
		}`),
	}
	messages := []agentcore.Message{
		agentcore.TextMessage(agentcore.RoleUser, "first request"),
		{
			Role:         agentcore.RoleAssistant,
			Content:      []agentcore.ContentBlock{{Type: agentcore.ContentThinking, Text: "private reasoning"}},
			StopReason:   agentcore.StopReasonStop,
			ProviderData: originalProviderData,
		},
		{
			Role:       agentcore.RoleAssistant,
			StopReason: agentcore.StopReasonError,
			IsError:    true,
			Error:      "upstream failed",
		},
		{
			Role: agentcore.RoleAssistant,
			Content: []agentcore.ContentBlock{{Type: agentcore.ContentToolCall, ToolCall: &agentcore.ToolCall{
				ID: "call-1", Name: "inspect", Arguments: json.RawMessage(`{}`),
			}}},
		},
	}

	sanitized, stats := sanitizeReplayMessages(messages)
	if stats.repaired != 2 || stats.reasoningOnly != 1 || stats.errors != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	if sanitized[1].Text() != interruptedAssistantPlaceholder || sanitized[2].Text() != interruptedAssistantPlaceholder {
		t.Fatalf("sanitized messages = %#v", sanitized)
	}
	if messages[1].Text() != "" || messages[2].Text() != "" {
		t.Fatal("sanitizer mutated stored history")
	}
	if sanitized[3].Text() != "" || len(sanitized[3].ToolCalls()) != 1 {
		t.Fatalf("valid tool-call assistant changed: %#v", sanitized[3])
	}

	var preserved struct {
		Message  map[string]any `json:"message"`
		Response map[string]any `json:"response"`
	}
	if err := json.Unmarshal(sanitized[1].ProviderData.Data, &preserved); err != nil {
		t.Fatal(err)
	}
	if preserved.Message["content"] != interruptedAssistantPlaceholder || preserved.Message["reasoning_content"] != "private reasoning" {
		t.Fatalf("repaired provider message = %#v", preserved.Message)
	}
	vendor, ok := preserved.Message["vendor_field"].(map[string]any)
	if !ok || vendor["signature"] != "sig-1" || preserved.Response["id"] != "response-1" {
		t.Fatalf("provider-specific data was lost: %#v", preserved)
	}
	if strings.Contains(string(originalProviderData.Data), interruptedAssistantPlaceholder) {
		t.Fatal("sanitizer mutated original ProviderData")
	}
}

func TestModelSelfHealsReasoningOnlyProviderDataOnWire(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		messages := body["messages"].([]any)
		if len(messages) != 3 {
			t.Fatalf("messages = %#v", messages)
		}
		assistant := messages[1].(map[string]any)
		if assistant["content"] != interruptedAssistantPlaceholder || assistant["reasoning_content"] != "private reasoning" {
			t.Fatalf("assistant = %#v", assistant)
		}
		vendor, ok := assistant["vendor_field"].(map[string]any)
		if !ok || vendor["signature"] != "sig-1" {
			t.Fatalf("vendor_field = %#v", assistant["vendor_field"])
		}

		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(response, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"recovered\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = fmt.Fprint(response, "data: [DONE]\n\n")
	}))
	defer server.Close()

	model, err := NewModel(Config{BaseURL: server.URL}, "reasoning-recovery-model")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentcore.New(agentcore.Config{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	poisonedState := agentcore.State{Messages: []agentcore.Message{
		agentcore.TextMessage(agentcore.RoleUser, "first request"),
		{
			Role:       agentcore.RoleAssistant,
			Content:    []agentcore.ContentBlock{{Type: agentcore.ContentThinking, Text: "private reasoning"}},
			StopReason: agentcore.StopReasonStop,
			ProviderData: &agentcore.ProviderData{
				Format: agentcoreopenai.ProviderDataFormat,
				Data:   json.RawMessage(`{"message":{"role":"assistant","content":null,"reasoning_content":"private reasoning","vendor_field":{"signature":"sig-1"}}}`),
			},
		},
	}}
	result, err := agent.Prompt(
		context.Background(),
		poisonedState,
		[]agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "retry")},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.State.Messages[len(result.State.Messages)-1].Text() != "recovered" {
		t.Fatalf("result = %+v", result)
	}
}

func TestModelSelfHealsPoisonedSessionAfterProviderError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		if call == 1 {
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(response, `{"error":{"message":"temporary upstream failure","type":"invalid_request_error"}}`)
			return
		}

		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		messages := body["messages"].([]any)
		if len(messages) != 3 {
			t.Fatalf("second request messages = %#v", messages)
		}
		failedAssistant := messages[1].(map[string]any)
		if failedAssistant["role"] != "assistant" || failedAssistant["content"] != interruptedAssistantPlaceholder {
			t.Fatalf("failed assistant was not healed: %#v", failedAssistant)
		}

		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(response, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"recovered\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = fmt.Fprint(response, "data: [DONE]\n\n")
	}))
	defer server.Close()

	model, err := NewModel(Config{BaseURL: server.URL}, "recovery-model")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentcore.New(agentcore.Config{Model: model, ModelRetry: agentcore.RetryPolicy{MaxAttempts: 1}})
	if err != nil {
		t.Fatal(err)
	}
	failed, firstErr := agent.Prompt(
		context.Background(),
		agentcore.State{},
		[]agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "first request")},
		nil,
	)
	if firstErr == nil {
		t.Fatal("first provider failure unexpectedly succeeded")
	}
	poison := failed.State.Messages[len(failed.State.Messages)-1]
	if poison.Role != agentcore.RoleAssistant || !poison.IsError || poison.Text() != "" {
		t.Fatalf("expected zero-content error assistant, got %#v", poison)
	}

	recovered, err := agent.Prompt(
		context.Background(),
		failed.State,
		[]agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "retry")},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || recovered.State.Messages[len(recovered.State.Messages)-1].Text() != "recovered" {
		t.Fatalf("calls=%d recovered=%+v", calls.Load(), recovered)
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
