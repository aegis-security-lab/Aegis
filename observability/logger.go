// Package observability provides structured logging, correlation context,
// redaction, metrics and lightweight tracing without tying core packages to a
// particular backend.
package observability

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
)

type Logger struct{ logger *slog.Logger }

type loggerBox struct{ logger *Logger }

var defaultLogger atomic.Pointer[loggerBox]

func init() {
	SetDefault(New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}), NewRedactor()))
}

func New(handler slog.Handler, redactor *Redactor) *Logger {
	if handler == nil {
		handler = discardHandler{}
	}
	if redactor != nil {
		handler = &redactingHandler{next: handler, redactor: redactor}
	}
	return &Logger{logger: slog.New(handler)}
}

func SetDefault(logger *Logger) {
	if logger == nil {
		logger = New(discardHandler{}, nil)
	}
	defaultLogger.Store(&loggerBox{logger: logger})
}

func Default() *Logger {
	if box := defaultLogger.Load(); box != nil && box.logger != nil {
		return box.logger
	}
	return New(discardHandler{}, nil)
}

func (l *Logger) With(attrs ...slog.Attr) *Logger {
	if l == nil || l.logger == nil {
		return Default().With(attrs...)
	}
	values := make([]any, len(attrs))
	for index := range attrs {
		values[index] = attrs[index]
	}
	return &Logger{logger: l.logger.With(values...)}
}

func (l *Logger) Log(ctx context.Context, level slog.Level, message string, attrs ...slog.Attr) {
	if l == nil || l.logger == nil {
		l = Default()
	}
	all := append(ScopeAttrs(ctx), attrs...)
	l.logger.LogAttrs(nonNil(ctx), level, message, all...)
}

func (l *Logger) Debug(ctx context.Context, message string, attrs ...slog.Attr) {
	l.Log(ctx, slog.LevelDebug, message, attrs...)
}
func (l *Logger) Info(ctx context.Context, message string, attrs ...slog.Attr) {
	l.Log(ctx, slog.LevelInfo, message, attrs...)
}
func (l *Logger) Warn(ctx context.Context, message string, attrs ...slog.Attr) {
	l.Log(ctx, slog.LevelWarn, message, attrs...)
}
func (l *Logger) Error(ctx context.Context, message string, attrs ...slog.Attr) {
	l.Log(ctx, slog.LevelError, message, attrs...)
}

func nonNil(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (discardHandler) WithAttrs([]slog.Attr) slog.Handler        { return discardHandler{} }
func (discardHandler) WithGroup(string) slog.Handler             { return discardHandler{} }
