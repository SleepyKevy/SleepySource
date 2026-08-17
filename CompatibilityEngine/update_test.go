package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.1", "v1.1.0", 0},
		{"1.3.2", "v1.3.3", -1},
		{"1.3.2", "v1.3.0", 1},
		{"v2.0", "2.0.0.0", 0},
	}
	for _, tt := range tests {
		got, err := compareVersions(tt.a, tt.b)
		if err != nil {
			t.Fatalf("compareVersions(%q,%q): %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Fatalf("compareVersions(%q,%q)=%d want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestFetchUpdateStatus(t *testing.T) {
	oldAPI, oldClient := githubLatestReleaseAPI, updateHTTPClient
	defer func() { githubLatestReleaseAPI, updateHTTPClient = oldAPI, oldClient }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" || r.Header.Get("X-GitHub-Api-Version") == "" {
			t.Fatal("expected GitHub API headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.3.3","name":"SleepySource 1.3.3","html_url":"https://example.invalid/release","body":"Fixes and polish","published_at":"2026-08-24T12:00:00Z"}`))
	}))
	defer server.Close()

	githubLatestReleaseAPI = server.URL
	updateHTTPClient = server.Client()
	status := fetchUpdateStatus(context.Background())
	if status.Status != "available" || !status.UpdateAvailable || status.LatestVersion != "1.3.3" {
		t.Fatalf("unexpected update status: %#v", status)
	}
	if status.ReleaseNotes != "Fixes and polish" || status.ReleaseURL != "https://example.invalid/release" {
		t.Fatalf("release metadata missing: %#v", status)
	}
}

func TestUpdateCheckHandlerMethod(t *testing.T) {
	app := &App{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/update", nil)
	app.handleUpdateCheck(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestUpdateRepositoryDestination(t *testing.T) {
	const want = "https://github.com/SleepyKevy/SleepySource"
	if updateRepositoryURL != want {
		t.Fatalf("updateRepositoryURL=%q want %q", updateRepositoryURL, want)
	}
}
