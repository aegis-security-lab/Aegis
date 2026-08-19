package control

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeTaskSkillCopiesDirectoryManifest(t *testing.T) {
	store := configuredStore(t)
	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)

	var skill SkillDefinition
	for _, candidate := range store.Skills() {
		if candidate.ID == "decompose-issues" {
			skill = candidate
			break
		}
	}
	if skill.ID == "" {
		t.Fatal("builtin decompose-issues Skill is missing")
	}
	paths := store.skillPaths([]string{skill.ID})
	if len(paths) != 1 {
		t.Fatalf("unexpected Skill paths: %#v", paths)
	}

	issue := Issue{ID: "issue-skill-copy"}
	containerPath, err := manager.materializeTaskSkill(issue, paths[0])
	if err != nil {
		t.Fatalf("materialize directory Skill: %v", err)
	}
	if containerPath != TaskWorkspacePath+"/.aegis/skills/decompose-issues" {
		t.Fatalf("unexpected container Skill root: %q", containerPath)
	}

	manifestPath := filepath.Join(store.DataDir(), "test-task-runtime", issue.ID, ".aegis", "skills", skill.ID, "SKILL.md")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read copied SKILL.md: %v", err)
	}
	if string(content) != skill.Content+"\n" {
		t.Fatalf("copied SKILL.md does not match registry content:\n%s", content)
	}
}
