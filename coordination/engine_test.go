package coordination

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestEnginePinsModeAndCommitsEffectsExactlyOnce(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	repository := NewMemoryRepository()
	registry := NewRegistry()
	decisions := 0
	if err := registry.Register(ModeFunc{ModeName: "test", ModeVersion: "1", DecideFunc: func(_ context.Context, event Event, binding Binding, snapshot Snapshot) ([]PlannedEffect, error) {
		decisions++
		if event.SubjectID != "issue-1" || binding.CoordinationID != "task-1" || snapshot.Current.ID != "issue-1" {
			t.Fatalf("event=%+v binding=%+v snapshot=%+v", event, binding, snapshot)
		}
		return []PlannedEffect{{Type: testEffectEnqueueSubject, Payload: json.RawMessage(`{"issueId":"issue-1"}`)}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveBinding(context.Background(), Binding{CoordinationID: "task-1", Mode: "test", Version: "1", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	engine := Engine{Repository: repository, Modes: registry, WorkerID: "decision-1", Now: func() time.Time { return now }, Snapshots: SnapshotLoaderFunc(func(context.Context, Event) (Snapshot, error) {
		return Snapshot{Current: &WorkItem{ID: "issue-1", Status: WorkPending}}, nil
	})}
	event := Event{ID: "event-1", Type: testEventSubjectAssigned, CoordinationID: "task-1", SubjectID: "issue-1"}
	inserted, err := engine.Submit(context.Background(), event)
	if err != nil || !inserted {
		t.Fatalf("submit inserted=%v err=%v", inserted, err)
	}
	inserted, err = engine.Submit(context.Background(), event)
	if err != nil || inserted {
		t.Fatalf("duplicate inserted=%v err=%v", inserted, err)
	}
	processed, err := engine.ProcessNext(context.Background())
	if err != nil || !processed || decisions != 1 {
		t.Fatalf("processed=%v decisions=%d err=%v", processed, decisions, err)
	}
	processed, err = engine.ProcessNext(context.Background())
	if err != nil || processed || decisions != 1 {
		t.Fatalf("second processed=%v decisions=%d err=%v", processed, decisions, err)
	}

	var effects []Effect
	worker := EffectWorker{Repository: repository, WorkerID: "effects-1", Now: func() time.Time { return now }, Handler: EffectHandlerFunc(func(_ context.Context, effect Effect) error {
		effects = append(effects, effect)
		return nil
	})}
	processed, err = worker.ProcessNext(context.Background())
	if err != nil || !processed || len(effects) != 1 || effects[0].ID != deterministicEffectID("event-1", 0) || effects[0].IdempotencyKey != "event-1:0" {
		t.Fatalf("processed=%v effects=%+v err=%v", processed, effects, err)
	}
	processed, err = worker.ProcessNext(context.Background())
	if err != nil || processed || len(effects) != 1 {
		t.Fatalf("duplicate effect processed=%v effects=%+v err=%v", processed, effects, err)
	}
}

func TestEventDecodesLegacyApplicationEnvelope(t *testing.T) {
	var event Event
	if err := json.Unmarshal([]byte(`{"id":"legacy","type":"assigned","coordinationId":"scope","taskId":"task","issueId":"item","parentIssueId":"parent","agentId":"actor","taskAgentId":"actor-instance","parentAgentId":"owner","parentTaskAgentId":"owner-instance"}`), &event); err != nil {
		t.Fatal(err)
	}
	if event.ScopeID != "task" || event.SubjectID != "item" || event.ParentSubjectID != "parent" || event.ActorID != "actor" || event.ActorInstanceID != "actor-instance" || event.ParentActorID != "owner" || event.ParentActorInstanceID != "owner-instance" {
		t.Fatalf("legacy event=%+v", event)
	}
}

func TestDecisionEffectConflictIsDeadLetteredAndDoesNotBlockFreshEvents(t *testing.T) {
	base := time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC)
	repository := NewMemoryRepository()
	registry := NewRegistry()
	if err := registry.Register(ModeFunc{ModeName: "conflict", ModeVersion: "1", DecideFunc: func(_ context.Context, event Event, _ Binding, _ Snapshot) ([]PlannedEffect, error) {
		key := "shared-effect"
		if event.SubjectID == "fresh" {
			key = "fresh-effect"
		}
		return []PlannedEffect{{Type: testEffectEnqueueSubject, IdempotencyKey: key, Payload: json.RawMessage(`{"issueId":"` + event.SubjectID + `"}`)}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveBinding(context.Background(), Binding{CoordinationID: "task", Mode: "conflict", Version: "1", UpdatedAt: base}); err != nil {
		t.Fatal(err)
	}
	current := base.Add(3 * time.Second)
	engine := Engine{Repository: repository, Modes: registry, WorkerID: "decision", Now: func() time.Time { return current }}
	for index, issueID := range []string{"first", "conflicting", "fresh"} {
		_, err := engine.Submit(context.Background(), Event{ID: "event-" + issueID, Type: testEventSubjectAssigned, CoordinationID: "task", SubjectID: issueID, OccurredAt: base.Add(time.Duration(index) * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 3; index++ {
		processed, err := engine.ProcessNext(context.Background())
		if err != nil || !processed {
			t.Fatalf("decision %d processed=%v err=%v", index+1, processed, err)
		}
	}
	if processed, err := engine.ProcessNext(context.Background()); err != nil || processed {
		t.Fatalf("dead-lettered conflict remained claimable: processed=%v err=%v", processed, err)
	}

	var keys []string
	worker := EffectWorker{Repository: repository, WorkerID: "effects", Now: func() time.Time { return current }, Handler: EffectHandlerFunc(func(_ context.Context, effect Effect) error {
		keys = append(keys, effect.IdempotencyKey)
		return nil
	})}
	for {
		processed, err := worker.ProcessNext(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !processed {
			break
		}
	}
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		seen[key] = true
	}
	if len(keys) != 2 || !seen["shared-effect"] || !seen["fresh-effect"] {
		t.Fatalf("unexpected committed effects after conflict: %v", keys)
	}
}

func TestEventRetryDoesNotStarveFreshWork(t *testing.T) {
	base := time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC)
	repository := NewMemoryRepository()
	for _, event := range []Event{
		{ID: "retry", Type: testEventSubjectAssigned, CoordinationID: "task", OccurredAt: base},
		{ID: "fresh", Type: testEventSubjectAssigned, CoordinationID: "task", OccurredAt: base.Add(time.Second)},
	} {
		if _, err := repository.SubmitEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	claim, ok, err := repository.ClaimEvent(context.Background(), ClaimRequest{WorkerID: "one", Now: base.Add(2 * time.Second), LeaseDuration: time.Minute})
	if err != nil || !ok || claim.Event.ID != "retry" {
		t.Fatalf("initial claim=%+v ok=%v err=%v", claim, ok, err)
	}
	if err = repository.RetryEvent(context.Background(), claim, base.Add(2*time.Second), base.Add(2*time.Second), errors.New("temporary")); err != nil {
		t.Fatal(err)
	}
	claim, ok, err = repository.ClaimEvent(context.Background(), ClaimRequest{WorkerID: "two", Now: base.Add(2 * time.Second), LeaseDuration: time.Minute})
	if err != nil || !ok || claim.Event.ID != "fresh" {
		t.Fatalf("fresh event was starved by retry: claim=%+v ok=%v err=%v", claim, ok, err)
	}
}

func TestDelayedEffectActsAsDurableTimer(t *testing.T) {
	base := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	current := base
	repository := NewMemoryRepository()
	registry := NewRegistry()
	_ = registry.Register(ModeFunc{ModeName: "timer", ModeVersion: "1", DecideFunc: func(context.Context, Event, Binding, Snapshot) ([]PlannedEffect, error) {
		return []PlannedEffect{{Type: EffectScheduleWakeup, AvailableAt: base.Add(10 * time.Minute), Payload: json.RawMessage(`{"agentId":"agent-1"}`)}}, nil
	}})
	_ = repository.SaveBinding(context.Background(), Binding{CoordinationID: "task-1", Mode: "timer", Version: "1", UpdatedAt: base})
	engine := Engine{Repository: repository, Modes: registry, WorkerID: "decision", Now: func() time.Time { return current }}
	_, _ = engine.Submit(context.Background(), Event{ID: "timer-request", Type: testEventWaitRequested, CoordinationID: "task-1"})
	if processed, err := engine.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("decision processed=%v err=%v", processed, err)
	}
	calls := 0
	worker := EffectWorker{Repository: repository, WorkerID: "effects", Now: func() time.Time { return current }, Handler: EffectHandlerFunc(func(context.Context, Effect) error { calls++; return nil })}
	if processed, err := worker.ProcessNext(context.Background()); err != nil || processed || calls != 0 {
		t.Fatalf("early processed=%v calls=%d err=%v", processed, calls, err)
	}
	current = base.Add(10 * time.Minute)
	if processed, err := worker.ProcessNext(context.Background()); err != nil || !processed || calls != 1 {
		t.Fatalf("due processed=%v calls=%d err=%v", processed, calls, err)
	}
}

func TestEffectRetryKeepsStableIdentity(t *testing.T) {
	base := time.Now().UTC()
	current := base
	repository := NewMemoryRepository()
	registry := NewRegistry()
	_ = registry.Register(ModeFunc{ModeName: "retry", ModeVersion: "1", DecideFunc: func(context.Context, Event, Binding, Snapshot) ([]PlannedEffect, error) {
		return []PlannedEffect{{Type: testEffectSendMessage, IdempotencyKey: "relay-1"}}, nil
	}})
	_ = repository.SaveBinding(context.Background(), Binding{CoordinationID: "task", Mode: "retry", Version: "1", UpdatedAt: base})
	engine := Engine{Repository: repository, Modes: registry, WorkerID: "decision", Now: func() time.Time { return current }}
	_, _ = engine.Submit(context.Background(), Event{ID: "event", Type: testEventMessageReceived, CoordinationID: "task"})
	_, _ = engine.ProcessNext(context.Background())
	var mu sync.Mutex
	var ids []string
	attempts := 0
	worker := EffectWorker{Repository: repository, WorkerID: "effects", RetryDelay: time.Minute, Now: func() time.Time { return current }, Handler: EffectHandlerFunc(func(_ context.Context, effect Effect) error {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		ids = append(ids, effect.ID)
		if attempts == 1 {
			return errors.New("temporary")
		}
		return nil
	})}
	if processed, err := worker.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("first processed=%v err=%v", processed, err)
	}
	if processed, err := worker.ProcessNext(context.Background()); err != nil || processed {
		t.Fatalf("early retry processed=%v err=%v", processed, err)
	}
	current = current.Add(time.Minute)
	if processed, err := worker.ProcessNext(context.Background()); err != nil || !processed || len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("retry processed=%v ids=%v err=%v", processed, ids, err)
	}
}

func TestPermanentEffectFailureStopsAtConfiguredAttemptLimit(t *testing.T) {
	current := time.Now().UTC()
	repository := NewMemoryRepository()
	registry := NewRegistry()
	_ = registry.Register(ModeFunc{ModeName: "failure", ModeVersion: "1", DecideFunc: func(context.Context, Event, Binding, Snapshot) ([]PlannedEffect, error) {
		return []PlannedEffect{{Type: testEffectStartWorker}}, nil
	}})
	_ = repository.SaveBinding(context.Background(), Binding{CoordinationID: "task", Mode: "failure", Version: "1", UpdatedAt: current})
	engine := Engine{Repository: repository, Modes: registry, WorkerID: "decision", Now: func() time.Time { return current }}
	_, _ = engine.Submit(context.Background(), Event{ID: "event", Type: testEventDelegationRequested, CoordinationID: "task"})
	_, _ = engine.ProcessNext(context.Background())
	attempts := 0
	worker := EffectWorker{Repository: repository, WorkerID: "effects", MaxAttempts: 2, RetryDelay: time.Millisecond, Now: func() time.Time { return current }, Handler: EffectHandlerFunc(func(context.Context, Effect) error {
		attempts++
		return errors.New("permanent")
	})}
	if processed, err := worker.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("first processed=%v err=%v", processed, err)
	}
	current = current.Add(time.Second)
	if processed, err := worker.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("second processed=%v err=%v", processed, err)
	}
	current = current.Add(time.Second)
	if processed, err := worker.ProcessNext(context.Background()); err != nil || processed || attempts != 2 {
		t.Fatalf("after limit processed=%v attempts=%d err=%v", processed, attempts, err)
	}
}
