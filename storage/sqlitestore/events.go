package sqlitestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"aegis/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var memorySequence atomic.Uint64

type Store struct {
	db    *gorm.DB
	owned bool
}

type eventCursor struct {
	ExecutionID string `gorm:"primaryKey;size:255"`
	Sequence    uint64 `gorm:"not null"`
}

func (eventCursor) TableName() string { return "agent_runtime_event_cursors" }

type eventRecord struct {
	ExecutionID string    `gorm:"primaryKey;size:255"`
	Sequence    uint64    `gorm:"primaryKey;autoIncrement:false"`
	Attempt     int       `gorm:"not null"`
	CreatedAt   time.Time `gorm:"not null;index"`
	Payload     string    `gorm:"type:text;not null"`
}

func (eventRecord) TableName() string { return "agent_runtime_events" }

func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("storage/sqlitestore: database path is required")
	}
	dsn := path
	filePath := ""
	if path == ":memory:" {
		dsn = fmt.Sprintf("file:event-memory-%d?mode=memory&cache=shared&_foreign_keys=on", memorySequence.Add(1))
	} else if !strings.HasPrefix(path, "file:") {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			return nil, err
		}
		filePath = absolute
		dsn = absolute + "?_journal_mode=WAL&_busy_timeout=30000&_foreign_keys=on&_synchronous=NORMAL"
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("storage/sqlitestore: open: %w", err)
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
			return nil, err
		}
	}
	return store, nil
}

func New(db *gorm.DB) (*Store, error) {
	if db == nil || db.Dialector == nil || db.Dialector.Name() != "sqlite" {
		return nil, errors.New("storage/sqlitestore: SQLite database is required")
	}
	if err := db.AutoMigrate(&eventCursor{}, &eventRecord{}); err != nil {
		return nil, fmt.Errorf("storage/sqlitestore: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) AppendExecutionEvent(ctx context.Context, input storage.NewExecutionEvent) (storage.ExecutionEvent, error) {
	if s == nil || s.db == nil {
		return storage.ExecutionEvent{}, errors.New("storage/sqlitestore: nil store")
	}
	if strings.TrimSpace(input.ExecutionID) == "" {
		return storage.ExecutionEvent{}, errors.New("storage/sqlitestore: execution ID is required")
	}
	payload, err := json.Marshal(input.Event)
	if err != nil {
		return storage.ExecutionEvent{}, fmt.Errorf("storage/sqlitestore: encode event: %w", err)
	}
	var sequence uint64
	err = s.db.WithContext(nonNil(ctx)).Transaction(func(tx *gorm.DB) error {
		row := tx.Raw(`INSERT INTO agent_runtime_event_cursors (execution_id, sequence) VALUES (?, 1)
			ON CONFLICT(execution_id) DO UPDATE SET sequence = sequence + 1 RETURNING sequence`, input.ExecutionID).Row()
		if err := row.Scan(&sequence); err != nil {
			return err
		}
		return tx.Create(&eventRecord{ExecutionID: input.ExecutionID, Sequence: sequence, Attempt: input.Attempt, CreatedAt: input.CreatedAt.UTC(), Payload: string(payload)}).Error
	})
	if err != nil {
		return storage.ExecutionEvent{}, fmt.Errorf("storage/sqlitestore: append event: %w", err)
	}
	return storage.ExecutionEvent{ExecutionID: input.ExecutionID, Attempt: input.Attempt, Sequence: sequence, CreatedAt: input.CreatedAt.UTC(), Event: input.Event}, nil
}

func (s *Store) ExecutionEvents(ctx context.Context, executionID string, after uint64, limit int) ([]storage.ExecutionEvent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("storage/sqlitestore: nil store")
	}
	if limit <= 0 {
		limit = 100
	}
	var records []eventRecord
	err := s.db.WithContext(nonNil(ctx)).Where("execution_id = ? AND sequence > ?", executionID, after).Order("sequence ASC").Limit(limit).Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("storage/sqlitestore: list events: %w", err)
	}
	result := make([]storage.ExecutionEvent, len(records))
	for index, record := range records {
		var event storage.ExecutionEvent
		event.ExecutionID, event.Attempt, event.Sequence, event.CreatedAt = record.ExecutionID, record.Attempt, record.Sequence, record.CreatedAt
		if err := json.Unmarshal([]byte(record.Payload), &event.Event); err != nil {
			return nil, fmt.Errorf("storage/sqlitestore: decode event %s/%d: %w", record.ExecutionID, record.Sequence, err)
		}
		result[index] = event
	}
	return result, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil || !s.owned {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	s.owned = false
	return sqlDB.Close()
}

func nonNil(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

var _ storage.EventStore = (*Store)(nil)
