package einoadapter

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"aegis/agentcore"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeEinoModel struct {
	mu        sync.Mutex
	responses [][]*schema.Message
	requests  [][]*schema.Message
	tools     []*schema.ToolInfo
}

func (m *fakeEinoModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	panic("Generate should not be called")
}

func (m *fakeEinoModel) Stream(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, messages)
	response := m.responses[0]
	m.responses = m.responses[1:]
	return schema.StreamReaderFromArray(response), nil
}

func (m *fakeEinoModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m.tools = tools
	return m, nil
}

func TestModelPreservesEinoProviderDataAcrossToolTurns(t *testing.T) {
	toolIndex := 0
	einoModel := &fakeEinoModel{responses: [][]*schema.Message{
		{
			{
				Role:             schema.Assistant,
				ReasoningContent: "think",
				ToolCalls: []schema.ToolCall{{
					Index: &toolIndex,
					ID:    "call-1",
					Type:  "function",
					Function: schema.FunctionCall{
						Name:      "echo",
						Arguments: `{"text":"hi"}`,
					},
				}},
				Extra: map[string]any{"provider_signature": "preserve-me"},
			},
			{Role: schema.Assistant, ResponseMeta: &schema.ResponseMeta{
				FinishReason: "tool_calls",
				Usage:        &schema.TokenUsage{PromptTokens: 4, CompletionTokens: 2},
			}},
		},
		{{Role: schema.Assistant, Content: "done", ResponseMeta: &schema.ResponseMeta{FinishReason: "stop"}}},
	}}
	echo := agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{
			Name:        "echo",
			Description: "echo text",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
		},
		ExecuteFunc: func(_ context.Context, arguments json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			return agentcore.TextToolResult(string(arguments)), nil
		},
	}
	agent, err := agentcore.New(agentcore.Config{
		Model:        Model{ChatModel: einoModel},
		SystemPrompt: "system",
		Tools:        []agentcore.Tool{echo},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := agent.Prompt(context.Background(), agentcore.State{}, []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "go")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.State.Messages[len(result.State.Messages)-1].Text() != "done" {
		t.Fatalf("result = %+v", result)
	}
	if len(einoModel.tools) != 1 || einoModel.tools[0].Name != "echo" {
		t.Fatalf("tools = %+v", einoModel.tools)
	}
	if len(einoModel.requests) != 2 {
		t.Fatalf("requests = %d", len(einoModel.requests))
	}
	second := einoModel.requests[1]
	if len(second) != 4 {
		t.Fatalf("second request messages = %+v", second)
	}
	if second[0].Role != schema.System || second[0].Content != "system" {
		t.Fatalf("system message = %+v", second[0])
	}
	assistant := second[2]
	if assistant.ReasoningContent != "think" || assistant.Extra["provider_signature"] != "preserve-me" {
		t.Fatalf("provider data was not preserved: %+v", assistant)
	}
	if got := result.State.Messages[1].Usage; !reflect.DeepEqual(got, agentcore.Usage{InputTokens: 4, OutputTokens: 2}) {
		t.Fatalf("usage = %+v", got)
	}
}
