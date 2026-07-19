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
	if _, err = s.GetSession(execution.SessionID); err == nil {
		t.Fatal("session detail must use the Aegis Execution ID")
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
	for index := 0; index < detailPageSize+25; index++ {
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
		if index == detailPageSize+24 {
			fullEvent = event
		}
	}

	detail, err := s.GetIssueDetail(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Events) != detailPageSize || !detail.EventsPage.HasMore || detail.EventsPage.Total != int64(detailPageSize+25) {
		t.Fatalf("unexpected initial event page: events=%d page=%+v", len(detail.Events), detail.EventsPage)
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
	older, err := s.IssueEventsPage(issue.ID, detail.EventsPage.NextCursor, detailPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Items) != 25 || older.Page.HasMore || older.Items[len(older.Items)-1].ID != "event-000" {
		t.Fatalf("unexpected older event page: items=%d page=%+v", len(older.Items), older.Page)
	}
	loaded, err := s.GetExecutionEvent(fullEvent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.InputJSON != fullEvent.InputJSON || loaded.OutputJSON != fullEvent.OutputJSON {
		t.Fatal("full event endpoint did not preserve the stored payload")
	}
}

func TestDetailCursorPagesAndSessionDelta(t *testing.T) {
	s := configuredStore(t)
	issue, err := s.CreateIssue(CreateIssueInput{
		Title: "Paginated activity", Objective: "Keep large histories bounded.", Priority: "medium", WorkMode: "guided",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Minute)
	for index := 0; index < detailPageSize+25; index++ {
		createdAt := base.Add(time.Duration(index) * time.Millisecond)
		if err = s.db.Create(&IssueComment{
			ID: fmt.Sprintf("comment-%03d", index), IssueID: issue.ID, AuthorType: "agent", AuthorID: "backend-engineer",
			Body: fmt.Sprintf("comment %d", index), CreatedAt: createdAt,
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.db.Create(&Message{
			ID: fmt.Sprintf("message-%03d", index), ExecutionID: execution.ID, IssueID: issue.ID, Role: "assistant",
			Content: fmt.Sprintf("message %d", index), CreatedAt: createdAt, UpdatedAt: createdAt,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	detail, err := s.GetIssueDetail(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Comments) != detailPageSize || detail.Comments[0].ID != "comment-025" || detail.CommentsPage.Total != int64(detailPageSize+25) {
		t.Fatalf("unexpected comment page: first=%q count=%d page=%+v", detail.Comments[0].ID, len(detail.Comments), detail.CommentsPage)
	}
	olderComments, err := s.IssueCommentsPage(issue.ID, detail.CommentsPage.NextCursor, detailPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(olderComments.Items) != 25 || olderComments.Items[0].ID != "comment-000" || olderComments.Page.HasMore {
		t.Fatalf("unexpected older comments: %+v", olderComments.Page)
	}

	session, err := s.GetSession(execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Messages) != detailPageSize || session.Messages[0].ID != "message-025" || !session.MessagesPage.HasMore {
		t.Fatalf("unexpected session message page: first=%q count=%d page=%+v", session.Messages[0].ID, len(session.Messages), session.MessagesPage)
	}
	newMessage := Message{
		ID: "message-live", ExecutionID: execution.ID, IssueID: issue.ID, Role: "assistant",
		Content: "streaming delta", Streaming: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err = s.db.Create(&newMessage).Error; err != nil {
		t.Fatal(err)
	}
	newEvent := ExecutionEvent{
		ID: "event-live", ExecutionID: execution.ID, IssueID: issue.ID, Type: "tool", ToolName: "bash",
		InputJSON:  encodeEventPayload(map[string]any{"command": strings.Repeat("echo x;", 500)}),
		OutputJSON: strings.Repeat("output", 20_000), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err = s.db.Create(&newEvent).Error; err != nil {
		t.Fatal(err)
	}
	delta, err := s.SessionDelta(execution.ID, session.Watermark)
	if err != nil {
		t.Fatal(err)
	}
	if len(delta.Messages) != 1 || delta.Messages[0].ID != newMessage.ID || len(delta.Events) != 1 {
		t.Fatalf("unexpected session delta: messages=%d events=%d", len(delta.Messages), len(delta.Events))
	}
	if delta.Events[0].OutputJSON != "" || len(delta.Events[0].InputJSON) >= len(newEvent.InputJSON) {
		t.Fatal("session delta did not compact the event payload")
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
