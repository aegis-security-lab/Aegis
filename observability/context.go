package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
)

// Scope is the correlation identity propagated across HTTP, task, Coordination,
// coordination, Agent and tool boundaries. Empty fields are omitted.
type Scope struct {
	TraceID        string `json:"traceId,omitempty"`
	SpanID         string `json:"spanId,omitempty"`
	ParentSpanID   string `json:"parentSpanId,omitempty"`
	RequestID      string `json:"requestId,omitempty"`
	TaskID         string `json:"taskId,omitempty"`
	IssueID        string `json:"issueId,omitempty"`
	ExecutionID    string `json:"executionId,omitempty"`
	AgentID        string `json:"agentId,omitempty"`
	JobID          string `json:"jobId,omitempty"`
	CoordinationID string `json:"coordinationId,omitempty"`
	Component      string `json:"component,omitempty"`
}

type scopeKey struct{}

func WithScope(ctx context.Context, update Scope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	current, _ := ScopeFromContext(ctx)
	merged := mergeScope(current, update)
	if merged.TraceID == "" {
		merged.TraceID = NewID(16)
	}
	return context.WithValue(ctx, scopeKey{}, merged)
}

func ScopeFromContext(ctx context.Context) (Scope, bool) {
	if ctx == nil {
		return Scope{}, false
	}
	scope, ok := ctx.Value(scopeKey{}).(Scope)
	return scope, ok
}

func ScopeAttrs(ctx context.Context) []slog.Attr {
	scope, ok := ScopeFromContext(ctx)
	if !ok {
		return nil
	}
	values := []struct{ key, value string }{
		{"trace_id", scope.TraceID}, {"span_id", scope.SpanID}, {"parent_span_id", scope.ParentSpanID},
		{"request_id", scope.RequestID}, {"task_id", scope.TaskID}, {"issue_id", scope.IssueID},
		{"execution_id", scope.ExecutionID}, {"agent_id", scope.AgentID}, {"job_id", scope.JobID},
		{"coordination_id", scope.CoordinationID}, {"component", scope.Component},
	}
	attrs := make([]slog.Attr, 0, len(values))
	for _, value := range values {
		if value.value != "" {
			attrs = append(attrs, slog.String(value.key, value.value))
		}
	}
	return attrs
}

func NewID(bytes int) string {
	if bytes <= 0 {
		bytes = 16
	}
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	return hex.EncodeToString(value)
}

func mergeScope(base, update Scope) Scope {
	result := base
	fields := []*string{&result.TraceID, &result.SpanID, &result.ParentSpanID, &result.RequestID, &result.TaskID, &result.IssueID, &result.ExecutionID, &result.AgentID, &result.JobID, &result.CoordinationID, &result.Component}
	updates := []string{update.TraceID, update.SpanID, update.ParentSpanID, update.RequestID, update.TaskID, update.IssueID, update.ExecutionID, update.AgentID, update.JobID, update.CoordinationID, update.Component}
	for index, value := range updates {
		if strings.TrimSpace(value) != "" {
			*fields[index] = strings.TrimSpace(value)
		}
	}
	return result
}
