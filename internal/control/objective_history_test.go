package control

import "testing"

func TestIssueObjectiveHistoryFromOperatorComment(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Versioned objective", Objective: "Deliver version one.",
		Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	comment, err := manager.AddIssueCommentWithObjective(issue.ID, "Please use the revised target.", "Deliver version two with evidence.")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Objective != "Deliver version two with evidence." {
		t.Fatalf("objective=%q", updated.Objective)
	}
	detail, err := store.GetIssueDetail(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Objectives) != 2 {
		t.Fatalf("objectives=%d, want 2", len(detail.Objectives))
	}
	if !detail.Objectives[0].Current || detail.Objectives[0].Version != 2 || detail.Objectives[0].SourceCommentID != comment.ID {
		t.Fatalf("unexpected current objective: %+v", detail.Objectives[0])
	}
	if detail.Objectives[1].Version != 1 {
		t.Fatalf("unexpected history: %+v", detail.Objectives[1])
	}
}

func TestIssueObjectiveValidationSummaryIsVersionScoped(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Validation history", Objective: "First target.", Priority: "middle", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	var first IssueObjective
	if err = store.db.Where("issue_id = ?", issue.ID).First(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&IssueValidation{ID: nextID("validation"), IssueID: issue.ID, ObjectiveID: first.ID, Attempt: 1, Objective: first.Content, Status: "passed", Passed: true}).Error; err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if _, err = manager.AddIssueCommentWithObjective(issue.ID, "", "Second target."); err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetIssueDetail(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Objectives[0].ValidationRounds != 0 {
		t.Fatalf("new target inherited validation rounds: %+v", detail.Objectives[0])
	}
	if detail.Objectives[1].ValidationRounds != 1 || !detail.Objectives[1].ValidationPassed {
		t.Fatalf("old target lost validation result: %+v", detail.Objectives[1])
	}
}
