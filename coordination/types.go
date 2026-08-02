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
	EventDelegationRequested EventType = "delegation_requested"
	EventIssueCreated        EventType = "issue_created"
	EventIssueAssigned       EventType = "issue_assigned"
	EventIssueCompleted      EventType = "issue_completed"
	EventExecutionCompleted  EventType = "execution_completed"
	EventRelayReceived       EventType = "relay_received"
	EventWaitRequested       EventType = "wait_requested"
	EventTimerFired          EventType = "timer_fired"
	EventAgentStopped        EventType = "agent_stopped"
	EventModeChanged         EventType = "mode_changed"
)

type EffectType string

const (
	EffectCreateIssue    EffectType = "create_issue"
	EffectAssignIssue    EffectType = "assign_issue"
	EffectEnqueueIssue   EffectType = "enqueue_issue"
	EffectStartSubagent  EffectType = "start_subagent"
	EffectSuspendAgent   EffectType = "suspend_agent"
	EffectResumeAgent    EffectType = "resume_agent"
	EffectDeliverMessage EffectType = "deliver_message"
	EffectSendRelay      EffectType = "send_relay"
	EffectScheduleWakeup EffectType = "schedule_wakeup"
	EffectCancelWork     EffectType = "cancel_work"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Event is the durable input to a coordination Mode. CoordinationID normally
// identifies a Task; it is the unit on which a mode/version is pinned.
type Event struct {
	ID                string          `json:"id"`
	Type              EventType       `json:"type"`
	CoordinationID    string          `json:"coordinationId"`
	TaskID            string          `json:"taskId,omitempty"`
	IssueID           string          `json:"issueId,omitempty"`
	ParentIssueID     string          `json:"parentIssueId,omitempty"`
	ExecutionID       string          `json:"executionId,omitempty"`
	AgentID           string          `json:"agentId,omitempty"`
	TaskAgentID       string          `json:"taskAgentId,omitempty"`
	ParentAgentID     string          `json:"parentAgentId,omitempty"`
	ParentTaskAgentID string          `json:"parentTaskAgentId,omitempty"`
	CorrelationID     string          `json:"correlationId,omitempty"`
	OccurredAt        time.Time       `json:"occurredAt"`
	Payload           json.RawMessage `json:"payload,omitempty"`
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
	ID              string     `json:"id"`
	ParentID        string     `json:"parentId,omitempty"`
	Title           string     `json:"title,omitempty"`
	AssigneeID      string     `json:"assigneeId,omitempty"`
	TaskAgentID     string     `json:"taskAgentId,omitempty"`
	TaskAgentName   string     `json:"taskAgentName,omitempty"`
	Status          WorkStatus `json:"status"`
	ExecutionPhase  string     `json:"executionPhase,omitempty"`
	SleepToken      string     `json:"sleepToken,omitempty"`
	ProgressSummary string     `json:"progressSummary,omitempty"`
	CurrentActivity string     `json:"currentActivity,omitempty"`
	UpdatedAt       time.Time  `json:"updatedAt,omitempty"`
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
