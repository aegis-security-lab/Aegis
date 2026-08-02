package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"aegis/observability"
	"github.com/z3r2ne/agentcore"
)

type Config struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Headers    map[string]string
}

type Model struct {
	endpoint string
	apiKey   string
	client   *http.Client
	headers  map[string]string
	model    string
}

func NewModel(config Config, model string) (*Model, error) {
	endpoint, err := chatCompletionsEndpoint(config.BaseURL)
	if err != nil {
		return nil, err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, errors.New("provider/openai: model is required")
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	headers := make(map[string]string, len(config.Headers))
	for key, value := range config.Headers {
		if strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "Content-Type") {
			continue
		}
		headers[key] = value
	}
	return &Model{endpoint: endpoint, apiKey: strings.TrimSpace(config.APIKey), client: client, headers: headers, model: model}, nil
}

func (m *Model) Stream(ctx context.Context, request agentcore.ModelRequest) (result agentcore.ModelStream, err error) {
	ctx, span := (&observability.Tracer{Logger: observability.Default(), Metrics: observability.DefaultMetrics()}).Start(ctx, "provider.openai.stream", slog.String("model", m.model), slog.Int("message_count", len(request.Messages)), slog.Int("tool_count", len(request.Tools)))
	defer func() {
		if err != nil {
			span.End(err, slog.String("provider", "openai"), slog.String("model", m.model))
			observability.DefaultMetrics().AddCounter("provider_requests_total", 1, observability.Labels{"provider": "openai", "model": m.model, "status": "error"})
		}
	}()
	body, err := m.requestBody(request)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("provider/openai: encode request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("provider/openai: create request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	if m.apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+m.apiKey)
	}
	for key, value := range m.headers {
		httpRequest.Header.Set(key, value)
	}
	response, err := m.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("provider/openai: request: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return nil, fmt.Errorf("provider/openai: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	return &observedStream{inner: newStream(response.Body), span: span, model: m.model}, nil
}

type observedStream struct {
	inner agentcore.ModelStream
	span  *observability.Span
	model string
	usage agentcore.Usage
	once  sync.Once
	done  bool
}

func (s *observedStream) Recv() (agentcore.ModelChunk, error) {
	chunk, err := s.inner.Recv()
	if chunk.Usage != nil {
		s.usage = *chunk.Usage
	}
	if errors.Is(err, io.EOF) {
		s.done = true
		s.finish(nil, false)
	} else if err != nil {
		s.done = true
		s.finish(err, false)
	}
	return chunk, err
}

func (s *observedStream) Close() error {
	err := s.inner.Close()
	s.finish(err, !s.done)
	return err
}

func (s *observedStream) finish(err error, incomplete bool) {
	s.once.Do(func() {
		status := "ok"
		if err != nil {
			status = "error"
		} else if incomplete {
			status = "closed"
		}
		attrs := []slog.Attr{
			slog.String("provider", "openai"), slog.String("model", s.model), slog.String("stream_status", status), slog.Bool("incomplete", incomplete),
			slog.Int("input_tokens", s.usage.InputTokens), slog.Int("output_tokens", s.usage.OutputTokens), slog.Int("cache_read_tokens", s.usage.CacheReadTokens), slog.Int("cache_write_tokens", s.usage.CacheWriteTokens),
		}
		s.span.End(err, attrs...)
		metrics := observability.DefaultMetrics()
		labels := observability.Labels{"provider": "openai", "model": s.model, "status": status}
		metrics.AddCounter("provider_requests_total", 1, labels)
		metrics.AddCounter("provider_input_tokens_total", float64(s.usage.InputTokens), observability.Labels{"provider": "openai", "model": s.model})
		metrics.AddCounter("provider_output_tokens_total", float64(s.usage.OutputTokens), observability.Labels{"provider": "openai", "model": s.model})
		metrics.AddCounter("provider_cache_read_tokens_total", float64(s.usage.CacheReadTokens), observability.Labels{"provider": "openai", "model": s.model})
		metrics.AddCounter("provider_cache_write_tokens_total", float64(s.usage.CacheWriteTokens), observability.Labels{"provider": "openai", "model": s.model})
	})
}

func (m *Model) requestBody(request agentcore.ModelRequest) (map[string]any, error) {
	messages := make([]map[string]any, 0, len(request.Messages)+1)
	if strings.TrimSpace(request.SystemPrompt) != "" {
		messages = append(messages, map[string]any{"role": "system", "content": request.SystemPrompt})
	}
	for _, message := range request.Messages {
		converted, err := convertMessage(message)
		if err != nil {
			return nil, err
		}
		messages = append(messages, converted)
	}
	body := map[string]any{"model": m.model, "messages": messages, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	if len(request.Tools) > 0 {
		tools := make([]map[string]any, len(request.Tools))
		for index, definition := range request.Tools {
			parameters := any(map[string]any{"type": "object"})
			if len(definition.Parameters) > 0 {
				decoder := json.NewDecoder(bytes.NewReader(definition.Parameters))
				decoder.UseNumber()
				if err := decoder.Decode(&parameters); err != nil {
					return nil, fmt.Errorf("provider/openai: invalid schema for tool %s: %w", definition.Name, err)
				}
			}
			tools[index] = map[string]any{"type": "function", "function": map[string]any{"name": definition.Name, "description": definition.Description, "parameters": parameters}}
		}
		body["tools"] = tools
	}
	for key, value := range request.Options {
		switch key {
		case "model", "messages", "tools", "stream", "stream_options":
			continue
		default:
			body[key] = value
		}
	}
	return body, nil
}

func convertMessage(message agentcore.Message) (map[string]any, error) {
	result := map[string]any{"role": string(message.Role)}
	switch message.Role {
	case agentcore.RoleTool:
		result["content"] = message.Text()
		result["tool_call_id"] = message.ToolCallID
		return result, nil
	case agentcore.RoleAssistant:
		result["content"] = message.Text()
		calls := message.ToolCalls()
		if len(calls) > 0 {
			converted := make([]map[string]any, len(calls))
			for index, call := range calls {
				converted[index] = map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": string(call.Arguments)}}
			}
			result["tool_calls"] = converted
		}
		return result, nil
	case agentcore.RoleSystem, agentcore.RoleUser:
		content, err := convertContent(message.Content)
		if err != nil {
			return nil, err
		}
		result["content"] = content
		return result, nil
	default:
		return nil, fmt.Errorf("provider/openai: unsupported message role %q", message.Role)
	}
}

func convertContent(blocks []agentcore.ContentBlock) (any, error) {
	if len(blocks) == 0 {
		return "", nil
	}
	if len(blocks) == 1 && blocks[0].Type == agentcore.ContentText {
		return blocks[0].Text, nil
	}
	result := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case agentcore.ContentText:
			result = append(result, map[string]any{"type": "text", "text": block.Text})
		case agentcore.ContentImage:
			if block.URL == "" {
				return nil, errors.New("provider/openai: inline image data is not supported")
			}
			result = append(result, map[string]any{"type": "image_url", "image_url": map[string]any{"url": block.URL}})
		default:
			return nil, fmt.Errorf("provider/openai: unsupported user content type %q", block.Type)
		}
	}
	return result, nil
}

func chatCompletionsEndpoint(base string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("provider/openai: valid HTTP(S) base URL is required")
	}
	if strings.HasSuffix(parsed.Path, "/chat/completions") {
		return parsed.String(), nil
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/chat/completions"
	return parsed.String(), nil
}

type stream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	done    bool
}

func newStream(body io.ReadCloser) *stream {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	return &stream{body: body, scanner: scanner}
}

type streamEnvelope struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		PromptDetails    struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *stream) Recv() (agentcore.ModelChunk, error) {
	if s.done {
		return agentcore.ModelChunk{}, io.EOF
	}
	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			s.done = true
			return agentcore.ModelChunk{}, io.EOF
		}
		var envelope streamEnvelope
		if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
			return agentcore.ModelChunk{}, fmt.Errorf("provider/openai: decode stream event: %w", err)
		}
		if envelope.Error != nil {
			return agentcore.ModelChunk{}, errors.New("provider/openai: " + envelope.Error.Message)
		}
		usage := agentcore.Usage{
			InputTokens: envelope.Usage.PromptTokens, OutputTokens: envelope.Usage.CompletionTokens,
			CacheReadTokens: envelope.Usage.PromptDetails.CachedTokens,
		}
		chunk := agentcore.ModelChunk{}
		if usage != (agentcore.Usage{}) {
			chunk.Usage = &usage
		}
		if len(envelope.Choices) > 0 {
			choice := envelope.Choices[0]
			chunk.TextDelta, chunk.ThinkingDelta = choice.Delta.Content, choice.Delta.ReasoningContent
			chunk.ToolCallDeltas = make([]agentcore.ToolCallDelta, len(choice.Delta.ToolCalls))
			for index, call := range choice.Delta.ToolCalls {
				chunk.ToolCallDeltas[index] = agentcore.ToolCallDelta{Index: call.Index, ID: call.ID, Name: call.Function.Name, ArgumentsDelta: call.Function.Arguments}
			}
			if choice.FinishReason != nil {
				chunk.StopReason = stopReason(*choice.FinishReason)
			}
		}
		return chunk, nil
	}
	s.done = true
	if err := s.scanner.Err(); err != nil {
		return agentcore.ModelChunk{}, fmt.Errorf("provider/openai: read stream: %w", err)
	}
	return agentcore.ModelChunk{}, io.EOF
}

func (s *stream) Close() error {
	if s == nil || s.body == nil {
		return nil
	}
	s.done = true
	err := s.body.Close()
	s.body = nil
	return err
}

func stopReason(reason string) agentcore.StopReason {
	switch reason {
	case "stop":
		return agentcore.StopReasonStop
	case "tool_calls", "function_call":
		return agentcore.StopReasonToolUse
	case "length":
		return agentcore.StopReasonLength
	case "content_filter":
		return agentcore.StopReasonError
	default:
		return agentcore.StopReason(reason)
	}
}

var _ agentcore.Model = (*Model)(nil)
