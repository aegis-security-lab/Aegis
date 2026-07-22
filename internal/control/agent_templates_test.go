package control

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestExistingAgentsSeedTalentTemplates(t *testing.T) {
	store := configuredStore(t)
	templates := store.AgentTemplates()
	if len(templates) < len(store.Agents()) {
		t.Fatalf("templates=%d agents=%d", len(templates), len(store.Agents()))
	}
	for _, agent := range store.Agents() {
		if agent.TemplateID == "" {
			t.Fatalf("Agent %s has no template", agent.ID)
		}
		template, err := store.GetAgentTemplate(agent.TemplateID)
		if err != nil {
			t.Fatalf("Agent %s template: %v", agent.ID, err)
		}
		if template.SystemPrompt != agent.SystemPrompt || template.Provider != agent.Model.Provider || template.Model != agent.Model.Model {
			t.Fatalf("Agent %s was not resolved from its template", agent.ID)
		}
	}
}

func TestDevelopmentLeadIsSeededWithPlanningResponsibilities(t *testing.T) {
	store := configuredStore(t)
	agent, err := store.GetAgent("development-lead")
	if err != nil {
		t.Fatal(err)
	}
	if !agent.Builtin || !agent.Enabled || agent.Category != "development" {
		t.Fatalf("unexpected development lead definition: %+v", agent)
	}
	for _, expected := range []string{"market", "development plan", "aegis_create_subissues", "backend-engineer", "frontend-engineer", "aegis_report_progress"} {
		if !strings.Contains(strings.ToLower(agent.SystemPrompt), strings.ToLower(expected)) {
			t.Fatalf("development lead prompt missing %q", expected)
		}
	}
	if !slices.Contains(agent.SkillIDs, "development-planning") || !slices.Contains(agent.SkillIDs, "decompose-issues") {
		t.Fatalf("development lead skills=%v", agent.SkillIDs)
	}
	if !agent.Permissions.AllowNetwork || !agent.Permissions.AllowWrite {
		t.Fatalf("development lead lacks planning permissions: %+v", agent.Permissions)
	}
}

func TestAgentTemplateIDAndEmployeeTemplateLock(t *testing.T) {
	store := configuredStore(t)
	prompt := "You are a focused release security engineer."
	template, err := store.SaveAgentTemplate(SaveAgentTemplateInput{
		Provider: "OpenAI", Model: "GPT-5.4", SystemPrompt: prompt, Note: "用户备注",
		Metadata: AgentTemplateMetadata{EnglishName: "Release Security Engineer", ChineseName: "发布安全工程师", Introduction: "负责发布前安全核验。", Positions: []string{"security", "release"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("OpenAI\x00GPT-5.4\x00" + prompt))
	expectedID := hex.EncodeToString(digest[:])
	if template.ID != expectedID || len(template.ID) != 64 {
		t.Fatalf("unexpected template id: %s", template.ID)
	}
	if template.Note != "用户备注" {
		t.Fatalf("template note=%q", template.Note)
	}
	if _, err := store.SaveAgentTemplate(SaveAgentTemplateInput{
		Provider: "OpenAI", Model: "GPT-5.4", SystemPrompt: prompt,
		Metadata: AgentTemplateMetadata{EnglishName: "Copy", ChineseName: "副本", Introduction: "重复身份。", Positions: []string{"security"}},
	}); !errors.Is(err, ErrAgentTemplateIDConflict) {
		t.Fatalf("duplicate talent error=%v", err)
	}
	updatedTemplate, err := store.UpdateAgentTemplateNote(template.ID, "修改后的备注")
	if err != nil {
		t.Fatal(err)
	}
	if updatedTemplate.ID != template.ID || updatedTemplate.Note != "修改后的备注" {
		t.Fatalf("updated template=%+v", updatedTemplate)
	}
	if _, err := store.UpdateAgentTemplateNote(template.ID, strings.Repeat("注", 10001)); err == nil {
		t.Fatal("expected oversized note to be rejected")
	}
	if AgentTemplateID("OpenAI", "GPT-5.4", prompt) == AgentTemplateID("Anthropic", "GPT-5.4", prompt) ||
		AgentTemplateID("OpenAI", "GPT-5.4", prompt) == AgentTemplateID("OpenAI", "Claude", prompt) {
		t.Fatal("provider and model must participate in the talent identity hash")
	}
	if AgentTemplateID(" OpenAI ", " GPT-5.4 ", " "+prompt+" ") != expectedID {
		t.Fatal("talent identity inputs must be trimmed before hashing")
	}
	agent, err := store.CreateAgent(SaveAgentInput{
		ID: "release-security", TemplateID: template.ID, Name: "ignored", Description: "ignored", Category: "general", Enabled: true,
		Model: AgentModelConfig{BaseURL: "https://example.invalid"}, SystemPrompt: "must not win", Tools: []string{"read"},
		Permissions: PermissionBoundary{WorkspaceScope: "run_workspace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.SystemPrompt != prompt || agent.Model.Provider != template.Provider || agent.Model.Model != template.Model || agent.Name != template.Metadata.ChineseName || agent.Category != "security" {
		t.Fatalf("employee did not inherit locked template fields: %+v", agent)
	}
}

func TestSeedAgentTemplatesNormalizesStoredTalentAndEmployeeIDs(t *testing.T) {
	store := configuredStore(t)
	agent := store.Agents()[0]
	template, err := store.GetAgentTemplate(agent.TemplateID)
	if err != nil {
		t.Fatal(err)
	}
	legacyID := "legacy-provider-model-prompt-hash"
	legacy := template
	legacy.ID = legacyID
	if err := store.db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	index, ok := agentIndex(store.agents, agent.ID)
	if !ok {
		store.mu.Unlock()
		t.Fatalf("agent %s is missing", agent.ID)
	}
	store.agents[index].TemplateID = legacyID
	updated := store.agents[index]
	if err := store.db.Save(&agentRecord{ID: updated.ID, Definition: updated, CreatedAt: updated.CreatedAt, UpdatedAt: time.Now()}).Error; err != nil {
		store.mu.Unlock()
		t.Fatal(err)
	}
	store.mu.Unlock()

	if err := store.seedAgentTemplates(time.Now()); err != nil {
		t.Fatal(err)
	}
	normalized, err := store.GetAgent(agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedID := AgentTemplateID(normalized.Model.Provider, normalized.Model.Model, normalized.SystemPrompt)
	if normalized.TemplateID != expectedID {
		t.Fatalf("employee template id=%q want content hash %q", normalized.TemplateID, expectedID)
	}
	if _, err := store.GetAgentTemplate(legacyID); err == nil {
		t.Fatal("legacy talent id was not removed")
	}
}
