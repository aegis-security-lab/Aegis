package coordination

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"aegis/agenthost"
	"aegis/observability"
	"github.com/z3r2ne/agentcore"
)

// ExecutionWorker claims Coordination-owned executions and invokes AgentHost.
type ExecutionWorker struct {
	Repository ExecutionRepository
	Executor   agenthost.Runner
	Sinks      ExecutionEventSinkFactory
	Retry      ExecutionRetryPolicy
	WorkerID   string

	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	MinimumPriority   int
	Now               func() time.Time
}

func (w *ExecutionWorker) RunNext(ctx context.Context) (outcome ExecutionOutcome, claimed bool, err error) {
	if err := w.validate(); err != nil {
		return ExecutionOutcome{}, false, err
	}
	leaseDuration := w.leaseDuration()
	claim, ok, err := w.Repository.ClaimExecution(ctx, ExecutionClaimRequest{WorkerID: w.WorkerID, Now: w.now(), LeaseDuration: leaseDuration, MinimumPriority: w.MinimumPriority})
	if err != nil || !ok {
		return ExecutionOutcome{}, ok, err
	}
	execution := claim.Execution
	outcome = ExecutionOutcome{ExecutionID: execution.ID, Attempt: execution.Attempt, Status: ExecutionRunning}
	ctx = observability.WithScope(ctx, observability.Scope{
		TaskID: specValue(execution.Spec.Values, "control.taskId"), IssueID: specValue(execution.Spec.Values, "control.issueId"),
		ExecutionID: execution.Spec.ExecutionID, AgentID: execution.Spec.AgentID, JobID: execution.ID, Component: "coordination.execution",
	})
	started := time.Now()
	logger := observability.Default()
	logger.Info(ctx, "coordination.execution.claimed", slog.Int("attempt", execution.Attempt), slog.Int("max_attempts", execution.MaxAttempts), slog.String("worker_id", w.WorkerID))
	observability.DefaultMetrics().AddCounter("coordination_executions_claimed_total", 1, observability.Labels{"worker": w.WorkerID})
	defer func() {
		attrs := []slog.Attr{slog.String("status", string(outcome.Status)), slog.Int("attempt", outcome.Attempt), slog.Float64("duration_ms", float64(time.Since(started).Microseconds())/1000)}
		if outcome.RunError != nil {
			attrs = append(attrs, slog.String("run_error", outcome.RunError.Error()))
		}
		if err != nil {
			attrs = append(attrs, slog.String("infrastructure_error", err.Error()))
			logger.Error(ctx, "coordination.execution.finished", attrs...)
		} else if outcome.Status == ExecutionFailed {
			logger.Warn(ctx, "coordination.execution.finished", attrs...)
		} else {
			logger.Info(ctx, "coordination.execution.finished", attrs...)
		}
		observability.DefaultMetrics().AddCounter("coordination_execution_outcomes_total", 1, observability.Labels{"status": string(outcome.Status)})
		observability.DefaultMetrics().ObserveHistogram("coordination_execution_duration_ms", float64(time.Since(started).Microseconds())/1000, observability.Labels{"status": string(outcome.Status)})
	}()

	var sink agentcore.EventSink
	if w.Sinks != nil {
		sink, err = w.Sinks.ExecutionEventSink(ctx, execution)
		if err != nil {
			return outcome, true, fmt.Errorf("coordination: create execution event sink: %w", err)
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	heartbeatDone := make(chan struct{})
	heartbeatResult := make(chan error, 1)
	go w.heartbeat(runCtx, claim, leaseDuration, heartbeatDone, heartbeatResult, cancel)
	result, runErr := w.Executor.Run(runCtx, execution.Spec, sink)
	close(heartbeatDone)
	heartbeatErr := <-heartbeatResult
	cancel()
	if heartbeatErr != nil {
		return outcome, true, heartbeatErr
	}
	if ctx.Err() != nil {
		return outcome, true, ctx.Err()
	}
	now := w.now()
	if runErr == nil {
		if err := w.Repository.CompleteExecution(ctx, CompleteExecutionRequest{ExecutionID: execution.ID, LeaseToken: claim.LeaseToken, Now: now, Result: result}); err != nil {
			return outcome, true, err
		}
		outcome.Status = ExecutionSucceeded
		return outcome, true, nil
	}
	outcome.RunError = runErr
	if delay, retry := w.retryPolicy().NextExecutionRetry(execution, runErr); retry {
		if delay < 0 {
			delay = 0
		}
		if err := w.Repository.RetryExecution(ctx, RetryExecutionRequest{ExecutionID: execution.ID, LeaseToken: claim.LeaseToken, Now: now, AvailableAt: now.Add(delay), Error: runErr.Error()}); err != nil {
			return outcome, true, err
		}
		outcome.Status = ExecutionQueued
		return outcome, true, nil
	}
	if err := w.Repository.FailExecution(ctx, FailExecutionRequest{ExecutionID: execution.ID, LeaseToken: claim.LeaseToken, Now: now, Error: runErr.Error()}); err != nil {
		return outcome, true, err
	}
	outcome.Status = ExecutionFailed
	return outcome, true, nil
}

func (w *ExecutionWorker) heartbeat(ctx context.Context, claim ExecutionClaim, leaseDuration time.Duration, done <-chan struct{}, result chan<- error, cancel context.CancelFunc) {
	ticker := time.NewTicker(w.heartbeatInterval(leaseDuration))
	defer ticker.Stop()
	for {
		select {
		case <-done:
			result <- nil
			return
		case <-ctx.Done():
			result <- nil
			return
		case <-ticker.C:
			now := w.now()
			err := w.Repository.RenewExecution(ctx, ExecutionLeaseRequest{ExecutionID: claim.Execution.ID, LeaseToken: claim.LeaseToken, Now: now, ExpiresAt: now.Add(leaseDuration)})
			if err != nil {
				observability.Default().Error(ctx, "coordination.execution.lease_renew_failed", slog.String("error", err.Error()))
				cancel()
				result <- fmt.Errorf("coordination: renew execution lease: %w", err)
				return
			}
		}
	}
}

func (w *ExecutionWorker) validate() error {
	if w == nil || w.Repository == nil {
		return errors.New("coordination: execution repository is required")
	}
	if w.Executor == nil {
		return errors.New("coordination: Agent executor is required")
	}
	if strings.TrimSpace(w.WorkerID) == "" {
		return errors.New("coordination: execution worker ID is required")
	}
	return nil
}

func (w *ExecutionWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *ExecutionWorker) leaseDuration() time.Duration {
	if w.LeaseDuration > 0 {
		return w.LeaseDuration
	}
	return defaultLeaseDuration
}

func (w *ExecutionWorker) heartbeatInterval(lease time.Duration) time.Duration {
	if w.HeartbeatInterval > 0 && w.HeartbeatInterval < lease {
		return w.HeartbeatInterval
	}
	return lease / 3
}

func (w *ExecutionWorker) retryPolicy() ExecutionRetryPolicy {
	if w.Retry != nil {
		return w.Retry
	}
	return ExecutionRetryPolicyFunc(func(execution Execution, _ error) (time.Duration, bool) {
		if execution.Attempt >= execution.MaxAttempts {
			return 0, false
		}
		return time.Second << min(execution.Attempt-1, 6), true
	})
}

func specValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
