package work

import (
	"context"
	"testing"
	"time"
)

type memorySubscriptions struct{ subscriptions map[string]Subscription }

func (m *memorySubscriptions) RegisterSubscription(_ context.Context, subscription Subscription) error {
	if m.subscriptions == nil {
		m.subscriptions = map[string]Subscription{}
	}
	m.subscriptions[subscription.ID] = subscription
	return nil
}

func (m *memorySubscriptions) Subscription(_ context.Context, id string) (Subscription, error) {
	subscription, ok := m.subscriptions[id]
	if !ok {
		return Subscription{}, ErrNotFound
	}
	return subscription, nil
}

func (m *memorySubscriptions) AckSubscription(_ context.Context, id string, cursor Cursor, at time.Time) error {
	subscription, ok := m.subscriptions[id]
	if !ok {
		return ErrNotFound
	}
	if cursor.Sequence > subscription.Cursor.Sequence {
		subscription.Cursor, subscription.UpdatedAt = cursor, at
		m.subscriptions[id] = subscription
	}
	return nil
}

func TestInboxPullDoesNotAdvanceUntilAck(t *testing.T) {
	events := &memoryEvents{events: []Event{{Sequence: 1, Type: EventScheduled}, {Sequence: 2, Type: EventStarted}}}
	subscriptions := &memorySubscriptions{}
	inbox := &Inbox{Events: events, Subscriptions: subscriptions}
	subscription := Subscription{ID: "board-controller", Filter: EventFilter{AppID: "aegis.board", ScopeID: "task-1"}}
	if err := inbox.Register(context.Background(), subscription); err != nil {
		t.Fatal(err)
	}
	first, err := inbox.Pull(context.Background(), subscription.ID, 10)
	if err != nil || len(first.Events) != 2 || first.Next.Sequence != 2 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	again, err := inbox.Pull(context.Background(), subscription.ID, 10)
	if err != nil || len(again.Events) != 2 {
		t.Fatalf("unacked delivery=%+v err=%v", again, err)
	}
	if err := inbox.Ack(context.Background(), subscription.ID, first.Next); err != nil {
		t.Fatal(err)
	}
	// The in-memory EventStore test double ignores its cursor, so inspect the
	// durable subscription directly for this contract test.
	stored, err := subscriptions.Subscription(context.Background(), subscription.ID)
	if err != nil || stored.Cursor.Sequence != 2 {
		t.Fatalf("subscription=%+v err=%v", stored, err)
	}
}
