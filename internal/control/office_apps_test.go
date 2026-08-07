package control

import (
	"testing"
	"time"
)

func TestBoardCommentRequiresExplicitAgentCommand(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Board work", Objective: "Deliver it", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions[execution.ID] = &PiSession{executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID, controlToken: "board-secret"}

	result, err := manager.BoardCommandFromExecution(execution.ID, "board-secret", BoardCommandInput{Action: "comment", IssueID: issue.ID, Body: "Explicit Board update."})
	if err != nil {
		t.Fatal(err)
	}
	if result.Comment == nil || result.Comment.Body != "Explicit Board update." || result.Comment.AuthorID != execution.AgentID {
		t.Fatalf("unexpected Board result: %+v", result)
	}
	var count int64
	if err := store.db.Model(&IssueComment{}).Where("issue_id = ? AND execution_id = ?", issue.ID, execution.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("explicit comment count=%d err=%v", count, err)
	}
}

func TestRelayInboxTracksUnreadAndRead(t *testing.T) {
	store := configuredStore(t)
	message, err := store.SendRelayMessage("agent", "backend-engineer", SendRelayMessageInput{RecipientID: "frontend-engineer", Body: "Please review the UI."})
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := store.RelayInbox("frontend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || inbox[0].UnreadCount != 1 || inbox[0].LastMessage == nil || inbox[0].LastMessage.ID != message.ID {
		t.Fatalf("unexpected inbox: %+v", inbox)
	}
	conversation, err := store.RelayConversation("frontend-engineer", message.ThreadID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(conversation.Messages) != 1 || conversation.Messages[0].Body != "Please review the UI." {
		t.Fatalf("unexpected conversation: %+v", conversation)
	}
	inbox, err = store.RelayInbox("frontend-engineer")
	if err != nil || inbox[0].UnreadCount != 0 {
		t.Fatalf("message was not marked read: %+v err=%v", inbox, err)
	}
}

func TestRelayKeepsDirectMessagesInsideTaskBoundary(t *testing.T) {
	store := configuredStore(t)
	first, err := store.SendRelayMessage("agent", "backend-engineer", SendRelayMessageInput{
		TaskID: "task-alpha", RecipientID: "frontend-engineer", Body: "Alpha-only context.",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.SendRelayMessage("agent", "backend-engineer", SendRelayMessageInput{
		TaskID: "task-beta", RecipientID: "frontend-engineer", Body: "Beta-only context.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ThreadID == second.ThreadID {
		t.Fatal("the same employees must receive separate Relay threads in different tasks")
	}
	inbox, err := store.RelayInboxForTask("frontend-engineer", "task-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || inbox[0].Thread.ID != first.ThreadID || inbox[0].LastMessage == nil || inbox[0].LastMessage.Body != "Alpha-only context." {
		t.Fatalf("task-scoped inbox leaked or omitted a thread: %+v", inbox)
	}
	if _, err = store.RelayConversationForTask("frontend-engineer", second.ThreadID, "task-alpha", false); err == nil {
		t.Fatal("a task must not open another task's Relay conversation")
	}
}

func TestBoardCanRemoveRelationAndArchiveRunningIssueTree(t *testing.T) {
	store := configuredStore(t)
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Parent", Objective: "Coordinate work", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Target", Objective: "Run work", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Blocker", Objective: "Block target", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := store.AddRelation(blocker.ID, CreateRelationInput{RelatedIssueID: target.ID, Type: "blocks"})
	if err != nil {
		t.Fatal(err)
	}
	actorExecution, err := store.createExecution(parent, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	targetExecution, err := store.createExecution(target, "frontend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions[actorExecution.ID] = &PiSession{executionID: actorExecution.ID, issueID: parent.ID, agentID: actorExecution.AgentID, controlToken: "board-secret"}
	targetSession := &PiSession{executionID: targetExecution.ID, issueID: target.ID, agentID: targetExecution.AgentID}
	manager.sessions[targetExecution.ID] = targetSession

	result, err := manager.BoardCommandFromExecution(actorExecution.ID, "board-secret", BoardCommandInput{Action: "unrelate", RelationID: relation.ID})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedRelationID != relation.ID {
		t.Fatalf("deleted relation id=%q", result.DeletedRelationID)
	}
	var relationCount int64
	if err := store.db.Model(&IssueRelation{}).Where("id = ?", relation.ID).Count(&relationCount).Error; err != nil || relationCount != 0 {
		t.Fatalf("relation still exists count=%d err=%v", relationCount, err)
	}

	result, err = manager.BoardCommandFromExecution(actorExecution.ID, "board-secret", BoardCommandInput{Action: "archive", IssueID: target.ID, Reason: "No longer needed"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Issue == nil || result.Issue.Status != "cancelled" {
		t.Fatalf("unexpected archived issue: %+v", result.Issue)
	}
	var archivedExecution Execution
	if err := store.db.First(&archivedExecution, "id = ?", targetExecution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if archivedExecution.Status != "cancelled" || !targetSession.closed.Load() {
		t.Fatalf("runtime was not terminated: status=%s closed=%v", archivedExecution.Status, targetSession.closed.Load())
	}
}

func TestBoardStatusInReviewCancelsRunningExecutionAndTriggersValidation(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Review me", Objective: "Deliver the review", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := store.db.Model(&Execution{}).Where("id = ?", execution.ID).Updates(map[string]any{"status": "running", "started_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "in_progress", "current_execution_id": execution.ID, "result": "已产出交付物"}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID}
	manager.sessions[execution.ID] = session

	status := "in_review"
	updated, err := manager.UpdateBoardIssue(issue.ID, UpdateIssueInput{Status: &status})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "in_review" {
		t.Fatalf("status=%q want in_review", updated.Status)
	}
	var terminated Execution
	if err := store.db.First(&terminated, "id = ?", execution.ID).Error; err != nil || terminated.Status != "cancelled" {
		t.Fatalf("running execution was not terminated: status=%s err=%v", terminated.Status, err)
	}
	if !session.closed.Load() {
		t.Fatal("live session was not closed")
	}
	var comment IssueComment
	if err := store.db.Where("issue_id = ? AND type = ?", issue.ID, "delivery").First(&comment).Error; err != nil {
		t.Fatalf("no delivery comment published: %v", err)
	}
	if comment.ExecutionID != execution.ID {
		t.Fatalf("delivery comment execution=%q want %q", comment.ExecutionID, execution.ID)
	}
	var wakeup AgentWakeup
	if err := store.db.Where("issue_id = ? AND agent_id = ? AND reason = ?", issue.ID, "acceptance-validator", "delivery_validation").First(&wakeup).Error; err != nil {
		t.Fatalf("no acceptance-validator wakeup: %v", err)
	}
	if wakeup.CommentID != comment.ID {
		t.Fatalf("wakeup comment=%q want %q", wakeup.CommentID, comment.ID)
	}
}
