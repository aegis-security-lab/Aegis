// Package work defines the asynchronous application-to-Agent scheduling
// contract. It contains no Board, Issue, validation or other product concepts.
package work

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aegis/capability"
	"aegis/platform/dataspace"
)

type Budget struct {
	MaxTurns   int           `json:"maxTurns,omitempty"`
	ActiveTime time.Duration `json:"activeTime,omitempty"`
	MaxCostUSD float64       `json:"maxCostUsd,omitempty"`
}

type Request struct {
	AppID          string `json:"appId"`
	TenantID       string `json:"tenantId,omitempty"`
	ScopeID        string `json:"scopeId"`
	CorrelationID  string `json:"correlationId"`
	IdempotencyKey string `json:"idempotencyKey"`

	AgentProfileID string     `json:"agentProfileId"`
	SessionKey     string     `json:"sessionKey,omitempty"`
	Prompt         string     `json:"prompt"`
	Priority       int        `json:"priority,omitempty"`
	StartAfter     *time.Time `json:"startAfter,omitempty"`
	Deadline       *time.Time `json:"deadline,omitempty"`
	Budget         Budget     `json:"budget,omitempty"`

	Capabilities  []capability.Ref  `json:"capabilities,omitempty"`
	DataSpaces    []dataspace.Grant `json:"dataSpaces,omitempty"`
	InstalledApps []string          `json:"installedApps,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.AppID) == "" || strings.TrimSpace(r.ScopeID) == "" || strings.TrimSpace(r.CorrelationID) == "" {
		return errors.New("agent work: app, scope and correlation IDs are required")
	}
	if strings.TrimSpace(r.IdempotencyKey) == "" || strings.TrimSpace(r.AgentProfileID) == "" || strings.TrimSpace(r.Prompt) == "" {
		return errors.New("agent work: idempotency key, Agent profile and prompt are required")
	}
	if r.Deadline != nil && r.StartAfter != nil && r.Deadline.Before(*r.StartAfter) {
		return errors.New("agent work: deadline cannot precede startAfter")
	}
	for _, grant := range r.DataSpaces {
		if err := grant.Validate(); err != nil {
			return err
		}
		if grant.AppID != r.AppID {
			return errors.New("agent work: cross-application DataSpace grants require an explicit platform link")
		}
	}
	return nil
}

type Receipt struct {
	RequestID string `json:"requestId"`
	WorkID    string `json:"workId"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
}

const (
	StatusAccepted  = "accepted"
	StatusScheduled = "scheduled"
	StatusClaimed   = "claimed"
	StatusRunning   = "running"
	StatusQueued    = "queued"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusExpired   = "expired"
)

type Record struct {
	RequestID   string    `json:"requestId"`
	WorkID      string    `json:"workId"`
	Request     Request   `json:"request"`
	Status      string    `json:"status"`
	ExecutionID string    `json:"executionId,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Repository interface {
	Create(context.Context, Record) error
	ByIdempotencyKey(context.Context, string, string) (Record, bool, error)
	Get(context.Context, string) (Record, error)
	Update(context.Context, Record) error
}

type ScheduledWork struct {
	RequestID string
	WorkID    string
	Request   Request
}

// Scheduler is implemented by a platform execution adapter. Product
// applications never receive this interface and can only use Service.
type Scheduler interface {
	Schedule(context.Context, ScheduledWork) (executionID string, err error)
}

type SchedulerFunc func(context.Context, ScheduledWork) (string, error)

func (f SchedulerFunc) Schedule(ctx context.Context, scheduled ScheduledWork) (string, error) {
	return f(ctx, scheduled)
}

type ControlRequest struct {
	AppID          string `json:"appId"`
	WorkID         string `json:"workId"`
	IdempotencyKey string `json:"idempotencyKey"`
	Reason         string `json:"reason,omitempty"`
}

type SteerRequest struct {
	ControlRequest
	Message string `json:"message"`
}

type ControlReceipt struct {
	RequestID string `json:"requestId"`
	WorkID    string `json:"workId"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
}

type View struct {
	RequestID   string    `json:"requestId"`
	WorkID      string    `json:"workId"`
	ExecutionID string    `json:"executionId,omitempty"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Service interface {
	Request(context.Context, Request) (Receipt, error)
	Get(context.Context, string) (View, error)
	Steer(context.Context, SteerRequest) (ControlReceipt, error)
	Suspend(context.Context, ControlRequest) (ControlReceipt, error)
	Resume(context.Context, SteerRequest) (ControlReceipt, error)
	Cancel(context.Context, ControlRequest) (ControlReceipt, error)
}

type EventType string

const (
	EventRequested      EventType = "agent.work.requested"
	EventAccepted       EventType = "agent.work.accepted"
	EventRejected       EventType = "agent.work.rejected"
	EventQueued         EventType = "agent.work.queued"
	EventScheduled      EventType = "agent.work.scheduled"
	EventClaimed        EventType = "agent.work.claimed"
	EventStarted        EventType = "agent.work.started"
	EventProgress       EventType = "agent.work.progress"
	EventWaiting        EventType = "agent.work.waiting"
	EventSuspended      EventType = "agent.work.suspended"
	EventResumed        EventType = "agent.work.resumed"
	EventSteered        EventType = "agent.work.steered"
	EventRetryScheduled EventType = "agent.work.retry_scheduled"
	EventAttemptFailed  EventType = "agent.work.attempt_failed"
	EventCompleted      EventType = "agent.work.completed"
	EventFailed         EventType = "agent.work.failed"
	EventCancelled      EventType = "agent.work.cancelled"
	EventExpired        EventType = "agent.work.expired"
)

type Event struct {
	EventID       string          `json:"eventId"`
	Sequence      int64           `json:"sequence"`
	Type          EventType       `json:"type"`
	OccurredAt    time.Time       `json:"occurredAt"`
	AppID         string          `json:"appId"`
	TenantID      string          `json:"tenantId,omitempty"`
	ScopeID       string          `json:"scopeId"`
	CorrelationID string          `json:"correlationId"`
	RequestID     string          `json:"requestId"`
	WorkID        string          `json:"workId"`
	ExecutionID   string          `json:"executionId,omitempty"`
	AttemptID     string          `json:"attemptId,omitempty"`
	TraceID       string          `json:"traceId,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.EventID) == "" || e.Sequence <= 0 || strings.TrimSpace(string(e.Type)) == "" || e.OccurredAt.IsZero() {
		return errors.New("agent work event: ID, positive sequence, type and time are required")
	}
	if strings.TrimSpace(e.AppID) == "" || strings.TrimSpace(e.ScopeID) == "" || strings.TrimSpace(e.CorrelationID) == "" {
		return errors.New("agent work event: app, scope and correlation IDs are required")
	}
	if strings.TrimSpace(e.RequestID) == "" || strings.TrimSpace(e.WorkID) == "" {
		return errors.New("agent work event: request and work IDs are required")
	}
	if len(e.Payload) > 0 && !json.Valid(e.Payload) {
		return errors.New("agent work event: payload must be valid JSON")
	}
	return nil
}

type Cursor struct {
	Sequence int64 `json:"sequence"`
}

type EventFilter struct {
	AppID         string
	TenantID      string
	ScopeID       string
	CorrelationID string
	RequestID     string
	WorkID        string
}

type EventStore interface {
	Publish(context.Context, Event) (Event, error)
	Replay(context.Context, EventFilter, Cursor, int) ([]Event, Cursor, error)
}

// Subscription is a durable application inbox cursor. A subscription is
// bound to one application event stream and can optionally narrow delivery to
// a correlation, request, or work ID.
type Subscription struct {
	ID        string      `json:"id"`
	Filter    EventFilter `json:"filter"`
	Cursor    Cursor      `json:"cursor"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

func (s Subscription) Validate() error {
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.Filter.AppID) == "" || strings.TrimSpace(s.Filter.ScopeID) == "" {
		return errors.New("application inbox: subscription, app and scope IDs are required")
	}
	if s.Cursor.Sequence < 0 {
		return errors.New("application inbox: cursor cannot be negative")
	}
	return nil
}

type SubscriptionRepository interface {
	RegisterSubscription(context.Context, Subscription) error
	Subscription(context.Context, string) (Subscription, error)
	AckSubscription(context.Context, string, Cursor, time.Time) error
}

type Delivery struct {
	SubscriptionID string  `json:"subscriptionId"`
	Events         []Event `json:"events"`
	Next           Cursor  `json:"next"`
}
