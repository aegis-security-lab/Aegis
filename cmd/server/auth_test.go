package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aegis/apps/board/control"
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
	headerProbe := httptest.NewRecorder()
	router.ServeHTTP(headerProbe, httptest.NewRequest(http.MethodGet, "/", nil))
	for name, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := headerProbe.Header().Get(name); got != want {
			t.Fatalf("%s=%q, want %q", name, got, want)
		}
	}

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
	var session map[string]any
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if _, exposed := session["token"]; exposed {
		t.Fatal("login exposed the session token to JavaScript")
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
	logout := httptest.NewRecorder()
	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutRequest.AddCookie(cookies[0])
	router.ServeHTTP(logout, logoutRequest)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", logout.Code)
	}
	loggedOut := httptest.NewRecorder()
	loggedOutRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	loggedOutRequest.AddCookie(cookies[0])
	router.ServeHTTP(loggedOut, loggedOutRequest)
	if loggedOut.Code != http.StatusUnauthorized {
		t.Fatalf("server session survived logout: status = %d", loggedOut.Code)
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
