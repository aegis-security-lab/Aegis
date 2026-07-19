package control

import (
	"strings"
	"testing"
	"time"
)

func TestIssueObjectiveIsOptionalAndPersistedWhenProvided(t *testing.T) {
	store := configuredStore(t)
	withoutObjective, err := store.CreateIssue(CreateIssueInput{
		Title: "No acceptance objective", Priority: "medium", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	if withoutObjective.Objective != "" {
		t.Fatalf("objective=%q, want empty", withoutObjective.Objective)
	}
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Objective task", Objective: "The API returns 200 and its integration test passes.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Objective != "The API returns 200 and its integration test passes." {
		t.Fatalf("objective=%q", issue.Objective)
	}
	if issue.ValidationMode != "fixed" || issue.MaxValidationAttempts != 3 {
		t.Fatalf("validation policy was not snapshotted: %+v", issue)
	}
}

func TestIssueWithoutObjectiveCompletesWithoutValidation(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Summarize the workspace", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CheckoutIssue(issue.ID, CheckoutIssueInput{
		AgentID: execution.AgentID, ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.db.Create(&Message{
		ID: nextID("message"), ExecutionID: execution.ID, IssueID: issue.ID, Role: "assistant",
		Content: "Workspace summary is complete.", CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.handleSettled(&PiSession{executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID, kind: "work"})

	completed, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "done" || completed.ExecutionPhase != "completed" || completed.Result != "Workspace summary is complete." {
		t.Fatalf("Issue without objective was not completed directly: %+v", completed)
	}
	var validationCount int64
	store.db.Model(&IssueValidation{}).Where("issue_id = ?", issue.ID).Count(&validationCount)
	if validationCount != 0 {
		t.Fatalf("validations=%d, want 0", validationCount)
	}
	var event ExecutionEvent
	if err = store.db.Where("execution_id = ? AND title = ?", execution.ID, "无需目标验收").First(&event).Error; err != nil {
		t.Fatalf("missing no-objective completion event: %v", err)
	}
}

func TestAcceptanceValidatorIsInternalAndReadOnly(t *testing.T) {
	store := configuredStore(t)
	agent, err := store.GetAgent("acceptance-validator")
	if err != nil {
		t.Fatal(err)
	}
	if !agent.Internal || !agent.Enabled || len(agent.Tools) != 0 || agent.Permissions.AllowShell || agent.Permissions.AllowNetwork || agent.Permissions.AllowWrite {
		t.Fatalf("acceptance validator is not isolated: %+v", agent)
	}
	if !strings.Contains(agent.SystemPrompt, "validation attachment tools") || !strings.Contains(agent.SystemPrompt, "never require the Worker to duplicate") {
		t.Fatalf("acceptance validator prompt does not explain attachment evidence: %s", agent.SystemPrompt)
	}
}

func TestValidationPromptProvidesAttachmentManifestWithoutInliningContent(t *testing.T) {
	issue := Issue{Identifier: "AEG-0042", Title: "Security report", Description: "Produce the report.", Objective: "A complete security report is attached."}
	prompt := validationPrompt(issue, "Report published as an attachment.", 2, []ValidationAttachmentInfo{{
		ID: "attachment-1", Name: "security-report.md", MimeType: "text/markdown", Size: 42000,
		Description: "Complete report", DownloadURL: "http://127.0.0.1:8080/api/attachments/attachment-1", Readable: true,
	}}, "fixed", 3, false)
	for _, expected := range []string{"attachment-1", "security-report.md", "http://127.0.0.1:8080/api/attachments/attachment-1", "aegis_read_validation_attachment", "do not require the Worker to duplicate"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("validation prompt missing %q: %s", expected, prompt)
		}
	}
	if strings.Contains(prompt, "full report body should not be inlined") {
		t.Fatal("validation prompt unexpectedly contains attachment body")
	}
	terminal := validationPrompt(issue, "Still incomplete.", 4, nil, "fixed", 3, true)
	for _, expected := range []string{"terminal validation", `"abandoned"`, "impossibility proof", "prior validation conversation"} {
		if !strings.Contains(terminal, expected) {
			t.Fatalf("terminal validation prompt missing %q: %s", expected, terminal)
		}
	}
}

func TestMergedNoProxyAlwaysBypassesLocalControlPlane(t *testing.T) {
	value := mergedNoProxy("example.internal,localhost", "another.internal")
	for _, expected := range []string{"example.internal", "another.internal", "127.0.0.1", "localhost", "::1"} {
		if !strings.Contains(value, expected) {
			t.Fatalf("NO_PROXY=%q does not contain %q", value, expected)
		}
	}
	if strings.Count(value, "localhost") != 1 {
		t.Fatalf("NO_PROXY contains duplicate localhost: %q", value)
	}
}

func TestParseValidationDecision(t *testing.T) {
	decision, err := parseValidationDecision("response: {\"outcome\":\"retry\",\"summary\":\"Tests are missing.\",\"feedback\":\"Run and report the integration test.\",\"impossibilityProof\":\"\"}")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Outcome != "retry" || decision.Summary != "Tests are missing." || decision.Feedback == "" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if _, err = parseValidationDecision("{\"outcome\":\"retry\",\"summary\":\"Incomplete\",\"feedback\":\"\"}"); err == nil {
		t.Fatal("failed validation without actionable feedback should be rejected")
	}
	if _, err = parseValidationDecision("{\"outcome\":\"abandoned\",\"summary\":\"Impossible\",\"impossibilityProof\":\"\"}"); err == nil {
		t.Fatal("abandoned validation without proof should be rejected")
	}
}

func TestValidationPassCompletesIssueAndFailurePreparesSameAgentRetry(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Validated work", Objective: "The implementation and tests are complete.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "validating",
		"current_execution_id": source.ID, "checkout_execution_id": "",
	}).Error; err != nil {
		t.Fatal(err)
	}
	issue, _ = store.GetIssue(issue.ID)
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	validationExecution, _, err := store.createInternalExecution(issue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	validation := IssueValidation{
		ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: source.ID,
		ValidationExecutionID: validationExecution.ID, Attempt: 1,
		Objective: issue.Objective, CandidateResult: "Implemented the feature; integration tests pass.", Status: "running", CreatedAt: time.Now(),
	}
	if err = store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}

	checkedOut, retry, agent, prompt, err := manager.prepareValidationRetry(issue, validation, validationDecision{
		Summary: "The output does not name the test command.", Feedback: "Run the integration suite and report its command and result.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if retry.Kind != "rework" || agent.ID != issue.AssigneeAgentID || checkedOut.CheckoutExecutionID != retry.ID {
		t.Fatalf("retry did not return to the original owner: issue=%+v execution=%+v agent=%+v", checkedOut, retry, agent)
	}
	if !strings.Contains(prompt, issue.Objective) || !strings.Contains(prompt, "Run the integration suite") {
		t.Fatalf("retry prompt is missing objective or feedback: %s", prompt)
	}
	detail, err := store.GetIssueDetail(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Comments) != 1 || detail.Comments[0].AuthorID != "acceptance-validator" {
		t.Fatalf("validation failure comment was not recorded: %+v", detail.Comments)
	}

	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"execution_phase": "validating", "checkout_execution_id": "",
	}).Error; err != nil {
		t.Fatal(err)
	}
	issue, _ = store.GetIssue(issue.ID)
	manager.completeValidatedIssue(issue, validation, validationDecision{Outcome: "passed", Summary: "All objective evidence is present."}, time.Now())
	completed, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "done" || completed.ExecutionPhase != "completed" || completed.Result != validation.CandidateResult {
		t.Fatalf("validated Issue was not completed: %+v", completed)
	}
}

func TestFixedValidationExecutionIsReusedAndAbandonmentIsVisible(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Fixed validator", Objective: "Produce a result that depends on an unavailable external system.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	first, _, err := manager.fixedValidationExecution(issue)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Update("validation_execution_id", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	issue, _ = store.GetIssue(issue.ID)
	second, _, err := manager.fixedValidationExecution(issue)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.SessionID != second.SessionID {
		t.Fatalf("validator session was not reused: first=%+v second=%+v", first, second)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		record := IssueValidation{
			ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: "source",
			ValidationExecutionID: first.ID, Attempt: attempt, Objective: issue.Objective,
			CandidateResult: "candidate", Status: "failed", CreatedAt: time.Now(),
		}
		if err = store.db.Create(&record).Error; err != nil {
			t.Fatalf("multiple attempts must share one validation execution: %v", err)
		}
	}

	validation := IssueValidation{ValidationExecutionID: first.ID, CandidateResult: "External dependency is unavailable."}
	manager.abandonValidatedObjective(issue, validation, validationDecision{
		Outcome: "abandoned", Summary: "The target cannot be reached under the fixed constraints.",
		ImpossibilityProof: "The required service has been permanently removed and the task forbids replacement or scope changes.",
	}, time.Now())
	abandoned, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if abandoned.Status != "cancelled" || !abandoned.ObjectiveAbandoned || abandoned.AbandonedAt == nil || abandoned.AbandonmentReason == "" {
		t.Fatalf("abandoned objective is not persisted: %+v", abandoned)
	}
}
