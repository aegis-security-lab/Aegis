package control

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCreateSubIssuesIsAtomicIdempotentAndBuildsDependencies(t *testing.T) {
	s := configuredStore(t)
	parent, err := s.CreateIssue(CreateIssueInput{
		Title: "Implement a broad feature", Priority: "high", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer", AcceptanceCriteria: "The integrated feature passes tests.",
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
			{Title: "Implement API", Description: "Add the API contract.", AcceptanceCriteria: "API tests pass.", Priority: "high", AgentID: "backend-engineer", DependsOn: []int{}},
			{Title: "Integrate UI", Description: "Consume the API.", AcceptanceCriteria: "UI build passes.", Priority: "medium", AgentID: "frontend-engineer", DependsOn: []int{1}},
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

func TestStoreNormalizesHistoricalIssueDepths(t *testing.T) {
	s := configuredStore(t)
	root, err := s.CreateIssue(CreateIssueInput{Title: "Root", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Child", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := s.CreateIssue(CreateIssueInput{ParentID: child.ID, Title: "Grandchild", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.db.Model(&Issue{}).Where("id IN ?", []string{child.ID, grandchild.ID}).Update("request_depth", 0).Error; err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(filepath.Clean(s.DataDir()))
	if err != nil {
		t.Fatal(err)
	}
	child, _ = reopened.GetIssue(child.ID)
	grandchild, _ = reopened.GetIssue(grandchild.ID)
	if child.RequestDepth != 1 || grandchild.RequestDepth != 2 {
		t.Fatalf("historical depths were not normalized: child=%d grandchild=%d", child.RequestDepth, grandchild.RequestDepth)
	}
}

func TestIssueDepthAndHierarchyCycleBoundaries(t *testing.T) {
	s := configuredStore(t)
	parent, err := s.CreateIssue(CreateIssueInput{Title: "Level 0", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	root := parent
	for depth := 1; depth <= maxIssueDepth; depth++ {
		parent, err = s.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Nested", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
		if err != nil {
			t.Fatalf("create depth %d: %v", depth, err)
		}
		if parent.RequestDepth != depth {
			t.Fatalf("depth=%d, want %d", parent.RequestDepth, depth)
		}
	}
	if _, err = s.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Too deep", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"}); err == nil {
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
