package sqlitestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aegis/coordination"
	"gorm.io/gorm"
)

func (s *Store) EnqueueExecution(ctx context.Context, execution coordination.Execution) error {
	record, err := encodeExecution(execution)
	if err != nil {
		return err
	}
	err = s.db.WithContext(nonNil(ctx)).Create(&record).Error
	if isUniqueError(err) {
		return coordination.ErrExecutionConflict
	}
	return wrap("enqueue execution", err)
}

func (s *Store) Execution(ctx context.Context, id string) (coordination.Execution, error) {
	var record executionRecord
	err := s.db.WithContext(nonNil(ctx)).First(&record, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return coordination.Execution{}, coordination.ErrExecutionNotFound
	}
	if err != nil {
		return coordination.Execution{}, wrap("get execution", err)
	}
	return decodeExecution(record)
}

func (s *Store) AppendExecutionPrompt(ctx context.Context, request coordination.AppendExecutionPromptRequest) error {
	request.ExecutionID, request.Key, request.Message = strings.TrimSpace(request.ExecutionID), strings.TrimSpace(request.Key), strings.TrimSpace(request.Message)
	if request.ExecutionID == "" || request.Key == "" || request.Message == "" {
		return errors.New("coordination/sqlitestore: execution prompt append requires execution ID, key and message")
	}
	return s.db.WithContext(nonNil(ctx)).Transaction(func(tx *gorm.DB) error {
		var record executionRecord
		if err := tx.First(&record, "id = ?", request.ExecutionID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return coordination.ErrExecutionNotFound
		} else if err != nil {
			return wrap("find queued execution for prompt append", err)
		}
		if record.Status != string(coordination.ExecutionQueued) {
			return coordination.ErrExecutionNotQueued
		}
		execution, err := decodeExecution(record)
		if err != nil {
			return err
		}
		marker := `<coordination_update id="` + request.Key + `">`
		if strings.Contains(execution.Spec.Prompt, marker) {
			return nil
		}
		execution.Spec.Prompt = strings.TrimSpace(execution.Spec.Prompt) + "\n\n" + marker + "\n" + request.Message + "\n</coordination_update>"
		if execution.Spec.Values == nil {
			execution.Spec.Values = map[string]any{}
		}
		execution.Spec.Values["control.resumePrompt"] = execution.Spec.Prompt
		execution.UpdatedAt = request.Now.UTC()
		encoded, err := encodeExecution(execution)
		if err != nil {
			return err
		}
		updated := tx.Model(&executionRecord{}).Where("id = ? AND status = ?", request.ExecutionID, string(coordination.ExecutionQueued)).Updates(executionRecordValues(encoded))
		if updated.Error != nil {
			return wrap("append queued execution prompt", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return coordination.ErrExecutionNotQueued
		}
		return nil
	})
}

func (s *Store) ClaimExecution(ctx context.Context, request coordination.ExecutionClaimRequest) (coordination.ExecutionClaim, bool, error) {
	if strings.TrimSpace(request.WorkerID) == "" || request.LeaseDuration <= 0 {
		return coordination.ExecutionClaim{}, false, errors.New("coordination/sqlitestore: invalid execution claim request")
	}
	for scan := 0; scan < 32; scan++ {
		var records []executionRecord
		query := s.db.WithContext(nonNil(ctx)).Where(
			"(status = ? AND available_at <= ?) OR (status = ? AND lease_expires_at <= ?)",
			string(coordination.ExecutionQueued), request.Now, string(coordination.ExecutionRunning), request.Now,
		)
		if request.MinimumPriority > 0 {
			query = query.Where("priority >= ?", request.MinimumPriority)
		}
		err := query.Order("priority DESC, available_at ASC, created_at ASC, id ASC").Limit(1).Find(&records).Error
		if err != nil {
			return coordination.ExecutionClaim{}, false, wrap("find execution claim", err)
		}
		if len(records) == 0 {
			return coordination.ExecutionClaim{}, false, nil
		}
		record := records[0]
		execution, err := decodeExecution(record)
		if err != nil {
			return coordination.ExecutionClaim{}, false, err
		}
		reclaimingExpiredLease := execution.Status == coordination.ExecutionRunning
		if !reclaimingExpiredLease && execution.Attempt >= execution.MaxAttempts {
			now := request.Now.UTC()
			execution.Status, execution.LastError = coordination.ExecutionFailed, "execution exhausted its configured attempts"
			execution.UpdatedAt, execution.FinishedAt = now, &now
			clearExecutionLease(&execution)
			updated, updateErr := s.updateRunnableExecution(ctx, record, execution, request.Now)
			if updateErr != nil {
				return coordination.ExecutionClaim{}, false, updateErr
			}
			if !updated {
				continue
			}
			continue
		}
		token, err := leaseToken()
		if err != nil {
			return coordination.ExecutionClaim{}, false, err
		}
		now := request.Now.UTC()
		expires := now.Add(request.LeaseDuration)
		execution.Status, execution.UpdatedAt = coordination.ExecutionRunning, now
		// Expired leases are infrastructure interruptions, not completed
		// attempts. Preserve the attempt number so MaxAttempts continues to
		// describe Agent/model failures only.
		if !reclaimingExpiredLease {
			execution.Attempt++
		}
		if execution.StartedAt == nil {
			execution.StartedAt = &now
		}
		execution.LeaseOwner, execution.LeaseToken, execution.LeaseExpiresAt = request.WorkerID, token, &expires
		updated, err := s.updateRunnableExecution(ctx, record, execution, request.Now)
		if err != nil {
			return coordination.ExecutionClaim{}, false, err
		}
		if updated {
			return coordination.ExecutionClaim{Execution: execution, LeaseToken: token}, true, nil
		}
	}
	return coordination.ExecutionClaim{}, false, nil
}

func (s *Store) RenewExecution(ctx context.Context, request coordination.ExecutionLeaseRequest) error {
	if !request.ExpiresAt.After(request.Now) {
		return errors.New("coordination/sqlitestore: renewed execution lease must expire in the future")
	}
	return s.mutateExecutionLease(ctx, request.ExecutionID, request.LeaseToken, request.Now, func(execution *coordination.Execution) {
		expires := request.ExpiresAt.UTC()
		execution.LeaseExpiresAt, execution.UpdatedAt = &expires, request.Now.UTC()
	})
}

func (s *Store) CompleteExecution(ctx context.Context, request coordination.CompleteExecutionRequest) error {
	result := request.Result
	return s.mutateExecutionLease(ctx, request.ExecutionID, request.LeaseToken, request.Now, func(execution *coordination.Execution) {
		now := request.Now.UTC()
		execution.Status, execution.UpdatedAt, execution.FinishedAt = coordination.ExecutionSucceeded, now, &now
		execution.Result, execution.LastError = &result, ""
		clearExecutionLease(execution)
	})
}

func (s *Store) RetryExecution(ctx context.Context, request coordination.RetryExecutionRequest) error {
	return s.mutateExecutionLease(ctx, request.ExecutionID, request.LeaseToken, request.Now, func(execution *coordination.Execution) {
		execution.Status, execution.UpdatedAt = coordination.ExecutionQueued, request.Now.UTC()
		execution.AvailableAt, execution.LastError = request.AvailableAt.UTC(), request.Error
		clearExecutionLease(execution)
	})
}

func (s *Store) FailExecution(ctx context.Context, request coordination.FailExecutionRequest) error {
	return s.mutateExecutionLease(ctx, request.ExecutionID, request.LeaseToken, request.Now, func(execution *coordination.Execution) {
		now := request.Now.UTC()
		execution.Status, execution.UpdatedAt, execution.FinishedAt = coordination.ExecutionFailed, now, &now
		execution.LastError = request.Error
		clearExecutionLease(execution)
	})
}

func (s *Store) CancelExecution(ctx context.Context, id string, now time.Time, reason string) error {
	execution, err := s.Execution(ctx, id)
	if err != nil {
		return err
	}
	if execution.Status == coordination.ExecutionSucceeded || execution.Status == coordination.ExecutionFailed || execution.Status == coordination.ExecutionCancelled {
		return nil
	}
	now = now.UTC()
	execution.Status, execution.UpdatedAt, execution.FinishedAt = coordination.ExecutionCancelled, now, &now
	execution.LastError = reason
	clearExecutionLease(&execution)
	record, err := encodeExecution(execution)
	if err != nil {
		return err
	}
	result := s.db.WithContext(nonNil(ctx)).Model(&executionRecord{}).Where("id = ? AND status NOT IN ?", id, []string{
		string(coordination.ExecutionSucceeded), string(coordination.ExecutionFailed), string(coordination.ExecutionCancelled),
	}).Updates(executionRecordValues(record))
	return wrap("cancel execution", result.Error)
}

func (s *Store) CancelCoordinationExecutions(ctx context.Context, coordinationID string, now time.Time, reason string) (int64, error) {
	now = now.UTC()
	var records []executionRecord
	terminal := []string{string(coordination.ExecutionSucceeded), string(coordination.ExecutionFailed), string(coordination.ExecutionCancelled)}
	if err := s.db.WithContext(nonNil(ctx)).Where("coordination_id = ? AND status NOT IN ?", coordinationID, terminal).Find(&records).Error; err != nil {
		return 0, wrap("find coordination executions to cancel", err)
	}
	var count int64
	for _, record := range records {
		execution, err := decodeExecution(record)
		if err != nil {
			return count, err
		}
		execution.Status, execution.UpdatedAt, execution.FinishedAt = coordination.ExecutionCancelled, now, &now
		execution.LastError = reason
		clearExecutionLease(&execution)
		encoded, err := encodeExecution(execution)
		if err != nil {
			return count, err
		}
		result := s.db.WithContext(nonNil(ctx)).Model(&executionRecord{}).Where("id = ? AND status NOT IN ?", execution.ID, terminal).Updates(executionRecordValues(encoded))
		if result.Error != nil {
			return count, wrap("cancel coordination execution", result.Error)
		}
		count += result.RowsAffected
	}
	return count, nil
}

func (s *Store) mutateExecutionLease(ctx context.Context, id, token string, now time.Time, mutate func(*coordination.Execution)) error {
	execution, err := s.Execution(ctx, id)
	if err != nil {
		return err
	}
	if execution.Status != coordination.ExecutionRunning || token == "" || execution.LeaseToken != token || execution.LeaseExpiresAt == nil || !now.Before(*execution.LeaseExpiresAt) {
		return coordination.ErrExecutionLeaseLost
	}
	mutate(&execution)
	record, err := encodeExecution(execution)
	if err != nil {
		return err
	}
	result := s.db.WithContext(nonNil(ctx)).Model(&executionRecord{}).Where(
		"id = ? AND status = ? AND lease_token = ? AND lease_expires_at > ?", id, string(coordination.ExecutionRunning), token, now,
	).Updates(executionRecordValues(record))
	if result.Error != nil {
		return wrap("fenced execution update", result.Error)
	}
	if result.RowsAffected != 1 {
		return coordination.ErrExecutionLeaseLost
	}
	return nil
}

func (s *Store) updateRunnableExecution(ctx context.Context, previous executionRecord, execution coordination.Execution, now time.Time) (bool, error) {
	record, err := encodeExecution(execution)
	if err != nil {
		return false, err
	}
	query := s.db.WithContext(nonNil(ctx)).Model(&executionRecord{}).Where("id = ?", previous.ID)
	if previous.Status == string(coordination.ExecutionQueued) {
		query = query.Where("status = ? AND available_at <= ?", previous.Status, now)
	} else {
		query = query.Where("status = ? AND lease_token = ? AND lease_expires_at <= ?", previous.Status, previous.LeaseToken, now)
	}
	result := query.Updates(executionRecordValues(record))
	return result.RowsAffected == 1, wrap("claim execution update", result.Error)
}

func encodeExecution(execution coordination.Execution) (executionRecord, error) {
	payload, err := json.Marshal(execution)
	if err != nil {
		return executionRecord{}, fmt.Errorf("coordination/sqlitestore: encode execution: %w", err)
	}
	return executionRecord{ID: execution.ID, CoordinationID: execution.CoordinationID, Status: string(execution.Status), Priority: execution.Priority, AvailableAt: execution.AvailableAt, CreatedAt: execution.CreatedAt, UpdatedAt: execution.UpdatedAt, LeaseToken: execution.LeaseToken, LeaseExpiresAt: execution.LeaseExpiresAt, Payload: string(payload)}, nil
}

func decodeExecution(record executionRecord) (coordination.Execution, error) {
	var execution coordination.Execution
	if err := json.Unmarshal([]byte(record.Payload), &execution); err != nil {
		return coordination.Execution{}, fmt.Errorf("coordination/sqlitestore: decode execution %q: %w", record.ID, err)
	}
	return execution, nil
}

func executionRecordValues(record executionRecord) map[string]any {
	return map[string]any{
		"coordination_id": record.CoordinationID, "status": record.Status, "priority": record.Priority, "available_at": record.AvailableAt, "created_at": record.CreatedAt, "updated_at": record.UpdatedAt,
		"lease_token": record.LeaseToken, "lease_expires_at": record.LeaseExpiresAt, "payload": record.Payload,
	}
}

func clearExecutionLease(execution *coordination.Execution) {
	execution.LeaseOwner, execution.LeaseToken, execution.LeaseExpiresAt = "", "", nil
}
