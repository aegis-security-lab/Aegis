package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"aegis/agentapp"
	"aegis/capability"
	"aegis/coordination"
)

// AgentPhoneClient runs the extracted Agent Phone in-process while preserving
// the same Client contract used by a remote Agent App service.
type AgentPhoneClient struct{ Phone *agentapp.Phone }

// TaskPhoneClient provisions or restores the one Phone owned by a TaskAgent
// inside one task run. Implementations must treat Start as idempotent for the
// TaskID + TaskAgentID pair.
type TaskPhoneClient interface {
	Start(context.Context, agentapp.StartSessionRequest) (agentapp.StartSessionResponse, error)
}

type TaskPhoneView struct {
	ID            string     `json:"id"`
	TaskID        string     `json:"taskId"`
	AgentID       string     `json:"agentId"`
	TaskAgentID   string     `json:"taskAgentId,omitempty"`
	Name          string     `json:"name,omitempty"`
	ExecutionID   string     `json:"executionId,omitempty"`
	WorkspaceID   string     `json:"workspaceId,omitempty"`
	InstalledApps []string   `json:"installedApps"`
	ActiveAppID   string     `json:"activeAppId,omitempty"`
	CurrentAppID  string     `json:"currentAppId,omitempty"`
	CurrentPageID string     `json:"currentPageId,omitempty"`
	ActionCount   int64      `json:"actionCount"`
	LastActionAt  *time.Time `json:"lastActionAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (m *Manager) TaskPhones(ctx context.Context, taskID string) ([]TaskPhoneView, error) {
	if m == nil || m.store == nil {
		return nil, errors.New("control: manager is required")
	}
	task, roots, _, err := taskIssueScopeWithDB(m.store.db.WithContext(ctx), taskID)
	if err != nil {
		return nil, err
	}
	if !m.store.db.Migrator().HasTable("agent_app_phone_sessions") {
		return []TaskPhoneView{}, nil
	}
	rootIDs := issueIDsOf(roots)
	if len(rootIDs) == 0 {
		return []TaskPhoneView{}, nil
	}
	type row struct {
		ID, TaskID, TaskAgentID, AgentID, ActorJSON, InstalledJSON, ActiveAppID, CurrentJSON string
		CreatedAt, UpdatedAt                                                                 time.Time
	}
	var rows []row
	if err := m.store.db.WithContext(ctx).Table("agent_app_phone_sessions").Where("task_id IN ?", rootIDs).Order("created_at asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]TaskPhoneView, len(rows))
	for index, item := range rows {
		var actor agentapp.Actor
		var installed []string
		var current struct {
			Page agentapp.Page `json:"page"`
		}
		_ = json.Unmarshal([]byte(item.ActorJSON), &actor)
		_ = json.Unmarshal([]byte(item.InstalledJSON), &installed)
		_ = json.Unmarshal([]byte(item.CurrentJSON), &current)
		view := TaskPhoneView{ID: item.ID, TaskID: task.ID, TaskAgentID: item.TaskAgentID, AgentID: item.AgentID, ExecutionID: actor.ExecutionID, WorkspaceID: actor.WorkspaceID, InstalledApps: installed, ActiveAppID: item.ActiveAppID, CurrentAppID: current.Page.AppID, CurrentPageID: current.Page.PageID, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
		if item.TaskAgentID != "" {
			var identity TaskAgent
			if identityErr := m.store.db.First(&identity, "id = ? AND task_id IN ?", item.TaskAgentID, rootIDs).Error; identityErr == nil {
				view.Name = identity.Name
			}
		}
		if m.store.db.Migrator().HasTable("agent_app_audit_events") {
			_ = m.store.db.WithContext(ctx).Table("agent_app_audit_events").Where("phone_session_id = ?", item.ID).Count(&view.ActionCount).Error
			var last struct{ OccurredAt time.Time }
			if queryErr := m.store.db.WithContext(ctx).Table("agent_app_audit_events").Select("occurred_at").Where("phone_session_id = ?", item.ID).Order("occurred_at desc").Limit(1).Scan(&last).Error; queryErr == nil && !last.OccurredAt.IsZero() {
				view.LastActionAt = &last.OccurredAt
			}
		}
		result[index] = view
	}
	return result, nil
}

func (m *Manager) SetAgentPhoneClient(client TaskPhoneClient) {
	if m == nil {
		return
	}
	m.phoneMu.Lock()
	m.phone = client
	m.phoneMu.Unlock()
}

func (m *Manager) EnsureTaskPhone(ctx context.Context, taskID string, issue Issue) (agentapp.StartSessionResponse, error) {
	if m == nil || strings.TrimSpace(taskID) == "" || strings.TrimSpace(issue.AssigneeAgentID) == "" || strings.TrimSpace(issue.AssigneeTaskAgentID) == "" {
		return agentapp.StartSessionResponse{}, errors.New("control: task and taskAgentId are required to provision a Phone")
	}
	m.phoneMu.RLock()
	client := m.phone
	m.phoneMu.RUnlock()
	if client == nil {
		return agentapp.StartSessionResponse{}, errors.New("control: Agent Phone is unavailable")
	}
	return client.Start(ctx, agentapp.StartSessionRequest{
		AgentID: issue.AssigneeAgentID, TaskAgentID: issue.AssigneeTaskAgentID, TaskID: taskID, ExecutionID: issue.CurrentExecutionID,
		WorkspaceID: issue.Workspace, InstalledApps: []string{"aegis.board", "aegis.relay"},
	})
}

func (c AgentPhoneClient) Start(ctx context.Context, request agentapp.StartSessionRequest) (agentapp.StartSessionResponse, error) {
	if c.Phone == nil {
		return agentapp.StartSessionResponse{}, errors.New("control: nil Agent Phone")
	}
	id, page, err := c.Phone.StartSession(ctx, agentapp.Actor{
		AgentID: request.AgentID, TaskAgentID: request.TaskAgentID, TaskID: request.TaskID, ExecutionID: request.ExecutionID, WorkspaceID: request.WorkspaceID,
		Scopes: append([]string(nil), request.Scopes...), Authenticated: true,
	}, request.InstalledApps)
	return agentapp.StartSessionResponse{PhoneSessionID: id, Page: page}, err
}

func (c AgentPhoneClient) View(ctx context.Context, sessionID string) (agentapp.Page, error) {
	if c.Phone == nil {
		return agentapp.Page{}, errors.New("control: nil Agent Phone")
	}
	return c.Phone.View(ctx, sessionID)
}

func (c AgentPhoneClient) Act(ctx context.Context, request agentapp.ActionRequest) (agentapp.ActionResponse, error) {
	if c.Phone == nil {
		return agentapp.ActionResponse{}, errors.New("control: nil Agent Phone")
	}
	return c.Phone.Act(ctx, request)
}

func (c AgentPhoneClient) Shortcuts(ctx context.Context, sessionID string) ([]agentapp.ShortcutDefinition, error) {
	if c.Phone == nil {
		return nil, errors.New("control: nil Agent Phone")
	}
	return c.Phone.Shortcuts(ctx, sessionID)
}

func (c AgentPhoneClient) RunShortcut(ctx context.Context, request agentapp.ShortcutRequest) (agentapp.ShortcutResponse, error) {
	if c.Phone == nil {
		return agentapp.ShortcutResponse{}, errors.New("control: nil Agent Phone")
	}
	return c.Phone.RunShortcut(ctx, request)
}

type controlBoardRepository struct{ manager *Manager }

func (r controlBoardRepository) ListIssues(ctx context.Context, actor agentapp.Actor, query string) ([]agentapp.BoardIssue, error) {
	if r.manager == nil || r.manager.store == nil {
		return nil, errors.New("control: Board store is unavailable")
	}
	coordinationRootID := strings.TrimSpace(actor.TaskID)
	if coordinationRootID == "" {
		return nil, agentapp.ErrPermissionDenied
	}
	var issues []Issue
	db := r.manager.store.db.WithContext(ctx).Where("hidden = ?", false).Order("updated_at desc")
	if err := db.Find(&issues).Error; err != nil {
		return nil, err
	}
	issueByID := make(map[string]Issue, len(issues))
	for _, issue := range issues {
		issueByID[issue.ID] = issue
	}
	coordinationRoot, ok := issueByID[coordinationRootID]
	if !ok || coordinationRoot.ParentID != "" {
		return nil, agentapp.ErrPermissionDenied
	}
	taskSourceID := strings.TrimSpace(coordinationRoot.TaskSourceID)
	if taskSourceID == "" {
		return nil, agentapp.ErrPermissionDenied
	}
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]agentapp.BoardIssue, 0, len(issues))
	taskSourceByIssueID := map[string]string{coordinationRoot.ID: taskSourceID}
	for _, issue := range issues {
		issueTaskSourceID, found := taskSourceIDFromIssueSet(issue, issueByID, taskSourceByIssueID)
		if !found || issueTaskSourceID != taskSourceID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(issue.Identifier+" "+issue.Title+" "+issue.Objective), query) {
			continue
		}
		result = append(result, r.boardIssue(issue))
	}
	return result, nil
}

func (r controlBoardRepository) GetIssue(_ context.Context, actor agentapp.Actor, id string) (agentapp.BoardIssue, []agentapp.BoardComment, error) {
	if r.manager == nil || r.manager.store == nil {
		return agentapp.BoardIssue{}, nil, errors.New("control: Board store is unavailable")
	}
	taskSourceID, err := r.actorTaskSourceID(actor)
	if err != nil {
		return agentapp.BoardIssue{}, nil, err
	}
	detail, err := r.manager.store.GetIssueDetail(id)
	if err != nil {
		return agentapp.BoardIssue{}, nil, err
	}
	root, rootErr := r.manager.store.taskRoot(detail.Issue)
	if rootErr != nil || strings.TrimSpace(root.TaskSourceID) == "" || root.TaskSourceID != taskSourceID {
		return agentapp.BoardIssue{}, nil, agentapp.ErrPermissionDenied
	}
	comments := make([]agentapp.BoardComment, len(detail.Comments))
	for index, comment := range detail.Comments {
		comments[index] = agentapp.BoardComment{ID: comment.ID, IssueID: comment.IssueID, AuthorID: comment.AuthorID, Body: comment.Body, CreatedAt: comment.CreatedAt}
	}
	return r.boardIssue(detail.Issue), comments, nil
}

// Actor.TaskID is the coordination-root Issue ID used by Phone sessions,
// Relay, and task-local Agent identities. Board authorization deliberately
// resolves that root to TaskSourceID so every root Issue in the same durable
// Task shares one Board, without changing the narrower coordination identity.
func (r controlBoardRepository) actorTaskSourceID(actor agentapp.Actor) (string, error) {
	coordinationRootID := strings.TrimSpace(actor.TaskID)
	if coordinationRootID == "" {
		return "", agentapp.ErrPermissionDenied
	}
	root, err := r.manager.store.GetIssue(coordinationRootID)
	if err != nil || root.ParentID != "" || strings.TrimSpace(root.TaskSourceID) == "" {
		return "", agentapp.ErrPermissionDenied
	}
	return root.TaskSourceID, nil
}

// taskSourceIDFromIssueSet resolves an Issue to its root Task without an SQL
// query per row. Broken or cyclic hierarchies are excluded from the Board.
func taskSourceIDFromIssueSet(issue Issue, issueByID map[string]Issue, cache map[string]string) (string, bool) {
	current := issue
	path := make([]string, 0, issue.RequestDepth+1)
	seen := make(map[string]bool, issue.RequestDepth+1)
	for {
		if taskSourceID, ok := cache[current.ID]; ok {
			for _, issueID := range path {
				cache[issueID] = taskSourceID
			}
			return taskSourceID, true
		}
		if seen[current.ID] {
			return "", false
		}
		seen[current.ID] = true
		path = append(path, current.ID)
		if current.ParentID == "" {
			taskSourceID := strings.TrimSpace(current.TaskSourceID)
			if taskSourceID == "" {
				return "", false
			}
			for _, issueID := range path {
				cache[issueID] = taskSourceID
			}
			return taskSourceID, true
		}
		parent, ok := issueByID[current.ParentID]
		if !ok {
			return "", false
		}
		current = parent
	}
}

func (r controlBoardRepository) CreateIssue(ctx context.Context, actor agentapp.Actor, input agentapp.BoardCreateIssueRequest, _ string) (agentapp.BoardIssue, error) {
	current, err := r.currentIssue(actor)
	if err != nil {
		return agentapp.BoardIssue{}, err
	}
	parentID := strings.TrimSpace(input.ParentIssueID)
	if parentID == "" {
		parentID = current.ID
	}
	if _, _, err = r.GetIssue(ctx, actor, parentID); err != nil {
		return agentapp.BoardIssue{}, err
	}
	assigneeID := strings.TrimSpace(input.AssigneeAgentID)
	if assigneeID != "" {
		if err = r.manager.store.ValidateDelegation(actor.AgentID, assigneeID); err != nil {
			return agentapp.BoardIssue{}, err
		}
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "todo"
	}
	if status != "todo" {
		return agentapp.BoardIssue{}, errors.New("Phone Board 创建 Issue 时状态只能是 todo")
	}
	attachmentIDs := r.manager.store.InputAttachmentIDsForExecution(actor.ExecutionID)
	created, err := r.manager.store.CreateIssue(CreateIssueInput{
		ParentID: parentID, Title: input.Title, Description: input.Description, Objective: input.Objective,
		Priority: fallback(strings.TrimSpace(input.Priority), "middle"), Status: status, WorkMode: "autonomous",
		AssigneeAgentID: assigneeID, AttachmentIDs: attachmentIDs, AttachmentSourceExecutionID: actor.ExecutionID,
		CreatedBy: actorRoutingID(actor),
	})
	if err != nil {
		return agentapp.BoardIssue{}, err
	}
	if assigneeID != "" && created.Status == "todo" {
		if bridge := r.manager.Coordination(); bridge != nil {
			if err = bridge.SubmitIssueCreated(ctx, created); err != nil {
				return agentapp.BoardIssue{}, err
			}
		} else {
			r.manager.notifyBoardAssignment(actorRoutingID(actor), created, "Phone Board 创建并委派了 Issue")
		}
	}
	r.manager.store.notify()
	return r.boardIssue(created), nil
}

func (r controlBoardRepository) UpdateIssue(ctx context.Context, actor agentapp.Actor, input agentapp.BoardUpdateIssueRequest, _ string) (agentapp.BoardIssue, error) {
	issueID := strings.TrimSpace(input.IssueID)
	if issueID == "" {
		return agentapp.BoardIssue{}, agentapp.ErrValidation
	}
	if _, _, err := r.GetIssue(ctx, actor, issueID); err != nil {
		return agentapp.BoardIssue{}, err
	}
	if input.AssigneeAgentID != nil {
		*input.AssigneeAgentID = strings.TrimSpace(*input.AssigneeAgentID)
		if *input.AssigneeAgentID != "" {
			if err := r.manager.store.ValidateDelegation(actor.AgentID, *input.AssigneeAgentID); err != nil {
				return agentapp.BoardIssue{}, err
			}
		}
	}
	status := ""
	if input.Status != nil {
		status = strings.TrimSpace(*input.Status)
		*input.Status = status
	}
	update := UpdateIssueInput{Title: input.Title, Description: input.Description, Objective: input.Objective, Priority: input.Priority, Status: input.Status, AssigneeAgentID: input.AssigneeAgentID}
	if status == "cancelled" {
		update.Status = nil
		if hasBoardIssueUpdate(update) {
			if _, err := r.manager.UpdateBoardIssue(issueID, update); err != nil {
				return agentapp.BoardIssue{}, err
			}
		}
		reason := fallback(strings.TrimSpace(input.Reason), "Agent 通过 Phone Board 取消了 Issue")
		cancelled, err := r.manager.archiveBoardIssue(issueID, reason)
		if err != nil {
			return agentapp.BoardIssue{}, err
		}
		return r.boardIssue(cancelled), nil
	}
	if !hasBoardIssueUpdate(update) {
		return agentapp.BoardIssue{}, agentapp.ErrValidation
	}
	updated, err := r.manager.UpdateBoardIssue(issueID, update)
	if err != nil {
		return agentapp.BoardIssue{}, err
	}
	return r.boardIssue(updated), nil
}

func (r controlBoardRepository) DeleteIssue(ctx context.Context, actor agentapp.Actor, issueID, _ string) (int64, error) {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return 0, agentapp.ErrValidation
	}
	if _, _, err := r.GetIssue(ctx, actor, issueID); err != nil {
		return 0, err
	}
	result, err := r.manager.DeleteBoardIssue(issueID)
	if err != nil {
		return 0, err
	}
	return result.DeletedIssues, nil
}

func (r controlBoardRepository) Delegate(ctx context.Context, actor agentapp.Actor, request agentapp.BoardDelegationRequest, key string) error {
	issue, err := r.currentIssue(actor)
	if err != nil {
		return err
	}
	children := make([]coordination.ChildWork, len(request.Children))
	for index, child := range request.Children {
		capabilities := make([]capability.Ref, len(child.Capabilities))
		for capabilityIndex, item := range child.Capabilities {
			capabilities[capabilityIndex] = capability.Ref{Kind: capability.Kind(item.Kind), Name: item.Name, Version: item.Version, Config: item.Config, Optional: item.Optional}
		}
		children[index] = coordination.ChildWork{AgentID: child.AgentID, Title: child.Title, Prompt: child.Prompt, Priority: child.Priority, Workspace: child.Workspace, Capabilities: capabilities, CapabilitySelection: coordination.CapabilitySelection(child.CapabilitySelection)}
	}
	invocation := r.invocation(actor, issue, key, "delegate")
	return ManagerBoardCoordinator{Manager: r.manager}.Delegate(ctx, invocation, coordination.DelegationRequest{Children: children, ParentBehavior: request.ParentBehavior, ResultDelivery: request.ResultDelivery})
}

func (r controlBoardRepository) Continue(ctx context.Context, actor agentapp.Actor, childIssueID, reason, key string) error {
	issue, err := r.currentIssue(actor)
	if err != nil {
		return err
	}
	return ManagerBoardCoordinator{Manager: r.manager}.Continue(ctx, r.invocation(actor, issue, key, "continue"), coordination.ContinueRequest{ChildIssueID: childIssueID, Reason: reason})
}

func (r controlBoardRepository) Wait(ctx context.Context, actor agentapp.Actor, childIDs []string, wakeAfterSeconds int64, message, key string) error {
	issue, err := r.currentIssue(actor)
	if err != nil {
		return err
	}
	return ManagerBoardCoordinator{Manager: r.manager}.Wait(ctx, r.invocation(actor, issue, key, "sleep"), coordination.WaitRequest{ChildIDs: childIDs, WakeAfterSeconds: wakeAfterSeconds, Message: message})
}

func (r controlBoardRepository) currentIssue(actor agentapp.Actor) (Issue, error) {
	if r.manager == nil || r.manager.store == nil || strings.TrimSpace(actor.ExecutionID) == "" || strings.TrimSpace(actor.TaskID) == "" {
		return Issue{}, agentapp.ErrPermissionDenied
	}
	var execution Execution
	if err := r.manager.store.db.First(&execution, "id = ?", actor.ExecutionID).Error; err != nil {
		return Issue{}, agentapp.ErrPermissionDenied
	}
	issue, err := r.manager.store.GetIssue(execution.IssueID)
	if err != nil {
		return Issue{}, err
	}
	root, err := r.manager.store.taskRoot(issue)
	if err != nil || root.ID != actor.TaskID || issue.AssigneeAgentID != actor.AgentID || (actor.TaskAgentID != "" && issue.AssigneeTaskAgentID != actor.TaskAgentID) {
		return Issue{}, agentapp.ErrPermissionDenied
	}
	return issue, nil
}

func (r controlBoardRepository) invocation(actor agentapp.Actor, issue Issue, key, action string) coordination.Invocation {
	key = strings.TrimSpace(key)
	if key == "" {
		key = nextID("phone-shortcut")
	}
	return coordination.Invocation{EventID: fmt.Sprintf("phone-board:%s:%s:%s", actor.ExecutionID, action, key), IssueID: issue.ID, ExecutionID: actor.ExecutionID, AgentID: actor.AgentID}
}

func (r controlBoardRepository) AddComment(ctx context.Context, actor agentapp.Actor, issueID, body, _ string) (agentapp.BoardComment, error) {
	if _, _, err := r.GetIssue(ctx, actor, issueID); err != nil {
		return agentapp.BoardComment{}, err
	}
	comment := r.manager.addTypedAgentComment(issueID, actorRoutingID(actor), "normal", body, actor.ExecutionID, []commentWakeupTarget{})
	if comment == nil {
		return agentapp.BoardComment{}, agentapp.ErrValidation
	}
	r.manager.store.notify()
	return agentapp.BoardComment{ID: comment.ID, IssueID: comment.IssueID, AuthorID: comment.AuthorID, Body: comment.Body, CreatedAt: comment.CreatedAt}, nil
}

func (r controlBoardRepository) boardIssue(issue Issue) agentapp.BoardIssue {
	result := agentapp.BoardIssue{
		ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, Objective: issue.Objective,
		Status: issue.Status, WorkflowStatus: issue.Status, Priority: issue.Priority, AssigneeID: issue.AssigneeAgentID, AssigneeTaskAgentID: issue.AssigneeTaskAgentID,
		ExecutionPhase: issue.ExecutionPhase, Blocked: hasIssueLabel(issue, issueLabelBlocked) || issue.ExecutionPhase == "blocked", UpdatedAt: issue.UpdatedAt,
	}
	if views := r.manager.store.issueRuntimeViews([]Issue{issue}); len(views) == 1 {
		result.ExecutionPhase = views[0].State
	}
	if issue.AssigneeAgentID != "" {
		if agent, err := r.manager.store.GetAgent(issue.AssigneeAgentID); err == nil {
			result.AssigneeTypeName = agent.Name
		}
	}
	if issue.AssigneeTaskAgentID != "" {
		var identity TaskAgent
		if err := r.manager.store.db.First(&identity, "id = ?", issue.AssigneeTaskAgentID).Error; err == nil {
			result.AssigneeName = identity.Name
		}
	}
	return result
}

type controlRelayRepository struct {
	store *Store
	mu    sync.Mutex
	keys  map[string]agentapp.RelayMessage
}

func (r *controlRelayRepository) Inbox(_ context.Context, actor agentapp.Actor) ([]agentapp.RelayThread, error) {
	items, err := r.store.RelayInboxForTask(actorRoutingID(actor), actor.TaskID)
	if err != nil {
		return nil, err
	}
	result := make([]agentapp.RelayThread, len(items))
	for index, item := range items {
		result[index] = agentapp.RelayThread{ID: item.Thread.ID, Title: item.Thread.Title, ParticipantIDs: append([]string(nil), item.Thread.ParticipantIDs...), UpdatedAt: item.Thread.UpdatedAt}
	}
	return result, nil
}

func (r *controlRelayRepository) Messages(_ context.Context, actor agentapp.Actor, threadID string) ([]agentapp.RelayMessage, error) {
	conversation, err := r.store.RelayConversationForTask(actorRoutingID(actor), threadID, actor.TaskID, true)
	if err != nil {
		return nil, agentapp.ErrPermissionDenied
	}
	result := make([]agentapp.RelayMessage, len(conversation.Messages))
	for index, message := range conversation.Messages {
		result[index] = agentapp.RelayMessage{ID: message.ID, ThreadID: message.ThreadID, SenderID: message.SenderID, Body: message.Body, CreatedAt: message.CreatedAt}
	}
	return result, nil
}

func (r *controlRelayRepository) Send(_ context.Context, actor agentapp.Actor, threadID, body, key string) (agentapp.RelayMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if key != "" {
		key = actorRoutingID(actor) + "\x00" + threadID + "\x00" + key
		if existing, ok := r.keys[key]; ok {
			return existing, nil
		}
	}
	senderRoutingID := actorRoutingID(actor)
	conversation, err := r.store.RelayConversationForTask(senderRoutingID, threadID, actor.TaskID, false)
	if err != nil {
		return agentapp.RelayMessage{}, agentapp.ErrPermissionDenied
	}
	recipient := ""
	for _, id := range conversation.Thread.ParticipantIDs {
		if id != senderRoutingID {
			recipient = id
			break
		}
	}
	if recipient == "" {
		return agentapp.RelayMessage{}, agentapp.ErrValidation
	}
	input := SendRelayMessageInput{Body: body, TaskID: actor.TaskID, IssueID: actor.TaskID}
	if identity, identityErr := r.store.taskAgent(actor.TaskID, recipient); identityErr == nil {
		input.RecipientID = identity.AgentID
		input.RecipientTaskAgentID = identity.ID
	} else {
		input.RecipientID = recipient
	}
	message, err := r.store.SendRelayMessage("agent", senderRoutingID, input)
	if err != nil {
		return agentapp.RelayMessage{}, err
	}
	converted := agentapp.RelayMessage{ID: message.ID, ThreadID: message.ThreadID, SenderID: message.SenderID, Body: message.Body, CreatedAt: message.CreatedAt}
	if key != "" {
		r.keys[key] = converted
	}
	return converted, nil
}

// NewControlAgentPhone assembles Board and Relay over the authoritative
// control Store and persists Phone sessions/audit records in the same SQLite DB.
func NewControlAgentPhone(manager *Manager) (*agentapp.Phone, AgentPhoneClient, error) {
	if manager == nil || manager.store == nil {
		return nil, AgentPhoneClient{}, errors.New("control: manager is required")
	}
	registry := agentapp.NewRegistry()
	board := controlBoardRepository{manager: manager}
	if err := registry.Register(agentapp.NewCoordinatedBoardApp(board, board)); err != nil {
		return nil, AgentPhoneClient{}, err
	}
	relay := &controlRelayRepository{store: manager.store, keys: make(map[string]agentapp.RelayMessage)}
	if err := registry.Register(agentapp.NewRelayApp(relay)); err != nil {
		return nil, AgentPhoneClient{}, err
	}
	persistence, err := agentapp.NewGormStore(manager.store.db)
	if err != nil {
		return nil, AgentPhoneClient{}, err
	}
	phone := agentapp.NewPersistentPhone(registry, persistence, nil)
	return phone, AgentPhoneClient{Phone: phone}, nil
}

var _ agentapp.BoardRepository = controlBoardRepository{}
var _ agentapp.BoardCoordinator = controlBoardRepository{}
var _ agentapp.BoardIssueManager = controlBoardRepository{}
var _ agentapp.RelayRepository = (*controlRelayRepository)(nil)
