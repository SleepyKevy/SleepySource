package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamSettingsKickAPI(t *testing.T) {
	var patched map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("missing bearer token: %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/public/v1/channels":
			writeJSON(w, map[string]any{"data": []any{map[string]any{
				"slug":         "sleepykev",
				"stream_title": "Late Night Stream",
				"category":     map[string]any{"id": 42, "name": "Just Chatting"},
				"stream":       map[string]any{"is_live": true},
			}}})
		case r.Method == http.MethodGet && r.URL.Path == "/public/v2/categories":
			if got := r.URL.Query().Get("name"); got != "Just" {
				t.Fatalf("category query=%q", got)
			}
			writeJSON(w, map[string]any{"data": []any{map[string]any{"id": 42, "name": "Just Chatting"}}})
		case r.Method == http.MethodPatch && r.URL.Path == "/public/v1/channels":
			if err := json.NewDecoder(r.Body).Decode(&patched); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	oldAPI, oldCategories := kickAPIBase, kickCategoriesAPIBase
	kickAPIBase = srv.URL + "/public/v1"
	kickCategoriesAPIBase = srv.URL + "/public/v2"
	defer func() { kickAPIBase, kickCategoriesAPIBase = oldAPI, oldCategories }()

	meta, err := fetchKickChannelMetadata("sleepykev", "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Late Night Stream" || meta.CategoryID != 42 || meta.CategoryName != "Just Chatting" || !meta.IsLive {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	cats, err := searchKickCategories("Just", "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 1 || cats[0].ID != 42 || cats[0].Name != "Just Chatting" {
		t.Fatalf("unexpected categories: %+v", cats)
	}
	if err := patchKickChannelMetadata("test-token", "New Title", 42); err != nil {
		t.Fatal(err)
	}
	if patched["stream_title"] != "New Title" || int64(patched["category_id"].(float64)) != 42 {
		t.Fatalf("unexpected PATCH payload: %+v", patched)
	}
}

func TestStreamSettingsIsMainModule(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, required := range []string{
		`data-module="stream-settings"`,
		`data-module-label="Stream Dashboard"`,
		`class="streamSettingsTabIcon" src="/assets/stream-settings-icon.png"`,
		`id="streamSettingsWorkspace" data-workspace-panel="stream-settings"`,
		`id="homeStreamTitleInput"`,
		`id="homeStreamCategoryInput"`,
		`id="homeStreamUpdateBtn"`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Stream Dashboard module is missing %q", required)
		}
	}
	for _, forbidden := range []string{`id="homeStreamMenu"`, `id="streamSettingsSidebar"`, `data-section="stream-settings-status"`, `<summary>Kick Stream Dashboard</summary>`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("Stream Dashboard must use its main workspace without a sidebar dropdown: found %q", forbidden)
		}
	}

	s := defaultSettings()
	s.LastModule = "stream-settings"
	normalizeSettings(&s)
	if s.LastModule != "stream-settings" {
		t.Fatalf("stream-settings should be valid LastModule, got %q", s.LastModule)
	}
}

func TestFetchKickActiveLivestreamMetadata(t *testing.T) {
	live := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer user-token" {
			t.Fatalf("missing bearer token: %q", r.Header.Get("Authorization"))
		}
		if r.Method != http.MethodGet || r.URL.Path != "/public/v1/users/livestreams" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("user_id"); got != "123" {
			t.Fatalf("user_id=%q", got)
		}
		if !live {
			writeJSON(w, map[string]any{"data": []any{}})
			return
		}
		writeJSON(w, map[string]any{"data": []any{map[string]any{
			"broadcaster_user": map[string]any{"id": 123},
			"channel":          map[string]any{"slug": "sleepykev"},
			"title":            "Verified Live Title",
			"category":         map[string]any{"id": 42, "name": "Just Chatting"},
		}}})
	}))
	defer srv.Close()

	oldAPI := kickAPIBase
	kickAPIBase = srv.URL + "/public/v1"
	defer func() { kickAPIBase = oldAPI }()

	meta, isLive, err := fetchKickActiveLivestreamMetadata(123, "user-token")
	if err != nil {
		t.Fatal(err)
	}
	if !isLive || !meta.IsLive || meta.Title != "Verified Live Title" || meta.CategoryID != 42 || meta.CategoryName != "Just Chatting" || meta.ChannelSlug != "sleepykev" {
		t.Fatalf("unexpected live metadata: live=%v meta=%+v", isLive, meta)
	}

	live = false
	meta, isLive, err = fetchKickActiveLivestreamMetadata(123, "user-token")
	if err != nil {
		t.Fatal(err)
	}
	if isLive || meta.IsLive || meta.BroadcasterUserID != 123 {
		t.Fatalf("unexpected offline metadata: live=%v meta=%+v", isLive, meta)
	}
}

func TestStreamSettingsOfflineReadbackMessaging(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, required := range []string{
		"Unavailable while offline",
		"Kick does not expose offline title/category for reliable read-back",
		"metadata_readback_available",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("offline Stream Dashboard guidance is missing %q", required)
		}
	}
}
