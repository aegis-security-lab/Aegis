package agentapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client is the transport used by an Agent runtime or a remote Phone host.
// It intentionally exposes generic page/actions instead of Board- or
// Relay-specific methods.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
}

type StartSessionRequest struct {
	AgentID       string   `json:"agentId"`
	TaskAgentID   string   `json:"taskAgentId,omitempty"`
	TaskID        string   `json:"taskId,omitempty"`
	ExecutionID   string   `json:"executionId,omitempty"`
	WorkspaceID   string   `json:"workspaceId,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	InstalledApps []string `json:"installedApps,omitempty"`
}

type StartSessionResponse struct {
	PhoneSessionID string `json:"phoneSessionId"`
	Page           Page   `json:"page"`
}

func (c Client) Start(ctx context.Context, request StartSessionRequest) (StartSessionResponse, error) {
	var response StartSessionResponse
	err := c.doJSON(ctx, http.MethodPost, "/phone/sessions", request, &response, "")
	return response, err
}

func (c Client) View(ctx context.Context, sessionID string) (Page, error) {
	var page Page
	err := c.doJSON(ctx, http.MethodGet, "/phone/sessions/"+sessionID+"/page", nil, &page, "")
	return page, err
}

func (c Client) Logs(ctx context.Context, sessionID string) ([]AuditEvent, error) {
	var response struct {
		Events []AuditEvent `json:"events"`
	}
	err := c.doJSON(ctx, http.MethodGet, "/phone/sessions/"+sessionID+"/logs", nil, &response, "")
	return response.Events, err
}

func (c Client) Sessions(ctx context.Context, agentID string) ([]SessionState, error) {
	var response struct {
		Sessions []SessionState `json:"sessions"`
	}
	path := "/phone/sessions"
	if agentID != "" {
		path += "?agentId=" + url.QueryEscape(agentID)
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &response, "")
	return response.Sessions, err
}

func (c Client) Act(ctx context.Context, request ActionRequest) (ActionResponse, error) {
	var response ActionResponse
	err := c.doJSON(ctx, http.MethodPost, "/phone/actions", request, &response, request.IdempotencyKey)
	return response, err
}

func (c Client) doJSON(ctx context.Context, method, path string, input, output any, idempotencyKey string) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/agent-ui+json")
	if c.Token != "" {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var actionErr ActionError
		if decodeErr := json.NewDecoder(response.Body).Decode(&actionErr); decodeErr != nil {
			return fmt.Errorf("agent app HTTP %d", response.StatusCode)
		}
		return &actionErr
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("decode agent app response: %w", err)
	}
	return nil
}
