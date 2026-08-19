package dataspace

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Registry contains DataSpace declarations only. Opening physical stores and
// applying migrations are adapter responsibilities and must happen after all
// application registrations succeed.
type Registry struct {
	mu     sync.RWMutex
	spaces map[string]Descriptor
}

func NewRegistry() *Registry { return &Registry{spaces: map[string]Descriptor{}} }

func (r *Registry) Register(descriptor Descriptor) error {
	if r == nil {
		return errors.New("dataspace: nil registry")
	}
	if err := descriptor.Validate(); err != nil {
		return err
	}
	key := Key(descriptor.AppID, descriptor.Name)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.spaces[key]; exists {
		return fmt.Errorf("dataspace: %s/%s is already registered", descriptor.AppID, descriptor.Name)
	}
	descriptor.Kinds = append([]Kind(nil), descriptor.Kinds...)
	r.spaces[key] = descriptor
	return nil
}

func (r *Registry) Resolve(appID, name string) (Descriptor, bool) {
	if r == nil {
		return Descriptor{}, false
	}
	r.mu.RLock()
	descriptor, ok := r.spaces[Key(appID, name)]
	r.mu.RUnlock()
	descriptor.Kinds = append([]Kind(nil), descriptor.Kinds...)
	return descriptor, ok
}

func (r *Registry) Descriptors() []Descriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	result := make([]Descriptor, 0, len(r.spaces))
	for _, descriptor := range r.spaces {
		descriptor.Kinds = append([]Kind(nil), descriptor.Kinds...)
		result = append(result, descriptor)
	}
	r.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		if result[i].AppID != result[j].AppID {
			return result[i].AppID < result[j].AppID
		}
		return result[i].Name < result[j].Name
	})
	return result
}
