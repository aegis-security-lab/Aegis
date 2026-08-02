package capability

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry maps durable refs to runtime sources. It is safe for concurrent
// resolution and registration.
type Registry struct {
	mu      sync.RWMutex
	sources map[string]Source
}

func (r *Registry) Descriptors() []Descriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	result := make([]Descriptor, 0, len(r.sources))
	for key := range r.sources {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) == 2 {
			result = append(result, Descriptor{Kind: Kind(parts[0]), Name: parts[1], Dynamic: parts[1] == "*"})
		}
	}
	r.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func NewRegistry() *Registry {
	return &Registry{sources: make(map[string]Source)}
}

// Has reports whether an exact capability source is available. Version and
// per-execution config are resolved by the source and do not affect lookup.
func (r *Registry) Has(ref Ref) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	_, ok := r.sources[sourceKey(ref.Kind, ref.Name)]
	if !ok {
		_, ok = r.sources[sourceKey(ref.Kind, "*")]
	}
	r.mu.RUnlock()
	return ok
}

// RegisterKind installs a dynamic source for names that do not have an exact
// registration. The source must validate each requested name itself.
func (r *Registry) RegisterKind(kind Kind, source Source) error {
	if r == nil || strings.TrimSpace(string(kind)) == "" || source == nil {
		return fmt.Errorf("capability: kind and dynamic source are required")
	}
	key := sourceKey(kind, "*")
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sources[key]; exists {
		return fmt.Errorf("capability: dynamic source %s/* is already registered", kind)
	}
	r.sources[key] = source
	return nil
}

// Register adds one exact kind/name source.
func (r *Registry) Register(kind Kind, name string, source Source) error {
	ref := Ref{Kind: kind, Name: name}
	if err := validateRef(ref); err != nil {
		return err
	}
	if source == nil {
		return fmt.Errorf("capability: nil source for %s/%s", kind, name)
	}
	key := sourceKey(kind, name)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sources[key]; exists {
		return fmt.Errorf("capability: source %s/%s is already registered", kind, name)
	}
	r.sources[key] = source
	return nil
}

// Resolve materializes refs in declaration order. If any source fails, already
// opened resources are closed before the error is returned.
func (r *Registry) Resolve(ctx context.Context, execution ResolveContext, refs []Ref) (Bundle, error) {
	if r == nil {
		return Bundle{}, fmt.Errorf("capability: nil registry")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	bundle := Bundle{}
	toolNames := make(map[string]Ref)
	for _, ref := range refs {
		if err := validateRef(ref); err != nil {
			_ = bundle.Close()
			return Bundle{}, err
		}
		r.mu.RLock()
		source := r.sources[sourceKey(ref.Kind, ref.Name)]
		if source == nil {
			source = r.sources[sourceKey(ref.Kind, "*")]
		}
		r.mu.RUnlock()
		if source == nil {
			if ref.Optional {
				continue
			}
			_ = bundle.Close()
			return Bundle{}, fmt.Errorf("capability: source %s/%s is not registered", ref.Kind, ref.Name)
		}
		resolved, err := source.Resolve(ctx, execution, ref)
		if err != nil {
			for _, closer := range resolved.Closers {
				if closer != nil {
					_ = closer.Close()
				}
			}
			_ = bundle.Close()
			return Bundle{}, fmt.Errorf("capability: resolve %s/%s: %w", ref.Kind, ref.Name, err)
		}
		if err := validateResolved(ref, resolved, toolNames); err != nil {
			for _, closer := range resolved.Closers {
				if closer != nil {
					_ = closer.Close()
				}
			}
			_ = bundle.Close()
			return Bundle{}, err
		}
		snapshot := resolved.Snapshot
		if snapshot.Kind == "" {
			snapshot.Kind = ref.Kind
		}
		if snapshot.Name == "" {
			snapshot.Name = ref.Name
		}
		if snapshot.Version == "" {
			snapshot.Version = ref.Version
		}
		bundle.Tools = append(bundle.Tools, resolved.Tools...)
		bundle.Instructions = append(bundle.Instructions, resolved.Instructions...)
		bundle.Snapshots = append(bundle.Snapshots, snapshot)
		bundle.closers = append(bundle.closers, resolved.Closers...)
	}
	return bundle, nil
}
