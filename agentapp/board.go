package agentapp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type BoardIssue struct {
	ID                  string    `json:"id"`
	Identifier          string    `json:"identifier"`
	Title               string    `json:"title"`
	Objective           string    `json:"objective"`
	Status              string    `json:"status"`
	WorkflowStatus      string    `json:"workflowStatus"`
	Priority            string    `json:"priority"`
	AssigneeID          string    `json:"assigneeId"`
	AssigneeTaskAgentID string    `json:"assigneeTaskAgentId,omitempty"`
	AssigneeName        string    `json:"assigneeName,omitempty"`
	AssigneeTypeName    string    `json:"assigneeTypeName,omitempty"`
	ExecutionPhase      string    `json:"executionPhase,omitempty"`
	Blocked             bool      `json:"blocked"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type BoardComment struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issueId"`
	AuthorID  string    `json:"authorId"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type BoardRepository interface {
	ListIssues(context.Context, Actor, string) ([]BoardIssue, error)
	GetIssue(context.Context, Actor, string) (BoardIssue, []BoardComment, error)
	AddComment(context.Context, Actor, string, string, string) (BoardComment, error)
}

type BoardCapability struct {
	Kind     string         `json:"kind"`
	Name     string         `json:"name"`
	Version  string         `json:"version,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	Optional bool           `json:"optional,omitempty"`
}

type BoardChildWork struct {
	AgentID             string            `json:"agentId"`
	Title               string            `json:"title,omitempty"`
	Prompt              string            `json:"prompt"`
	Priority            string            `json:"priority,omitempty"`
	Workspace           string            `json:"workspace,omitempty"`
	CapabilitySelection string            `json:"capabilitySelection,omitempty"`
	Capabilities        []BoardCapability `json:"capabilities,omitempty"`
}

type BoardDelegationRequest struct {
	Children       []BoardChildWork `json:"children"`
	ParentBehavior string           `json:"parentBehavior,omitempty"`
	ResultDelivery string           `json:"resultDelivery,omitempty"`
}

type BoardCoordinator interface {
	Delegate(context.Context, Actor, BoardDelegationRequest, string) error
	Continue(context.Context, Actor, string, string, string) error
	Wait(context.Context, Actor, []string, int64, string, string) error
}

// BoardIssueManager exposes durable Issue lifecycle operations independently
// from delegation. In particular, an Issue may be created without an assignee
// and remain in todo until a later assignment claims a task-local Agent.
type BoardIssueManager interface {
	CreateIssue(context.Context, Actor, BoardCreateIssueRequest, string) (BoardIssue, error)
	UpdateIssue(context.Context, Actor, BoardUpdateIssueRequest, string) (BoardIssue, error)
	DeleteIssue(context.Context, Actor, string, string) (int64, error)
}

type BoardCreateIssueRequest struct {
	ParentIssueID   string `json:"parentIssueId,omitempty"`
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	Objective       string `json:"objective,omitempty"`
	Priority        string `json:"priority,omitempty"`
	Status          string `json:"status,omitempty"`
	AssigneeAgentID string `json:"assigneeAgentId,omitempty"`
}

type BoardUpdateIssueRequest struct {
	IssueID         string  `json:"issueId"`
	Title           *string `json:"title,omitempty"`
	Description     *string `json:"description,omitempty"`
	Objective       *string `json:"objective,omitempty"`
	Priority        *string `json:"priority,omitempty"`
	Status          *string `json:"status,omitempty"`
	AssigneeAgentID *string `json:"assigneeAgentId,omitempty"`
	Reason          string  `json:"reason,omitempty"`
}

type BoardApp struct {
	Repository  BoardRepository
	Coordinator BoardCoordinator
	Manager     BoardIssueManager
}

func NewBoardApp(repository BoardRepository) *BoardApp {
	app := &BoardApp{Repository: repository}
	app.Manager, _ = repository.(BoardIssueManager)
	return app
}

func NewCoordinatedBoardApp(repository BoardRepository, coordinator BoardCoordinator) *BoardApp {
	app := &BoardApp{Repository: repository, Coordinator: coordinator}
	app.Manager, _ = repository.(BoardIssueManager)
	return app
}

func (a *BoardApp) Manifest() Manifest {
	return Manifest{AppID: "aegis.board", Name: "Board", Version: "1.0.0", Description: "Tasks, issues, evidence and reviews", Web: WebManifest{Entry: "/apps/aegis.board"}, AI: AIManifest{ProtocolVersion: ProtocolVersion, Entry: "/apps/aegis.board/ai/pages/home", Actions: "/phone/actions", ContentTypes: []string{"text/agent-ui", "application/agent-ui+json"}}, Capabilities: []string{ActionClick, ActionInput, ActionSubmit, ActionBack, ActionHome, ActionRefresh, ActionScroll, ActionSwipe, ActionLongPress, ActionLoadMore}}
}

func (a *BoardApp) Render(ctx context.Context, request RenderRequest) (Page, error) {
	switch request.Route {
	case "", "home":
		return a.home(ctx, request)
	case "issue":
		return a.issue(ctx, request)
	case "issue_menu":
		return a.issueMenu(ctx, request)
	case "new_issue":
		return a.newIssue(request), nil
	case "sleep":
		return a.sleep(request), nil
	default:
		return Page{}, fmt.Errorf("unknown Board route %q", request.Route)
	}
}

func (a *BoardApp) home(ctx context.Context, request RenderRequest) (Page, error) {
	query, _ := request.Draft["query"].(string)
	issues, err := a.Repository.ListIssues(ctx, request.Actor, query)
	if err != nil {
		return Page{}, err
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Blocked != issues[j].Blocked {
			return issues[i].Blocked
		}
		return issues[i].UpdatedAt.After(issues[j].UpdatedAt)
	})
	totalBlocked := countBlocked(issues)
	visibleLimit := 5
	if parsed, parseErr := strconv.Atoi(request.Params["limit"]); parseErr == nil && parsed > 0 {
		visibleLimit = parsed
	}
	totalIssues := len(issues)
	if len(issues) > visibleLimit {
		issues = issues[:visibleLimit]
	}
	page := Page{PageID: "board.home", Title: "Task Issues", Summary: fmt.Sprintf("Showing: %d of %d · Needs attention: %d", len(issues), totalIssues, totalBlocked)}
	attention := Section{Title: "Needs attention"}
	other := Section{Title: "Issues"}
	for index, issue := range issues {
		ref := fmt.Sprintf("@%d", index+1)
		line := Line{Ref: ref, Kind: "issue", Label: fmt.Sprintf("%s · %s · %s · %s", issue.Identifier, issue.Title, boardWorkflowStatus(issue), issue.Priority), Detail: "Owner: " + boardOwnerLabel(issue) + boardExecutionLabel(issue)}
		if issue.Blocked {
			attention.Lines = append(attention.Lines, line)
		} else {
			other.Lines = append(other.Lines, line)
		}
		page.Refs = append(page.Refs, Ref{Ref: ref, Kind: "issue", Label: issue.Identifier + " " + issue.Title, Actions: []string{ActionClick, ActionLongPress}, Target: "issue:" + issue.ID})
	}
	if totalIssues > len(issues) {
		viewportRef := fmt.Sprintf("@%d", len(page.Refs)+1)
		other.Lines = append(other.Lines, Line{Ref: viewportRef, Kind: "viewport", Label: fmt.Sprintf("Load more issues · %d remaining", totalIssues-len(issues)), Detail: "Actions: scroll down, load_more"})
		page.Refs = append(page.Refs, Ref{Ref: viewportRef, Kind: "viewport", Label: "More issues", Actions: []string{ActionScroll, ActionLoadMore}, Target: "issues-viewport", Metadata: map[string]string{"remaining": strconv.Itoa(totalIssues - len(issues))}})
	}
	inputRef := fmt.Sprintf("@%d", len(page.Refs)+1)
	submitRef := fmt.Sprintf("@%d", len(page.Refs)+2)
	actions := Section{Title: "Actions", Lines: []Line{{Ref: inputRef, Kind: "input:text name=query", Label: "Search issues", Detail: fmt.Sprintf("Current value: %q", query)}, {Ref: submitRef, Kind: "button", Label: "Submit search"}}}
	page.Refs = append(page.Refs, Ref{Ref: inputRef, Kind: "input", Label: "Search issues", Actions: []string{ActionInput}, Target: "search-input", Field: "query", InputType: "text"}, Ref{Ref: submitRef, Kind: "button", Label: "Submit search", Actions: []string{ActionSubmit}, Target: "search-submit"})
	if a.Coordinator != nil || a.Manager != nil {
		newRef := fmt.Sprintf("@%d", len(page.Refs)+1)
		label, detail := "Delegate child Issue", "Create and assign one focused child"
		if a.Manager != nil {
			label, detail = "Create child Issue", "Assignee is optional; unassigned work remains in todo"
		}
		actions.Lines = append(actions.Lines, Line{Ref: newRef, Kind: "button", Label: label, Detail: detail})
		page.Refs = append(page.Refs, Ref{Ref: newRef, Kind: "button", Label: label, Actions: []string{ActionClick}, Target: "new_issue"})
		if a.Coordinator != nil {
			sleepRef := fmt.Sprintf("@%d", len(page.Refs)+1)
			actions.Lines = append(actions.Lines, Line{Ref: sleepRef, Kind: "button", Label: "Sleep until an event", Detail: "Release this Agent loop durably"})
			page.Refs = append(page.Refs, Ref{Ref: sleepRef, Kind: "button", Label: "Sleep", Actions: []string{ActionClick}, Target: "sleep"})
		}
	}
	if len(attention.Lines) > 0 {
		page.Sections = append(page.Sections, attention)
	}
	page.Sections = append(page.Sections, other, actions)
	return page, nil
}

func (a *BoardApp) issue(ctx context.Context, request RenderRequest) (Page, error) {
	issue, comments, err := a.Repository.GetIssue(ctx, request.Actor, request.Params["id"])
	if err != nil {
		return Page{}, err
	}
	page := Page{PageID: "issue.detail", Title: issue.Identifier + " " + issue.Title, Summary: fmt.Sprintf("%s · %s · owner %s%s", boardWorkflowStatus(issue), issue.Priority, boardOwnerLabel(issue), boardExecutionLabel(issue))}
	page.Sections = append(page.Sections, Section{Title: "Objective", Lines: []Line{{Label: issue.Objective}}})
	commentSection := Section{Title: "Recent comments"}
	for _, comment := range comments {
		commentSection.Lines = append(commentSection.Lines, Line{Kind: "comment", Label: comment.AuthorID + ": " + comment.Body, Detail: comment.CreatedAt.UTC().Format(time.RFC3339)})
	}
	if len(commentSection.Lines) == 0 {
		commentSection.Lines = append(commentSection.Lines, Line{Label: "No comments"})
	}
	page.Sections = append(page.Sections, commentSection)
	body, _ := request.Draft["comment"].(string)
	actions := []Line{{Ref: "@1", Kind: "input:text name=comment", Label: "Comment", Detail: fmt.Sprintf("Current value: %q", body)}, {Ref: "@2", Kind: "button", Label: "Post comment"}}
	refs := []Ref{{Ref: "@1", Kind: "input", Label: "Comment", Actions: []string{ActionInput}, Target: "comment-input", Field: "comment", InputType: "text"}, {Ref: "@2", Kind: "button", Label: "Post comment", Actions: []string{ActionSubmit}, Target: "comment-submit"}}
	if a.Coordinator != nil || a.Manager != nil {
		actions = append(actions, Line{Ref: "@3", Kind: "button", Label: "Issue actions", Detail: "Update workflow state, delete stopped work, or continue a failed execution"})
		refs = append(refs, Ref{Ref: "@3", Kind: "button", Label: "Issue actions", Actions: []string{ActionClick}, Target: "issue_actions:" + issue.ID})
	}
	page.Sections = append(page.Sections, Section{Title: "Actions", Lines: actions})
	page.Refs = refs
	return page, nil
}

func (a *BoardApp) issueMenu(ctx context.Context, request RenderRequest) (Page, error) {
	issue, _, err := a.Repository.GetIssue(ctx, request.Actor, request.Params["id"])
	if err != nil {
		return Page{}, err
	}
	lines := []Line{{Ref: "@1", Kind: "button", Label: "Open issue details", Detail: issue.Title}}
	refs := []Ref{{Ref: "@1", Kind: "button", Label: "Open issue details", Actions: []string{ActionClick}, Target: "open_issue:" + issue.ID}}
	if a.Coordinator != nil && (issue.Status == "failed" || issue.Status == "budget_exceeded") {
		lines = append(lines, Line{Ref: "@2", Kind: "input:text name=continueReason", Label: "Continuation reason"}, Line{Ref: "@3", Kind: "button", Label: "Continue Issue", Detail: "Starts a fresh Execution budget"})
		refs = append(refs, Ref{Ref: "@2", Kind: "input", Label: "Continuation reason", Actions: []string{ActionInput}, Target: "continue-reason", Field: "continueReason", InputType: "text"}, Ref{Ref: "@3", Kind: "button", Label: "Continue Issue", Actions: []string{ActionSubmit}, Target: "continue:" + issue.ID})
	}
	if a.Manager != nil {
		base := len(refs) + 1
		lines = append(lines,
			Line{Ref: fmt.Sprintf("@%d", base), Kind: "input:text name=status", Label: "Set status", Detail: "todo/in_progress/in_review/done/cancelled"},
			Line{Ref: fmt.Sprintf("@%d", base+1), Kind: "button", Label: "Apply status"},
			Line{Ref: fmt.Sprintf("@%d", base+2), Kind: "button", Label: "Delete Issue permanently", Detail: "Requires all work in this Issue subtree to be stopped"},
		)
		refs = append(refs,
			Ref{Ref: fmt.Sprintf("@%d", base), Kind: "input", Label: "Status", Actions: []string{ActionInput}, Target: "issue-status", Field: "status", InputType: "text"},
			Ref{Ref: fmt.Sprintf("@%d", base+1), Kind: "button", Label: "Apply status", Actions: []string{ActionSubmit}, Target: "set_status:" + issue.ID},
			Ref{Ref: fmt.Sprintf("@%d", base+2), Kind: "button", Label: "Delete Issue", Actions: []string{ActionClick}, Target: "delete_issue:" + issue.ID},
		)
	}
	return Page{PageID: "issue.menu", Title: issue.Identifier + " Actions", Summary: "Opened with long_press · " + boardWorkflowStatus(issue) + " · " + issue.Priority + boardExecutionLabel(issue), Sections: []Section{{Title: "Issue actions", Lines: lines}}, Refs: refs}, nil
}

func (a *BoardApp) newIssue(request RenderRequest) Page {
	draft := request.Draft
	title, summary, button, target := "Delegate child Issue", "Create one small, independently verifiable unit of work", "Create and assign", "delegate-submit"
	if a.Manager != nil {
		title, summary, button, target = "Create child Issue", "Create one focused Issue; leave Agent type empty to keep it unassigned in todo", "Create Issue", "create-issue-submit"
	}
	return Page{PageID: "issue.new", Title: title, Summary: summary, Sections: []Section{{Title: "Assignment", Lines: []Line{
		{Ref: "@1", Kind: "input:text name=agentId", Label: "Agent type (optional)", Detail: draftText(draft, "agentId")},
		{Ref: "@2", Kind: "input:text name=title", Label: "Title", Detail: draftText(draft, "title")},
		{Ref: "@3", Kind: "input:text name=prompt", Label: "Objective and acceptance evidence", Detail: draftText(draft, "prompt")},
		{Ref: "@4", Kind: "input:text name=workspace", Label: "Workspace override (optional)", Detail: draftText(draft, "workspace")},
		{Ref: "@5", Kind: "button", Label: button},
	}}}, Refs: []Ref{
		{Ref: "@1", Kind: "input", Label: "Agent type", Actions: []string{ActionInput}, Target: "delegate-agent", Field: "agentId", InputType: "text"},
		{Ref: "@2", Kind: "input", Label: "Title", Actions: []string{ActionInput}, Target: "delegate-title", Field: "title", InputType: "text"},
		{Ref: "@3", Kind: "input", Label: "Objective", Actions: []string{ActionInput}, Target: "delegate-prompt", Field: "prompt", InputType: "text"},
		{Ref: "@4", Kind: "input", Label: "Workspace", Actions: []string{ActionInput}, Target: "delegate-workspace", Field: "workspace", InputType: "text"},
		{Ref: "@5", Kind: "button", Label: button, Actions: []string{ActionSubmit}, Target: target},
	}}
}

func (a *BoardApp) sleep(request RenderRequest) Page {
	return Page{PageID: "board.sleep", Title: "Sleep Agent", Summary: "The next task event or timer will start a new turn", Sections: []Section{{Title: "Wake condition", Lines: []Line{
		{Ref: "@1", Kind: "input:number name=wakeAfterSeconds", Label: "Wake after seconds", Detail: draftText(request.Draft, "wakeAfterSeconds")},
		{Ref: "@2", Kind: "input:text name=sleepMessage", Label: "Reason (optional)", Detail: draftText(request.Draft, "sleepMessage")},
		{Ref: "@3", Kind: "button", Label: "Sleep now"},
	}}}, Refs: []Ref{
		{Ref: "@1", Kind: "input", Label: "Wake after seconds", Actions: []string{ActionInput}, Target: "sleep-seconds", Field: "wakeAfterSeconds", InputType: "number"},
		{Ref: "@2", Kind: "input", Label: "Reason", Actions: []string{ActionInput}, Target: "sleep-message", Field: "sleepMessage", InputType: "text"},
		{Ref: "@3", Kind: "button", Label: "Sleep now", Actions: []string{ActionSubmit}, Target: "sleep-submit"},
	}}
}

func (a *BoardApp) Execute(ctx context.Context, command Command) (CommandResult, error) {
	if command.Action == ActionClick && strings.HasPrefix(command.Target, "issue:") {
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "issue", Params: map[string]string{"id": strings.TrimPrefix(command.Target, "issue:")}}, Effect: "navigated"}, nil
	}
	if command.Action == ActionClick && strings.HasPrefix(command.Target, "open_issue:") {
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "issue", Params: map[string]string{"id": strings.TrimPrefix(command.Target, "open_issue:")}}, Effect: "navigated"}, nil
	}
	if command.Action == ActionLongPress && strings.HasPrefix(command.Target, "issue:") {
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "issue_menu", Params: map[string]string{"id": strings.TrimPrefix(command.Target, "issue:")}}, Effect: "context_opened"}, nil
	}
	if command.Action == ActionClick && strings.HasPrefix(command.Target, "issue_actions:") {
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "issue_menu", Params: map[string]string{"id": strings.TrimPrefix(command.Target, "issue_actions:")}}, Effect: "navigated"}, nil
	}
	if command.Action == ActionClick && command.Target == "new_issue" {
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "new_issue"}, Effect: "navigated"}, nil
	}
	if command.Action == ActionClick && command.Target == "sleep" {
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "sleep"}, Effect: "navigated"}, nil
	}
	if (command.Action == ActionScroll || command.Action == ActionLoadMore) && command.Target == "issues-viewport" {
		limit := 5
		if parsed, err := strconv.Atoi(command.Location.Params["limit"]); err == nil && parsed > 0 {
			limit = parsed
		}
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "home", Params: map[string]string{"limit": strconv.Itoa(limit + 5)}}, Effect: "content_extended"}, nil
	}
	if command.Action == ActionInput {
		return CommandResult{Effect: "draft_updated", Draft: command.Draft}, nil
	}
	if command.Action == ActionSubmit && command.Target == "search-submit" {
		return CommandResult{Effect: "replaced", Location: &Location{AppID: "aegis.board", Route: "home"}, Draft: command.Draft}, nil
	}
	if command.Action == ActionSubmit && command.Target == "comment-submit" {
		body, _ := command.Draft["comment"].(string)
		body = strings.TrimSpace(body)
		if body == "" {
			return CommandResult{}, ErrValidation
		}
		_, err := a.Repository.AddComment(ctx, command.Actor, command.Location.Params["id"], body, command.IdempotencyKey)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Effect: "replaced", Location: &command.Location, Toast: "Comment posted", Draft: map[string]any{}}, nil
	}
	if command.Action == ActionSubmit && command.Target == "delegate-submit" && a.Coordinator != nil {
		request := BoardDelegationRequest{Children: []BoardChildWork{{AgentID: stringValue(command.Draft, "agentId"), Title: stringValue(command.Draft, "title"), Prompt: stringValue(command.Draft, "prompt"), Workspace: stringValue(command.Draft, "workspace")}}}
		if request.Children[0].AgentID == "" || request.Children[0].Prompt == "" {
			return CommandResult{}, ErrValidation
		}
		if err := a.Coordinator.Delegate(ctx, command.Actor, request, command.IdempotencyKey); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Effect: "replaced", Location: &Location{AppID: "aegis.board", Route: "home"}, Toast: "Child Issue created and assigned", Draft: map[string]any{}}, nil
	}
	if command.Action == ActionSubmit && command.Target == "create-issue-submit" && a.Manager != nil {
		input := BoardCreateIssueRequest{Title: stringValue(command.Draft, "title"), Objective: stringValue(command.Draft, "prompt"), AssigneeAgentID: stringValue(command.Draft, "agentId")}
		if input.Title == "" || input.Objective == "" {
			return CommandResult{}, ErrValidation
		}
		created, err := a.Manager.CreateIssue(ctx, command.Actor, input, command.IdempotencyKey)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Effect: "created", Location: &Location{AppID: "aegis.board", Route: "issue", Params: map[string]string{"id": created.ID}}, Toast: "Issue created", Draft: map[string]any{}}, nil
	}
	if command.Action == ActionSubmit && strings.HasPrefix(command.Target, "set_status:") && a.Manager != nil {
		id, status := strings.TrimPrefix(command.Target, "set_status:"), stringValue(command.Draft, "status")
		if status == "" {
			return CommandResult{}, ErrValidation
		}
		_, err := a.Manager.UpdateIssue(ctx, command.Actor, BoardUpdateIssueRequest{IssueID: id, Status: &status, Reason: "Agent 通过 Phone Board 更新了 Issue 状态"}, command.IdempotencyKey)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Effect: "updated", Location: &Location{AppID: "aegis.board", Route: "issue", Params: map[string]string{"id": id}}, Toast: "Issue status updated", Draft: map[string]any{}}, nil
	}
	if command.Action == ActionClick && strings.HasPrefix(command.Target, "delete_issue:") && a.Manager != nil {
		id := strings.TrimPrefix(command.Target, "delete_issue:")
		if _, err := a.Manager.DeleteIssue(ctx, command.Actor, id, command.IdempotencyKey); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Effect: "deleted", Location: &Location{AppID: "aegis.board", Route: "home"}, Toast: "Issue deleted"}, nil
	}
	if command.Action == ActionSubmit && strings.HasPrefix(command.Target, "continue:") && a.Coordinator != nil {
		reason := stringValue(command.Draft, "continueReason")
		if reason == "" {
			return CommandResult{}, ErrValidation
		}
		if err := a.Coordinator.Continue(ctx, command.Actor, strings.TrimPrefix(command.Target, "continue:"), reason, command.IdempotencyKey); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Effect: "replaced", Location: &Location{AppID: "aegis.board", Route: "issue", Params: map[string]string{"id": strings.TrimPrefix(command.Target, "continue:")}}, Toast: "Issue continued with a fresh Execution budget", Draft: map[string]any{}}, nil
	}
	if command.Action == ActionSubmit && command.Target == "sleep-submit" && a.Coordinator != nil {
		seconds, err := int64Value(command.Draft, "wakeAfterSeconds")
		if err != nil || seconds < 1 {
			return CommandResult{}, ErrValidation
		}
		if err := a.Coordinator.Wait(ctx, command.Actor, nil, seconds, stringValue(command.Draft, "sleepMessage"), command.IdempotencyKey); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Effect: "replaced", Location: &Location{AppID: "aegis.board", Route: "home"}, Toast: "Sleep scheduled", Draft: map[string]any{}, Terminate: true}, nil
	}
	return CommandResult{}, ErrActionNotAllowed
}

func (a *BoardApp) Shortcuts() []ShortcutDefinition {
	items := []ShortcutDefinition{
		{Name: "phone_board_list_issues", Description: "List or search Issues in this task through the Phone Board.", Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"additionalProperties":false}`), Frequency: 100},
		{Name: "phone_board_get_issue", Description: "Open one Issue and read its objective, status and recent comments through the Phone Board.", Parameters: json.RawMessage(`{"type":"object","properties":{"issueId":{"type":"string","minLength":1}},"required":["issueId"],"additionalProperties":false}`), Frequency: 99},
		{Name: "phone_board_comment_issue", Description: "Post a durable comment to an Issue through the Phone Board.", Parameters: json.RawMessage(`{"type":"object","properties":{"issueId":{"type":"string","minLength":1},"body":{"type":"string","minLength":1}},"required":["issueId","body"],"additionalProperties":false}`), Frequency: 96},
	}
	if a.Coordinator != nil {
		items = append(items,
			ShortcutDefinition{Name: "phone_board_delegate", Description: "Create and assign one or more child Issues through the Phone Board while the parent continues working.", Parameters: delegateShortcutSchema, Frequency: 98},
			ShortcutDefinition{Name: "phone_board_continue_issue", Description: "Continue a direct failed or budget-exceeded child Issue with a fresh Execution budget through the Phone Board.", Parameters: json.RawMessage(`{"type":"object","properties":{"childIssueId":{"type":"string","minLength":1},"reason":{"type":"string","minLength":1}},"required":["childIssueId","reason"],"additionalProperties":false}`), Frequency: 94},
			ShortcutDefinition{Name: "phone_board_sleep", Description: "Release the current Agent loop and schedule a durable wakeup through the Phone Board.", Parameters: json.RawMessage(`{"type":"object","properties":{"childIds":{"type":"array","items":{"type":"string"}},"wakeAfterSeconds":{"type":"integer","minimum":1},"message":{"type":"string"}},"required":["wakeAfterSeconds"],"additionalProperties":false}`), Frequency: 80, Terminates: true},
		)
	}
	if a.Manager != nil {
		items = append(items,
			ShortcutDefinition{Name: "phone_board_create_issue", Description: "Create one child Issue. assigneeAgentId is optional; omit it to leave the Issue unassigned in todo.", Parameters: createIssueShortcutSchema, Frequency: 97},
			ShortcutDefinition{Name: "phone_board_update_issue", Description: "Update an Issue's fields, status, or assignee. Set assigneeAgentId to an empty string to unassign it; cancelled stops its active subtree.", Parameters: updateIssueShortcutSchema, Frequency: 95},
			ShortcutDefinition{Name: "phone_board_delete_issue", Description: "Permanently delete an Issue subtree after all of its work has stopped. Use status=cancelled first when it is active.", Parameters: json.RawMessage(`{"type":"object","properties":{"issueId":{"type":"string","minLength":1}} ,"required":["issueId"],"additionalProperties":false}`), Frequency: 70},
		)
	}
	return items
}

func (a *BoardApp) ExecuteShortcut(ctx context.Context, command ShortcutCommand) (CommandResult, error) {
	switch command.Name {
	case "phone_board_list_issues":
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "home"}, Effect: "listed", Draft: map[string]any{"query": stringValue(command.Arguments, "query")}}, nil
	case "phone_board_get_issue":
		id := stringValue(command.Arguments, "issueId")
		if _, _, err := a.Repository.GetIssue(ctx, command.Actor, id); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "issue", Params: map[string]string{"id": id}}, Effect: "opened"}, nil
	case "phone_board_comment_issue":
		id, body := stringValue(command.Arguments, "issueId"), stringValue(command.Arguments, "body")
		if id == "" || body == "" {
			return CommandResult{}, ErrValidation
		}
		if _, err := a.Repository.AddComment(ctx, command.Actor, id, body, command.IdempotencyKey); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "issue", Params: map[string]string{"id": id}}, Effect: "commented", Toast: "Comment posted"}, nil
	case "phone_board_create_issue":
		var input BoardCreateIssueRequest
		if a.Manager == nil || decodeArguments(command.Arguments, &input) != nil || strings.TrimSpace(input.Title) == "" {
			return CommandResult{}, ErrValidation
		}
		created, err := a.Manager.CreateIssue(ctx, command.Actor, input, command.IdempotencyKey)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "issue", Params: map[string]string{"id": created.ID}}, Effect: "created", Toast: "Issue created"}, nil
	case "phone_board_update_issue":
		var input BoardUpdateIssueRequest
		if a.Manager == nil || decodeArguments(command.Arguments, &input) != nil || strings.TrimSpace(input.IssueID) == "" {
			return CommandResult{}, ErrValidation
		}
		updated, err := a.Manager.UpdateIssue(ctx, command.Actor, input, command.IdempotencyKey)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "issue", Params: map[string]string{"id": updated.ID}}, Effect: "updated", Toast: "Issue updated"}, nil
	case "phone_board_delete_issue":
		if a.Manager == nil {
			return CommandResult{}, ErrActionNotAllowed
		}
		id := stringValue(command.Arguments, "issueId")
		if id == "" {
			return CommandResult{}, ErrValidation
		}
		count, err := a.Manager.DeleteIssue(ctx, command.Actor, id, command.IdempotencyKey)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "home"}, Effect: "deleted", Toast: fmt.Sprintf("%d Issue(s) deleted", count)}, nil
	case "phone_board_delegate":
		var request BoardDelegationRequest
		if a.Coordinator == nil || decodeArguments(command.Arguments, &request) != nil || len(request.Children) == 0 {
			return CommandResult{}, ErrValidation
		}
		if err := a.Coordinator.Delegate(ctx, command.Actor, request, command.IdempotencyKey); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "home"}, Effect: "delegated", Toast: fmt.Sprintf("%d child Issues created and assigned", len(request.Children))}, nil
	case "phone_board_continue_issue":
		if a.Coordinator == nil {
			return CommandResult{}, ErrActionNotAllowed
		}
		id, reason := stringValue(command.Arguments, "childIssueId"), stringValue(command.Arguments, "reason")
		if id == "" || reason == "" {
			return CommandResult{}, ErrValidation
		}
		if err := a.Coordinator.Continue(ctx, command.Actor, id, reason, command.IdempotencyKey); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "issue", Params: map[string]string{"id": id}}, Effect: "continued", Toast: "Issue continued with a fresh Execution budget"}, nil
	case "phone_board_sleep":
		if a.Coordinator == nil {
			return CommandResult{}, ErrActionNotAllowed
		}
		var input struct {
			ChildIDs []string `json:"childIds"`
			Seconds  int64    `json:"wakeAfterSeconds"`
			Message  string   `json:"message"`
		}
		if decodeArguments(command.Arguments, &input) != nil || input.Seconds < 1 {
			return CommandResult{}, ErrValidation
		}
		if err := a.Coordinator.Wait(ctx, command.Actor, input.ChildIDs, input.Seconds, input.Message, command.IdempotencyKey); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Location: &Location{AppID: "aegis.board", Route: "home"}, Effect: "sleeping", Toast: "Sleep scheduled", Terminate: true}, nil
	default:
		return CommandResult{}, ErrActionNotAllowed
	}
}

var delegateShortcutSchema = json.RawMessage(`{
  "type":"object","properties":{"children":{"type":"array","minItems":1,"items":{"type":"object","properties":{"agentId":{"type":"string","minLength":1},"title":{"type":"string"},"prompt":{"type":"string","minLength":1},"priority":{"type":"string","enum":["high","middle","low"]},"workspace":{"type":"string"},"capabilitySelection":{"type":"string","enum":["inherit","merge","replace"]},"capabilities":{"type":"array","maxItems":64,"items":{"type":"object","properties":{"kind":{"type":"string","minLength":1},"name":{"type":"string","minLength":1},"version":{"type":"string"},"config":{"type":"object"},"optional":{"type":"boolean"}},"required":["kind","name"],"additionalProperties":false}}},"required":["agentId","prompt"],"additionalProperties":false}},"parentBehavior":{"type":"string"},"resultDelivery":{"type":"string"}},"required":["children"],"additionalProperties":false
}`)

var createIssueShortcutSchema = json.RawMessage(`{
  "type":"object","properties":{"parentIssueId":{"type":"string"},"title":{"type":"string","minLength":1},"description":{"type":"string"},"objective":{"type":"string"},"priority":{"type":"string","enum":["high","middle","low"]},"status":{"type":"string","enum":["todo"]},"assigneeAgentId":{"type":"string"}},"required":["title"],"additionalProperties":false
}`)

var updateIssueShortcutSchema = json.RawMessage(`{
  "type":"object","properties":{"issueId":{"type":"string","minLength":1},"title":{"type":"string","minLength":1},"description":{"type":"string"},"objective":{"type":"string"},"priority":{"type":"string","enum":["high","middle","low"]},"status":{"type":"string","enum":["todo","in_progress","in_review","done","cancelled"]},"assigneeAgentId":{"type":"string"},"reason":{"type":"string"}},"required":["issueId"],"additionalProperties":false
}`)

func boardOwnerLabel(issue BoardIssue) string {
	if strings.TrimSpace(issue.AssigneeID) == "" {
		return "Unassigned"
	}
	name := strings.TrimSpace(issue.AssigneeName)
	if name == "" {
		name = issue.AssigneeID
	}
	typeName := strings.TrimSpace(issue.AssigneeTypeName)
	if typeName == "" || typeName == name {
		return name + " (" + issue.AssigneeID + ")"
	}
	return name + " · " + typeName + " (" + issue.AssigneeID + ")"
}

func boardExecutionLabel(issue BoardIssue) string {
	if strings.TrimSpace(issue.ExecutionPhase) == "" {
		return ""
	}
	return " · runtime " + issue.ExecutionPhase
}

func boardWorkflowStatus(issue BoardIssue) string {
	if strings.TrimSpace(issue.WorkflowStatus) != "" {
		return issue.WorkflowStatus
	}
	switch issue.Status {
	case "backlog", "blocked", "failed", "budget_exceeded":
		return "todo"
	default:
		return issue.Status
	}
}

func decodeArguments(arguments map[string]any, target any) error {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

func stringValue(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return strings.TrimSpace(value)
}

func int64Value(arguments map[string]any, key string) (int64, error) {
	value := arguments[key]
	switch typed := value.(type) {
	case float64:
		return int64(typed), nil
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
	default:
		return 0, ErrValidation
	}
}

func draftText(draft map[string]any, key string) string {
	value := stringValue(draft, key)
	if value == "" {
		return "Not set"
	}
	return "Current value: " + value
}

func countBlocked(issues []BoardIssue) int {
	count := 0
	for _, issue := range issues {
		if issue.Blocked {
			count++
		}
	}
	return count
}

type MemoryBoardRepository struct {
	mu       sync.RWMutex
	Issues   []BoardIssue
	Comments []BoardComment
	keys     map[string]BoardComment
}

func NewMemoryBoardRepository(issues ...BoardIssue) *MemoryBoardRepository {
	now := time.Now().UTC()
	for index := range issues {
		if issues[index].UpdatedAt.IsZero() {
			issues[index].UpdatedAt = now
		}
	}
	return &MemoryBoardRepository{Issues: issues, keys: map[string]BoardComment{}}
}

func (r *MemoryBoardRepository) ListIssues(_ context.Context, actor Actor, query string) ([]BoardIssue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := []BoardIssue{}
	for _, issue := range r.Issues {
		if !boardIssueAssignedTo(issue, actor) && !actor.HasScope("board:read:all") {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(issue.Identifier+" "+issue.Title+" "+issue.Objective), query) {
			continue
		}
		result = append(result, issue)
	}
	return result, nil
}

func (r *MemoryBoardRepository) GetIssue(_ context.Context, actor Actor, id string) (BoardIssue, []BoardComment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, issue := range r.Issues {
		if issue.ID != id {
			continue
		}
		if !boardIssueAssignedTo(issue, actor) && !actor.HasScope("board:read:all") {
			return BoardIssue{}, nil, ErrPermissionDenied
		}
		comments := []BoardComment{}
		for _, comment := range r.Comments {
			if comment.IssueID == id {
				comments = append(comments, comment)
			}
		}
		return issue, comments, nil
	}
	return BoardIssue{}, nil, fmt.Errorf("issue not found")
}

func (r *MemoryBoardRepository) AddComment(_ context.Context, actor Actor, issueID, body, key string) (BoardComment, error) {
	if _, _, err := r.GetIssue(context.Background(), actor, issueID); err != nil {
		return BoardComment{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if key != "" {
		if existing, ok := r.keys[key]; ok {
			return existing, nil
		}
	}
	comment := BoardComment{ID: newID("comment"), IssueID: issueID, AuthorID: actorIdentityID(actor), Body: body, CreatedAt: time.Now().UTC()}
	r.Comments = append(r.Comments, comment)
	if key != "" {
		r.keys[key] = comment
	}
	return comment, nil
}

func boardIssueAssignedTo(issue BoardIssue, actor Actor) bool {
	if actor.TaskAgentID != "" && issue.AssigneeTaskAgentID != "" {
		return issue.AssigneeTaskAgentID == actor.TaskAgentID
	}
	return issue.AssigneeID == "" || issue.AssigneeID == actor.AgentID
}
