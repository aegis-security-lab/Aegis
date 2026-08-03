package control

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func configuredStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	_, err = s.SaveConfig(SaveConfigInput{Provider: "test", Model: "test-model", Pricing: ModelPricing{Input: 2, Output: 10, CacheRead: 0.2, CacheWrite: 3}, Thinking: "medium", AuthMode: "environment", Workspace: workspace, Concurrency: 2, ApprovalMode: "risky"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIssueTitleLengthLimit(t *testing.T) {
	s := configuredStore(t)
	validTitle := strings.Repeat("任", IssueTitleMaxLength)
	issue, err := s.CreateIssue(CreateIssueInput{Title: validTitle, Objective: "Complete the task.", Priority: "medium", WorkMode: "guided"})
	if err != nil {
		t.Fatalf("create title at limit: %v", err)
	}
	tooLong := validTitle + "务"
	if _, err = s.CreateIssue(CreateIssueInput{Title: tooLong, Objective: "Complete the task.", Priority: "medium", WorkMode: "guided"}); err == nil {
		t.Fatal("create should reject an Issue title over the limit")
	}
	if _, err = s.UpdateIssue(issue.ID, UpdateIssueInput{Title: &tooLong}); err == nil {
		t.Fatal("update should reject an Issue title over the limit")
	}
	if _, err = s.GetIssue(issue.Identifier); err == nil {
		t.Fatal("Issue lookup must use the immutable Aegis ID")
	}
}

func TestBoardVocabularyNormalizesPriorityAndRequiresOwnerForInProgress(t *testing.T) {
	s := configuredStore(t)
	issue, err := s.CreateIssue(CreateIssueInput{Title: "Queued work", Priority: "medium", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Priority != "middle" || issue.Status != "todo" || issue.AssigneeAgentID != "" {
		t.Fatalf("normalized Issue=%+v", issue)
	}
	inProgress := "in_progress"
	if _, err = s.UpdateIssue(issue.ID, UpdateIssueInput{Status: &inProgress}); err == nil || !strings.Contains(err.Error(), "必须有负责人") {
		t.Fatalf("unassigned in_progress error=%v", err)
	}
	high, err := s.CreateIssue(CreateIssueInput{Title: "Legacy urgent work", Priority: "critical", WorkMode: "autonomous"})
	if err != nil || high.Priority != "high" {
		t.Fatalf("legacy high Issue=%+v err=%v", high, err)
	}
}

func TestIssueHierarchyRelationsAndCheckout(t *testing.T) {
	s := configuredStore(t)
	parent, err := s.CreateIssue(CreateIssueInput{Title: "Build product", Objective: "Deliver the complete product.", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "aegis-orchestrator"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Backend", Objective: "Deliver the backend.", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Frontend", Objective: "Deliver the frontend.", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := s.AddRelation(a.ID, CreateRelationInput{RelatedIssueID: b.ID, Type: "blocks"})
	if err != nil {
		t.Fatal(err)
	}
	checked, err := s.CheckoutIssue(b.ID, CheckoutIssueInput{AgentID: "frontend-engineer", ExecutionID: "exec-b", ExpectedStatuses: []string{"todo"}})
	if err != nil {
		t.Fatalf("informational dependency must not block checkout: %v", err)
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
		Title: "Implement a focused API", Objective: "The API works and is tested.", Priority: "high", WorkMode: "autonomous",
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
	if slices.Contains(custom.Tools, "aegis_create_subissues") || slices.Contains(custom.Tools, "aegis_wait_for_child_issues") {
		t.Fatalf("custom Agent retained legacy coordination tools: %v", custom.Tools)
	}
	if !slices.Contains(custom.Tools, "aegis_publish_attachment") {
		t.Fatalf("custom agent tools=%v, missing attachment capability", custom.Tools)
	}
	if !slices.Contains(custom.Tools, "aegis_submit_final_result") {
		t.Fatalf("custom agent tools=%v, missing final-result capability", custom.Tools)
	}
	if !slices.Contains(custom.Tools, "aegis_report_progress") {
		t.Fatalf("custom agent tools=%v, missing progress capability", custom.Tools)
	}
	if !slices.Contains(custom.Tools, "aegis_board") || !slices.Contains(custom.Tools, "aegis_relay") {
		t.Fatalf("custom agent tools=%v, missing office app capabilities", custom.Tools)
	}

	custom.Tools = []string{"read"}
	updated, err := s.UpdateAgent(custom.ID, SaveAgentInput{
		Name: custom.Name, Enabled: true, SystemPrompt: custom.SystemPrompt,
		Tools: custom.Tools, Permissions: custom.Permissions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(updated.Tools, "aegis_create_subissues") || slices.Contains(updated.Tools, "aegis_wait_for_child_issues") {
		t.Fatalf("updated Agent retained legacy coordination tools: %v", updated.Tools)
	}
	if !slices.Contains(updated.Tools, "aegis_publish_attachment") {
		t.Fatalf("updated agent tools=%v, attachment capability was removed", updated.Tools)
	}
	if !slices.Contains(updated.Tools, "aegis_submit_final_result") {
		t.Fatalf("updated agent tools=%v, final-result capability was removed", updated.Tools)
	}
	if !slices.Contains(updated.Tools, "aegis_report_progress") {
		t.Fatalf("updated agent tools=%v, progress capability was removed", updated.Tools)
	}
	if !slices.Contains(updated.Tools, "aegis_board") || !slices.Contains(updated.Tools, "aegis_relay") {
		t.Fatalf("updated agent tools=%v, office app capabilities were removed", updated.Tools)
	}
}

func TestRequiredAgentToolsMigrationUpdatesExistingWorkers(t *testing.T) {
	s := configuredStore(t)
	index, exists := agentIndex(s.agents, "backend-engineer")
	if !exists {
		t.Fatal("backend-engineer is missing")
	}
	legacy := cloneAgent(s.agents[index])
	legacy.Tools = stringsWithout(legacy.Tools, "aegis_submit_final_result")
	s.agents[index] = legacy
	if err := s.db.Save(&agentRecord{ID: legacy.ID, Definition: legacy, CreatedAt: legacy.CreatedAt, UpdatedAt: legacy.UpdatedAt}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Delete(&registrySeedMigrationRecord{}, "id = ?", requiredAgentToolsMigrationID).Error; err != nil {
		t.Fatal(err)
	}

	if err := s.applyRequiredAgentToolsMigration(time.Now()); err != nil {
		t.Fatal(err)
	}
	updated, err := s.GetAgent(legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(updated.Tools, "aegis_submit_final_result") {
		t.Fatalf("migrated Agent tools=%v", updated.Tools)
	}
	var record agentRecord
	if err := s.db.First(&record, "id = ?", legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(record.Definition.Tools, "aegis_submit_final_result") {
		t.Fatalf("persisted Agent tools=%v", record.Definition.Tools)
	}
}

func TestSessionDetailIncludesPromptSnapshots(t *testing.T) {
	s := configuredStore(t)
	issue, err := s.CreateIssue(CreateIssueInput{
		Title: "Prompt snapshot task", Objective: "Capture prompt snapshots.", Priority: "medium", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.updateExecution(execution.ID, map[string]any{
		"initial_prompt": "Complete **this task**.",
		"system_prompt":  "You are a backend engineer.",
		"tools_snapshot": snapshotTools([]string{"read", "aegis_create_subissues"}),
	}); err != nil {
		t.Fatal(err)
	}

	detail, err := s.GetSession(execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Session.Execution.InitialPrompt != "Complete **this task**." {
		t.Fatalf("initial prompt=%q", detail.Session.Execution.InitialPrompt)
	}
	if detail.Session.Execution.SystemPrompt != "You are a backend engineer." {
		t.Fatalf("system prompt=%q", detail.Session.Execution.SystemPrompt)
	}
	if len(detail.Session.Execution.ToolsSnapshot) != 2 {
		t.Fatalf("tools snapshot=%+v", detail.Session.Execution.ToolsSnapshot)
	}
	if detail.Session.Execution.ToolsSnapshot[0].Name != "read" || len(detail.Session.Execution.ToolsSnapshot[0].Parameters) != 5 {
		t.Fatalf("read snapshot=%+v", detail.Session.Execution.ToolsSnapshot[0])
	}
	if purpose := detail.Session.Execution.ToolsSnapshot[0].Parameters[0]; purpose.Name != "description" || !purpose.Required {
		t.Fatalf("read purpose parameter=%+v", purpose)
	}
	children := detail.Session.Execution.ToolsSnapshot[1].Parameters[4].Children
	if len(children) != 5 || children[3].Name != "priority" || len(children[3].Enum) != 3 {
		t.Fatalf("nested tool parameters=%+v", children)
	}
}

func TestRelationCycleRejectedAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	_, err = s.SaveConfig(SaveConfigInput{Provider: "test", Model: "m", AuthMode: "environment", Workspace: workspace, Concurrency: 1, ApprovalMode: "none"})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.CreateIssue(CreateIssueInput{Title: "A", Objective: "Complete A.", Priority: "medium", WorkMode: "guided"})
	b, _ := s.CreateIssue(CreateIssueInput{Title: "B", Objective: "Complete B.", Priority: "medium", WorkMode: "guided"})
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
	ws := t.TempDir()
	_, err = s.SaveConfig(SaveConfigInput{Provider: "test", Model: "m", AuthMode: "environment", Workspace: ws, Concurrency: 1, ApprovalMode: "none"})
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
	if len(s.Agents()) < 5 {
		t.Fatal("default agents missing")
	}
	for _, id := range []string{"aegis-orchestrator", "backend-engineer", "frontend-engineer", "red-team-lead", "recon-engineer", "red-team-engineer", "vulnerability-report-engineer"} {
		a, err := s.GetAgent(id)
		if err != nil {
			t.Fatal(err)
		}
		if a.SystemPrompt == "" || len(a.Tools) == 0 || len(a.SkillIDs) == 0 {
			t.Fatalf("incomplete agent %s", id)
		}
	}
}

func TestExploreAgentIsSeededForReadOnlyCodeDiscovery(t *testing.T) {
	s := configuredStore(t)
	agent, err := s.GetAgent("explore")
	if err != nil {
		t.Fatal(err)
	}
	if !agent.Builtin || !agent.Enabled || agent.Category != "exploration" {
		t.Fatalf("unexpected explore Agent definition: %+v", agent)
	}
	if agent.Permissions.AllowNetwork || agent.Permissions.AllowShell || agent.Permissions.AllowWrite {
		t.Fatalf("explore Agent must remain read-only: %+v", agent.Permissions)
	}
	workspaceRef := workspaceCapabilityRef(agent.Permissions)
	if workspaceRef.Config["allowShell"] != false || workspaceRef.Config["allowWrite"] != false {
		t.Fatalf("explore workspace capability is not read-only: %+v", workspaceRef.Config)
	}
	for _, required := range []string{
		"code exploration specialist",
		"Trace relevant control flow and data flow end to end",
		"Stay read-only",
		"repository-relative file paths and line numbers",
		"implementation-ready handoff",
	} {
		if !strings.Contains(agent.SystemPrompt, required) {
			t.Fatalf("explore system prompt missing %q", required)
		}
	}
	for _, required := range []string{"read", "grep", "find", "ls", "aegis_submit_final_result"} {
		if !slices.Contains(agent.Tools, required) {
			t.Fatalf("explore Agent missing tool %s: %v", required, agent.Tools)
		}
	}
	for _, forbidden := range []string{"bash", "edit", "write"} {
		if slices.Contains(agent.Tools, forbidden) {
			t.Fatalf("explore Agent unexpectedly has mutating tool %s: %v", forbidden, agent.Tools)
		}
	}
}

func TestExploreAgentReceivesCodeDiscoveryIssuesWithoutStealingSecurityAudits(t *testing.T) {
	s := configuredStore(t)
	exploration, err := s.CreateIssue(CreateIssueInput{
		Title: "阅读项目并梳理调用链", Objective: "说明入口、核心调用链和实现改动的影响范围。", Priority: "medium", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	routed, err := s.chooseAgent(exploration)
	if err != nil {
		t.Fatal(err)
	}
	if routed.ID != "explore" {
		t.Fatalf("code discovery issue agent=%s, want explore", routed.ID)
	}

	securityAudit, err := s.CreateIssue(CreateIssueInput{
		Title: "审计代码中的认证安全漏洞", Objective: "识别并验证认证与权限边界问题。", Priority: "high", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	routed, err = s.chooseAgent(securityAudit)
	if err != nil {
		t.Fatal(err)
	}
	if routed.ID != "red-team-lead" {
		t.Fatalf("security audit issue agent=%s, want red-team-lead", routed.ID)
	}
}

func TestRegistryRestartDoesNotRewritePersistedDefinitions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var record agentRecord
	if err = store.db.First(&record, "id = ?", "backend-engineer").Error; err != nil {
		t.Fatal(err)
	}
	record.Definition.SystemPrompt = "Use the current operator-owned backend contract."
	record.Definition.Tools = []string{"read"}
	if err = store.db.Save(&record).Error; err != nil {
		t.Fatal(err)
	}
	database, _ := store.db.DB()
	_ = database.Close()

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := reopened.GetAgent("backend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	if agent.SystemPrompt != record.Definition.SystemPrompt || !slices.Equal(agent.Tools, record.Definition.Tools) {
		t.Fatalf("restart rewrote persisted Agent definition: %+v", agent)
	}
}

func TestAgentMemoPersistsAndRequiredToolsAreAvailable(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdateAgentMemo("backend-engineer", "- 用户偏好简洁的变更说明。\n- 提交前总是运行 Go 测试。")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updated.Content, "用户偏好") {
		t.Fatalf("memo was not updated: %+v", updated)
	}
	agent, err := s.GetAgent("backend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"aegis_get_memo", "aegis_update_memo"} {
		if !slices.Contains(agent.Tools, tool) {
			t.Fatalf("agent missing required memo tool %s: %v", tool, agent.Tools)
		}
	}
	database, _ := s.db.DB()
	_ = database.Close()

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.AgentMemo("backend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != updated.Content {
		t.Fatalf("memo did not persist: got=%q want=%q", got.Content, updated.Content)
	}
}

func TestAgentMemoIsIncludedInSystemPrompt(t *testing.T) {
	base := "Complete the Issue."
	withMemo := agentMemoSystemPrompt(base, "Always include test evidence.")
	if !strings.Contains(withMemo, base) || !strings.Contains(withMemo, "<agent_memo>") || !strings.Contains(withMemo, "Always include test evidence.") {
		t.Fatalf("memo was not included in system prompt: %s", withMemo)
	}
	if got := agentMemoSystemPrompt(base, "  "); got != base {
		t.Fatalf("empty memo changed system prompt: %q", got)
	}
}

func TestPermissionBoundaryIsIncludedInSystemPrompt(t *testing.T) {
	prompt := agentPermissionSystemPrompt("You are a backend engineer.", "/tmp/authorized-workspace", PermissionBoundary{
		WorkspaceScope: "run_workspace", AllowShell: true, AllowNetwork: false, AllowWrite: false,
	})
	for _, required := range []string{
		"<agent_permission_boundary>",
		"/tmp/authorized-workspace",
		"not a filesystem access boundary",
		"Absolute paths and \"..\" traversal are allowed",
		"Network access: false",
		"Workspace write access: false",
		"default working directory does not restrict file paths",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("permission prompt missing %q: %s", required, prompt)
		}
	}
}

func TestExecutionEfficiencyIsIncludedInEmployeeSystemPrompt(t *testing.T) {
	prompt := agentExecutionEfficiencySystemPrompt("You are a red-team lead.")
	for _, required := range []string{
		"<execution_efficiency>",
		"Start useful work immediately",
		"delegate parallel work",
		"Limited deliberate redundancy is allowed",
		"exchange findings through Relay",
		"never trade away correctness",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("execution efficiency prompt missing %q: %s", required, prompt)
		}
	}
}

func TestAgentOrganizationContextIncludesRosterLeadersAndCapacity(t *testing.T) {
	prompt := agentOrganizationContextSystemPrompt("base", []AgentDefinition{
		{ID: "red-team-lead", Name: "红队负责人", Category: "security", Description: "Own broad security goals", Enabled: true, SkillIDs: []string{"decompose-issues"}},
		{ID: "backend-engineer", Name: "后端工程师", Category: "backend", Description: "Implement bounded backend work", Enabled: true},
		{ID: "disabled", Name: "Disabled", Category: "general", Enabled: false},
	}, Config{Concurrency: 7}, 3)
	for _, expected := range []string{"red-team-lead", "红队负责人", `"decompositionLeader":true`, "backend-engineer", "at most 7", "3 were active", "high-priority Issues are claimed before middle, then low", "Phone Board"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("organization prompt missing %q:\n%s", expected, prompt)
		}
	}
	if strings.Contains(prompt, `"agentId":"disabled"`) {
		t.Fatalf("disabled Agent leaked into roster:\n%s", prompt)
	}
}

func TestGoalPreservingDecompositionPromptTargetsOnlyCapableLeaders(t *testing.T) {
	cfg := Config{
		MaxIssueDepth: 6, MaxChildrenPerRequest: 9, MaxDirectChildren: 18,
		IssueBudget: IssueBudgetConfig{MaxTurns: 73, ActiveTimeMinutes: 17, SummaryTurns: 8, SummaryTimeMinutes: 2},
	}
	leader := AgentDefinition{ID: "custom-leader", SkillIDs: []string{"decompose-issues"}}
	prompt := agentGoalPreservingDecompositionSystemPrompt("base", leader, cfg)
	for _, required := range []string{
		"<goal_preserving_decomposition>",
		"exactly one top-level goal",
		"do not create a redundant child that merely restates that goal",
		"one distinct direct child Issue for every goal",
		"never combine two or more goals into one Issue",
		"73 model turns and 17 active minutes",
		"current request limit is 9 children",
		"direct-child limit is 18",
		"hierarchy depth limit is 6",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("leader decomposition prompt is missing %q: %s", required, prompt)
		}
	}

	worker := AgentDefinition{ID: "backend-engineer", SkillIDs: []string{"go-service-engineering"}}
	if got := agentGoalPreservingDecompositionSystemPrompt("base", worker, cfg); got != "base" {
		t.Fatalf("ordinary worker unexpectedly received Leader decomposition policy: %s", got)
	}
}

func TestSecurityOutcomeGradeContractOnlyAppliesToSecurityAgents(t *testing.T) {
	base := "You are a security engineer."
	prompt := agentSecurityOutcomeGradeSystemPrompt(base, "security")
	for _, required := range []string{
		"<security_outcome_grade_contract>",
		"成果等级必须遵循任务 Goal",
		"S：已取得 root / SYSTEM / Administrator",
		"A：已证明稳定任意系统命令执行",
		"B：已证明目标主机、容器、应用进程或 worker 中存在系统级执行 foothold",
		"C：高价值非系统执行型成果",
		"D：高可信漏洞链路",
		"WebShell 只有在已证明稳定任意系统命令执行时才算 A",
		"不能用高等级名称掩盖稳定性或权限缺口",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("security outcome grade prompt missing %q: %s", required, prompt)
		}
	}
	if got := agentSecurityOutcomeGradeSystemPrompt(base, "backend"); got != base {
		t.Fatalf("non-security agent unexpectedly received outcome grade contract: %s", got)
	}
}

func TestLegacySystemContextIsRemovedFromVisibleUserPrompt(t *testing.T) {
	original := "帮我对目标进行红队测试，创建一个 Issue"
	for _, injected := range []string{
		"\n\n## Organization delegation boundary\nAvailable direct reports: ...",
		"\n\n<organization_delegation_boundary>\nAvailable direct reports: ...\n</organization_delegation_boundary>",
		"\n\n<agent_permission_boundary>\nWorkspace boundary: ...\n</agent_permission_boundary>",
		"\n\n<agent_memo>\nRemember prior work.\n</agent_memo>",
	} {
		if got := stripLegacyUserPromptContext(original + injected); got != original {
			t.Fatalf("legacy context was not removed: got=%q", got)
		}
	}
	untouched := "  用户主动输入 <agent_memo> 作为普通文本  "
	if got := stripLegacyUserPromptContext(untouched); got != untouched {
		t.Fatalf("ordinary user text was changed: got=%q", got)
	}
}

func TestVulnerabilityReportAgentUsesRequiredFormatAndReceivesReportIssues(t *testing.T) {
	s := configuredStore(t)
	agent, err := s.GetAgent("vulnerability-report-engineer")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"FORMAT CONTRACT", "FIXED WORKFLOW", "DEFAULT REPORT TEMPLATE", "## 5. 漏洞详情", "Explicit user format overrides"} {
		if !strings.Contains(agent.SystemPrompt, required) {
			t.Fatalf("report system prompt missing %q", required)
		}
	}
	if !agent.Permissions.AllowNetwork || !agent.Permissions.AllowWrite || agent.Permissions.ApprovalMode != "none" || agent.Permissions.ReworkApprovalMode != "none" {
		t.Fatalf("unexpected report permissions: %+v", agent.Permissions)
	}
	issue, err := s.CreateIssue(CreateIssueInput{
		Title: "按照客户模板编写漏洞评估报告", Objective: "交付符合客户模板的完整漏洞评估报告。", Priority: "high", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	routed, err := s.chooseAgent(issue)
	if err != nil {
		t.Fatal(err)
	}
	if routed.ID != agent.ID {
		t.Fatalf("report issue agent=%s, want %s", routed.ID, agent.ID)
	}
}

func TestReconAgentUsesFixedWorkflowAndReceivesReconIssues(t *testing.T) {
	s := configuredStore(t)
	agent, err := s.GetAgent("recon-engineer")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"FIXED WORKFLOW", "REQUIRED OUTPUT TEMPLATE", "## 8. 信息点完成矩阵", "## 9. 红队后续建议"} {
		if !strings.Contains(agent.SystemPrompt, required) {
			t.Fatalf("recon system prompt missing %q", required)
		}
	}
	if !agent.Permissions.AllowNetwork || !agent.Permissions.AllowWrite || agent.Permissions.ApprovalMode != "none" || agent.Permissions.ReworkApprovalMode != "none" {
		t.Fatalf("unexpected recon permissions: %+v", agent.Permissions)
	}
	issue, err := s.CreateIssue(CreateIssueInput{
		Title: "收集目标子域名和 Web 技术指纹", Objective: "交付有证据的子域名和 Web 技术指纹清单。", Priority: "high", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	routed, err := s.chooseAgent(issue)
	if err != nil {
		t.Fatal(err)
	}
	if routed.ID != agent.ID {
		t.Fatalf("recon issue agent=%s, want %s", routed.ID, agent.ID)
	}
}

func TestSecurityAgentRoutingUsesLeadForTopLevelAndWorkerForChildren(t *testing.T) {
	s := configuredStore(t)
	parent, err := s.CreateIssue(CreateIssueInput{
		Title: "评估目标的安全风险", Objective: "完成目标安全风险评估并提供证据。", Priority: "high", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	lead, err := s.chooseAgent(parent)
	if err != nil {
		t.Fatal(err)
	}
	if lead.ID != "red-team-lead" {
		t.Fatalf("top-level security task agent=%s, want red-team-lead", lead.ID)
	}
	if !lead.Permissions.AllowNetwork || !lead.Permissions.AllowWrite || lead.Permissions.ApprovalMode != "none" || lead.Permissions.ReworkApprovalMode != "none" {
		t.Fatalf("unexpected red-team lead permissions: %+v", lead.Permissions)
	}
	if slices.Contains(lead.Tools, "aegis_create_subissues") || slices.Contains(lead.Tools, "aegis_wait_for_child_issues") {
		t.Fatalf("red-team lead retained legacy coordination tools: %v", lead.Tools)
	}

	child, err := s.CreateIssue(CreateIssueInput{
		ParentID: parent.ID, Title: "验证认证与越权漏洞", Objective: "验证认证与越权风险并提供证据。", Priority: "high", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := s.chooseAgent(child)
	if err != nil {
		t.Fatal(err)
	}
	if worker.ID != "red-team-engineer" {
		t.Fatalf("child security issue agent=%s, want red-team-engineer", worker.ID)
	}
}

func TestRedTeamLeadHasMandatoryComplexTaskBoundary(t *testing.T) {
	s := configuredStore(t)
	lead, err := s.GetAgent("red-team-lead")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"COMPLEX TASK BOUNDARY (mandatory)",
		"two or more distinct security objectives",
		"requires reconnaissance or information collection",
		"requires a formal vulnerability",
		"covers two or more vulnerability classes",
		"you MUST call the Phone Board shortcut phone_board_delegate",
	} {
		if !strings.Contains(lead.SystemPrompt, required) {
			t.Fatalf("red-team lead prompt is missing %q", required)
		}
	}
}

func TestRedTeamAgentsReceiveAgentBrowserSkill(t *testing.T) {
	store := configuredStore(t)
	if _, exists := skillIndex(store.Skills(), agentBrowserSkillID); !exists {
		t.Fatalf("agent-browser skill %s was not seeded", agentBrowserSkillID)
	}
	for _, agentID := range []string{"red-team-lead", "recon-engineer", "red-team-engineer"} {
		agent, err := store.GetAgent(agentID)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(agent.SkillIDs, agentBrowserSkillID) {
			t.Fatalf("agent %s does not have %s: %v", agentID, agentBrowserSkillID, agent.SkillIDs)
		}
	}
}
