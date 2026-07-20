package control

import (
	"strings"
	"testing"
)

func TestContinuationPromptRequiresCompletionReassessmentAndFollowupDelegation(t *testing.T) {
	prompt := continuationPrompt(
		Issue{Identifier: "AEG-0001", Title: "Parent", Objective: "Complete coverage", Workspace: "/workspace"},
		[]Issue{{Identifier: "AEG-0002", Title: "Child", Status: "done", Result: "Discovered another attack path."}},
	)
	for _, expected := range []string{
		"Call aegis_list_child_issues to refresh the complete direct-child state",
		"Reassess the parent objective and overall task completion",
		"new facts that were unavailable during the original decomposition",
		"missing coverage, insufficient detail, an untested path",
		"call aegis_create_subissues again and dispatch a new bounded wave",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("continuation prompt is missing %q:\n%s", expected, prompt)
		}
	}
}
