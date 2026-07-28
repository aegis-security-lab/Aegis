package control

import (
	"strings"
	"testing"
	"time"
)

func TestIssueHeartbeatDefaultsToOneMinute(t *testing.T) {
	config := normalizeIssueHeartbeat(IssueHeartbeatConfig{})
	if config.IntervalSeconds != 60 {
		t.Fatalf("default heartbeat interval = %d, want 60", config.IntervalSeconds)
	}
	if err := validateIssueHeartbeat(IssueHeartbeatConfig{IntervalSeconds: 0}); err == nil {
		t.Fatal("zero heartbeat interval must be rejected after explicit validation")
	}
	if err := validateIssueHeartbeat(IssueHeartbeatConfig{IntervalSeconds: 3601}); err == nil {
		t.Fatal("heartbeat interval above one hour must be rejected")
	}
}

func TestQueueDueIssueHeartbeatPersistsClockAndDeduplicates(t *testing.T) {
	store := configuredStore(t)
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Coordinate child work", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Child work", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	priorExecution, err := store.createExecution(parent, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	finished := time.Now().Add(-3 * time.Minute)
	if err = store.updateExecution(priorExecution.ID, map[string]any{"status": "completed", "finished_at": finished}); err != nil {
		t.Fatal(err)
	}
	anchor := time.Now().Add(-2 * time.Minute)
	if err = store.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "waiting_children", "checkout_execution_id": "",
		"current_execution_id": priorExecution.ID, "updated_at": anchor,
	}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	now := time.Now()
	queued := manager.queueDueIssueHeartbeats(now)
	if len(queued) != 1 {
		t.Fatalf("queued heartbeats = %v, want one", queued)
	}
	queuedAgain := manager.queueDueIssueHeartbeats(now.Add(time.Second))
	if len(queuedAgain) != 1 || queuedAgain[0] != queued[0] {
		t.Fatalf("live heartbeat was not reused: first=%v second=%v", queued, queuedAgain)
	}
	var count int64
	if err = store.db.Model(&AgentWakeup{}).Where("issue_id = ? AND reason = ?", parent.ID, issueHeartbeatReason).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("heartbeat count = %d, want 1", count)
	}
	var wakeup AgentWakeup
	if err = store.db.First(&wakeup, "id = ?", queued[0]).Error; err != nil {
		t.Fatal(err)
	}
	if wakeup.HeartbeatElapsedSeconds < 119 || wakeup.AgentID != parent.AssigneeAgentID {
		t.Fatalf("unexpected heartbeat: %+v", wakeup)
	}
	if child.ID == "" {
		t.Fatal("child setup failed")
	}
}

func TestDependencyRelationDoesNotCreateWaitingHeartbeat(t *testing.T) {
	store := configuredStore(t)
	blocker, _ := store.CreateIssue(CreateIssueInput{Title: "Prerequisite", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	blocked, _ := store.CreateIssue(CreateIssueInput{Title: "Dependent work", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	now := time.Now()
	if err := store.db.Create(&IssueRelation{ID: nextID("relation"), IssueID: blocker.ID, RelatedIssueID: blocked.ID, Type: "blocks", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&Issue{}).Where("id = ?", blocked.ID).UpdateColumn("updated_at", now.Add(-2*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	queued := manager.queueDueIssueHeartbeats(now)
	if len(queued) != 0 {
		t.Fatalf("informational dependency queued waiting heartbeats: %v", queued)
	}
	if eligible, reason := manager.issueHeartbeatEligibility(blocked); eligible || reason != "" {
		t.Fatalf("dependency relation still produced a waiting state: eligible=%t reason=%q", eligible, reason)
	}
}

func TestHeartbeatPromptContainsBudgetAndCoordinationContext(t *testing.T) {
	store := configuredStore(t)
	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Ship feature", Description: "Coordinate implementation", Objective: "Feature is verified", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Implement UI", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	_ = store.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "waiting_children", "checkout_execution_id": ""}).Error
	parent, _ = store.GetIssue(parent.ID)
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	prompt, err := manager.heartbeatPrompt(parent, AgentWakeup{HeartbeatElapsedSeconds: 75})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Aegis Issue 心跳唤醒", "1 分 15 秒", "时间预算", "剩余", child.Identifier, "aegis_get_issue_progress", "很严重的惩罚", "很大的奖励"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("heartbeat prompt missing %q:\n%s", expected, prompt)
		}
	}
}

func TestHeartbeatWakeupRestoresWaitingStateAndPriorExecution(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{Title: "Waiting parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	prior, _ := store.createExecution(issue, "backend-engineer", "work")
	_ = store.updateExecution(prior.ID, map[string]any{"status": "completed", "finished_at": time.Now()})
	_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "waiting_children", "current_execution_id": prior.ID, "checkout_execution_id": "",
	}).Error
	issue, _ = store.GetIssue(issue.ID)
	wakeup := AgentWakeup{ID: nextID("wakeup"), IssueID: issue.ID, AgentID: issue.AssigneeAgentID, Reason: issueHeartbeatReason, Status: "queued", CreatedAt: time.Now()}
	_ = store.db.Create(&wakeup).Error
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	if err := manager.markIssueForWakeup(&wakeup, issue, "heartbeat-execution", time.Now()); err != nil {
		t.Fatal(err)
	}
	manager.restoreIssueAfterWakeup(issue.ID, "heartbeat-execution")
	restored, _ := store.GetIssue(issue.ID)
	if restored.Status != "in_progress" || restored.ExecutionPhase != "waiting_children" || restored.CurrentExecutionID != prior.ID || restored.CheckoutExecutionID != "" {
		t.Fatalf("heartbeat did not restore waiting state: %+v", restored)
	}
}

func TestHeartbeatSettlesWithoutForcingParentContinuation(t *testing.T) {
	store := configuredStore(t)
	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Heartbeat parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Still blocked child", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	_ = store.db.Model(&Issue{}).Where("id = ?", child.ID).Updates(map[string]any{"status": "blocked", "execution_phase": "blocked"}).Error
	prior, _ := store.createExecution(parent, "backend-engineer", "work")
	_ = store.updateExecution(prior.ID, map[string]any{"status": "completed", "finished_at": time.Now()})
	heartbeat, _ := store.createExecution(parent, "backend-engineer", "heartbeat")
	_ = store.updateExecution(heartbeat.ID, map[string]any{"status": "running"})
	now := time.Now()
	wakeup := AgentWakeup{
		ID: nextID("wakeup"), IssueID: parent.ID, AgentID: parent.AssigneeAgentID, ExecutionID: heartbeat.ID,
		Reason: issueHeartbeatReason, Status: "delivered", PriorIssueStatus: "in_progress", PriorExecutionPhase: "waiting_children",
		PriorExecutionID: prior.ID, CreatedAt: now, DeliveredAt: &now,
	}
	_ = store.db.Create(&wakeup).Error
	_ = store.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": heartbeat.ID, "current_execution_id": heartbeat.ID,
	}).Error
	_ = store.db.Create(&Message{ID: nextID("message"), ExecutionID: heartbeat.ID, IssueID: parent.ID, Role: "assistant", Content: "Checked the child and sent useful guidance.", CreatedAt: now, UpdatedAt: now}).Error
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.handleSettled(&PiSession{manager: manager, executionID: heartbeat.ID, issueID: parent.ID, agentID: parent.AssigneeAgentID, kind: "heartbeat", wakeupID: wakeup.ID})
	restored, _ := store.GetIssue(parent.ID)
	if restored.Status != "in_progress" || restored.ExecutionPhase != "waiting_children" || restored.CurrentExecutionID != prior.ID {
		t.Fatalf("settled heartbeat did not return parent to waiting: %+v", restored)
	}
	if err := store.db.First(&heartbeat, "id = ?", heartbeat.ID).Error; err != nil || heartbeat.Status != "completed" {
		t.Fatalf("heartbeat execution not completed: %+v err=%v", heartbeat, err)
	}
}

func TestStoreRestartRestoresInterruptedHeartbeatWaitingState(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{Title: "Restart-safe parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	prior, _ := store.createExecution(issue, "backend-engineer", "work")
	_ = store.updateExecution(prior.ID, map[string]any{"status": "completed", "finished_at": time.Now()})
	heartbeat, _ := store.createExecution(issue, "backend-engineer", "heartbeat")
	now := time.Now()
	_ = store.updateExecution(heartbeat.ID, map[string]any{"status": "running"})
	_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": heartbeat.ID, "current_execution_id": heartbeat.ID,
	}).Error
	wakeup := AgentWakeup{
		ID: nextID("wakeup"), IssueID: issue.ID, AgentID: issue.AssigneeAgentID, ExecutionID: heartbeat.ID,
		Reason: issueHeartbeatReason, Status: "delivered", PriorIssueStatus: "in_progress",
		PriorExecutionPhase: "waiting_children", PriorExecutionID: prior.ID, CreatedAt: now, DeliveredAt: &now,
	}
	if err := store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	dataDir := store.DataDir()
	sqlDB, _ := store.db.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db, _ := reopened.db.DB()
		_ = db.Close()
	}()
	restored, _ := reopened.GetIssue(issue.ID)
	if restored.Status != "in_progress" || restored.ExecutionPhase != "waiting_children" || restored.CurrentExecutionID != prior.ID || restored.CheckoutExecutionID != "" {
		t.Fatalf("restart did not restore heartbeat waiting state: %+v", restored)
	}
	if err = reopened.db.First(&wakeup, "id = ?", wakeup.ID).Error; err != nil {
		t.Fatal(err)
	}
	if wakeup.Status != "failed" || wakeup.CompletedAt == nil {
		t.Fatalf("interrupted heartbeat wakeup not finalized: %+v", wakeup)
	}
}
