package control

import (
	"context"
	"errors"
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
		Title: "Phone-visible issue", Description: "Preserve the complete persisted Issue description.", Objective: "Exercise the extracted Board app.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	sameTaskRoot, err := store.CreateIssue(CreateIssueInput{
		TaskSourceID: issue.TaskSourceID,
		Title:        "Second root in the same task",
		Objective:    "Must share the Task-scoped Phone Board.",
		Priority:     "middle",
		WorkMode:     "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed", "updated_at": time.Now().Add(time.Second)}).Error; err != nil {
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
	phones, err := manager.TaskPhones(context.Background(), issue.TaskSourceID)
	if err != nil || len(phones) != 1 || phones[0].TaskID != issue.TaskSourceID {
		t.Fatalf("Task-scoped Phone list=%+v err=%v", phones, err)
	}
	if _, err = manager.TaskPhones(context.Background(), issue.ID); err == nil {
		t.Fatal("TaskPhones accepted an Issue id instead of a Task id")
	}
	board := phoneAction(t, client, started.PhoneSessionID, started.Page, agentapp.ActionOpenApp, "@1", "open-board", nil).Page
	if board.PageID != "board.home" || !strings.Contains(board.Text, issue.Identifier) || !strings.Contains(board.Text, sameTaskRoot.Identifier) || strings.Contains(board.Text, other.Identifier) {
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
	if !strings.Contains(detail.Text, "[SECTION] Description") || !strings.Contains(detail.Text, issue.Description) {
		t.Fatalf("Phone Board omitted the persisted Issue description:\n%s", detail.Text)
	}
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

func TestControlPhoneBoardAuthorizesByTaskAcrossRootIssueTrees(t *testing.T) {
	store, manager := bridgeTestManager(t)
	_, firstRoot, err := store.CreateTask(CreateIssueInput{
		Title: "Shared task root", Objective: "Coordinate the shared task.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondRoot, err := store.CreateIssue(CreateIssueInput{
		TaskSourceID:    firstRoot.TaskSourceID,
		Title:           "Independent root in shared task",
		Objective:       "Produce a result visible to the first root.",
		Priority:        "middle",
		WorkMode:        "autonomous",
		AssigneeAgentID: "frontend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondChild, err := store.CreateIssue(CreateIssueInput{
		ParentID: secondRoot.ID, Title: "Nested result", Objective: "Remain visible across roots.",
		Priority: "low", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, otherRoot, err := store.CreateTask(CreateIssueInput{
		Title: "Different task", Objective: "Stay isolated.", Priority: "low", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}

	repository := controlBoardRepository{manager: manager}
	actor := agentapp.Actor{
		AgentID: firstRoot.AssigneeAgentID, TaskAgentID: firstRoot.AssigneeTaskAgentID,
		TaskID: firstRoot.ID, Authenticated: true,
	}
	issues, err := repository.ListIssues(context.Background(), actor, "")
	if err != nil {
		t.Fatal(err)
	}
	visible := make(map[string]bool, len(issues))
	for _, issue := range issues {
		visible[issue.ID] = true
	}
	for _, expectedID := range []string{firstRoot.ID, secondRoot.ID, secondChild.ID} {
		if !visible[expectedID] {
			t.Fatalf("same-Task Issue %s was omitted: %+v", expectedID, issues)
		}
	}
	if visible[otherRoot.ID] {
		t.Fatalf("cross-Task root %s leaked into Board: %+v", otherRoot.ID, issues)
	}
	if _, _, err = repository.GetIssue(context.Background(), actor, secondRoot.ID); err != nil {
		t.Fatalf("same-Task root must be readable: %v", err)
	}
	if _, _, err = repository.GetIssue(context.Background(), actor, secondChild.ID); err != nil {
		t.Fatalf("same-Task nested Issue must be readable: %v", err)
	}
	if _, _, err = repository.GetIssue(context.Background(), actor, otherRoot.ID); !errors.Is(err, agentapp.ErrPermissionDenied) {
		t.Fatalf("cross-Task read err=%v, want permission denied", err)
	}

	forgedActor := actor
	forgedActor.TaskID = secondChild.ID
	if _, err = repository.ListIssues(context.Background(), forgedActor, ""); !errors.Is(err, agentapp.ErrPermissionDenied) {
		t.Fatalf("child Issue cannot be used as a Phone coordination root: %v", err)
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
