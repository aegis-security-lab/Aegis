package observability

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"sync"
)

type MultiHandler struct{ Handlers []slog.Handler }

func (h MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.Handlers {
		if handler != nil && handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	var result error
	for _, handler := range h.Handlers {
		if handler != nil && handler.Enabled(ctx, record.Level) {
			result = errors.Join(result, handler.Handle(ctx, record.Clone()))
		}
	}
	return result
}

func (h MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	result := MultiHandler{Handlers: make([]slog.Handler, len(h.Handlers))}
	for index, handler := range h.Handlers {
		if handler != nil {
			result.Handlers[index] = handler.WithAttrs(attrs)
		}
	}
	return result
}

func (h MultiHandler) WithGroup(name string) slog.Handler {
	result := MultiHandler{Handlers: make([]slog.Handler, len(h.Handlers))}
	for index, handler := range h.Handlers {
		if handler != nil {
			result.Handlers[index] = handler.WithGroup(name)
		}
	}
	return result
}

type Redactor struct {
	mu       sync.RWMutex
	secrets  []string
	patterns []*regexp.Regexp
}

func NewRedactor(secrets ...string) *Redactor {
	return &Redactor{secrets: cleanSecrets(secrets), patterns: []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`),
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{12,}\b`),
		regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|password|secret)\s*[:=]\s*[^\s,;]+`),
	}}
}

func (r *Redactor) RegisterSecret(secret string) {
	secret = strings.TrimSpace(secret)
	if r == nil || len(secret) < 4 {
		return
	}
	r.mu.Lock()
	r.secrets = append(r.secrets, secret)
	r.mu.Unlock()
}

func (r *Redactor) String(key, value string) string {
	if sensitiveKey(key) {
		return "[REDACTED]"
	}
	if r == nil {
		return value
	}
	r.mu.RLock()
	for _, secret := range r.secrets {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	patterns := append([]*regexp.Regexp(nil), r.patterns...)
	r.mu.RUnlock()
	for _, pattern := range patterns {
		value = pattern.ReplaceAllString(value, "[REDACTED]")
	}
	return value
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(key, "-", "_")))
	if key == "authorization" || key == "cookie" || key == "set_cookie" || key == "password" || key == "secret" || key == "credential" || key == "private_key" || key == "api_key" {
		return true
	}
	return key == "token" || strings.HasSuffix(key, "_token") || strings.HasSuffix(key, "_secret") || strings.HasSuffix(key, "_password") || strings.HasSuffix(key, "_api_key")
}

type redactingHandler struct {
	next     slog.Handler
	redactor *Redactor
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	copy := slog.NewRecord(record.Time, record.Level, h.redactor.String("message", record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		copy.AddAttrs(redactAttr(h.redactor, attr))
		return true
	})
	return h.next.Handle(ctx, copy)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for index, attr := range attrs {
		redacted[index] = redactAttr(h.redactor, attr)
	}
	return &redactingHandler{next: h.next.WithAttrs(redacted), redactor: h.redactor}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name), redactor: h.redactor}
}

func redactAttr(redactor *Redactor, attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if sensitiveKey(attr.Key) {
		return slog.String(attr.Key, "[REDACTED]")
	}
	switch attr.Value.Kind() {
	case slog.KindString:
		return slog.String(attr.Key, redactor.String(attr.Key, attr.Value.String()))
	case slog.KindGroup:
		children := attr.Value.Group()
		for index := range children {
			children[index] = redactAttr(redactor, children[index])
		}
		return slog.Group(attr.Key, attrsToAny(children)...)
	case slog.KindAny:
		return slog.Any(attr.Key, redactAny(redactor, attr.Key, attr.Value.Any(), 0))
	default:
		return attr
	}
}

func redactAny(redactor *Redactor, key string, value any, depth int) any {
	if sensitiveKey(key) {
		return "[REDACTED]"
	}
	if depth > 8 || value == nil {
		return value
	}
	if text, ok := value.(string); ok {
		return redactor.String(key, text)
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		return redactAny(redactor, key, rv.Elem().Interface(), depth+1)
	}
	switch rv.Kind() {
	case reflect.Map:
		result := map[string]any{}
		iter := rv.MapRange()
		for iter.Next() {
			childKey := strings.TrimSpace(toString(iter.Key().Interface()))
			result[childKey] = redactAny(redactor, childKey, iter.Value().Interface(), depth+1)
		}
		return result
	case reflect.Slice, reflect.Array:
		result := make([]any, rv.Len())
		for index := 0; index < rv.Len(); index++ {
			result[index] = redactAny(redactor, key, rv.Index(index).Interface(), depth+1)
		}
		return result
	default:
		return value
	}
}

func attrsToAny(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for index := range attrs {
		result[index] = attrs[index]
	}
	return result
}

func toString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return "field"
}

func cleanSecrets(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); len(value) >= 4 {
			result = append(result, value)
		}
	}
	return result
}
