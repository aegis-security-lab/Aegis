// Package coordinationadapter connects the generic Coordination execution
// engine to application-facing Agent Work lifecycle events.
package coordinationadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aegis/coordination"
	"aegis/platform/work"
)

// Lifecycle records Coordination execution transitions in the owning
// application's Work record and durable event stream. Executions without a
// Work origin are intentionally ignored by Coordination before reaching here.
type Lifecycle struct {
	Repository work.Repository
	Events     work.EventStore
	Now        func() time.Time
}

func (l *Lifecycle) RecordExecutionTransition(ctx context.Context, execution coordination.Execution, transition coordination.ExecutionTransition, runErr error) error {
	if l == nil || l.Repository == nil || l.Events == nil {
		return errors.New("agent work lifecycle: repository and event store are required")
	}
	eventType, status, updateStatus, err := mapTransition(transition)
	if err != nil {
		return err
	}
	record, err := l.Repository.Get(ctx, strings.TrimSpace(execution.Origin.WorkID))
	if err != nil {
		return fmt.Errorf("agent work lifecycle: load work: %w", err)
	}
	if err := validateOrigin(record, execution); err != nil {
		return err
	}

	now := l.now()
	if updateStatus {
		record.Status = status
		record.UpdatedAt = now
		if runErr != nil {
			record.LastError = runErr.Error()
		} else if transition == coordination.TransitionCompleted {
			record.LastError = ""
		}
		if err := l.Repository.Update(ctx, record); err != nil {
			return fmt.Errorf("agent work lifecycle: update work: %w", err)
		}
	}

	payload, err := lifecyclePayload(runErr)
	if err != nil {
		return err
	}
	event := work.Event{
		EventID:       lifecycleEventID(execution, transition),
		Type:          eventType,
		OccurredAt:    now,
		AppID:         execution.Origin.AppID,
		TenantID:      execution.Origin.TenantID,
		ScopeID:       execution.Origin.ScopeID,
		CorrelationID: execution.Origin.CorrelationID,
		RequestID:     execution.Origin.RequestID,
		WorkID:        execution.Origin.WorkID,
		ExecutionID:   execution.ID,
		AttemptID:     execution.ID + ":" + strconv.Itoa(execution.Attempt),
		TraceID:       execution.Origin.TraceID,
		Payload:       payload,
	}
	if _, err := l.Events.Publish(ctx, event); err != nil {
		return fmt.Errorf("agent work lifecycle: publish %s: %w", transition, err)
	}
	return nil
}

func mapTransition(transition coordination.ExecutionTransition) (work.EventType, string, bool, error) {
	switch transition {
	case coordination.TransitionClaimed:
		return work.EventClaimed, work.StatusClaimed, true, nil
	case coordination.TransitionStarted:
		return work.EventStarted, work.StatusRunning, true, nil
	case coordination.TransitionAttemptFailed:
		return work.EventAttemptFailed, "", false, nil
	case coordination.TransitionRetryScheduled:
		return work.EventRetryScheduled, work.StatusQueued, true, nil
	case coordination.TransitionCompleted:
		return work.EventCompleted, work.StatusCompleted, true, nil
	case coordination.TransitionFailed:
		return work.EventFailed, work.StatusFailed, true, nil
	default:
		return "", "", false, fmt.Errorf("agent work lifecycle: unsupported transition %q", transition)
	}
}

func validateOrigin(record work.Record, execution coordination.Execution) error {
	origin := execution.Origin
	if record.RequestID != origin.RequestID || record.WorkID != origin.WorkID || record.Request.AppID != origin.AppID || record.Request.TenantID != origin.TenantID || record.Request.ScopeID != origin.ScopeID || record.Request.CorrelationID != origin.CorrelationID {
		return errors.New("agent work lifecycle: execution origin does not match persisted work")
	}
	if record.ExecutionID != "" && record.ExecutionID != execution.ID {
		return errors.New("agent work lifecycle: execution ID does not match persisted work")
	}
	return nil
}

func lifecyclePayload(runErr error) (json.RawMessage, error) {
	if runErr == nil {
		return nil, nil
	}
	payload, err := json.Marshal(map[string]string{"error": runErr.Error()})
	if err != nil {
		return nil, fmt.Errorf("agent work lifecycle: encode error: %w", err)
	}
	return payload, nil
}

func lifecycleEventID(execution coordination.Execution, transition coordination.ExecutionTransition) string {
	return fmt.Sprintf("work-lifecycle-%s-%d-%s", execution.Origin.WorkID, execution.Attempt, transition)
}

func (l *Lifecycle) now() time.Time {
	if l.Now != nil {
		return l.Now().UTC()
	}
	return time.Now().UTC()
}

var _ coordination.ExecutionLifecycleSink = (*Lifecycle)(nil)
