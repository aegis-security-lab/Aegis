package control

import (
	"strings"
	"testing"
	"time"
)

func TestExecutionProgressIsAuthenticatedPersistedAndVisibleInSession(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Implement progress reporting", Objective: "Progress is visible in the Session.",
		Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions[execution.ID] = &PiSession{
		executionID: execution.ID, issueID: issue.ID, agentID: "backend-engineer", controlToken: "progress-secret",
	}

	if _, err = manager.ReportExecutionProgress(execution.ID, "wrong-secretxx", ReportExecutionProgressInput{
		Stage: "调查", Summary: "找到了实现入口。", CurrentActivity: "开始实现。",
	}); err == nil {
		t.Fatal("expected an invalid execution control token to be rejected")
	}
	progress, err := manager.ReportExecutionProgress(execution.ID, "progress-secret", ReportExecutionProgressInput{
		Stage: " 调查完成 ", Summary: " 已确认 Pi Extension、内部 API 和 Session 数据链路。 ", CurrentActivity: " 正在实现持久化和页面展示。 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if progress.Stage != "调查完成" || progress.CurrentActivity != "正在实现持久化和页面展示。" {
		t.Fatalf("progress was not normalized: %+v", progress)
	}

	detail, err := store.GetSession(execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.ProgressUpdates) != 1 || detail.ProgressUpdates[0].ID != progress.ID {
		t.Fatalf("session progress=%+v", detail.ProgressUpdates)
	}
	if !strings.Contains(detail.Session.Execution.Checkpoint, "正在实现持久化和页面展示") {
		t.Fatalf("execution checkpoint=%q", detail.Session.Execution.Checkpoint)
	}
}

func TestExecutionProgressValidatesRequiredFieldsAndLengths(t *testing.T) {
	store := configuredStore(t)
	manager := &Manager{store: store, sessions: map[string]*PiSession{
		"execution-1": {executionID: "execution-1", issueID: "issue-1", controlToken: "progress-secret"},
	}}
	if _, err := manager.ReportExecutionProgress("execution-1", "progress-secret", ReportExecutionProgressInput{}); err == nil {
		t.Fatal("expected empty progress to be rejected")
	}
	if _, err := manager.ReportExecutionProgress("execution-1", "progress-secret", ReportExecutionProgressInput{
		Stage: strings.Repeat("阶", 121), Summary: "完成。", CurrentActivity: "继续。",
	}); err == nil {
		t.Fatal("expected an overlong stage to be rejected")
	}
}

func TestProgressPromptRequiresMaterialStageUpdates(t *testing.T) {
	prompt := agentProgressSystemPrompt("You are a Worker.")
	for _, required := range []string{"aegis_report_progress", "After completing each material stage", "currentActivity"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("progress prompt missing %q: %s", required, prompt)
		}
	}
}

func TestAgentCanReadSessionProgressInsideCurrentTaskTree(t *testing.T) {
	store := configuredStore(t)
	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Child", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	sourceExecution, _ := store.createExecution(parent, "backend-engineer", "work")
	targetExecution, _ := store.createExecution(child, "frontend-engineer", "work")
	now := time.Now()
	progress := ExecutionProgress{ID: nextID("progress"), ExecutionID: targetExecution.ID, IssueID: child.ID, Stage: "调查完成", Summary: "已定位入口。", CurrentActivity: "正在验证。", CreatedAt: now}
	if err := store.db.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&Execution{}).Where("id = ?", targetExecution.ID).Update("checkpoint", "正在验证").Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{
		sourceExecution.ID: {executionID: sourceExecution.ID, issueID: parent.ID, agentID: "backend-engineer", controlToken: "secret"},
	}}
	result, err := manager.GetIssueProgress(sourceExecution.ID, "secret", GetIssueProgressInput{SessionID: targetExecution.ID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Issue.ID != child.ID || result.Execution.ID != targetExecution.ID || len(result.ProgressUpdates) != 1 || result.ProgressUpdates[0].ID != progress.ID {
		t.Fatalf("unexpected progress result: %+v", result)
	}

	other, _ := store.CreateIssue(CreateIssueInput{Title: "Other task", Priority: "low", WorkMode: "autonomous"})
	otherExecution, _ := store.createExecution(other, "backend-engineer", "work")
	if _, err = manager.GetIssueProgress(sourceExecution.ID, "secret", GetIssueProgressInput{SessionID: otherExecution.ID}); err == nil {
		t.Fatal("expected cross-task progress read to be rejected")
	}
}
