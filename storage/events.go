package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/z3r2ne/agentcore"
)

type NewExecutionEvent struct {
	ExecutionID string
	Attempt     int
	CreatedAt   time.Time
	Event       agentcore.Event
}

type ExecutionEvent struct {
	ExecutionID string          `json:"executionId"`
	Attempt     int             `json:"attempt"`
	Sequence    uint64          `json:"sequence"`
	CreatedAt   time.Time       `json:"createdAt"`
	Event       agentcore.Event `json:"event"`
}

type EventStore interface {
	AppendExecutionEvent(context.Context, NewExecutionEvent) (ExecutionEvent, error)
	ExecutionEvents(context.Context, string, uint64, int) ([]ExecutionEvent, error)
}

// EventSink adapts an EventStore to agentcore. Sequence assignment belongs to
// the Store so it remains atomic across workers and process restarts.
func EventSink(store EventStore, executionID string, attempt int, now func() time.Time) agentcore.EventSink {
	return func(ctx context.Context, event agentcore.Event) error {
		if store == nil {
			return errors.New("storage: nil event store")
		}
		// message_update contains the full assistant message accumulated so far
		// for every streamed delta. It is useful live but grows roughly
		// quadratically when persisted. message_end retains the completed message.
		if event.Type == agentcore.EventMessageUpdate {
			return nil
		}
		createdAt := time.Now().UTC()
		if now != nil {
			createdAt = now().UTC()
		}
		_, err := store.AppendExecutionEvent(ctx, NewExecutionEvent{
			ExecutionID: executionID, Attempt: attempt, CreatedAt: createdAt, Event: event,
		})
		return err
	}
}

// MemoryEventStore is a concurrency-safe reference implementation.
type MemoryEventStore struct {
	mu     sync.Mutex
	events map[string][]ExecutionEvent
}

func NewMemoryEventStore() *MemoryEventStore {
	return &MemoryEventStore{events: make(map[string][]ExecutionEvent)}
}

func (s *MemoryEventStore) AppendExecutionEvent(_ context.Context, input NewExecutionEvent) (ExecutionEvent, error) {
	if s == nil {
		return ExecutionEvent{}, errors.New("storage: nil memory event store")
	}
	if input.ExecutionID == "" {
		return ExecutionEvent{}, errors.New("storage: execution ID is required")
	}
	event, err := durableEvent(input.Event)
	if err != nil {
		return ExecutionEvent{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sequence := uint64(len(s.events[input.ExecutionID]) + 1)
	record := ExecutionEvent{ExecutionID: input.ExecutionID, Attempt: input.Attempt, Sequence: sequence, CreatedAt: input.CreatedAt.UTC(), Event: event}
	s.events[input.ExecutionID] = append(s.events[input.ExecutionID], record)
	return record, nil
}

func (s *MemoryEventStore) ExecutionEvents(_ context.Context, executionID string, after uint64, limit int) ([]ExecutionEvent, error) {
	if s == nil {
		return nil, errors.New("storage: nil memory event store")
	}
	if limit <= 0 {
		limit = 100
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]ExecutionEvent, 0, min(limit, len(s.events[executionID])))
	for _, record := range s.events[executionID] {
		if record.Sequence <= after {
			continue
		}
		copy, err := durableExecutionEvent(record)
		if err != nil {
			return nil, err
		}
		result = append(result, copy)
		if len(result) == limit {
			break
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Sequence < result[j].Sequence })
	return result, nil
}

func durableEvent(event agentcore.Event) (agentcore.Event, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return agentcore.Event{}, fmt.Errorf("storage: event is not durable: %w", err)
	}
	var result agentcore.Event
	if err := json.Unmarshal(data, &result); err != nil {
		return agentcore.Event{}, fmt.Errorf("storage: copy event: %w", err)
	}
	return result, nil
}

func durableExecutionEvent(event ExecutionEvent) (ExecutionEvent, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return ExecutionEvent{}, err
	}
	var result ExecutionEvent
	return result, json.Unmarshal(data, &result)
}
