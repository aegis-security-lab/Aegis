package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"aegis/observability"
)

const defaultLeaseDuration = 30 * time.Second

type Engine struct {
	Repository  Repository
	Modes       *Registry
	Snapshots   SnapshotLoader
	WorkerID    string
	Lease       time.Duration
	MaxAttempts int
	Now         func() time.Time
}

func (e *Engine) Submit(ctx context.Context, event Event) (bool, error) {
	if err := e.validate(); err != nil {
		return false, err
	}
	if err := event.Validate(); err != nil {
		return false, err
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = e.now()
	}
	ctx = observability.WithScope(ctx, observability.Scope{TaskID: event.TaskID, IssueID: event.IssueID, ExecutionID: event.ExecutionID, AgentID: event.AgentID, CoordinationID: event.CoordinationID, Component: "coordination.engine"})
	inserted, err := e.Repository.SubmitEvent(ctx, event)
	attrs := []slog.Attr{slog.String("event_id", event.ID), slog.String("event_type", string(event.Type)), slog.Bool("inserted", inserted)}
	if err != nil {
		observability.Default().Error(ctx, "coordination.event.submit_failed", append(attrs, slog.String("error", err.Error()))...)
	} else {
		observability.Default().Info(ctx, "coordination.event.submitted", attrs...)
	}
	observability.DefaultMetrics().AddCounter("coordination_events_submitted_total", 1, observability.Labels{"type": string(event.Type), "inserted": fmt.Sprint(inserted)})
	return inserted, err
}

// ProcessNext makes one durable policy decision. Mode errors are persisted as
// retryable event failures; repository/lease errors are returned to the worker.
func (e *Engine) ProcessNext(ctx context.Context) (bool, error) {
	if err := e.validate(); err != nil {
		return false, err
	}
	now := e.now()
	claim, ok, err := e.Repository.ClaimEvent(ctx, ClaimRequest{WorkerID: e.WorkerID, Now: now, LeaseDuration: e.lease()})
	if err != nil || !ok {
		return ok, err
	}
	ctx = observability.WithScope(ctx, observability.Scope{TaskID: claim.Event.TaskID, IssueID: claim.Event.IssueID, ExecutionID: claim.Event.ExecutionID, AgentID: claim.Event.AgentID, CoordinationID: claim.Event.CoordinationID, Component: "coordination.decision"})
	observability.Default().Info(ctx, "coordination.event.claimed", slog.String("event_id", claim.Event.ID), slog.String("event_type", string(claim.Event.Type)), slog.Int("attempt", claim.Attempt), slog.String("worker_id", e.WorkerID))
	binding, err := e.Repository.Binding(ctx, claim.Event.CoordinationID)
	if err != nil {
		return true, e.retryDecision(ctx, claim, err)
	}
	mode, err := e.Modes.Resolve(binding.Mode, binding.Version)
	if err != nil {
		return true, e.retryDecision(ctx, claim, err)
	}
	observability.Default().Debug(ctx, "coordination.mode.selected", slog.String("mode", binding.Mode), slog.String("version", binding.Version), slog.String("event_id", claim.Event.ID))
	snapshot := Snapshot{}
	if e.Snapshots != nil {
		snapshot, err = e.Snapshots.Snapshot(ctx, claim.Event)
		if err != nil {
			return true, e.retryDecision(ctx, claim, fmt.Errorf("coordination: load snapshot: %w", err))
		}
	}
	planned, err := mode.Decide(ctx, claim.Event, binding, snapshot)
	if err != nil {
		return true, e.retryDecision(ctx, claim, fmt.Errorf("coordination: mode %s@%s: %w", binding.Mode, binding.Version, err))
	}
	effects := make([]Effect, len(planned))
	for index, item := range planned {
		if err := validatePlannedEffect(item); err != nil {
			return true, e.retryDecision(ctx, claim, err)
		}
		availableAt := item.AvailableAt.UTC()
		if availableAt.IsZero() {
			availableAt = now
		}
		key := strings.TrimSpace(item.IdempotencyKey)
		if key == "" {
			key = fmt.Sprintf("%s:%d", claim.Event.ID, index)
		}
		effects[index] = Effect{
			ID: deterministicEffectID(claim.Event.ID, index), EventID: claim.Event.ID,
			CoordinationID: claim.Event.CoordinationID, Type: item.Type,
			IdempotencyKey: key, AvailableAt: availableAt,
			Payload: append([]byte(nil), item.Payload...),
		}
	}
	err = e.Repository.CommitDecision(ctx, claim, effects, now)
	if err != nil {
		observability.Default().Error(ctx, "coordination.decision.commit_failed", slog.String("event_id", claim.Event.ID), slog.Int("effect_count", len(effects)), slog.String("error", err.Error()))
		observability.DefaultMetrics().AddCounter("coordination_decisions_total", 1, observability.Labels{"mode": binding.Mode, "status": "error"})
		if errors.Is(err, ErrLeaseLost) {
			return true, err
		}
		if errors.Is(err, ErrConflict) {
			observability.Default().Error(ctx, "coordination.event.failed", slog.String("event_id", claim.Event.ID), slog.Int("attempt", claim.Attempt), slog.String("error", err.Error()))
			return true, e.Repository.FailEvent(ctx, claim, now, err)
		}
		return true, e.retryDecision(ctx, claim, err)
	}
	observability.Default().Info(ctx, "coordination.decision.committed", slog.String("event_id", claim.Event.ID), slog.String("mode", binding.Mode), slog.Int("effect_count", len(effects)))
	observability.DefaultMetrics().AddCounter("coordination_decisions_total", 1, observability.Labels{"mode": binding.Mode, "status": "ok"})
	return true, nil
}

func (e *Engine) retryDecision(ctx context.Context, claim EventClaim, cause error) error {
	now := e.now()
	if claim.Attempt >= e.maxAttempts() {
		observability.Default().Error(ctx, "coordination.event.failed", slog.String("event_id", claim.Event.ID), slog.Int("attempt", claim.Attempt), slog.String("error", cause.Error()))
		return e.Repository.FailEvent(ctx, claim, now, cause)
	}
	observability.Default().Warn(ctx, "coordination.event.retry", slog.String("event_id", claim.Event.ID), slog.Int("attempt", claim.Attempt), slog.String("error", cause.Error()))
	delay := decisionRetryDelay(claim.Attempt)
	if err := e.Repository.RetryEvent(ctx, claim, now, now.Add(delay), cause); err != nil {
		return errors.Join(cause, err)
	}
	return nil
}

func decisionRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 6 {
		shift = 6
	}
	return time.Second * time.Duration(1<<shift)
}

func (e *Engine) maxAttempts() int {
	if e.MaxAttempts > 0 {
		return e.MaxAttempts
	}
	return 8
}

func (e *Engine) validate() error {
	if e == nil || e.Repository == nil || e.Modes == nil {
		return errors.New("coordination: engine repository and mode registry are required")
	}
	if strings.TrimSpace(e.WorkerID) == "" {
		return errors.New("coordination: engine worker ID is required")
	}
	return nil
}

func (e *Engine) lease() time.Duration {
	if e.Lease > 0 {
		return e.Lease
	}
	return defaultLeaseDuration
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func deterministicEffectID(eventID string, index int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", eventID, index)))
	return "coord-effect-" + hex.EncodeToString(digest[:12])
}

type EffectWorker struct {
	Repository  Repository
	Handler     EffectHandler
	WorkerID    string
	Lease       time.Duration
	RetryDelay  time.Duration
	MaxAttempts int
	Now         func() time.Time
}

func (w *EffectWorker) ProcessNext(ctx context.Context) (bool, error) {
	if w == nil || w.Repository == nil || w.Handler == nil || strings.TrimSpace(w.WorkerID) == "" {
		return false, errors.New("coordination: effect worker repository, handler and worker ID are required")
	}
	now := w.now()
	lease := w.Lease
	if lease <= 0 {
		lease = defaultLeaseDuration
	}
	claim, ok, err := w.Repository.ClaimEffect(ctx, ClaimRequest{WorkerID: w.WorkerID, Now: now, LeaseDuration: lease})
	if err != nil || !ok {
		return ok, err
	}
	ctx = observability.WithScope(ctx, observability.Scope{CoordinationID: claim.Effect.CoordinationID, Component: "coordination.effect"})
	observability.Default().Info(ctx, "coordination.effect.claimed", slog.String("effect_id", claim.Effect.ID), slog.String("event_id", claim.Effect.EventID), slog.String("effect_type", string(claim.Effect.Type)), slog.Int("attempt", claim.Attempt))
	if err := w.Handler.HandleEffect(ctx, claim.Effect); err != nil {
		if claim.Attempt >= w.maxAttempts() {
			observability.Default().Error(ctx, "coordination.effect.failed", slog.String("effect_id", claim.Effect.ID), slog.String("effect_type", string(claim.Effect.Type)), slog.String("error", err.Error()))
			return true, w.Repository.FailEffect(ctx, claim, now, err)
		}
		observability.Default().Warn(ctx, "coordination.effect.retry", slog.String("effect_id", claim.Effect.ID), slog.String("effect_type", string(claim.Effect.Type)), slog.Int("attempt", claim.Attempt), slog.String("error", err.Error()))
		delay := w.RetryDelay
		if delay <= 0 {
			delay = time.Second
		}
		if retryErr := w.Repository.RetryEffect(ctx, claim, now, now.Add(delay), err); retryErr != nil {
			return true, errors.Join(err, retryErr)
		}
		return true, nil
	}
	// Once an external side effect succeeded, persist its outbox completion even
	// if shutdown canceled the worker context between the handler return and the
	// fenced write. Otherwise the effect may be replayed after its lease expires.
	completionCtx := context.WithoutCancel(ctx)
	err = w.Repository.CompleteEffect(completionCtx, claim, now)
	if err != nil {
		observability.Default().Error(ctx, "coordination.effect.complete_failed", slog.String("effect_id", claim.Effect.ID), slog.String("error", err.Error()))
	} else {
		observability.Default().Info(ctx, "coordination.effect.completed", slog.String("effect_id", claim.Effect.ID), slog.String("effect_type", string(claim.Effect.Type)))
	}
	observability.DefaultMetrics().AddCounter("coordination_effects_total", 1, observability.Labels{"type": string(claim.Effect.Type), "status": map[bool]string{true: "error", false: "ok"}[err != nil]})
	return true, err
}

func (w *EffectWorker) maxAttempts() int {
	if w.MaxAttempts > 0 {
		return w.MaxAttempts
	}
	return 8
}

func (w *EffectWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}
