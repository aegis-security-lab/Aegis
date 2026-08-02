package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"aegis/agentapp"
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
	root, err := m.store.GetIssue(strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	root, err = m.store.taskRoot(root)
	if err != nil {
		return nil, err
	}
	if !m.store.db.Migrator().HasTable("agent_app_phone_sessions") {
		return []TaskPhoneView{}, nil
	}
	type row struct {
		ID, TaskID, TaskAgentID, AgentID, ActorJSON, InstalledJSON, ActiveAppID, CurrentJSON string
		CreatedAt, UpdatedAt                                                                 time.Time
	}
	var rows []row
	if err := m.store.db.WithContext(ctx).Table("agent_app_phone_sessions").Where("task_id = ?", root.ID).Order("created_at asc").Find(&rows).Error; err != nil {
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
		view := TaskPhoneView{ID: item.ID, TaskID: root.ID, TaskAgentID: item.TaskAgentID, AgentID: item.AgentID, ExecutionID: actor.ExecutionID, WorkspaceID: actor.WorkspaceID, InstalledApps: installed, ActiveAppID: item.ActiveAppID, CurrentAppID: current.Page.AppID, CurrentPageID: current.Page.PageID, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
		if item.TaskAgentID != "" {
			var identity TaskAgent
			if identityErr := m.store.db.First(&identity, "id = ? AND task_id = ?", item.TaskAgentID, root.ID).Error; identityErr == nil {
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

type controlBoardRepository struct{ manager *Manager }

func (r controlBoardRepository) ListIssues(_ context.Context, actor agentapp.Actor, query string) ([]agentapp.BoardIssue, error) {
	if r.manager == nil || r.manager.store == nil {
		return nil, errors.New("control: Board store is unavailable")
	}
	if strings.TrimSpace(actor.TaskID) == "" {
		return nil, agentapp.ErrPermissionDenied
	}
	var issues []Issue
	db := r.manager.store.db.Where("hidden = ?", false).Order("updated_at desc")
	if !actor.HasScope("board:read:all") {
		if strings.TrimSpace(actor.TaskAgentID) != "" {
			db = db.Where("assignee_task_agent_id = ?", actor.TaskAgentID)
		} else {
			db = db.Where("assignee_agent_id = ?", actor.AgentID)
		}
	}
	if err := db.Find(&issues).Error; err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]agentapp.BoardIssue, 0, len(issues))
	for _, issue := range issues {
		root, rootErr := r.manager.store.taskRoot(issue)
		if rootErr != nil || root.ID != actor.TaskID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(issue.Identifier+" "+issue.Title+" "+issue.Objective), query) {
			continue
		}
		result = append(result, boardIssue(issue))
	}
	return result, nil
}

func (r controlBoardRepository) GetIssue(_ context.Context, actor agentapp.Actor, id string) (agentapp.BoardIssue, []agentapp.BoardComment, error) {
	detail, err := r.manager.store.GetIssueDetail(id)
	if err != nil {
		return agentapp.BoardIssue{}, nil, err
	}
	root, rootErr := r.manager.store.taskRoot(detail.Issue)
	if rootErr != nil || strings.TrimSpace(actor.TaskID) == "" || root.ID != actor.TaskID {
		return agentapp.BoardIssue{}, nil, agentapp.ErrPermissionDenied
	}
	assigned := detail.Issue.AssigneeAgentID == actor.AgentID
	if strings.TrimSpace(actor.TaskAgentID) != "" {
		assigned = detail.Issue.AssigneeTaskAgentID == actor.TaskAgentID
	}
	if !assigned && !actor.HasScope("board:read:all") {
		return agentapp.BoardIssue{}, nil, agentapp.ErrPermissionDenied
	}
	comments := make([]agentapp.BoardComment, len(detail.Comments))
	for index, comment := range detail.Comments {
		comments[index] = agentapp.BoardComment{ID: comment.ID, IssueID: comment.IssueID, AuthorID: comment.AuthorID, Body: comment.Body, CreatedAt: comment.CreatedAt}
	}
	return boardIssue(detail.Issue), comments, nil
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

func boardIssue(issue Issue) agentapp.BoardIssue {
	return agentapp.BoardIssue{
		ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, Objective: issue.Objective,
		Status: issue.Status, Priority: issue.Priority, AssigneeID: issue.AssigneeAgentID, AssigneeTaskAgentID: issue.AssigneeTaskAgentID,
		Blocked: issue.Status == "blocked" || issue.ExecutionPhase == "blocked", UpdatedAt: issue.UpdatedAt,
	}
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
	if err := registry.Register(agentapp.NewBoardApp(controlBoardRepository{manager: manager})); err != nil {
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
var _ agentapp.RelayRepository = (*controlRelayRepository)(nil)
