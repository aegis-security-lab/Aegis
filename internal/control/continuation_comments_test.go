package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContinuationPromptIncludesAllChildCommentsAndSpillsLargeHistory(t *testing.T) {
	store := configuredStore(t)
	workspace := t.TempDir()
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Parent", Objective: "Integrate everything", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Child", Objective: "Produce evidence", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	oldMarker := "OLD-COMMENT-MUST-BE-IN-FILE"
	newMarker := "NEWEST-COMMENT-MUST-BE-IN-PROMPT"
	comments := []IssueComment{
		{ID: nextID("comment"), IssueID: child.ID, Type: "delivery", AuthorType: "agent", AuthorID: "frontend-engineer", Body: oldMarker + "\n" + strings.Repeat("old evidence line\n", 4000), CreatedAt: now},
		{ID: nextID("comment"), IssueID: child.ID, Type: "validation_feedback", AuthorType: "agent", AuthorID: "acceptance-validator", Body: newMarker, CreatedAt: now.Add(time.Second)},
	}
	if err = store.db.Create(&comments).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store}
	prompt := manager.continuationPromptWithChildComments(parent, []Issue{child})
	if strings.Contains(prompt, oldMarker) {
		t.Fatal("oldest comment was not truncated from the continuation prompt")
	}
	if !strings.Contains(prompt, newMarker) || !strings.Contains(prompt, "You MUST read that file") {
		t.Fatalf("prompt does not contain newest tail and file instruction: %s", prompt)
	}
	contextPath := filepath.Join(store.DataDir(), "test-task-runtime", parent.ID, ".aegis", "context", "child-comments-"+parent.ID+".md")
	content, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), oldMarker) || !strings.Contains(string(content), newMarker) {
		t.Fatal("persisted comment history is incomplete")
	}
}

func TestTruncateTextTailKeepsNewestContent(t *testing.T) {
	tail, truncated := truncateTextTail("one\ntwo\nthree\nfour", 2, 1024)
	if !truncated || tail != "three\nfour" {
		t.Fatalf("tail=%q truncated=%v", tail, truncated)
	}
}
