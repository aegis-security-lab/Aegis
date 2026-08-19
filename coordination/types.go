package coordination

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type EventType string

const (
	EventTimerFired  EventType = "timer_fired"
	EventModeChanged EventType = "mode_changed"
)

type EffectType string

const (
	EffectScheduleWakeup EffectType = "schedule_wakeup"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Event is the durable, application-neutral input to a coordination Mode.
// CoordinationID is the scope on which a mode/version is pinned; SubjectID
// and actor fields are opaque identifiers interpreted only by that mode.
type Event struct {
	ID                    string          `json:"id"`
	Type                  EventType       `json:"type"`
	CoordinationID        string          `json:"coordinationId"`
	ScopeID               string          `json:"scopeId,omitempty"`
	SubjectID             string          `json:"subjectId,omitempty"`
	ParentSubjectID       string          `json:"parentSubjectId,omitempty"`
	ExecutionID           string          `json:"executionId,omitempty"`
	ActorID               string          `json:"actorId,omitempty"`
	ActorInstanceID       string          `json:"actorInstanceId,omitempty"`
	ParentActorID         string          `json:"parentActorId,omitempty"`
	ParentActorInstanceID string          `json:"parentActorInstanceId,omitempty"`
	CorrelationID         string          `json:"correlationId,omitempty"`
	OccurredAt            time.Time       `json:"occurredAt"`
	Payload               json.RawMessage `json:"payload,omitempty"`
}

// UnmarshalJSON accepts the generic envelope and the legacy Board field names
// previously persisted by Aegis. New events are always marshalled with generic
// names, while existing queues remain restart-compatible during migration.
func (e *Event) UnmarshalJSON(data []byte) error {
	type eventAlias Event
	var current eventAlias
	if err := json.Unmarshal(data, &current); err != nil {
		return err
	}
	var legacy struct {
		LegacyTaskID            string `json:"taskId"`
		LegacyIssueID           string `json:"issueId"`
		LegacyParentIssueID     string `json:"parentIssueId"`
		LegacyAgentID           string `json:"agentId"`
		LegacyTaskAgentID       string `json:"taskAgentId"`
		LegacyParentAgentID     string `json:"parentAgentId"`
		LegacyParentTaskAgentID string `json:"parentTaskAgentId"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	*e = Event(current)
	if e.ScopeID == "" {
		e.ScopeID = legacy.LegacyTaskID
	}
	if e.SubjectID == "" {
		e.SubjectID = legacy.LegacyIssueID
	}
	if e.ParentSubjectID == "" {
		e.ParentSubjectID = legacy.LegacyParentIssueID
	}
	if e.ActorID == "" {
		e.ActorID = legacy.LegacyAgentID
	}
	if e.ActorInstanceID == "" {
		e.ActorInstanceID = legacy.LegacyTaskAgentID
	}
	if e.ParentActorID == "" {
		e.ParentActorID = legacy.LegacyParentAgentID
	}
	if e.ParentActorInstanceID == "" {
		e.ParentActorInstanceID = legacy.LegacyParentTaskAgentID
	}
	return nil
}

// Effect is a durable outbox command. AvailableAt makes timers ordinary
// delayed effects and avoids a second, unrelated scheduling mechanism.
type Effect struct {
	ID             string          `json:"id"`
	EventID        string          `json:"eventId"`
	CoordinationID string          `json:"coordinationId"`
	Type           EffectType      `json:"type"`
	IdempotencyKey string          `json:"idempotencyKey"`
	AvailableAt    time.Time       `json:"availableAt"`
	Payload        json.RawMessage `json:"payload,omitempty"`
}

// PlannedEffect is returned by a Mode. Engine supplies deterministic IDs,
// source EventID, CoordinationID and scheduling defaults before persistence.
type PlannedEffect struct {
	Type           EffectType      `json:"type"`
	IdempotencyKey string          `json:"idempotencyKey,omitempty"`
	AvailableAt    time.Time       `json:"availableAt,omitempty"`
	Payload        json.RawMessage `json:"payload,omitempty"`
}

// Binding pins a Task/coordination scope to one immutable mode version.
// Config is owned and interpreted by that mode.
type Binding struct {
	CoordinationID string          `json:"coordinationId"`
	Mode           string          `json:"mode"`
	Version        string          `json:"version"`
	Config         json.RawMessage `json:"config,omitempty"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type WorkStatus string

const (
	WorkPending   WorkStatus = "pending"
	WorkRunning   WorkStatus = "running"
	WorkWaiting   WorkStatus = "waiting"
	WorkSucceeded WorkStatus = "succeeded"
	WorkFailed    WorkStatus = "failed"
	WorkCancelled WorkStatus = "cancelled"
)

type WorkItem struct {
	ID                   string     `json:"id"`
	ParentID             string     `json:"parentId,omitempty"`
	Title                string     `json:"title,omitempty"`
	AssigneeID           string     `json:"assigneeId,omitempty"`
	AssigneeInstanceID   string     `json:"assigneeInstanceId,omitempty"`
	AssigneeInstanceName string     `json:"assigneeInstanceName,omitempty"`
	Status               WorkStatus `json:"status"`
	ExecutionPhase       string     `json:"executionPhase,omitempty"`
	SleepToken           string     `json:"sleepToken,omitempty"`
	ProgressSummary      string     `json:"progressSummary,omitempty"`
	CurrentActivity      string     `json:"currentActivity,omitempty"`
	UpdatedAt            time.Time  `json:"updatedAt,omitempty"`
}

func (w WorkItem) Terminal() bool {
	return w.Status == WorkSucceeded || w.Status == WorkFailed || w.Status == WorkCancelled
}

// Snapshot is the runtime-neutral read model supplied to a Mode. Extra allows
// an application to add facts without changing the core interface.
type Snapshot struct {
	Current  *WorkItem                  `json:"current,omitempty"`
	Parent   *WorkItem                  `json:"parent,omitempty"`
	Children []WorkItem                 `json:"children,omitempty"`
	Extra    map[string]json.RawMessage `json:"extra,omitempty"`
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.New("coordination: event ID is required")
	}
	if strings.TrimSpace(string(e.Type)) == "" {
		return errors.New("coordination: event type is required")
	}
	if strings.TrimSpace(e.CoordinationID) == "" {
		return errors.New("coordination: coordination ID is required")
	}
	if len(e.Payload) > 0 && !json.Valid(e.Payload) {
		return errors.New("coordination: event payload must be valid JSON")
	}
	return nil
}

func (b Binding) Validate() error {
	if strings.TrimSpace(b.CoordinationID) == "" || strings.TrimSpace(b.Mode) == "" || strings.TrimSpace(b.Version) == "" {
		return errors.New("coordination: binding coordination ID, mode and version are required")
	}
	if len(b.Config) > 0 && !json.Valid(b.Config) {
		return errors.New("coordination: binding config must be valid JSON")
	}
	return nil
}

func validatePlannedEffect(effect PlannedEffect) error {
	if strings.TrimSpace(string(effect.Type)) == "" {
		return errors.New("coordination: effect type is required")
	}
	if len(effect.Payload) > 0 && !json.Valid(effect.Payload) {
		return fmt.Errorf("coordination: %s effect payload must be valid JSON", effect.Type)
	}
	return nil
}
