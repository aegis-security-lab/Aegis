package agentapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type GormStore struct{ db *gorm.DB }

type phoneSessionRow struct {
	ID            string    `gorm:"primaryKey;size:64"`
	AgentID       string    `gorm:"index;size:128;not null"`
	TaskAgentID   string    `gorm:"index;size:128"`
	TaskID        string    `gorm:"index;size:128"`
	ActorJSON     string    `gorm:"type:text;not null"`
	InstalledJSON string    `gorm:"type:text;not null"`
	ActiveAppID   string    `gorm:"size:128"`
	StacksJSON    string    `gorm:"type:text;not null"`
	DraftsJSON    string    `gorm:"type:text;not null"`
	CurrentJSON   string    `gorm:"type:text;not null"`
	ResultsJSON   string    `gorm:"type:text;not null"`
	CreatedAt     time.Time `gorm:"index"`
	UpdatedAt     time.Time `gorm:"index"`
}

func (phoneSessionRow) TableName() string { return "agent_app_phone_sessions" }

type auditEventRow struct {
	ID             string    `gorm:"primaryKey;size:64"`
	OccurredAt     time.Time `gorm:"index;not null"`
	AgentID        string    `gorm:"index;size:128;not null"`
	PhoneSessionID string    `gorm:"index;size:64;not null"`
	AppID          string    `gorm:"size:128"`
	PageID         string    `gorm:"size:160"`
	PageRevision   string    `gorm:"size:64"`
	Ref            string    `gorm:"size:32"`
	Action         string    `gorm:"size:64;not null"`
	Target         string    `gorm:"size:512"`
	Result         string    `gorm:"index;size:32;not null"`
	Effect         string    `gorm:"size:64"`
	ErrorCode      string    `gorm:"index;size:64"`
	ErrorMessage   string    `gorm:"type:text"`
	ArgumentsJSON  string    `gorm:"type:text"`
}

func (auditEventRow) TableName() string { return "agent_app_audit_events" }

type storedPage struct {
	Page    Page              `json:"page"`
	Targets map[string]string `json:"targets,omitempty"`
}

func OpenSQLiteStore(path string) (*GormStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("SQLite path is required")
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return NewGormStore(db)
}

func NewGormStore(db *gorm.DB) (*GormStore, error) {
	if db == nil {
		return nil, fmt.Errorf("gorm DB is required")
	}
	if err := db.AutoMigrate(&phoneSessionRow{}, &auditEventRow{}); err != nil {
		return nil, err
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_phone_task_identity ON agent_app_phone_sessions(task_id, task_agent_id) WHERE task_id <> '' AND task_agent_id <> ''").Error; err != nil {
		return nil, fmt.Errorf("create task Phone identity index: %w", err)
	}
	return &GormStore{db: db}, nil
}

func (s *GormStore) SaveSession(ctx context.Context, session *Session) error {
	if session == nil || session.ID == "" {
		return fmt.Errorf("phone session is required")
	}
	actorJSON, err := marshalJSON(session.Actor)
	if err != nil {
		return err
	}
	installedJSON, err := marshalJSON(session.Installed)
	if err != nil {
		return err
	}
	stacksJSON, err := marshalJSON(session.Stacks)
	if err != nil {
		return err
	}
	draftsJSON, err := marshalJSON(session.Drafts)
	if err != nil {
		return err
	}
	targets := make(map[string]string, len(session.Current.Refs))
	for _, ref := range session.Current.Refs {
		if ref.Target != "" {
			targets[ref.Ref] = ref.Target
		}
	}
	currentJSON, err := marshalJSON(storedPage{Page: session.Current, Targets: targets})
	if err != nil {
		return err
	}
	resultsJSON, err := marshalJSON(session.results)
	if err != nil {
		return err
	}
	row := phoneSessionRow{ID: session.ID, AgentID: session.Actor.AgentID, TaskAgentID: session.Actor.TaskAgentID, TaskID: session.Actor.TaskID, ActorJSON: actorJSON, InstalledJSON: installedJSON, ActiveAppID: session.ActiveAppID, StacksJSON: stacksJSON, DraftsJSON: draftsJSON, CurrentJSON: currentJSON, ResultsJSON: resultsJSON, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt}
	return s.db.WithContext(ctx).Save(&row).Error
}

func (s *GormStore) FindTaskSession(ctx context.Context, taskID, taskAgentID string) (*Session, error) {
	var row phoneSessionRow
	err := s.db.WithContext(ctx).First(&row, "task_id = ? AND task_agent_id = ?", strings.TrimSpace(taskID), strings.TrimSpace(taskAgentID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return sessionFromRow(row)
}

func (s *GormStore) LoadSession(ctx context.Context, id string) (*Session, error) {
	var row phoneSessionRow
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
		}
		return nil, err
	}
	session := &Session{ID: row.ID, ActiveAppID: row.ActiveAppID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if err := unmarshalJSON(row.ActorJSON, &session.Actor); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(row.InstalledJSON, &session.Installed); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(row.StacksJSON, &session.Stacks); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(row.DraftsJSON, &session.Drafts); err != nil {
		return nil, err
	}
	var current storedPage
	if err := unmarshalJSON(row.CurrentJSON, &current); err != nil {
		return nil, err
	}
	for index := range current.Page.Refs {
		current.Page.Refs[index].Target = current.Targets[current.Page.Refs[index].Ref]
	}
	session.Current = current.Page
	if err := unmarshalJSON(row.ResultsJSON, &session.results); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *GormStore) ListSessions(ctx context.Context, agentID string, limit int) ([]SessionState, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query := s.db.WithContext(ctx).Order("updated_at desc").Limit(limit)
	if strings.TrimSpace(agentID) != "" {
		query = query.Where("agent_id = ?", strings.TrimSpace(agentID))
	}
	var rows []phoneSessionRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	states := make([]SessionState, 0, len(rows))
	for _, row := range rows {
		session, err := sessionFromRow(row)
		if err != nil {
			return nil, err
		}
		states = append(states, stateFromSession(session))
	}
	return states, nil
}

func (s *GormStore) Record(ctx context.Context, event AuditEvent) {
	argumentsJSON, _ := marshalJSON(event.Arguments)
	row := auditEventRow{ID: event.ID, OccurredAt: event.OccurredAt, AgentID: event.AgentID, PhoneSessionID: event.PhoneSessionID, AppID: event.AppID, PageID: event.PageID, PageRevision: event.PageRevision, Ref: event.Ref, Action: event.Action, Target: event.Target, Result: event.Result, Effect: event.Effect, ErrorCode: event.ErrorCode, ErrorMessage: event.ErrorMessage, ArgumentsJSON: argumentsJSON}
	_ = s.db.WithContext(ctx).Create(&row).Error
}

func (s *GormStore) Events(ctx context.Context, phoneSessionID string, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []auditEventRow
	if err := s.db.WithContext(ctx).Where("phone_session_id = ?", phoneSessionID).Order("occurred_at desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	events := make([]AuditEvent, 0, len(rows))
	for _, row := range rows {
		event := AuditEvent{ID: row.ID, OccurredAt: row.OccurredAt, AgentID: row.AgentID, PhoneSessionID: row.PhoneSessionID, AppID: row.AppID, PageID: row.PageID, PageRevision: row.PageRevision, Ref: row.Ref, Action: row.Action, Target: row.Target, Result: row.Result, Effect: row.Effect, ErrorCode: row.ErrorCode, ErrorMessage: row.ErrorMessage}
		if row.ArgumentsJSON != "" {
			if err := unmarshalJSON(row.ArgumentsJSON, &event.Arguments); err != nil {
				return nil, err
			}
		}
		events = append(events, event)
	}
	return events, nil
}

func sessionFromRow(row phoneSessionRow) (*Session, error) {
	session := &Session{ID: row.ID, ActiveAppID: row.ActiveAppID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if err := unmarshalJSON(row.ActorJSON, &session.Actor); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(row.InstalledJSON, &session.Installed); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(row.StacksJSON, &session.Stacks); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(row.DraftsJSON, &session.Drafts); err != nil {
		return nil, err
	}
	var current storedPage
	if err := unmarshalJSON(row.CurrentJSON, &current); err != nil {
		return nil, err
	}
	for index := range current.Page.Refs {
		current.Page.Refs[index].Target = current.Targets[current.Page.Refs[index].Ref]
	}
	session.Current = current.Page
	if err := unmarshalJSON(row.ResultsJSON, &session.results); err != nil {
		return nil, err
	}
	return session, nil
}

func marshalJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal phone state: %w", err)
	}
	return string(encoded), nil
}

func unmarshalJSON(value string, target any) error {
	if value == "" {
		value = "null"
	}
	if err := json.Unmarshal([]byte(value), target); err != nil {
		return fmt.Errorf("unmarshal phone state: %w", err)
	}
	return nil
}
