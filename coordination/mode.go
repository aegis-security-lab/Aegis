package coordination

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type ModeDescriptor struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Mode contains coordination policy only. Implementations must not perform
// external side effects; return PlannedEffects for the outbox instead.
type Mode interface {
	Name() string
	Version() string
	Decide(context.Context, Event, Binding, Snapshot) ([]PlannedEffect, error)
}

type ModeFunc struct {
	ModeName    string
	ModeVersion string
	DecideFunc  func(context.Context, Event, Binding, Snapshot) ([]PlannedEffect, error)
}

func (m ModeFunc) Name() string    { return m.ModeName }
func (m ModeFunc) Version() string { return m.ModeVersion }
func (m ModeFunc) Decide(ctx context.Context, event Event, binding Binding, snapshot Snapshot) ([]PlannedEffect, error) {
	if m.DecideFunc == nil {
		return nil, errors.New("coordination: nil mode decision function")
	}
	return m.DecideFunc(ctx, event, binding, snapshot)
}

type Registry struct {
	mu    sync.RWMutex
	modes map[string]Mode
}

func NewRegistry() *Registry { return &Registry{modes: make(map[string]Mode)} }

func (r *Registry) Register(mode Mode) error {
	if r == nil || mode == nil {
		return errors.New("coordination: mode registry and mode are required")
	}
	name, version := strings.TrimSpace(mode.Name()), strings.TrimSpace(mode.Version())
	if name == "" || version == "" {
		return errors.New("coordination: mode name and version are required")
	}
	key := modeKey(name, version)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.modes[key]; exists {
		return fmt.Errorf("coordination: mode %s@%s is already registered", name, version)
	}
	r.modes[key] = mode
	return nil
}

func (r *Registry) Resolve(name, version string) (Mode, error) {
	if r == nil {
		return nil, errors.New("coordination: nil mode registry")
	}
	r.mu.RLock()
	mode, ok := r.modes[modeKey(name, version)]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("coordination: mode %s@%s is not registered", name, version)
	}
	return mode, nil
}

func (r *Registry) Modes() []ModeDescriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	result := make([]ModeDescriptor, 0, len(r.modes))
	for _, mode := range r.modes {
		result = append(result, ModeDescriptor{Name: mode.Name(), Version: mode.Version()})
	}
	r.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Version < result[j].Version
	})
	return result
}

func modeKey(name, version string) string {
	return strings.TrimSpace(name) + "@" + strings.TrimSpace(version)
}

// Composite combines small orthogonal policies into one selectable mode.
// Effects retain child order, making generated IDs deterministic.
type Composite struct {
	ModeName    string
	ModeVersion string
	Modes       []Mode
}

func (m Composite) Name() string    { return m.ModeName }
func (m Composite) Version() string { return m.ModeVersion }
func (m Composite) Decide(ctx context.Context, event Event, binding Binding, snapshot Snapshot) ([]PlannedEffect, error) {
	var result []PlannedEffect
	keys := make(map[string]bool)
	for _, child := range m.Modes {
		if child == nil {
			continue
		}
		effects, err := child.Decide(ctx, event, binding, snapshot)
		if err != nil {
			return nil, fmt.Errorf("composed mode %s@%s: %w", child.Name(), child.Version(), err)
		}
		for _, effect := range effects {
			if effect.IdempotencyKey != "" {
				if keys[effect.IdempotencyKey] {
					return nil, fmt.Errorf("coordination: composed modes produced duplicate idempotency key %q", effect.IdempotencyKey)
				}
				keys[effect.IdempotencyKey] = true
			}
			result = append(result, effect)
		}
	}
	return result, nil
}
