package sqlitestore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aegis/agenthost"
	"aegis/coordination"
)

func TestExecutionLeaseIsFencedAndDurable(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "coordination.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	queue := &coordination.ExecutionQueue{Repository: store, Now: func() time.Time { return now }}
	execution := coordination.Execution{ID: "execution-1", MaxAttempts: 3, Spec: agenthost.ExecutionSpec{
		ExecutionID: "execution-1", AgentID: "agent", Prompt: "work", Model: agenthost.ModelRef{Provider: "test", Model: "model"},
	}}
	if err = queue.Enqueue(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	first, ok, err := store.ClaimExecution(context.Background(), coordination.ExecutionClaimRequest{WorkerID: "worker-1", Now: now, LeaseDuration: time.Minute})
	if err != nil || !ok {
		t.Fatalf("first=%+v ok=%v err=%v", first, ok, err)
	}
	available := now.Add(2 * time.Minute)
	if err = store.RetryExecution(context.Background(), coordination.RetryExecutionRequest{ExecutionID: execution.ID, LeaseToken: first.LeaseToken, Now: now.Add(time.Second), AvailableAt: available, Error: "retry"}); err != nil {
		t.Fatal(err)
	}
	second, ok, err := store.ClaimExecution(context.Background(), coordination.ExecutionClaimRequest{WorkerID: "worker-2", Now: available, LeaseDuration: time.Minute})
	if err != nil || !ok || second.LeaseToken == first.LeaseToken {
		t.Fatalf("second=%+v ok=%v err=%v", second, ok, err)
	}
	if err = store.FailExecution(context.Background(), coordination.FailExecutionRequest{ExecutionID: execution.ID, LeaseToken: first.LeaseToken, Now: available.Add(time.Second), Error: "stale"}); !errors.Is(err, coordination.ErrExecutionLeaseLost) {
		t.Fatalf("stale error=%v", err)
	}
	if err = store.CompleteExecution(context.Background(), coordination.CompleteExecutionRequest{ExecutionID: execution.ID, LeaseToken: second.LeaseToken, Now: available.Add(time.Second), Result: agenthost.Result{}}); err != nil {
		t.Fatal(err)
	}
	stored, err := queue.Get(context.Background(), execution.ID)
	if err != nil || stored.Status != coordination.ExecutionSucceeded || stored.Result == nil || stored.LeaseExpiresAt != nil {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
}

func TestExpiredLeaseAtAttemptLimitIsReclaimed(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "coordination.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	queue := &coordination.ExecutionQueue{Repository: store, Now: func() time.Time { return now }}
	execution := coordination.Execution{ID: "execution-restart", MaxAttempts: 1, Spec: agenthost.ExecutionSpec{
		ExecutionID: "execution-restart", AgentID: "agent", Prompt: "work", Model: agenthost.ModelRef{Provider: "test", Model: "model"},
	}}
	if err = queue.Enqueue(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	first, ok, err := store.ClaimExecution(context.Background(), coordination.ExecutionClaimRequest{WorkerID: "worker-1", Now: now, LeaseDuration: time.Minute})
	if err != nil || !ok || first.Execution.Attempt != 1 {
		t.Fatalf("first=%+v ok=%v err=%v", first, ok, err)
	}
	second, ok, err := store.ClaimExecution(context.Background(), coordination.ExecutionClaimRequest{WorkerID: "worker-2", Now: now.Add(time.Minute), LeaseDuration: time.Minute})
	if err != nil || !ok {
		t.Fatalf("expired lease was not reclaimed: second=%+v ok=%v err=%v", second, ok, err)
	}
	if second.Execution.Attempt != 1 || second.LeaseToken == first.LeaseToken || second.Execution.Status != coordination.ExecutionRunning {
		t.Fatalf("lease reclaim consumed an attempt: first=%+v second=%+v", first, second)
	}
}

func TestUrgentExecutionPriorityAndPromptMergeAreDurable(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "coordination.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	queue := &coordination.ExecutionQueue{Repository: store, Now: func() time.Time { return now }}
	for _, execution := range []coordination.Execution{
		{ID: "normal", Spec: agenthost.ExecutionSpec{ExecutionID: "normal", AgentID: "agent", Prompt: "normal", Model: agenthost.ModelRef{Provider: "test", Model: "model"}}},
		{ID: "urgent", Priority: coordination.ExecutionPriorityWakeup, Spec: agenthost.ExecutionSpec{ExecutionID: "urgent", AgentID: "agent", Prompt: "wake", Model: agenthost.ModelRef{Provider: "test", Model: "model"}, Values: map[string]any{"control.resumePrompt": "wake"}}},
	} {
		if err = queue.Enqueue(context.Background(), execution); err != nil {
			t.Fatal(err)
		}
	}
	if err = queue.AppendPrompt(context.Background(), "urgent", "budget-child", "budget exceeded evidence"); err != nil {
		t.Fatal(err)
	}
	stored, err := queue.Get(context.Background(), "urgent")
	if err != nil || stored.Priority != coordination.ExecutionPriorityWakeup || !strings.Contains(stored.Spec.Prompt, "budget exceeded evidence") || stored.Spec.Values["control.resumePrompt"] != stored.Spec.Prompt {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	claim, ok, err := store.ClaimExecution(context.Background(), coordination.ExecutionClaimRequest{WorkerID: "reserved", Now: now, LeaseDuration: time.Minute, MinimumPriority: coordination.ExecutionPriorityWakeup})
	if err != nil || !ok || claim.Execution.ID != "urgent" {
		t.Fatalf("claim=%+v ok=%v err=%v", claim, ok, err)
	}
	if err = queue.AppendPrompt(context.Background(), "urgent", "late", "too late"); !errors.Is(err, coordination.ErrExecutionNotQueued) {
		t.Fatalf("running append error=%v", err)
	}
}

func TestOpenPromotesQueuedResumeCreatedBeforePrioritySupport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coordination.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	queue := &coordination.ExecutionQueue{Repository: store, Now: func() time.Time { return now }}
	execution := coordination.Execution{ID: "legacy-resume", Spec: agenthost.ExecutionSpec{
		ExecutionID: "legacy-resume", AgentID: "agent", Prompt: "wake", Model: agenthost.ModelRef{Provider: "test", Model: "model"},
		Values: map[string]any{"control.resume": true},
	}}
	if err = queue.Enqueue(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	legacy, err := queue.Get(context.Background(), execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacy.Priority = coordination.ExecutionPriorityNormal
	record, err := encodeExecution(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&executionRecord{}).Where("id = ?", execution.ID).Updates(executionRecordValues(record)).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	promoted, err := reopened.Execution(context.Background(), execution.ID)
	if err != nil || promoted.Priority != coordination.ExecutionPriorityWakeup {
		t.Fatalf("promoted=%+v err=%v", promoted, err)
	}
}
