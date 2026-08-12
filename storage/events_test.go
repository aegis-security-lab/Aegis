package storage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/z3r2ne/agentcore"
)

func TestMemoryEventStoreAssignsAtomicCursorAndDetachesEvents(t *testing.T) {
	store := NewMemoryEventStore()
	now := time.Date(2026, 7, 30, 14, 0, 0, 0, time.UTC)
	const count = 32
	var wg sync.WaitGroup
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			_, err := store.AppendExecutionEvent(context.Background(), NewExecutionEvent{
				ExecutionID: "exec-1", Attempt: 1, CreatedAt: now,
				Event: agentcore.Event{Type: agentcore.EventMessageUpdate, Error: fmt.Sprintf("event-%d", value)},
			})
			if err != nil {
				t.Errorf("append: %v", err)
			}
		}(index)
	}
	wg.Wait()
	events, err := store.ExecutionEvents(context.Background(), "exec-1", 10, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != count-10 || events[0].Sequence != 11 || events[len(events)-1].Sequence != count {
		t.Fatalf("events = %+v", events)
	}
	events[0].Event.Error = "mutated"
	reloaded, err := store.ExecutionEvents(context.Background(), "exec-1", 10, 1)
	if err != nil || reloaded[0].Event.Error == "mutated" {
		t.Fatalf("reloaded=%+v err=%v", reloaded, err)
	}
}

func TestEventSinkPersistsAttemptAndTimestamp(t *testing.T) {
	store := NewMemoryEventStore()
	now := time.Date(2026, 7, 30, 15, 0, 0, 0, time.UTC)
	sink := EventSink(store, "exec-2", 3, func() time.Time { return now })
	if err := sink(context.Background(), agentcore.Event{Type: agentcore.EventAgentStart}); err != nil {
		t.Fatal(err)
	}
	events, _ := store.ExecutionEvents(context.Background(), "exec-2", 0, 10)
	if len(events) != 1 || events[0].Attempt != 3 || !events[0].CreatedAt.Equal(now) {
		t.Fatalf("events = %+v", events)
	}
}

func TestEventSinkDoesNotPersistMessageUpdates(t *testing.T) {
	store := NewMemoryEventStore()
	sink := EventSink(store, "exec-filter", 1, nil)
	for _, eventType := range []agentcore.EventType{
		agentcore.EventMessageStart,
		agentcore.EventMessageUpdate,
		agentcore.EventMessageEnd,
	} {
		if err := sink(context.Background(), agentcore.Event{Type: eventType}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.ExecutionEvents(context.Background(), "exec-filter", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events=%d, want 2", len(events))
	}
	if events[0].Event.Type != agentcore.EventMessageStart || events[0].Sequence != 1 {
		t.Fatalf("first event=%+v", events[0])
	}
	if events[1].Event.Type != agentcore.EventMessageEnd || events[1].Sequence != 2 {
		t.Fatalf("second event=%+v", events[1])
	}
}
