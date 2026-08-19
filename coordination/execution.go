package coordination

import (
	"context"
	"errors"
	"strings"
	"time"

	"aegis/agenthost"
	"github.com/z3r2ne/agentcore"
)

// ExecutionStatus is the durable lifecycle of an Agent execution owned by
// the Coordination control plane.
type ExecutionStatus string

const (
	ExecutionQueued    ExecutionStatus = "queued"
	ExecutionRunning   ExecutionStatus = "running"
	ExecutionSucceeded ExecutionStatus = "succeeded"
	ExecutionFailed    ExecutionStatus = "failed"
	ExecutionCancelled ExecutionStatus = "cancelled"

	ExecutionPriorityNormal      = 0
	ExecutionPriorityIssueLow    = 10
	ExecutionPriorityIssueMiddle = 20
	ExecutionPriorityIssueHigh   = 30
	ExecutionPriorityWakeup      = 100
)

var (
	ErrExecutionNotFound  = errors.New("coordination: execution not found")
	ErrExecutionConflict  = errors.New("coordination: execution already exists")
	ErrExecutionLeaseLost = errors.New("coordination: execution lease lost")
	ErrExecutionNotQueued = errors.New("coordination: execution is not queued")
)

// Execution contains only durable execution intent and outcome. Concrete
// model clients, tools, MCP sessions and Phone sessions are materialized by
// AgentHost after a worker claims it.
type Execution struct {
	ID             string                  `json:"id"`
	CoordinationID string                  `json:"coordinationId"`
	Spec           agenthost.ExecutionSpec `json:"spec"`
	Status         ExecutionStatus         `json:"status"`
	Attempt        int                     `json:"attempt"`
	MaxAttempts    int                     `json:"maxAttempts"`
	Priority       int                     `json:"priority,omitempty"`
	AvailableAt    time.Time               `json:"availableAt"`
	CreatedAt      time.Time               `json:"createdAt"`
	UpdatedAt      time.Time               `json:"updatedAt"`
	StartedAt      *time.Time              `json:"startedAt,omitempty"`
	FinishedAt     *time.Time              `json:"finishedAt,omitempty"`
	LastError      string                  `json:"lastError,omitempty"`
	Result         *agenthost.Result       `json:"result,omitempty"`
	Origin         ExecutionOrigin         `json:"origin,omitempty"`

	LeaseOwner     string     `json:"leaseOwner,omitempty"`
	LeaseToken     string     `json:"leaseToken,omitempty"`
	LeaseExpiresAt *time.Time `json:"leaseExpiresAt,omitempty"`
}

// ExecutionOrigin links a generic platform execution back to the Application
// Work request that created it. It contains correlation only, never product
// domain fields such as Issue ID.
type ExecutionOrigin struct {
	AppID         string `json:"appId,omitempty"`
	TenantID      string `json:"tenantId,omitempty"`
	ScopeID       string `json:"scopeId,omitempty"`
	CorrelationID string `json:"correlationId,omitempty"`
	RequestID     string `json:"requestId,omitempty"`
	WorkID        string `json:"workId,omitempty"`
	TraceID       string `json:"traceId,omitempty"`
}

type ExecutionTransition string

const (
	TransitionClaimed        ExecutionTransition = "claimed"
	TransitionStarted        ExecutionTransition = "started"
	TransitionAttemptFailed  ExecutionTransition = "attempt_failed"
	TransitionRetryScheduled ExecutionTransition = "retry_scheduled"
	TransitionCompleted      ExecutionTransition = "completed"
	TransitionFailed         ExecutionTransition = "failed"
)

type ExecutionLifecycleSink interface {
	RecordExecutionTransition(context.Context, Execution, ExecutionTransition, error) error
}

type ExecutionLifecycleSinkFunc func(context.Context, Execution, ExecutionTransition, error) error

func (f ExecutionLifecycleSinkFunc) RecordExecutionTransition(ctx context.Context, execution Execution, transition ExecutionTransition, runErr error) error {
	return f(ctx, execution, transition, runErr)
}

type ExecutionClaim struct {
	Execution  Execution
	LeaseToken string
}

type ExecutionClaimRequest struct {
	WorkerID        string
	Now             time.Time
	LeaseDuration   time.Duration
	MinimumPriority int
}

type AppendExecutionPromptRequest struct {
	ExecutionID string
	Key         string
	Message     string
	Now         time.Time
}

type ExecutionLeaseRequest struct {
	ExecutionID string
	LeaseToken  string
	Now         time.Time
	ExpiresAt   time.Time
}

type CompleteExecutionRequest struct {
	ExecutionID string
	LeaseToken  string
	Now         time.Time
	Result      agenthost.Result
}

type RetryExecutionRequest struct {
	ExecutionID string
	LeaseToken  string
	Now         time.Time
	AvailableAt time.Time
	Error       string
}

type FailExecutionRequest struct {
	ExecutionID string
	LeaseToken  string
	Now         time.Time
	Error       string
}

// ExecutionRepository is the durable Agent execution boundary owned by
// Coordination. All lease mutations must be atomic and fenced.
type ExecutionRepository interface {
	EnqueueExecution(context.Context, Execution) error
	Execution(context.Context, string) (Execution, error)
	AppendExecutionPrompt(context.Context, AppendExecutionPromptRequest) error
	ClaimExecution(context.Context, ExecutionClaimRequest) (ExecutionClaim, bool, error)
	RenewExecution(context.Context, ExecutionLeaseRequest) error
	CompleteExecution(context.Context, CompleteExecutionRequest) error
	RetryExecution(context.Context, RetryExecutionRequest) error
	FailExecution(context.Context, FailExecutionRequest) error
	CancelExecution(context.Context, string, time.Time, string) error
	CancelCoordinationExecutions(context.Context, string, time.Time, string) (int64, error)
}

type ExecutionEventSinkFactory interface {
	ExecutionEventSink(context.Context, Execution) (agentcore.EventSink, error)
}

type ExecutionEventSinkFactoryFunc func(context.Context, Execution) (agentcore.EventSink, error)

func (f ExecutionEventSinkFactoryFunc) ExecutionEventSink(ctx context.Context, execution Execution) (agentcore.EventSink, error) {
	return f(ctx, execution)
}

type ExecutionRetryPolicy interface {
	NextExecutionRetry(Execution, error) (time.Duration, bool)
}

type ExecutionRetryPolicyFunc func(Execution, error) (time.Duration, bool)

func (f ExecutionRetryPolicyFunc) NextExecutionRetry(execution Execution, err error) (time.Duration, bool) {
	return f(execution, err)
}

type ExecutionOutcome struct {
	ExecutionID string
	Attempt     int
	Status      ExecutionStatus
	RunError    error
}

// ExecutionQueue is the Coordination-owned command surface for creating,
// observing and cancelling Agent executions.
type ExecutionQueue struct {
	Repository         ExecutionRepository
	DefaultMaxAttempts int
	Now                func() time.Time
	Wake               func()
}

func (q *ExecutionQueue) Enqueue(ctx context.Context, execution Execution) error {
	if q == nil || q.Repository == nil {
		return errors.New("coordination: execution repository is required")
	}
	now := q.now()
	if strings.TrimSpace(execution.ID) == "" {
		return errors.New("coordination: execution ID is required")
	}
	if strings.TrimSpace(execution.Spec.ExecutionID) == "" {
		execution.Spec.ExecutionID = execution.ID
	}
	if execution.Spec.ExecutionID != execution.ID {
		return errors.New("coordination: execution ID and spec execution ID must match")
	}
	if err := agenthost.ValidateSpec(execution.Spec); err != nil {
		return err
	}
	if execution.MaxAttempts <= 0 {
		execution.MaxAttempts = q.maxAttempts()
	}
	if resume, _ := execution.Spec.Values["control.resume"].(bool); resume && execution.Priority < ExecutionPriorityWakeup {
		execution.Priority = ExecutionPriorityWakeup
	}
	execution.Status = ExecutionQueued
	execution.Attempt = 0
	execution.LastError = ""
	execution.Result = nil
	execution.LeaseOwner = ""
	execution.LeaseToken = ""
	execution.LeaseExpiresAt = nil
	execution.StartedAt = nil
	execution.FinishedAt = nil
	if execution.AvailableAt.IsZero() {
		execution.AvailableAt = now
	}
	if execution.CreatedAt.IsZero() {
		execution.CreatedAt = now
	}
	execution.UpdatedAt = now
	if err := q.Repository.EnqueueExecution(ctx, execution); err != nil {
		return err
	}
	if q.Wake != nil {
		q.Wake()
	}
	return nil
}

func (q *ExecutionQueue) Get(ctx context.Context, id string) (Execution, error) {
	if q == nil || q.Repository == nil {
		return Execution{}, errors.New("coordination: execution repository is required")
	}
	return q.Repository.Execution(ctx, id)
}

func (q *ExecutionQueue) AppendPrompt(ctx context.Context, id, key, message string) error {
	if q == nil || q.Repository == nil {
		return errors.New("coordination: execution repository is required")
	}
	return q.Repository.AppendExecutionPrompt(ctx, AppendExecutionPromptRequest{ExecutionID: id, Key: key, Message: message, Now: q.now()})
}

func (q *ExecutionQueue) Cancel(ctx context.Context, id, reason string) error {
	if q == nil || q.Repository == nil {
		return errors.New("coordination: execution repository is required")
	}
	return q.Repository.CancelExecution(ctx, id, q.now(), reason)
}

func (q *ExecutionQueue) CancelCoordination(ctx context.Context, coordinationID, reason string) (int64, error) {
	if q == nil || q.Repository == nil {
		return 0, errors.New("coordination: execution repository is required")
	}
	if strings.TrimSpace(coordinationID) == "" {
		return 0, errors.New("coordination: coordination ID is required")
	}
	return q.Repository.CancelCoordinationExecutions(ctx, coordinationID, q.now(), reason)
}

func (q *ExecutionQueue) maxAttempts() int {
	if q.DefaultMaxAttempts > 0 {
		return q.DefaultMaxAttempts
	}
	return 3
}

func (q *ExecutionQueue) now() time.Time {
	if q.Now != nil {
		return q.Now().UTC()
	}
	return time.Now().UTC()
}
