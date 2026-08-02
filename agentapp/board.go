package agentapp

import (
	"context"
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
	Priority            string    `json:"priority"`
	AssigneeID          string    `json:"assigneeId"`
	AssigneeTaskAgentID string    `json:"assigneeTaskAgentId,omitempty"`
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

type BoardApp struct{ Repository BoardRepository }

func NewBoardApp(repository BoardRepository) *BoardApp { return &BoardApp{Repository: repository} }

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
	page := Page{PageID: "board.home", Title: "My Issues", Summary: fmt.Sprintf("Showing: %d of %d · Blocked: %d", len(issues), totalIssues, totalBlocked)}
	attention := Section{Title: "Needs attention"}
	other := Section{Title: "Issues"}
	for index, issue := range issues {
		ref := fmt.Sprintf("@%d", index+1)
		line := Line{Ref: ref, Kind: "issue", Label: fmt.Sprintf("%s · %s · %s · %s", issue.Identifier, issue.Title, issue.Status, issue.Priority), Detail: "Owner: " + issue.AssigneeID}
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
	page := Page{PageID: "issue.detail", Title: issue.Identifier + " " + issue.Title, Summary: fmt.Sprintf("%s · %s · owner %s", issue.Status, issue.Priority, issue.AssigneeID)}
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
	page.Sections = append(page.Sections, Section{Title: "Actions", Lines: []Line{{Ref: "@1", Kind: "input:text name=comment", Label: "Comment", Detail: fmt.Sprintf("Current value: %q", body)}, {Ref: "@2", Kind: "button", Label: "Post comment"}}})
	page.Refs = []Ref{{Ref: "@1", Kind: "input", Label: "Comment", Actions: []string{ActionInput}, Target: "comment-input", Field: "comment", InputType: "text"}, {Ref: "@2", Kind: "button", Label: "Post comment", Actions: []string{ActionSubmit}, Target: "comment-submit"}}
	return page, nil
}

func (a *BoardApp) issueMenu(ctx context.Context, request RenderRequest) (Page, error) {
	issue, _, err := a.Repository.GetIssue(ctx, request.Actor, request.Params["id"])
	if err != nil {
		return Page{}, err
	}
	return Page{PageID: "issue.menu", Title: issue.Identifier + " Actions", Summary: "Opened with long_press", Sections: []Section{{Title: "Issue actions", Lines: []Line{{Ref: "@1", Kind: "button", Label: "Open issue details", Detail: issue.Title}}}}, Refs: []Ref{{Ref: "@1", Kind: "button", Label: "Open issue details", Actions: []string{ActionClick}, Target: "open_issue:" + issue.ID}}}, nil
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
	return CommandResult{}, ErrActionNotAllowed
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
