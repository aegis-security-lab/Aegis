// Package sqlitestore persists application-facing Agent Work lifecycle events.
package sqlitestore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aegis/platform/work"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct{ db *gorm.DB }

type workRecord struct {
	WorkID         string    `gorm:"primaryKey;size:255"`
	RequestID      string    `gorm:"size:255;not null;uniqueIndex"`
	AppID          string    `gorm:"size:255;not null;uniqueIndex:idx_agent_work_idempotency,priority:1;index"`
	IdempotencyKey string    `gorm:"size:512;not null;uniqueIndex:idx_agent_work_idempotency,priority:2"`
	ScopeID        string    `gorm:"size:255;not null;index"`
	CorrelationID  string    `gorm:"size:255;not null;index"`
	Status         string    `gorm:"size:64;not null;index"`
	ExecutionID    string    `gorm:"size:255;index"`
	LastError      string    `gorm:"type:text"`
	Payload        string    `gorm:"type:text;not null"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (workRecord) TableName() string { return "agent_work_requests" }

type sequenceRecord struct {
	StreamKey string `gorm:"primaryKey;size:768"`
	Sequence  int64  `gorm:"not null"`
}

func (sequenceRecord) TableName() string { return "application_event_sequences" }

type eventRecord struct {
	EventID       string    `gorm:"primaryKey;size:255"`
	StreamKey     string    `gorm:"size:768;not null;uniqueIndex:idx_application_event_sequence,priority:1"`
	Sequence      int64     `gorm:"not null;uniqueIndex:idx_application_event_sequence,priority:2"`
	Type          string    `gorm:"size:128;not null;index"`
	OccurredAt    time.Time `gorm:"not null;index"`
	AppID         string    `gorm:"size:255;not null;index:idx_application_event_filter,priority:1"`
	TenantID      string    `gorm:"size:255;index:idx_application_event_filter,priority:2"`
	ScopeID       string    `gorm:"size:255;not null;index:idx_application_event_filter,priority:3"`
	CorrelationID string    `gorm:"size:255;not null;index"`
	RequestID     string    `gorm:"size:255;not null;index"`
	WorkID        string    `gorm:"size:255;not null;index"`
	ExecutionID   string    `gorm:"size:255;index"`
	AttemptID     string    `gorm:"size:255;index"`
	TraceID       string    `gorm:"size:255;index"`
	Payload       string    `gorm:"type:text;not null"`
}

func (eventRecord) TableName() string { return "application_events" }

type subscriptionRecord struct {
	ID        string    `gorm:"primaryKey;size:255"`
	AppID     string    `gorm:"size:255;not null;index"`
	TenantID  string    `gorm:"size:255;index"`
	ScopeID   string    `gorm:"size:255;not null;index"`
	Filter    string    `gorm:"type:text;not null"`
	Cursor    int64     `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (subscriptionRecord) TableName() string { return "application_event_subscriptions" }

func New(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("application event store: database is required")
	}
	if err := db.AutoMigrate(&workRecord{}, &sequenceRecord{}, &eventRecord{}, &subscriptionRecord{}); err != nil {
		return nil, fmt.Errorf("application event store: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) RegisterSubscription(ctx context.Context, subscription work.Subscription) error {
	if err := subscription.Validate(); err != nil {
		return err
	}
	filter, err := json.Marshal(subscription.Filter)
	if err != nil {
		return fmt.Errorf("application inbox: encode filter: %w", err)
	}
	record := subscriptionRecord{
		ID: subscription.ID, AppID: subscription.Filter.AppID, TenantID: subscription.Filter.TenantID, ScopeID: subscription.Filter.ScopeID,
		Filter: string(filter), Cursor: subscription.Cursor.Sequence, CreatedAt: subscription.CreatedAt.UTC(), UpdatedAt: subscription.UpdatedAt.UTC(),
	}
	result := s.db.WithContext(nonNil(ctx)).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return fmt.Errorf("application inbox: register subscription: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	existing, err := s.Subscription(ctx, subscription.ID)
	if err != nil {
		return err
	}
	if existing.Filter != subscription.Filter {
		return errors.New("application inbox: subscription ID is already registered with another filter")
	}
	return nil
}

func (s *Store) Subscription(ctx context.Context, id string) (work.Subscription, error) {
	var record subscriptionRecord
	err := s.db.WithContext(nonNil(ctx)).First(&record, "id = ?", strings.TrimSpace(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return work.Subscription{}, work.ErrNotFound
	}
	if err != nil {
		return work.Subscription{}, fmt.Errorf("application inbox: get subscription: %w", err)
	}
	var filter work.EventFilter
	if err := json.Unmarshal([]byte(record.Filter), &filter); err != nil {
		return work.Subscription{}, fmt.Errorf("application inbox: decode subscription filter: %w", err)
	}
	return work.Subscription{ID: record.ID, Filter: filter, Cursor: work.Cursor{Sequence: record.Cursor}, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

func (s *Store) AckSubscription(ctx context.Context, id string, cursor work.Cursor, at time.Time) error {
	if cursor.Sequence < 0 {
		return errors.New("application inbox: cursor cannot be negative")
	}
	result := s.db.WithContext(nonNil(ctx)).Model(&subscriptionRecord{}).
		Where("id = ? AND cursor < ?", strings.TrimSpace(id), cursor.Sequence).
		Updates(map[string]any{"cursor": cursor.Sequence, "updated_at": at.UTC()})
	if result.Error != nil {
		return fmt.Errorf("application inbox: acknowledge: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var count int64
	if err := s.db.WithContext(nonNil(ctx)).Model(&subscriptionRecord{}).Where("id = ?", strings.TrimSpace(id)).Count(&count).Error; err != nil {
		return fmt.Errorf("application inbox: acknowledge lookup: %w", err)
	}
	if count == 0 {
		return work.ErrNotFound
	}
	return nil
}

func (s *Store) Create(ctx context.Context, record work.Record) error {
	encoded, err := encodeWork(record)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(nonNil(ctx)).Create(&encoded).Error; err != nil {
		return fmt.Errorf("application work store: create: %w", err)
	}
	return nil
}

func (s *Store) ByIdempotencyKey(ctx context.Context, appID, key string) (work.Record, bool, error) {
	var record workRecord
	err := s.db.WithContext(nonNil(ctx)).Where("app_id = ? AND idempotency_key = ?", strings.TrimSpace(appID), strings.TrimSpace(key)).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return work.Record{}, false, nil
	}
	if err != nil {
		return work.Record{}, false, fmt.Errorf("application work store: find idempotency key: %w", err)
	}
	decoded, err := decodeWork(record)
	return decoded, true, err
}

func (s *Store) Get(ctx context.Context, workID string) (work.Record, error) {
	var record workRecord
	err := s.db.WithContext(nonNil(ctx)).First(&record, "work_id = ?", strings.TrimSpace(workID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return work.Record{}, work.ErrNotFound
	}
	if err != nil {
		return work.Record{}, fmt.Errorf("application work store: get: %w", err)
	}
	return decodeWork(record)
}

func (s *Store) Update(ctx context.Context, record work.Record) error {
	encoded, err := encodeWork(record)
	if err != nil {
		return err
	}
	result := s.db.WithContext(nonNil(ctx)).Model(&workRecord{}).Where("work_id = ?", record.WorkID).Updates(map[string]any{
		"status": record.Status, "execution_id": record.ExecutionID, "last_error": record.LastError,
		"payload": encoded.Payload, "updated_at": encoded.UpdatedAt,
	})
	if result.Error != nil {
		return fmt.Errorf("application work store: update: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return work.ErrNotFound
	}
	return nil
}

// Publish allocates a monotonically increasing sequence per app/tenant/scope
// stream in the same transaction that saves the event. Re-publishing the same
// EventID returns the original event and does not allocate another sequence.
func (s *Store) Publish(ctx context.Context, event work.Event) (work.Event, error) {
	if s == nil || s.db == nil {
		return work.Event{}, errors.New("application event store: unavailable")
	}
	event.AppID, event.TenantID, event.ScopeID = strings.TrimSpace(event.AppID), strings.TrimSpace(event.TenantID), strings.TrimSpace(event.ScopeID)
	event.CorrelationID, event.RequestID, event.WorkID = strings.TrimSpace(event.CorrelationID), strings.TrimSpace(event.RequestID), strings.TrimSpace(event.WorkID)
	if event.AppID == "" || event.ScopeID == "" || event.CorrelationID == "" || event.RequestID == "" || event.WorkID == "" || strings.TrimSpace(string(event.Type)) == "" {
		return work.Event{}, errors.New("application event store: type and correlation envelope are required")
	}
	if len(event.Payload) > 0 && !json.Valid(event.Payload) {
		return work.Event{}, errors.New("application event store: payload must be valid JSON")
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = newID("app-event")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	} else {
		event.OccurredAt = event.OccurredAt.UTC()
	}
	key := streamKey(event.AppID, event.TenantID, event.ScopeID)
	err := s.db.WithContext(nonNil(ctx)).Transaction(func(tx *gorm.DB) error {
		var existing eventRecord
		if err := tx.First(&existing, "event_id = ?", event.EventID).Error; err == nil {
			decoded, decodeErr := decode(existing)
			if decodeErr != nil {
				return decodeErr
			}
			event = decoded
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var sequence sequenceRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sequence, "stream_key = ?", key).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			sequence = sequenceRecord{StreamKey: key, Sequence: 1}
			if err := tx.Create(&sequence).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			sequence.Sequence++
			if err := tx.Model(&sequenceRecord{}).Where("stream_key = ?", key).Update("sequence", sequence.Sequence).Error; err != nil {
				return err
			}
		}
		event.Sequence = sequence.Sequence
		if err := event.Validate(); err != nil {
			return err
		}
		return tx.Create(encode(key, event)).Error
	})
	if err != nil {
		return work.Event{}, fmt.Errorf("application event store: publish: %w", err)
	}
	return event, nil
}

func (s *Store) Replay(ctx context.Context, filter work.EventFilter, cursor work.Cursor, limit int) ([]work.Event, work.Cursor, error) {
	if s == nil || s.db == nil {
		return nil, cursor, errors.New("application event store: unavailable")
	}
	filter.AppID, filter.TenantID, filter.ScopeID = strings.TrimSpace(filter.AppID), strings.TrimSpace(filter.TenantID), strings.TrimSpace(filter.ScopeID)
	if filter.AppID == "" || filter.ScopeID == "" {
		return nil, cursor, errors.New("application event store: app and scope filters are required")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	query := s.db.WithContext(nonNil(ctx)).Where("stream_key = ? AND sequence > ?", streamKey(filter.AppID, filter.TenantID, filter.ScopeID), cursor.Sequence)
	if value := strings.TrimSpace(filter.CorrelationID); value != "" {
		query = query.Where("correlation_id = ?", value)
	}
	if value := strings.TrimSpace(filter.RequestID); value != "" {
		query = query.Where("request_id = ?", value)
	}
	if value := strings.TrimSpace(filter.WorkID); value != "" {
		query = query.Where("work_id = ?", value)
	}
	var records []eventRecord
	if err := query.Order("sequence ASC").Limit(limit).Find(&records).Error; err != nil {
		return nil, cursor, fmt.Errorf("application event store: replay: %w", err)
	}
	result := make([]work.Event, len(records))
	next := cursor
	for index, record := range records {
		event, err := decode(record)
		if err != nil {
			return nil, cursor, err
		}
		result[index] = event
		next.Sequence = event.Sequence
	}
	return result, next, nil
}

func encode(key string, event work.Event) eventRecord {
	payload := string(event.Payload)
	if payload == "" {
		payload = "null"
	}
	return eventRecord{
		EventID: event.EventID, StreamKey: key, Sequence: event.Sequence, Type: string(event.Type), OccurredAt: event.OccurredAt,
		AppID: event.AppID, TenantID: event.TenantID, ScopeID: event.ScopeID, CorrelationID: event.CorrelationID,
		RequestID: event.RequestID, WorkID: event.WorkID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID,
		TraceID: event.TraceID, Payload: payload,
	}
}

func encodeWork(record work.Record) (workRecord, error) {
	if strings.TrimSpace(record.RequestID) == "" || strings.TrimSpace(record.WorkID) == "" || strings.TrimSpace(record.Status) == "" {
		return workRecord{}, errors.New("application work store: request, work and status are required")
	}
	if err := record.Request.Validate(); err != nil {
		return workRecord{}, err
	}
	payload, err := json.Marshal(record.Request)
	if err != nil {
		return workRecord{}, fmt.Errorf("application work store: encode request: %w", err)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	return workRecord{
		WorkID: record.WorkID, RequestID: record.RequestID, AppID: record.Request.AppID, IdempotencyKey: record.Request.IdempotencyKey,
		ScopeID: record.Request.ScopeID, CorrelationID: record.Request.CorrelationID, Status: record.Status,
		ExecutionID: record.ExecutionID, LastError: record.LastError, Payload: string(payload),
		CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
	}, nil
}

func decodeWork(record workRecord) (work.Record, error) {
	var request work.Request
	if err := json.Unmarshal([]byte(record.Payload), &request); err != nil {
		return work.Record{}, fmt.Errorf("application work store: decode request %s: %w", record.WorkID, err)
	}
	return work.Record{
		RequestID: record.RequestID, WorkID: record.WorkID, Request: request, Status: record.Status,
		ExecutionID: record.ExecutionID, LastError: record.LastError, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

func decode(record eventRecord) (work.Event, error) {
	event := work.Event{
		EventID: record.EventID, Sequence: record.Sequence, Type: work.EventType(record.Type), OccurredAt: record.OccurredAt,
		AppID: record.AppID, TenantID: record.TenantID, ScopeID: record.ScopeID, CorrelationID: record.CorrelationID,
		RequestID: record.RequestID, WorkID: record.WorkID, ExecutionID: record.ExecutionID, AttemptID: record.AttemptID,
		TraceID: record.TraceID, Payload: json.RawMessage(record.Payload),
	}
	if err := event.Validate(); err != nil {
		return work.Event{}, fmt.Errorf("application event store: decode %s: %w", record.EventID, err)
	}
	return event, nil
}

func streamKey(appID, tenantID, scopeID string) string {
	return strings.TrimSpace(appID) + "\x00" + strings.TrimSpace(tenantID) + "\x00" + strings.TrimSpace(scopeID)
}

func newID(prefix string) string {
	var data [16]byte
	_, _ = rand.Read(data[:])
	return prefix + "-" + hex.EncodeToString(data[:])
}

func nonNil(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

var _ work.EventStore = (*Store)(nil)
var _ work.Repository = (*Store)(nil)
var _ work.SubscriptionRepository = (*Store)(nil)
