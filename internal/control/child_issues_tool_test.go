package control

import "testing"

func TestExecutionChildIssuesReturnsOnlyCurrentIssueDirectChildren(t *testing.T) {
	store := configuredStore(t)
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "First", Objective: "First result", Priority: "high", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Second", Objective: "Second result", Priority: "medium", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := store.CreateIssue(CreateIssueInput{ParentID: first.ID, Title: "Nested", Priority: "low", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateIssue(CreateIssueInput{Title: "Unrelated", Priority: "low", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{key: "execution-parent", executionID: "execution-parent", issueID: parent.ID, controlToken: "secret"}
	manager.sessions[session.key] = session
	comment := IssueComment{ID: nextID("comment"), IssueID: first.ID, AuthorType: "agent", AuthorID: "backend-engineer", Body: "Detailed child evidence", Attachments: []IssueAttachment{}}
	if err := store.db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	children, err := manager.ExecutionChildIssues(session.executionID, "secret", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 || children[0].ID != first.ID || children[1].ID != second.ID {
		t.Fatalf("children=%+v; expected direct children %s and %s only (not %s or %s)", children, first.ID, second.ID, grandchild.ID, other.ID)
	}
	if len(children[0].Comments) != 1 || children[0].Comments[0].Body != comment.Body || children[1].Comments == nil || len(children[1].Comments) != 0 {
		t.Fatalf("comments were not included for every child: %+v", children)
	}
	withoutComments, err := manager.ExecutionChildIssues(session.executionID, "secret", false)
	if err != nil {
		t.Fatal(err)
	}
	if withoutComments[0].Comments != nil || withoutComments[1].Comments != nil {
		t.Fatalf("comments were returned when disabled: %+v", withoutComments)
	}
	if _, err = manager.ExecutionChildIssues(session.executionID, "wrong", true); err == nil {
		t.Fatal("invalid control token was accepted")
	}
}
