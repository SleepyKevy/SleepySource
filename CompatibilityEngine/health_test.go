package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelayHealthAllowsPublicProbeOnlyForSafeMethods(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r := httptest.NewRequest(method, "https://example.invalid/api/relay-health", nil)
		r.Host = "example.trycloudflare.com"
		if !requestOriginAllowed(r) {
			t.Fatalf("%s relay health probe should be allowed through the public tunnel", method)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "https://example.invalid/api/relay-health", strings.NewReader("x"))
	r.Host = "example.trycloudflare.com"
	if requestOriginAllowed(r) {
		t.Fatal("POST relay health probe must stay blocked for non-local hosts")
	}
}

func TestHealthHelpers(t *testing.T) {
	if !containsHealthError("Native Windows media-session detection unavailable: test") {
		t.Fatal("health error detector should recognize unavailable status")
	}
	if containsHealthError("Ready") {
		t.Fatal("healthy status must not be classified as an error")
	}
	if got := healthSuffix("Song"); got != ": Song" {
		t.Fatalf("health suffix=%q, want %q", got, ": Song")
	}
}

func TestSystemHealthRoutesAreRegistered(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/api/relay-health", nil)
	req.Host = "127.0.0.1:17891"
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("relay health status=%d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "ok" {
		t.Fatalf("relay health body=%q, want ok", got)
	}
}
