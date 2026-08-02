package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggerPropagatesScopeAndRedactsSecrets(t *testing.T) {
	var output bytes.Buffer
	logger := New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}), NewRedactor("known-secret"))
	ctx := WithScope(context.Background(), Scope{TraceID: "trace-1", TaskID: "task-1", ExecutionID: "exec-1", Component: "test"})
	logger.Info(ctx, "request Bearer abc.def", slog.String("api_key", "raw-key"), slog.String("detail", "known-secret and sk-abcdefghijklmnop"), slog.Int64("input_tokens", 42))
	text := output.String()
	for _, expected := range []string{`"trace_id":"trace-1"`, `"task_id":"task-1"`, `"execution_id":"exec-1"`, `"api_key":"[REDACTED]"`, `"input_tokens":42`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s in %s", expected, text)
		}
	}
	for _, forbidden := range []string{"abc.def", "raw-key", "known-secret", "sk-abcdefghijklmnop"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("secret %q leaked in %s", forbidden, text)
		}
	}
}

func TestMemoryMetricsProducesDeterministicKeys(t *testing.T) {
	metrics := NewMemoryMetrics()
	metrics.AddCounter("requests", 1, Labels{"status": "ok", "method": "GET"})
	metrics.ObserveHistogram("latency", 5, Labels{"route": "/api"})
	metrics.ObserveHistogram("latency", 10, Labels{"route": "/api"})
	snapshot := metrics.Snapshot()
	if snapshot.Counters["requests{method=GET,status=ok}"] != 1 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	distribution := snapshot.Histograms["latency{route=/api}"]
	if distribution.Count != 2 || distribution.Sum != 15 || distribution.Min != 5 || distribution.Max != 10 {
		t.Fatalf("distribution=%+v", distribution)
	}
}
