package coordination

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"aegis/agenthost"
)

// MemoryExecutionRepository mirrors the durable lease semantics for tests and
// small embedded runtimes.
type MemoryExecutionRepository struct {
	mu         sync.Mutex
	executions map[string]Execution
	sequence   uint64
}

func NewMemoryExecutionRepository() *MemoryExecutionRepository {
	return &MemoryExecutionRepository{executions: make(map[string]Execution)}
}

func (r *MemoryExecutionRepository) EnqueueExecution(_ context.Context, execution Execution) error {
	copy, err := copyExecution(execution)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.executions[execution.ID]; exists {
		return ErrExecutionConflict
	}
	r.executions[execution.ID] = copy
	return nil
}

func (r *MemoryExecutionRepository) Execution(_ context.Context, id string) (Execution, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	execution, exists := r.executions[id]
	if !exists {
		return Execution{}, ErrExecutionNotFound
	}
	return copyExecution(execution)
}

func (r *MemoryExecutionRepository) AppendExecutionPrompt(_ context.Context, request AppendExecutionPromptRequest) error {
	request.ExecutionID, request.Key, request.Message = strings.TrimSpace(request.ExecutionID), strings.TrimSpace(request.Key), strings.TrimSpace(request.Message)
	if request.ExecutionID == "" || request.Key == "" || request.Message == "" {
		return errors.New("coordination: execution prompt append requires execution ID, key and message")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	execution, exists := r.executions[request.ExecutionID]
	if !exists {
		return ErrExecutionNotFound
	}
	if execution.Status != ExecutionQueued {
		return ErrExecutionNotQueued
	}
	appendCoordinationPrompt(&execution, request.Key, request.Message)
	execution.UpdatedAt = request.Now.UTC()
	r.executions[execution.ID] = execution
	return nil
}

func (r *MemoryExecutionRepository) ClaimExecution(_ context.Context, request ExecutionClaimRequest) (ExecutionClaim, bool, error) {
	if strings.TrimSpace(request.WorkerID) == "" || request.LeaseDuration <= 0 {
		return ExecutionClaim{}, false, errors.New("coordination: invalid execution claim request")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]Execution, 0, len(r.executions))
	for _, execution := range r.executions {
		if execution.Priority >= request.MinimumPriority && executionRunnable(execution, request.Now) {
			items = append(items, execution)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority > items[j].Priority
		}
		if !items[i].AvailableAt.Equal(items[j].AvailableAt) {
			return items[i].AvailableAt.Before(items[j].AvailableAt)
		}
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return items[i].ID < items[j].ID
	})
	for _, candidate := range items {
		execution := r.executions[candidate.ID]
		reclaimingExpiredLease := execution.Status == ExecutionRunning
		if !reclaimingExpiredLease && execution.Attempt >= execution.MaxAttempts {
			now := request.Now.UTC()
			execution.Status, execution.LastError = ExecutionFailed, "execution exhausted its configured attempts"
			execution.UpdatedAt, execution.FinishedAt = now, &now
			clearExecutionLease(&execution)
			r.executions[execution.ID] = execution
			continue
		}
		r.sequence++
		now := request.Now.UTC()
		expires := now.Add(request.LeaseDuration)
		execution.Status, execution.UpdatedAt = ExecutionRunning, now
		// A lease expiry means the worker disappeared before it could report an
		// outcome. Reclaim the same attempt instead of treating infrastructure
		// interruption as another model/Agent failure.
		if !reclaimingExpiredLease {
			execution.Attempt++
		}
		if execution.StartedAt == nil {
			execution.StartedAt = &now
		}
		execution.LeaseOwner = request.WorkerID
		execution.LeaseToken = fmt.Sprintf("execution-lease-%d", r.sequence)
		execution.LeaseExpiresAt = &expires
		r.executions[execution.ID] = execution
		copy, err := copyExecution(execution)
		return ExecutionClaim{Execution: copy, LeaseToken: execution.LeaseToken}, true, err
	}
	return ExecutionClaim{}, false, nil
}

func appendCoordinationPrompt(execution *Execution, key, message string) {
	if execution == nil {
		return
	}
	marker := `<coordination_update id="` + key + `">`
	if strings.Contains(execution.Spec.Prompt, marker) {
		return
	}
	execution.Spec.Prompt = strings.TrimSpace(execution.Spec.Prompt) + "\n\n" + marker + "\n" + strings.TrimSpace(message) + "\n</coordination_update>"
	if execution.Spec.Values == nil {
		execution.Spec.Values = map[string]any{}
	}
	execution.Spec.Values["control.resumePrompt"] = execution.Spec.Prompt
}

func (r *MemoryExecutionRepository) RenewExecution(_ context.Context, request ExecutionLeaseRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	execution, err := r.leased(request.ExecutionID, request.LeaseToken, request.Now)
	if err != nil {
		return err
	}
	if !request.ExpiresAt.After(request.Now) {
		return errors.New("coordination: renewed execution lease must expire in the future")
	}
	expires := request.ExpiresAt.UTC()
	execution.LeaseExpiresAt, execution.UpdatedAt = &expires, request.Now.UTC()
	r.executions[execution.ID] = execution
	return nil
}

func (r *MemoryExecutionRepository) CompleteExecution(_ context.Context, request CompleteExecutionRequest) error {
	result, err := copyExecutionResult(request.Result)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	execution, err := r.leased(request.ExecutionID, request.LeaseToken, request.Now)
	if err != nil {
		return err
	}
	now := request.Now.UTC()
	execution.Status, execution.UpdatedAt, execution.FinishedAt = ExecutionSucceeded, now, &now
	execution.Result, execution.LastError = &result, ""
	clearExecutionLease(&execution)
	r.executions[execution.ID] = execution
	return nil
}

func (r *MemoryExecutionRepository) RetryExecution(_ context.Context, request RetryExecutionRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	execution, err := r.leased(request.ExecutionID, request.LeaseToken, request.Now)
	if err != nil {
		return err
	}
	execution.Status, execution.UpdatedAt = ExecutionQueued, request.Now.UTC()
	execution.AvailableAt, execution.LastError = request.AvailableAt.UTC(), request.Error
	clearExecutionLease(&execution)
	r.executions[execution.ID] = execution
	return nil
}

func (r *MemoryExecutionRepository) FailExecution(_ context.Context, request FailExecutionRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	execution, err := r.leased(request.ExecutionID, request.LeaseToken, request.Now)
	if err != nil {
		return err
	}
	now := request.Now.UTC()
	execution.Status, execution.UpdatedAt, execution.FinishedAt = ExecutionFailed, now, &now
	execution.LastError = request.Error
	clearExecutionLease(&execution)
	r.executions[execution.ID] = execution
	return nil
}

func (r *MemoryExecutionRepository) CancelExecution(_ context.Context, id string, now time.Time, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	execution, exists := r.executions[id]
	if !exists {
		return ErrExecutionNotFound
	}
	if executionTerminal(execution.Status) {
		return nil
	}
	now = now.UTC()
	execution.Status, execution.UpdatedAt, execution.FinishedAt = ExecutionCancelled, now, &now
	execution.LastError = reason
	clearExecutionLease(&execution)
	r.executions[id] = execution
	return nil
}

func (r *MemoryExecutionRepository) CancelCoordinationExecutions(_ context.Context, coordinationID string, now time.Time, reason string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var count int64
	for id, execution := range r.executions {
		if execution.CoordinationID != coordinationID || executionTerminal(execution.Status) {
			continue
		}
		finished := now.UTC()
		execution.Status, execution.UpdatedAt, execution.FinishedAt = ExecutionCancelled, finished, &finished
		execution.LastError = reason
		clearExecutionLease(&execution)
		r.executions[id] = execution
		count++
	}
	return count, nil
}

func (r *MemoryExecutionRepository) leased(id, token string, now time.Time) (Execution, error) {
	execution, exists := r.executions[id]
	if !exists {
		return Execution{}, ErrExecutionNotFound
	}
	if execution.Status != ExecutionRunning || token == "" || execution.LeaseToken != token || execution.LeaseExpiresAt == nil || !now.Before(*execution.LeaseExpiresAt) {
		return Execution{}, ErrExecutionLeaseLost
	}
	return execution, nil
}

func executionRunnable(execution Execution, now time.Time) bool {
	if execution.Status == ExecutionQueued {
		return !execution.AvailableAt.After(now)
	}
	return execution.Status == ExecutionRunning && execution.LeaseExpiresAt != nil && !execution.LeaseExpiresAt.After(now)
}

func executionTerminal(status ExecutionStatus) bool {
	return status == ExecutionSucceeded || status == ExecutionFailed || status == ExecutionCancelled
}

func clearExecutionLease(execution *Execution) {
	execution.LeaseOwner, execution.LeaseToken, execution.LeaseExpiresAt = "", "", nil
}

func copyExecution(execution Execution) (Execution, error) {
	data, err := json.Marshal(execution)
	if err != nil {
		return Execution{}, fmt.Errorf("coordination: execution is not durable: %w", err)
	}
	var copy Execution
	if err := json.Unmarshal(data, &copy); err != nil {
		return Execution{}, fmt.Errorf("coordination: copy execution: %w", err)
	}
	return copy, nil
}

func copyExecutionResult(result agenthost.Result) (agenthost.Result, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return agenthost.Result{}, fmt.Errorf("coordination: execution result is not durable: %w", err)
	}
	var copy agenthost.Result
	if err := json.Unmarshal(data, &copy); err != nil {
		return agenthost.Result{}, fmt.Errorf("coordination: copy execution result: %w", err)
	}
	return copy, nil
}
