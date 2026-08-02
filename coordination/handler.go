package coordination

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type EffectRouter struct {
	mu       sync.RWMutex
	handlers map[EffectType]EffectHandler
}

func NewEffectRouter() *EffectRouter {
	return &EffectRouter{handlers: make(map[EffectType]EffectHandler)}
}

func (r *EffectRouter) Register(kind EffectType, handler EffectHandler) error {
	if r == nil || kind == "" || handler == nil {
		return errors.New("coordination: effect type and handler are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[kind]; exists {
		return fmt.Errorf("coordination: effect handler %q is already registered", kind)
	}
	r.handlers[kind] = handler
	return nil
}

func (r *EffectRouter) HandleEffect(ctx context.Context, effect Effect) error {
	if r == nil {
		return errors.New("coordination: nil effect router")
	}
	r.mu.RLock()
	handler := r.handlers[effect.Type]
	r.mu.RUnlock()
	if handler == nil {
		return fmt.Errorf("coordination: no handler for effect %q", effect.Type)
	}
	return handler.HandleEffect(ctx, effect)
}

// WakeupHandler converts a due ScheduleWakeup effect back into a durable
// TimerFired Event. Effect ID becomes the stable event ID across retries.
type WakeupHandler struct {
	Submit func(context.Context, Event) (bool, error)
	Now    func() time.Time
}

func (h WakeupHandler) HandleEffect(ctx context.Context, effect Effect) error {
	if h.Submit == nil {
		return errors.New("coordination: wakeup submitter is required")
	}
	var command ScheduleWakeupCommand
	if err := json.Unmarshal(effect.Payload, &command); err != nil {
		return fmt.Errorf("coordination: decode wakeup effect: %w", err)
	}
	event := command.Event
	if event.ID == "" {
		event.ID = effect.ID + ":fired"
	}
	if event.CoordinationID == "" {
		event.CoordinationID = effect.CoordinationID
	}
	if event.Type == "" {
		event.Type = EventTimerFired
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
		if h.Now != nil {
			event.OccurredAt = h.Now().UTC()
		}
	}
	_, err := h.Submit(ctx, event)
	return err
}

var _ EffectHandler = (*EffectRouter)(nil)
var _ EffectHandler = WakeupHandler{}
