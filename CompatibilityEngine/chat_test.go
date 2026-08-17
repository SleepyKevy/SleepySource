package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestNormalizeChatSettings(t *testing.T) {
	s := defaultChatSettings()
	s.CanvasWidth = 99999
	s.CanvasHeight = 1
	s.BoxWidth = 1
	s.BoxHeight = 99999
	s.FontSize = 500
	s.UsernameSize = 1
	s.BackgroundOpacity = 500
	s.MessageColor = "bad"
	s.Direction = "sideways"
	s.Animation = "explode"
	normalizeChatSettings(&s)
	if s.CanvasWidth != 3840 || s.CanvasHeight != 240 {
		t.Fatalf("canvas normalized to %dx%d", s.CanvasWidth, s.CanvasHeight)
	}
	if s.BoxWidth != 180 || s.BoxHeight != 2160 {
		t.Fatalf("box normalized to %dx%d", s.BoxWidth, s.BoxHeight)
	}
	if s.FontSize != 96 || s.UsernameSize != 10 || s.BackgroundOpacity != 100 {
		t.Fatalf("chat numeric bounds not applied: %+v", s)
	}
	if s.MessageColor != "#FFFFFF" || s.Direction != "bottom-up" || s.Animation != "slide-up" {
		t.Fatalf("chat defaults not restored: %+v", s)
	}
}

func TestChatSettingsPersistAcrossSaves(t *testing.T) {
	dir := t.TempDir()
	m := newChatManager(dir)
	s := m.state().Settings
	s.FontSize = 31
	s.SevenTVEnabled = true
	s.KickChannel = "TheSleepyKev"
	if err := m.setSettings(s); err != nil {
		t.Fatal(err)
	}
	s.FontSize = 33
	if err := m.setSettings(s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "chat_settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved ChatSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.FontSize != 33 || saved.KickChannel != "TheSleepyKev" {
		t.Fatalf("persisted chat settings = %+v", saved)
	}
}

func TestOfficialKickWebhookChatIngest(t *testing.T) {
	app := newTestApp(t)
	h := app.routes()
	body := `{
	  "message_id":"msg-123",
	  "sender":{"user_id":987,"username":"ModViewer","profile_picture":"https://example.com/a.webp","identity":{"username_color":"#FF5733","badges":[{"text":"Moderator","type":"moderator"}]}},
	  "content":"Hello 7TV",
	  "created_at":"2026-08-15T12:00:00Z"
	}`
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/chat/ingest", strings.NewReader(body))
	req.Host = "127.0.0.1:17891"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:17891")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("ingest status=%d body=%s", rec.Code, rec.Body.String())
	}
	state := app.chat.state()
	if len(state.Messages) != 1 {
		t.Fatalf("messages=%d", len(state.Messages))
	}
	msg := state.Messages[0]
	if msg.ID != "msg-123" || msg.UserID != "987" || msg.Username != "ModViewer" || msg.Color != "#FF5733" || !msg.IsMod {
		t.Fatalf("normalized webhook message = %+v", msg)
	}
	if len(msg.Badges) != 1 || msg.Badges[0] != "Moderator" {
		t.Fatalf("badges = %#v", msg.Badges)
	}
}

func TestChatCustomFontUploadSelectsFontAndPersists(t *testing.T) {
	app := newTestApp(t)
	h := app.routes()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("font", "Chat Custom.ttf")
	if err != nil {
		t.Fatal(err)
	}
	fontBytes := append([]byte{0x00, 0x01, 0x00, 0x00}, make([]byte, 28)...)
	if _, err := part.Write(fontBytes); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/chat/upload-font", &buf)
	req.Host = "127.0.0.1:17891"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", "http://127.0.0.1:17891")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat font upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Settings ChatSettings `json:"settings"`
		Fonts    []FontInfo   `json:"fonts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Settings.FontFamily != "NPF_Chat_Custom" {
		t.Fatalf("chat font family=%q", result.Settings.FontFamily)
	}
	if len(result.Fonts) != 1 || result.Fonts[0].Family != "NPF_Chat_Custom" {
		t.Fatalf("fonts=%+v", result.Fonts)
	}
	data, err := os.ReadFile(filepath.Join(app.dataDir, "chat_settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved ChatSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.FontFamily != "NPF_Chat_Custom" {
		t.Fatalf("persisted chat font family=%q", saved.FontFamily)
	}
}

func TestChatSettingsRouteIsOriginProtected(t *testing.T) {
	app := newTestApp(t)
	h := app.routes()
	payload := `{"font_size":42}`
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/chat/settings", strings.NewReader(payload))
	req.Host = "127.0.0.1:17891"
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign chat settings status=%d", rec.Code)
	}
}

func TestChatSettingsRejectTrailingJSON(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/chat/settings", strings.NewReader(`{"font_size":42}{"font_size":43}`))
	req.Host = "127.0.0.1:17891"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:17891")
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("trailing chat settings JSON status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCorruptChatSettingsAreBackedUpAndReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chat_settings.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}
	m := newChatManager(dir)
	if m.state().Settings.CanvasWidth != 900 {
		t.Fatalf("chat defaults were not restored: %+v", m.state().Settings)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "chat_settings.corrupt-*.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("corrupt chat backup matches=%v err=%v", matches, err)
	}
}

func TestChatReloadFromDiskBacksUpCorruptSettingsAndClearsRuntimeDiagnostics(t *testing.T) {
	dir := t.TempDir()
	m := newChatManager(dir)
	m.mu.Lock()
	m.webhookRequestCount = 9
	m.webhookVerifiedCount = 8
	m.webhookAcceptedCount = 7
	m.webhookRejectedCount = 6
	m.webhookLastRequestAt = time.Now().UnixMilli()
	m.webhookLastEventType = "chat.message.sent"
	m.webhookLastError = "old error"
	m.mu.Unlock()

	if err := os.WriteFile(filepath.Join(dir, "chat_settings.json"), []byte("{bad restored json"), 0644); err != nil {
		t.Fatal(err)
	}
	m.reloadFromDisk()
	st := m.state()
	if st.Settings.CanvasWidth != 900 || st.WebhookRequestCount != 0 || st.WebhookVerifiedCount != 0 || st.WebhookAcceptedCount != 0 || st.WebhookRejectedCount != 0 || st.WebhookLastRequestAt != 0 || st.WebhookLastEventType != "" || st.WebhookLastError != "" {
		t.Fatalf("reloaded chat state retained stale/corrupt data: %+v", st)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "chat_settings.corrupt-*.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("restored corrupt chat backup matches=%v err=%v", matches, err)
	}
}

func TestTrustedKickAvatarURL(t *testing.T) {
	good := []string{"https://kick.com/avatar.webp", "https://files.kick.com/images/a.png", "https://cdn.kick.com/a.jpg"}
	for _, raw := range good {
		if _, ok := isTrustedKickImageURL(raw); !ok {
			t.Fatalf("expected trusted Kick URL: %s", raw)
		}
	}
	bad := []string{"http://files.kick.com/a.png", "https://kick.com.evil.example/a.png", "https://127.0.0.1/a.png", "file:///tmp/a.png"}
	for _, raw := range bad {
		if _, ok := isTrustedKickImageURL(raw); ok {
			t.Fatalf("unexpected trusted URL: %s", raw)
		}
	}
}

func TestModerationRouteRemoved(t *testing.T) {
	app := newTestApp(t)
	h := app.routes()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/api/chat/mod", nil)
	req.Host = "127.0.0.1:17891"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:17891")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("removed moderation route status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNormalizeKickChannelSlug(t *testing.T) {
	cases := map[string]string{
		"TheSleepyKev":                  "TheSleepyKev",
		"@TheSleepyKev":                 "TheSleepyKev",
		"https://kick.com/TheSleepyKev": "TheSleepyKev",
		"kick.com/TheSleepyKev/":        "TheSleepyKev",
	}
	for input, want := range cases {
		if got := normalizeKickChannelSlug(input); got != want {
			t.Fatalf("normalizeKickChannelSlug(%q)=%q want %q", input, got, want)
		}
	}
}

func TestKickChannelLookupRequiresSessionToken(t *testing.T) {
	app := newTestApp(t)
	s := app.chat.state().Settings
	s.KickChannel = "TheSleepyKev"
	if err := app.chat.setSettings(s); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/api/chat/channel", nil)
	req.Host = "127.0.0.1:17891"
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("lookup without token status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestKickChannelLookupUsesUsername(t *testing.T) {
	oldBase := kickAPIBase
	defer func() { kickAPIBase = oldBase }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels" || r.Method != http.MethodGet {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("slug"); got != "TheSleepyKev" {
			t.Fatalf("slug = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"broadcaster_user_id":12345,"slug":"TheSleepyKev"}],"message":"OK"}`))
	}))
	defer server.Close()
	kickAPIBase = server.URL

	app := newTestApp(t)
	s := app.chat.state().Settings
	s.KickChannel = "TheSleepyKev"
	if err := app.chat.setSettings(s); err != nil {
		t.Fatal(err)
	}
	app.chat.setAccessToken("test-token")

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/api/chat/channel", nil)
	req.Host = "127.0.0.1:17891"
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("lookup status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"channel":"TheSleepyKev"`) || !strings.Contains(rec.Body.String(), `"broadcaster_user_id":"12345"`) {
		t.Fatalf("lookup response=%s", rec.Body.String())
	}
}

func TestKickChatSendRouteRemoved(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/api/chat/send", nil)
	req.Host = "127.0.0.1:17891"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:17891")
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("removed send route status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestKickConnectCreatesAppTokenAndResolvesChannel(t *testing.T) {
	oldAPI := kickAPIBase
	oldOAuth := kickOAuthBase
	defer func() {
		kickAPIBase = oldAPI
		kickOAuthBase = oldOAuth
	}()

	var oauthCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			oauthCalls++
			if r.Method != http.MethodPost {
				t.Fatalf("oauth method = %s", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "cid" || r.Form.Get("client_secret") != "secret" {
				t.Fatalf("unexpected oauth form: %#v", r.Form)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"app-token","token_type":"Bearer","expires_in":3600}`))
		case "/channels":
			if got := r.Header.Get("Authorization"); got != "Bearer app-token" {
				t.Fatalf("authorization = %q", got)
			}
			if got := r.URL.Query().Get("slug"); got != "TheSleepyKev" {
				t.Fatalf("slug = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"broadcaster_user_id":12345,"slug":"TheSleepyKev"}],"message":"OK"}`))
		case "/events/subscriptions":
			if got := r.Header.Get("Authorization"); got != "Bearer app-token" {
				t.Fatalf("event authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"data":[],"message":"OK"}`))
				return
			}
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"data":[{"name":"chat.message.sent","version":1,"subscription_id":"sub-123"},{"name":"channel.followed","version":1,"subscription_id":"sub-follow"},{"name":"channel.subscription.new","version":1,"subscription_id":"sub-new"},{"name":"channel.subscription.renewal","version":1,"subscription_id":"sub-renew"},{"name":"channel.subscription.gifts","version":1,"subscription_id":"sub-gifts"},{"name":"kicks.gifted","version":1,"subscription_id":"sub-kicks"},{"name":"channel.reward.redemption.updated","version":1,"subscription_id":"sub-reward"}],"message":"OK"}`))
				return
			}
			http.Error(w, "method", http.StatusMethodNotAllowed)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	kickAPIBase = server.URL
	kickOAuthBase = server.URL

	app := newTestApp(t)
	s := app.chat.state().Settings
	s.KickChannel = "TheSleepyKev"
	if err := app.chat.setSettings(s); err != nil {
		t.Fatal(err)
	}
	body := strings.NewReader(`{"channel":"TheSleepyKev","client_id":"cid","client_secret":"secret"}`)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/chat/connect", body)
	req.Host = "127.0.0.1:17891"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:17891")
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status=%d body=%s", rec.Code, rec.Body.String())
	}
	if oauthCalls != 1 {
		t.Fatalf("oauth calls=%d want 1", oauthCalls)
	}
	state := app.chat.state()
	if !state.AuthReady || state.AuthMode != "app" || state.ConnectedChannel != "TheSleepyKev" || state.BroadcasterUserID != "12345" || !state.WebhookSubscribed || state.WebhookSubscriptionID != "sub-123" {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestKickAppTokenDoesNotUsePrivateFallback(t *testing.T) {
	oldAPI := kickAPIBase
	defer func() { kickAPIBase = oldAPI }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/channels" && r.URL.Query().Get("slug") == "TheSleepyKev" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"data":"broadcaster_user_id is required for app tokens","message":"Bad request"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	kickAPIBase = server.URL

	_, _, err := resolveKickBroadcasterUserID("TheSleepyKev", "app-token")
	if err == nil || !strings.Contains(err.Error(), "official channel lookup failed") {
		t.Fatalf("expected official lookup error, got %v", err)
	}
}

func TestKickAppTokenIsReusedUntilExpiry(t *testing.T) {
	oldOAuth := kickOAuthBase
	defer func() { kickOAuthBase = oldOAuth }()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"cached-token","expires_in":3600}`))
	}))
	defer server.Close()
	kickOAuthBase = server.URL

	m := newChatManager(t.TempDir())
	m.setAppCredentials("cid", "secret")
	first, err := m.ensureKickAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.ensureKickAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if first != "cached-token" || second != "cached-token" || calls != 1 {
		t.Fatalf("first=%q second=%q calls=%d", first, second, calls)
	}
}

func TestVerifiedKickWebhookReachesOverlayFeedThroughPublicHost(t *testing.T) {
	oldAPI := kickAPIBase
	defer func() { kickAPIBase = oldAPI }()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public-key" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"public_key": publicPEM}, "message": "OK"})
	}))
	defer server.Close()
	kickAPIBase = server.URL
	kickPublicKeyCache.Lock()
	kickPublicKeyCache.base = ""
	kickPublicKeyCache.key = nil
	kickPublicKeyCache.at = time.Time{}
	kickPublicKeyCache.Unlock()

	body := []byte(`{"message_id":"evt-msg-1","content":"Hello from official Kick webhook","created_at":"2026-08-15T20:30:00Z","sender":{"user_id":42,"username":"KickViewer","profile_picture":"https://files.kick.com/a.webp","identity":{"username_color":"#55B7FF","badges":[{"text":"Moderator","type":"moderator"}]}}}`)
	messageID := "01TESTEVENTMESSAGE"
	timestamp := "2026-08-15T20:30:01Z"
	signed := []byte(messageID + "." + timestamp + "." + string(body))
	hash := sha256.Sum256(signed)
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}

	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "https://public-tunnel.example/api/chat/kick-webhook", strings.NewReader(string(body)))
	req.Host = "public-tunnel.example"
	req.Header.Set("Kick-Event-Message-Id", messageID)
	req.Header.Set("Kick-Event-Message-Timestamp", timestamp)
	req.Header.Set("Kick-Event-Signature", base64.StdEncoding.EncodeToString(sig))
	req.Header.Set("Kick-Event-Type", "chat.message.sent")
	req.Header.Set("Kick-Event-Version", "1")
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("webhook status=%d body=%s", rec.Code, rec.Body.String())
	}
	state := app.chat.state()
	if len(state.Messages) != 1 || state.Messages[0].Text != "Hello from official Kick webhook" || state.Messages[0].Username != "KickViewer" {
		t.Fatalf("unexpected webhook message state: %+v", state)
	}
	if !state.LiveChatConnected || !state.WebhookSubscribed || state.WebhookLastEventAt == 0 {
		t.Fatalf("webhook live state not recorded: %+v", state)
	}
	if state.WebhookRequestCount != 1 || state.WebhookVerifiedCount != 1 || state.WebhookAcceptedCount != 1 || state.WebhookRejectedCount != 0 {
		t.Fatalf("verified webhook diagnostics not recorded: %+v", state)
	}
	if state.WebhookLastRequestAt == 0 || state.WebhookLastEventType != "chat.message.sent" || state.WebhookLastError != "" {
		t.Fatalf("unexpected verified webhook diagnostics: %+v", state)
	}

	// Replaying the same correctly signed Kick event must be idempotent.
	replayReq := httptest.NewRequest(http.MethodPost, "https://public-tunnel.example/api/chat/kick-webhook", strings.NewReader(string(body)))
	replayReq.Host = "public-tunnel.example"
	replayReq.Header.Set("Kick-Event-Message-Id", messageID)
	replayReq.Header.Set("Kick-Event-Message-Timestamp", timestamp)
	replayReq.Header.Set("Kick-Event-Signature", base64.StdEncoding.EncodeToString(sig))
	replayReq.Header.Set("Kick-Event-Type", "chat.message.sent")
	replayReq.Header.Set("Kick-Event-Version", "1")
	replayRec := httptest.NewRecorder()
	app.routes().ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusNoContent {
		t.Fatalf("replayed webhook status=%d body=%s", replayRec.Code, replayRec.Body.String())
	}
	replayedState := app.chat.state()
	if len(replayedState.Messages) != 1 || replayedState.WebhookAcceptedCount != 1 {
		t.Fatalf("replayed webhook was accepted twice: %+v", replayedState)
	}
	if replayedState.WebhookRequestCount != 2 || replayedState.WebhookVerifiedCount != 2 || replayedState.WebhookRejectedCount != 0 {
		t.Fatalf("replayed webhook diagnostics unexpected: %+v", replayedState)
	}
}

func TestRefreshKickChatWebhookSubscriptionDeletesOldAndCreatesFresh(t *testing.T) {
	oldAPI := kickAPIBase
	defer func() { kickAPIBase = oldAPI }()

	var deletedIDs []string
	postCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events/subscriptions" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer app-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":[{"id":"old-chat-1","event":"chat.message.sent","version":1,"broadcaster_user_id":12345},{"id":"other-1","event":"channel.followed","version":1,"broadcaster_user_id":12345}],"message":"OK"}`))
		case http.MethodDelete:
			deletedIDs = append(deletedIDs, r.URL.Query()["id"]...)
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPost:
			postCalls++
			_, _ = w.Write([]byte(`{"data":[{"name":"chat.message.sent","version":1,"subscription_id":"fresh-chat-2"},{"name":"channel.followed","version":1,"subscription_id":"sub-follow"},{"name":"channel.subscription.new","version":1,"subscription_id":"sub-new"},{"name":"channel.subscription.renewal","version":1,"subscription_id":"sub-renew"},{"name":"channel.subscription.gifts","version":1,"subscription_id":"sub-gifts"},{"name":"kicks.gifted","version":1,"subscription_id":"sub-kicks"},{"name":"channel.reward.redemption.updated","version":1,"subscription_id":"sub-reward"}],"message":"OK"}`))
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	kickAPIBase = server.URL

	id, replaced, err := refreshKickChatWebhookSubscription("app-token", "12345")
	if err != nil {
		t.Fatal(err)
	}
	if id != "fresh-chat-2" || replaced != 2 || postCalls != 1 {
		t.Fatalf("id=%q replaced=%d postCalls=%d", id, replaced, postCalls)
	}
	sort.Strings(deletedIDs)
	if len(deletedIDs) != 2 || deletedIDs[0] != "old-chat-1" || deletedIDs[1] != "other-1" {
		t.Fatalf("deleted IDs = %#v", deletedIDs)
	}
}

func TestKickWebhookDiagnosticsRecordRejectedRequest(t *testing.T) {
	app := newTestApp(t)
	body := `{"message_id":"bad-1","content":"not signed","sender":{"user_id":1,"username":"Viewer"}}`
	req := httptest.NewRequest(http.MethodPost, "https://public-tunnel.example/api/chat/kick-webhook", strings.NewReader(body))
	req.Host = "public-tunnel.example"
	req.Header.Set("Kick-Event-Type", "chat.message.sent")
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	state := app.chat.state()
	if state.WebhookRequestCount != 1 || state.WebhookRejectedCount != 1 || state.WebhookVerifiedCount != 0 || state.WebhookAcceptedCount != 0 {
		t.Fatalf("unexpected diagnostics: %+v", state)
	}
	if state.WebhookLastRequestAt == 0 || state.WebhookLastEventType != "chat.message.sent" || state.WebhookLastError == "" {
		t.Fatalf("missing rejected-request diagnostics: %+v", state)
	}
}

func TestKickWebhookPreservesBadgeTypesCountsAndBroadcaster(t *testing.T) {
	body := []byte(`{
	  "message_id":"badge-msg-1",
	  "broadcaster":{"user_id":42,"username":"TheSleepyKev"},
	  "sender":{"user_id":42,"username":"TheSleepyKev","profile_picture":"https://files.kick.com/test.webp","identity":{"username_color":"#53FC18","badges":[{"text":"Moderator","type":"moderator"},{"text":"Subscriber","type":"subscriber","count":12},{"text":"OG","type":"og"}]}},
	  "content":"badge test",
	  "created_at":"2026-08-15T12:00:00Z"
	}`)
	msg, ok := parseKickWebhookChatMessage(body)
	if !ok {
		t.Fatal("expected valid Kick chat message")
	}
	if len(msg.BadgeDetails) != 4 {
		t.Fatalf("badge details = %#v", msg.BadgeDetails)
	}
	if msg.BadgeDetails[0].Type != "broadcaster" {
		t.Fatalf("first badge = %#v", msg.BadgeDetails[0])
	}
	var sub ChatBadge
	for _, b := range msg.BadgeDetails {
		if b.Type == "subscriber" {
			sub = b
		}
	}
	if sub.Count != 12 {
		t.Fatalf("subscriber badge count = %d", sub.Count)
	}
	if !msg.IsMod {
		t.Fatal("moderator badge should set IsMod")
	}
}

func TestKickRoleBadgeVariants(t *testing.T) {
	cases := map[int]string{0: "sub_gifter", 24: "sub_gifter", 25: "sub_gifter_25", 50: "sub_gifter_50", 100: "sub_gifter_100", 200: "sub_gifter_200", 500: "sub_gifter_200"}
	for count, want := range cases {
		if got := kickRoleBadgeVariant("sub_gifter", count); got != want {
			t.Fatalf("count %d: got %q want %q", count, got, want)
		}
	}
}

func TestValidKickEmoteID(t *testing.T) {
	valid := []string{"1", "37236", "4819423", "12345678901234567890"}
	for _, id := range valid {
		if !isValidKickEmoteID(id) {
			t.Fatalf("expected Kick emote ID %q to be valid", id)
		}
	}
	invalid := []string{"", "abc", "12-34", "123456789012345678901", "  "}
	for _, id := range invalid {
		if isValidKickEmoteID(id) {
			t.Fatalf("expected Kick emote ID %q to be invalid", id)
		}
	}
}

func TestKickConnectRejectedNewCredentialsDoNotReplaceExistingLogin(t *testing.T) {
	oldOAuth := kickOAuthBase
	defer func() { kickOAuthBase = oldOAuth }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
	}))
	defer server.Close()
	kickOAuthBase = server.URL

	app := newTestApp(t)
	if err := app.chat.setAppCredentials("good-client", "good-secret"); err != nil {
		t.Fatal(err)
	}
	body := strings.NewReader(`{"channel":"thesleepykev","client_id":"bad-client","client_secret":"bad-secret"}`)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/chat/connect", body)
	req.Host = "127.0.0.1:17891"
	req.Header.Set("Origin", "http://127.0.0.1:17891")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	app.chat.mu.RLock()
	clientID, clientSecret := app.chat.clientID, app.chat.clientSecret
	app.chat.mu.RUnlock()
	if clientID != "good-client" || clientSecret != "good-secret" {
		t.Fatalf("existing credentials were replaced: clientID=%q secret=%q", clientID, clientSecret)
	}
}

func TestInvalidKickWebhookSignatureDoesNotRefetchFreshPublicKey(t *testing.T) {
	oldAPI := kickAPIBase
	defer func() { kickAPIBase = oldAPI }()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"public_key": publicPEM}})
	}))
	defer server.Close()
	kickAPIBase = server.URL
	kickPublicKeyCache.Lock()
	kickPublicKeyCache.base = ""
	kickPublicKeyCache.key = nil
	kickPublicKeyCache.at = time.Time{}
	kickPublicKeyCache.Unlock()

	badSig := base64.StdEncoding.EncodeToString(make([]byte, 256))
	for i := 0; i < 2; i++ {
		if err := verifyKickWebhookSignature("message-id", "2026-08-15T20:30:01Z", []byte(`{"x":1}`), badSig); err == nil {
			t.Fatal("expected invalid signature")
		}
	}
	if calls != 1 {
		t.Fatalf("public key endpoint calls=%d, want 1", calls)
	}
}

func TestChatThemeAndAnimationSettingsNormalize(t *testing.T) {
	s := defaultChatSettings()
	s.Theme = "unknown"
	s.Animation = "explode"
	s.AnimationEasing = "teleport"
	s.MessageBorderWidth = 999
	s.MessageBorderColor = "bad"
	s.MessageRadius = 999
	s.BoxBlur = 999
	s.UsernameWeight = 777
	normalizeChatSettings(&s)
	if s.SchemaVersion != 6 || s.Theme != "midnight" || s.Animation != "slide-up" || s.AnimationEasing != "smooth" {
		t.Fatalf("chat theme/animation normalization = %+v", s)
	}
	if s.MessageBorderWidth != 12 || s.MessageBorderColor != "#2F78B7" || s.MessageRadius != 80 || s.BoxBlur != 40 || s.UsernameWeight != 777 {
		t.Fatalf("chat effect bounds = %+v", s)
	}
}
