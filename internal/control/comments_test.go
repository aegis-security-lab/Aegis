package control

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestOperatorCommentWakesIssueAssignee(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Review API behavior", Objective: "Verify the API response contract.", Priority: "medium", WorkMode: "guided", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.scheduleMu.Lock()
	comment, err := manager.AddIssueComment(issue.ID, "Please verify the response contract.")
	if err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	var wakeups []AgentWakeup
	if err = store.db.Where("comment_id = ?", comment.ID).Find(&wakeups).Error; err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	if len(wakeups) != 1 || wakeups[0].AgentID != "backend-engineer" || wakeups[0].Reason != "issue_comment_assignee" {
		manager.scheduleMu.Unlock()
		t.Fatalf("unexpected assignee wakeups: %+v", wakeups)
	}
	prompt, err := wakeupPrompt(issue, wakeups[0], store.Agents(), store.db)
	if err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	if !strings.Contains(prompt, comment.Body) {
		manager.scheduleMu.Unlock()
		t.Fatalf("wakeup prompt does not contain comment body: %s", prompt)
	}
	store.db.Delete(&AgentWakeup{}, "comment_id = ?", comment.ID)
	manager.scheduleMu.Unlock()
}

func TestCommentWakeupWithoutObjectiveDoesNotStartValidation(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Completed discussion task", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "done", "execution_phase": "completed", "completed_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	comment := IssueComment{ID: nextID("comment"), IssueID: issue.ID, AuthorType: "operator", AuthorID: "operator", Body: "Please explain the latest result.", CreatedAt: now}
	if err = store.db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	wakeup := AgentWakeup{
		ID: nextID("wakeup"), IssueID: issue.ID, CommentID: comment.ID, AgentID: execution.AgentID,
		ExecutionID: execution.ID, Reason: "issue_comment_assignee", Status: "delivered", CreatedAt: now, DeliveredAt: &now,
	}
	if err = store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&Message{
		ID: nextID("message"), ExecutionID: execution.ID, IssueID: issue.ID, Role: "assistant",
		Content: "The latest result is unchanged; here is the requested explanation.", CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.handleSettled(&PiSession{executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID, kind: "work"})

	if err = store.db.First(&wakeup, "id = ?", wakeup.ID).Error; err != nil {
		t.Fatal(err)
	}
	if wakeup.Status != "completed" {
		t.Fatalf("wakeup status=%s, want completed", wakeup.Status)
	}
	completed, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "done" || completed.ExecutionPhase != "completed" || completed.Error != "" {
		t.Fatalf("comment wakeup changed completed Issue state: %+v", completed)
	}
	var validationCount int64
	store.db.Model(&IssueValidation{}).Where("issue_id = ?", issue.ID).Count(&validationCount)
	if validationCount != 0 {
		t.Fatalf("validations=%d, want 0", validationCount)
	}
}

func TestCancelledIssueCommentWakeupSettlesWithoutChangingIssue(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Cancelled discussion", Objective: "Original objective", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "cancelled", "execution_phase": "completed", "cancelled_at": now,
		"objective_abandoned": true, "abandon_requested_at": now, "abandoned_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "wakeup")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	comment := IssueComment{ID: nextID("comment"), IssueID: issue.ID, AuthorType: "operator", AuthorID: "operator", Body: "Please summarize.", CreatedAt: now}
	if err = store.db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	wakeup := AgentWakeup{
		ID: nextID("wakeup"), IssueID: issue.ID, CommentID: comment.ID, AgentID: execution.AgentID, ExecutionID: execution.ID,
		Reason: "issue_comment_assignee", Status: "delivered", PriorIssueStatus: "cancelled", PriorExecutionPhase: "completed",
		CreatedAt: now, DeliveredAt: &now,
	}
	if err = store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": execution.ID, "current_execution_id": execution.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	result := "Summary response"
	if err = store.db.Create(&Message{ID: nextID("message"), ExecutionID: execution.ID, IssueID: issue.ID, Role: "assistant", Content: result, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	attachment := IssueAttachment{ID: nextID("attachment"), IssueID: issue.ID, ExecutionID: execution.ID, Name: "evidence.txt", SourcePath: "evidence.txt", StoragePath: "artifacts/evidence.txt", MimeType: "text/plain", Size: 8, CreatedAt: now}
	if err = store.db.Create(&attachment).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.handleSettled(&PiSession{executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID, kind: execution.Kind})
	if err = store.db.First(&execution, "id = ?", execution.ID).Error; err != nil || execution.Status != "completed" {
		t.Fatalf("execution was not completed: execution=%+v err=%v", execution, err)
	}
	if err = store.db.First(&wakeup, "id = ?", wakeup.ID).Error; err != nil || wakeup.Status != "completed" {
		t.Fatalf("wakeup was not completed: wakeup=%+v err=%v", wakeup, err)
	}
	unchanged, _ := store.GetIssue(issue.ID)
	if unchanged.Status != "cancelled" || unchanged.ExecutionPhase != "completed" {
		t.Fatalf("comment response changed cancelled Issue: %+v", unchanged)
	}
	var response IssueComment
	if err = store.db.Where("issue_id = ? AND execution_id = ? AND author_type = ?", issue.ID, execution.ID, "agent").First(&response).Error; err != nil {
		t.Fatal(err)
	}
	if response.Body != result {
		t.Fatalf("unexpected comment response: %+v", response)
	}
	if err = store.db.First(&attachment, "id = ?", attachment.ID).Error; err != nil || attachment.CommentID != response.ID {
		t.Fatalf("attachment was not bound to comment: attachment=%+v err=%v", attachment, err)
	}
}

func TestRestartRecoversCompletedCommentWakeupAndBindsAttachments(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{Title: "Recovered comment", Objective: "Original objective", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	execution, _ := store.createExecution(issue, "backend-engineer", "wakeup")
	now := time.Now()
	result := "Recovered response with attachment."
	if err := store.db.Model(&Execution{}).Where("id = ?", execution.ID).Updates(map[string]any{
		"status": "completed", "result": result, "finished_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	wakeup := AgentWakeup{
		ID: nextID("wakeup"), IssueID: issue.ID, AgentID: execution.AgentID, ExecutionID: execution.ID,
		Reason: "issue_comment_assignee", Status: "completed", PriorIssueStatus: "cancelled",
		PriorExecutionPhase: "completed", CreatedAt: now, DeliveredAt: &now, CompletedAt: &now,
	}
	if err := store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	existingResponse := IssueComment{ID: nextID("comment"), IssueID: issue.ID, AuthorType: "agent", AuthorID: execution.AgentID, ExecutionID: execution.ID, Body: result, CreatedAt: now}
	if err := store.db.Create(&existingResponse).Error; err != nil {
		t.Fatal(err)
	}
	attachment := IssueAttachment{ID: nextID("attachment"), IssueID: issue.ID, ExecutionID: execution.ID, Name: "proof.txt", SourcePath: "proof.txt", StoragePath: "artifacts/proof.txt", MimeType: "text/plain", Size: 5, CreatedAt: now}
	if err := store.db.Create(&attachment).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": execution.ID,
		"current_execution_id": execution.ID, "objective_abandoned": true,
		"abandon_requested_at": now, "abandoned_at": now, "cancelled_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	recovering, _ := reopened.GetIssue(issue.ID)
	if recovering.Status != "todo" || recovering.ExecutionPhase != "recovering" || recovering.RecoveryPhase != "settled" {
		t.Fatalf("completed wakeup was not queued for settled recovery: %+v", recovering)
	}
	manager := &Manager{store: reopened, sessions: map[string]*PiSession{}}
	if err = manager.resumeSettledIssueLocked(recovering); err != nil {
		t.Fatal(err)
	}
	finished, _ := reopened.GetIssue(issue.ID)
	if finished.Status != "cancelled" || finished.ExecutionPhase != "completed" || finished.CheckoutExecutionID != "" {
		t.Fatalf("recovered wakeup did not restore prior Issue state: %+v", finished)
	}
	if err = reopened.db.First(&wakeup, "id = ?", wakeup.ID).Error; err != nil || wakeup.Status != "completed" {
		t.Fatalf("recovered wakeup was not completed: wakeup=%+v err=%v", wakeup, err)
	}
	var response IssueComment
	if err = reopened.db.Where("issue_id = ? AND execution_id = ?", issue.ID, execution.ID).First(&response).Error; err != nil || response.Body != result {
		t.Fatalf("recovered response comment missing: response=%+v err=%v", response, err)
	}
	var responseCount int64
	if err = reopened.db.Model(&IssueComment{}).Where("issue_id = ? AND execution_id = ? AND author_type = ?", issue.ID, execution.ID, "agent").Count(&responseCount).Error; err != nil || responseCount != 1 {
		t.Fatalf("recovery duplicated existing response: count=%d err=%v", responseCount, err)
	}
	if err = reopened.db.First(&attachment, "id = ?", attachment.ID).Error; err != nil || attachment.CommentID != response.ID {
		t.Fatalf("recovered attachment was not bound: attachment=%+v err=%v", attachment, err)
	}
}

func TestCompletedWakeupChainRestoresOriginalTerminalIssue(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Chained discussion", Objective: "Original objective", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	firstExecution, err := store.createExecution(issue, "backend-engineer", "wakeup")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(firstExecution.ID, map[string]any{"status": "completed", "result": "First response", "finished_at": now}); err != nil {
		t.Fatal(err)
	}
	firstWakeup := AgentWakeup{
		ID: nextID("wakeup"), IssueID: issue.ID, AgentID: firstExecution.AgentID, ExecutionID: firstExecution.ID,
		Reason: "issue_comment_assignee", Status: "delivered", PriorIssueStatus: "cancelled", PriorExecutionPhase: "completed",
		CreatedAt: now.Add(-time.Minute), DeliveredAt: &now,
	}
	if err = store.db.Create(&firstWakeup).Error; err != nil {
		t.Fatal(err)
	}
	firstAttachment := IssueAttachment{ID: nextID("attachment"), IssueID: issue.ID, ExecutionID: firstExecution.ID, Name: "old-proof.txt", SourcePath: "old-proof.txt", StoragePath: "artifacts/old-proof.txt", MimeType: "text/plain", Size: 5, CreatedAt: now}
	if err = store.db.Create(&firstAttachment).Error; err != nil {
		t.Fatal(err)
	}

	secondExecution, err := store.createExecution(issue, "backend-engineer", "wakeup")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(secondExecution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	secondWakeup := AgentWakeup{
		ID: nextID("wakeup"), IssueID: issue.ID, AgentID: secondExecution.AgentID, ExecutionID: secondExecution.ID,
		Reason: "issue_comment_assignee", Status: "delivered", PriorIssueStatus: "in_progress", PriorExecutionPhase: "active", PriorExecutionID: firstExecution.ID,
		CreatedAt: now, DeliveredAt: &now,
	}
	if err = store.db.Create(&secondWakeup).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": secondExecution.ID, "current_execution_id": secondExecution.ID,
		"objective_abandoned": true, "abandon_requested_at": now, "abandoned_at": now, "cancelled_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&Message{ID: nextID("message"), ExecutionID: secondExecution.ID, IssueID: issue.ID, Role: "assistant", Content: "Second response", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.handleSettled(&PiSession{executionID: secondExecution.ID, issueID: issue.ID, agentID: secondExecution.AgentID, kind: "wakeup", wakeupID: secondWakeup.ID})

	finished, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "cancelled" || finished.ExecutionPhase != "completed" || finished.CheckoutExecutionID != "" {
		t.Fatalf("completed wakeup chain did not restore original terminal state: %+v", finished)
	}
	if err = store.db.First(&firstWakeup, "id = ?", firstWakeup.ID).Error; err != nil || firstWakeup.Status != "completed" {
		t.Fatalf("older delivered wakeup was not reconciled: wakeup=%+v err=%v", firstWakeup, err)
	}
	var firstResponse IssueComment
	if err = store.db.Where("issue_id = ? AND execution_id = ? AND author_type = ?", issue.ID, firstExecution.ID, "agent").First(&firstResponse).Error; err != nil || firstResponse.Body != "First response" {
		t.Fatalf("older completed wakeup response was not recovered: response=%+v err=%v", firstResponse, err)
	}
	if err = store.db.First(&firstAttachment, "id = ?", firstAttachment.ID).Error; err != nil || firstAttachment.CommentID != firstResponse.ID {
		t.Fatalf("older completed wakeup attachment was not bound: attachment=%+v err=%v", firstAttachment, err)
	}
}

func TestDispatchWakeupReusesLatestAgentSessionEvenWhenIssueCancelled(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Continue review", Objective: "Finish the review.", Priority: "medium", WorkMode: "guided", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "wakeup")
	if err != nil {
		t.Fatal(err)
	}
	comment := IssueComment{ID: nextID("comment"), IssueID: issue.ID, AuthorType: "operator", AuthorID: "operator", Body: "Please continue in context.", CreatedAt: time.Now()}
	if err = store.db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	wakeup := AgentWakeup{ID: nextID("wakeup"), IssueID: issue.ID, CommentID: comment.ID, AgentID: "backend-engineer", Reason: "issue_comment_assignee", Status: "queued", CreatedAt: comment.CreatedAt}
	if err = store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "cancelled", "execution_phase": "completed", "cancelled_at": time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions[execution.ID] = &PiSession{manager: manager, key: execution.ID, executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID, kind: execution.Kind, stdin: &trackedWriteCloser{}}

	manager.dispatchWakeup(wakeup.ID)

	var executionCount int64
	if err = store.db.Model(&Execution{}).Where("issue_id = ? AND agent_id = ?", issue.ID, execution.AgentID).Count(&executionCount).Error; err != nil {
		t.Fatal(err)
	}
	if executionCount != 2 {
		t.Fatalf("wakeup created a new execution: count=%d", executionCount)
	}
	if err = store.db.First(&wakeup, "id = ?", wakeup.ID).Error; err != nil {
		t.Fatal(err)
	}
	if wakeup.ExecutionID != execution.ID || wakeup.Status != "delivered" {
		t.Fatalf("wakeup was not delivered to existing execution: %+v", wakeup)
	}
	var message Message
	if err = store.db.Where("execution_id = ? AND role = ?", execution.ID, "user").Order("created_at desc").First(&message).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.Content, comment.Body) {
		t.Fatalf("continued prompt does not contain comment: %s", message.Content)
	}
}

func TestDeliveredCommentWakeupDoesNotSkipWorkerCompletion(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Finish after operator feedback", Objective: "Finish the implementation after incorporating operator feedback.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "rework")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "validation_disabled": true,
		"checkout_execution_id": execution.ID, "current_execution_id": execution.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	comment := IssueComment{
		ID: nextID("comment"), IssueID: issue.ID, AuthorType: "operator", AuthorID: "operator",
		Body: "Please include the final verification evidence.", CreatedAt: time.Now(),
	}
	if err = store.db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	wakeup := AgentWakeup{
		ID: nextID("wakeup"), IssueID: issue.ID, CommentID: comment.ID, AgentID: execution.AgentID,
		ExecutionID: execution.ID, Reason: "issue_comment_assignee", Status: "delivered", CreatedAt: comment.CreatedAt,
	}
	if err = store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	result := "Implementation complete; final verification passed."
	if err = store.db.Create(&Message{
		ID: nextID("message"), ExecutionID: execution.ID, IssueID: issue.ID, Role: "assistant",
		Content: result, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{
		manager: manager, key: execution.ID, executionID: execution.ID, issueID: issue.ID,
		agentID: execution.AgentID, kind: execution.Kind, stdin: &trackedWriteCloser{},
	}

	manager.handleSettled(session)

	finished, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "done" || finished.ExecutionPhase != "completed" || finished.Result != result || finished.CheckoutExecutionID != "" {
		t.Fatalf("worker completion was skipped after a delivered wakeup: %+v", finished)
	}
	if err = store.db.First(&wakeup, "id = ?", wakeup.ID).Error; err != nil {
		t.Fatal(err)
	}
	if wakeup.Status != "completed" || wakeup.CompletedAt == nil {
		t.Fatalf("delivered wakeup was not completed: %+v", wakeup)
	}
	if err = store.db.First(&execution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "completed" || execution.Result != result || execution.FinishedAt == nil {
		t.Fatalf("worker execution was not completed: %+v", execution)
	}
}

func TestCompletedIssueReworkRequestCreatesTypedApproval(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Refine completed security review", Objective: "Produce a complete security review.", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "red-team-lead",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "red-team-lead", "work")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed", "current_execution_id": execution.ID, "completed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	token := "rework-control-token"
	manager.sessions[execution.ID] = &PiSession{manager: manager, key: execution.ID, executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID, kind: execution.Kind, controlToken: token, stdin: &trackedWriteCloser{}}

	result, err := manager.RequestExecutionRework(execution.ID, token, RequestIssueReworkInput{
		Reason: "The operator requested a more detailed decomposition.", RequestedOutcome: "Create bounded child Issues and execute them after approval.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "approved" || result.ApprovalID != "" {
		t.Fatalf("unexpected rework request result: %+v", result)
	}
}

func TestOperatorCommentDeduplicatesAssigneeAndMentions(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Coordinate implementation", Objective: "Coordinate and complete implementation.", Priority: "medium", WorkMode: "guided", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.scheduleMu.Lock()
	body := "[@后端](agent://backend-engineer) 和 [@前端](agent://frontend-engineer) 请一起复核。"
	comment, err := manager.AddIssueComment(issue.ID, body)
	if err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	var wakeups []AgentWakeup
	if err = store.db.Where("comment_id = ?", comment.ID).Order("created_at asc").Find(&wakeups).Error; err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	agentIDs := make([]string, len(wakeups))
	for index, wakeup := range wakeups {
		agentIDs[index] = wakeup.AgentID
	}
	if len(wakeups) != 2 || !slices.Contains(agentIDs, "backend-engineer") || !slices.Contains(agentIDs, "frontend-engineer") {
		manager.scheduleMu.Unlock()
		t.Fatalf("unexpected deduplicated wakeups: %+v", wakeups)
	}
	store.db.Delete(&AgentWakeup{}, "comment_id = ?", comment.ID)
	manager.scheduleMu.Unlock()
}
