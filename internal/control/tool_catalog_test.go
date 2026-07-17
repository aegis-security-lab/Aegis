package control

import "testing"

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
