package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aegis/internal/control"
)

func TestAuthenticatedRouterProtectsEveryAPI(t *testing.T) {
	store, err := control.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := control.NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	auth, err := newAuthService("correct horse battery staple", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := buildRouterWithAuth(store, manager, t.TempDir(), auth)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated API status = %d, want 401", unauthorized.Code)
	}

	wrongBody, _ := json.Marshal(map[string]string{"password": "wrong"})
	wrong := httptest.NewRecorder()
	wrongRequest := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(wrongBody))
	wrongRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wrong, wrongRequest)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401", wrong.Code)
	}

	loginBody, _ := json.Marshal(map[string]string{"password": "correct horse battery staple"})
	login := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, loginRequest)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", login.Code, login.Body.String())
	}
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Token == "" {
		t.Fatal("login did not return a token")
	}

	authorized := httptest.NewRecorder()
	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer "+session.Token)
	router.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("bearer-authenticated API status = %d", authorized.Code)
	}

	cookies := login.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName || !cookies[0].HttpOnly {
		t.Fatalf("login did not set protected session cookie: %#v", cookies)
	}
	cookieAuthorized := httptest.NewRecorder()
	cookieRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	cookieRequest.AddCookie(cookies[0])
	router.ServeHTTP(cookieAuthorized, cookieRequest)
	if cookieAuthorized.Code != http.StatusOK {
		t.Fatalf("cookie-authenticated API status = %d", cookieAuthorized.Code)
	}
}

func TestAccessTokenRequiresBearerScheme(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Basic abc")
	if token := accessToken(request); token != "" {
		t.Fatalf("accepted non-bearer token %q", token)
	}
	request.Header.Set("Authorization", "bearer  token-value ")
	if token := accessToken(request); !strings.EqualFold(token, "token-value") {
		t.Fatalf("parsed token = %q", token)
	}
}
