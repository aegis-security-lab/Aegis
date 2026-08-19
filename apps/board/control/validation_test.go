package control

import (
	"strings"
	"testing"
	"time"
)

func TestIssueObjectiveIsOptionalAndPersistedWhenProvided(t *testing.T) {
	store := configuredStore(t)
	withoutObjective, err := store.CreateIssue(CreateIssueInput{
		Title: "No acceptance objective", Priority: "middle", WorkMode: "autonomous",
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
		Title: "Summarize the workspace", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
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

func TestAcceptanceValidatorUsesWorkspaceEvidenceAndStructuredDecisions(t *testing.T) {
	store := configuredStore(t)
	agent, err := store.GetAgent("acceptance-validator")
	if err != nil {
		t.Fatal(err)
	}
	if !agent.Internal || !agent.Enabled || len(agent.Tools) != 0 || !agent.Permissions.AllowShell || agent.Permissions.AllowNetwork || !agent.Permissions.AllowWrite || agent.Permissions.WorkspaceScope != "run_workspace" {
		t.Fatalf("acceptance validator does not have the expected workspace boundary: %+v", agent)
	}
	if !strings.Contains(agent.SystemPrompt, "ordinary read, search, and shell tools") || !strings.Contains(agent.SystemPrompt, "complete final delivery") || !strings.Contains(agent.SystemPrompt, "never pass by combining an earlier report") {
		t.Fatalf("acceptance validator prompt does not explain attachment evidence: %s", agent.SystemPrompt)
	}
}

func TestValidationPromptProvidesAttachmentManifestWithoutInliningContent(t *testing.T) {
	issue := Issue{Identifier: "AEG-0042", Title: "Security report", Description: "Produce the report.", Objective: "A complete security report is attached."}
	prompt := validationPrompt(issue, "Report published as an attachment.", 2, []ValidationAttachmentInfo{{
		ID: "attachment-1", Name: "security-report.md", MimeType: "text/markdown", Size: 42000,
		Description: "Complete report", Path: "/workspace/.aegis/validation-evidence/execution-1/attachment-1/security-report.md",
	}}, "fixed", 3, false)
	for _, expected := range []string{"## Submission message", "## Published attachments", "attachment-1", "security-report.md", "/workspace/.aegis/validation-evidence/execution-1/attachment-1/security-report.md", "ordinary read", "user-facing ZIP", "newly consolidated complete replacement package"} {
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

	manager.scheduleMu.Lock()
	manager.continueAfterValidationFailure(issue, validation, validationDecision{
		Summary: "The output does not name the test command.", Feedback: "Run the integration suite and report its command and result.",
	})
	detail, err := store.GetIssueDetail(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Comments) != 1 || detail.Comments[0].AuthorID != "acceptance-validator" || detail.Comments[0].Type != "validation_feedback" || !strings.Contains(detail.Comments[0].Body, "Run the integration suite") {
		t.Fatalf("validation failure comment was not recorded: %+v", detail.Comments)
	}
	var wakeup AgentWakeup
	if err = store.db.First(&wakeup, "comment_id = ?", detail.Comments[0].ID).Error; err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	if wakeup.AgentID != issue.AssigneeAgentID || wakeup.Reason != "validation_feedback" || wakeup.Status != "queued" {
		manager.scheduleMu.Unlock()
		t.Fatalf("validation feedback did not queue the original owner: %+v", wakeup)
	}
	store.db.Delete(&AgentWakeup{}, "id = ?", wakeup.ID)
	manager.scheduleMu.Unlock()

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

func TestSubmitValidationDecisionPersistsStructuredResult(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Structured validation", Objective: "Provide evidence.", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, _, err := store.createInternalExecution(issue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	record := IssueValidation{ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: "source", ValidationExecutionID: execution.ID, Attempt: 1, Objective: issue.Objective, Status: "running", CreatedAt: time.Now()}
	if err = store.db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions[execution.ID] = &PiSession{executionID: execution.ID, issueID: issue.ID, kind: "validation", controlToken: "validation-token"}
	input := SubmitValidationDecisionInput{Outcome: "retry", Summary: "Evidence is incomplete.", Feedback: "Publish the test output."}
	if _, err = manager.SubmitValidationDecision(execution.ID, "validation-token", input); err != nil {
		t.Fatal(err)
	}
	if err = store.db.First(&record, "id = ?", record.ID).Error; err != nil {
		t.Fatal(err)
	}
	decision, err := submittedValidationDecision(record)
	if err != nil || decision.Outcome != "retry" || decision.Feedback != input.Feedback {
		t.Fatalf("structured decision was not persisted: decision=%+v err=%v", decision, err)
	}
	if _, err = manager.SubmitValidationDecision(execution.ID, "wrong-token", input); err == nil {
		t.Fatal("wrong control token accepted")
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

func TestWorkerDeliveryCommentAutomaticallyQueuesAcceptanceAgent(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{Title: "Delivery", Objective: "Verified output", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	source, _ := store.createExecution(issue, "backend-engineer", "work")
	_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "active", "checkout_execution_id": source.ID}).Error
	_ = store.updateExecution(source.ID, map[string]any{"status": "completed", "result": "Implemented and tested."})
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.scheduleMu.Lock()
	if err := manager.publishDeliveryForValidation(issue, source, source.AgentID, "Implemented and tested."); err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	var comment IssueComment
	if err := store.db.First(&comment, "issue_id = ? AND type = ?", issue.ID, "delivery").Error; err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	var wakeup AgentWakeup
	if err := store.db.First(&wakeup, "comment_id = ?", comment.ID).Error; err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	updated, _ := store.GetIssue(issue.ID)
	if comment.ExecutionID != source.ID || wakeup.AgentID != "acceptance-validator" || wakeup.Reason != "delivery_validation" || updated.ExecutionPhase != "awaiting_validation" {
		manager.scheduleMu.Unlock()
		t.Fatalf("delivery workflow mismatch: comment=%+v wakeup=%+v issue=%+v", comment, wakeup, updated)
	}
	store.db.Delete(&AgentWakeup{}, "id = ?", wakeup.ID)
	manager.scheduleMu.Unlock()
}

func TestAcceptanceAgentCloseToolCreatesPassedCommentAndClosesIssue(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{Title: "Validated", Objective: "All checks pass", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	source, _ := store.createExecution(issue, "backend-engineer", "work")
	report := "# Final report\n\nAll checks pass.\n"
	if _, err := store.captureUploadedAttachment(issue, source.ID, PublishAttachmentInput{Path: "final-report.md", Description: "Complete final report"}, strings.NewReader(report), int64(len(report))); err != nil {
		t.Fatal(err)
	}
	validationExecution, _, _ := store.createInternalExecution(issue, "acceptance-validator", "validation")
	_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "validating", "result": "Checks pass", "current_execution_id": validationExecution.ID}).Error
	validation := IssueValidation{ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: source.ID, ValidationExecutionID: validationExecution.ID, Attempt: 1, Objective: issue.Objective, CandidateResult: "Checks pass", Status: "running", CreatedAt: time.Now()}
	if err := store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{manager: manager, executionID: validationExecution.ID, issueID: issue.ID, agentID: "acceptance-validator", kind: "validation", controlToken: "validator-secret"}
	manager.sessions[validationExecution.ID] = session
	if _, err := manager.CloseValidatedIssue(validationExecution.ID, "validator-secret", CloseValidatedIssueInput{Summary: "Objective and published evidence verified."}); err != nil {
		t.Fatal(err)
	}
	manager.handleValidationSettled(issue, session, "")
	completed, _ := store.GetIssue(issue.ID)
	if completed.Status != "done" {
		t.Fatalf("validated Issue was not closed: %+v", completed)
	}
	var comment IssueComment
	if err := store.db.First(&comment, "issue_id = ? AND type = ?", issue.ID, "validation_passed").Error; err != nil {
		t.Fatal(err)
	}
	if comment.AuthorID != "acceptance-validator" || !strings.Contains(comment.Body, "Objective and published evidence verified") {
		t.Fatalf("unexpected passed comment: %+v", comment)
	}
}

func TestAcceptanceAgentCannotPassLatestSubmissionWithoutAttachments(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{Title: "Missing package", Objective: "Deliver a complete report", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	source, _ := store.createExecution(issue, "backend-engineer", "work")
	validationExecution, _, _ := store.createInternalExecution(issue, "acceptance-validator", "validation")
	validation := IssueValidation{ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: source.ID, ValidationExecutionID: validationExecution.ID, Attempt: 1, Objective: issue.Objective, CandidateResult: "The report is described only in text.", Status: "running", CreatedAt: time.Now()}
	if err := store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	_, err := manager.closeValidatedIssue(validationExecution.ID, CloseValidatedIssueInput{Summary: "Pass without inspecting a package."})
	if err == nil || !strings.Contains(err.Error(), "当前最新提交没有附件") {
		t.Fatalf("expected latest-submission attachment gate, got %v", err)
	}
	var refreshed IssueValidation
	if err = store.db.First(&refreshed, "id = ?", validation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.DecisionJSON != "" || refreshed.Status != "running" {
		t.Fatalf("rejected pass must leave validation open for retry: %+v", refreshed)
	}
}
