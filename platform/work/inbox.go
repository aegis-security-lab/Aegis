package work

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Inbox gives an application at-least-once pull delivery. Pull never advances
// durable state; the application acknowledges Next only after its domain
// transaction succeeds. Recreating Inbox after a restart resumes from the
// persisted cursor.
type Inbox struct {
	Events        EventStore
	Subscriptions SubscriptionRepository
	Now           func() time.Time
}

func (i *Inbox) Register(ctx context.Context, subscription Subscription) error {
	if i == nil || i.Subscriptions == nil {
		return errors.New("application inbox: subscription repository is required")
	}
	now := i.now()
	if subscription.CreatedAt.IsZero() {
		subscription.CreatedAt = now
	}
	if subscription.UpdatedAt.IsZero() {
		subscription.UpdatedAt = subscription.CreatedAt
	}
	if err := subscription.Validate(); err != nil {
		return err
	}
	return i.Subscriptions.RegisterSubscription(ctx, subscription)
}

func (i *Inbox) Pull(ctx context.Context, subscriptionID string, limit int) (Delivery, error) {
	if i == nil || i.Events == nil || i.Subscriptions == nil {
		return Delivery{}, errors.New("application inbox: event and subscription stores are required")
	}
	subscription, err := i.Subscriptions.Subscription(ctx, strings.TrimSpace(subscriptionID))
	if err != nil {
		return Delivery{}, err
	}
	events, next, err := i.Events.Replay(ctx, subscription.Filter, subscription.Cursor, limit)
	if err != nil {
		return Delivery{}, err
	}
	return Delivery{SubscriptionID: subscription.ID, Events: events, Next: next}, nil
}

func (i *Inbox) Ack(ctx context.Context, subscriptionID string, cursor Cursor) error {
	if i == nil || i.Subscriptions == nil {
		return errors.New("application inbox: subscription repository is required")
	}
	if cursor.Sequence < 0 {
		return errors.New("application inbox: cursor cannot be negative")
	}
	return i.Subscriptions.AckSubscription(ctx, strings.TrimSpace(subscriptionID), cursor, i.now())
}

func (i *Inbox) now() time.Time {
	if i.Now != nil {
		return i.Now().UTC()
	}
	return time.Now().UTC()
}
