package control

import (
	"context"
	"testing"

	"aegis/agentapp"
)

func TestSameAgentTypeGetsDistinctTaskIdentitiesAndPhones(t *testing.T) {
	store := configuredStore(t)
	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, phone, err := NewControlAgentPhone(manager)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetAgentPhoneClient(phone)

	root, err := store.CreateIssue(CreateIssueInput{Title: "Parallel red team", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "red-team-lead"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Audit API", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "red-team-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Audit auth", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "red-team-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if first.AssigneeTaskAgentID == "" || second.AssigneeTaskAgentID == "" || first.AssigneeTaskAgentID == second.AssigneeTaskAgentID {
		t.Fatalf("task identities were not unique: first=%+v second=%+v", first, second)
	}
	identities, err := store.TaskAgents(root.TaskSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 3 {
		t.Fatalf("task identities=%+v", identities)
	}
	for _, identity := range identities {
		agent, agentErr := store.GetAgent(identity.AgentID)
		if agentErr != nil || identity.Name != agent.Name {
			t.Fatalf("task identity uses a generated alias: identity=%+v agent=%+v err=%v", identity, agent, agentErr)
		}
	}

	firstPhone, err := phone.Start(context.Background(), agentapp.StartSessionRequest{AgentID: first.AssigneeAgentID, TaskAgentID: first.AssigneeTaskAgentID, TaskID: root.ID, InstalledApps: []string{"aegis.board", "aegis.relay"}})
	if err != nil {
		t.Fatal(err)
	}
	secondPhone, err := phone.Start(context.Background(), agentapp.StartSessionRequest{AgentID: second.AssigneeAgentID, TaskAgentID: second.AssigneeTaskAgentID, TaskID: root.ID, InstalledApps: []string{"aegis.board", "aegis.relay"}})
	if err != nil {
		t.Fatal(err)
	}
	if firstPhone.PhoneSessionID == secondPhone.PhoneSessionID {
		t.Fatal("two task identities of the same Agent type shared one Phone")
	}
	firstExecution, err := store.createExecution(first, first.AssigneeAgentID, "work")
	if err != nil {
		t.Fatal(err)
	}
	if firstExecution.TaskAgentID != first.AssigneeTaskAgentID {
		t.Fatalf("execution identity=%q want %q", firstExecution.TaskAgentID, first.AssigneeTaskAgentID)
	}
}

func TestCommentRoutingRepairsMissingIssueTaskAgent(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Legacy assigned Issue", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Update("assignee_task_agent_id", "").Error; err != nil {
		t.Fatal(err)
	}
	issue.AssigneeTaskAgentID = ""
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	bridge := &CoordinationBridge{manager: manager}
	repaired, err := bridge.ensureIssueTaskAgent(issue)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.AssigneeTaskAgentID == "" {
		t.Fatal("missing task-local Agent identity was not repaired")
	}
	identity, err := store.taskAgent(repaired.ID, repaired.AssigneeTaskAgentID)
	if err != nil {
		t.Fatal(err)
	}
	if identity.AgentID != issue.AssigneeAgentID {
		t.Fatalf("identity=%+v", identity)
	}
}

func TestExistingTaskAgentAliasesAreReplacedByAgentTypeNames(t *testing.T) {
	store := configuredStore(t)
	root, err := store.CreateIssue(CreateIssueInput{Title: "Normalize identity", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&TaskAgent{}).Where("id = ?", root.AssigneeTaskAgentID).Update("name", "云舟").Error; err != nil {
		t.Fatal(err)
	}
	if err = store.syncTaskAgentNames(); err != nil {
		t.Fatal(err)
	}
	identity, err := store.taskAgent(root.ID, root.AssigneeTaskAgentID)
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := store.GetAgent(root.AssigneeAgentID)
	if identity.Name != agent.Name {
		t.Fatalf("legacy alias was not removed: identity=%+v agent=%+v", identity, agent)
	}
}

func TestRelayInboxIsolatedByTaskAgentIdentity(t *testing.T) {
	store := configuredStore(t)
	root, err := store.CreateIssue(CreateIssueInput{Title: "Coordinate audit", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "red-team-lead"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Audit API", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "red-team-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Audit auth", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "red-team-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := store.SendRelayMessage("agent", root.AssigneeTaskAgentID, SendRelayMessageInput{
		RecipientID: first.AssigneeAgentID, RecipientTaskAgentID: first.AssigneeTaskAgentID,
		TaskID: root.ID, IssueID: first.ID, Body: "Please verify the API boundary.",
	})
	if err != nil {
		t.Fatal(err)
	}
	firstInbox, err := store.RelayInboxForTask(first.AssigneeTaskAgentID, root.ID)
	if err != nil || len(firstInbox) != 1 || firstInbox[0].LastMessage == nil || firstInbox[0].LastMessage.ID != message.ID {
		t.Fatalf("first inbox=%+v err=%v", firstInbox, err)
	}
	secondInbox, err := store.RelayInboxForTask(second.AssigneeTaskAgentID, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondInbox) != 0 {
		t.Fatalf("second instance received another identity's message: %+v", secondInbox)
	}
}
