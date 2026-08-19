package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aegis/apps/board/control"
)

func TestUncoverProviderAPIStoresAndRedactsCredentials(t *testing.T) {
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
			if encodeErr := json.NewEncoder(&body).Encode(input); encodeErr != nil {
				t.Fatal(encodeErr)
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

	const secret = "never-return-this-key"
	response := requestJSON(http.MethodPut, "/api/tools/uncover/providers/shodan", control.SaveUncoverProviderInput{Values: map[string]string{"apiKey": secret}})
	if response.Code != http.StatusOK {
		t.Fatalf("save provider status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) || !strings.Contains(response.Body.String(), `"configured":true`) {
		t.Fatalf("save response leaked secret or omitted status: %s", response.Body.String())
	}

	response = requestJSON(http.MethodGet, "/api/tools/uncover/status", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status request status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), "providerConfigPath") {
		t.Fatalf("status response exposed provider storage details: %s", response.Body.String())
	}

	response = requestJSON(http.MethodPut, "/api/tools/uncover/providers/censys", control.SaveUncoverProviderInput{Values: map[string]string{"apiToken": "token-only"}})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("incomplete provider status=%d body=%s", response.Code, response.Body.String())
	}

	response = requestJSON(http.MethodDelete, "/api/tools/uncover/providers/shodan", nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete provider status=%d body=%s", response.Code, response.Body.String())
	}
	response = requestJSON(http.MethodGet, "/api/tools/uncover/status", nil)
	if strings.Contains(response.Body.String(), `"id":"shodan","name":"Shodan","configured":true`) {
		t.Fatalf("deleted provider remains configured: %s", response.Body.String())
	}
}
