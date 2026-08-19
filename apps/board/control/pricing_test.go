package control

import (
	"math"
	"testing"
)

func TestExecutionPricingSnapshotAndConfiguredCost(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Costed work", Objective: "Complete and account for the work.", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if execution.Pricing.Input != 2 || execution.Pricing.Output != 10 || execution.Pricing.CacheRead != 0.2 || execution.Pricing.CacheWrite != 3 {
		t.Fatalf("unexpected execution pricing snapshot: %+v", execution.Pricing)
	}

	manager := &Manager{store: store}
	manager.updateStats(&PiSession{executionID: execution.ID}, map[string]any{
		"tokens": map[string]any{
			"input": float64(1_000_000), "output": float64(500_000),
			"cacheRead": float64(250_000), "cacheWrite": float64(100_000),
			"total": float64(1_850_000),
		},
		"cost": float64(999),
	})

	var saved Execution
	if err := store.db.First(&saved, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.InputTokens != 1_000_000 || saved.OutputTokens != 500_000 || saved.CacheReadTokens != 250_000 || saved.CacheWriteTokens != 100_000 || saved.Tokens != 1_850_000 {
		t.Fatalf("unexpected token breakdown: %+v", saved)
	}
	if math.Abs(saved.Cost-7.35) > 1e-12 {
		t.Fatalf("cost=%f, want 7.35", saved.Cost)
	}
}

func TestAgentPricingOverride(t *testing.T) {
	store := configuredStore(t)
	agent, err := store.GetAgent("backend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	override := ModelPricing{Input: 1, Output: 4, CacheRead: 0.1, CacheWrite: 1.2}
	agent.Model.Pricing = &override
	updated, err := store.UpdateAgent(agent.ID, SaveAgentInput{
		ID: agent.ID, Name: agent.Name, Description: agent.Description, Avatar: agent.Avatar,
		Category: agent.Category, Enabled: agent.Enabled, Model: agent.Model,
		SystemPrompt: agent.SystemPrompt, Tools: agent.Tools, SkillIDs: agent.SkillIDs, Permissions: agent.Permissions,
	})
	if err != nil {
		t.Fatal(err)
	}
	config := store.effectiveAgentConfig(updated)
	if config.Pricing != override {
		t.Fatalf("effective pricing=%+v, want %+v", config.Pricing, override)
	}
}

func TestNegativeModelPricingRejected(t *testing.T) {
	if err := validateModelPricing(ModelPricing{Input: -0.01}); err == nil {
		t.Fatal("expected negative model pricing to be rejected")
	}
}
