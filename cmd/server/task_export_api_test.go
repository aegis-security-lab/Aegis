package main

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aegis/internal/control"
)

func TestTaskEvidenceDownloadAPI(t *testing.T) {
	store, err := control.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveConfig(control.SaveConfigInput{Provider: "test", Model: "test-model", AuthMode: "environment", Workspace: t.TempDir(), Concurrency: 1, ApprovalMode: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err = control.EnableObservability(store, io.Discard, "debug"); err != nil {
		t.Fatal(err)
	}
	manager, err := control.NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	issue, err := manager.CreateIssue(control.CreateIssueInput{Title: "Export through API", Objective: "download evidence", Priority: "low", Status: "backlog", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	router := buildRouter(store, manager, t.TempDir())

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/tasks/"+issue.ID+"/export?includeArtifacts=false", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/zip" || !strings.Contains(response.Header().Get("Content-Disposition"), "evidence.zip") {
		t.Fatalf("unexpected download headers: %v", response.Header())
	}
	if response.Header().Get("X-Aegis-Evidence-Redacted") != "true" {
		t.Fatalf("redaction header=%q", response.Header().Get("X-Aegis-Evidence-Redacted"))
	}
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	foundManifest := false
	for _, entry := range reader.File {
		if entry.Name == "manifest.json" {
			foundManifest = true
		}
		if strings.HasPrefix(entry.Name, "artifacts/") {
			t.Fatalf("artifacts were disabled but archive contains %s", entry.Name)
		}
	}
	if !foundManifest {
		t.Fatal("manifest.json is missing")
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/tasks/"+issue.ID+"/export?redactSecrets=maybe", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid boolean status=%d", response.Code)
	}
}
