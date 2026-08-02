package sqlitestore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"aegis/coordination"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var memorySequence atomic.Uint64

type Store struct {
	db    *gorm.DB
	owned bool
}

type bindingRecord struct {
	CoordinationID string    `gorm:"primaryKey;size:255"`
	Mode           string    `gorm:"size:128;not null"`
	Version        string    `gorm:"size:64;not null"`
	Config         string    `gorm:"type:text;not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (bindingRecord) TableName() string { return "coordination_bindings" }

type eventRecord struct {
	ID             string     `gorm:"primaryKey;size:255"`
	CoordinationID string     `gorm:"size:255;not null;index"`
	Status         string     `gorm:"size:32;not null;index:idx_coordination_event_claim,priority:1"`
	AvailableAt    time.Time  `gorm:"not null;index:idx_coordination_event_claim,priority:2"`
	CreatedAt      time.Time  `gorm:"not null;index:idx_coordination_event_claim,priority:3"`
	UpdatedAt      time.Time  `gorm:"not null"`
	Attempts       int        `gorm:"not null"`
	LeaseOwner     string     `gorm:"size:255"`
	LeaseToken     string     `gorm:"size:255;index"`
	LeaseExpiresAt *time.Time `gorm:"index"`
	LastError      string     `gorm:"type:text"`
	Payload        string     `gorm:"type:text;not null"`
}

func (eventRecord) TableName() string { return "coordination_events" }

type effectRecord struct {
	ID             string     `gorm:"primaryKey;size:255"`
	EventID        string     `gorm:"size:255;not null;index"`
	CoordinationID string     `gorm:"size:255;not null;index"`
	IdempotencyKey string     `gorm:"size:512;not null;uniqueIndex"`
	Status         string     `gorm:"size:32;not null;index:idx_coordination_effect_claim,priority:1"`
	AvailableAt    time.Time  `gorm:"not null;index:idx_coordination_effect_claim,priority:2"`
	CreatedAt      time.Time  `gorm:"not null;index:idx_coordination_effect_claim,priority:3"`
	UpdatedAt      time.Time  `gorm:"not null"`
	Attempts       int        `gorm:"not null"`
	LeaseOwner     string     `gorm:"size:255"`
	LeaseToken     string     `gorm:"size:255;index"`
	LeaseExpiresAt *time.Time `gorm:"index"`
	LastError      string     `gorm:"type:text"`
	Payload        string     `gorm:"type:text;not null"`
}

func (effectRecord) TableName() string { return "coordination_effects" }

type executionRecord struct {
	ID             string     `gorm:"primaryKey;size:255"`
	CoordinationID string     `gorm:"size:255;not null;index"`
	Status         string     `gorm:"size:32;not null;index:idx_coordination_execution_claim,priority:1"`
	Priority       int        `gorm:"not null;default:0;index:idx_coordination_execution_claim,priority:2"`
	AvailableAt    time.Time  `gorm:"not null;index:idx_coordination_execution_claim,priority:3"`
	CreatedAt      time.Time  `gorm:"not null;index:idx_coordination_execution_claim,priority:4"`
	UpdatedAt      time.Time  `gorm:"not null"`
	LeaseToken     string     `gorm:"size:255;index"`
	LeaseExpiresAt *time.Time `gorm:"index"`
	Payload        string     `gorm:"type:text;not null"`
}

func (executionRecord) TableName() string { return "coordination_executions" }

func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("coordination/sqlitestore: database path is required")
	}
	dsn, filePath := path, ""
	if path == ":memory:" {
		dsn = fmt.Sprintf("file:coordination-memory-%d?mode=memory&cache=shared&_foreign_keys=on", memorySequence.Add(1))
	} else if !strings.HasPrefix(path, "file:") {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("coordination/sqlitestore: resolve path: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			return nil, fmt.Errorf("coordination/sqlitestore: create directory: %w", err)
		}
		filePath = absolute
		dsn = absolute + "?_journal_mode=WAL&_busy_timeout=30000&_foreign_keys=on&_synchronous=NORMAL"
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("coordination/sqlitestore: open SQLite: %w", err)
	}
	if sqlDB, dbErr := db.DB(); dbErr == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	store, err := New(db)
	if err != nil {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}
	store.owned = true
	if filePath != "" {
		if err := os.Chmod(filePath, 0o600); err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("coordination/sqlitestore: secure database: %w", err)
		}
	}
	return store, nil
}

func New(db *gorm.DB) (*Store, error) {
	if db == nil || db.Dialector == nil || db.Dialector.Name() != "sqlite" {
		return nil, errors.New("coordination/sqlitestore: SQLite database is required")
	}
	if err := db.AutoMigrate(&bindingRecord{}, &eventRecord{}, &effectRecord{}, &executionRecord{}); err != nil {
		return nil, fmt.Errorf("coordination/sqlitestore: migrate: %w", err)
	}
	store := &Store{db: db}
	if err := store.promoteQueuedWakeupExecutions(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) promoteQueuedWakeupExecutions() error {
	var records []executionRecord
	if err := s.db.Where("status = ? AND priority < ? AND payload LIKE ?", string(coordination.ExecutionQueued), coordination.ExecutionPriorityWakeup, `%"control.resume":true%`).Find(&records).Error; err != nil {
		return wrap("find queued wakeup executions", err)
	}
	for _, record := range records {
		execution, err := decodeExecution(record)
		if err != nil {
			return err
		}
		resume, _ := execution.Spec.Values["control.resume"].(bool)
		if !resume {
			continue
		}
		execution.Priority = coordination.ExecutionPriorityWakeup
		encoded, err := encodeExecution(execution)
		if err != nil {
			return err
		}
		updated := s.db.Model(&executionRecord{}).Where("id = ? AND status = ? AND priority < ?", execution.ID, string(coordination.ExecutionQueued), coordination.ExecutionPriorityWakeup).Updates(executionRecordValues(encoded))
		if updated.Error != nil {
			return wrap("promote queued wakeup execution", updated.Error)
		}
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || !s.owned {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) SaveBinding(ctx context.Context, binding coordination.Binding) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	config := string(binding.Config)
	if config == "" {
		config = "null"
	}
	record := bindingRecord{CoordinationID: binding.CoordinationID, Mode: binding.Mode, Version: binding.Version, Config: config, UpdatedAt: binding.UpdatedAt.UTC()}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now().UTC()
	}
	return wrap("save binding", s.db.WithContext(nonNil(ctx)).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "coordination_id"}}, DoUpdates: clause.AssignmentColumns([]string{"mode", "version", "config", "updated_at"})}).Create(&record).Error)
}

func (s *Store) Binding(ctx context.Context, id string) (coordination.Binding, error) {
	var record bindingRecord
	err := s.db.WithContext(nonNil(ctx)).First(&record, "coordination_id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return coordination.Binding{}, coordination.ErrNotFound
	}
	if err != nil {
		return coordination.Binding{}, wrap("get binding", err)
	}
	return coordination.Binding{CoordinationID: record.CoordinationID, Mode: record.Mode, Version: record.Version, Config: json.RawMessage(record.Config), UpdatedAt: record.UpdatedAt}, nil
}

func (s *Store) SubmitEvent(ctx context.Context, event coordination.Event) (bool, error) {
	if err := event.Validate(); err != nil {
		return false, err
	}
	payload, err := marshal(event)
	if err != nil {
		return false, err
	}
	createdAt := event.OccurredAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	record := eventRecord{ID: event.ID, CoordinationID: event.CoordinationID, Status: string(coordination.StatusPending), AvailableAt: createdAt, CreatedAt: createdAt, UpdatedAt: createdAt, Payload: payload}
	result := s.db.WithContext(nonNil(ctx)).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	return result.RowsAffected == 1, wrap("submit event", result.Error)
}

func (s *Store) ClaimEvent(ctx context.Context, request coordination.ClaimRequest) (coordination.EventClaim, bool, error) {
	if err := validateClaim(request); err != nil {
		return coordination.EventClaim{}, false, err
	}
	for scan := 0; scan < 32; scan++ {
		var records []eventRecord
		err := s.db.WithContext(nonNil(ctx)).Where(
			"(status = ? AND available_at <= ?) OR (status = ? AND lease_expires_at <= ?)",
			string(coordination.StatusPending), request.Now, string(coordination.StatusProcessing), request.Now,
		).Order("attempts asc, available_at asc, created_at asc, id asc").Limit(1).Find(&records).Error
		if err != nil {
			return coordination.EventClaim{}, false, wrap("find event claim", err)
		}
		if len(records) == 0 {
			return coordination.EventClaim{}, false, nil
		}
		record := records[0]
		token, err := leaseToken()
		if err != nil {
			return coordination.EventClaim{}, false, err
		}
		expires := request.Now.UTC().Add(request.LeaseDuration)
		updated := s.db.WithContext(nonNil(ctx)).Model(&eventRecord{}).Where(
			"id = ? AND ((status = ? AND available_at <= ?) OR (status = ? AND lease_expires_at <= ?))",
			record.ID, string(coordination.StatusPending), request.Now, string(coordination.StatusProcessing), request.Now,
		).Updates(map[string]any{"status": string(coordination.StatusProcessing), "attempts": gorm.Expr("attempts + 1"), "lease_owner": request.WorkerID, "lease_token": token, "lease_expires_at": expires, "updated_at": request.Now.UTC()})
		if updated.Error != nil {
			return coordination.EventClaim{}, false, wrap("claim event", updated.Error)
		}
		if updated.RowsAffected == 0 {
			continue
		}
		event, err := decodeEvent(record.Payload)
		return coordination.EventClaim{Event: event, Token: token, Attempt: record.Attempts + 1}, true, err
	}
	return coordination.EventClaim{}, false, nil
}

func (s *Store) CommitDecision(ctx context.Context, claim coordination.EventClaim, effects []coordination.Effect, now time.Time) error {
	return s.db.WithContext(nonNil(ctx)).Transaction(func(tx *gorm.DB) error {
		for _, effect := range effects {
			if effect.ID == "" || effect.EventID != claim.Event.ID || effect.CoordinationID != claim.Event.CoordinationID || effect.IdempotencyKey == "" {
				return errors.New("coordination/sqlitestore: invalid committed effect identity")
			}
			payload, err := marshal(effect)
			if err != nil {
				return err
			}
			var existing effectRecord
			lookup := tx.Where("id = ? OR idempotency_key = ?", effect.ID, effect.IdempotencyKey).Limit(1).Find(&existing)
			if lookup.Error != nil {
				return wrap("find committed effect", lookup.Error)
			}
			if existing.ID != "" {
				if existing.ID == effect.ID && existing.EventID == effect.EventID && existing.CoordinationID == effect.CoordinationID && existing.IdempotencyKey == effect.IdempotencyKey && existing.Payload == string(payload) {
					continue
				}
				return fmt.Errorf("%w: effect identity", coordination.ErrConflict)
			}
			record := effectRecord{ID: effect.ID, EventID: effect.EventID, CoordinationID: effect.CoordinationID, IdempotencyKey: effect.IdempotencyKey, Status: string(coordination.StatusPending), AvailableAt: effect.AvailableAt.UTC(), CreatedAt: now.UTC(), UpdatedAt: now.UTC(), Payload: payload}
			if err := tx.Create(&record).Error; err != nil {
				if isUniqueError(err) {
					return fmt.Errorf("%w: effect identity", coordination.ErrConflict)
				}
				return wrap("insert effect", err)
			}
		}
		updated := tx.Model(&eventRecord{}).Where("id = ? AND status = ? AND lease_token = ? AND lease_expires_at > ?", claim.Event.ID, string(coordination.StatusProcessing), claim.Token, now).Updates(map[string]any{
			"status": string(coordination.StatusCompleted), "lease_owner": "", "lease_token": "", "lease_expires_at": nil, "last_error": "", "updated_at": now.UTC(),
		})
		if updated.Error != nil {
			return wrap("complete event", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return coordination.ErrLeaseLost
		}
		return nil
	})
}

func (s *Store) RetryEvent(ctx context.Context, claim coordination.EventClaim, now, availableAt time.Time, cause error) error {
	return s.mutateEventLease(ctx, claim, now, map[string]any{"status": string(coordination.StatusPending), "available_at": availableAt.UTC(), "last_error": errorText(cause), "lease_owner": "", "lease_token": "", "lease_expires_at": nil, "updated_at": now.UTC()})
}

func (s *Store) FailEvent(ctx context.Context, claim coordination.EventClaim, now time.Time, cause error) error {
	return s.mutateEventLease(ctx, claim, now, map[string]any{"status": string(coordination.StatusFailed), "last_error": errorText(cause), "lease_owner": "", "lease_token": "", "lease_expires_at": nil, "updated_at": now.UTC()})
}

func (s *Store) ClaimEffect(ctx context.Context, request coordination.ClaimRequest) (coordination.EffectClaim, bool, error) {
	if err := validateClaim(request); err != nil {
		return coordination.EffectClaim{}, false, err
	}
	for scan := 0; scan < 32; scan++ {
		var records []effectRecord
		err := s.db.WithContext(nonNil(ctx)).Where(
			"(status = ? AND available_at <= ?) OR (status = ? AND lease_expires_at <= ?)",
			string(coordination.StatusPending), request.Now, string(coordination.StatusProcessing), request.Now,
		).Order("available_at asc, created_at asc, id asc").Limit(1).Find(&records).Error
		if err != nil {
			return coordination.EffectClaim{}, false, wrap("find effect claim", err)
		}
		if len(records) == 0 {
			return coordination.EffectClaim{}, false, nil
		}
		record := records[0]
		token, err := leaseToken()
		if err != nil {
			return coordination.EffectClaim{}, false, err
		}
		expires := request.Now.UTC().Add(request.LeaseDuration)
		updated := s.db.WithContext(nonNil(ctx)).Model(&effectRecord{}).Where(
			"id = ? AND ((status = ? AND available_at <= ?) OR (status = ? AND lease_expires_at <= ?))",
			record.ID, string(coordination.StatusPending), request.Now, string(coordination.StatusProcessing), request.Now,
		).Updates(map[string]any{"status": string(coordination.StatusProcessing), "attempts": gorm.Expr("attempts + 1"), "lease_owner": request.WorkerID, "lease_token": token, "lease_expires_at": expires, "updated_at": request.Now.UTC()})
		if updated.Error != nil {
			return coordination.EffectClaim{}, false, wrap("claim effect", updated.Error)
		}
		if updated.RowsAffected == 0 {
			continue
		}
		effect, err := decodeEffect(record.Payload)
		effect.AvailableAt = record.AvailableAt
		return coordination.EffectClaim{Effect: effect, Token: token, Attempt: record.Attempts + 1}, true, err
	}
	return coordination.EffectClaim{}, false, nil
}

func (s *Store) CompleteEffect(ctx context.Context, claim coordination.EffectClaim, now time.Time) error {
	return s.mutateEffectLease(ctx, claim, now, map[string]any{"status": string(coordination.StatusCompleted), "last_error": "", "lease_owner": "", "lease_token": "", "lease_expires_at": nil, "updated_at": now.UTC()})
}

func (s *Store) RetryEffect(ctx context.Context, claim coordination.EffectClaim, now, availableAt time.Time, cause error) error {
	return s.mutateEffectLease(ctx, claim, now, map[string]any{"status": string(coordination.StatusPending), "available_at": availableAt.UTC(), "last_error": errorText(cause), "lease_owner": "", "lease_token": "", "lease_expires_at": nil, "updated_at": now.UTC()})
}

func (s *Store) FailEffect(ctx context.Context, claim coordination.EffectClaim, now time.Time, cause error) error {
	return s.mutateEffectLease(ctx, claim, now, map[string]any{"status": string(coordination.StatusFailed), "last_error": errorText(cause), "lease_owner": "", "lease_token": "", "lease_expires_at": nil, "updated_at": now.UTC()})
}

func (s *Store) mutateEventLease(ctx context.Context, claim coordination.EventClaim, now time.Time, values map[string]any) error {
	result := s.db.WithContext(nonNil(ctx)).Model(&eventRecord{}).Where("id = ? AND status = ? AND lease_token = ? AND lease_expires_at > ?", claim.Event.ID, string(coordination.StatusProcessing), claim.Token, now).Updates(values)
	if result.Error != nil {
		return wrap("mutate event lease", result.Error)
	}
	if result.RowsAffected != 1 {
		return coordination.ErrLeaseLost
	}
	return nil
}

func (s *Store) mutateEffectLease(ctx context.Context, claim coordination.EffectClaim, now time.Time, values map[string]any) error {
	result := s.db.WithContext(nonNil(ctx)).Model(&effectRecord{}).Where("id = ? AND status = ? AND lease_token = ? AND lease_expires_at > ?", claim.Effect.ID, string(coordination.StatusProcessing), claim.Token, now).Updates(values)
	if result.Error != nil {
		return wrap("mutate effect lease", result.Error)
	}
	if result.RowsAffected != 1 {
		return coordination.ErrLeaseLost
	}
	return nil
}

func validateClaim(request coordination.ClaimRequest) error {
	if strings.TrimSpace(request.WorkerID) == "" || request.LeaseDuration <= 0 {
		return errors.New("coordination/sqlitestore: invalid claim request")
	}
	return nil
}

func decodeEvent(payload string) (coordination.Event, error) {
	var event coordination.Event
	err := json.Unmarshal([]byte(payload), &event)
	return event, wrap("decode event", err)
}

func decodeEffect(payload string) (coordination.Effect, error) {
	var effect coordination.Effect
	err := json.Unmarshal([]byte(payload), &effect)
	return effect, wrap("decode effect", err)
}

func marshal(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("coordination/sqlitestore: marshal: %w", err)
	}
	return string(encoded), nil
}

func leaseToken() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("coordination/sqlitestore: create lease token: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func isUniqueError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unique constraint") || strings.Contains(text, "constraint failed")
}

func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("coordination/sqlitestore: %s: %w", operation, err)
}

func nonNil(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

var _ coordination.Repository = (*Store)(nil)
var _ coordination.ExecutionRepository = (*Store)(nil)
