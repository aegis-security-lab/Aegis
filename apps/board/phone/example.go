package boardphone

import (
	"time"

	. "aegis/agentapp"
)

// NewExampleModule returns a self-contained module with Board and Relay. It is
// useful for protocol development and can be replaced with production
// repositories without changing Phone or HTTP clients.
func NewExampleModule() (*Registry, *Phone, *HTTPServer) {
	registry := NewExampleRegistry()
	phone := NewPhone(registry, nil)
	return registry, phone, NewHTTPServer(registry, phone)
}

func NewPersistentExampleModule(databasePath string) (*Registry, *Phone, *HTTPServer, *GormStore, error) {
	registry := NewExampleRegistry()
	store, err := OpenSQLiteStore(databasePath)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	phone := NewPersistentPhone(registry, store, nil)
	return registry, phone, NewHTTPServer(registry, phone), store, nil
}

func NewExampleRegistry() *Registry {
	board := NewMemoryBoardRepository(
		BoardIssue{ID: "issue-1", Identifier: "ISSUE-1", Title: "Implement Agent Phone", Objective: "Ship the standalone module", Status: "in_progress", Priority: "high", AssigneeID: "demo-agent", UpdatedAt: time.Now().UTC()},
		BoardIssue{ID: "issue-2", Identifier: "ISSUE-2", Title: "Review app contract", Objective: "Check safety and compatibility", Status: "blocked", Priority: "medium", AssigneeID: "demo-agent", Blocked: true, UpdatedAt: time.Now().UTC().Add(-time.Hour)},
	)
	relay := NewMemoryRelayRepository(RelayThread{ID: "thread-1", Title: "Platform team", ParticipantIDs: []string{"demo-agent", "reviewer-agent"}, UpdatedAt: time.Now().UTC()})
	relay.MessagesList = append(relay.MessagesList, RelayMessage{ID: "message-1", ThreadID: "thread-1", SenderID: "reviewer-agent", Body: "Please share the protocol draft.", CreatedAt: time.Now().UTC()})
	registry := NewRegistry()
	_ = registry.Register(NewBoardApp(board))
	_ = registry.Register(NewRelayApp(relay))
	return registry
}
