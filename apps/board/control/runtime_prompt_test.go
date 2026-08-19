package control

import (
	"strings"
	"testing"
)

func TestPlanningPromptIncludesCompleteIssueContext(t *testing.T) {
	issue := Issue{
		Identifier:  "AEG-0042",
		Title:       "Run the benchmark",
		Description: "Use the supplied endpoint and credentials before decomposing the work.",
		Objective:   "Complete every challenge.",
		Workspace:   "/workspace",
	}
	prompt := planningPrompt(issue, 4, 10, 20, IssueBudgetConfig{MaxTurns: 100, ActiveTimeMinutes: 20})
	for _, expected := range []string{issue.Identifier, issue.Title, issue.Description, issue.Objective, issue.Workspace} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("planning prompt is missing %q:\n%s", expected, prompt)
		}
	}
}

func TestContinuationPromptRequiresCompletionReassessmentAndFollowupDelegation(t *testing.T) {
	prompt := continuationPrompt(
		Issue{Identifier: "AEG-0001", Title: "Parent", Objective: "Complete coverage", Workspace: "/workspace"},
		[]Issue{{Identifier: "AEG-0002", Title: "Child", Status: "done", Result: "Discovered another attack path."}},
	)
	for _, expected := range []string{
		"Call aegis_list_child_issues to refresh the complete direct-child state",
		"acceptance owner for the parent objective",
		"classify it as PASS, FAIL, or UNPROVEN",
		"proves only that child's objective",
		"post a concrete Board comment describing the failed acceptance criterion and notify that owner to continue",
		"dispatch the next small dependency-free wave",
		"estimatedWaitMinutes",
		"MUST continue implementation",
		"Do not reinterpret, weaken, or replace the parent objective",
		"Only then run parent-level verification",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("continuation prompt is missing %q:\n%s", expected, prompt)
		}
	}
}
