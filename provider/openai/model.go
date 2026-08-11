package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"

	"aegis/observability"
	"github.com/z3r2ne/agentcore"
	agentcoreopenai "github.com/z3r2ne/agentcore/provider/openai"
)

// Config is the upstream AgentCore OpenAI-compatible provider configuration.
// Keeping the alias preserves Aegis' public constructor while delegating wire
// compatibility, response limits, and provider-data round trips to AgentCore.
type Config = agentcoreopenai.Config

// Model adds Aegis observability around AgentCore's OpenAI-compatible model.
// AgentCore owns protocol translation so reasoning and provider-specific data
// survive repeated model -> tool -> model turns.
type Model struct {
	inner *agentcoreopenai.Model
	model string
}

const interruptedAssistantPlaceholder = "[response interrupted]"

type replaySanitization struct {
	repaired      int
	reasoningOnly int
	errors        int
}

func NewModel(config Config, model string) (*Model, error) {
	config.Model = model
	inner, err := agentcoreopenai.New(config)
	if err != nil {
		return nil, err
	}
	return &Model{inner: inner, model: model}, nil
}

func (m *Model) Stream(ctx context.Context, request agentcore.ModelRequest) (result agentcore.ModelStream, err error) {
	sanitizedMessages, sanitization := sanitizeReplayMessages(request.Messages)
	request.Messages = sanitizedMessages
	if sanitization.repaired > 0 {
		observability.Default().Warn(
			ctx,
			"provider.openai.history.sanitized",
			slog.String("model", m.model),
			slog.Int("repaired_message_count", sanitization.repaired),
			slog.Int("reasoning_only_message_count", sanitization.reasoningOnly),
			slog.Int("error_message_count", sanitization.errors),
		)
		observability.DefaultMetrics().AddCounter(
			"provider_history_messages_sanitized_total",
			float64(sanitization.repaired),
			observability.Labels{"provider": "openai", "model": m.model},
		)
	}
	ctx, span := (&observability.Tracer{Logger: observability.Default(), Metrics: observability.DefaultMetrics()}).Start(
		ctx,
		"provider.openai.stream",
		slog.String("model", m.model),
		slog.Int("message_count", len(request.Messages)),
		slog.Int("tool_count", len(request.Tools)),
	)
	defer func() {
		if err != nil {
			span.End(err, slog.String("provider", "openai"), slog.String("model", m.model))
			observability.DefaultMetrics().AddCounter("provider_requests_total", 1, observability.Labels{"provider": "openai", "model": m.model, "status": "error"})
		}
	}()
	inner, err := m.inner.Stream(ctx, request)
	if err != nil {
		return nil, err
	}
	return &observedStream{inner: inner, span: span, model: m.model}, nil
}

// sanitizeReplayMessages is the final wire-boundary defense for restored or
// externally supplied histories. OpenAI-compatible providers reject an
// assistant message whose serialized form has neither content nor tool calls.
// A zero-token failed stream or a reasoning-only completion can leave exactly
// that shape in a durable AgentCore Session and otherwise poison every later
// execution that restores it.
//
// The repair is intentionally applied to a detached request copy. It preserves
// hidden reasoning and vendor fields in ProviderData, adds only an honest
// interruption marker as visible content, and never rewrites a valid tool-call
// turn. The stored Session and UI transcript remain unchanged.
func sanitizeReplayMessages(messages []agentcore.Message) ([]agentcore.Message, replaySanitization) {
	var stats replaySanitization
	result := messages
	for index, message := range messages {
		if message.Role != agentcore.RoleAssistant {
			continue
		}

		neutralHasPayload := strings.TrimSpace(message.Text()) != "" || len(message.ToolCalls()) > 0
		preserved, hasPreserved := preservedOpenAIMessage(message)
		if hasPreserved {
			if openAIAssistantHasPayload(preserved) {
				continue
			}
		} else if neutralHasPayload {
			continue
		}

		if stats.repaired == 0 {
			result = append([]agentcore.Message(nil), messages...)
		}
		fixed := message
		fixed.Content = append([]agentcore.ContentBlock(nil), message.Content...)

		if neutralHasPayload {
			// ProviderData wins over neutral content in AgentCore's OpenAI
			// adapter. If those representations disagree, discard only the
			// stale provider copy so the valid neutral message is serialized.
			fixed.ProviderData = nil
		} else {
			fixed.Content = append(fixed.Content, agentcore.ContentBlock{
				Type: agentcore.ContentText,
				Text: interruptedAssistantPlaceholder,
			})
			if hasPreserved {
				preserved["content"] = interruptedAssistantPlaceholder
				fixed.ProviderData = repairedProviderData(message.ProviderData, preserved)
			} else {
				fixed.ProviderData = nil
			}
		}

		result[index] = fixed
		stats.repaired++
		if assistantHasThinking(message) && strings.TrimSpace(message.Text()) == "" && len(message.ToolCalls()) == 0 {
			stats.reasoningOnly++
		}
		if message.IsError || message.StopReason == agentcore.StopReasonError {
			stats.errors++
		}
	}
	return result, stats
}

func preservedOpenAIMessage(message agentcore.Message) (map[string]any, bool) {
	if message.ProviderData == nil || message.ProviderData.Format != agentcoreopenai.ProviderDataFormat {
		return nil, false
	}
	var preserved struct {
		Message map[string]any `json:"message"`
	}
	if json.Unmarshal(message.ProviderData.Data, &preserved) != nil || preserved.Message == nil {
		return nil, false
	}
	return preserved.Message, true
}

func openAIAssistantHasPayload(message map[string]any) bool {
	return nonEmptyWireValue(message["content"]) ||
		nonEmptyWireValue(message["tool_calls"]) ||
		nonEmptyWireValue(message["function_call"])
}

func nonEmptyWireValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func repairedProviderData(original *agentcore.ProviderData, message map[string]any) *agentcore.ProviderData {
	if original == nil {
		return nil
	}
	var preserved map[string]any
	if json.Unmarshal(original.Data, &preserved) != nil {
		return nil
	}
	preserved["message"] = message
	data, err := json.Marshal(preserved)
	if err != nil {
		return nil
	}
	return &agentcore.ProviderData{Format: original.Format, Data: data}
}

func assistantHasThinking(message agentcore.Message) bool {
	for _, block := range message.Content {
		if block.Type == agentcore.ContentThinking && strings.TrimSpace(block.Text) != "" {
			return true
		}
	}
	return false
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

var _ agentcore.Model = (*Model)(nil)
