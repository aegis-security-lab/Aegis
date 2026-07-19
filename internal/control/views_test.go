package control

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestStateAndSessionListsOmitHeavyExecutionContent(t *testing.T) {
	s := configuredStore(t)
	issue, err := s.CreateIssue(CreateIssueInput{
		Title:     "Inspect compact state",
		Objective: "Keep list payloads small while preserving detail data.",
		Context:   strings.Repeat("context", 2_000),
		Priority:  "medium",
		WorkMode:  "guided",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("payload", 20_000)
	if err := s.updateExecution(execution.ID, map[string]any{
		"initial_prompt": large,
		"system_prompt":  large,
		"result":         large,
		"tools_snapshot": []ToolSnapshot{{Name: "write", Description: large}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"context": large,
		"result":  large,
	}).Error; err != nil {
		t.Fatal(err)
	}

	state := s.State()
	if len(state.Executions) != 1 {
		t.Fatalf("executions=%d, want 1", len(state.Executions))
	}
	assertCompactExecution(t, state.Executions[0])
	if len(state.Issues) != 1 || state.Issues[0].Context != "" || state.Issues[0].Result != "" {
		t.Fatalf("state issue still contains heavy detail: %+v", state.Issues)
	}
	if len(state.Sessions) != 1 {
		t.Fatalf("sessions=%d, want 1", len(state.Sessions))
	}
	assertCompactExecution(t, state.Sessions[0].Execution)

	sessions := s.Sessions()
	if len(sessions) != 1 {
		t.Fatalf("session list=%d, want 1", len(sessions))
	}
	assertCompactExecution(t, sessions[0].Execution)
	detail, err := s.GetSession(execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Session.Execution.InitialPrompt != large || detail.Session.Execution.Result != large {
		t.Fatal("session detail should preserve the complete execution snapshot")
	}
}

func TestIssueDetailBoundsEventsAndLoadsFullEventOnDemand(t *testing.T) {
	s := configuredStore(t)
	issue, err := s.CreateIssue(CreateIssueInput{
		Title:     "Inspect event pagination",
		Objective: "Return a bounded event summary and preserve full event details.",
		Priority:  "medium",
		WorkMode:  "guided",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&Message{
		ID:          "message-heavy",
		ExecutionID: execution.ID,
		IssueID:     issue.ID,
		Role:        "assistant",
		Content:     strings.Repeat("message", 20_000),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	var fullEvent ExecutionEvent
	for index := 0; index < issueDetailEventLimit+25; index++ {
		createdAt := time.Now().Add(time.Duration(index) * time.Millisecond)
		event := ExecutionEvent{
			ID:          fmt.Sprintf("event-%03d", index),
			ExecutionID: execution.ID,
			IssueID:     issue.ID,
			Type:        "tool",
			Title:       "Write file",
			ToolName:    "write",
			Status:      "completed",
			InputJSON: encodeEventPayload(map[string]any{
				"description": "生成最终证据报告供验收 Agent 核对。",
				"path":        "/tmp/report.md",
				"content":     strings.Repeat("line\n", 4_000),
			}),
			OutputJSON: strings.Repeat("output", 10_000),
			CreatedAt:  createdAt,
			UpdatedAt:  createdAt,
		}
		if err := s.db.Create(&event).Error; err != nil {
			t.Fatal(err)
		}
		if index == issueDetailEventLimit+24 {
			fullEvent = event
		}
	}

	detail, err := s.GetIssueDetail(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Events) != issueDetailEventLimit {
		t.Fatalf("events=%d, want %d", len(detail.Events), issueDetailEventLimit)
	}
	if len(detail.Messages) != 0 {
		t.Fatalf("issue detail should not duplicate session messages, got %d", len(detail.Messages))
	}
	latest := detail.Events[0]
	if latest.ID != fullEvent.ID {
		t.Fatalf("latest event=%q, want %q", latest.ID, fullEvent.ID)
	}
	if strings.Contains(latest.InputJSON, "line\\nline") || latest.OutputJSON != "" {
		t.Fatal("event summary still contains the full tool payload")
	}
	if !strings.Contains(latest.InputJSON, `"_lineCount":4001`) {
		t.Fatalf("event summary does not expose line count: %s", latest.InputJSON)
	}
	if !strings.Contains(latest.InputJSON, `"description":"生成最终证据报告供验收 Agent 核对。"`) {
		t.Fatalf("event summary does not preserve the invocation description: %s", latest.InputJSON)
	}
	loaded, err := s.GetExecutionEvent(fullEvent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.InputJSON != fullEvent.InputJSON || loaded.OutputJSON != fullEvent.OutputJSON {
		t.Fatal("full event endpoint did not preserve the stored payload")
	}
}

func TestNotifyCoalescesRapidBroadcasts(t *testing.T) {
	s := configuredStore(t)
	updates, unsubscribe := s.Subscribe()
	defer unsubscribe()
	<-updates // Initial state.

	for index := 0; index < 50; index++ {
		s.notify()
	}
	select {
	case <-updates:
	case <-time.After(2 * time.Second):
		t.Fatal("coalesced broadcast was not delivered")
	}
	select {
	case <-updates:
		t.Fatal("rapid notifications produced more than one state broadcast")
	case <-time.After(200 * time.Millisecond):
	}
}

func assertCompactExecution(t *testing.T, execution Execution) {
	t.Helper()
	if execution.InitialPrompt != "" || execution.SystemPrompt != "" || execution.Result != "" || len(execution.ToolsSnapshot) != 0 {
		t.Fatalf("execution still contains heavy detail: %+v", execution)
	}
}
