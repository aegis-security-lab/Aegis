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
	if len(tools[1].Parameters) != 2 || tools[1].Parameters[1].Name != "timeout" {
		t.Fatalf("unknown tool must expose the common timeout parameter: %+v", tools[1].Parameters)
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
	if len(tools[0].Parameters) != 5 || tools[0].Parameters[0].Name != "invocationDescription" || tools[0].Parameters[1].Name != "timeout" || tools[0].Parameters[4].Name != "currentActivity" {
		t.Fatalf("progress parameters=%+v", tools[0].Parameters)
	}
}

func TestGetIssueProgressToolSnapshotHasModesAndSeparateLimits(t *testing.T) {
	tools := snapshotTools([]string{"aegis_get_issue_progress"})
	if len(tools) != 1 || len(tools[0].Parameters) != 6 {
		t.Fatalf("issue progress tool snapshot=%+v", tools)
	}
	mode := tools[0].Parameters[3]
	if mode.Name != "mode" || len(mode.Enum) != 3 || mode.Enum[0] != "progress" {
		t.Fatalf("issue progress mode=%+v", mode)
	}
	if tools[0].Parameters[4].Name != "progressLimit" || tools[0].Parameters[5].Name != "messageLimit" {
		t.Fatalf("issue progress limits=%+v", tools[0].Parameters)
	}
}

func TestOfficeAppToolSnapshotsHaveStructuredActions(t *testing.T) {
	tools := snapshotTools([]string{"aegis_board", "aegis_relay"})
	if len(tools) != 2 || len(tools[0].Parameters) != 7 || len(tools[1].Parameters) != 6 {
		t.Fatalf("office app tool snapshots=%+v", tools)
	}
	if action := tools[0].Parameters[2]; action.Name != "action" || len(action.Enum) != 10 {
		t.Fatalf("Board action parameter=%+v", action)
	}
	if action := tools[1].Parameters[2]; action.Name != "action" || len(action.Enum) != 4 {
		t.Fatalf("Relay action parameter=%+v", action)
	}
}

func TestConciergeCreateTaskRequiresObjectiveAndExecutionBoundary(t *testing.T) {
	tools := snapshotTools([]string{"aegis_create_task"})
	if len(tools) != 1 {
		t.Fatalf("create task tool snapshot=%+v", tools)
	}
	required := map[string]bool{}
	for _, parameter := range tools[0].Parameters {
		if parameter.Required {
			required[parameter.Name] = true
		}
	}
	for _, name := range []string{"objective"} {
		if !required[name] {
			t.Fatalf("create task tool must require %s: %+v", name, tools[0].Parameters)
		}
	}
}

func TestEveryCatalogToolRequiresInvocationDescription(t *testing.T) {
	for name, tool := range toolCatalog() {
		if len(tool.Parameters) == 0 {
			t.Fatalf("tool %s has no parameters", name)
		}
		purpose := tool.Parameters[0]
		if purpose.Name != "invocationDescription" || purpose.Type != "string" || purpose.Required || purpose.Description == "" {
			t.Fatalf("tool %s does not expose a backward-compatible invocation description: %+v", name, purpose)
		}
		if len(tool.Parameters) < 2 {
			t.Fatalf("tool %s has no common timeout parameter", name)
		}
		timeout := tool.Parameters[1]
		if timeout.Name != "timeout" || timeout.Type != "number" || timeout.Required || !strings.Contains(timeout.Description, "60") {
			t.Fatalf("tool %s does not expose the optional 60-second timeout: %+v", name, timeout)
		}
		seen := map[string]bool{}
		for _, parameter := range tool.Parameters {
			if seen[parameter.Name] {
				t.Fatalf("tool %s has duplicate top-level parameter %q", name, parameter.Name)
			}
			seen[parameter.Name] = true
		}
	}
}

func TestAgentPromptRequiresPurposeForEveryToolCall(t *testing.T) {
	prompt := agentToolDescriptionSystemPrompt("base")
	for _, expected := range []string{"Every tool schema", "invocationDescription field", "reason and purpose of this specific invocation", "one short sentence"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("tool invocation prompt missing %q: %s", expected, prompt)
		}
	}
}
