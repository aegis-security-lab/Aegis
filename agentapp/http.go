package agentapp

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type HTTPServer struct {
	Registry      *Registry
	Phone         *Phone
	Authenticator Authenticator
	Handler       http.Handler
}

type Authenticator interface {
	Authenticate(*http.Request) (Actor, error)
}

type BearerTokens map[string]Actor

func (tokens BearerTokens) Authenticate(request *http.Request) (Actor, error) {
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return Actor{}, ErrPermissionDenied
	}
	actor, ok := tokens[strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))]
	if !ok || actor.AgentID == "" {
		return Actor{}, ErrPermissionDenied
	}
	actor.Authenticated = true
	return actor, nil
}

func NewHTTPServer(registry *Registry, phone *Phone) *HTTPServer {
	return NewAuthenticatedHTTPServer(registry, phone, nil)
}

// NewAuthenticatedHTTPServer creates a production-capable server. A nil
// authenticator is explicit development mode and trusts agentId from the
// session request.
func NewAuthenticatedHTTPServer(registry *Registry, phone *Phone, authenticator Authenticator) *HTTPServer {
	server := &HTTPServer{Registry: registry, Phone: phone, Authenticator: authenticator}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/agent-apps.json", server.listApps)
	mux.HandleFunc("GET /apps/{appID}/.well-known/agent-app.json", server.manifest)
	mux.HandleFunc("POST /phone/sessions", server.startSession)
	mux.HandleFunc("GET /phone/sessions", server.listSessions)
	mux.HandleFunc("GET /phone/sessions/{sessionID}/page", server.viewPage)
	mux.HandleFunc("GET /phone/sessions/{sessionID}/logs", server.sessionLogs)
	mux.HandleFunc("GET /phone/sessions/{sessionID}/shortcuts", server.sessionShortcuts)
	mux.HandleFunc("POST /phone/actions", server.action)
	mux.HandleFunc("POST /phone/shortcuts", server.shortcut)
	mux.HandleFunc("GET /", server.web)
	server.Handler = securityHeaders(mux)
	return server
}

func (s *HTTPServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	s.Handler.ServeHTTP(writer, request)
}

func (s *HTTPServer) listApps(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"protocolVersion": ProtocolVersion, "apps": s.Registry.Manifests()})
}

func (s *HTTPServer) manifest(writer http.ResponseWriter, request *http.Request) {
	app, ok := s.Registry.App(request.PathValue("appID"))
	if !ok {
		writeAPIError(writer, ErrAppNotFound, nil)
		return
	}
	writeJSON(writer, http.StatusOK, app.Manifest())
}

func (s *HTTPServer) startSession(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		AgentID       string   `json:"agentId"`
		TaskAgentID   string   `json:"taskAgentId"`
		TaskID        string   `json:"taskId"`
		ExecutionID   string   `json:"executionId"`
		WorkspaceID   string   `json:"workspaceId"`
		Scopes        []string `json:"scopes"`
		InstalledApps []string `json:"installedApps"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeAPIError(writer, err, nil)
		return
	}
	actor := Actor{AgentID: input.AgentID, TaskAgentID: input.TaskAgentID, TaskID: input.TaskID, ExecutionID: input.ExecutionID, WorkspaceID: input.WorkspaceID, Scopes: input.Scopes}
	if s.Authenticator != nil {
		authenticated, err := s.Authenticator.Authenticate(request)
		if err != nil {
			writeAPIError(writer, err, nil)
			return
		}
		if input.AgentID != "" && input.AgentID != authenticated.AgentID {
			writeAPIError(writer, ErrPermissionDenied, nil)
			return
		}
		if !mayBindIdentity(authenticated, input.ExecutionID, authenticated.ExecutionID, "phone:bind-execution") ||
			!mayBindIdentity(authenticated, input.WorkspaceID, authenticated.WorkspaceID, "phone:bind-workspace") ||
			!mayBindIdentity(authenticated, input.TaskID, authenticated.TaskID, "phone:bind-task") {
			writeAPIError(writer, ErrPermissionDenied, nil)
			return
		}
		actor = authenticated
		if input.ExecutionID != "" {
			actor.ExecutionID = input.ExecutionID
		}
		if input.TaskID != "" {
			actor.TaskID = input.TaskID
		}
		if input.WorkspaceID != "" {
			actor.WorkspaceID = input.WorkspaceID
		}
	}
	if len(input.InstalledApps) == 0 {
		for _, manifest := range s.Registry.Manifests() {
			input.InstalledApps = append(input.InstalledApps, manifest.AppID)
		}
	}
	sessionID, page, err := s.Phone.StartSession(request.Context(), actor, input.InstalledApps)
	if err != nil {
		writeAPIError(writer, err, nil)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"phoneSessionId": sessionID, "page": page})
}

// mayBindIdentity prevents a bearer token from claiming arbitrary execution or
// workspace ownership. Production authenticators should preferably bind these
// values in the token actor; delegation is an explicit, narrow scope.
func mayBindIdentity(actor Actor, requested, bound, delegationScope string) bool {
	if requested == "" {
		return true
	}
	return requested == bound || (bound == "" && actor.HasScope(delegationScope))
}

func (s *HTTPServer) listSessions(writer http.ResponseWriter, request *http.Request) {
	agentID := strings.TrimSpace(request.URL.Query().Get("agentId"))
	if s.Authenticator != nil {
		actor, err := s.Authenticator.Authenticate(request)
		if err != nil {
			writeAPIError(writer, err, nil)
			return
		}
		if agentID != "" && agentID != actor.AgentID {
			writeAPIError(writer, ErrPermissionDenied, nil)
			return
		}
		agentID = actor.AgentID
	}
	states, err := s.Phone.Sessions(request.Context(), agentID, 100)
	if err != nil {
		writeAPIError(writer, err, nil)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"sessions": states})
}

func (s *HTTPServer) viewPage(writer http.ResponseWriter, request *http.Request) {
	if !s.authorizeSession(writer, request, request.PathValue("sessionID")) {
		return
	}
	page, err := s.Phone.View(request.Context(), request.PathValue("sessionID"))
	if err != nil {
		writeAPIError(writer, err, nil)
		return
	}
	if strings.Contains(request.Header.Get("Accept"), "text/agent-ui") {
		writer.Header().Set("Content-Type", "text/agent-ui; charset=utf-8")
		_, _ = writer.Write([]byte(page.Text))
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (s *HTTPServer) sessionLogs(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("sessionID")
	if !s.authorizeSession(writer, request, sessionID) {
		return
	}
	limit := 100
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	events, err := s.Phone.Logs(request.Context(), sessionID, limit)
	if err != nil {
		writeAPIError(writer, err, nil)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"events": events})
}

func (s *HTTPServer) action(writer http.ResponseWriter, request *http.Request) {
	var input ActionRequest
	if err := decodeJSON(request, &input); err != nil {
		writeAPIError(writer, err, nil)
		return
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = request.Header.Get("Idempotency-Key")
	}
	if !s.authorizeSession(writer, request, input.PhoneSessionID) {
		return
	}
	response, err := s.Phone.Act(request.Context(), input)
	if err != nil {
		writeAPIError(writer, err, nil)
		return
	}
	if strings.Contains(request.Header.Get("Accept"), "text/agent-ui") {
		writer.Header().Set("Content-Type", "text/agent-ui; charset=utf-8")
		_, _ = writer.Write([]byte(response.Page.Text))
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (s *HTTPServer) sessionShortcuts(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("sessionID")
	if !s.authorizeSession(writer, request, sessionID) {
		return
	}
	shortcuts, err := s.Phone.Shortcuts(request.Context(), sessionID)
	if err != nil {
		writeAPIError(writer, err, nil)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"shortcuts": shortcuts})
}

func (s *HTTPServer) shortcut(writer http.ResponseWriter, request *http.Request) {
	var input ShortcutRequest
	if err := decodeJSON(request, &input); err != nil {
		writeAPIError(writer, err, nil)
		return
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = request.Header.Get("Idempotency-Key")
	}
	if !s.authorizeSession(writer, request, input.PhoneSessionID) {
		return
	}
	response, err := s.Phone.RunShortcut(request.Context(), input)
	if err != nil {
		writeAPIError(writer, err, nil)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (s *HTTPServer) authorizeSession(writer http.ResponseWriter, request *http.Request, sessionID string) bool {
	if s.Authenticator == nil {
		return true
	}
	actor, err := s.Authenticator.Authenticate(request)
	if err != nil {
		writeAPIError(writer, err, nil)
		return false
	}
	owner, err := s.Phone.SessionActor(request.Context(), sessionID)
	if err != nil {
		writeAPIError(writer, err, nil)
		return false
	}
	if owner.AgentID != actor.AgentID ||
		(actor.ExecutionID != "" && owner.ExecutionID != actor.ExecutionID) ||
		(actor.WorkspaceID != "" && owner.WorkspaceID != actor.WorkspaceID) {
		writeAPIError(writer, ErrPermissionDenied, nil)
		return false
	}
	return true
}

func (s *HTTPServer) web(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = writer.Write([]byte(webHTML))
}

func decodeJSON(request *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeAPIError(writer http.ResponseWriter, err error, page *Page) {
	status := http.StatusUnprocessableEntity
	code := "APP_ERROR"
	message := err.Error()
	var actionErr *ActionError
	if errors.As(err, &actionErr) {
		code = actionErr.Code
		message = actionErr.Message
		page = actionErr.Page
		if code == "STALE_PAGE" {
			status = http.StatusConflict
		}
	}
	switch {
	case errors.Is(err, ErrAppNotFound), errors.Is(err, ErrSessionNotFound):
		status = http.StatusNotFound
		if errors.Is(err, ErrAppNotFound) {
			code = "APP_NOT_FOUND"
		} else {
			code = "SESSION_NOT_FOUND"
		}
	case errors.Is(err, ErrPermissionDenied):
		status = http.StatusForbidden
		code = "PERMISSION_DENIED"
	case errors.Is(err, ErrValidation):
		code = "VALIDATION_ERROR"
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
		code = "CONFLICT"
	case strings.HasPrefix(message, "invalid JSON"):
		status = http.StatusBadRequest
	}
	writeJSON(writer, status, ActionError{Code: code, Message: message, Page: page})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'")
		next.ServeHTTP(writer, request)
	})
}

//go:embed web/index.html
var webHTML string
