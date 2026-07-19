package control

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCreateSubIssuesIsAtomicIdempotentAndBuildsDependencies(t *testing.T) {
	s := configuredStore(t)
	parent, err := s.CreateIssue(CreateIssueInput{
		Title: "Implement a broad feature", Priority: "high", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer", Objective: "The integrated feature passes tests.",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.createExecution(parent, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckoutIssue(parent.ID, CheckoutIssueInput{
		AgentID: "backend-engineer", ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = s.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}

	input := DecomposeIssueInput{
		RequestKey: "feature-v1",
		Summary:    "Backend and UI have separate acceptance criteria.",
		Children: []SubIssueSpec{
			{Title: "Implement API", Description: "Add the API contract.", Objective: "API tests pass.", Priority: "high", AgentID: "backend-engineer", DependsOn: []int{}},
			{Title: "Integrate UI", Description: "Consume the API.", Objective: "UI build passes.", Priority: "medium", AgentID: "frontend-engineer", DependsOn: []int{1}},
		},
	}
	tooLong := input
	tooLong.RequestKey = "feature-title-too-long"
	tooLong.Children = slices.Clone(input.Children)
	tooLong.Children[0].Title = strings.Repeat("子", IssueTitleMaxLength+1)
	if _, err = s.CreateSubIssues(parent.ID, execution.ID, "backend-engineer", tooLong); err == nil {
		t.Fatal("decomposition should reject a child Issue title over the limit")
	}
	result, err := s.CreateSubIssues(parent.ID, execution.ID, "backend-engineer", input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reused || len(result.Children) != 2 {
		t.Fatalf("unexpected decomposition result: %+v", result)
	}
	for _, child := range result.Children {
		if child.ParentID != parent.ID || child.RequestDepth != 1 || child.Status != "todo" {
			t.Fatalf("invalid child: %+v", child)
		}
	}
	updatedParent, err := s.GetIssue(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedParent.Status != "in_progress" || updatedParent.ExecutionPhase != "waiting_children" || updatedParent.CheckoutExecutionID != "" {
		t.Fatalf("parent did not enter waiting state: %+v", updatedParent)
	}
	var relations []IssueRelation
	if err = s.db.Order("created_at asc").Find(&relations).Error; err != nil {
		t.Fatal(err)
	}
	if len(relations) != 3 {
		t.Fatalf("relations=%d, want child dependency plus two parent blockers", len(relations))
	}
	if !slices.ContainsFunc(relations, func(relation IssueRelation) bool {
		return relation.IssueID == result.Children[0].ID && relation.RelatedIssueID == result.Children[1].ID
	}) {
		t.Fatal("child dependency edge is missing")
	}

	reused, err := s.CreateSubIssues(parent.ID, execution.ID, "backend-engineer", input)
	if err != nil {
		t.Fatal(err)
	}
	if !reused.Reused || len(reused.Children) != 2 || reused.Children[0].ID != result.Children[0].ID {
		t.Fatalf("retry was not idempotent: %+v", reused)
	}
	var issueCount int64
	s.db.Model(&Issue{}).Count(&issueCount)
	if issueCount != 3 {
		t.Fatalf("idempotent retry created duplicates: %d issues", issueCount)
	}

	detail, err := s.GetIssueDetail(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Decompositions) != 1 || len(detail.Children) != 2 || len(detail.BlockedBy) != 2 {
		t.Fatalf("incomplete issue detail: %+v", detail)
	}
}

func TestCreateSubIssuesReopensCompletedIssueFromActiveSession(t *testing.T) {
	s := configuredStore(t)
	parent, err := s.CreateIssue(CreateIssueInput{
		Title: "Completed task with new requested work", Objective: "Deliver the original task.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.createExecution(parent, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckoutIssue(parent.ID, CheckoutIssueInput{
		AgentID: execution.AgentID, ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = s.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{
		"status": "done", "execution_phase": "completed", "checkout_execution_id": "", "completed_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.updateExecution(execution.ID, map[string]any{"status": "running", "finished_at": nil}); err != nil {
		t.Fatal(err)
	}

	result, err := s.CreateSubIssues(parent.ID, execution.ID, execution.AgentID, DecomposeIssueInput{
		RequestKey: "comment-follow-up", Summary: "The operator requested two new workstreams.",
		Children: []SubIssueSpec{
			{Title: "Investigate the follow-up", Objective: "Investigation evidence is recorded.", Priority: "high", AgentID: "backend-engineer"},
			{Title: "Deliver the follow-up", Objective: "The requested follow-up is delivered.", Priority: "medium", AgentID: "frontend-engineer", DependsOn: []int{1}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Children) != 2 {
		t.Fatalf("children=%d, want 2", len(result.Children))
	}
	reopened, err := s.GetIssue(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status != "in_progress" || reopened.ExecutionPhase != "waiting_children" || reopened.CompletedAt != nil || reopened.CurrentExecutionID != execution.ID {
		t.Fatalf("completed Issue was not reopened by decomposition: %+v", reopened)
	}
}

func TestChildIssuesSkipValidationWhenParentHasNoObjective(t *testing.T) {
	s := configuredStore(t)
	parent, err := s.CreateIssue(CreateIssueInput{
		Title: "Task without acceptance flow", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	manualChild, err := s.CreateIssue(CreateIssueInput{
		ParentID: parent.ID, Title: "Manually added work item", Objective: "Record the manual outcome.",
		Priority: "low", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !manualChild.ValidationDisabled {
		t.Fatalf("manually created child did not inherit no-validation policy: %+v", manualChild)
	}
	execution, err := s.createExecution(parent, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckoutIssue(parent.ID, CheckoutIssueInput{
		AgentID: execution.AgentID, ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = s.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	result, err := s.CreateSubIssues(parent.ID, execution.ID, execution.AgentID, DecomposeIssueInput{
		RequestKey: "no-validation-tree", Summary: "Split the work without enabling acceptance validation.",
		Children: []SubIssueSpec{
			{Title: "First work item", Objective: "Record the first outcome.", Priority: "medium", AgentID: "backend-engineer"},
			{Title: "Second work item", Objective: "Record the second outcome.", Priority: "medium", AgentID: "frontend-engineer"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range result.Children {
		if !child.ValidationDisabled {
			t.Fatalf("child did not inherit no-validation policy: %+v", child)
		}
	}
}

func TestIssueDepthAndHierarchyCycleBoundaries(t *testing.T) {
	s := configuredStore(t)
	parent, err := s.CreateIssue(CreateIssueInput{Title: "Level 0", Objective: "Complete level.", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	root := parent
	for depth := 1; depth <= maxIssueDepth; depth++ {
		parent, err = s.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Nested", Objective: "Complete nested level.", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
		if err != nil {
			t.Fatalf("create depth %d: %v", depth, err)
		}
		if parent.RequestDepth != depth {
			t.Fatalf("depth=%d, want %d", parent.RequestDepth, depth)
		}
	}
	if _, err = s.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Too deep", Objective: "Complete excess level.", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"}); err == nil {
		t.Fatal("creating beyond the maximum hierarchy depth should fail")
	}
	if _, err = s.UpdateIssue(root.ID, UpdateIssueInput{ParentID: &parent.ID}); err == nil {
		t.Fatal("moving an ancestor below its descendant should fail")
	}
}

func TestManagerRejectsInvalidExecutionControlToken(t *testing.T) {
	s := configuredStore(t)
	m, err := NewManager(s)
	if err != nil {
		t.Fatal(err)
	}
	m.sessions["execution-1"] = &PiSession{
		executionID: "execution-1", issueID: "issue-1", agentID: "backend-engineer", controlToken: "secret-token",
	}
	if _, err = m.DecomposeExecution("execution-1", "wrong-token", DecomposeIssueInput{}); err == nil {
		t.Fatal("invalid control token should be rejected")
	}
	delete(m.sessions, "execution-1")
	m.Close()
}
