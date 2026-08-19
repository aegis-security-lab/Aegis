package sqlitestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"aegis/coordination"
)

func TestSQLiteCoordinationDecisionOutboxAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coordination.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	registry := coordination.NewRegistry()
	if err := registry.Register(coordination.ModeFunc{ModeName: "board", ModeVersion: "1", DecideFunc: func(context.Context, coordination.Event, coordination.Binding, coordination.Snapshot) ([]coordination.PlannedEffect, error) {
		return []coordination.PlannedEffect{{Type: coordination.EffectType("enqueue_subject"), IdempotencyKey: "enqueue:issue-1", Payload: json.RawMessage(`{"issueId":"issue-1"}`)}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBinding(context.Background(), coordination.Binding{CoordinationID: "task-1", Mode: "board", Version: "1", UpdatedAt: base}); err != nil {
		t.Fatal(err)
	}
	engine := coordination.Engine{Repository: store, Modes: registry, WorkerID: "decision", Now: func() time.Time { return base }}
	inserted, err := engine.Submit(context.Background(), coordination.Event{ID: "event-1", Type: coordination.EventType("subject_assigned"), CoordinationID: "task-1", SubjectID: "issue-1"})
	if err != nil || !inserted {
		t.Fatalf("inserted=%v err=%v", inserted, err)
	}
	if processed, err := engine.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var handled []coordination.Effect
	worker := coordination.EffectWorker{Repository: store, WorkerID: "effects", Now: func() time.Time { return base }, Handler: coordination.EffectHandlerFunc(func(_ context.Context, effect coordination.Effect) error {
		handled = append(handled, effect)
		return nil
	})}
	if processed, err := worker.ProcessNext(context.Background()); err != nil || !processed || len(handled) != 1 || handled[0].IdempotencyKey != "enqueue:issue-1" {
		t.Fatalf("processed=%v handled=%+v err=%v", processed, handled, err)
	}
	if processed, err := worker.ProcessNext(context.Background()); err != nil || processed {
		t.Fatalf("duplicate processed=%v err=%v", processed, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
}

func TestSQLiteClaimsAreAtomicAndFenced(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := time.Now().UTC()
	_ = store.SaveBinding(context.Background(), coordination.Binding{CoordinationID: "task", Mode: "unused", Version: "1", UpdatedAt: base})
	inserted, err := store.SubmitEvent(context.Background(), coordination.Event{ID: "event", Type: coordination.EventType("subject_created"), CoordinationID: "task", OccurredAt: base})
	if err != nil || !inserted {
		t.Fatalf("inserted=%v err=%v", inserted, err)
	}
	var wg sync.WaitGroup
	claims := make(chan coordination.EventClaim, 2)
	errs := make(chan error, 2)
	for _, workerID := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			claim, ok, claimErr := store.ClaimEvent(context.Background(), coordination.ClaimRequest{WorkerID: id, Now: base, LeaseDuration: time.Minute})
			if claimErr != nil {
				errs <- claimErr
				return
			}
			if ok {
				claims <- claim
			}
		}(workerID)
	}
	wg.Wait()
	close(claims)
	close(errs)
	for claimErr := range errs {
		t.Fatal(claimErr)
	}
	var claimed []coordination.EventClaim
	for claim := range claims {
		claimed = append(claimed, claim)
	}
	if len(claimed) != 1 {
		t.Fatalf("claims=%d", len(claimed))
	}
	stale := claimed[0]
	stale.Token = "forged"
	if err := store.CommitDecision(context.Background(), stale, nil, base); !errors.Is(err, coordination.ErrLeaseLost) {
		t.Fatalf("stale commit err=%v", err)
	}
	if err := store.CommitDecision(context.Background(), claimed[0], nil, base); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteCommitDecisionAcceptsExactExistingEffect(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := time.Now().UTC()
	if _, err = store.SubmitEvent(context.Background(), coordination.Event{ID: "event", Type: coordination.EventType("subject_created"), CoordinationID: "task", OccurredAt: base}); err != nil {
		t.Fatal(err)
	}
	claim, ok, err := store.ClaimEvent(context.Background(), coordination.ClaimRequest{WorkerID: "decision", Now: base, LeaseDuration: time.Minute})
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	effect := coordination.Effect{ID: "effect", EventID: "event", CoordinationID: "task", Type: coordination.EffectType("enqueue_subject"), IdempotencyKey: "enqueue:event", AvailableAt: base, Payload: json.RawMessage(`{"issueId":"issue"}`)}
	payload, err := marshal(effect)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&effectRecord{ID: effect.ID, EventID: effect.EventID, CoordinationID: effect.CoordinationID, IdempotencyKey: effect.IdempotencyKey, Status: string(coordination.StatusPending), AvailableAt: base, CreatedAt: base, UpdatedAt: base, Payload: payload}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.CommitDecision(context.Background(), claim, []coordination.Effect{effect}, base); err != nil {
		t.Fatalf("exact existing Effect was not idempotent: %v", err)
	}
	var count int64
	if err = store.db.Model(&effectRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("effect count=%d err=%v", count, err)
	}
}

func TestSQLiteRetriedEventDoesNotStarveFreshEvent(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := time.Now().UTC()
	for _, event := range []coordination.Event{
		{ID: "retry", Type: coordination.EventType("subject_created"), CoordinationID: "task", OccurredAt: base},
		{ID: "fresh", Type: coordination.EventType("subject_created"), CoordinationID: "task", OccurredAt: base.Add(time.Second)},
	} {
		if _, err = store.SubmitEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	now := base.Add(2 * time.Second)
	claim, ok, err := store.ClaimEvent(context.Background(), coordination.ClaimRequest{WorkerID: "one", Now: now, LeaseDuration: time.Minute})
	if err != nil || !ok || claim.Event.ID != "retry" {
		t.Fatalf("initial claim=%+v ok=%v err=%v", claim, ok, err)
	}
	if err = store.RetryEvent(context.Background(), claim, now, now, errors.New("temporary")); err != nil {
		t.Fatal(err)
	}
	claim, ok, err = store.ClaimEvent(context.Background(), coordination.ClaimRequest{WorkerID: "two", Now: now, LeaseDuration: time.Minute})
	if err != nil || !ok || claim.Event.ID != "fresh" {
		t.Fatalf("fresh event was starved by retry: claim=%+v ok=%v err=%v", claim, ok, err)
	}
}

func TestSQLiteDelayedEffectSurvivesUntilDue(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := time.Now().UTC()
	_ = store.SaveBinding(context.Background(), coordination.Binding{CoordinationID: "task", Mode: "timer", Version: "1", UpdatedAt: base})
	_, _ = store.SubmitEvent(context.Background(), coordination.Event{ID: "event", Type: coordination.EventType("wait_requested"), CoordinationID: "task", OccurredAt: base})
	claim, ok, err := store.ClaimEvent(context.Background(), coordination.ClaimRequest{WorkerID: "decision", Now: base, LeaseDuration: time.Minute})
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	due := base.Add(time.Hour)
	effect := coordination.Effect{ID: "effect", EventID: "event", CoordinationID: "task", Type: coordination.EffectScheduleWakeup, IdempotencyKey: "timer", AvailableAt: due, Payload: json.RawMessage(`{}`)}
	if err := store.CommitDecision(context.Background(), claim, []coordination.Effect{effect}, base); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.ClaimEffect(context.Background(), coordination.ClaimRequest{WorkerID: "effects", Now: base, LeaseDuration: time.Minute}); err != nil || ok {
		t.Fatalf("early ok=%v err=%v", ok, err)
	}
	claimed, ok, err := store.ClaimEffect(context.Background(), coordination.ClaimRequest{WorkerID: "effects", Now: due, LeaseDuration: time.Minute})
	if err != nil || !ok || claimed.Effect.AvailableAt != due {
		t.Fatalf("due claim=%+v ok=%v err=%v", claimed, ok, err)
	}
}
