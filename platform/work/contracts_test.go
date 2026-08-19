package work

import (
	"testing"
	"time"

	"aegis/platform/dataspace"
)

func TestRequestRequiresApplicationScopeAndExplicitDataGrant(t *testing.T) {
	request := Request{
		AppID: "aegis.board", ScopeID: "task-1", CorrelationID: "issue-1", IdempotencyKey: "assign-1",
		AgentProfileID: "backend-engineer", Prompt: "Implement the Issue",
		DataSpaces: []dataspace.Grant{{Ref: dataspace.Ref{AppID: "aegis.board", Space: "primary", ScopeID: "task-1"}, SubjectID: "task-agent-1", Actions: []string{"read", "write"}}},
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.DataSpaces[0].AppID = "media.shortvideo"
	if err := request.Validate(); err == nil {
		t.Fatal("implicit cross-application DataSpace grant was accepted")
	}
}

func TestEventRequiresDurableCorrelationEnvelope(t *testing.T) {
	event := Event{
		EventID: "event-1", Sequence: 1, Type: EventScheduled, OccurredAt: time.Now().UTC(),
		AppID: "aegis.board", ScopeID: "task-1", CorrelationID: "issue-1", RequestID: "request-1", WorkID: "work-1",
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	event.RequestID = ""
	if err := event.Validate(); err == nil {
		t.Fatal("event without request identity was accepted")
	}
}
