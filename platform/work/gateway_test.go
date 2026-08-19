package work

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type memoryRepository struct{ records map[string]Record }

func (r *memoryRepository) Create(_ context.Context, record Record) error {
	if r.records == nil {
		r.records = map[string]Record{}
	}
	for _, existing := range r.records {
		if existing.Request.AppID == record.Request.AppID && existing.Request.IdempotencyKey == record.Request.IdempotencyKey {
			return errors.New("duplicate")
		}
	}
	r.records[record.WorkID] = record
	return nil
}
func (r *memoryRepository) ByIdempotencyKey(_ context.Context, appID, key string) (Record, bool, error) {
	for _, record := range r.records {
		if record.Request.AppID == appID && record.Request.IdempotencyKey == key {
			return record, true, nil
		}
	}
	return Record{}, false, nil
}
func (r *memoryRepository) Get(_ context.Context, id string) (Record, error) {
	record, ok := r.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return record, nil
}
func (r *memoryRepository) Update(_ context.Context, record Record) error {
	r.records[record.WorkID] = record
	return nil
}

type memoryEvents struct{ events []Event }

func (s *memoryEvents) Publish(_ context.Context, event Event) (Event, error) {
	event.Sequence = int64(len(s.events) + 1)
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if err := event.Validate(); err != nil {
		return Event{}, err
	}
	s.events = append(s.events, event)
	return event, nil
}
func (s *memoryEvents) Replay(context.Context, EventFilter, Cursor, int) ([]Event, Cursor, error) {
	return append([]Event(nil), s.events...), Cursor{Sequence: int64(len(s.events))}, nil
}

func TestGatewayPersistsAndPublishesSchedulingLifecycle(t *testing.T) {
	repository, events := &memoryRepository{}, &memoryEvents{}
	sequence := 0
	gateway := &Gateway{
		Repository: repository, Events: events,
		Scheduler: SchedulerFunc(func(_ context.Context, scheduled ScheduledWork) (string, error) {
			if scheduled.Request.AppID != "aegis.board" || scheduled.WorkID == "" {
				t.Fatalf("scheduled=%+v", scheduled)
			}
			return "execution-1", nil
		}),
		Now:   func() time.Time { return time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC) },
		NewID: func(prefix string) string { sequence++; return fmt.Sprintf("%s-%d", prefix, sequence) },
	}
	request := Request{AppID: "aegis.board", ScopeID: "task-1", CorrelationID: "issue-1", IdempotencyKey: "assign-1", AgentProfileID: "backend", Prompt: "work"}
	receipt, err := gateway.Request(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "accepted" || receipt.WorkID == "" {
		t.Fatalf("receipt=%+v", receipt)
	}
	want := []EventType{EventRequested, EventAccepted, EventQueued, EventScheduled}
	if len(events.events) != len(want) {
		t.Fatalf("events=%+v", events.events)
	}
	for index, eventType := range want {
		if events.events[index].Type != eventType {
			t.Fatalf("event %d=%s want %s", index, events.events[index].Type, eventType)
		}
	}
	second, err := gateway.Request(context.Background(), request)
	if err != nil || second.RequestID != receipt.RequestID || len(events.events) != len(want) {
		t.Fatalf("idempotent receipt=%+v err=%v events=%d", second, err, len(events.events))
	}
}
