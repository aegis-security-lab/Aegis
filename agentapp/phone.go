package agentapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	mu   sync.RWMutex
	apps map[string]App
}

func NewRegistry() *Registry { return &Registry{apps: map[string]App{}} }

func (r *Registry) Register(app App) error {
	manifest := app.Manifest()
	if err := ValidateManifest(manifest); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.apps[manifest.AppID]; exists {
		return fmt.Errorf("app %q already registered", manifest.AppID)
	}
	r.apps[manifest.AppID] = app
	return nil
}

func (r *Registry) App(id string) (App, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	app, ok := r.apps[id]
	return app, ok
}

func (r *Registry) Manifests() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Manifest, 0, len(r.apps))
	for _, app := range r.apps {
		items = append(items, app.Manifest())
	}
	sort.Slice(items, func(i, j int) bool { return items[i].AppID < items[j].AppID })
	return items
}

type Session struct {
	ID          string
	Actor       Actor
	Installed   []string
	ActiveAppID string
	Stacks      map[string][]Location
	Drafts      map[string]map[string]any
	Current     Page
	CreatedAt   time.Time
	UpdatedAt   time.Time
	results     map[string]ActionResponse
}

type Phone struct {
	registry    *Registry
	auditor     Auditor
	logs        AuditReader
	persistence SessionPersistence
	mu          sync.Mutex
	sessions    map[string]*Session
}

func NewPhone(registry *Registry, auditor Auditor) *Phone {
	logs := NewMemoryAuditor(defaultAuditCapacity)
	if auditor == nil {
		auditor = logs
	} else {
		auditor = multiAuditor{logs, auditor}
	}
	return &Phone{registry: registry, auditor: auditor, logs: logs, sessions: map[string]*Session{}}
}

func NewPersistentPhone(registry *Registry, persistence SessionPersistence, auditor Auditor) *Phone {
	if persistence == nil {
		return NewPhone(registry, auditor)
	}
	writers := multiAuditor{persistence}
	if auditor != nil {
		writers = append(writers, auditor)
	}
	return &Phone{registry: registry, auditor: writers, logs: persistence, persistence: persistence, sessions: map[string]*Session{}}
}

func (p *Phone) Start(ctx context.Context, actor Actor, installed []string) (Page, error) {
	_, page, err := p.StartSession(ctx, actor, installed)
	return page, err
}

func (p *Phone) StartSession(ctx context.Context, actor Actor, installed []string) (string, Page, error) {
	if actor.AgentID == "" {
		return "", Page{}, fmt.Errorf("agentId is required")
	}
	if strings.TrimSpace(actor.TaskID) != "" && strings.TrimSpace(actor.TaskAgentID) == "" {
		return "", Page{}, fmt.Errorf("taskAgentId is required for a task Phone")
	}
	for _, id := range installed {
		if _, ok := p.registry.App(id); !ok {
			return "", Page{}, fmt.Errorf("%w: %s", ErrAppNotFound, id)
		}
	}
	// A task gives each TaskAgent one durable Phone. Later
	// executions restore that Phone and refresh only the execution identity;
	// navigation, drafts and idempotency results remain task-local.
	identityID := strings.TrimSpace(actor.TaskAgentID)
	if p.persistence != nil && strings.TrimSpace(actor.TaskID) != "" {
		existing, err := p.persistence.FindTaskSession(ctx, actor.TaskID, identityID)
		if err == nil {
			existing.Actor.ExecutionID = actor.ExecutionID
			existing.Actor.WorkspaceID = actor.WorkspaceID
			existing.Actor.Scopes = append([]string(nil), actor.Scopes...)
			existing.Actor.Authenticated = true
			existing.Installed = append([]string(nil), installed...)
			existing.UpdatedAt = time.Now().UTC()
			if saveErr := p.persistence.SaveSession(ctx, existing); saveErr != nil {
				return "", Page{}, fmt.Errorf("refresh task Phone session: %w", saveErr)
			}
			p.mu.Lock()
			p.sessions[existing.ID] = existing
			p.mu.Unlock()
			return existing.ID, existing.Current, nil
		}
		if !errors.Is(err, ErrSessionNotFound) {
			return "", Page{}, fmt.Errorf("find task Phone session: %w", err)
		}
	}
	id := newID("phone")
	actor.SessionID = id
	actor.Authenticated = true
	session := &Session{ID: id, Actor: actor, Installed: append([]string(nil), installed...), Stacks: map[string][]Location{}, Drafts: map[string]map[string]any{}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), results: map[string]ActionResponse{}}
	p.mu.Lock()
	p.sessions[id] = session
	page := p.homePage(session)
	session.Current = page
	p.mu.Unlock()
	if p.persistence != nil {
		if err := p.persistence.SaveSession(ctx, session); err != nil {
			// Two workers may provision the same task-local Agent concurrently. The
			// partial unique index elects one Phone; attach the loser to it.
			if actor.TaskID != "" {
				if existing, findErr := p.persistence.FindTaskSession(ctx, actor.TaskID, identityID); findErr == nil {
					p.mu.Lock()
					delete(p.sessions, id)
					p.sessions[existing.ID] = existing
					p.mu.Unlock()
					return existing.ID, existing.Current, nil
				}
			}
			p.mu.Lock()
			delete(p.sessions, id)
			p.mu.Unlock()
			return "", Page{}, fmt.Errorf("persist phone session: %w", err)
		}
	}
	p.auditor.Record(ctx, AuditEvent{ID: newID("audit"), OccurredAt: time.Now().UTC(), AgentID: actor.AgentID, PhoneSessionID: id, AppID: page.AppID, PageID: page.PageID, PageRevision: page.Revision, Action: "start", Result: "success", Effect: "session_started"})
	return id, page, nil
}

func (p *Phone) View(ctx context.Context, sessionID string) (Page, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	session, err := p.sessionLocked(ctx, sessionID)
	if err != nil {
		return Page{}, err
	}
	return session.Current, nil
}

func (p *Phone) SessionActor(ctx context.Context, sessionID string) (Actor, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	session, err := p.sessionLocked(ctx, sessionID)
	if err != nil {
		return Actor{}, err
	}
	return session.Actor, nil
}

func (p *Phone) Logs(ctx context.Context, sessionID string, limit int) ([]AuditEvent, error) {
	if _, err := p.SessionActor(ctx, sessionID); err != nil {
		return nil, err
	}
	return p.logs.Events(ctx, sessionID, limit)
}

func (p *Phone) Sessions(ctx context.Context, agentID string, limit int) ([]SessionState, error) {
	if p.persistence != nil {
		return p.persistence.ListSessions(ctx, agentID, limit)
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	states := make([]SessionState, 0, len(p.sessions))
	for _, session := range p.sessions {
		if agentID == "" || session.Actor.AgentID == agentID {
			states = append(states, stateFromSession(session))
		}
	}
	sort.Slice(states, func(i, j int) bool { return states[i].UpdatedAt.After(states[j].UpdatedAt) })
	if len(states) > limit {
		states = states[:limit]
	}
	return states, nil
}

func (p *Phone) Act(ctx context.Context, request ActionRequest) (ActionResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	session, err := p.sessionLocked(ctx, request.PhoneSessionID)
	if err != nil {
		return ActionResponse{}, err
	}
	if request.IdempotencyKey != "" {
		if cached, exists := session.results[request.IdempotencyKey]; exists {
			p.auditor.Record(ctx, audit(session, session.Current, request, "", "success", "idempotent_replay", "", nil))
			return cached, nil
		}
	}
	current := session.Current
	if request.PageRevision != current.Revision {
		return ActionResponse{}, p.fail(ctx, session, request, "STALE_PAGE", ErrStalePage)
	}
	ref, found := findRef(current.Refs, request.Ref)
	if !found {
		return ActionResponse{}, p.fail(ctx, session, request, "INVALID_REF", ErrInvalidRef)
	}
	if !HasAction(ref, request.Action) {
		return ActionResponse{}, p.fail(ctx, session, request, "ACTION_NOT_ALLOWED", ErrActionNotAllowed)
	}
	if validationErr := validateGestureRequest(&request, current, ref); validationErr != nil {
		return ActionResponse{}, p.fail(ctx, session, request, "VALIDATION_ERROR", validationErr)
	}

	var response ActionResponse
	switch request.Action {
	case ActionHome:
		session.ActiveAppID = ""
		response = ActionResponse{Status: "ok", Effect: "home", Page: p.homePage(session)}
	case ActionBack:
		response, err = p.back(ctx, session)
	case ActionRefresh:
		response, err = p.refresh(ctx, session)
	case ActionOpenApp:
		response, err = p.openApp(ctx, session, ref.Target)
	case ActionSwipe:
		direction, _ := request.Arguments["direction"].(string)
		switch {
		case ref.Ref == "@back" && direction == "right":
			response, err = p.back(ctx, session)
		case ref.Ref == "@home" && direction == "up":
			session.ActiveAppID = ""
			response = ActionResponse{Status: "ok", Effect: "home_gesture", Page: p.homePage(session)}
		default:
			response, err = p.appAction(ctx, session, current, ref, request)
		}
	case ActionClick, ActionInput, ActionSelect, ActionToggle, ActionSubmit, ActionScroll, ActionLongPress, ActionDrag, ActionLoadMore:
		response, err = p.appAction(ctx, session, current, ref, request)
	default:
		err = ErrActionNotAllowed
	}
	if err != nil {
		return ActionResponse{}, p.fail(ctx, session, request, errorCode(err), err)
	}
	session.Current = response.Page
	session.UpdatedAt = time.Now().UTC()
	if request.IdempotencyKey != "" {
		session.results[request.IdempotencyKey] = response
	}
	if p.persistence != nil {
		if persistErr := p.persistence.SaveSession(ctx, session); persistErr != nil {
			wrapped := fmt.Errorf("persist phone state: %w", persistErr)
			p.auditor.Record(ctx, audit(session, current, request, ref.Target, "error", "", "PERSISTENCE_ERROR", wrapped))
			return ActionResponse{}, &ActionError{Code: "PERSISTENCE_ERROR", Message: wrapped.Error(), Page: &session.Current}
		}
	}
	p.auditor.Record(ctx, audit(session, current, request, ref.Target, "success", response.Effect, "", nil))
	return response, nil
}

func validateGestureRequest(request *ActionRequest, page Page, source Ref) error {
	if request.Arguments == nil {
		request.Arguments = map[string]any{}
	}
	switch request.Action {
	case ActionScroll:
		direction := stringArgument(request.Arguments, "direction", "down")
		amount := stringArgument(request.Arguments, "amount", "medium")
		if !oneOf(direction, "up", "down", "left", "right") || !oneOf(amount, "small", "medium", "large", "page") {
			return fmt.Errorf("scroll requires direction up/down/left/right and amount small/medium/large/page")
		}
		request.Arguments["direction"] = direction
		request.Arguments["amount"] = amount
	case ActionSwipe:
		direction := stringArgument(request.Arguments, "direction", "")
		distance := stringArgument(request.Arguments, "distance", "medium")
		if !oneOf(direction, "up", "down", "left", "right") || !oneOf(distance, "small", "medium", "large", "full") {
			return fmt.Errorf("swipe requires direction up/down/left/right and distance small/medium/large/full")
		}
		request.Arguments["direction"] = direction
		request.Arguments["distance"] = distance
		if source.Ref == "@back" && direction != "right" {
			return fmt.Errorf("swipe on @back requires direction right")
		}
		if source.Ref == "@home" && direction != "up" {
			return fmt.Errorf("swipe on @home requires direction up")
		}
	case ActionDrag:
		targetRef := stringArgument(request.Arguments, "targetRef", "")
		if targetRef == "" || targetRef == source.Ref {
			return fmt.Errorf("drag requires a different targetRef")
		}
		destination, found := findRef(page.Refs, targetRef)
		if !found || (destination.Kind != "drop_target" && destination.Metadata["dropTarget"] != "true") {
			return fmt.Errorf("drag targetRef must reference a drop target on the current page")
		}
		request.Arguments["destinationTarget"] = destination.Target
	}
	return nil
}

func stringArgument(arguments map[string]any, key, fallback string) string {
	value, _ := arguments[key].(string)
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func (p *Phone) sessionLocked(ctx context.Context, sessionID string) (*Session, error) {
	if session, ok := p.sessions[sessionID]; ok {
		return session, nil
	}
	if p.persistence == nil {
		return nil, ErrSessionNotFound
	}
	session, err := p.persistence.LoadSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Stacks == nil {
		session.Stacks = map[string][]Location{}
	}
	if session.Drafts == nil {
		session.Drafts = map[string]map[string]any{}
	}
	if session.results == nil {
		session.results = map[string]ActionResponse{}
	}
	p.sessions[sessionID] = session
	return session, nil
}

func (p *Phone) openApp(ctx context.Context, session *Session, appID string) (ActionResponse, error) {
	app, ok := p.registry.App(appID)
	if !ok {
		return ActionResponse{}, ErrAppNotFound
	}
	location := Location{AppID: appID, Route: "home"}
	session.ActiveAppID = appID
	session.Stacks[appID] = []Location{location}
	page, err := app.Render(ctx, RenderRequest{Actor: session.Actor, Route: location.Route, Params: location.Params, Draft: session.Drafts[appID]})
	if err != nil {
		return ActionResponse{}, err
	}
	page = withApp(page, app.Manifest(), false)
	return ActionResponse{Status: "ok", Effect: "opened_app", Page: page}, nil
}

func (p *Phone) appAction(ctx context.Context, session *Session, current Page, ref Ref, request ActionRequest) (ActionResponse, error) {
	app, ok := p.registry.App(session.ActiveAppID)
	if !ok {
		return ActionResponse{}, ErrAppNotFound
	}
	stack := session.Stacks[session.ActiveAppID]
	if len(stack) == 0 {
		return ActionResponse{}, ErrConflict
	}
	location := stack[len(stack)-1]
	if session.Drafts[session.ActiveAppID] == nil {
		session.Drafts[session.ActiveAppID] = map[string]any{}
	}
	if request.Action == ActionInput || request.Action == ActionSelect || request.Action == ActionToggle {
		value, exists := request.Arguments["value"]
		if request.Action == ActionToggle {
			value, exists = request.Arguments["checked"]
		}
		if !exists || ref.Field == "" {
			return ActionResponse{}, ErrValidation
		}
		session.Drafts[session.ActiveAppID][ref.Field] = value
	}
	result, err := app.Execute(ctx, Command{Actor: session.Actor, Location: location, Action: request.Action, Target: ref.Target, Arguments: request.Arguments, Draft: cloneMap(session.Drafts[session.ActiveAppID]), IdempotencyKey: request.IdempotencyKey})
	if err != nil {
		return ActionResponse{}, err
	}
	if result.Draft != nil {
		session.Drafts[session.ActiveAppID] = result.Draft
	}
	if result.Location != nil {
		if result.Effect == "replaced" {
			stack[len(stack)-1] = *result.Location
		} else {
			stack = append(stack, *result.Location)
		}
		session.Stacks[session.ActiveAppID] = stack
		location = *result.Location
	}
	page, err := app.Render(ctx, RenderRequest{Actor: session.Actor, Route: location.Route, Params: location.Params, Draft: session.Drafts[session.ActiveAppID]})
	if err != nil {
		return ActionResponse{}, err
	}
	page = withApp(page, app.Manifest(), len(stack) > 1)
	effect := result.Effect
	if effect == "" {
		effect = "updated"
	}
	return ActionResponse{Status: "ok", Effect: effect, Page: page, Toast: result.Toast}, nil
}

func (p *Phone) back(ctx context.Context, session *Session) (ActionResponse, error) {
	if session.ActiveAppID == "" {
		return ActionResponse{Status: "ok", Effect: "home", Page: p.homePage(session)}, nil
	}
	stack := session.Stacks[session.ActiveAppID]
	if len(stack) <= 1 {
		session.ActiveAppID = ""
		return ActionResponse{Status: "ok", Effect: "home", Page: p.homePage(session)}, nil
	}
	stack = stack[:len(stack)-1]
	session.Stacks[session.ActiveAppID] = stack
	location := stack[len(stack)-1]
	app, _ := p.registry.App(session.ActiveAppID)
	page, err := app.Render(ctx, RenderRequest{Actor: session.Actor, Route: location.Route, Params: location.Params, Draft: session.Drafts[session.ActiveAppID]})
	if err != nil {
		return ActionResponse{}, err
	}
	return ActionResponse{Status: "ok", Effect: "back", Page: withApp(page, app.Manifest(), len(stack) > 1)}, nil
}

func (p *Phone) refresh(ctx context.Context, session *Session) (ActionResponse, error) {
	if session.ActiveAppID == "" {
		return ActionResponse{Status: "ok", Effect: "refreshed", Page: p.homePage(session)}, nil
	}
	stack := session.Stacks[session.ActiveAppID]
	app, _ := p.registry.App(session.ActiveAppID)
	location := stack[len(stack)-1]
	page, err := app.Render(ctx, RenderRequest{Actor: session.Actor, Route: location.Route, Params: location.Params, Draft: session.Drafts[session.ActiveAppID]})
	if err != nil {
		return ActionResponse{}, err
	}
	return ActionResponse{Status: "ok", Effect: "refreshed", Page: withApp(page, app.Manifest(), len(stack) > 1)}, nil
}

func (p *Phone) homePage(session *Session) Page {
	page := Page{AppID: "agent.phone", PageID: "phone.home", Title: "Home", Summary: fmt.Sprintf("%d apps installed", len(session.Installed)), Metadata: map[string]string{"appName": session.Actor.AgentID + "'s phone"}}
	section := Section{Title: "Apps"}
	for index, id := range session.Installed {
		app, ok := p.registry.App(id)
		if !ok {
			continue
		}
		manifest := app.Manifest()
		ref := fmt.Sprintf("@%d", index+1)
		section.Lines = append(section.Lines, Line{Ref: ref, Kind: "app", Label: manifest.Name, Detail: manifest.Description})
		page.Refs = append(page.Refs, Ref{Ref: ref, Kind: "app", Label: manifest.Name, Actions: []string{ActionOpenApp}, Target: id})
	}
	page.Sections = []Section{section}
	return FinalizePage(page, false)
}

func withApp(page Page, manifest Manifest, canBack bool) Page {
	page.AppID = manifest.AppID
	if page.Metadata == nil {
		page.Metadata = map[string]string{}
	}
	page.Metadata["appName"] = manifest.Name
	return FinalizePage(page, canBack)
}

func findRef(refs []Ref, value string) (Ref, bool) {
	for _, ref := range refs {
		if ref.Ref == value {
			return ref, true
		}
	}
	return Ref{}, false
}

func (p *Phone) fail(ctx context.Context, session *Session, request ActionRequest, code string, err error) error {
	if p.persistence != nil {
		_ = p.persistence.SaveSession(ctx, session)
	}
	p.auditor.Record(ctx, audit(session, session.Current, request, "", "error", "", code, err))
	return &ActionError{Code: code, Message: err.Error(), Page: &session.Current}
}

func stateFromSession(session *Session) SessionState {
	depth := make(map[string]int, len(session.Stacks))
	for appID, stack := range session.Stacks {
		depth[appID] = len(stack)
	}
	return SessionState{ID: session.ID, AgentID: session.Actor.AgentID, TaskAgentID: session.Actor.TaskAgentID, TaskID: session.Actor.TaskID, ExecutionID: session.Actor.ExecutionID, WorkspaceID: session.Actor.WorkspaceID, InstalledApps: append([]string(nil), session.Installed...), ActiveAppID: session.ActiveAppID, CurrentAppID: session.Current.AppID, CurrentPageID: session.Current.PageID, CurrentRevision: session.Current.Revision, NavigationDepth: depth, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt}
}

func audit(session *Session, page Page, request ActionRequest, target, result, effect, code string, actionErr error) AuditEvent {
	event := AuditEvent{ID: newID("audit"), OccurredAt: time.Now().UTC(), AgentID: session.Actor.AgentID, PhoneSessionID: session.ID, AppID: page.AppID, PageID: page.PageID, PageRevision: page.Revision, Ref: request.Ref, Action: request.Action, Target: target, Result: result, Effect: effect, ErrorCode: code, Arguments: auditArguments(request.Arguments)}
	if actionErr != nil {
		event.ErrorMessage = actionErr.Error()
	}
	return event
}

func auditArguments(arguments map[string]any) map[string]any {
	if len(arguments) == 0 {
		return nil
	}
	result := make(map[string]any, len(arguments))
	for key, value := range arguments {
		if key == "value" {
			result[key] = "[redacted]"
			continue
		}
		result[key] = value
	}
	return result
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrValidation):
		return "VALIDATION_ERROR"
	case errors.Is(err, ErrPermissionDenied):
		return "PERMISSION_DENIED"
	case errors.Is(err, ErrConflict):
		return "CONFLICT"
	default:
		return "APP_ERROR"
	}
}

func cloneMap(input map[string]any) map[string]any {
	output := map[string]any{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func newID(prefix string) string {
	var data [10]byte
	_, _ = rand.Read(data[:])
	return prefix + "_" + hex.EncodeToString(data[:])
}
