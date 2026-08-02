package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aegis/internal/control"
)

func TestFindingsAPI(t *testing.T) {
	s, err := control.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveConfig(control.SaveConfigInput{Provider: "test", Model: "m", AuthMode: "environment", Workspace: t.TempDir(), Concurrency: 1, ApprovalMode: "none"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := control.NewManager(s)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	router := buildRouter(s, m, t.TempDir())

	// POST /api/findings — create a finding
	createBody := map[string]any{
		"domain":           "example.com",
		"category":         "tls",
		"severity":         "high",
		"title":            "Weak TLS cipher",
		"description":      "Server supports weak ciphers.",
		"evidence":         map[string]any{"cipher": "RC4"},
		"stepsToReproduce": "Run sslscan",
		"remediation":      "Disable weak ciphers",
	}
	body, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/findings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("POST status=%d body=%s", res.Code, res.Body.String())
	}
	var finding control.Finding
	if err = json.Unmarshal(res.Body.Bytes(), &finding); err != nil {
		t.Fatal(err)
	}
	if finding.ID == "" || finding.Status != "open" {
		t.Fatalf("bad finding: %+v", finding)
	}

	// GET /api/findings — list all
	req = httptest.NewRequest(http.MethodGet, "/api/findings", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET list status=%d", res.Code)
	}
	var list control.FindingList
	if err = json.Unmarshal(res.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Findings) != 1 {
		t.Fatalf("expected 1 finding, got total=%d items=%d", list.Total, len(list.Findings))
	}

	// GET /api/findings?category=tls — filter
	req = httptest.NewRequest(http.MethodGet, "/api/findings?category=tls", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET filter status=%d", res.Code)
	}
	if err = json.Unmarshal(res.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 {
		t.Fatalf("expected 1 finding with category=tls, got %d", list.Total)
	}

	// GET /api/findings?severity=info — no match
	req = httptest.NewRequest(http.MethodGet, "/api/findings?severity=info", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET filter severity status=%d", res.Code)
	}
	if err = json.Unmarshal(res.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 0 {
		t.Fatalf("expected 0 findings with severity=info, got %d", list.Total)
	}

	// GET /api/findings/:id — get by ID
	req = httptest.NewRequest(http.MethodGet, "/api/findings/"+finding.ID, nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET by id status=%d", res.Code)
	}
	var fetched control.Finding
	if err = json.Unmarshal(res.Body.Bytes(), &fetched); err != nil {
		t.Fatal(err)
	}
	if fetched.Title != "Weak TLS cipher" {
		t.Fatalf("title mismatch: %s", fetched.Title)
	}

	// PATCH /api/findings/:id — update status
	patchBody := map[string]any{"status": "verified"}
	patchData, _ := json.Marshal(patchBody)
	req = httptest.NewRequest(http.MethodPatch, "/api/findings/"+finding.ID, bytes.NewReader(patchData))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("PATCH status=%d body=%s", res.Code, res.Body.String())
	}
	var patched control.Finding
	if err = json.Unmarshal(res.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Status != "verified" {
		t.Fatalf("status not updated: %s", patched.Status)
	}

	// PATCH with invalid status should fail
	patchBody = map[string]any{"status": "bogus"}
	patchData, _ = json.Marshal(patchBody)
	req = httptest.NewRequest(http.MethodPatch, "/api/findings/"+finding.ID, bytes.NewReader(patchData))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid status, got %d", res.Code)
	}

	// GET /api/findings/nonexistent should 404
	req = httptest.NewRequest(http.MethodGet, "/api/findings/nonexistent", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent, got %d", res.Code)
	}

	// POST with missing required fields should 422
	badBody, _ := json.Marshal(map[string]any{"domain": "x"})
	req = httptest.NewRequest(http.MethodPost, "/api/findings", bytes.NewReader(badBody))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for bad POST, got %d", res.Code)
	}

	// PATCH with empty title should 422
	emptyTitle := ""
	patchBody = map[string]any{"title": emptyTitle}
	patchData, _ = json.Marshal(patchBody)
	req = httptest.NewRequest(http.MethodPatch, "/api/findings/"+finding.ID, bytes.NewReader(patchData))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty title, got %d", res.Code)
	}
}

func TestIssueAPI(t *testing.T) {
	s, err := control.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveConfig(control.SaveConfigInput{Provider: "test", Model: "m", AuthMode: "environment", Workspace: t.TempDir(), Concurrency: 1, ApprovalMode: "none"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := control.NewManager(s)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	router := buildRouter(s, m, t.TempDir())
	body, _ := json.Marshal(control.CreateIssueInput{Title: "API Issue", Objective: "The API Issue is completed and verified.", Priority: "high", WorkMode: "guided"})
	req := httptest.NewRequest(http.MethodPost, "/api/issues", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var issue control.Issue
	if err = json.Unmarshal(res.Body.Bytes(), &issue); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/issues/"+issue.ID, nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d", res.Code)
	}
	var detail control.IssueDetail
	if err = json.Unmarshal(res.Body.Bytes(), &detail); err != nil || detail.Issue.Identifier == "" {
		t.Fatalf("bad detail: %v %+v", err, detail)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/tasks/"+issue.ID+"/timeline", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("timeline status=%d body=%s", res.Code, res.Body.String())
	}
	var timeline control.TaskTimeline
	if err = json.Unmarshal(res.Body.Bytes(), &timeline); err != nil || timeline.Task.ID != issue.ID || len(timeline.Events) == 0 {
		t.Fatalf("bad timeline: %v %+v", err, timeline)
	}
	cancelBody, _ := json.Marshal(control.CancelTaskInput{Reason: "API cancellation test"})
	req = httptest.NewRequest(http.MethodPost, "/api/tasks/"+issue.ID+"/cancel", bytes.NewReader(cancelBody))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", res.Code, res.Body.String())
	}
	var cancellation control.TaskCancellationResult
	if err = json.Unmarshal(res.Body.Bytes(), &cancellation); err != nil {
		t.Fatal(err)
	}
	if cancellation.Task.Status != "cancelled" || cancellation.TotalIssues != 1 {
		t.Fatalf("bad cancellation: %+v", cancellation)
	}
}
