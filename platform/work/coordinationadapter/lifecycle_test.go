package coordinationadapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"aegis/agenthost"
	"aegis/coordination"
	"aegis/platform/work"
	worksqlite "aegis/platform/work/sqlitestore"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLifecycleUpdatesWorkAndPublishesIdempotentEvents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := worksqlite.New(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	request := work.Request{AppID: "example.app", TenantID: "tenant-1", ScopeID: "scope-1", CorrelationID: "item-1", IdempotencyKey: "key-1", AgentProfileID: "writer", Prompt: "write"}
	record := work.Record{RequestID: "request-1", WorkID: "work-1", Request: request, Status: work.StatusScheduled, ExecutionID: "execution-1", CreatedAt: now, UpdatedAt: now}
	if err := store.Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	execution := coordination.Execution{
		ID: "execution-1", Attempt: 1, Spec: agenthost.ExecutionSpec{ExecutionID: "execution-1", AgentID: "writer", Prompt: "write"},
		Origin: coordination.ExecutionOrigin{AppID: request.AppID, TenantID: request.TenantID, ScopeID: request.ScopeID, CorrelationID: request.CorrelationID, RequestID: record.RequestID, WorkID: record.WorkID},
	}
	lifecycle := &Lifecycle{Repository: store, Events: store, Now: func() time.Time { return now.Add(time.Second) }}
	for _, transition := range []coordination.ExecutionTransition{coordination.TransitionClaimed, coordination.TransitionStarted, coordination.TransitionCompleted} {
		if err := lifecycle.RecordExecutionTransition(context.Background(), execution, transition, nil); err != nil {
			t.Fatal(err)
		}
	}
	// A repeated callback must not create a duplicate application event.
	if err := lifecycle.RecordExecutionTransition(context.Background(), execution, coordination.TransitionCompleted, nil); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), record.WorkID)
	if err != nil || got.Status != work.StatusCompleted {
		t.Fatalf("work=%+v err=%v", got, err)
	}
	events, cursor, err := store.Replay(context.Background(), work.EventFilter{AppID: request.AppID, TenantID: request.TenantID, ScopeID: request.ScopeID}, work.Cursor{}, 20)
	if err != nil {
		t.Fatal(err)
	}
	want := []work.EventType{work.EventClaimed, work.EventStarted, work.EventCompleted}
	if len(events) != len(want) || cursor.Sequence != int64(len(want)) {
		t.Fatalf("events=%+v cursor=%+v", events, cursor)
	}
	for index := range want {
		if events[index].Type != want[index] || events[index].AttemptID != "execution-1:1" {
			t.Fatalf("event %d=%+v", index, events[index])
		}
	}
}

func TestLifecycleRejectsMismatchedApplicationOrigin(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := worksqlite.New(db)
	if err != nil {
		t.Fatal(err)
	}
	request := work.Request{AppID: "example.app", ScopeID: "scope-1", CorrelationID: "item-1", IdempotencyKey: "key-1", AgentProfileID: "writer", Prompt: "write"}
	if err := store.Create(context.Background(), work.Record{RequestID: "request-1", WorkID: "work-1", Request: request, Status: work.StatusScheduled}); err != nil {
		t.Fatal(err)
	}
	lifecycle := &Lifecycle{Repository: store, Events: store}
	execution := coordination.Execution{ID: "execution-1", Attempt: 1, Origin: coordination.ExecutionOrigin{AppID: "other.app", ScopeID: "scope-1", CorrelationID: "item-1", RequestID: "request-1", WorkID: "work-1"}}
	if err := lifecycle.RecordExecutionTransition(context.Background(), execution, coordination.TransitionStarted, errors.New("should not leak")); err == nil {
		t.Fatal("expected mismatched origin to be rejected")
	}
}
