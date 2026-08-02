package coordination

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound  = errors.New("coordination: not found")
	ErrConflict  = errors.New("coordination: conflict")
	ErrLeaseLost = errors.New("coordination: lease lost")
)

type ClaimRequest struct {
	WorkerID      string
	Now           time.Time
	LeaseDuration time.Duration
}

type EventClaim struct {
	Event   Event
	Token   string
	Attempt int
}

type EffectClaim struct {
	Effect  Effect
	Token   string
	Attempt int
}

// Repository owns the durable inbox, mode bindings, transactional decision
// commit, and effect outbox. CommitDecision must atomically mark the claimed
// event complete and insert every effect.
type Repository interface {
	SaveBinding(context.Context, Binding) error
	Binding(context.Context, string) (Binding, error)
	SubmitEvent(context.Context, Event) (bool, error)
	ClaimEvent(context.Context, ClaimRequest) (EventClaim, bool, error)
	CommitDecision(context.Context, EventClaim, []Effect, time.Time) error
	RetryEvent(context.Context, EventClaim, time.Time, time.Time, error) error
	FailEvent(context.Context, EventClaim, time.Time, error) error
	ClaimEffect(context.Context, ClaimRequest) (EffectClaim, bool, error)
	CompleteEffect(context.Context, EffectClaim, time.Time) error
	RetryEffect(context.Context, EffectClaim, time.Time, time.Time, error) error
	FailEffect(context.Context, EffectClaim, time.Time, error) error
}

type SnapshotLoader interface {
	Snapshot(context.Context, Event) (Snapshot, error)
}

type SnapshotLoaderFunc func(context.Context, Event) (Snapshot, error)

func (f SnapshotLoaderFunc) Snapshot(ctx context.Context, event Event) (Snapshot, error) {
	if f == nil {
		return Snapshot{}, nil
	}
	return f(ctx, event)
}

type EffectHandler interface {
	HandleEffect(context.Context, Effect) error
}

type EffectHandlerFunc func(context.Context, Effect) error

func (f EffectHandlerFunc) HandleEffect(ctx context.Context, effect Effect) error {
	if f == nil {
		return errors.New("coordination: nil effect handler")
	}
	return f(ctx, effect)
}
