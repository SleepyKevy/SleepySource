package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testCountdownManager(t *testing.T) *CountdownManager {
	t.Helper()
	return newCountdownManager(t.TempDir())
}

func TestCountdownFormattingMatchesReferenceBehavior(t *testing.T) {
	s := defaultCountdownSettings()

	tests := []struct {
		name    string
		format  string
		custom  string
		seconds int64
		want    string
	}{
		{"auto minutes", "auto", "", 300, "05:00"},
		{"auto hours", "auto", "", 3661, "1:01:01"},
		{"auto days", "auto", "", 90061, "1:01:01:01"},
		{"hhmmss", "hhmmss", "", 3661, "01:01:01"},
		{"mmss", "mmss", "", 3661, "61:01"},
		{"total seconds", "seconds", "", 3661, "3661"},
		{"negative", "hhmmss", "", -1, "-00:00:01"},
		{"custom tokens", "custom", "{sign}{d}|{h}|{hh}|{m}|{mm}|{s}|{ss}", -90061, "-1|1|01|1|01|1|01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.Format = tt.format
			if tt.custom != "" {
				s.CustomFormat = tt.custom
			}
			if got := formatCountdownBody(s, tt.seconds); got != tt.want {
				t.Fatalf("formatCountdownBody = %q, want %q", got, tt.want)
			}
		})
	}

	if got := countdownDisplaySeconds("countdown", 1); got != 1 {
		t.Fatalf("countdown 1ms display seconds = %d, want 1", got)
	}
	if got := countdownDisplaySeconds("countdown", 1001); got != 2 {
		t.Fatalf("countdown 1001ms display seconds = %d, want 2", got)
	}
	if got := countdownDisplaySeconds("stopwatch", 1999); got != 1 {
		t.Fatalf("stopwatch 1999ms display seconds = %d, want 1", got)
	}

	s.Prefix = "Starts in "
	s.Suffix = "!"
	s.FinishedText = "LIVE"
	if got := countdownDisplayText(s, 0, true); got != "Starts in LIVE!" {
		t.Fatalf("finished output = %q", got)
	}
	s.BlankOnFinish = true
	if got := countdownDisplayText(s, 0, true); got != "" {
		t.Fatalf("blank-on-finish output = %q", got)
	}
}

func TestCountdownControlsAndOverlayLifecycle(t *testing.T) {
	m := testCountdownManager(t)
	m.mu.Lock()
	m.settings.Hours = 0
	m.settings.Minutes = 2
	m.settings.Seconds = 0
	m.resetLocked(time.Now(), true)
	m.mu.Unlock()

	if err := m.control("stop"); err != nil {
		t.Fatal(err)
	}
	st := m.state(nil)
	if st.Running || st.Paused || st.HasStarted {
		t.Fatalf("stop before start should remain ready: %+v", st)
	}

	if err := m.control("start"); err != nil {
		t.Fatal(err)
	}
	st = m.state(nil)
	if !st.Running || !st.HasStarted {
		t.Fatalf("start state = %+v", st)
	}
	if err := m.control("pause"); err != nil {
		t.Fatal(err)
	}
	st = m.state(nil)
	if st.Running || !st.Paused {
		t.Fatalf("pause state = %+v", st)
	}
	if err := m.control("start"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := m.control("stop"); err != nil {
		t.Fatal(err)
	}
	st = m.state(nil)
	if st.Running || st.Paused || !st.HasStarted {
		t.Fatalf("stop state = %+v", st)
	}
	before := st.CurrentMS
	if err := m.control("add10"); err != nil {
		t.Fatal(err)
	}
	st = m.state(nil)
	if st.CurrentMS < before+9990 {
		t.Fatalf("add10 current=%d before=%d", st.CurrentMS, before)
	}
	if err := m.control("sub60"); err != nil {
		t.Fatal(err)
	}
	if err := m.control("reset"); err != nil {
		t.Fatal(err)
	}
	st = m.state(nil)
	if st.CurrentMS != 120000 || st.Running || st.Paused {
		t.Fatalf("reset state = %+v", st)
	}

	next := st.Settings
	next.StartBehavior = "overlay-load"
	next.RestartOnLoad = true
	next.ResetOnUnload = true
	if err := m.applySettings(next); err != nil {
		t.Fatal(err)
	}
	if err := m.control("overlay_loaded"); err != nil {
		t.Fatal(err)
	}
	st = m.state(nil)
	if !st.Running {
		t.Fatalf("overlay load should start timer: %+v", st)
	}
	if err := m.control("overlay_unloaded"); err != nil {
		t.Fatal(err)
	}
	st = m.state(nil)
	if st.Running || st.Paused || st.CurrentMS != 120000 {
		t.Fatalf("overlay unload should reset timer: %+v", st)
	}
}

func TestCountdownSettingsValidationAndPersistence(t *testing.T) {
	app := newTestApp(t)
	h := app.routes()

	bad := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/countdown/settings", strings.NewReader(`{"font_size":80}{"font_size":90}`))
	bad.Host = "127.0.0.1:17891"
	bad.Header.Set("Content-Type", "application/json")
	bad.Header.Set("Origin", "http://127.0.0.1:17891")
	badRec := httptest.NewRecorder()
	h.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status = %d, want 400; body=%s", badRec.Code, badRec.Body.String())
	}

	good := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/countdown/settings", strings.NewReader(`{"font_size":123,"mode":"stopwatch","canvas_width":1000}`))
	good.Host = "127.0.0.1:17891"
	good.Header.Set("Content-Type", "application/json")
	good.Header.Set("Origin", "http://127.0.0.1:17891")
	goodRec := httptest.NewRecorder()
	h.ServeHTTP(goodRec, good)
	if goodRec.Code != http.StatusOK {
		t.Fatalf("settings status=%d body=%s", goodRec.Code, goodRec.Body.String())
	}
	var got CountdownState
	if err := json.Unmarshal(goodRec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Settings.FontSize != 123 || got.Settings.Mode != "stopwatch" || got.Settings.CanvasWidth != 1000 {
		t.Fatalf("saved settings = %+v", got.Settings)
	}
	if got.Settings.FontFamily != "Segoe UI" || got.Settings.TextColor != "#FFFFFF" {
		t.Fatalf("partial update reset unrelated settings: %+v", got.Settings)
	}

	reloaded := newCountdownManager(app.dataDir)
	persisted := reloaded.state(nil)
	if persisted.Settings.FontSize != 123 || persisted.Settings.Mode != "stopwatch" || persisted.Settings.CanvasWidth != 1000 {
		t.Fatalf("reloaded settings = %+v", persisted.Settings)
	}
}

func TestCorruptCountdownSettingsAreBackedUpAndRewritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "countdown_settings.json")
	if err := os.WriteFile(path, []byte("{bad countdown json"), 0644); err != nil {
		t.Fatal(err)
	}
	m := newCountdownManager(dir)
	st := m.state(nil)
	if st.Settings.CanvasWidth != 900 || st.Settings.Mode != "countdown" {
		t.Fatalf("countdown defaults were not restored: %+v", st.Settings)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "countdown_settings.corrupt-*.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("corrupt countdown backup matches=%v err=%v", matches, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved CountdownSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("recovered countdown settings were not rewritten as valid JSON: %v; data=%q", err, data)
	}
	if saved.CanvasWidth != 900 || saved.Mode != "countdown" {
		t.Fatalf("rewritten countdown settings = %+v", saved)
	}
}

func TestCountdownNormalization(t *testing.T) {
	s := defaultCountdownSettings()
	s.Mode = "bad"
	s.Format = "bad"
	s.StartBehavior = "bad"
	s.Hours = -5
	s.Minutes = 99
	s.Seconds = 99
	s.CanvasWidth = 99999
	s.CanvasHeight = 1
	s.FontSize = 9999
	s.FontWeight = 123
	s.TextColor = "red"
	s.PanelOpacity = 999
	normalizeCountdownSettings(&s)
	if s.Mode != "countdown" || s.Format != "auto" || s.StartBehavior != "manual" {
		t.Fatalf("enum normalization failed: %+v", s)
	}
	if s.Hours != 0 || s.Minutes != 59 || s.Seconds != 59 || s.CanvasWidth != 3840 || s.CanvasHeight != 60 || s.FontSize != 400 || s.FontWeight != 700 || s.TextColor != "#FFFFFF" || s.PanelOpacity != 100 {
		t.Fatalf("bounds normalization failed: %+v", s)
	}
}

func TestCountdownBrowserSourceAndCustomFontUpload(t *testing.T) {
	app := newTestApp(t)
	h := app.routes()

	pageReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/countdown", nil)
	pageReq.Host = "127.0.0.1:17891"
	pageRec := httptest.NewRecorder()
	h.ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("countdown page status=%d body=%s", pageRec.Code, pageRec.Body.String())
	}
	body := pageRec.Body.String()
	for _, marker := range []string{"SleepySource Countdown", "/api/countdown/state", "overlay_loaded", "overlay_unloaded", "s.loop&&d>0"} {
		if !strings.Contains(body, marker) {
			t.Fatalf("countdown page missing %q", marker)
		}
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("font", "Countdown Test.ttf")
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

	uploadReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/countdown/upload-font", &buf)
	uploadReq.Host = "127.0.0.1:17891"
	uploadReq.Header.Set("Content-Type", mw.FormDataContentType())
	uploadReq.Header.Set("Origin", "http://127.0.0.1:17891")
	uploadRec := httptest.NewRecorder()
	h.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusOK {
		t.Fatalf("font upload status=%d body=%s", uploadRec.Code, uploadRec.Body.String())
	}
	var st CountdownState
	if err := json.Unmarshal(uploadRec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Settings.FontFamily != "NPF_Countdown_Test" {
		t.Fatalf("font family=%q", st.Settings.FontFamily)
	}
	if len(st.Fonts) != 1 || st.Fonts[0].Family != "NPF_Countdown_Test" {
		t.Fatalf("fonts=%+v", st.Fonts)
	}
}

func TestCountdownProfilesSaveLoadDelete(t *testing.T) {
	m := testCountdownManager(t)
	next := m.state(nil).Settings
	next.Minutes = 12
	next.Seconds = 34
	next.Prefix = "BRB • "
	next.TimerAnimation = "float"
	next.TickAnimation = "pop"
	if err := m.applySettings(next); err != nil {
		t.Fatal(err)
	}
	if err := m.saveProfile("BRB Timer"); err != nil {
		t.Fatal(err)
	}
	profiles := m.listProfiles()
	if len(profiles) != 1 || profiles[0].Name != "BRB Timer" {
		t.Fatalf("profiles = %#v", profiles)
	}

	changed := m.state(nil).Settings
	changed.Minutes = 1
	changed.Seconds = 0
	changed.Prefix = ""
	changed.TimerAnimation = "none"
	if err := m.applySettings(changed); err != nil {
		t.Fatal(err)
	}
	if err := m.loadProfile("BRB Timer"); err != nil {
		t.Fatal(err)
	}
	loaded := m.state(nil)
	if loaded.Settings.Minutes != 12 || loaded.Settings.Seconds != 34 || loaded.Settings.Prefix != "BRB • " || loaded.Settings.TimerAnimation != "float" || loaded.Settings.TickAnimation != "pop" {
		t.Fatalf("loaded profile = %+v", loaded.Settings)
	}
	if loaded.CurrentMS != (12*60+34)*1000 {
		t.Fatalf("loaded current ms = %d", loaded.CurrentMS)
	}

	if err := m.deleteProfile("BRB Timer"); err != nil {
		t.Fatal(err)
	}
	if got := m.listProfiles(); len(got) != 0 {
		t.Fatalf("profiles after delete = %#v", got)
	}
}

func TestCountdownAnimationSettingsNormalize(t *testing.T) {
	s := defaultCountdownSettings()
	s.TimerAnimation = "explode"
	s.TickAnimation = "warp"
	s.PanelAnimation = "flash"
	s.OverlayAnimation = "spin"
	s.AnimationMS = 99999
	normalizeCountdownSettings(&s)
	if s.SchemaVersion != 2 || s.TimerAnimation != "none" || s.TickAnimation != "none" || s.PanelAnimation != "none" || s.OverlayAnimation != "none" || s.AnimationMS != 12000 {
		t.Fatalf("normalized countdown animation settings = %+v", s)
	}
}
