package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestKickUserAuthorizationURLUsesPKCEAndChannelScopes(t *testing.T) {
	m := &KickUserAuthManager{}
	raw, err := m.begin("client-123")
	if err != nil {
		t.Fatalf("begin authorization: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	if u.Scheme != "https" || u.Host != "id.kick.com" || u.Path != "/oauth/authorize" {
		t.Fatalf("unexpected authorization endpoint: %s", raw)
	}
	q := u.Query()
	for key, want := range map[string]string{
		"response_type":         "code",
		"client_id":             "client-123",
		"redirect":              "127.0.0.1",
		"redirect_uri":          kickUserOAuthRedirectURI,
		"scope":                 kickUserOAuthScopes,
		"code_challenge_method": "S256",
	} {
		if got := q.Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if len(q.Get("state")) < 32 || len(q.Get("code_challenge")) < 32 {
		t.Fatalf("authorization URL missing strong state/PKCE values: %s", raw)
	}
	state := m.state(true)
	if !state.Pending || state.Authorized {
		t.Fatalf("unexpected local auth state: %+v", state)
	}
}

func TestStreamUpdateUsesUserAccessTokenForPatch(t *testing.T) {
	oldAPIBase := kickAPIBase
	oldCategoriesBase := kickCategoriesAPIBase
	defer func() {
		kickAPIBase = oldAPIBase
		kickCategoriesAPIBase = oldCategoriesBase
	}()

	var patchAuth string
	patched := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/public/v1/channels":
			patchAuth = r.Header.Get("Authorization")
			if patchAuth != "Bearer user-token" {
				http.Error(w, "wrong token", http.StatusForbidden)
				return
			}
			patched = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/public/v1/channels" && r.URL.Query().Get("slug") != "":
			if r.Header.Get("Authorization") != "Bearer app-token" {
				http.Error(w, "wrong app read token", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"broadcaster_user_id":123,"slug":"sleepykev","stream_title":"Old title","category":{"id":10,"name":"Old Category"},"stream":{"is_live":true}}],"message":"OK"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/public/v1/channels":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				http.Error(w, "wrong user read token", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if patched {
				_, _ = w.Write([]byte(`{"data":[{"broadcaster_user_id":123,"slug":"sleepykev","stream_title":"Updated title","category":{"id":42,"name":"Just Chatting"},"stream":{"is_live":true}}],"message":"OK"}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[{"broadcaster_user_id":123,"slug":"sleepykev","stream_title":"Old title","category":{"id":10,"name":"Old Category"},"stream":{"is_live":true}}],"message":"OK"}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	kickAPIBase = server.URL + "/public/v1"
	kickCategoriesAPIBase = server.URL + "/public/v2"

	chat := newChatManager(t.TempDir())
	chat.mu.Lock()
	chat.clientID = "client-id"
	chat.clientSecret = "client-secret"
	chat.accessToken = "app-token"
	chat.accessTokenExpiresAt = time.Now().Add(time.Hour)
	chat.authMode = "app"
	chat.connectedChannel = "sleepykev"
	chat.broadcasterUserID = "123"
	chat.mu.Unlock()

	app := &App{
		chat: chat,
		streamAuth: &KickUserAuthManager{
			accessToken:  "user-token",
			refreshToken: "refresh-token",
			scope:        kickUserOAuthScopes,
			expiresAt:    time.Now().Add(time.Hour),
		},
	}

	body := strings.NewReader(`{"title":"Updated title","category_id":42,"category_name":"Just Chatting"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/stream/update", body)
	rec := httptest.NewRecorder()
	app.handleKickStreamUpdate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stream update status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if patchAuth != "Bearer user-token" {
		t.Fatalf("PATCH used %q instead of user access token", patchAuth)
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["title"] != "Updated title" || response["category_name"] != "Just Chatting" {
		t.Fatalf("unexpected update response: %#v", response)
	}
}

func TestStreamUpdateRequiresUserAuthorization(t *testing.T) {
	chat := newChatManager(t.TempDir())
	chat.mu.Lock()
	chat.clientID = "client-id"
	chat.clientSecret = "client-secret"
	chat.accessToken = "app-token"
	chat.accessTokenExpiresAt = time.Now().Add(time.Hour)
	chat.authMode = "app"
	chat.connectedChannel = "sleepykev"
	chat.mu.Unlock()
	app := &App{chat: chat, streamAuth: &KickUserAuthManager{}}

	req := httptest.NewRequest(http.MethodPost, "/api/stream/update", strings.NewReader(`{"title":"Test","category_id":42}`))
	rec := httptest.NewRecorder()
	app.handleKickStreamUpdate(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Authorize Stream Controls first") {
		t.Fatalf("unexpected error: %s", rec.Body.String())
	}
}

func TestKickUserAuthorizationCredentialsStayOutOfBackups(t *testing.T) {
	for _, rel := range []string{"kick_user_authorization.json", ".kick-user-auth-123.tmp"} {
		if !skipBackupRelative(rel) {
			t.Fatalf("%s must be excluded from backups", rel)
		}
	}
	if !preserveRestoreRelative("kick_user_authorization.json") {
		t.Fatal("restore must preserve local Kick user authorization")
	}
}

func TestStreamUpdateRejectsDifferentAuthorizedKickAccount(t *testing.T) {
	oldAPIBase := kickAPIBase
	defer func() { kickAPIBase = oldAPIBase }()

	patchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/public/v1/channels" && r.URL.Query().Get("slug") != "":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"broadcaster_user_id":123,"slug":"sleepykev","stream_title":"Old title","category":{"id":10,"name":"Old Category"}}],"message":"OK"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/public/v1/channels":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				http.Error(w, "wrong user token", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"broadcaster_user_id":999,"slug":"otheraccount","stream_title":"Other title","category":{"id":20,"name":"Other Category"}}],"message":"OK"}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/public/v1/channels":
			patchCalls++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	kickAPIBase = server.URL + "/public/v1"

	chat := newChatManager(t.TempDir())
	chat.mu.Lock()
	chat.clientID = "client-id"
	chat.clientSecret = "client-secret"
	chat.accessToken = "app-token"
	chat.accessTokenExpiresAt = time.Now().Add(time.Hour)
	chat.authMode = "app"
	chat.connectedChannel = "sleepykev"
	chat.broadcasterUserID = "123"
	chat.mu.Unlock()
	app := &App{chat: chat, streamAuth: &KickUserAuthManager{accessToken: "user-token", refreshToken: "refresh-token", scope: kickUserOAuthScopes, expiresAt: time.Now().Add(time.Hour)}}

	req := httptest.NewRequest(http.MethodPost, "/api/stream/update", strings.NewReader(`{"title":"Should not change","category_id":42,"category_name":"Just Chatting"}`))
	rec := httptest.NewRecorder()
	app.handleKickStreamUpdate(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if patchCalls != 0 {
		t.Fatalf("PATCH was called %d times for mismatched authorized account", patchCalls)
	}
	if !strings.Contains(rec.Body.String(), "otheraccount") || !strings.Contains(rec.Body.String(), "sleepykev") {
		t.Fatalf("mismatch error should name both accounts: %s", rec.Body.String())
	}
}
