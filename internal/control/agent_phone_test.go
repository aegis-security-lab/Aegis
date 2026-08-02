package control

import (
	"context"
	"strings"
	"testing"

	"aegis/agentapp"
)

func phoneAction(t *testing.T, client AgentPhoneClient, sessionID string, page agentapp.Page, action, ref, key string, arguments map[string]any) agentapp.ActionResponse {
	t.Helper()
	response, err := client.Act(context.Background(), agentapp.ActionRequest{
		PhoneSessionID: sessionID, PageRevision: page.Revision, Action: action,
		Ref: ref, IdempotencyKey: key, Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("%s %s: %v", action, ref, err)
	}
	return response
}

func TestControlAgentPhoneReadsBoardAndAttributesIdempotentComment(t *testing.T) {
	store, manager := bridgeTestManager(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Phone-visible issue", Objective: "Exercise the extracted Board app.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed"}).Error; err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateIssue(CreateIssueInput{
		Title: "Other task issue", Objective: "Must stay outside this task Phone.",
		Priority: "low", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, client, err := NewControlAgentPhone(manager)
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.Start(context.Background(), agentapp.StartSessionRequest{
		AgentID: "backend-engineer", TaskAgentID: issue.AssigneeTaskAgentID, TaskID: issue.ID, ExecutionID: "execution-native-1",
		InstalledApps: []string{"aegis.board", "aegis.relay"},
	})
	if err != nil {
		t.Fatal(err)
	}
	board := phoneAction(t, client, started.PhoneSessionID, started.Page, agentapp.ActionOpenApp, "@1", "open-board", nil).Page
	if board.PageID != "board.home" || !strings.Contains(board.Text, issue.Identifier) || strings.Contains(board.Text, other.Identifier) {
		t.Fatalf("control issue was not exposed through Board:\n%s", board.Text)
	}
	detail := phoneAction(t, client, started.PhoneSessionID, board, agentapp.ActionClick, "@1", "open-issue", nil).Page
	detail = phoneAction(t, client, started.PhoneSessionID, detail, agentapp.ActionInput, "@1", "draft-comment", map[string]any{"value": "Verified through Agent Phone."}).Page
	posted := phoneAction(t, client, started.PhoneSessionID, detail, agentapp.ActionSubmit, "@2", "post-comment", nil)
	if posted.Toast != "Comment posted" {
		t.Fatalf("posted=%+v", posted)
	}

	// Tool retries are answered from the persisted Phone result cache and must
	// not duplicate the Board side effect even with a stale page revision.
	replayed, err := client.Act(context.Background(), agentapp.ActionRequest{
		PhoneSessionID: started.PhoneSessionID, PageRevision: "stale", Action: agentapp.ActionSubmit,
		Ref: "@2", IdempotencyKey: "post-comment",
	})
	if err != nil || replayed.Toast != "Comment posted" {
		t.Fatalf("idempotent replay=%+v err=%v", replayed, err)
	}
	var comments []IssueComment
	if err := store.db.Where("issue_id = ?", issue.ID).Find(&comments).Error; err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].AuthorID != issue.AssigneeTaskAgentID || comments[0].ExecutionID != "execution-native-1" || comments[0].Body != "Verified through Agent Phone." {
		t.Fatalf("comments=%+v", comments)
	}
}
