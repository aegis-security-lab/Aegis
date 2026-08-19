// Package application defines the installation boundary between the Agent
// platform and independently owned business applications.
package application

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"aegis/platform/dataspace"
)

var applicationIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)+$`)

type Manifest struct {
	ID            string   `json:"id"`
	Version       string   `json:"version"`
	DisplayName   string   `json:"displayName"`
	SDKVersion    string   `json:"sdkVersion"`
	DataSpaces    []string `json:"dataSpaces,omitempty"`
	PhoneModules  []string `json:"phoneModules,omitempty"`
	AgentProfiles []string `json:"agentProfiles,omitempty"`
	Prompts       []string `json:"prompts,omitempty"`
	Controllers   []string `json:"controllers,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
}

func (m Manifest) Validate() error {
	m.ID = strings.TrimSpace(m.ID)
	if !applicationIDPattern.MatchString(m.ID) {
		return fmt.Errorf("application: invalid application ID %q", m.ID)
	}
	if strings.TrimSpace(m.Version) == "" || strings.TrimSpace(m.SDKVersion) == "" || strings.TrimSpace(m.DisplayName) == "" {
		return errors.New("application: version, SDK version and display name are required")
	}
	for kind, values := range map[string][]string{
		"DataSpace": m.DataSpaces, "Phone module": m.PhoneModules, "Agent profile": m.AgentProfiles,
		"Prompt": m.Prompts, "Controller": m.Controllers, "Capability": m.Capabilities,
	} {
		if err := uniqueNonEmpty(kind, values); err != nil {
			return err
		}
	}
	return nil
}

type Module interface {
	Manifest() Manifest
}

type DataSpaceProvider interface {
	DataSpaces() []dataspace.Descriptor
}

// Catalog is the platform-facing installation view. It keeps Application and
// DataSpace registration explicit so a bare platform contains no product app.
type Catalog struct {
	mu         sync.Mutex
	apps       *Registry
	dataspaces *dataspace.Registry
}

func NewCatalog() *Catalog {
	return &Catalog{apps: NewRegistry(), dataspaces: dataspace.NewRegistry()}
}

func (c *Catalog) Register(module Module) error {
	if c == nil || module == nil {
		return errors.New("application: catalog and module are required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	manifest := module.Manifest()
	if err := manifest.Validate(); err != nil {
		return err
	}
	if _, exists := c.apps.Resolve(manifest.ID); exists {
		return fmt.Errorf("application: %s is already registered", manifest.ID)
	}
	provider, _ := module.(DataSpaceProvider)
	var descriptors []dataspace.Descriptor
	if provider != nil {
		descriptors = provider.DataSpaces()
	}
	declared := map[string]bool{}
	for _, name := range manifest.DataSpaces {
		declared[strings.TrimSpace(name)] = true
	}
	for _, descriptor := range descriptors {
		if descriptor.AppID != manifest.ID {
			return fmt.Errorf("application: DataSpace %s belongs to %s, expected %s", descriptor.Name, descriptor.AppID, manifest.ID)
		}
		if !declared[descriptor.Name] {
			return fmt.Errorf("application: undeclared DataSpace %s/%s", descriptor.AppID, descriptor.Name)
		}
		if err := descriptor.Validate(); err != nil {
			return err
		}
		if _, exists := c.dataspaces.Resolve(descriptor.AppID, descriptor.Name); exists {
			return fmt.Errorf("application: DataSpace %s/%s is already registered", descriptor.AppID, descriptor.Name)
		}
		delete(declared, descriptor.Name)
	}
	if len(declared) > 0 {
		return errors.New("application: manifest declares a DataSpace without a descriptor")
	}
	// The catalog mutex serializes its writers and all inputs were preflighted,
	// so the following registrations cannot leave a partial install.
	if err := c.apps.Register(module); err != nil {
		return err
	}
	for _, descriptor := range descriptors {
		if err := c.dataspaces.Register(descriptor); err != nil {
			return err
		}
	}
	return nil
}

func (c *Catalog) Applications() []Manifest {
	if c == nil {
		return nil
	}
	return c.apps.Manifests()
}

func (c *Catalog) DataSpaces() []dataspace.Descriptor {
	if c == nil {
		return nil
	}
	return c.dataspaces.Descriptors()
}

type Registry struct {
	mu      sync.RWMutex
	modules map[string]Module
}

func NewRegistry() *Registry { return &Registry{modules: map[string]Module{}} }

func (r *Registry) Register(module Module) error {
	if r == nil || module == nil {
		return errors.New("application: registry and module are required")
	}
	manifest := module.Manifest()
	if err := manifest.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.modules[manifest.ID]; exists {
		return fmt.Errorf("application: %s is already registered", manifest.ID)
	}
	r.modules[manifest.ID] = module
	return nil
}

func (r *Registry) Resolve(id string) (Module, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	module, ok := r.modules[strings.TrimSpace(id)]
	r.mu.RUnlock()
	return module, ok
}

func (r *Registry) Manifests() []Manifest {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	result := make([]Manifest, 0, len(r.modules))
	for _, module := range r.modules {
		result = append(result, cloneManifest(module.Manifest()))
	}
	r.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func cloneManifest(manifest Manifest) Manifest {
	manifest.DataSpaces = append([]string(nil), manifest.DataSpaces...)
	manifest.PhoneModules = append([]string(nil), manifest.PhoneModules...)
	manifest.AgentProfiles = append([]string(nil), manifest.AgentProfiles...)
	manifest.Prompts = append([]string(nil), manifest.Prompts...)
	manifest.Controllers = append([]string(nil), manifest.Controllers...)
	manifest.Capabilities = append([]string(nil), manifest.Capabilities...)
	return manifest
}

func uniqueNonEmpty(kind string, values []string) error {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("application: %s name cannot be empty", kind)
		}
		if seen[value] {
			return fmt.Errorf("application: duplicate %s %q", kind, value)
		}
		seen[value] = true
	}
	return nil
}
