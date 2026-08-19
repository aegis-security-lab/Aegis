package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	boardapp "aegis/apps/board"
	"aegis/apps/board/control"
	platformapp "aegis/platform/application"
)

func TestApplicationsAPIListsOnlyExplicitlyRegisteredApps(t *testing.T) {
	store, err := control.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := control.NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	catalog := platformapp.NewCatalog()
	if err = catalog.Register(boardapp.Module{}); err != nil {
		t.Fatal(err)
	}
	router := buildRouterWithApplications(store, manager, t.TempDir(), nil, catalog)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/applications", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"aegis.board"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"name":"primary"`)) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
