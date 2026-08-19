package control

import (
	"context"
	"encoding/json"
	"testing"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

func TestNativeConciergeCapabilityCreatesTaskWithoutRPCToken(t *testing.T) {
	store := configuredStore(t)
	conversation, err := store.CreateConciergeConversation()
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store}
	resolved, err := (NativeConciergeSource{Manager: manager}).Resolve(context.Background(), capability.ResolveContext{
		ExecutionID: conversation.ExecutionID,
		AgentID:     conciergeAgentID,
	}, capability.Ref{Kind: capability.KindTool, Name: "concierge"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Tools) != 1 || resolved.Tools[0].Definition().Name != "aegis_create_task" {
		t.Fatalf("unexpected concierge tools: %+v", resolved.Tools)
	}
	if _, err = agentcore.New(agentcore.Config{Model: nativeDeliverySchemaModel{}, Tools: resolved.Tools}); err != nil {
		t.Fatalf("native concierge tool schema is invalid: %v", err)
	}
	result, err := resolved.Tools[0].Execute(context.Background(), json.RawMessage(`{
		"title":"AgentCore concierge task",
		"objective":"Create a durable task without a legacy RPC session.",
		"priority":"high",
		"workMode":"autonomous",
		"agentId":"backend-engineer"
	}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		IssueID string `json:"issueId"`
		Created bool   `json:"created"`
	}
	if err = json.Unmarshal([]byte(result.Content[0].Text), &output); err != nil {
		t.Fatal(err)
	}
	if output.IssueID == "" || !output.Created {
		t.Fatalf("unexpected tool result: %+v", output)
	}
	issue, err := store.GetIssue(output.IssueID)
	if err != nil {
		t.Fatal(err)
	}
	if issue.Title != "AgentCore concierge task" || issue.AssigneeAgentID != "backend-engineer" {
		t.Fatalf("unexpected created issue: %+v", issue)
	}
}
