package agentcoreadapter

import (
	"context"
	"encoding/json"
	"testing"

	"aegis/capability"
	"aegis/coordination"
	"github.com/z3r2ne/agentcore"
)

type recordingClient struct {
	delegation   Invocation
	continuation Invocation
	wait         Invocation
}

type schemaTestModel struct{}

func (schemaTestModel) Stream(context.Context, agentcore.ModelRequest) (agentcore.ModelStream, error) {
	return nil, nil
}

func (c *recordingClient) Delegate(_ context.Context, invocation Invocation, _ coordination.DelegationRequest) error {
	c.delegation = invocation
	return nil
}

func (c *recordingClient) Continue(_ context.Context, invocation Invocation, _ coordination.ContinueRequest) error {
	c.continuation = invocation
	return nil
}

func (c *recordingClient) Wait(_ context.Context, invocation Invocation, _ coordination.WaitRequest) error {
	c.wait = invocation
	return nil
}

func TestSourceExposesModeAwareDurableTools(t *testing.T) {
	client := &recordingClient{}
	resolved, err := (Source{Client: client}).Resolve(context.Background(), capability.ResolveContext{
		ExecutionID: "execution-1", AgentID: "parent-agent", Values: map[string]any{"control.issueId": "issue-1"},
	}, capability.Ref{Kind: capability.KindTool, Name: "coordination"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Tools) != 3 {
		t.Fatalf("tools=%d", len(resolved.Tools))
	}
	for _, tool := range resolved.Tools {
		if !json.Valid(tool.Definition().Parameters) {
			t.Fatalf("%s schema is not valid JSON: %s", tool.Definition().Name, tool.Definition().Parameters)
		}
	}
	if _, err := agentcore.New(agentcore.Config{Model: schemaTestModel{}, Tools: resolved.Tools}); err != nil {
		t.Fatalf("build agent with coordination tools: %v", err)
	}
	delegate := resolved.Tools[0]
	result, err := delegate.Execute(context.Background(), json.RawMessage(`{"children":[{"agentId":"child","prompt":"work"}]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.delegation.IssueID != "issue-1" || client.delegation.EventID == "" || result.Text() == "" {
		t.Fatalf("delegation=%+v result=%q", client.delegation, result.Text())
	}
	continueIssue := resolved.Tools[1]
	if continueIssue.Definition().Name != "coordinate_continue" {
		t.Fatalf("continue tool=%s", continueIssue.Definition().Name)
	}
	if _, err = continueIssue.Execute(context.Background(), json.RawMessage(`{"childIssueId":"child-1","reason":"evidence is promising"}`), nil); err != nil {
		t.Fatal(err)
	}
	if client.continuation.IssueID != "issue-1" || client.continuation.EventID == "" {
		t.Fatalf("continuation=%+v", client.continuation)
	}
	sleep := resolved.Tools[2]
	if sleep.Definition().Name != "coordinate_sleep" {
		t.Fatalf("sleep tool=%s", sleep.Definition().Name)
	}
	sleepResult, err := sleep.Execute(context.Background(), json.RawMessage(`{"wakeAfterSeconds":5}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.wait.ExecutionID != "execution-1" || client.wait.EventID == "" || !sleepResult.Terminate {
		t.Fatalf("wait=%+v result=%+v", client.wait, sleepResult)
	}
}
