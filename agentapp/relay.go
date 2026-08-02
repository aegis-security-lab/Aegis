package agentapp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type RelayThread struct {
	ID, Title      string
	ParticipantIDs []string
	UpdatedAt      time.Time
}
type RelayMessage struct {
	ID, ThreadID, SenderID, Body string
	CreatedAt                    time.Time
}

type RelayRepository interface {
	Inbox(context.Context, Actor) ([]RelayThread, error)
	Messages(context.Context, Actor, string) ([]RelayMessage, error)
	Send(context.Context, Actor, string, string, string) (RelayMessage, error)
}

type RelayApp struct{ Repository RelayRepository }

func NewRelayApp(repository RelayRepository) *RelayApp { return &RelayApp{Repository: repository} }
func (a *RelayApp) Manifest() Manifest {
	return Manifest{AppID: "aegis.relay", Name: "Relay", Version: "1.0.0", Description: "Agent messages, mentions and task channels", Web: WebManifest{Entry: "/apps/aegis.relay"}, AI: AIManifest{ProtocolVersion: ProtocolVersion, Entry: "/apps/aegis.relay/ai/pages/home", Actions: "/phone/actions", ContentTypes: []string{"text/agent-ui", "application/agent-ui+json"}}, Capabilities: []string{ActionClick, ActionInput, ActionSubmit, ActionBack, ActionHome, ActionRefresh}}
}

func (a *RelayApp) Render(ctx context.Context, request RenderRequest) (Page, error) {
	switch request.Route {
	case "", "home":
		return a.inbox(ctx, request)
	case "thread":
		return a.thread(ctx, request)
	default:
		return Page{}, fmt.Errorf("unknown Relay route %q", request.Route)
	}
}

func (a *RelayApp) inbox(ctx context.Context, request RenderRequest) (Page, error) {
	threads, err := a.Repository.Inbox(ctx, request.Actor)
	if err != nil {
		return Page{}, err
	}
	sort.SliceStable(threads, func(i, j int) bool { return threads[i].UpdatedAt.After(threads[j].UpdatedAt) })
	page := Page{PageID: "relay.inbox", Title: "Inbox", Summary: fmt.Sprintf("%d conversations", len(threads))}
	section := Section{Title: "Conversations"}
	for index, thread := range threads {
		ref := fmt.Sprintf("@%d", index+1)
		section.Lines = append(section.Lines, Line{Ref: ref, Kind: "thread", Label: thread.Title, Detail: thread.UpdatedAt.UTC().Format(time.RFC3339)})
		page.Refs = append(page.Refs, Ref{Ref: ref, Kind: "thread", Label: thread.Title, Actions: []string{ActionClick}, Target: "thread:" + thread.ID})
	}
	if len(section.Lines) == 0 {
		section.Lines = append(section.Lines, Line{Label: "No conversations"})
	}
	page.Sections = []Section{section}
	return page, nil
}

func (a *RelayApp) thread(ctx context.Context, request RenderRequest) (Page, error) {
	messages, err := a.Repository.Messages(ctx, request.Actor, request.Params["id"])
	if err != nil {
		return Page{}, err
	}
	page := Page{PageID: "relay.thread.detail", Title: "Conversation", Summary: fmt.Sprintf("%d messages", len(messages))}
	section := Section{Title: "Recent messages"}
	start := 0
	if len(messages) > 20 {
		start = len(messages) - 20
	}
	for _, message := range messages[start:] {
		section.Lines = append(section.Lines, Line{Kind: "message", Label: message.SenderID + ": " + message.Body, Detail: message.CreatedAt.UTC().Format(time.RFC3339)})
	}
	if len(section.Lines) == 0 {
		section.Lines = append(section.Lines, Line{Label: "No messages"})
	}
	body, _ := request.Draft["message"].(string)
	page.Sections = []Section{section, {Title: "Actions", Lines: []Line{{Ref: "@1", Kind: "input:text name=message", Label: "Message", Detail: fmt.Sprintf("Current value: %q", body)}, {Ref: "@2", Kind: "button", Label: "Send message"}}}}
	page.Refs = []Ref{{Ref: "@1", Kind: "input", Label: "Message", Actions: []string{ActionInput}, Target: "message-input", Field: "message", InputType: "text"}, {Ref: "@2", Kind: "button", Label: "Send message", Actions: []string{ActionSubmit}, Target: "message-submit"}}
	return page, nil
}

func (a *RelayApp) Execute(ctx context.Context, command Command) (CommandResult, error) {
	if command.Action == ActionClick && strings.HasPrefix(command.Target, "thread:") {
		return CommandResult{Location: &Location{AppID: "aegis.relay", Route: "thread", Params: map[string]string{"id": strings.TrimPrefix(command.Target, "thread:")}}, Effect: "navigated"}, nil
	}
	if command.Action == ActionInput {
		return CommandResult{Effect: "draft_updated", Draft: command.Draft}, nil
	}
	if command.Action == ActionSubmit && command.Target == "message-submit" {
		body, _ := command.Draft["message"].(string)
		body = strings.TrimSpace(body)
		if body == "" {
			return CommandResult{}, ErrValidation
		}
		_, err := a.Repository.Send(ctx, command.Actor, command.Location.Params["id"], body, command.IdempotencyKey)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &command.Location, Effect: "replaced", Toast: "Message sent", Draft: map[string]any{}}, nil
	}
	return CommandResult{}, ErrActionNotAllowed
}

func (a *RelayApp) Shortcuts() []ShortcutDefinition {
	return []ShortcutDefinition{
		{Name: "phone_relay_list_threads", Description: "List task-scoped Relay conversations through the Phone.", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`), Frequency: 92},
		{Name: "phone_relay_get_thread", Description: "Open and read one task-scoped Relay conversation through the Phone.", Parameters: json.RawMessage(`{"type":"object","properties":{"threadId":{"type":"string","minLength":1}},"required":["threadId"],"additionalProperties":false}`), Frequency: 91},
		{Name: "phone_relay_send_message", Description: "Send an asynchronous message in an existing task-scoped Relay conversation through the Phone.", Parameters: json.RawMessage(`{"type":"object","properties":{"threadId":{"type":"string","minLength":1},"body":{"type":"string","minLength":1}},"required":["threadId","body"],"additionalProperties":false}`), Frequency: 90},
	}
}

func (a *RelayApp) ExecuteShortcut(ctx context.Context, command ShortcutCommand) (CommandResult, error) {
	switch command.Name {
	case "phone_relay_list_threads":
		return CommandResult{Location: &Location{AppID: "aegis.relay", Route: "home"}, Effect: "listed"}, nil
	case "phone_relay_get_thread":
		id := stringValue(command.Arguments, "threadId")
		if _, err := a.Repository.Messages(ctx, command.Actor, id); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &Location{AppID: "aegis.relay", Route: "thread", Params: map[string]string{"id": id}}, Effect: "opened"}, nil
	case "phone_relay_send_message":
		id, body := stringValue(command.Arguments, "threadId"), stringValue(command.Arguments, "body")
		if id == "" || body == "" {
			return CommandResult{}, ErrValidation
		}
		if _, err := a.Repository.Send(ctx, command.Actor, id, body, command.IdempotencyKey); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &Location{AppID: "aegis.relay", Route: "thread", Params: map[string]string{"id": id}}, Effect: "sent", Toast: "Message sent", Draft: map[string]any{}}, nil
	default:
		return CommandResult{}, ErrActionNotAllowed
	}
}

type MemoryRelayRepository struct {
	mu           sync.RWMutex
	Threads      []RelayThread
	MessagesList []RelayMessage
	keys         map[string]RelayMessage
}

func NewMemoryRelayRepository(threads ...RelayThread) *MemoryRelayRepository {
	now := time.Now().UTC()
	for i := range threads {
		if threads[i].UpdatedAt.IsZero() {
			threads[i].UpdatedAt = now
		}
	}
	return &MemoryRelayRepository{Threads: threads, keys: map[string]RelayMessage{}}
}
func (r *MemoryRelayRepository) Inbox(_ context.Context, actor Actor) ([]RelayThread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := []RelayThread{}
	for _, thread := range r.Threads {
		if contains(thread.ParticipantIDs, actorIdentityID(actor)) {
			result = append(result, thread)
		}
	}
	return result, nil
}
func (r *MemoryRelayRepository) Messages(_ context.Context, actor Actor, threadID string) ([]RelayMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.canRead(actorIdentityID(actor), threadID) {
		return nil, ErrPermissionDenied
	}
	result := []RelayMessage{}
	for _, message := range r.MessagesList {
		if message.ThreadID == threadID {
			result = append(result, message)
		}
	}
	return result, nil
}
func (r *MemoryRelayRepository) Send(_ context.Context, actor Actor, threadID, body, key string) (RelayMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.canRead(actorIdentityID(actor), threadID) {
		return RelayMessage{}, ErrPermissionDenied
	}
	if key != "" {
		if item, ok := r.keys[key]; ok {
			return item, nil
		}
	}
	message := RelayMessage{ID: newID("message"), ThreadID: threadID, SenderID: actorIdentityID(actor), Body: body, CreatedAt: time.Now().UTC()}
	r.MessagesList = append(r.MessagesList, message)
	for i := range r.Threads {
		if r.Threads[i].ID == threadID {
			r.Threads[i].UpdatedAt = message.CreatedAt
		}
	}
	if key != "" {
		r.keys[key] = message
	}
	return message, nil
}

func actorIdentityID(actor Actor) string {
	if value := strings.TrimSpace(actor.TaskAgentID); value != "" {
		return value
	}
	return strings.TrimSpace(actor.AgentID)
}
func (r *MemoryRelayRepository) canRead(agentID, threadID string) bool {
	for _, thread := range r.Threads {
		if thread.ID == threadID {
			return contains(thread.ParticipantIDs, agentID)
		}
	}
	return false
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
