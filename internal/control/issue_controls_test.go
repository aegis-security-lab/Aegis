package control

import (
	"strings"
	"testing"
	"time"
)

func TestManualAbandonSteersOwnerToSummarizeThenSkipsValidation(t *testing.T) {
	store := configuredStore(t)
	parent, err := store.CreateIssue(CreateIssueInput{
		Title: "Parent", Objective: "Complete the parent.", Priority: "high", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{
		ParentID: parent.ID, Title: "Child", Objective: "Complete the child.", Priority: "middle",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(child, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", child.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": execution.ID, "current_execution_id": execution.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{manager: manager, key: execution.ID, executionID: execution.ID, issueID: child.ID, agentID: execution.AgentID, kind: execution.Kind, stdin: &trackedWriteCloser{}}
	manager.sessions[execution.ID] = session

	requested, err := manager.AbandonIssue(child.ID, "需求已经失去价值")
	if err != nil {
		t.Fatal(err)
	}
	if requested.Status != "in_progress" || requested.ExecutionPhase != "summarizing" || requested.AbandonRequestedAt == nil || !requested.ValidationDisabled || requested.ObjectiveAbandoned {
		t.Fatalf("Issue did not enter cancellation summary: %+v", requested)
	}
	var systemComment IssueComment
	if err = store.db.Where("issue_id = ? AND author_type = ?", child.ID, "system").First(&systemComment).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(systemComment.Body, "操作员已放弃目标") || !strings.Contains(systemComment.Body, "最终总结") {
		t.Fatalf("unexpected cancellation system comment: %s", systemComment.Body)
	}
	var prompt Message
	if err = store.db.Where("execution_id = ? AND role = ?", execution.ID, "user").Order("created_at desc").First(&prompt).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.Content, "operator_abandoned_issue") || !strings.Contains(prompt.Content, "剩余风险") {
		t.Fatalf("owner did not receive the summary instruction: %s", prompt.Content)
	}

	summary := "已停止实施。完成了数据库变更；没有附件；剩余风险是尚未部署。"
	if err = store.db.Create(&Message{ID: nextID("message"), ExecutionID: execution.ID, IssueID: child.ID, Role: "assistant", Content: summary, CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	manager.handleSettled(session)
	finished, err := store.GetIssue(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "cancelled" || !finished.ObjectiveAbandoned || finished.AbandonedAt == nil || finished.Result != summary {
		t.Fatalf("manual abandonment did not finish correctly: %+v", finished)
	}
	var validationCount int64
	if err = store.db.Model(&IssueValidation{}).Where("issue_id = ?", child.ID).Count(&validationCount).Error; err != nil || validationCount != 0 {
		t.Fatalf("manual abandonment unexpectedly entered validation: count=%d err=%v", validationCount, err)
	}
}

func TestManualAbandonWithoutOwnerSessionFinishesImmediately(t *testing.T) {
	store := configuredStore(t)
	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Parent", Objective: "Complete parent.", Priority: "high", WorkMode: "autonomous"})
	child, _ := store.CreateIssue(CreateIssueInput{
		ParentID: parent.ID, Title: "Unstarted child", Objective: "Complete child.", Priority: "low", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	finished, err := manager.AbandonIssue(child.ID, "操作员不再需要该目标")
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "cancelled" || !finished.ObjectiveAbandoned || finished.AbandonRequestedAt == nil || finished.AbandonedAt == nil {
		t.Fatalf("Issue without a session was not abandoned immediately: %+v", finished)
	}
}

func TestBudgetAbandonReusesActiveWakeupForCancellationSummary(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{
		Title: "Restarted issue", Objective: "Finish current request.", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	execution, _ := store.createExecution(issue, "backend-engineer", "wakeup")
	if err := store.db.Model(&Execution{}).Where("id = ?", execution.ID).Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": execution.ID, "current_execution_id": execution.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{manager: manager, key: execution.ID, executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID, kind: execution.Kind, stdin: &trackedWriteCloser{}}
	manager.sessions[execution.ID] = session

	requested, err := manager.abandonIssueWithSummary(issue.ID, "本次 Execution 超出预算", issueBudgetSummaryInstruction, "Issue 执行预算已耗尽", "issue_budget_exhausted", true)
	if err != nil {
		t.Fatal(err)
	}
	if requested.ExecutionPhase != "summarizing" || requested.CurrentExecutionID != execution.ID || requested.ObjectiveAbandoned {
		t.Fatalf("active wakeup was not retained for cancellation summary: %+v", requested)
	}
	var prompt Message
	if err := store.db.Where("execution_id = ? AND role = ?", execution.ID, "user").Order("created_at desc").First(&prompt).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.Content, "issue_budget_exhausted") || !strings.Contains(prompt.Content, "最终总结") {
		t.Fatalf("active wakeup did not receive cancellation summary prompt: %s", prompt.Content)
	}
}

func TestDisableValidationStopsCurrentValidatorAndCompletesCandidate(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Skip current validation", Objective: "Produce the accepted output.", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	source, _ := store.createExecution(issue, "backend-engineer", "work")
	validator, _, err := store.createInternalExecution(issue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	candidate := "Implementation and tests are ready."
	validation := IssueValidation{
		ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: source.ID, ValidationExecutionID: validator.ID,
		Attempt: 1, Objective: issue.Objective, CandidateResult: candidate, Status: "running", CreatedAt: time.Now(),
	}
	if err = store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "validating", "result": candidate,
		"current_execution_id": validator.ID, "validation_execution_id": validator.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	writer := &trackedWriteCloser{}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions[validator.ID] = &PiSession{manager: manager, key: validator.ID, executionID: validator.ID, issueID: issue.ID, agentID: "acceptance-validator", kind: "validation", stdin: writer}

	updated, err := manager.SetIssueValidationDisabled(issue.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.ValidationDisabled || updated.Status != "done" || updated.Result != candidate || updated.ExecutionPhase != "completed" {
		t.Fatalf("validation cancellation did not complete the candidate: %+v", updated)
	}
	if !writer.closed {
		t.Fatal("validator session was not closed")
	}
	if err = store.db.First(&validation, "id = ?", validation.ID).Error; err != nil || validation.Status != "skipped" {
		t.Fatalf("validation was not recorded as skipped: err=%v validation=%+v", err, validation)
	}
	if err = store.db.First(&validator, "id = ?", validator.ID).Error; err != nil || validator.Status != "cancelled" {
		t.Fatalf("validation execution was not cancelled: err=%v execution=%+v", err, validator)
	}
}

func TestDisabledValidationPersistsAcrossWorkerCompletion(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Permanently skip validation", Objective: "Complete without validator.", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	if _, err = manager.SetIssueValidationDisabled(issue.ID, true); err != nil {
		t.Fatal(err)
	}
	execution, _ := store.createExecution(issue, "backend-engineer", "work")
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": execution.ID, "current_execution_id": execution.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	session := &PiSession{manager: manager, key: execution.ID, executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID, kind: execution.Kind, stdin: &trackedWriteCloser{}}
	manager.sessions[execution.ID] = session
	result := "Work completed directly."
	if err = store.db.Create(&Message{ID: nextID("message"), ExecutionID: execution.ID, IssueID: issue.ID, Role: "assistant", Content: result, CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	manager.handleSettled(session)
	finished, _ := store.GetIssue(issue.ID)
	if finished.Status != "done" || !finished.ValidationDisabled || finished.Result != result {
		t.Fatalf("disabled validation did not persist through completion: %+v", finished)
	}
	var count int64
	_ = store.db.Model(&IssueValidation{}).Where("issue_id = ?", issue.ID).Count(&count).Error
	if count != 0 {
		t.Fatalf("worker completion created %d validation records", count)
	}
}

func TestDisableValidationAdoptsCandidateBlockedAtValidationLimit(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{
		Title: "Blocked at validation limit", Objective: "Ship the candidate.", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	source, _ := store.createExecution(issue, "backend-engineer", "work")
	validator, _, _ := store.createInternalExecution(issue, "acceptance-validator", "validation")
	candidate := "The final candidate after all validation attempts."
	validation := IssueValidation{
		ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: source.ID, ValidationExecutionID: validator.ID,
		Attempt: 4, Objective: issue.Objective, CandidateResult: candidate, Status: "failed", CreatedAt: time.Now(),
	}
	if err := store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "labels": issueLabelsColumn([]string{"blocked"}), "execution_phase": "blocked", "result": candidate,
		"current_execution_id": validator.ID, "validation_execution_id": validator.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	updated, err := manager.SetIssueValidationDisabled(issue.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "done" || updated.Result != candidate || !updated.ValidationDisabled {
		t.Fatalf("blocked validation candidate was not adopted: %+v", updated)
	}
}

func TestRestartFinalizesPendingAbandonmentWithoutReexecutingWork(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Cancellation summary interrupted", Objective: "Stop this objective.", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "summarizing", "validation_disabled": true,
		"abandon_requested_at": now, "abandonment_reason": "operator cancelled",
	}).Error; err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	finished, err := reopened.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "cancelled" || finished.ExecutionPhase != "completed" || !finished.ObjectiveAbandoned || finished.AbandonedAt == nil {
		t.Fatalf("restart requeued pending abandonment: %+v", finished)
	}
}
