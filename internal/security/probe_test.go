package security

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPProbe_Success(t *testing.T) {
	body := `{"status":"ok","data":"hello world"}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.UserAgent() != "Aegis-Test/1.0" {
			t.Errorf("unexpected UA: %s", r.UserAgent())
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
	}))
	defer ts.Close()

	result := HTTPProbe(ts.URL, ProbeConfig{UserAgent: "Aegis-Test/1.0"})

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", result.StatusCode, http.StatusOK)
	}
	if ct := result.Headers["Content-Type"]; len(ct) != 1 || ct[0] != "application/json" {
		t.Fatalf("unexpected Content-Type: %v", ct)
	}
	if !strings.Contains(result.BodyPreview, "hello world") {
		t.Fatalf("body preview missing expected text: %s", result.BodyPreview)
	}
	if result.DurationMs <= 0 {
		t.Fatal("duration should be positive")
	}
}

func TestHTTPProbe_DefaultConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	// Zero-value config should use defaults
	result := HTTPProbe(ts.URL, ProbeConfig{})

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", result.StatusCode, http.StatusNoContent)
	}
}

func TestHTTPProbe_NoRedirect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Without following redirects
	follow := false
	result := HTTPProbe(ts.URL+"/redirect", ProbeConfig{FollowRedirect: &follow})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", result.StatusCode)
	}
}

func TestHTTPProbe_WithClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "yes")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "custom-client-test")
	}))
	defer ts.Close()

	client := ts.Client()
	result := HTTPProbe(ts.URL, ProbeConfig{Client: client})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d", result.StatusCode)
	}
	if result.Headers["X-Custom"][0] != "yes" {
		t.Fatal("missing custom header")
	}
}

func TestHTTPProbe_InvalidURL(t *testing.T) {
	result := HTTPProbe("://invalid", ProbeConfig{})
	if result.Error == "" {
		t.Fatal("expected error for invalid URL")
	}
}

func TestHTTPProbe_BodyPreviewTrimmed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return body larger than 500 chars
		_, _ = io.WriteString(w, strings.Repeat("a", 1000))
	}))
	defer ts.Close()

	result := HTTPProbe(ts.URL, ProbeConfig{})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if len(result.BodyPreview) > 510 { // 500 + ellipsis
		t.Fatalf("body preview too long: %d", len(result.BodyPreview))
	}
	if !strings.HasSuffix(result.BodyPreview, "…") {
		t.Fatal("body preview should end with ellipsis when truncated")
	}
}

func TestHTTPProbe_CloseBodyOnAnyStatus(t *testing.T) {
	// Verify body is read even on error status
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "error details")
	}))
	defer ts.Close()

	result := HTTPProbe(ts.URL, ProbeConfig{})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status code = %d", result.StatusCode)
	}
	if !strings.Contains(result.BodyPreview, "error details") {
		t.Fatal("body should be read on error status")
	}
}

func TestHTTPProbe_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond - block indefinitely
		select {}
	}))
	defer ts.Close()

	result := HTTPProbe(ts.URL, ProbeConfig{Timeout: 1}) // 1ms - should timeout
	if result.Error == "" {
		t.Fatal("expected timeout error")
	}
}

func TestHTTPProbe_StatusCodeAndDuration(t *testing.T) {
	statusCodes := []int{200, 201, 301, 302, 400, 401, 403, 404, 500, 502, 503}
	for _, code := range statusCodes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer ts.Close()

			result := HTTPProbe(ts.URL, ProbeConfig{})
			if result.Error != "" {
				t.Fatalf("unexpected error: %s", result.Error)
			}
			if result.StatusCode != code {
				t.Fatalf("status code = %d, want %d", result.StatusCode, code)
			}
			if result.DurationMs < 0 {
				t.Fatal("duration should be non-negative")
			}
		})
	}
}
