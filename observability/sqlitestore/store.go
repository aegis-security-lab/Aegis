// Package sqlitestore persists structured observability records in SQLite.
package sqlitestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"aegis/observability"
	"gorm.io/gorm"
)

type Record struct {
	ID             string         `json:"id"`
	OccurredAt     time.Time      `json:"occurredAt"`
	Level          string         `json:"level"`
	Message        string         `json:"message"`
	Component      string         `json:"component,omitempty"`
	TraceID        string         `json:"traceId,omitempty"`
	SpanID         string         `json:"spanId,omitempty"`
	ParentSpanID   string         `json:"parentSpanId,omitempty"`
	RequestID      string         `json:"requestId,omitempty"`
	TaskID         string         `json:"taskId,omitempty"`
	IssueID        string         `json:"issueId,omitempty"`
	ExecutionID    string         `json:"executionId,omitempty"`
	AgentID        string         `json:"agentId,omitempty"`
	JobID          string         `json:"jobId,omitempty"`
	CoordinationID string         `json:"coordinationId,omitempty"`
	Attributes     map[string]any `json:"attributes,omitempty"`
}

type Filter struct {
	TaskID          string
	IssueIDs        []string
	ExecutionIDs    []string
	CoordinationID  string
	CoordinationIDs []string
	TraceID         string
	MinimumLevel    slog.Level
	From            *time.Time
	To              *time.Time
	Limit           int
	MatchAnyScope   bool
}

type recordRow struct {
	ID             string    `gorm:"primaryKey;size:64"`
	OccurredAt     time.Time `gorm:"not null;index:idx_observability_time"`
	Level          int       `gorm:"not null;index"`
	LevelName      string    `gorm:"size:16;not null"`
	Message        string    `gorm:"type:text;not null"`
	Component      string    `gorm:"size:128;index"`
	TraceID        string    `gorm:"size:64;index"`
	SpanID         string    `gorm:"size:32;index"`
	ParentSpanID   string    `gorm:"size:32"`
	RequestID      string    `gorm:"size:128;index"`
	TaskID         string    `gorm:"size:255;index"`
	IssueID        string    `gorm:"size:255;index"`
	ExecutionID    string    `gorm:"size:255;index"`
	AgentID        string    `gorm:"size:255;index"`
	JobID          string    `gorm:"size:255;index"`
	CoordinationID string    `gorm:"size:255;index"`
	Attributes     string    `gorm:"type:text;not null"`
}

func (recordRow) TableName() string { return "observability_logs" }

type Handler struct {
	db     *gorm.DB
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
}

func New(db *gorm.DB, level slog.Leveler) (*Handler, error) {
	if db == nil {
		return nil, errors.New("observability/sqlitestore: database is required")
	}
	if err := db.AutoMigrate(&recordRow{}); err != nil {
		return nil, fmt.Errorf("observability/sqlitestore: migrate: %w", err)
	}
	if level == nil {
		level = slog.LevelInfo
	}
	return &Handler{db: db, level: level}, nil
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return h != nil && h.db != nil && level >= h.level.Level()
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	attributes := map[string]any{}
	for _, attr := range h.attrs {
		addAttr(attributes, h.groups, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		addAttr(attributes, h.groups, attr)
		return true
	})
	encoded, err := json.Marshal(attributes)
	if err != nil {
		return fmt.Errorf("observability/sqlitestore: encode attributes: %w", err)
	}
	row := recordRow{
		ID: observability.NewID(16), OccurredAt: record.Time.UTC(), Level: int(record.Level), LevelName: record.Level.String(), Message: record.Message,
		Component: stringAttr(attributes, "component"), TraceID: stringAttr(attributes, "trace_id"), SpanID: stringAttr(attributes, "span_id"), ParentSpanID: stringAttr(attributes, "parent_span_id"),
		RequestID: stringAttr(attributes, "request_id"), TaskID: stringAttr(attributes, "task_id"), IssueID: stringAttr(attributes, "issue_id"), ExecutionID: stringAttr(attributes, "execution_id"),
		AgentID: stringAttr(attributes, "agent_id"), JobID: stringAttr(attributes, "job_id"), CoordinationID: stringAttr(attributes, "coordination_id"), Attributes: string(encoded),
	}
	if row.OccurredAt.IsZero() {
		row.OccurredAt = time.Now().UTC()
	}
	var writeErr error
	for attempt := 0; attempt < 4; attempt++ {
		writeErr = h.db.WithContext(nonNil(ctx)).Create(&row).Error
		if writeErr == nil || !strings.Contains(strings.ToLower(writeErr.Error()), "database is locked") {
			break
		}
		time.Sleep(time.Duration(25*(1<<attempt)) * time.Millisecond)
	}
	return writeErr
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	clone.groups = append([]string(nil), h.groups...)
	return &clone
}

func (h *Handler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.attrs = append([]slog.Attr(nil), h.attrs...)
	clone.groups = append(append([]string(nil), h.groups...), name)
	return &clone
}

func (h *Handler) Query(ctx context.Context, filter Filter) ([]Record, error) {
	if h == nil || h.db == nil {
		return nil, errors.New("observability/sqlitestore: nil handler")
	}
	query := h.db.WithContext(nonNil(ctx)).Model(&recordRow{})
	if filter.MinimumLevel != 0 {
		query = query.Where("level >= ?", int(filter.MinimumLevel))
	}
	if filter.From != nil {
		query = query.Where("occurred_at >= ?", filter.From.UTC())
	}
	if filter.To != nil {
		query = query.Where("occurred_at <= ?", filter.To.UTC())
	}
	if filter.TraceID != "" {
		query = query.Where("trace_id = ?", filter.TraceID)
	}
	coordinationIDs := append([]string(nil), filter.CoordinationIDs...)
	if filter.CoordinationID != "" {
		coordinationIDs = append(coordinationIDs, filter.CoordinationID)
	}
	if filter.MatchAnyScope && (filter.TaskID != "" || len(filter.IssueIDs) > 0 || len(filter.ExecutionIDs) > 0 || len(coordinationIDs) > 0) {
		condition := h.db.Where("1 = 0")
		if filter.TaskID != "" {
			condition = condition.Or("task_id = ?", filter.TaskID)
		}
		if len(filter.IssueIDs) > 0 {
			condition = condition.Or("issue_id IN ?", filter.IssueIDs)
		}
		if len(filter.ExecutionIDs) > 0 {
			condition = condition.Or("execution_id IN ?", filter.ExecutionIDs)
		}
		if len(coordinationIDs) > 0 {
			condition = condition.Or("coordination_id IN ?", coordinationIDs)
		}
		query = query.Where(condition)
	} else {
		if len(coordinationIDs) > 0 {
			query = query.Where("coordination_id IN ?", coordinationIDs)
		}
		if filter.TaskID != "" || len(filter.IssueIDs) > 0 || len(filter.ExecutionIDs) > 0 {
			condition := h.db.Where("1 = 0")
			if filter.TaskID != "" {
				condition = condition.Or("task_id = ?", filter.TaskID)
			}
			if len(filter.IssueIDs) > 0 {
				condition = condition.Or("issue_id IN ?", filter.IssueIDs)
			}
			if len(filter.ExecutionIDs) > 0 {
				condition = condition.Or("execution_id IN ?", filter.ExecutionIDs)
			}
			query = query.Where(condition)
		}
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100000 {
		limit = 100000
	}
	var rows []recordRow
	if err := query.Order("occurred_at asc, id asc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Record, len(rows))
	for index, row := range rows {
		attributes := map[string]any{}
		_ = json.Unmarshal([]byte(row.Attributes), &attributes)
		result[index] = Record{ID: row.ID, OccurredAt: row.OccurredAt, Level: row.LevelName, Message: row.Message, Component: row.Component, TraceID: row.TraceID, SpanID: row.SpanID, ParentSpanID: row.ParentSpanID, RequestID: row.RequestID, TaskID: row.TaskID, IssueID: row.IssueID, ExecutionID: row.ExecutionID, AgentID: row.AgentID, JobID: row.JobID, CoordinationID: row.CoordinationID, Attributes: attributes}
	}
	return result, nil
}

func addAttr(target map[string]any, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Key == "" {
		return
	}
	current := target
	for _, group := range groups {
		nested, _ := current[group].(map[string]any)
		if nested == nil {
			nested = map[string]any{}
			current[group] = nested
		}
		current = nested
	}
	if attr.Value.Kind() == slog.KindGroup {
		nested := map[string]any{}
		for _, child := range attr.Value.Group() {
			addAttr(nested, nil, child)
		}
		current[attr.Key] = nested
		return
	}
	current[attr.Key] = attr.Value.Any()
}

func stringAttr(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func nonNil(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

var _ slog.Handler = (*Handler)(nil)
