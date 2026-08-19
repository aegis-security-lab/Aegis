package sqlitestore

import (
	"context"
	"testing"
	"time"

	"aegis/platform/work"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPublishReplayAndIdempotentEventID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:app-events?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	base := work.Event{Type: work.EventAccepted, AppID: "aegis.board", ScopeID: "task-1", CorrelationID: "issue-1", RequestID: "request-1", WorkID: "work-1"}
	first, err := store.Publish(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	second := base
	second.Type = work.EventQueued
	second.RequestID = "request-2"
	second.WorkID = "work-2"
	second, err = store.Publish(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequences=%d,%d", first.Sequence, second.Sequence)
	}
	again, err := store.Publish(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if again.Sequence != first.Sequence || again.EventID != first.EventID {
		t.Fatalf("idempotent publish=%+v first=%+v", again, first)
	}
	events, cursor, err := store.Replay(context.Background(), work.EventFilter{AppID: "aegis.board", ScopeID: "task-1"}, work.Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || cursor.Sequence != 2 {
		t.Fatalf("events=%+v cursor=%+v", events, cursor)
	}
	filtered, _, err := store.Replay(context.Background(), work.EventFilter{AppID: "aegis.board", ScopeID: "task-1", RequestID: "request-2"}, work.Cursor{}, 10)
	if err != nil || len(filtered) != 1 || filtered[0].WorkID != "work-2" {
		t.Fatalf("filtered=%+v err=%v", filtered, err)
	}
}

func TestWorkRepositoryPersistsIdempotencyAndStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:app-work?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	record := work.Record{
		RequestID: "request-1", WorkID: "work-1", Status: "accepted",
		Request: work.Request{AppID: "aegis.board", ScopeID: "task-1", CorrelationID: "issue-1", IdempotencyKey: "assign-1", AgentProfileID: "backend", Prompt: "work"},
	}
	if err := store.Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	found, ok, err := store.ByIdempotencyKey(context.Background(), "aegis.board", "assign-1")
	if err != nil || !ok || found.WorkID != record.WorkID {
		t.Fatalf("found=%+v ok=%v err=%v", found, ok, err)
	}
	found.Status, found.ExecutionID, found.UpdatedAt = "scheduled", "execution-1", found.CreatedAt.Add(time.Second)
	if err := store.Update(context.Background(), found); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), "work-1")
	if err != nil || got.Status != "scheduled" || got.ExecutionID != "execution-1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestApplicationInboxResumesFromAcknowledgedCursor(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:app-inbox?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	filter := work.EventFilter{AppID: "aegis.board", ScopeID: "task-1"}
	inbox := &work.Inbox{Events: store, Subscriptions: store}
	if err := inbox.Register(context.Background(), work.Subscription{ID: "board-task-1", Filter: filter}); err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []work.EventType{work.EventQueued, work.EventScheduled} {
		if _, err := store.Publish(context.Background(), work.Event{Type: eventType, AppID: filter.AppID, ScopeID: filter.ScopeID, CorrelationID: "issue-1", RequestID: "request-1", WorkID: "work-1"}); err != nil {
			t.Fatal(err)
		}
	}
	delivery, err := inbox.Pull(context.Background(), "board-task-1", 10)
	if err != nil || len(delivery.Events) != 2 {
		t.Fatalf("delivery=%+v err=%v", delivery, err)
	}
	if err := inbox.Ack(context.Background(), delivery.SubscriptionID, delivery.Next); err != nil {
		t.Fatal(err)
	}
	// Recreate both the store adapter and Inbox to model a process restart.
	restartedStore, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &work.Inbox{Events: restartedStore, Subscriptions: restartedStore}
	empty, err := restarted.Pull(context.Background(), delivery.SubscriptionID, 10)
	if err != nil || len(empty.Events) != 0 || empty.Next.Sequence != delivery.Next.Sequence {
		t.Fatalf("restarted delivery=%+v err=%v", empty, err)
	}
	if _, err := restartedStore.Publish(context.Background(), work.Event{Type: work.EventStarted, AppID: filter.AppID, ScopeID: filter.ScopeID, CorrelationID: "issue-1", RequestID: "request-1", WorkID: "work-1"}); err != nil {
		t.Fatal(err)
	}
	next, err := restarted.Pull(context.Background(), delivery.SubscriptionID, 10)
	if err != nil || len(next.Events) != 1 || next.Events[0].Type != work.EventStarted {
		t.Fatalf("next delivery=%+v err=%v", next, err)
	}
}
