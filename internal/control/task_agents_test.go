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
	first, err := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Audit API", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "red-team-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Audit auth", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "red-team-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if first.AssigneeTaskAgentID == "" || second.AssigneeTaskAgentID == "" || first.AssigneeTaskAgentID == second.AssigneeTaskAgentID {
		t.Fatalf("task identities were not unique: first=%+v second=%+v", first, second)
	}
	identities, err := store.TaskAgents(root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 3 || identities[0].Name == identities[1].Name || identities[1].Name == identities[2].Name || identities[0].Name == identities[2].Name {
		t.Fatalf("task aliases=%+v", identities)
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

func TestTaskAgentNameCatalogHasOneHundredUniqueNames(t *testing.T) {
	names := TaskAgentNameCatalog()
	if len(names) != 100 {
		t.Fatalf("names=%d", len(names))
	}
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" || seen[name] {
			t.Fatalf("invalid name catalog entry %q", name)
		}
		seen[name] = true
	}
}

func TestRelayInboxIsolatedByTaskAgentIdentity(t *testing.T) {
	store := configuredStore(t)
	root, err := store.CreateIssue(CreateIssueInput{Title: "Coordinate audit", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "red-team-lead"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Audit API", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "red-team-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Audit auth", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "red-team-engineer"})
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
