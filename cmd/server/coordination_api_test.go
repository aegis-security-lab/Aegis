package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aegis/agenthost"
	"aegis/coordination"
	"aegis/internal/control"
	"github.com/z3r2ne/agentcore"
)

type coordinationAPIRunner struct{}

func (coordinationAPIRunner) Run(context.Context, agenthost.ExecutionSpec, agentcore.EventSink) (agenthost.Result, error) {
	return agenthost.Result{Core: agentcore.Result{StopReason: agentcore.StopReasonStop}}, nil
}

func TestCoordinationModeAPI(t *testing.T) {
	store, err := control.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveConfig(control.SaveConfigInput{Provider: "test", Model: "test-model", AuthMode: "environment", Workspace: t.TempDir(), Concurrency: 1, ApprovalMode: "none"}); err != nil {
		t.Fatal(err)
	}
	manager, err := control.NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	_, phone, err := control.NewControlAgentPhone(manager)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetAgentPhoneClient(phone)
	t.Cleanup(manager.Close)
	bridge, err := control.NewCoordinationBridge(manager, control.CoordinationBridgeOptions{WorkerID: "api-test", DefaultMode: "board_autonomy", Executor: coordinationAPIRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	issue, err := manager.CreateIssue(control.CreateIssueInput{Title: "Mode API", Objective: "test", Priority: "low", Status: "backlog", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	router := buildRouter(store, manager, t.TempDir())

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/coordination/modes", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"board_autonomy"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"direct_supervisor"`)) {
		t.Fatalf("modes status=%d body=%s", response.Code, response.Body.String())
	}

	body := []byte(`{"mode":"board_autonomy","version":"1","config":{"heartbeatIntervalSeconds":60}}`)
	request := httptest.NewRequest(http.MethodPut, "/api/issues/"+issue.ID+"/coordination", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", response.Code, response.Body.String())
	}
	var binding coordination.Binding
	if err := json.Unmarshal(response.Body.Bytes(), &binding); err != nil {
		t.Fatal(err)
	}
	if binding.Mode != "board_autonomy" || string(binding.Config) != `{"heartbeatIntervalSeconds":60}` {
		t.Fatalf("unexpected binding: %+v", binding)
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/issues/"+issue.ID+"/coordination", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"mode":"board_autonomy"`)) {
		t.Fatalf("get status=%d body=%s", response.Code, response.Body.String())
	}

	planBody := []byte(`{"issueId":"` + issue.ID + `","child":{"agentId":"backend-engineer","prompt":"preview","capabilitySelection":"replace","capabilities":[{"kind":"skill","name":"go-service-engineering"}]}}`)
	request = httptest.NewRequest(http.MethodPost, "/api/coordination/capabilities/plan", bytes.NewReader(planBody))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"selection":"replace"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"name":"coordination"`)) {
		t.Fatalf("plan status=%d body=%s", response.Code, response.Body.String())
	}

	invokeBody := []byte(`{"issueId":"` + issue.ID + `","child":{"agentId":"backend-engineer","prompt":"start proactively"},"parentBehavior":"continue"}`)
	request = httptest.NewRequest(http.MethodPost, "/api/coordination/invoke", bytes.NewReader(invokeBody))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !bytes.Contains(response.Body.Bytes(), []byte(`"accepted":true`)) {
		t.Fatalf("invoke status=%d body=%s", response.Code, response.Body.String())
	}
}
