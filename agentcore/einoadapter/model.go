package einoadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aegis/agentcore"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	jsonschema "github.com/eino-contrib/jsonschema"
)

// Model adapts an immutable Eino ToolCallingChatModel to agentcore.Model.
type Model struct {
	ChatModel model.ToolCallingChatModel
}

func (m Model) Stream(ctx context.Context, request agentcore.ModelRequest) (agentcore.ModelStream, error) {
	if m.ChatModel == nil {
		return nil, errors.New("einoadapter: nil Eino chat model")
	}
	infos := make([]*schema.ToolInfo, 0, len(request.Tools))
	for _, definition := range request.Tools {
		info, err := toolInfo(definition)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	chatModel := m.ChatModel
	if len(infos) > 0 {
		var err error
		chatModel, err = chatModel.WithTools(infos)
		if err != nil {
			return nil, fmt.Errorf("einoadapter: bind tools: %w", err)
		}
	}
	messages := make([]*schema.Message, 0, len(request.Messages)+1)
	if request.SystemPrompt != "" {
		messages = append(messages, schema.SystemMessage(request.SystemPrompt))
	}
	for _, message := range request.Messages {
		converted, err := toEinoMessage(message)
		if err != nil {
			return nil, err
		}
		messages = append(messages, converted)
	}
	stream, err := chatModel.Stream(ctx, messages)
	if err != nil {
		return nil, err
	}
	return &modelStream{stream: stream}, nil
}

type modelStream struct {
	stream *schema.StreamReader[*schema.Message]
	merged *schema.Message
}

func (s *modelStream) Recv() (agentcore.ModelChunk, error) {
	message, err := s.stream.Recv()
	if err != nil {
		return agentcore.ModelChunk{}, err
	}
	chunk := agentcore.ModelChunk{
		TextDelta:     message.Content,
		ThinkingDelta: message.ReasoningContent,
	}
	toMerge := []*schema.Message{message}
	if s.merged != nil {
		toMerge = append([]*schema.Message{s.merged}, toMerge...)
	}
	s.merged, err = schema.ConcatMessages(toMerge)
	if err != nil {
		return agentcore.ModelChunk{}, fmt.Errorf("einoadapter: merge model chunks: %w", err)
	}
	chunk.ProviderData = s.merged
	for position, call := range message.ToolCalls {
		index := position
		if call.Index != nil {
			index = *call.Index
		}
		chunk.ToolCallDeltas = append(chunk.ToolCallDeltas, agentcore.ToolCallDelta{
			Index:          index,
			ID:             call.ID,
			Name:           call.Function.Name,
			ArgumentsDelta: call.Function.Arguments,
		})
	}
	if message.ResponseMeta != nil {
		chunk.StopReason = normalizeStopReason(message.ResponseMeta.FinishReason)
		if message.ResponseMeta.Usage != nil {
			chunk.Usage = &agentcore.Usage{
				InputTokens:     message.ResponseMeta.Usage.PromptTokens,
				OutputTokens:    message.ResponseMeta.Usage.CompletionTokens,
				CacheReadTokens: message.ResponseMeta.Usage.PromptTokenDetails.CachedTokens,
			}
		}
	}
	return chunk, nil
}

func (s *modelStream) Close() error {
	s.stream.Close()
	return nil
}

func toolInfo(definition agentcore.ToolDefinition) (*schema.ToolInfo, error) {
	info := &schema.ToolInfo{Name: definition.Name, Desc: definition.Description}
	if len(definition.Parameters) == 0 {
		return info, nil
	}
	var parameters jsonschema.Schema
	if err := json.Unmarshal(definition.Parameters, &parameters); err != nil {
		return nil, fmt.Errorf("einoadapter: invalid JSON schema for tool %q: %w", definition.Name, err)
	}
	info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(&parameters)
	return info, nil
}

func toEinoMessage(message agentcore.Message) (*schema.Message, error) {
	if preserved, ok := message.ProviderData.(*schema.Message); ok && preserved != nil {
		return preserved, nil
	}
	converted := &schema.Message{
		Role:       schema.RoleType(message.Role),
		ToolCallID: message.ToolCallID,
		ToolName:   message.ToolName,
	}
	for _, block := range message.Content {
		switch block.Type {
		case agentcore.ContentText:
			converted.Content += block.Text
		case agentcore.ContentThinking:
			converted.ReasoningContent += block.Text
		case agentcore.ContentToolCall:
			if block.ToolCall == nil {
				continue
			}
			index := len(converted.ToolCalls)
			converted.ToolCalls = append(converted.ToolCalls, schema.ToolCall{
				Index: &index,
				ID:    block.ToolCall.ID,
				Type:  "function",
				Function: schema.FunctionCall{
					Name:      block.ToolCall.Name,
					Arguments: string(block.ToolCall.Arguments),
				},
			})
		default:
			return nil, fmt.Errorf("einoadapter: unsupported content type %q", block.Type)
		}
	}
	return converted, nil
}

func normalizeStopReason(reason string) agentcore.StopReason {
	switch strings.ToLower(reason) {
	case "":
		return ""
	case "stop", "end_turn", "stop_sequence":
		return agentcore.StopReasonStop
	case "tool_calls", "tool_use":
		return agentcore.StopReasonToolUse
	case "length", "max_tokens", "max_output_tokens":
		return agentcore.StopReasonLength
	case "error", "content_filter":
		return agentcore.StopReasonError
	default:
		return agentcore.StopReason(reason)
	}
}
