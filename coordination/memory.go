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
)

type MemoryRepository struct {
	mu       sync.Mutex
	bindings map[string]Binding
	events   map[string]memoryEvent
	effects  map[string]memoryEffect
	keys     map[string]string
	sequence uint64
}

type memoryEvent struct {
	event       Event
	status      Status
	availableAt time.Time
	leaseToken  string
	leaseUntil  time.Time
	attempts    int
	lastError   string
}

type memoryEffect struct {
	effect     Effect
	status     Status
	leaseToken string
	leaseUntil time.Time
	attempts   int
	lastError  string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		bindings: make(map[string]Binding), events: make(map[string]memoryEvent),
		effects: make(map[string]memoryEffect), keys: make(map[string]string),
	}
}

func (r *MemoryRepository) SaveBinding(_ context.Context, binding Binding) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	copy, err := durableCopy(binding)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.bindings[binding.CoordinationID] = copy
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) Binding(_ context.Context, id string) (Binding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	binding, ok := r.bindings[id]
	if !ok {
		return Binding{}, ErrNotFound
	}
	return durableCopy(binding)
}

func (r *MemoryRepository) SubmitEvent(_ context.Context, event Event) (bool, error) {
	if err := event.Validate(); err != nil {
		return false, err
	}
	copy, err := durableCopy(event)
	if err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.events[event.ID]; exists {
		return false, nil
	}
	r.events[event.ID] = memoryEvent{event: copy, status: StatusPending, availableAt: event.OccurredAt.UTC()}
	return true, nil
}

func (r *MemoryRepository) ClaimEvent(_ context.Context, request ClaimRequest) (EventClaim, bool, error) {
	if err := validateClaim(request); err != nil {
		return EventClaim{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.events))
	for id, record := range r.events {
		if record.status == StatusPending && !record.availableAt.After(request.Now) || record.status == StatusProcessing && !record.leaseUntil.After(request.Now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := r.events[ids[i]], r.events[ids[j]]
		if a.attempts != b.attempts {
			return a.attempts < b.attempts
		}
		if !a.availableAt.Equal(b.availableAt) {
			return a.availableAt.Before(b.availableAt)
		}
		return ids[i] < ids[j]
	})
	if len(ids) == 0 {
		return EventClaim{}, false, nil
	}
	id := ids[0]
	record := r.events[id]
	r.sequence++
	record.status, record.attempts = StatusProcessing, record.attempts+1
	record.leaseToken = fmt.Sprintf("event-lease-%d", r.sequence)
	record.leaseUntil = request.Now.UTC().Add(request.LeaseDuration)
	r.events[id] = record
	event, err := durableCopy(record.event)
	return EventClaim{Event: event, Token: record.leaseToken, Attempt: record.attempts}, true, err
}

func (r *MemoryRepository) CommitDecision(_ context.Context, claim EventClaim, effects []Effect, now time.Time) error {
	copies := make([]Effect, len(effects))
	for index, effect := range effects {
		copy, err := durableCopy(effect)
		if err != nil {
			return err
		}
		copies[index] = copy
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record, err := r.leasedEvent(claim, now)
	if err != nil {
		return err
	}
	for _, effect := range copies {
		if effect.ID == "" || effect.EventID != claim.Event.ID || effect.CoordinationID != claim.Event.CoordinationID || effect.IdempotencyKey == "" {
			return errors.New("coordination: invalid committed effect identity")
		}
		if existingID, exists := r.keys[effect.IdempotencyKey]; exists && existingID != effect.ID {
			return fmt.Errorf("%w: effect idempotency key %q", ErrConflict, effect.IdempotencyKey)
		}
		if existing, exists := r.effects[effect.ID]; exists && !sameJSON(existing.effect, effect) {
			return fmt.Errorf("%w: effect ID %q", ErrConflict, effect.ID)
		}
	}
	for _, effect := range copies {
		if _, exists := r.effects[effect.ID]; !exists {
			r.effects[effect.ID] = memoryEffect{effect: effect, status: StatusPending}
			r.keys[effect.IdempotencyKey] = effect.ID
		}
	}
	record.status, record.leaseToken, record.leaseUntil, record.lastError = StatusCompleted, "", time.Time{}, ""
	r.events[claim.Event.ID] = record
	return nil
}

func (r *MemoryRepository) RetryEvent(_ context.Context, claim EventClaim, now, availableAt time.Time, cause error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, err := r.leasedEvent(claim, now)
	if err != nil {
		return err
	}
	record.status, record.availableAt = StatusPending, availableAt.UTC()
	record.leaseToken, record.leaseUntil = "", time.Time{}
	if cause != nil {
		record.lastError = cause.Error()
	}
	r.events[claim.Event.ID] = record
	return nil
}

func (r *MemoryRepository) FailEvent(_ context.Context, claim EventClaim, now time.Time, cause error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, err := r.leasedEvent(claim, now)
	if err != nil {
		return err
	}
	record.status, record.leaseToken, record.leaseUntil = StatusFailed, "", time.Time{}
	if cause != nil {
		record.lastError = cause.Error()
	}
	r.events[claim.Event.ID] = record
	return nil
}

func (r *MemoryRepository) ClaimEffect(_ context.Context, request ClaimRequest) (EffectClaim, bool, error) {
	if err := validateClaim(request); err != nil {
		return EffectClaim{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.effects))
	for id, record := range r.effects {
		if record.status == StatusPending && !record.effect.AvailableAt.After(request.Now) || record.status == StatusProcessing && !record.leaseUntil.After(request.Now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := r.effects[ids[i]], r.effects[ids[j]]
		if !a.effect.AvailableAt.Equal(b.effect.AvailableAt) {
			return a.effect.AvailableAt.Before(b.effect.AvailableAt)
		}
		return ids[i] < ids[j]
	})
	if len(ids) == 0 {
		return EffectClaim{}, false, nil
	}
	id := ids[0]
	record := r.effects[id]
	r.sequence++
	record.status, record.attempts = StatusProcessing, record.attempts+1
	record.leaseToken = fmt.Sprintf("effect-lease-%d", r.sequence)
	record.leaseUntil = request.Now.UTC().Add(request.LeaseDuration)
	r.effects[id] = record
	effect, err := durableCopy(record.effect)
	return EffectClaim{Effect: effect, Token: record.leaseToken, Attempt: record.attempts}, true, err
}

func (r *MemoryRepository) CompleteEffect(_ context.Context, claim EffectClaim, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, err := r.leasedEffect(claim, now)
	if err != nil {
		return err
	}
	record.status, record.leaseToken, record.leaseUntil, record.lastError = StatusCompleted, "", time.Time{}, ""
	r.effects[claim.Effect.ID] = record
	return nil
}

func (r *MemoryRepository) RetryEffect(_ context.Context, claim EffectClaim, now, availableAt time.Time, cause error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, err := r.leasedEffect(claim, now)
	if err != nil {
		return err
	}
	record.status, record.effect.AvailableAt = StatusPending, availableAt.UTC()
	record.leaseToken, record.leaseUntil = "", time.Time{}
	if cause != nil {
		record.lastError = cause.Error()
	}
	r.effects[claim.Effect.ID] = record
	return nil
}

func (r *MemoryRepository) FailEffect(_ context.Context, claim EffectClaim, now time.Time, cause error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, err := r.leasedEffect(claim, now)
	if err != nil {
		return err
	}
	record.status, record.leaseToken, record.leaseUntil = StatusFailed, "", time.Time{}
	if cause != nil {
		record.lastError = cause.Error()
	}
	r.effects[claim.Effect.ID] = record
	return nil
}

func (r *MemoryRepository) leasedEvent(claim EventClaim, now time.Time) (memoryEvent, error) {
	record, ok := r.events[claim.Event.ID]
	if !ok {
		return memoryEvent{}, ErrNotFound
	}
	if record.status != StatusProcessing || claim.Token == "" || record.leaseToken != claim.Token || !now.Before(record.leaseUntil) {
		return memoryEvent{}, ErrLeaseLost
	}
	return record, nil
}

func (r *MemoryRepository) leasedEffect(claim EffectClaim, now time.Time) (memoryEffect, error) {
	record, ok := r.effects[claim.Effect.ID]
	if !ok {
		return memoryEffect{}, ErrNotFound
	}
	if record.status != StatusProcessing || claim.Token == "" || record.leaseToken != claim.Token || !now.Before(record.leaseUntil) {
		return memoryEffect{}, ErrLeaseLost
	}
	return record, nil
}

func validateClaim(request ClaimRequest) error {
	if strings.TrimSpace(request.WorkerID) == "" || request.LeaseDuration <= 0 {
		return errors.New("coordination: invalid claim request")
	}
	return nil
}

func durableCopy[T any](value T) (T, error) {
	var result T
	encoded, err := json.Marshal(value)
	if err != nil {
		return result, fmt.Errorf("coordination: value is not durable: %w", err)
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return result, fmt.Errorf("coordination: copy durable value: %w", err)
	}
	return result, nil
}

func sameJSON(left, right any) bool {
	a, errA := json.Marshal(left)
	b, errB := json.Marshal(right)
	return errA == nil && errB == nil && string(a) == string(b)
}
