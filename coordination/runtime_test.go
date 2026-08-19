package coordination

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestRuntimeProcessesDelayedWakeupBackIntoMode(t *testing.T) {
	repository := NewMemoryRepository()
	registry := NewRegistry()
	if err := registry.Register(ModeFunc{ModeName: "wake", ModeVersion: "1", DecideFunc: func(_ context.Context, event Event, _ Binding, _ Snapshot) ([]PlannedEffect, error) {
		if event.Type == testEventWaitRequested {
			payload, _ := json.Marshal(ScheduleWakeupCommand{Event: Event{Type: EventTimerFired, CoordinationID: event.CoordinationID, ActorID: event.ActorID}})
			return []PlannedEffect{{Type: EffectScheduleWakeup, AvailableAt: time.Now().UTC(), Payload: payload}}, nil
		}
		if event.Type == EventTimerFired {
			payload, _ := json.Marshal(testDeliveryCommand{ActorID: event.ActorID, Message: "wake"})
			return []PlannedEffect{{Type: testEffectResumeActor, Payload: payload}}, nil
		}
		return nil, nil
	}}); err != nil {
		t.Fatal(err)
	}
	_ = repository.SaveBinding(context.Background(), Binding{CoordinationID: "task", Mode: "wake", Version: "1", UpdatedAt: time.Now()})
	router := NewEffectRouter()
	engine := &Engine{Repository: repository, Modes: registry, WorkerID: "decision"}
	runtime, _ := NewRuntime(RuntimeConfig{Engine: engine, Effects: &EffectWorker{Repository: repository, Handler: router, WorkerID: "effects"}})
	_ = router.Register(EffectScheduleWakeup, WakeupHandler{Submit: runtime.Submit})
	resumed := make(chan struct{}, 1)
	_ = router.Register(testEffectResumeActor, EffectHandlerFunc(func(context.Context, Effect) error { resumed <- struct{}{}; return nil }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime.Poll = 10 * time.Millisecond
	runtime.Start(ctx)
	defer runtime.Close()
	_, err := runtime.Submit(context.Background(), Event{ID: "wait", Type: testEventWaitRequested, CoordinationID: "task", ActorID: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-resumed:
	case <-time.After(time.Second):
		t.Fatal("wakeup was not routed back through the mode")
	}
}
