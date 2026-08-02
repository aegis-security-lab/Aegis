package coordination

import (
	"testing"

	"aegis/capability"
)

type capabilityCatalog map[string]bool

func (c capabilityCatalog) Has(ref capability.Ref) bool { return c[string(ref.Kind)+"/"+ref.Name] }

func TestCapabilityPlannerMergesFiltersAndRequires(t *testing.T) {
	planner := CapabilityPlanner{Catalog: capabilityCatalog{"skill/go": true, "web/default": true, "phone/default": true}}
	decision, err := planner.Plan(CapabilityPlanRequest{
		Defaults:       []capability.Ref{{Kind: capability.KindSkill, Name: "go"}, {Kind: capability.KindWeb, Name: "default"}},
		Requested:      []capability.Ref{{Kind: capability.KindPhone, Name: "default", Config: map[string]any{"installedApps": []any{"aegis.board"}}}},
		SystemRequired: []capability.Ref{{Kind: capability.KindPhone, Name: "default"}},
		Policy:         CapabilityPolicy{Allowed: []CapabilityPattern{{Kind: "*", Name: "*"}}, Denied: []CapabilityPattern{{Kind: capability.KindWeb, Name: "default"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Capabilities) != 2 || decision.Capabilities[0].Name != "go" || decision.Capabilities[1].Name != "default" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestCapabilityPlannerRejectsUnregisteredAndRawSecrets(t *testing.T) {
	planner := CapabilityPlanner{Catalog: capabilityCatalog{}}
	if _, err := planner.Plan(CapabilityPlanRequest{Requested: []capability.Ref{{Kind: capability.KindMCP, Name: "missing"}}, Selection: CapabilityReplace, Policy: CapabilityPolicy{Allowed: []CapabilityPattern{{Kind: capability.KindMCP, Name: "missing"}}}}); err == nil {
		t.Fatal("unregistered capability should fail")
	}
	if _, err := (CapabilityPlanner{}).Plan(CapabilityPlanRequest{Requested: []capability.Ref{{Kind: capability.KindMCP, Name: "db", Config: map[string]any{"apiKey": "secret"}}}, Selection: CapabilityReplace}); err == nil {
		t.Fatal("raw secret should fail")
	}
}

func TestCapabilityPlannerRequiresExplicitPolicyForAdditionalPlugin(t *testing.T) {
	planner := CapabilityPlanner{Catalog: capabilityCatalog{"skill/go": true, "mcp/github": true}}
	defaults := []capability.Ref{{Kind: capability.KindSkill, Name: "go"}}
	requested := []capability.Ref{{Kind: capability.KindMCP, Name: "github"}}
	if _, err := planner.Plan(CapabilityPlanRequest{Defaults: defaults, Requested: requested}); err == nil {
		t.Fatal("an ad-hoc plugin must not be enabled without an explicit allow policy")
	}
	decision, err := planner.Plan(CapabilityPlanRequest{
		Defaults: defaults, Requested: requested,
		Policy: CapabilityPolicy{Allowed: []CapabilityPattern{{Kind: capability.KindSkill, Name: "*"}, {Kind: capability.KindMCP, Name: "github"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Capabilities) != 2 || decision.Capabilities[1].Name != "github" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestCapabilityPlannerDoesNotSilentlyDropRequestedCapability(t *testing.T) {
	planner := CapabilityPlanner{Catalog: capabilityCatalog{"mcp/github": true}}
	_, err := planner.Plan(CapabilityPlanRequest{
		Requested: []capability.Ref{{Kind: capability.KindMCP, Name: "github"}}, Selection: CapabilityReplace,
		Policy: CapabilityPolicy{Allowed: []CapabilityPattern{{Kind: capability.KindMCP, Name: "gitlab"}}},
	})
	if err == nil {
		t.Fatal("explicitly requested but disallowed capability should fail")
	}
}
