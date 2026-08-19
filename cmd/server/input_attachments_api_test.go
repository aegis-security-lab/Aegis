package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"aegis/apps/board/control"
)

func TestInputAttachmentUploadAPIsStreamAndDeleteStagedFiles(t *testing.T) {
	store, err := control.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := control.NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	router := buildRouter(store, manager, t.TempDir())

	upload := func(endpoint, name, content string) control.InputAttachment {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		file, createErr := writer.CreateFormFile("file", name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, createErr = io.WriteString(file, content); createErr != nil {
			t.Fatal(createErr)
		}
		if createErr = writer.Close(); createErr != nil {
			t.Fatal(createErr)
		}
		request := httptest.NewRequest(http.MethodPost, endpoint, &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("upload %s status=%d body=%s", endpoint, response.Code, response.Body.String())
		}
		var attachment control.InputAttachment
		if createErr = json.Unmarshal(response.Body.Bytes(), &attachment); createErr != nil {
			t.Fatal(createErr)
		}
		if attachment.ID == "" || attachment.Name != name || attachment.Size != int64(len(content)) {
			t.Fatalf("unexpected uploaded attachment: %+v", attachment)
		}
		storedPath := filepath.Join(store.DataDir(), "input-attachments", attachment.ID, attachment.Name)
		if stored, statErr := os.ReadFile(storedPath); statErr != nil || string(stored) != content {
			t.Fatalf("server-side attachment content=%q err=%v", stored, statErr)
		}
		return attachment
	}

	for _, attachment := range []control.InputAttachment{
		upload("/api/tasks/attachments", "audit-image.tar", "task image"),
	} {
		request := httptest.NewRequest(http.MethodDelete, "/api/input-attachments/"+attachment.ID, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
		}
		if _, statErr := os.Stat(filepath.Join(store.DataDir(), "input-attachments", attachment.ID)); !os.IsNotExist(statErr) {
			t.Fatalf("staged attachment directory still exists: %v", statErr)
		}
	}
}

func TestConciergeAttachmentUploadIsConversationScopedAndDownloadable(t *testing.T) {
	dataDir := t.TempDir()
	store, err := control.NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveConfig(control.SaveConfigInput{Provider: "test", Model: "test", AuthMode: "environment", Workspace: t.TempDir(), Concurrency: 1, ApprovalMode: "none"}); err != nil {
		t.Fatal(err)
	}
	manager, err := control.NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	conversation, err := store.CreateConciergeConversation()
	if err != nil {
		t.Fatal(err)
	}
	router := buildRouter(store, manager, dataDir)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "brief.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(file, "concierge task brief"); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/concierge/conversations/"+conversation.ID+"/attachments", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("concierge upload status=%d body=%s", response.Code, response.Body.String())
	}
	var attachment control.InputAttachment
	if err = json.Unmarshal(response.Body.Bytes(), &attachment); err != nil {
		t.Fatal(err)
	}
	if attachment.Scope != "concierge" || attachment.OwnerID != conversation.ID || attachment.Name != "brief.md" {
		t.Fatalf("unexpected concierge attachment: %+v", attachment)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(http.MethodGet, "/api/input-attachments/"+attachment.ID, nil))
	if download.Code != http.StatusOK || download.Body.String() != "concierge task brief" {
		t.Fatalf("download status=%d body=%q", download.Code, download.Body.String())
	}

	missing := httptest.NewRecorder()
	badBody := bytes.NewBufferString("not multipart")
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/api/concierge/conversations/missing/attachments", badBody))
	if missing.Code == http.StatusCreated {
		t.Fatal("attachment upload unexpectedly accepted a missing concierge conversation")
	}
}
