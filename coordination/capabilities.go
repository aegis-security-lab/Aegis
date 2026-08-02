package coordination

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"aegis/capability"
)

const DefaultMaxCapabilities = 64

type CapabilitySelection string

const (
	CapabilityInherit CapabilitySelection = "inherit"
	CapabilityMerge   CapabilitySelection = "merge"
	CapabilityReplace CapabilitySelection = "replace"
)

// CapabilityPattern is an allow/deny selector. Empty or "*" fields match all.
type CapabilityPattern struct {
	Kind capability.Kind `json:"kind,omitempty"`
	Name string          `json:"name,omitempty"`
}

// CapabilityPolicy belongs to a Coordination binding. It controls what a
// delegation may materialize; raw credentials are never valid capability config.
type CapabilityPolicy struct {
	DefaultSelection CapabilitySelection `json:"defaultSelection,omitempty"`
	Allowed          []CapabilityPattern `json:"allowed,omitempty"`
	Denied           []CapabilityPattern `json:"denied,omitempty"`
	Required         []capability.Ref    `json:"required,omitempty"`
	MaxCapabilities  int                 `json:"maxCapabilities,omitempty"`
}

type CapabilityCatalog interface {
	Has(capability.Ref) bool
}

type CapabilityPlanRequest struct {
	Defaults       []capability.Ref
	Requested      []capability.Ref
	SystemRequired []capability.Ref
	Selection      CapabilitySelection
	Policy         CapabilityPolicy
}

type CapabilityDecision struct {
	Capabilities   []capability.Ref    `json:"capabilities"`
	Selection      CapabilitySelection `json:"selection"`
	DefaultCount   int                 `json:"defaultCount"`
	RequestedCount int                 `json:"requestedCount"`
	RequiredCount  int                 `json:"requiredCount"`
}

type CapabilityPlanner struct{ Catalog CapabilityCatalog }

func CapabilityPolicyFromBinding(binding Binding) (CapabilityPolicy, error) {
	if len(binding.Config) == 0 || string(binding.Config) == "null" {
		return CapabilityPolicy{}, nil
	}
	var envelope struct {
		CapabilityPolicy CapabilityPolicy `json:"capabilityPolicy"`
	}
	if err := json.Unmarshal(binding.Config, &envelope); err != nil {
		return CapabilityPolicy{}, fmt.Errorf("coordination: decode capability policy: %w", err)
	}
	if err := ValidateCapabilityPolicy(envelope.CapabilityPolicy); err != nil {
		return CapabilityPolicy{}, err
	}
	return envelope.CapabilityPolicy, nil
}

// ValidateCapabilityPolicy rejects malformed policy before it is persisted or
// used to authorize an execution. Zero values intentionally mean safe defaults.
func ValidateCapabilityPolicy(policy CapabilityPolicy) error {
	if policy.DefaultSelection != "" && policy.DefaultSelection != CapabilityInherit && policy.DefaultSelection != CapabilityMerge && policy.DefaultSelection != CapabilityReplace {
		return fmt.Errorf("coordination: invalid default capability selection %q", policy.DefaultSelection)
	}
	if policy.MaxCapabilities < 0 || policy.MaxCapabilities > DefaultMaxCapabilities {
		return fmt.Errorf("coordination: maxCapabilities must be between 0 and %d", DefaultMaxCapabilities)
	}
	for _, ref := range policy.Required {
		if err := validatePlannedCapability(ref); err != nil {
			return err
		}
	}
	return nil
}

func (p CapabilityPlanner) Plan(request CapabilityPlanRequest) (CapabilityDecision, error) {
	if err := ValidateCapabilityPolicy(request.Policy); err != nil {
		return CapabilityDecision{}, err
	}
	selection := request.Selection
	if selection == "" {
		selection = request.Policy.DefaultSelection
	}
	if selection == "" {
		selection = CapabilityMerge
	}
	if selection != CapabilityInherit && selection != CapabilityMerge && selection != CapabilityReplace {
		return CapabilityDecision{}, fmt.Errorf("coordination: invalid capability selection %q", selection)
	}
	maximum := request.Policy.MaxCapabilities
	if maximum <= 0 {
		maximum = DefaultMaxCapabilities
	}
	var candidates []capability.Ref
	switch selection {
	case CapabilityInherit:
		candidates = append(candidates, request.Defaults...)
	case CapabilityMerge:
		candidates = append(candidates, request.Defaults...)
		candidates = append(candidates, request.Requested...)
	case CapabilityReplace:
		candidates = append(candidates, request.Requested...)
	}
	required := append(append([]capability.Ref(nil), request.Policy.Required...), request.SystemRequired...)
	candidates = append(candidates, required...)

	byKey := map[string]capability.Ref{}
	order := make([]string, 0, len(candidates))
	for _, ref := range candidates {
		if err := validatePlannedCapability(ref); err != nil {
			return CapabilityDecision{}, err
		}
		key := capabilityKey(ref)
		if _, exists := byKey[key]; !exists {
			order = append(order, key)
		}
		byKey[key] = cloneCapabilityRef(ref)
	}
	requiredKeys := map[string]bool{}
	for _, ref := range required {
		requiredKeys[capabilityKey(ref)] = true
	}
	defaultKeys := map[string]bool{}
	for _, ref := range request.Defaults {
		defaultKeys[capabilityKey(ref)] = true
	}
	requestedKeys := map[string]bool{}
	if selection != CapabilityInherit {
		for _, ref := range request.Requested {
			requestedKeys[capabilityKey(ref)] = true
		}
	}
	result := make([]capability.Ref, 0, len(order))
	for _, key := range order {
		ref := byKey[key]
		if matchesAny(request.Policy.Denied, ref) {
			if requiredKeys[key] {
				return CapabilityDecision{}, fmt.Errorf("coordination: required capability %s/%s is denied by policy", ref.Kind, ref.Name)
			}
			if requestedKeys[key] {
				return CapabilityDecision{}, fmt.Errorf("coordination: requested capability %s/%s is denied by policy", ref.Kind, ref.Name)
			}
			continue
		}
		if len(request.Policy.Allowed) > 0 && !matchesAny(request.Policy.Allowed, ref) {
			if requiredKeys[key] {
				return CapabilityDecision{}, fmt.Errorf("coordination: required capability %s/%s is not allowed by policy", ref.Kind, ref.Name)
			}
			if requestedKeys[key] {
				return CapabilityDecision{}, fmt.Errorf("coordination: requested capability %s/%s is not allowed by policy", ref.Kind, ref.Name)
			}
			continue
		}
		// An empty allowlist is not an allow-all. It permits the Agent's
		// declared defaults and system-required control tools only. Bindings
		// must explicitly allow any additional plugin requested at runtime.
		if len(request.Policy.Allowed) == 0 && requestedKeys[key] && !defaultKeys[key] && !requiredKeys[key] {
			return CapabilityDecision{}, fmt.Errorf("coordination: requested capability %s/%s requires an explicit allowed policy", ref.Kind, ref.Name)
		}
		if p.Catalog != nil && !p.Catalog.Has(ref) {
			if ref.Optional && !requiredKeys[key] {
				continue
			}
			return CapabilityDecision{}, fmt.Errorf("coordination: capability %s/%s is not registered", ref.Kind, ref.Name)
		}
		result = append(result, ref)
	}
	if len(result) > maximum {
		return CapabilityDecision{}, fmt.Errorf("coordination: capability plan contains %d entries, maximum is %d", len(result), maximum)
	}
	return CapabilityDecision{Capabilities: result, Selection: selection, DefaultCount: len(request.Defaults), RequestedCount: len(request.Requested), RequiredCount: len(required)}, nil
}

func validatePlannedCapability(ref capability.Ref) error {
	if strings.TrimSpace(string(ref.Kind)) == "" || strings.TrimSpace(ref.Name) == "" {
		return errors.New("coordination: capability kind and name are required")
	}
	for key, value := range ref.Config {
		normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
		if strings.HasSuffix(normalized, "ref") {
			continue
		}
		if strings.Contains(normalized, "apikey") || strings.Contains(normalized, "password") || strings.Contains(normalized, "authorization") || normalized == "token" || normalized == "secret" {
			if value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
				return fmt.Errorf("coordination: capability %s/%s config contains raw secret field %q; use a secret reference", ref.Kind, ref.Name, key)
			}
		}
	}
	return nil
}

func matchesAny(patterns []CapabilityPattern, ref capability.Ref) bool {
	for _, pattern := range patterns {
		kind := strings.TrimSpace(string(pattern.Kind))
		name := strings.TrimSpace(pattern.Name)
		if (kind == "" || kind == "*" || kind == string(ref.Kind)) && (name == "" || name == "*" || name == ref.Name) {
			return true
		}
	}
	return false
}

func capabilityKey(ref capability.Ref) string {
	return string(ref.Kind) + "\x00" + strings.TrimSpace(ref.Name)
}

func cloneCapabilityRef(ref capability.Ref) capability.Ref {
	result := ref
	if ref.Config != nil {
		result.Config = make(map[string]any, len(ref.Config))
		keys := make([]string, 0, len(ref.Config))
		for key := range ref.Config {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result.Config[key] = ref.Config[key]
		}
	}
	return result
}
