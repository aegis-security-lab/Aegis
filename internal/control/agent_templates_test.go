package control

import (
	"strings"
	"testing"
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

func TestAgentTemplateIDAndEmployeeTemplateLock(t *testing.T) {
	store := configuredStore(t)
	prompt := "You are a focused release security engineer."
	template, err := store.SaveAgentTemplate(SaveAgentTemplateInput{
		Provider: "OpenAI", Model: "GPT-5.4", SystemPrompt: prompt,
		Metadata: AgentTemplateMetadata{EnglishName: "Release Security Engineer", ChineseName: "发布安全工程师", Introduction: "负责发布前安全核验。", Positions: []string{"security", "release"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if template.ID != AgentTemplateID("OpenAI", "GPT-5.4", prompt) || !strings.HasSuffix(template.ID, "-"+template.ID[len(template.ID)-6:]) {
		t.Fatalf("unexpected template id: %s", template.ID)
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
