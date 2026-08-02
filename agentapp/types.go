// Package agentapp implements a provider-neutral application surface for AI
// agents. Apps expose the same state through a human web UI and a compact,
// reference-based AI UI.
package agentapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const ProtocolVersion = "1.0"

const MaxPhoneShortcuts = 20

const (
	ActionClick     = "click"
	ActionInput     = "input"
	ActionSelect    = "select"
	ActionToggle    = "toggle"
	ActionSubmit    = "submit"
	ActionBack      = "back"
	ActionHome      = "home"
	ActionRefresh   = "refresh"
	ActionOpenApp   = "open_app"
	ActionScroll    = "scroll"
	ActionSwipe     = "swipe"
	ActionLongPress = "long_press"
	ActionDrag      = "drag"
	ActionLoadMore  = "load_more"
)

var (
	ErrAppNotFound      = errors.New("agent app not found")
	ErrSessionNotFound  = errors.New("phone session not found")
	ErrInvalidRef       = errors.New("invalid ref")
	ErrStalePage        = errors.New("stale page")
	ErrActionNotAllowed = errors.New("action not allowed")
	ErrValidation       = errors.New("validation error")
	ErrPermissionDenied = errors.New("permission denied")
	ErrConflict         = errors.New("conflict")
)

type Manifest struct {
	AppID        string      `json:"appId"`
	Name         string      `json:"name"`
	Version      string      `json:"version"`
	Description  string      `json:"description,omitempty"`
	Web          WebManifest `json:"web"`
	AI           AIManifest  `json:"ai"`
	Capabilities []string    `json:"capabilities"`
}

type WebManifest struct {
	Entry string `json:"entry"`
}

type AIManifest struct {
	ProtocolVersion string   `json:"protocolVersion"`
	Entry           string   `json:"entry"`
	Actions         string   `json:"actions"`
	ContentTypes    []string `json:"contentTypes"`
}

type Actor struct {
	AgentID       string   `json:"agentId"`
	TaskAgentID   string   `json:"taskAgentId,omitempty"`
	SessionID     string   `json:"sessionId"`
	TaskID        string   `json:"taskId,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	WorkspaceID   string   `json:"workspaceId,omitempty"`
	ExecutionID   string   `json:"executionId,omitempty"`
	Authenticated bool     `json:"authenticated"`
}

func (a Actor) HasScope(scope string) bool {
	for _, candidate := range a.Scopes {
		if candidate == "*" || candidate == scope {
			return true
		}
	}
	return false
}

type Page struct {
	AppID       string            `json:"appId"`
	PageID      string            `json:"pageId"`
	Revision    string            `json:"pageRevision"`
	Title       string            `json:"title"`
	Summary     string            `json:"summary,omitempty"`
	Sections    []Section         `json:"sections,omitempty"`
	Refs        []Ref             `json:"refs"`
	Text        string            `json:"text"`
	CanBack     bool              `json:"canBack"`
	CanHome     bool              `json:"canHome"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	GeneratedAt time.Time         `json:"generatedAt"`
}

type Section struct {
	Title string `json:"title"`
	Lines []Line `json:"lines"`
}

type Line struct {
	Ref      string            `json:"ref,omitempty"`
	Kind     string            `json:"kind,omitempty"`
	Label    string            `json:"label"`
	Detail   string            `json:"detail,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Ref struct {
	Ref       string            `json:"ref"`
	Kind      string            `json:"kind"`
	Label     string            `json:"label"`
	Actions   []string          `json:"actions"`
	Target    string            `json:"-"`
	Field     string            `json:"field,omitempty"`
	InputType string            `json:"inputType,omitempty"`
	Options   []Option          `json:"options,omitempty"`
	Dangerous bool              `json:"dangerous,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Location struct {
	AppID  string            `json:"appId"`
	Route  string            `json:"route"`
	Params map[string]string `json:"params,omitempty"`
}

type ActionRequest struct {
	PhoneSessionID string         `json:"phoneSessionId"`
	PageRevision   string         `json:"pageRevision"`
	Action         string         `json:"action"`
	Ref            string         `json:"ref"`
	Arguments      map[string]any `json:"arguments,omitempty"`
	IdempotencyKey string         `json:"idempotencyKey,omitempty"`
}

type ActionResponse struct {
	Status    string `json:"status"`
	Effect    string `json:"effect,omitempty"`
	Page      Page   `json:"page"`
	Toast     string `json:"toast,omitempty"`
	Terminate bool   `json:"terminate,omitempty"`
}

// ShortcutDefinition describes one high-frequency semantic operation exposed
// by an installed Phone app. Shortcuts are still executed by Phone, so they
// share its identity, authorization, idempotency, persistence and audit trail.
type ShortcutDefinition struct {
	Name        string          `json:"name"`
	AppID       string          `json:"appId"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Frequency   int             `json:"frequency,omitempty"`
	Terminates  bool            `json:"terminates,omitempty"`
}

type ShortcutRequest struct {
	PhoneSessionID string         `json:"phoneSessionId"`
	Name           string         `json:"name"`
	Arguments      map[string]any `json:"arguments,omitempty"`
	IdempotencyKey string         `json:"idempotencyKey,omitempty"`
}

type ShortcutResponse struct {
	Status    string `json:"status"`
	Effect    string `json:"effect,omitempty"`
	Page      Page   `json:"page"`
	Toast     string `json:"toast,omitempty"`
	Terminate bool   `json:"terminate,omitempty"`
}

type ActionError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	Page    *Page             `json:"page,omitempty"`
}

func (e *ActionError) Error() string { return e.Message }

type RenderRequest struct {
	Actor  Actor
	Route  string
	Params map[string]string
	Draft  map[string]any
}

type Command struct {
	Actor          Actor
	Location       Location
	Action         string
	Target         string
	Arguments      map[string]any
	Draft          map[string]any
	IdempotencyKey string
}

type CommandResult struct {
	Location  *Location
	Toast     string
	Effect    string
	Draft     map[string]any
	Terminate bool
}

type App interface {
	Manifest() Manifest
	Render(context.Context, RenderRequest) (Page, error)
	Execute(context.Context, Command) (CommandResult, error)
}

// ShortcutApp is optional. Phone discovers shortcuts only from apps installed
// in the current session and caps the resulting high-frequency set at 20.
type ShortcutApp interface {
	Shortcuts() []ShortcutDefinition
	ExecuteShortcut(context.Context, ShortcutCommand) (CommandResult, error)
}

type ShortcutCommand struct {
	Actor          Actor
	Name           string
	Arguments      map[string]any
	IdempotencyKey string
}

type AuditEvent struct {
	ID             string         `json:"id"`
	OccurredAt     time.Time      `json:"occurredAt"`
	AgentID        string         `json:"agentId"`
	PhoneSessionID string         `json:"phoneSessionId"`
	AppID          string         `json:"appId"`
	PageID         string         `json:"pageId"`
	PageRevision   string         `json:"pageRevision"`
	Ref            string         `json:"ref"`
	Action         string         `json:"action"`
	Target         string         `json:"target,omitempty"`
	Result         string         `json:"result"`
	Effect         string         `json:"effect,omitempty"`
	ErrorCode      string         `json:"errorCode,omitempty"`
	ErrorMessage   string         `json:"errorMessage,omitempty"`
	Arguments      map[string]any `json:"arguments,omitempty"`
}

type SessionState struct {
	ID              string         `json:"id"`
	AgentID         string         `json:"agentId"`
	TaskAgentID     string         `json:"taskAgentId,omitempty"`
	TaskID          string         `json:"taskId,omitempty"`
	ExecutionID     string         `json:"executionId,omitempty"`
	WorkspaceID     string         `json:"workspaceId,omitempty"`
	InstalledApps   []string       `json:"installedApps"`
	ActiveAppID     string         `json:"activeAppId,omitempty"`
	CurrentAppID    string         `json:"currentAppId"`
	CurrentPageID   string         `json:"currentPageId"`
	CurrentRevision string         `json:"currentRevision"`
	NavigationDepth map[string]int `json:"navigationDepth"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

type Auditor interface {
	Record(context.Context, AuditEvent)
}

type NopAuditor struct{}

func (NopAuditor) Record(context.Context, AuditEvent) {}

func ValidateManifest(manifest Manifest) error {
	if manifest.AppID == "" || manifest.Name == "" || manifest.Version == "" {
		return fmt.Errorf("manifest appId, name and version are required")
	}
	if manifest.AI.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("unsupported protocol version %q", manifest.AI.ProtocolVersion)
	}
	return nil
}
