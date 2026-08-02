package observability

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Tracer struct {
	Logger  *Logger
	Metrics Metrics
	Now     func() time.Time
}

type Span struct {
	tracer *Tracer
	ctx    context.Context
	name   string
	start  time.Time
	once   sync.Once
}

func (t *Tracer) Start(ctx context.Context, name string, attrs ...slog.Attr) (context.Context, *Span) {
	if t == nil {
		t = &Tracer{Logger: Default()}
	}
	if t.Logger == nil {
		t.Logger = Default()
	}
	scope, _ := ScopeFromContext(ctx)
	parent := scope.SpanID
	scope.ParentSpanID, scope.SpanID = parent, NewID(8)
	if scope.TraceID == "" {
		scope.TraceID = NewID(16)
	}
	ctx = WithScope(ctx, scope)
	span := &Span{tracer: t, ctx: ctx, name: name, start: t.now()}
	t.Logger.Debug(ctx, "span.start", append([]slog.Attr{slog.String("span_name", name)}, attrs...)...)
	if t.Metrics != nil {
		t.Metrics.AddCounter("spans_started_total", 1, Labels{"name": name})
	}
	return ctx, span
}

func (s *Span) End(err error, attrs ...slog.Attr) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		duration := s.tracer.now().Sub(s.start)
		base := []slog.Attr{slog.String("span_name", s.name), slog.Float64("duration_ms", float64(duration.Microseconds())/1000)}
		base = append(base, attrs...)
		status := "ok"
		if err != nil {
			status = "error"
			base = append(base, slog.String("error", err.Error()))
			s.tracer.Logger.Error(s.ctx, "span.end", append(base, slog.String("status", status))...)
		} else {
			s.tracer.Logger.Info(s.ctx, "span.end", append(base, slog.String("status", status))...)
		}
		if s.tracer.Metrics != nil {
			s.tracer.Metrics.ObserveHistogram("span_duration_ms", float64(duration.Microseconds())/1000, Labels{"name": s.name, "status": status})
		}
	})
}

func (t *Tracer) now() time.Time {
	if t.Now != nil {
		return t.Now().UTC()
	}
	return time.Now().UTC()
}
