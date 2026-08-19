package work

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNotFound = errors.New("agent work: not found")

// Gateway is the only application-facing entry point for Agent work. It
// persists request identity before handing work to the scheduler and publishes
// application lifecycle events for every accepted scheduling stage.
type Gateway struct {
	Repository Repository
	Events     EventStore
	Scheduler  Scheduler
	Now        func() time.Time
	NewID      func(string) string
}

func (g *Gateway) Request(ctx context.Context, request Request) (Receipt, error) {
	if g == nil || g.Repository == nil || g.Events == nil || g.Scheduler == nil {
		return Receipt{}, errors.New("agent work: repository, event store and scheduler are required")
	}
	if err := request.Validate(); err != nil {
		return Receipt{Status: "rejected", Reason: err.Error()}, err
	}
	if existing, ok, err := g.Repository.ByIdempotencyKey(ctx, request.AppID, request.IdempotencyKey); err != nil {
		return Receipt{}, err
	} else if ok {
		return Receipt{RequestID: existing.RequestID, WorkID: existing.WorkID, Status: receiptStatus(existing.Status), Reason: existing.LastError}, nil
	}
	now := g.now()
	record := Record{
		RequestID: g.id("work-request"), WorkID: g.id("agent-work"), Request: request,
		Status: "accepted", CreatedAt: now, UpdatedAt: now,
	}
	if err := g.Repository.Create(ctx, record); err != nil {
		// Another caller may have won the idempotency race.
		if existing, ok, lookupErr := g.Repository.ByIdempotencyKey(ctx, request.AppID, request.IdempotencyKey); lookupErr == nil && ok {
			return Receipt{RequestID: existing.RequestID, WorkID: existing.WorkID, Status: receiptStatus(existing.Status), Reason: existing.LastError}, nil
		}
		return Receipt{}, err
	}
	base := Event{
		AppID: request.AppID, TenantID: request.TenantID, ScopeID: request.ScopeID, CorrelationID: request.CorrelationID,
		RequestID: record.RequestID, WorkID: record.WorkID, OccurredAt: now,
	}
	for _, eventType := range []EventType{EventRequested, EventAccepted} {
		event := base
		event.EventID = g.id("work-event")
		event.Type = eventType
		if _, err := g.Events.Publish(ctx, event); err != nil {
			return Receipt{RequestID: record.RequestID, WorkID: record.WorkID, Status: "accepted"}, err
		}
	}
	executionID, err := g.Scheduler.Schedule(ctx, ScheduledWork{RequestID: record.RequestID, WorkID: record.WorkID, Request: request})
	if err != nil {
		record.Status, record.LastError, record.UpdatedAt = "failed", err.Error(), g.now()
		if updateErr := g.Repository.Update(ctx, record); updateErr != nil {
			return Receipt{RequestID: record.RequestID, WorkID: record.WorkID, Status: "accepted"}, errors.Join(err, updateErr)
		}
		failed := base
		failed.EventID, failed.Type, failed.OccurredAt = g.id("work-event"), EventFailed, record.UpdatedAt
		_, publishErr := g.Events.Publish(ctx, failed)
		return Receipt{RequestID: record.RequestID, WorkID: record.WorkID, Status: "accepted", Reason: err.Error()}, errors.Join(err, publishErr)
	}
	record.Status, record.ExecutionID, record.UpdatedAt = "scheduled", strings.TrimSpace(executionID), g.now()
	if err := g.Repository.Update(ctx, record); err != nil {
		return Receipt{RequestID: record.RequestID, WorkID: record.WorkID, Status: "accepted"}, err
	}
	base.ExecutionID, base.OccurredAt = record.ExecutionID, record.UpdatedAt
	for _, eventType := range []EventType{EventQueued, EventScheduled} {
		event := base
		event.EventID, event.Type = g.id("work-event"), eventType
		if _, err := g.Events.Publish(ctx, event); err != nil {
			return Receipt{RequestID: record.RequestID, WorkID: record.WorkID, Status: "accepted"}, err
		}
	}
	return Receipt{RequestID: record.RequestID, WorkID: record.WorkID, Status: "accepted"}, nil
}

func (g *Gateway) Get(ctx context.Context, workID string) (View, error) {
	if g == nil || g.Repository == nil {
		return View{}, errors.New("agent work: repository is required")
	}
	record, err := g.Repository.Get(ctx, strings.TrimSpace(workID))
	if err != nil {
		return View{}, err
	}
	return View{RequestID: record.RequestID, WorkID: record.WorkID, ExecutionID: record.ExecutionID, Status: record.Status, UpdatedAt: record.UpdatedAt}, nil
}

func (g *Gateway) Steer(context.Context, SteerRequest) (ControlReceipt, error) {
	return ControlReceipt{}, errors.New("agent work: steer controller is not configured")
}
func (g *Gateway) Suspend(context.Context, ControlRequest) (ControlReceipt, error) {
	return ControlReceipt{}, errors.New("agent work: suspend controller is not configured")
}
func (g *Gateway) Resume(context.Context, SteerRequest) (ControlReceipt, error) {
	return ControlReceipt{}, errors.New("agent work: resume controller is not configured")
}
func (g *Gateway) Cancel(context.Context, ControlRequest) (ControlReceipt, error) {
	return ControlReceipt{}, errors.New("agent work: cancel controller is not configured")
}

func (g *Gateway) now() time.Time {
	if g.Now != nil {
		return g.Now().UTC()
	}
	return time.Now().UTC()
}

func (g *Gateway) id(prefix string) string {
	if g.NewID != nil {
		return g.NewID(prefix)
	}
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(fmt.Sprintf("agent work: generate ID: %v", err))
	}
	return prefix + "-" + hex.EncodeToString(data[:])
}

func receiptStatus(status string) string {
	if status == "rejected" {
		return "rejected"
	}
	return "accepted"
}

var _ Service = (*Gateway)(nil)
