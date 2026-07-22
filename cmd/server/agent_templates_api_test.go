package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aegis/internal/control"
)

func TestAgentTemplateAPIRejectsIdentityConflictsAndUpdatesNotes(t *testing.T) {
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
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	input := control.SaveAgentTemplateInput{
		Provider: "OpenAI", Model: "GPT-5", SystemPrompt: "You are a release engineer.", Note: "首条备注",
		Metadata: control.AgentTemplateMetadata{
			EnglishName: "release-engineer", ChineseName: "发布工程师",
			Introduction: "负责发布质量。", Positions: []string{"release", "backend"},
		},
	}
	response := requestJSON(http.MethodPost, "/api/agent-templates", input)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var created control.AgentTemplate
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Note != "首条备注" || len(created.ID) != 64 {
		t.Fatalf("created talent=%+v", created)
	}

	response = requestJSON(http.MethodPost, "/api/agent-templates", input)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "人才 ID 已存在") {
		t.Fatalf("conflict status=%d body=%s", response.Code, response.Body.String())
	}

	response = requestJSON(http.MethodPatch, "/api/agent-templates/"+created.ID+"/note", map[string]string{"note": "更新后的备注"})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "更新后的备注") {
		t.Fatalf("note status=%d body=%s", response.Code, response.Body.String())
	}
	updated, err := store.GetAgentTemplate(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Note != "更新后的备注" || updated.ID != created.ID {
		t.Fatalf("updated talent=%+v", updated)
	}
}
