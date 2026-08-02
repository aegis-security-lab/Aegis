package agentapp

import (
	"context"
	"sync"
)

const defaultAuditCapacity = 2000

// MemoryAuditor is the Phone module's built-in system log. It keeps a bounded
// operation history and can be replaced by a persistent Auditor in the host.
type MemoryAuditor struct {
	mu       sync.RWMutex
	events   []AuditEvent
	capacity int
}

type AuditReader interface {
	Events(context.Context, string, int) ([]AuditEvent, error)
}

type SessionPersistence interface {
	Auditor
	AuditReader
	SaveSession(context.Context, *Session) error
	LoadSession(context.Context, string) (*Session, error)
	FindTaskSession(context.Context, string, string) (*Session, error)
	ListSessions(context.Context, string, int) ([]SessionState, error)
}

func NewMemoryAuditor(capacity int) *MemoryAuditor {
	if capacity <= 0 {
		capacity = defaultAuditCapacity
	}
	return &MemoryAuditor{capacity: capacity}
}

func (a *MemoryAuditor) Record(_ context.Context, event AuditEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, event)
	if overflow := len(a.events) - a.capacity; overflow > 0 {
		copy(a.events, a.events[overflow:])
		a.events = a.events[:a.capacity]
	}
}

// Events returns newest entries first, scoped to one Phone Session.
func (a *MemoryAuditor) Events(_ context.Context, phoneSessionID string, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]AuditEvent, 0, limit)
	for index := len(a.events) - 1; index >= 0 && len(result) < limit; index-- {
		if a.events[index].PhoneSessionID == phoneSessionID {
			result = append(result, a.events[index])
		}
	}
	return result, nil
}

type multiAuditor []Auditor

func (auditors multiAuditor) Record(ctx context.Context, event AuditEvent) {
	for _, auditor := range auditors {
		auditor.Record(ctx, event)
	}
}
