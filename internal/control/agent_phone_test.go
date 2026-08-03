package control

import (
	"context"
	"strings"
	"testing"
	"time"

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

func TestControlPhoneBoardShortcutDelegatesThroughCoordination(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "phone-board-delegate", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Phone parent", Objective: "Delegate through Phone", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(parent, parent.AssigneeAgentID, "work")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "active", "current_execution_id": execution.ID, "checkout_execution_id": execution.ID}).Error; err != nil {
		t.Fatal(err)
	}
	_, client, err := NewControlAgentPhone(manager)
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.Start(context.Background(), agentapp.StartSessionRequest{AgentID: parent.AssigneeAgentID, TaskAgentID: parent.AssigneeTaskAgentID, TaskID: parent.ID, ExecutionID: execution.ID, InstalledApps: []string{"aegis.board", "aegis.relay"}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.RunShortcut(context.Background(), agentapp.ShortcutRequest{PhoneSessionID: started.PhoneSessionID, Name: "phone_board_delegate", IdempotencyKey: "delegate-one", Arguments: map[string]any{"children": []any{map[string]any{"agentId": "frontend-engineer", "title": "Focused child", "prompt": "Produce one independently verifiable result"}}}})
	if err != nil || response.Effect != "delegated" {
		t.Fatalf("shortcut response=%+v err=%v", response, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var children []Issue
		if err = store.db.Where("parent_id = ?", parent.ID).Find(&children).Error; err == nil && len(children) == 1 {
			if children[0].AssigneeAgentID != "frontend-engineer" || children[0].CreatedBy != parent.AssigneeTaskAgentID {
				t.Fatalf("child=%+v", children[0])
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Phone Board shortcut did not create a child Issue through Coordination")
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
	var identity TaskAgent
	if err = store.db.First(&identity, "id = ?", issue.AssigneeTaskAgentID).Error; err != nil {
		t.Fatal(err)
	}
	agentType, err := store.GetAgent(issue.AssigneeAgentID)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{identity.Name, agentType.Name, "done", "runtime completed"} {
		if !strings.Contains(board.Text, expected) {
			t.Fatalf("Board omitted assignment or status %q:\n%s", expected, board.Text)
		}
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

func TestControlPhoneBoardManagesUnassignedIssueLifecycle(t *testing.T) {
	store, manager := bridgeTestManager(t)
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Phone manager", Objective: "Manage child work", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(parent, parent.AssigneeAgentID, "work")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "active", "current_execution_id": execution.ID, "checkout_execution_id": execution.ID}).Error; err != nil {
		t.Fatal(err)
	}
	_, client, err := NewControlAgentPhone(manager)
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.Start(context.Background(), agentapp.StartSessionRequest{AgentID: parent.AssigneeAgentID, TaskAgentID: parent.AssigneeTaskAgentID, TaskID: parent.ID, ExecutionID: execution.ID, InstalledApps: []string{"aegis.board", "aegis.relay"}})
	if err != nil {
		t.Fatal(err)
	}
	shortcuts, err := client.Shortcuts(context.Background(), started.PhoneSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(shortcuts) > agentapp.MaxPhoneShortcuts {
		t.Fatalf("Phone shortcut cap exceeded: %d", len(shortcuts))
	}
	names := map[string]bool{}
	for _, shortcut := range shortcuts {
		names[shortcut.Name] = true
	}
	for _, expected := range []string{"phone_board_create_issue", "phone_board_update_issue", "phone_board_delete_issue"} {
		if !names[expected] {
			t.Fatalf("missing shortcut %s: %+v", expected, shortcuts)
		}
	}

	createdResponse, err := client.RunShortcut(context.Background(), agentapp.ShortcutRequest{PhoneSessionID: started.PhoneSessionID, Name: "phone_board_create_issue", IdempotencyKey: "create-unassigned", Arguments: map[string]any{"title": "Unclaimed investigation", "objective": "Record one finding", "priority": "middle"}})
	if err != nil || createdResponse.Effect != "created" {
		t.Fatalf("create response=%+v err=%v", createdResponse, err)
	}
	var child Issue
	if err = store.db.Where("parent_id = ?", parent.ID).First(&child).Error; err != nil {
		t.Fatal(err)
	}
	if child.Status != "todo" || child.Priority != "middle" || child.AssigneeAgentID != "" || child.AssigneeTaskAgentID != "" {
		t.Fatalf("unassigned child=%+v", child)
	}

	updatedResponse, err := client.RunShortcut(context.Background(), agentapp.ShortcutRequest{PhoneSessionID: started.PhoneSessionID, Name: "phone_board_update_issue", IdempotencyKey: "assign-child", Arguments: map[string]any{"issueId": child.ID, "priority": "high", "assigneeAgentId": "frontend-engineer"}})
	if err != nil || updatedResponse.Effect != "updated" {
		t.Fatalf("update response=%+v err=%v", updatedResponse, err)
	}
	child, _ = store.GetIssue(child.ID)
	if child.Priority != "high" || child.AssigneeAgentID != "frontend-engineer" || child.AssigneeTaskAgentID == "" {
		t.Fatalf("assigned child=%+v", child)
	}

	_, err = client.RunShortcut(context.Background(), agentapp.ShortcutRequest{PhoneSessionID: started.PhoneSessionID, Name: "phone_board_update_issue", IdempotencyKey: "cancel-child", Arguments: map[string]any{"issueId": child.ID, "status": "cancelled", "reason": "Direction is no longer useful"}})
	if err != nil {
		t.Fatal(err)
	}
	child, _ = store.GetIssue(child.ID)
	if child.Status != "cancelled" || child.Error != "Direction is no longer useful" {
		t.Fatalf("cancelled child=%+v", child)
	}
	deleted, err := client.RunShortcut(context.Background(), agentapp.ShortcutRequest{PhoneSessionID: started.PhoneSessionID, Name: "phone_board_delete_issue", IdempotencyKey: "delete-child", Arguments: map[string]any{"issueId": child.ID}})
	if err != nil || deleted.Effect != "deleted" {
		t.Fatalf("delete response=%+v err=%v", deleted, err)
	}
	if _, err = store.GetIssue(child.ID); err == nil {
		t.Fatal("deleted child still exists")
	}
}
