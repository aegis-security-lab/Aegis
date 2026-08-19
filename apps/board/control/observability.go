package control

import (
	"errors"
	"io"
	"log/slog"
	"strings"

	"aegis/observability"
	observabilitysqlite "aegis/observability/sqlitestore"
)

type SystemObservability struct {
	Logger   *observability.Logger
	Metrics  *observability.MemoryMetrics
	Logs     *observabilitysqlite.Handler
	Tracer   *observability.Tracer
	Redactor *observability.Redactor
}

func EnableObservability(store *Store, output io.Writer, levelName string) (*SystemObservability, error) {
	if store == nil || store.db == nil {
		return nil, errors.New("control observability: store is required")
	}
	level := parseLogLevel(levelName)
	levelVar := new(slog.LevelVar)
	levelVar.Set(level)
	persistent, err := observabilitysqlite.New(store.db, levelVar)
	if err != nil {
		return nil, err
	}
	handlers := []slog.Handler{persistent}
	if output != nil {
		handlers = append(handlers, slog.NewJSONHandler(output, &slog.HandlerOptions{Level: levelVar, AddSource: level <= slog.LevelDebug}))
	}
	redactor := observability.NewRedactor(store.Config().APIKey, store.Config().WebSearch.APIKey)
	logger := observability.New(observability.MultiHandler{Handlers: handlers}, redactor)
	metrics := observability.NewMemoryMetrics()
	result := &SystemObservability{Logger: logger, Metrics: metrics, Logs: persistent, Tracer: &observability.Tracer{Logger: logger, Metrics: metrics}, Redactor: redactor}
	observability.SetDefault(logger)
	observability.SetDefaultMetrics(metrics)
	store.observabilityMu.Lock()
	store.observability = result
	store.observabilityMu.Unlock()
	return result, nil
}

func (s *Store) Observability() *SystemObservability {
	if s == nil {
		return nil
	}
	s.observabilityMu.RLock()
	result := s.observability
	s.observabilityMu.RUnlock()
	return result
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
