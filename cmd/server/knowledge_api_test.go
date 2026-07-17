package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aegis/internal/control"
)

func TestKnowledgeBaseAPI(t *testing.T) {
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

	requestJSON := func(method, path string, input any) *httptest.ResponseRecorder {
		var body bytes.Buffer
		if input != nil {
			if err := json.NewEncoder(&body).Encode(input); err != nil {
				t.Fatal(err)
			}
		}
		request := httptest.NewRequest(method, path, &body)
		if input != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	response := requestJSON(http.MethodPost, "/api/knowledge-bases", control.SaveKnowledgeBaseInput{Name: "工程规范", Description: "后端工程约束。", RetrievalProvider: control.KnowledgeProviderKeywordAI})
	if response.Code != http.StatusCreated {
		t.Fatalf("create knowledge base status=%d body=%s", response.Code, response.Body.String())
	}
	var base control.KnowledgeBase
	if err := json.Unmarshal(response.Body.Bytes(), &base); err != nil {
		t.Fatal(err)
	}
	response = requestJSON(http.MethodPost, "/api/knowledge-bases/"+base.ID+"/documents", control.SaveKnowledgeDocumentInput{Name: "go.md", Content: "# Go\n\n保持 handler 简洁。"})
	if response.Code != http.StatusCreated {
		t.Fatalf("create knowledge document status=%d body=%s", response.Code, response.Body.String())
	}
	var document control.KnowledgeDocument
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	response = requestJSON(http.MethodGet, "/api/knowledge-bases/"+base.ID, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("get knowledge base status=%d body=%s", response.Code, response.Body.String())
	}
	var detail control.KnowledgeBaseDetail
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.KnowledgeBase.DocumentCount != 1 || len(detail.Documents) != 1 {
		t.Fatalf("unexpected knowledge detail: %+v", detail)
	}
	response = requestJSON(http.MethodPut, "/api/knowledge-documents/"+document.ID, control.SaveKnowledgeDocumentInput{Name: "go-api.md", Content: "# API\n\n更新后的内容。"})
	if response.Code != http.StatusOK {
		t.Fatalf("update knowledge document status=%d body=%s", response.Code, response.Body.String())
	}
	response = requestJSON(http.MethodDelete, "/api/knowledge-documents/"+document.ID, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete knowledge document status=%d body=%s", response.Code, response.Body.String())
	}
	response = requestJSON(http.MethodDelete, "/api/knowledge-bases/"+base.ID, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete knowledge base status=%d body=%s", response.Code, response.Body.String())
	}
	response = requestJSON(http.MethodPost, "/api/internal/executions/not-running/knowledge/search", control.KnowledgeSearchInput{Query: "test"})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("knowledge search without execution token status=%d body=%s", response.Code, response.Body.String())
	}
}
