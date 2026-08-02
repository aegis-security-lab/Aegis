package agentcoreadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"aegis/agentapp"
	"github.com/z3r2ne/agentcore"
)

const defaultMaxPageBytes = 64 * 1024

// Client is the provider-neutral Phone transport used by the tools.
type Client interface {
	Start(context.Context, agentapp.StartSessionRequest) (agentapp.StartSessionResponse, error)
	View(context.Context, string) (agentapp.Page, error)
	Act(context.Context, agentapp.ActionRequest) (agentapp.ActionResponse, error)
	Shortcuts(context.Context, string) ([]agentapp.ShortcutDefinition, error)
	RunShortcut(context.Context, agentapp.ShortcutRequest) (agentapp.ShortcutResponse, error)
}

// ToolsetConfig binds one execution identity to one hidden Phone session.
type ToolsetConfig struct {
	Client        Client
	ExecutionID   string
	AgentID       string
	TaskAgentID   string
	TaskID        string
	WorkspaceID   string
	Scopes        []string
	InstalledApps []string
	SessionID     string
	MaxPageBytes  int
	Policy        agentcore.ToolPolicy
}

// Toolset owns the current Phone page used by four sequential agent tools.
type Toolset struct {
	config ToolsetConfig

	mu        sync.Mutex
	sessionID string
	current   agentapp.Page
	shortcuts []agentapp.ShortcutDefinition
	sequence  atomic.Uint64
}

func NewToolset(config ToolsetConfig) (*Toolset, error) {
	if config.Client == nil {
		return nil, errors.New("agentphone tools: client is required")
	}
	if strings.TrimSpace(config.AgentID) == "" {
		return nil, errors.New("agentphone tools: agent ID is required")
	}
	if config.MaxPageBytes <= 0 {
		config.MaxPageBytes = defaultMaxPageBytes
	}
	return &Toolset{config: config, sessionID: strings.TrimSpace(config.SessionID)}, nil
}

// Initialize starts or restores the hidden Phone session. It is safe to call
// repeatedly and is also performed lazily by each tool.
func (t *Toolset) Initialize(ctx context.Context) (agentapp.Page, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.initializeLocked(ctx)
}

func (t *Toolset) initializeLocked(ctx context.Context) (agentapp.Page, error) {
	if t.current.Revision != "" {
		if t.shortcuts == nil {
			shortcuts, err := t.config.Client.Shortcuts(ctx, t.sessionID)
			if err != nil {
				return agentapp.Page{}, fmt.Errorf("discover Phone shortcuts: %w", err)
			}
			t.shortcuts = shortcuts
		}
		return t.current, nil
	}
	if t.sessionID != "" {
		page, err := t.config.Client.View(ctx, t.sessionID)
		if err != nil {
			return agentapp.Page{}, fmt.Errorf("restore Phone session: %w", err)
		}
		t.current = page
		shortcuts, shortcutErr := t.config.Client.Shortcuts(ctx, t.sessionID)
		if shortcutErr != nil {
			return agentapp.Page{}, fmt.Errorf("discover Phone shortcuts: %w", shortcutErr)
		}
		t.shortcuts = shortcuts
		return page, nil
	}
	started, err := t.config.Client.Start(ctx, agentapp.StartSessionRequest{
		AgentID: t.config.AgentID, TaskAgentID: t.config.TaskAgentID, TaskID: t.config.TaskID, ExecutionID: t.config.ExecutionID, WorkspaceID: t.config.WorkspaceID,
		Scopes:        append([]string(nil), t.config.Scopes...),
		InstalledApps: append([]string(nil), t.config.InstalledApps...),
	})
	if err != nil {
		return agentapp.Page{}, fmt.Errorf("start Phone session: %w", err)
	}
	if strings.TrimSpace(started.PhoneSessionID) == "" {
		return agentapp.Page{}, errors.New("start Phone session: server returned empty session ID")
	}
	t.sessionID = started.PhoneSessionID
	t.current = started.Page
	shortcuts, shortcutErr := t.config.Client.Shortcuts(ctx, t.sessionID)
	if shortcutErr != nil {
		return agentapp.Page{}, fmt.Errorf("discover Phone shortcuts: %w", shortcutErr)
	}
	t.shortcuts = shortcuts
	return t.current, nil
}

// SessionID returns the hidden Phone session ID after initialization.
func (t *Toolset) SessionID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessionID
}

// Tools returns stable model-facing tools. All force sequential batches because
// Phone actions consume and replace one page revision.
func (t *Toolset) Tools() []agentcore.Tool {
	tools := []agentcore.Tool{
		t.viewTool(),
		t.actionTool(),
		t.navigationTool("phone_back", "Return to the previous Agent Phone page.", agentapp.ActionBack, "@back"),
		t.navigationTool("phone_home", "Return to the Agent Phone home page.", agentapp.ActionHome, "@home"),
	}
	for _, shortcut := range t.shortcuts {
		tools = append(tools, t.shortcutTool(shortcut))
	}
	return tools
}

func (t *Toolset) shortcutTool(definition agentapp.ShortcutDefinition) agentcore.Tool {
	parameters := definition.Parameters
	if len(parameters) == 0 {
		parameters = json.RawMessage(`{"type":"object","additionalProperties":false}`)
	}
	return agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{Name: definition.Name, Description: definition.Description, Parameters: parameters},
		Mode:           agentcore.ToolExecutionSequential, Policy: t.config.Policy,
		ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			arguments := map[string]any{}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &arguments); err != nil {
					return agentcore.ToolResult{}, err
				}
			}
			t.mu.Lock()
			defer t.mu.Unlock()
			if _, err := t.initializeLocked(ctx); err != nil {
				return agentcore.ToolResult{}, err
			}
			response, err := t.config.Client.RunShortcut(ctx, agentapp.ShortcutRequest{PhoneSessionID: t.sessionID, Name: definition.Name, Arguments: arguments, IdempotencyKey: t.idempotencyKey(ctx)})
			if err != nil {
				return agentcore.ToolResult{}, err
			}
			t.current = response.Page
			result := t.pageResult(response.Effect, response.Toast, response.Page)
			result.Terminate = definition.Terminates || response.Terminate
			return result, nil
		},
	}
}

func (t *Toolset) viewTool() agentcore.Tool {
	return agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{
			Name: "phone_view", Description: "Read the current Agent Phone page and its available REF actions.",
			Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		},
		Mode: agentcore.ToolExecutionSequential, Policy: t.config.Policy,
		ExecuteFunc: func(ctx context.Context, _ json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			page, err := t.Initialize(ctx)
			if err != nil {
				return agentcore.ToolResult{}, err
			}
			return t.pageResult("viewed", "", page), nil
		},
	}
}

type actionArguments struct {
	Action    string         `json:"action"`
	Ref       string         `json:"ref"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

func (t *Toolset) actionTool() agentcore.Tool {
	return agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{
			Name: "phone_action", Description: "Execute one action against a REF on the current Agent Phone page and return the complete resulting page.",
			Parameters: json.RawMessage(`{
				"type":"object",
				"properties":{
					"action":{"type":"string","enum":["click","input","select","toggle","submit","back","home","refresh","open_app","scroll","swipe","long_press","drag","load_more"]},
					"ref":{"type":"string","minLength":1},
					"arguments":{"type":"object"}
				},
				"required":["action","ref"],
				"additionalProperties":false
			}`),
		},
		Mode: agentcore.ToolExecutionSequential, Policy: t.config.Policy,
		ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			var arguments actionArguments
			if err := json.Unmarshal(raw, &arguments); err != nil {
				return agentcore.ToolResult{}, err
			}
			return t.act(ctx, arguments)
		},
	}
}

func (t *Toolset) navigationTool(name, description, action, ref string) agentcore.Tool {
	return agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{Name: name, Description: description, Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
		Mode:           agentcore.ToolExecutionSequential, Policy: t.config.Policy,
		ExecuteFunc: func(ctx context.Context, _ json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			return t.act(ctx, actionArguments{Action: action, Ref: ref})
		},
	}
}

func (t *Toolset) act(ctx context.Context, arguments actionArguments) (agentcore.ToolResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	page, err := t.initializeLocked(ctx)
	if err != nil {
		return agentcore.ToolResult{}, err
	}
	request := agentapp.ActionRequest{
		PhoneSessionID: t.sessionID, PageRevision: page.Revision,
		Action: arguments.Action, Ref: arguments.Ref, Arguments: arguments.Arguments,
		IdempotencyKey: t.idempotencyKey(ctx),
	}
	response, err := t.config.Client.Act(ctx, request)
	if err != nil {
		var actionError *agentapp.ActionError
		if errors.As(err, &actionError) && actionError.Page != nil {
			t.current = *actionError.Page
			return t.pageResult("failed", actionError.Message, t.current), err
		}
		return agentcore.ToolResult{}, err
	}
	t.current = response.Page
	result := t.pageResult(response.Effect, response.Toast, response.Page)
	result.Terminate = response.Terminate
	return result, nil
}

func (t *Toolset) idempotencyKey(ctx context.Context) string {
	if invocation, ok := agentcore.ToolInvocationFromContext(ctx); ok && invocation.Call.ID != "" {
		return "phone:" + safeKeyPart(t.config.ExecutionID) + ":" + safeKeyPart(invocation.Call.ID)
	}
	return fmt.Sprintf("phone:%s:direct-%d", safeKeyPart(t.config.ExecutionID), t.sequence.Add(1))
}

func (t *Toolset) pageResult(effect, toast string, page agentapp.Page) agentcore.ToolResult {
	var text strings.Builder
	if effect != "" {
		fmt.Fprintf(&text, "[PHONE RESULT] %s\n", effect)
	}
	if toast != "" {
		fmt.Fprintf(&text, "[TOAST] %s\n", strings.Join(strings.Fields(toast), " "))
	}
	text.WriteString(page.Text)
	return agentcore.ToolResult{
		Content: []agentcore.ContentBlock{{Type: agentcore.ContentText, Text: truncateUTF8(text.String(), t.config.MaxPageBytes)}},
		Details: map[string]any{"phoneSessionId": t.sessionID, "pageRevision": page.Revision, "appId": page.AppID, "pageId": page.PageID},
	}
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	const marker = "\n... Phone page truncated ..."
	limit := maximum - len(marker)
	if limit <= 0 {
		return marker[:maximum]
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + marker
}

func safeKeyPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, value)
}
