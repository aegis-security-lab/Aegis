package control

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func configuredStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	_, err = s.SaveConfig(SaveConfigInput{NodePath: executable, PiPath: executable, Provider: "test", Model: "test-model", Thinking: "medium", AuthMode: "environment", Workspace: workspace, Concurrency: 2, ApprovalMode: "risky"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIssueHierarchyRelationsAndCheckout(t *testing.T) {
	s := configuredStore(t)
	parent, err := s.CreateIssue(CreateIssueInput{Title: "Build product", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "aegis-orchestrator"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Backend", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Frontend", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := s.AddRelation(a.ID, CreateRelationInput{RelatedIssueID: b.ID, Type: "blocks"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckoutIssue(b.ID, CheckoutIssueInput{AgentID: "frontend-engineer", ExecutionID: "exec-b", ExpectedStatuses: []string{"todo"}}); err == nil {
		t.Fatal("blocked issue checkout should fail")
	}
	done := "done"
	if _, err = s.UpdateIssue(a.ID, UpdateIssueInput{Status: &done}); err != nil {
		t.Fatal(err)
	}
	checked, err := s.CheckoutIssue(b.ID, CheckoutIssueInput{AgentID: "frontend-engineer", ExecutionID: "exec-b", ExpectedStatuses: []string{"todo"}})
	if err != nil {
		t.Fatal(err)
	}
	if checked.Status != "in_progress" || checked.CheckoutExecutionID != "exec-b" {
		t.Fatalf("unexpected checkout: %+v", checked)
	}
	if _, err = s.CheckoutIssue(b.ID, CheckoutIssueInput{AgentID: "frontend-engineer", ExecutionID: "other", ExpectedStatuses: []string{"todo", "in_progress"}}); err == nil {
		t.Fatal("second checkout should conflict")
	}
	if err = s.DeleteRelation(relation.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := s.GetIssueDetail(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Children) != 2 {
		t.Fatalf("children=%d", len(detail.Children))
	}
}

func TestTaskKeepsSelectedAgentAndEveryAgentCanDecompose(t *testing.T) {
	s := configuredStore(t)
	task, err := s.CreateIssue(CreateIssueInput{
		Title: "Implement a focused API", Priority: "high", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.AssigneeAgentID != "backend-engineer" {
		t.Fatalf("assignee=%q, want backend-engineer", task.AssigneeAgentID)
	}

	custom, err := s.CreateAgent(SaveAgentInput{
		ID: "custom-worker", Name: "Custom Worker", Enabled: true,
		SystemPrompt: "Complete the assigned Issue and report evidence.",
		Tools:        []string{"read"},
		Permissions:  PermissionBoundary{WorkspaceScope: "run_workspace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(custom.Tools, "aegis_create_subissues") {
		t.Fatalf("custom agent tools=%v, missing decomposition capability", custom.Tools)
	}

	custom.Tools = []string{"read"}
	updated, err := s.UpdateAgent(custom.ID, SaveAgentInput{
		Name: custom.Name, Enabled: true, SystemPrompt: custom.SystemPrompt,
		Tools: custom.Tools, Permissions: custom.Permissions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(updated.Tools, "aegis_create_subissues") {
		t.Fatalf("updated agent tools=%v, required capability was removed", updated.Tools)
	}
}

func TestRelationCycleRejectedAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	workspace := t.TempDir()
	_, err = s.SaveConfig(SaveConfigInput{NodePath: exe, PiPath: exe, Provider: "test", Model: "m", AuthMode: "environment", Workspace: workspace, Concurrency: 1, ApprovalMode: "none"})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.CreateIssue(CreateIssueInput{Title: "A", Priority: "medium", WorkMode: "guided"})
	b, _ := s.CreateIssue(CreateIssueInput{Title: "B", Priority: "medium", WorkMode: "guided"})
	if _, err = s.AddRelation(a.ID, CreateRelationInput{RelatedIssueID: b.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddRelation(b.ID, CreateRelationInput{RelatedIssueID: a.ID}); err == nil {
		t.Fatal("cycle should fail")
	}
	reopened, err := NewStore(filepath.Clean(dir))
	if err != nil {
		t.Fatal(err)
	}
	state := reopened.State()
	if len(state.Issues) != 2 || len(state.Relations) != 1 {
		t.Fatalf("state not persisted: %+v", state)
	}
	if state.Config.HasAPIKey {
		t.Fatal("unexpected secret flag")
	}
}

func TestFindingCRUD(t *testing.T) {
	s := configuredStore(t)

	// Create a finding
	f, err := s.CreateFinding(CreateFindingInput{
		Domain:           "example.com",
		Category:         "tls",
		Severity:         "high",
		Title:            "Weak TLS cipher",
		Description:      "The server supports weak TLS 1.0 ciphers.",
		Evidence:         map[string]any{"cipher": "TLS_RSA_WITH_3DES_EDE_CBC_SHA", "port": 443},
		StepsToReproduce: "Run: nmap --script ssl-enum-ciphers -p 443 example.com",
		Remediation:      "Disable TLS 1.0 and use only TLS 1.2+ with strong ciphers.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.ID == "" || f.Status != "open" || f.Domain != "example.com" {
		t.Fatalf("unexpected finding: %+v", f)
	}
	if f.Evidence == nil {
		t.Fatal("evidence should be set")
	}

	// Get finding by ID
	got, err := s.GetFinding(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Weak TLS cipher" {
		t.Fatalf("title mismatch: %s", got.Title)
	}

	// List findings with no filter
	list, err := s.ListFindings("", "", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", list.Total)
	}

	// List with category filter
	list, err = s.ListFindings("tls", "", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 {
		t.Fatalf("expected 1 tls finding, got %d", list.Total)
	}

	// List with severity filter
	list, err = s.ListFindings("", "high", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 {
		t.Fatalf("expected 1 high finding, got %d", list.Total)
	}

	// List with non-matching filter
	list, err = s.ListFindings("vuln", "", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 0 || len(list.Findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", list.Total)
	}

	// Create another finding for pagination test
	_, err = s.CreateFinding(CreateFindingInput{
		Domain:   "test.org",
		Category: "recon",
		Severity: "info",
		Title:    "Open ports detected",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Pagination: page 1 with pageSize 1 should return 1 item
	list, err = s.ListFindings("", "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 2 || len(list.Findings) != 1 {
		t.Fatalf("expected total=2 items=1, got total=%d items=%d", list.Total, len(list.Findings))
	}
	if list.Page != 1 || list.PageSize != 1 {
		t.Fatalf("page/pageSize mismatch: %d/%d", list.Page, list.PageSize)
	}

	// Page 2 should have the remaining 1
	list, err = s.ListFindings("", "", 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Findings) != 1 {
		t.Fatalf("expected 1 finding on page 2, got %d", len(list.Findings))
	}

	// Update status
	status := "verified"
	updated, err := s.UpdateFinding(f.ID, UpdateFindingInput{Status: &status})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "verified" {
		t.Fatalf("status not updated: %s", updated.Status)
	}

	// Update multiple fields
	title := "Updated title"
	sev := "critical"
	updated, err = s.UpdateFinding(f.ID, UpdateFindingInput{Title: &title, Severity: &sev})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Updated title" || updated.Severity != "critical" {
		t.Fatalf("update mismatch: %+v", updated)
	}

	// Invalid category should fail
	invalidCat := "invalid"
	if _, err = s.UpdateFinding(f.ID, UpdateFindingInput{Category: &invalidCat}); err == nil {
		t.Fatal("expected error for invalid category")
	}

	// Invalid severity should fail
	invalidSev := "invalid"
	if _, err = s.UpdateFinding(f.ID, UpdateFindingInput{Severity: &invalidSev}); err == nil {
		t.Fatal("expected error for invalid severity")
	}

	// Invalid status should fail
	invalidStatus := "invalid"
	if _, err = s.UpdateFinding(f.ID, UpdateFindingInput{Status: &invalidStatus}); err == nil {
		t.Fatal("expected error for invalid status")
	}

	// Get non-existent finding should fail
	if _, err = s.GetFinding("nonexistent"); err == nil {
		t.Fatal("expected error for non-existent finding")
	}

	// Empty domain should fail
	if _, err = s.CreateFinding(CreateFindingInput{Category: "tls", Severity: "high", Title: "Test"}); err == nil {
		t.Fatal("expected error for empty domain")
	}

	// Empty title should fail
	if _, err = s.CreateFinding(CreateFindingInput{Domain: "example.com", Category: "tls", Severity: "high"}); err == nil {
		t.Fatal("expected error for empty title")
	}

	// Invalid category should fail
	if _, err = s.CreateFinding(CreateFindingInput{Domain: "example.com", Category: "invalid", Severity: "high", Title: "Test"}); err == nil {
		t.Fatal("expected error for invalid category")
	}
}

func TestFindingEvidenceJSON(t *testing.T) {
	s := configuredStore(t)

	// Evidence as object
	f, err := s.CreateFinding(CreateFindingInput{
		Domain:   "example.com",
		Category: "headers",
		Severity: "medium",
		Title:    "Missing HSTS",
		Evidence: map[string]any{"header": "Strict-Transport-Security", "present": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.Evidence == nil {
		t.Fatal("evidence should not be nil")
	}

	// Evidence as array
	f2, err := s.CreateFinding(CreateFindingInput{
		Domain:   "example.com",
		Category: "recon",
		Severity: "info",
		Title:    "Subdomains",
		Evidence: []any{"admin.example.com", "api.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f2.Evidence == nil {
		t.Fatal("evidence should not be nil")
	}

	// Evidence as string
	f3, err := s.CreateFinding(CreateFindingInput{
		Domain:   "example.com",
		Category: "vuln",
		Severity: "critical",
		Title:    "SQL Injection",
		Evidence: "' OR '1'='1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if f3.Evidence == nil {
		t.Fatal("evidence should not be nil")
	}

	// Update evidence
	newEvidence := map[string]any{"patched": true}
	updated, err := s.UpdateFinding(f.ID, UpdateFindingInput{Evidence: newEvidence})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Evidence == nil {
		t.Fatal("evidence should be updated")
	}
}

func TestFindingPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	ws := t.TempDir()
	_, err = s.SaveConfig(SaveConfigInput{NodePath: exe, PiPath: exe, Provider: "test", Model: "m", AuthMode: "environment", Workspace: ws, Concurrency: 1, ApprovalMode: "none"})
	if err != nil {
		t.Fatal(err)
	}

	// Create a finding
	f, err := s.CreateFinding(CreateFindingInput{
		Domain:   "example.com",
		Category: "cors",
		Severity: "low",
		Title:    "CORS misconfiguration",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reopen the store and verify finding persists
	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.GetFinding(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "CORS misconfiguration" || got.Status != "open" {
		t.Fatalf("finding not persisted: %+v", got)
	}
}

func TestDefaultRegistry(t *testing.T) {
	s := configuredStore(t)
	if len(s.Agents()) < 4 {
		t.Fatal("default agents missing")
	}
	for _, id := range []string{"aegis-orchestrator", "backend-engineer", "frontend-engineer", "red-team-engineer"} {
		a, err := s.GetAgent(id)
		if err != nil {
			t.Fatal(err)
		}
		if a.SystemPrompt == "" || len(a.Tools) == 0 || len(a.SkillIDs) == 0 {
			t.Fatalf("incomplete agent %s", id)
		}
	}
}
