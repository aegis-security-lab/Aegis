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

	"aegis/internal/control"
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
