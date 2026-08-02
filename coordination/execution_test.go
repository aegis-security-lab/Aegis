package coordination

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aegis/agenthost"
	"github.com/z3r2ne/agentcore"
)

type executionRunnerFunc func(context.Context, agenthost.ExecutionSpec, agentcore.EventSink) (agenthost.Result, error)

func (f executionRunnerFunc) Run(ctx context.Context, spec agenthost.ExecutionSpec, sink agentcore.EventSink) (agenthost.Result, error) {
	return f(ctx, spec, sink)
}

func validExecution(id string) Execution {
	return Execution{ID: id, Spec: agenthost.ExecutionSpec{
		ExecutionID: id, AgentID: "agent", Prompt: "work",
		Model: agenthost.ModelRef{Provider: "test", Model: "model"},
	}}
}

func TestExecutionQueueAndWorkerAreOwnedByCoordination(t *testing.T) {
	repository := NewMemoryExecutionRepository()
	queue := &ExecutionQueue{Repository: repository}
	worker := &ExecutionWorker{Repository: repository, WorkerID: "worker", Executor: executionRunnerFunc(func(_ context.Context, spec agenthost.ExecutionSpec, _ agentcore.EventSink) (agenthost.Result, error) {
		message := agentcore.TextMessage(agentcore.RoleAssistant, "done "+spec.Prompt)
		return agenthost.Result{Core: agentcore.Result{State: agentcore.State{Messages: []agentcore.Message{message}}, StopReason: agentcore.StopReasonStop}}, nil
	})}
	if err := queue.Enqueue(context.Background(), validExecution("execution-1")); err != nil {
		t.Fatal(err)
	}
	outcome, claimed, err := worker.RunNext(context.Background())
	if err != nil || !claimed || outcome.Status != ExecutionSucceeded {
		t.Fatalf("outcome=%+v claimed=%v err=%v", outcome, claimed, err)
	}
	stored, err := queue.Get(context.Background(), "execution-1")
	if err != nil || stored.Result == nil || stored.Result.Core.State.Messages[0].Text() != "done work" {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
}

func TestExecutionWorkerRetriesWithoutASecondControlPlane(t *testing.T) {
	repository := NewMemoryExecutionRepository()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	queue := &ExecutionQueue{Repository: repository, Now: func() time.Time { return now }}
	calls := 0
	worker := &ExecutionWorker{
		Repository: repository, WorkerID: "worker", Now: func() time.Time { return now },
		Executor: executionRunnerFunc(func(context.Context, agenthost.ExecutionSpec, agentcore.EventSink) (agenthost.Result, error) {
			calls++
			return agenthost.Result{}, errors.New("temporary")
		}),
		Retry: ExecutionRetryPolicyFunc(func(Execution, error) (time.Duration, bool) { return time.Minute, true }),
	}
	execution := validExecution("execution-retry")
	execution.MaxAttempts = 2
	if err := queue.Enqueue(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	outcome, claimed, err := worker.RunNext(context.Background())
	if err != nil || !claimed || outcome.Status != ExecutionQueued || calls != 1 {
		t.Fatalf("outcome=%+v claimed=%v calls=%d err=%v", outcome, claimed, calls, err)
	}
	stored, _ := queue.Get(context.Background(), execution.ID)
	if stored.Status != ExecutionQueued || !stored.AvailableAt.Equal(now.Add(time.Minute)) || stored.LastError != "temporary" {
		t.Fatalf("stored=%+v", stored)
	}
}

func TestExpiredWorkerLeaseReclaimsSameAttempt(t *testing.T) {
	repository := NewMemoryExecutionRepository()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	queue := &ExecutionQueue{Repository: repository, Now: func() time.Time { return now }}
	execution := validExecution("execution-worker-restart")
	execution.MaxAttempts = 1
	if err := queue.Enqueue(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	first, ok, err := repository.ClaimExecution(context.Background(), ExecutionClaimRequest{WorkerID: "worker-1", Now: now, LeaseDuration: time.Minute})
	if err != nil || !ok || first.Execution.Attempt != 1 {
		t.Fatalf("first=%+v ok=%v err=%v", first, ok, err)
	}
	second, ok, err := repository.ClaimExecution(context.Background(), ExecutionClaimRequest{WorkerID: "worker-2", Now: now.Add(time.Minute), LeaseDuration: time.Minute})
	if err != nil || !ok {
		t.Fatalf("expired lease was not reclaimable: second=%+v ok=%v err=%v", second, ok, err)
	}
	if second.Execution.Attempt != 1 || second.LeaseToken == first.LeaseToken || second.Execution.Status != ExecutionRunning {
		t.Fatalf("lease reclaim consumed an Agent attempt: first=%+v second=%+v", first, second)
	}
}

func TestExecutionQueueCancelsWholeCoordinationScope(t *testing.T) {
	repository := NewMemoryExecutionRepository()
	queue := &ExecutionQueue{Repository: repository}
	for _, id := range []string{"parent", "direct-child"} {
		execution := validExecution(id)
		execution.CoordinationID = "task-1"
		if err := queue.Enqueue(context.Background(), execution); err != nil {
			t.Fatal(err)
		}
	}
	count, err := queue.CancelCoordination(context.Background(), "task-1", "operator cancelled")
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	for _, id := range []string{"parent", "direct-child"} {
		execution, getErr := queue.Get(context.Background(), id)
		if getErr != nil || execution.Status != ExecutionCancelled {
			t.Fatalf("execution=%+v err=%v", execution, getErr)
		}
	}
}

func TestReservedWakeupWorkerClaimsUrgentExecutionWhileNormalWorkRemainsQueued(t *testing.T) {
	repository := NewMemoryExecutionRepository()
	queue := &ExecutionQueue{Repository: repository}
	normal := validExecution("normal-work")
	urgent := validExecution("parent-wakeup")
	urgent.Priority = ExecutionPriorityWakeup
	if err := queue.Enqueue(context.Background(), normal); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(context.Background(), urgent); err != nil {
		t.Fatal(err)
	}
	claimedID := ""
	worker := &ExecutionWorker{
		Repository: repository, WorkerID: "reserved-wakeup", MinimumPriority: ExecutionPriorityWakeup,
		Executor: executionRunnerFunc(func(_ context.Context, spec agenthost.ExecutionSpec, _ agentcore.EventSink) (agenthost.Result, error) {
			claimedID = spec.ExecutionID
			return agenthost.Result{}, nil
		}),
	}
	if outcome, claimed, err := worker.RunNext(context.Background()); err != nil || !claimed || outcome.Status != ExecutionSucceeded {
		t.Fatalf("outcome=%+v claimed=%v err=%v", outcome, claimed, err)
	}
	if claimedID != urgent.ID {
		t.Fatalf("reserved worker claimed %q, want %q", claimedID, urgent.ID)
	}
	stored, err := queue.Get(context.Background(), normal.ID)
	if err != nil || stored.Status != ExecutionQueued {
		t.Fatalf("normal execution=%+v err=%v", stored, err)
	}
}

func TestQueuedExecutionPromptMergeIsIdempotent(t *testing.T) {
	repository := NewMemoryExecutionRepository()
	queue := &ExecutionQueue{Repository: repository}
	execution := validExecution("parent-wakeup")
	execution.Spec.Values = map[string]any{"control.resumePrompt": "initial"}
	if err := queue.Enqueue(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := queue.AppendPrompt(context.Background(), execution.ID, "child-budget-1", "child exceeded its budget"); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := queue.Get(context.Background(), execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(stored.Spec.Prompt, "child exceeded its budget") != 1 {
		t.Fatalf("merged prompt=%q", stored.Spec.Prompt)
	}
	if stored.Spec.Values["control.resumePrompt"] != stored.Spec.Prompt {
		t.Fatalf("resume prompt was not synchronized: %+v", stored.Spec.Values)
	}
}
