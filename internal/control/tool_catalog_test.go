package control

import (
	"strings"
	"testing"
)

func TestSnapshotToolsPreservesOrderDeduplicatesAndRetainsUnknownTools(t *testing.T) {
	tools := snapshotTools([]string{"bash", "custom_tool", "bash", "read"})
	if len(tools) != 3 {
		t.Fatalf("expected 3 unique tool snapshots, got %d", len(tools))
	}
	if tools[0].Name != "bash" || tools[1].Name != "custom_tool" || tools[2].Name != "read" {
		t.Fatalf("unexpected tool order: %#v", []string{tools[0].Name, tools[1].Name, tools[2].Name})
	}
	if tools[1].Source != "unknown" {
		t.Fatalf("expected unknown source, got %q", tools[1].Source)
	}
	if tools[1].Description == "" {
		t.Fatal("expected unknown tools to retain an explanatory description")
	}
}

func TestSnapshotToolsReturnsNonNilEmptySnapshot(t *testing.T) {
	tools := snapshotTools(nil)
	if tools == nil {
		t.Fatal("expected an empty snapshot slice instead of nil")
	}
	if len(tools) != 0 {
		t.Fatalf("expected no tools, got %d", len(tools))
	}
}

func TestProgressToolSnapshotHasStructuredParameters(t *testing.T) {
	tools := snapshotTools([]string{"aegis_report_progress"})
	if len(tools) != 1 || tools[0].Source != "aegis_extension" {
		t.Fatalf("progress tool snapshot=%+v", tools)
	}
	if len(tools[0].Parameters) != 4 || tools[0].Parameters[0].Name != "description" || tools[0].Parameters[3].Name != "currentActivity" {
		t.Fatalf("progress parameters=%+v", tools[0].Parameters)
	}
}

func TestBroadcastToolSnapshotsHaveStructuredParameters(t *testing.T) {
	tools := snapshotTools([]string{"aegis_broadcast", "aegis_list_broadcasts"})
	if len(tools) != 2 || len(tools[0].Parameters) != 4 || len(tools[1].Parameters) != 2 {
		t.Fatalf("broadcast tool snapshots=%+v", tools)
	}
	importance := tools[0].Parameters[3]
	if importance.Name != "importance" || len(importance.Enum) != 3 {
		t.Fatalf("broadcast importance parameter=%+v", importance)
	}
}

func TestEveryCatalogToolRequiresInvocationDescription(t *testing.T) {
	for name, tool := range toolCatalog() {
		if len(tool.Parameters) == 0 {
			t.Fatalf("tool %s has no parameters", name)
		}
		purpose := tool.Parameters[0]
		if purpose.Name != "description" || purpose.Type != "string" || !purpose.Required || purpose.Description == "" {
			t.Fatalf("tool %s does not require an invocation description: %+v", name, purpose)
		}
	}
}

func TestAgentPromptRequiresPurposeForEveryToolCall(t *testing.T) {
	prompt := agentToolDescriptionSystemPrompt("base")
	for _, expected := range []string{"Every tool schema", "required description field", "purpose of this specific invocation", "one short sentence"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("tool invocation prompt missing %q: %s", expected, prompt)
		}
	}
}
