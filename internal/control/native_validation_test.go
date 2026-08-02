package control

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

func TestNativeValidationCapabilityIsNarrowAndPersistsDecisionWithoutPiSession(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Native validation", Objective: "Verify published evidence", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	source, _ := store.createExecution(issue, issue.AssigneeAgentID, "work")
	validationExecution, _, err := store.createInternalExecution(issue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	validation := IssueValidation{
		ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: source.ID,
		ValidationExecutionID: validationExecution.ID, Attempt: 1, Objective: issue.Objective,
		CandidateResult: "evidence", Status: "running", CreatedAt: time.Now(),
	}
	if err = store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	resolved, err := (NativeValidationSource{Manager: manager}).Resolve(context.Background(), capability.ResolveContext{
		ExecutionID: validationExecution.ID, AgentID: "acceptance-validator",
	}, capability.Ref{Kind: capability.KindTool, Name: "validation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Tools) != 2 {
		t.Fatalf("validation tools=%d, want 2", len(resolved.Tools))
	}
	if _, err = agentcore.New(agentcore.Config{Model: nativeDeliverySchemaModel{}, Tools: resolved.Tools}); err != nil {
		t.Fatalf("native validation tool schema is invalid: %v", err)
	}
	closeTool := controlNamedTool(t, resolved.Tools, "aegis_close_current_issue")
	result, err := closeTool.Execute(context.Background(), json.RawMessage(`{"summary":"Published evidence satisfies every requirement."}`), nil)
	if err != nil || !result.Terminate {
		t.Fatalf("close result=%+v err=%v", result, err)
	}
	if err = store.db.First(&validation, "id = ?", validation.ID).Error; err != nil {
		t.Fatal(err)
	}
	decision, err := submittedValidationDecision(validation, "")
	if err != nil || decision.Outcome != "passed" {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
	for _, forbidden := range []string{"aegis_list_validation_attachments", "aegis_read_validation_attachment", "coordinate_delegate", "phone_view"} {
		for _, tool := range resolved.Tools {
			if tool.Definition().Name == forbidden {
				t.Fatalf("validator received forbidden tool %q", forbidden)
			}
		}
	}
}
