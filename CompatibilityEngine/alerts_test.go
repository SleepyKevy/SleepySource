package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAlertQueueStartsOnlyWhenOverlayConsumes(t *testing.T) {
	m := newAlertManager(t.TempDir())
	if !m.enqueue(AlertEvent{Type: "follow", Source: "test", Username: "Viewer"}, "one") {
		t.Fatal("follow test alert was not queued")
	}
	idle := m.state(false)
	if idle.Current != nil || idle.QueueDepth != 1 {
		t.Fatalf("non-consuming state unexpectedly started alert: %+v", idle)
	}
	live := m.state(true)
	if live.Current == nil || live.Current.Type != "follow" || live.QueueDepth != 0 {
		t.Fatalf("overlay did not start queued alert: %+v", live)
	}
	if live.Current.StartedAtMS == 0 || live.Current.EndsAtMS <= live.Current.StartedAtMS {
		t.Fatalf("invalid alert timing: %+v", live.Current)
	}
}

func TestAlertQueueDedupeAndSequentialPlayback(t *testing.T) {
	m := newAlertManager(t.TempDir())
	if !m.enqueue(AlertEvent{Type: "follow", Username: "First"}, "same") {
		t.Fatal("first alert was not queued")
	}
	if m.enqueue(AlertEvent{Type: "follow", Username: "Duplicate"}, "same") {
		t.Fatal("duplicate alert was queued")
	}
	if !m.enqueue(AlertEvent{Type: "kicks", Username: "Second", Amount: 500, GiftName: "Rage Quit"}, "two") {
		t.Fatal("second alert was not queued")
	}
	first := m.state(true)
	if first.Current == nil || first.Current.Username != "First" || first.QueueDepth != 1 {
		t.Fatalf("unexpected first state: %+v", first)
	}
	m.mu.Lock()
	m.current.EndsAtMS = time.Now().UnixMilli() - 1
	m.mu.Unlock()
	second := m.state(true)
	if second.Current == nil || second.Current.Type != "kicks" || second.Current.Username != "Second" || second.QueueDepth != 0 {
		t.Fatalf("unexpected second state: %+v", second)
	}
}

func TestParseKickAlertEvents(t *testing.T) {
	cases := []struct {
		typ, body, user       string
		amount, count, months int
	}{
		{"channel.followed", `{"broadcaster":{"user_id":12345},"follower":{"user_id":2,"username":"Follower"}}`, "Follower", 0, 0, 0},
		{"channel.subscription.new", `{"broadcaster":{"user_id":12345},"subscriber":{"user_id":3,"username":"Subber"},"duration":1,"created_at":"2025-01-14T16:08:06Z"}`, "Subber", 0, 0, 1},
		{"channel.subscription.renewal", `{"broadcaster":{"user_id":12345},"subscriber":{"user_id":3,"username":"Subber"},"duration":6}`, "Subber", 0, 0, 6},
		{"channel.subscription.gifts", `{"broadcaster":{"user_id":12345},"gifter":{"user_id":4,"username":"Gifter"},"giftees":[{"user_id":5,"username":"A"},{"user_id":6,"username":"B"}]}`, "Gifter", 0, 2, 0},
		{"kicks.gifted", `{"broadcaster":{"user_id":12345},"sender":{"user_id":7,"username":"Sender"},"gift":{"amount":500,"name":"Rage Quit"}}`, "Sender", 500, 0, 0},
	}
	for _, tc := range cases {
		event, key, ok := parseKickAlertEvent(tc.typ, "message-id", []byte(tc.body))
		if !ok || key == "" || event.Username != tc.user || event.Amount != tc.amount || event.Count != tc.count || event.Months != tc.months {
			t.Fatalf("%s parsed incorrectly: ok=%v key=%q event=%+v", tc.typ, ok, key, event)
		}
	}
	reward, key, ok := parseKickAlertEvent("channel.reward.redemption.updated", "reward-msg", []byte(`{"id":"redemption-1","user_input":"Water break!","status":"pending","redeemed_at":"2025-12-02T22:54:19.323Z","reward":{"title":"Hydrate"},"redeemer":{"user_id":8,"username":"Viewer"},"broadcaster":{"user_id":12345}}`))
	if !ok || key != "kick:reward:redemption-1" || reward.RewardTitle != "Hydrate" || reward.UserInput != "Water break!" {
		t.Fatalf("reward parsed incorrectly: ok=%v key=%q event=%+v", ok, key, reward)
	}
}

func TestAlertHTTPTestAndOverlayRoutes(t *testing.T) {
	app := newTestApp(t)
	h := app.routes()
	overlayReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/alerts", nil)
	overlayReq.Host = "127.0.0.1:17891"
	overlayRec := httptest.NewRecorder()
	h.ServeHTTP(overlayRec, overlayReq)
	if overlayRec.Code != http.StatusOK || !strings.Contains(overlayRec.Body.String(), "/api/alerts/state?consumer=overlay") {
		t.Fatalf("alert overlay route status=%d", overlayRec.Code)
	}

	testReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/alerts/test", strings.NewReader(`{"type":"follow"}`))
	testReq.Host = "127.0.0.1:17891"
	testReq.Header.Set("Origin", "http://127.0.0.1:17891")
	testReq.Header.Set("Content-Type", "application/json")
	testRec := httptest.NewRecorder()
	h.ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("test alert status=%d body=%s", testRec.Code, testRec.Body.String())
	}
	if state := app.alerts.state(false); state.QueueDepth != 1 || state.Current != nil {
		t.Fatalf("test alert did not remain queued before overlay consumption: %+v", state)
	}
}

func TestAlertStyleV1MigrationAndIndependentLayouts(t *testing.T) {
	settings := AlertSettings{
		SchemaVersion: 1,
		CanvasWidth:   1920,
		CanvasHeight:  1080,
		QueueLimit:    40,
		Types: map[string]AlertStyle{
			"subscription-new": {
				Enabled:         true,
				TitleTemplate:   "Legacy {user}",
				MessageTemplate: "Legacy message",
				DurationMS:      5000,
				Animation:       "slide-up",
				Layout:          "card",
				X:               500,
				Y:               300,
				Width:           700,
				Height:          350,
				MediaWidth:      180,
				MediaHeight:     180,
				TextColor:       "#abcdef",
				SoundVolume:     70,
			},
		},
	}
	normalized := normalizeAlertSettings(settings)
	if normalized.SchemaVersion != 2 {
		t.Fatalf("alert schema=%d, want 2", normalized.SchemaVersion)
	}
	newSub := normalized.Types["subscription-new"]
	if newSub.DisplayMode != "card" || !newSub.ShowTitle || !newSub.ShowMessage {
		t.Fatalf("v1 alert display migration failed: %+v", newSub)
	}
	if newSub.EnterAnimation != "slide-up" || newSub.ExitAnimation == "" {
		t.Fatalf("v1 animation migration failed: %+v", newSub)
	}
	if newSub.TitleColor != "#ABCDEF" || newSub.MessageColor != "#ABCDEF" {
		t.Fatalf("v1 text color migration failed: title=%q message=%q", newSub.TitleColor, newSub.MessageColor)
	}
	follow := normalized.Types["follow"]
	follow.VisualFile = "follow.png"
	normalized.Types["follow"] = follow
	gift := normalized.Types["subscription-gift"]
	gift.VisualFile = "gift.webp"
	normalized.Types["subscription-gift"] = gift
	if normalized.Types["follow"].VisualFile == normalized.Types["subscription-gift"].VisualFile {
		t.Fatal("alert types must retain independent visual files")
	}
}

func TestAlertPresentationTimingIncludesEnterHoldAndExit(t *testing.T) {
	m := newAlertManager(t.TempDir())
	settings := m.settingsSnapshot()
	style := settings.Types["follow"]
	style.EnterDurationMS = 300
	style.DurationMS = 1200
	style.ExitDurationMS = 400
	settings.Types["follow"] = style
	if err := m.setSettings(settings); err != nil {
		t.Fatal(err)
	}
	if !m.enqueue(AlertEvent{Type: "follow", Username: "Viewer"}, "timing") {
		t.Fatal("follow alert was not queued")
	}
	state := m.state(true)
	if state.Current == nil {
		t.Fatal("follow alert did not start")
	}
	if got := state.Current.EndsAtMS - state.Current.StartedAtMS; got != 1900 {
		t.Fatalf("presentation duration=%dms, want 1900ms", got)
	}
}
