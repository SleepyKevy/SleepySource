package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNormalizeSettingsBoundsAndColors(t *testing.T) {
	s := defaultSettings()
	s.SchemaVersion = 1
	s.CanvasWidth = 99999
	s.CanvasHeight = 1
	s.TextX = 99999
	s.TextWidth = 99999
	s.ImageWidth = 99999
	s.ProgressWidth = 99999
	s.TimeWidth = 99999
	s.TextColor = "javascript:red"
	s.ProgressColor = "#abc"
	s.ProgressTrackColor = "#12abEF"
	s.BackgroundColor = "  #abcdef  "
	s.ProgressTextColor = ""
	s.Template = strings.Repeat("x", 700)

	normalizeSettings(&s)

	if s.SchemaVersion != 69 {
		t.Fatalf("schema = %d, want 69", s.SchemaVersion)
	}
	if s.CanvasWidth != 7680 || s.CanvasHeight != 180 {
		t.Fatalf("canvas normalized to %dx%d", s.CanvasWidth, s.CanvasHeight)
	}
	if s.TextX != 7680 || s.TextWidth != 7680 || s.ImageWidth != 7680 || s.ProgressWidth != 7680 || s.TimeWidth != 7680 {
		t.Fatalf("size/position bounds were not applied: %+v", s)
	}
	if s.TextColor != "#FFFFFF" || s.ProgressColor != "#3AA7FF" || s.ProgressTrackColor != "#12ABEF" || s.BackgroundColor != "#ABCDEF" || s.ProgressTextColor != "#DCEBFF" {
		t.Fatalf("colors were not normalized: text=%s progress=%s track=%s bg=%s time=%s", s.TextColor, s.ProgressColor, s.ProgressTrackColor, s.BackgroundColor, s.ProgressTextColor)
	}
	if len(s.Template) != 512 {
		t.Fatalf("template length = %d, want 512", len(s.Template))
	}
}

func TestApplyTemplate(t *testing.T) {
	got := applyTemplate("{title} • {artist} / {song}", "Artist", "Track")
	if got != "Track • Artist / Track" {
		t.Fatalf("applyTemplate = %q", got)
	}
}

func TestCommitTempFileReplacesTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	tmp, err := os.CreateTemp(dir, ".new-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString("new"); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}
	if err := commitTempFile(tmp.Name(), target, 2); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("target = %q, want new", data)
	}
}

func TestCorruptSettingsAreBackedUpAndReset(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		settings:     defaultSettings(),
		dataDir:      dir,
		settingsPath: filepath.Join(dir, "settings.json"),
	}
	if err := os.WriteFile(app.settingsPath, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	app.loadSettings()
	if app.settings.SchemaVersion != 69 {
		t.Fatalf("schema after recovery = %d", app.settings.SchemaVersion)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "settings.corrupt-*.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("corrupt backup matches=%v err=%v", matches, err)
	}
	data, err := os.ReadFile(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var recovered Settings
	if err := json.Unmarshal(data, &recovered); err != nil {
		t.Fatalf("recovered settings are invalid: %v", err)
	}
}

func TestRequestOriginPolicy(t *testing.T) {
	tests := []struct {
		name   string
		method string
		host   string
		origin string
		want   bool
	}{
		{"same-origin post", http.MethodPost, "127.0.0.1:17891", "http://127.0.0.1:17891", true},
		{"localhost same-origin post", http.MethodPost, "localhost:17891", "http://localhost:17891", true},
		{"native post no origin", http.MethodPost, "127.0.0.1:17891", "", true},
		{"null origin blocked", http.MethodPost, "127.0.0.1:17891", "null", false},
		{"foreign origin blocked", http.MethodPost, "127.0.0.1:17891", "https://example.com", false},
		{"wrong local port blocked", http.MethodPost, "127.0.0.1:17891", "http://127.0.0.1:9999", false},
		{"foreign host blocked", http.MethodGet, "evil.example:17891", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "http://"+tt.host+"/api/settings", nil)
			r.Host = tt.host
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if got := requestOriginAllowed(r); got != tt.want {
				t.Fatalf("requestOriginAllowed = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlbumArtworkModeMigratesToFallback(t *testing.T) {
	s := defaultSettings()
	s.ImageMode = "album"
	normalizeSettings(&s)
	if s.ImageMode != "fallback" {
		t.Fatalf("image mode = %q, want fallback", s.ImageMode)
	}
}

func TestSanitizeFontBase(t *testing.T) {
	if got := sanitizeFontBase(`..\\My Font!?.ttf`); got != "My_Font" {
		t.Fatalf("sanitizeFontBase = %q", got)
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	media := filepath.Join(dir, "Media")
	fonts := filepath.Join(media, "fonts")
	if err := os.MkdirAll(fonts, 0755); err != nil {
		t.Fatal(err)
	}
	app := &App{
		settings:       defaultSettings(),
		dataDir:        dir,
		settingsPath:   filepath.Join(dir, "settings.json"),
		outputPath:     filepath.Join(dir, "now_playing.txt"),
		customDir:      media,
		fontDir:        fonts,
		profileDir:     filepath.Join(dir, "Profiles"),
		detectorStatus: "test",
	}
	app.chat = newChatManager(dir)
	app.countdown = newCountdownManager(dir)
	app.alerts = newAlertManager(dir)
	return app
}

func TestRoutesRejectForeignMutationAndAcceptPartialLocalSettings(t *testing.T) {
	app := newTestApp(t)
	h := app.routes()

	foreign := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/settings", strings.NewReader(`{"progress_color":"#123456"}`))
	foreign.Host = "127.0.0.1:17891"
	foreign.Header.Set("Content-Type", "application/json")
	foreign.Header.Set("Origin", "https://example.com")
	foreignRec := httptest.NewRecorder()
	h.ServeHTTP(foreignRec, foreign)
	if foreignRec.Code != http.StatusForbidden {
		t.Fatalf("foreign mutation status = %d, want %d", foreignRec.Code, http.StatusForbidden)
	}

	local := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/settings", strings.NewReader(`{"progress_color":"#123456"}`))
	local.Host = "127.0.0.1:17891"
	local.Header.Set("Content-Type", "application/json")
	local.Header.Set("Origin", "http://127.0.0.1:17891")
	localRec := httptest.NewRecorder()
	h.ServeHTTP(localRec, local)
	if localRec.Code != http.StatusOK {
		t.Fatalf("local settings status = %d body=%s", localRec.Code, localRec.Body.String())
	}
	if app.settings.ProgressColor != "#123456" {
		t.Fatalf("progress color = %s", app.settings.ProgressColor)
	}
	if app.settings.CanvasWidth != 900 || app.settings.TextFont != "Segoe UI" {
		t.Fatalf("partial settings update reset unrelated fields: %+v", app.settings)
	}
	if got := localRec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'self'") {
		t.Fatalf("missing expected CSP: %q", got)
	}
}

func TestNormalizeNewDesignerSettings(t *testing.T) {
	s := defaultSettings()
	s.ProgressStyle = "bad"
	s.TransitionStyle = "bad"
	s.TransitionMS = 99999
	s.TransitionEasing = "bad"
	s.MediaSourceMode = "bad"
	s.GridSize = 0
	s.DesignerTheme = "bad"
	s.StartupPage = "bad"
	s.LastModule = "bad"
	normalizeSettings(&s)
	if s.ProgressStyle != "rounded" || s.TransitionStyle != "fade" || s.TransitionMS != 5000 || s.TransitionEasing != "smooth" || s.MediaSourceMode != "spotify" || s.GridSize != 10 || s.DesignerTheme != "blue" || s.StartupPage != "home" || s.LastModule != "now-playing" {
		t.Fatalf("new settings normalization failed: %+v", s)
	}
}

func TestNormalizePinkDesignerTheme(t *testing.T) {
	s := defaultSettings()
	s.DesignerTheme = "pink"
	normalizeSettings(&s)
	if s.DesignerTheme != "pink" {
		t.Fatalf("pink theme should be preserved, got %q", s.DesignerTheme)
	}
}

func TestSaveAndLoadProfile(t *testing.T) {
	app := newTestApp(t)
	if err := os.MkdirAll(app.profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	app.settings.TextX = 321
	app.settings.ProgressStyle = "glow"
	if err := app.saveProfile("Gaming"); err != nil {
		t.Fatal(err)
	}
	app.settings.TextX = 12
	app.settings.ProgressStyle = "square"
	if err := app.loadProfile("Gaming"); err != nil {
		t.Fatal(err)
	}
	if app.settings.TextX != 321 || app.settings.ProgressStyle != "glow" {
		t.Fatalf("profile did not restore settings: %+v", app.settings)
	}
	profiles := app.listProfiles()
	if len(profiles) != 1 || profiles[0].Name != "Gaming" {
		t.Fatalf("profiles=%+v", profiles)
	}
}

func TestSanitizeProfileName(t *testing.T) {
	if got := sanitizeProfileName(`  My/Profile:*  `); got != "MyProfile" {
		t.Fatalf("sanitizeProfileName=%q", got)
	}
}

func TestProfileKeepsItsOwnArtwork(t *testing.T) {
	app := newTestApp(t)
	if err := os.MkdirAll(app.profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	art := filepath.Join(app.customDir, "custom_now_playing.png")
	if err := os.WriteFile(art, []byte("profile-A"), 0644); err != nil {
		t.Fatal(err)
	}
	app.customPath = art
	app.settings.CustomImage = filepath.Base(art)
	app.settings.ImageMode = "custom"
	if err := app.saveProfile("A"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(art, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := app.loadProfile("A"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(app.customPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "profile-A" {
		t.Fatalf("profile artwork=%q", got)
	}
}

func TestProfilePackagesAndRestoresCustomFont(t *testing.T) {
	app := newTestApp(t)
	if err := os.MkdirAll(app.profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	fontName := "Night_Font.ttf"
	fontPath := filepath.Join(app.fontDir, fontName)
	if err := os.WriteFile(fontPath, []byte{0x00, 0x01, 0x00, 0x00, 0, 0, 0, 0}, 0644); err != nil {
		t.Fatal(err)
	}
	app.settings.TextFont = fontFamilyForFile(fontName)
	if err := app.saveProfile("Font Profile"); err != nil {
		t.Fatal(err)
	}
	packaged := filepath.Join(app.profileDir, "Font Profile", "font_"+fontName)
	if _, err := os.Stat(packaged); err != nil {
		t.Fatalf("packaged font missing: %v", err)
	}
	if err := os.Remove(fontPath); err != nil {
		t.Fatal(err)
	}
	app.settings.TextFont = "Segoe UI"
	if err := app.loadProfile("Font Profile"); err != nil {
		t.Fatal(err)
	}
	if app.settings.TextFont != fontFamilyForFile(fontName) {
		t.Fatalf("font family=%q", app.settings.TextFont)
	}
	if _, err := os.Stat(fontPath); err != nil {
		t.Fatalf("font not restored: %v", err)
	}
}

func TestProfileBundleExportImport(t *testing.T) {
	source := newTestApp(t)
	if err := os.MkdirAll(source.profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	source.settings.TextX = 444
	if err := source.saveProfile("Bundle Test"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/api/profile-export?name=Bundle%20Test", nil)
	req.Host = "127.0.0.1:17891"
	rec := httptest.NewRecorder()
	source.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("export status=%d body=%s", rec.Code, rec.Body.String())
	}

	target := newTestApp(t)
	if err := os.MkdirAll(target.profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("bundle", "Bundle_Test.sleepyprofile.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(rec.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	importReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/profile-import", &body)
	importReq.Host = "127.0.0.1:17891"
	importReq.Header.Set("Content-Type", mw.FormDataContentType())
	importReq.Header.Set("Origin", "http://127.0.0.1:17891")
	importRec := httptest.NewRecorder()
	target.routes().ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("import status=%d body=%s", importRec.Code, importRec.Body.String())
	}
	if err := target.loadProfile("Bundle Test"); err != nil {
		t.Fatal(err)
	}
	if target.settings.TextX != 444 {
		t.Fatalf("imported profile TextX=%d", target.settings.TextX)
	}
}

func TestMediaSourceAllowed(t *testing.T) {
	s := defaultSettings()
	if !mediaSourceAllowed("Spotify.exe", s) {
		t.Fatal("spotify mode should accept Spotify")
	}
	if mediaSourceAllowed("firefox.exe", s) {
		t.Fatal("spotify mode should reject Firefox")
	}
	s.MediaSourceMode = "any"
	s.MediaSourceExclude = "discord, chrome"
	if !mediaSourceAllowed("firefox.exe", s) || mediaSourceAllowed("Discord.exe", s) {
		t.Fatal("any/exclude filtering failed")
	}
	s.MediaSourceMode = "custom"
	s.MediaSourceInclude = "firefox, spotify"
	if !mediaSourceAllowed("Mozilla.Firefox", s) || mediaSourceAllowed("vlc.exe", s) {
		t.Fatal("custom include filtering failed")
	}
}

func TestCleanBackupRelativeRejectsTraversal(t *testing.T) {
	bad := []string{"../settings.json", "../../evil", "/absolute/file", `..\\evil`}
	for _, name := range bad {
		if _, ok := cleanBackupRelative(name); ok {
			t.Fatalf("unsafe backup path accepted: %q", name)
		}
	}
	if got, ok := cleanBackupRelative("SleepySource_Data/Media/custom_now_playing.png"); !ok || got == "" {
		t.Fatalf("safe backup path rejected: got=%q ok=%v", got, ok)
	}
}

func TestBackupSkipsRuntimeCredentialsAndTransientFiles(t *testing.T) {
	for _, rel := range []string{
		"DesktopRuntime", "DesktopRuntime/EBWebView/Cache/data", "app.ico", "kick_credentials.json", "Alerts/legacy.json", "KICK/legacy.json",
		".kick-credentials-123.tmp", "Media/fonts/.countdown-font-upload-123.tmp", "Media/fonts/.chat-font-upload-123.tmp", "Media/fonts/.font-upload-123.tmp",
		"Media/.artwork-upload-123.tmp", "Media/.background-upload-123.tmp", ".icon-123.tmp", "Profiles/.bundle-123.zip", "Profiles/Test/.profile-copy-123.tmp",
	} {
		if !skipBackupRelative(rel) {
			t.Fatalf("backup should skip local/runtime/transient path %q", rel)
		}
	}
	for _, rel := range []string{"settings.json", "chat_settings.json", "countdown_settings.json", "Media/background.png", "Profiles/Gaming/settings.json"} {
		if skipBackupRelative(rel) {
			t.Fatalf("backup unexpectedly skips portable app data %q", rel)
		}
	}
}

func TestClearRestorableDataPreservesLocalRuntimeAndLogin(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string) {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("settings.json")
	write("Media/background.png")
	write("DesktopRuntime/EBWebView/Cache/data")
	write("kick_credentials.json")
	write("app.ico")
	if err := clearRestorableData(dir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"DesktopRuntime/EBWebView/Cache/data", "kick_credentials.json", "app.ico"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("preserved local file %q missing after restore cleanup: %v", rel, err)
		}
	}
	for _, rel := range []string{"settings.json", "Media/background.png"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("restorable file %q was not cleared", rel)
		}
	}
}

func TestSnapshotRestorableDataExcludesLocalOnlyFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	write := func(rel, value string) {
		path := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("settings.json", "settings")
	write("Media/background.png", "background")
	write("DesktopRuntime/EBWebView/Cache/data", "runtime")
	write("kick_credentials.json", "secret")
	write("app.ico", "icon")
	if err := snapshotRestorableData(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"settings.json", "Media/background.png"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("snapshot missing restorable file %q: %v", rel, err)
		}
	}
	for _, rel := range []string{"DesktopRuntime/EBWebView/Cache/data", "kick_credentials.json", "app.ico"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("snapshot unexpectedly copied local-only file %q", rel)
		}
	}
}

func TestRestoreSnapshotReplacesPortableDataAndPreservesLocalOnlyFiles(t *testing.T) {
	dataDir := t.TempDir()
	snapshot := t.TempDir()
	write := func(root, rel, value string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(dataDir, "settings.json", "new")
	write(dataDir, "Media/new.png", "new media")
	write(dataDir, "DesktopRuntime/cache", "runtime")
	write(dataDir, "kick_credentials.json", "secret")
	write(snapshot, "settings.json", "old")
	write(snapshot, "Media/old.png", "old media")

	if err := restoreSnapshot(dataDir, snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dataDir, "settings.json"))
	if err != nil || string(got) != "old" {
		t.Fatalf("restored settings=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "Media", "new.png")); !os.IsNotExist(err) {
		t.Fatal("new portable file should have been replaced by rollback snapshot")
	}
	for _, rel := range []string{"DesktopRuntime/cache", "kick_credentials.json"} {
		if _, err := os.Stat(filepath.Join(dataDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("local-only file %q was not preserved: %v", rel, err)
		}
	}
}

func TestProxyImageURLAllowedHostPolicy(t *testing.T) {
	if !proxyImageURLAllowed("https://files.kick.com/emotes/1/fullsize", true) {
		t.Fatal("trusted Kick image URL was rejected")
	}
	if proxyImageURLAllowed("https://example.com/image.png", true) {
		t.Fatal("non-Kick URL passed trusted Kick image policy")
	}
	allowed := []string{"www.kickdatabase.com", "cpwemotes.co.uk"}
	if !proxyImageURLAllowed("https://www.kickdatabase.com/kickBadges/mod.svg", false, allowed...) {
		t.Fatal("known badge mirror was rejected")
	}
	if proxyImageURLAllowed("https://127.0.0.1/private.png", false, allowed...) {
		t.Fatal("local redirect target passed third-party badge mirror policy")
	}
	if proxyImageURLAllowed("http://www.kickdatabase.com/kickBadges/mod.svg", false, allowed...) {
		t.Fatal("insecure badge mirror URL passed HTTPS-only policy")
	}
}

func TestFullBackupExportImportRoundTrip(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "SleepySource_Data")
	mediaDir := filepath.Join(dataDir, "Media")
	app := &App{
		settings:       defaultSettings(),
		exeDir:         root,
		dataDir:        dataDir,
		settingsPath:   filepath.Join(dataDir, "settings.json"),
		outputPath:     filepath.Join(dataDir, "now_playing.txt"),
		customDir:      mediaDir,
		fontDir:        filepath.Join(mediaDir, "fonts"),
		profileDir:     filepath.Join(dataDir, "Profiles"),
		updatedAt:      time.Now().UnixMilli(),
		detectorStatus: "test",
	}
	for _, dir := range []string{app.dataDir, app.customDir, app.fontDir, app.profileDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	app.settings.TextX = 321
	if err := app.saveSettingsLocked(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "portable.txt"), []byte("portable-v1"), 0644); err != nil {
		t.Fatal(err)
	}
	credentialPath := filepath.Join(dataDir, "kick_credentials.json")
	if err := os.WriteFile(credentialPath, []byte("local-secret-placeholder"), 0600); err != nil {
		t.Fatal(err)
	}

	exportRR := httptest.NewRecorder()
	app.handleBackupExport(exportRR, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/api/backup/export", nil))
	if exportRR.Code != http.StatusOK {
		t.Fatalf("backup export status=%d body=%s", exportRR.Code, exportRR.Body.String())
	}
	backupBytes := exportRR.Body.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(backupBytes), int64(len(backupBytes)))
	if err != nil {
		t.Fatalf("exported backup is not a valid ZIP: %v", err)
	}
	for _, zf := range zr.File {
		if strings.EqualFold(filepath.Base(zf.Name), "kick_credentials.json") {
			t.Fatal("full backup unexpectedly contains Kick credentials")
		}
	}

	// Change the live portable setup after export so the import has something
	// meaningful to replace while preserving local-only credentials.
	app.settings.TextX = 777
	if err := app.saveSettingsLocked(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "portable.txt"), []byte("portable-v2"), 0644); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("backup", "SleepySource_Test.sleepysource")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(backupBytes); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	importReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/backup/import", &body)
	importReq.Header.Set("Content-Type", mw.FormDataContentType())
	importRR := httptest.NewRecorder()
	app.handleBackupImport(importRR, importReq)
	if importRR.Code != http.StatusOK {
		t.Fatalf("backup import status=%d body=%s", importRR.Code, importRR.Body.String())
	}
	if app.settings.TextX != 321 {
		t.Fatalf("restored TextX=%d, want 321", app.settings.TextX)
	}
	media, err := os.ReadFile(filepath.Join(mediaDir, "portable.txt"))
	if err != nil || string(media) != "portable-v1" {
		t.Fatalf("restored media=%q err=%v", media, err)
	}
	credentials, err := os.ReadFile(credentialPath)
	if err != nil || string(credentials) != "local-secret-placeholder" {
		t.Fatalf("local credentials changed during restore: %q err=%v", credentials, err)
	}
}

func TestReleaseVersionAndRetiredUILabels(t *testing.T) {
	if appVersion != "1.3.2" {
		t.Fatalf("release appVersion=%q, want 1.3.2", appVersion)
	}
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{"Press Enter to continue", "chatCopyWebhookBtn", ">BETA<", "SleepySource Quick Setup"} {
		if strings.Contains(string(index), retired) {
			t.Fatalf("retired UI marker %q is still present", retired)
		}
	}
	if !strings.Contains(string(index), "OBS Setup Guide") {
		t.Fatal("designer is missing the manual OBS Setup Guide control")
	}
	if !strings.Contains(string(index), "SleepySource 1.0 Beta") || !strings.Contains(string(index), "Version 1.0 Beta") {
		t.Fatal("designer public branding is not standardized to SleepySource 1.0 Beta")
	}
	if strings.Contains(string(index), "SleepySource v1.0") {
		t.Fatal("designer still contains old v-prefixed release branding")
	}
	if strings.Contains(string(index), "Work in Progress") {
		t.Fatal("designer still contains pre-release Work in Progress branding")
	}
	if strings.Contains(string(index), "if(settings&&!settings.onboarding_complete)setTimeout(showOnboarding") {
		t.Fatal("OBS setup guide still auto-opens after entering the designer")
	}
	enter, err := embedded.ReadFile("web/enter.html")
	if err != nil {
		t.Fatal(err)
	}
	enterText := string(enter)
	for _, required := range []string{"SleepySource", "Streaming Tools, All in One Place", "Now Playing", "Chat Overlay", "Kick Integration", "Open SleepySource", "Made by SleepyKev", "Build 1.0 Beta", "Ready", "particleField", "introRise", "--alpha-low", "--dx1"} {
		if !strings.Contains(enterText, required) {
			t.Fatalf("splash screen is missing required marker %q", required)
		}
	}
	for _, retired := range []string{"logoSweep", "logoLightSweep"} {
		if strings.Contains(enterText, retired) {
			t.Fatalf("removed splash light sweep returned: %q", retired)
		}
	}
	if strings.Contains(enterText, "Work in Progress") {
		t.Fatal("splash still contains pre-release Work in Progress branding")
	}
	if strings.Contains(enterText, ">Version 1.3.2<") {
		t.Fatal("splash should not contain a separate visible Version 1.3.2 label")
	}
	if !strings.Contains(enterText, "button.addEventListener('click', openSleepySource)") {
		t.Fatal("splash Open SleepySource button is not wired")
	}
	if !strings.Contains(enterText, "event.key !== 'Enter'") {
		t.Fatal("splash Enter-key shortcut is not wired")
	}
	if !strings.Contains(enterText, "window.location.assign('/designer')") {
		t.Fatal("splash does not navigate to the designer")
	}
}

func TestChatPrimaryStatusAndDiagnosticsLayout(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, required := range []string{
		`class="chatPrimaryStatus"`,
		`<strong>Kick Channel</strong><span id="chatKickLookupStatus">`,
		`<strong>Chat Subscription</strong><span id="chatSubscriptionStatus">`,
		`<strong>7TV</strong><span id="chat7TVStatus">`,
		`id="connectionsWorkspace" data-workspace-panel="connections"`,
		`class="connectionsStatusGrid"`,
		`id="chatFeedStatus"`,
		`id="chatWebhookRequests"`,
		`id="chatWebhookVerified"`,
		`id="chatWebhookAccepted"`,
		`id="chatWebhookRejected"`,
		`id="chatWebhookLast"`,
		`id="chatWebhookError"`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("chat status/connections diagnostics UI is missing %q", required)
		}
	}
	if strings.Contains(text, `<details class="chatDiagnostics">`) {
		t.Fatal("webhook diagnostics should live on Connections instead of a Chat Overlay accordion")
	}
}

func TestDesignerThemeHasStandaloneGlobalSelector(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, required := range []string{
		`class="globalThemeControl"`,
		`class="globalThemeLabel">Designer theme</div>`,
		`id="themePicker"`,
		`id="designerThemeValue"`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("standalone designer theme selector is missing %q", required)
		}
	}
	if !strings.Contains(text, `.globalThemeControl{margin:0 0 8px;padding:0;border:0;border-radius:0;background:transparent;box-shadow:none;min-width:0}`) {
		t.Fatal("designer theme selector must not have an outer panel box")
	}
	if !strings.Contains(text, `.themePickerButton{width:100%;min-height:41px;display:flex;align-items:center;gap:10px;border:1px solid var(--line);background:linear-gradient(180deg,var(--card-top),var(--card-bottom));`) {
		t.Fatal("theme picker should match the sidebar card surface")
	}
	pickerStart := strings.Index(text, `id="themePicker"`)
	pickerEnd := strings.Index(text, `id="designerThemeValue"`)
	if pickerStart < 0 || pickerEnd < 0 || pickerEnd <= pickerStart {
		t.Fatal("could not isolate designer theme picker markup")
	}
	pickerMarkup := text[pickerStart:pickerEnd]
	if !strings.Contains(pickerMarkup, `src="/assets/nav-theme.svg"`) {
		t.Fatal("designer theme picker button must use the monochrome theme icon")
	}
	menuStart := strings.Index(pickerMarkup, `id="themePickerMenu"`)
	if menuStart < 0 {
		t.Fatal("could not isolate designer theme picker menu")
	}
	menuMarkup := pickerMarkup[menuStart:]
	if strings.Contains(menuMarkup, `<img`) || strings.Contains(menuMarkup, `themeOptionCheck`) {
		t.Fatal("designer theme picker entries must remain text-only with no theme artwork or option icons")
	}
	if !strings.Contains(text, `.moduleNav{display:grid;gap:5px;margin:0 0 10px;min-width:0}`) {
		t.Fatal("permanent module navigation is missing or has unexpected spacing")
	}
	if strings.Contains(text, `id="moduleSwitcher"`) || strings.Contains(text, `class="moduleSwitcher"`) {
		t.Fatal("module selection should use permanent buttons instead of the old dropdown")
	}
	moduleAt := strings.Index(text, `id="moduleNav"`)
	themeAt := strings.Index(text, `class="globalThemeControl"`)
	toolsAt := strings.Index(text, `id="sidebarToolsScroll"`)
	nowPlayingAt := strings.Index(text, `id="nowPlayingSidebar"`)
	if moduleAt < 0 || themeAt < 0 || toolsAt < 0 || nowPlayingAt < 0 || !(themeAt < moduleAt && moduleAt < toolsAt && toolsAt < nowPlayingAt) {
		t.Fatal("designer theme must sit above permanent module navigation and the scrolling module tools")
	}
	settingsAt := strings.Index(text, `data-section="settings"`)
	mediaSourceAt := strings.Index(text, `<label>Media-session source</label>`)
	if settingsAt < 0 || mediaSourceAt < 0 || strings.Contains(text[settingsAt:mediaSourceAt], `id="themePicker"`) {
		t.Fatal("designer theme selector must not remain inside Settings")
	}
}

func TestBuiltInHelpGuideModule(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, required := range []string{
		`id="staticHelpButton"`,
		`class="staticHelpButton"`,
		`class="sidebarPinnedBottom"`,
		`id="helpSidebar"`,
		`id="helpWorkspace"`,
		`id="helpNowPlaying"`,
		`id="helpChat"`,
		`id="helpCountdown"`,
		`id="helpThemesFonts"`,
		`id="helpProfilesBackups"`,
		`id="helpOBS"`,
		`id="helpTroubleshooting"`,
		`data-help-jump-module="now-playing"`,
		`function openHelpGuide()`,
		`if(helpGuideOpen)setActiveModule(activeToolModule,false);else openHelpGuide()`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("built-in Help & Guide is missing %q", required)
		}
	}
	for _, required := range []string{`class="helpDetails"`, `Quick start`, `Open only the problem you are having.`} {
		if !strings.Contains(text, required) {
			t.Fatalf("condensed Help & Guide is missing %q", required)
		}
	}
	if strings.Contains(text, `data-module="help-guide"`) {
		t.Fatal("Help & Guide should be a permanent sidebar control, not a module-navigation item")
	}
	helpPanelAt := strings.Index(text, `id="helpSidebar"`)
	helpAt := strings.Index(text, `id="staticHelpButton"`)
	if helpPanelAt < 0 || helpAt <= helpPanelAt {
		t.Fatal("Help & Guide button should be pinned after the scrolling tool area")
	}
	if !strings.Contains(text, `.sidebarPinnedBottom{flex:none;display:grid;gap:6px;margin-top:8px;padding-top:8px;border-top:1px solid`) {
		t.Fatal("System Health / Help & Guide bottom pinning guard is missing")
	}
}

func TestSidebarUsesPermanentModuleNavigation(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, label := range []string{"Home", "Now Playing", "Chat Overlay", "Countdown Pro"} {
		if !strings.Contains(text, `data-module-label="`+label+`"`) {
			t.Fatalf("permanent module navigation is missing %q", label)
		}
	}
	for _, required := range []string{
		`id="homeNavButton"`,
		`id="toolsNavGroup"`,
		`id="toolsNavToggle"`,
		`aria-controls="moduleNav"`,
		`id="moduleNav"`,
		`class="toolsNavChevron"`,
		`sleepysource-tools-nav-open-v1`,
		`function setToolsNavOpen(open,persist=true)`,
		`function restoreToolsNavState()`,
		`$('#toolsNavToggle')?.addEventListener('click'`,
		`id="moduleToolsLabel"></div>`,
		`id="sidebarToolsScroll"`,
		`.sidebar{min-width:0;height:100vh;padding:22px;border-right:1px solid var(--line);background:var(--sidebar-bg);backdrop-filter:blur(14px);display:flex;flex-direction:column;overflow:hidden;`,
		`.sidebarToolsScroll{flex:1;min-height:0;overflow-y:auto;overflow-x:hidden;`,
		`.moduleButton.active{color:#fff;border-color:color-mix(in srgb,var(--accent) 72%,var(--line));background:color-mix(in srgb,var(--accent) 15%,var(--panel2));box-shadow:none}`,
		`Array.from(panel.children).forEach(other=>{if(other!==section&&other.matches('details.accordion'))other.open=false})`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("sidebar redesign is missing %q", required)
		}
	}
	toolsToggleAt := strings.Index(text, `id="toolsNavToggle"`)
	moduleNavAt := strings.Index(text, `id="moduleNav"`)
	toolsScrollAt := strings.Index(text, `id="sidebarToolsScroll"`)
	if toolsToggleAt < 0 || moduleNavAt < 0 || toolsScrollAt < 0 || !(toolsToggleAt < moduleNavAt && moduleNavAt < toolsScrollAt) {
		t.Fatal("Tools parent must sit above and contain the module navigation before the scrolling settings area")
	}
	if !strings.Contains(text, `.toolsNavGroup.collapsed .moduleNav{max-height:0;opacity:0;`) {
		t.Fatal("Tools navigation needs a real collapsed visual state")
	}
}

func TestSystemHealthWorkspace(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, required := range []string{
		`id="staticSystemHealthButton"`,
		`<span class="staticHelpArrow" aria-hidden="true">→</span>`,
		`id="systemHealthWorkspace" data-workspace-panel="system-health"`,
		`id="healthRunMainBtn"`,
		`id="healthCopyMainBtn"`,
		`id="healthGroups"`,
		`fetch('/api/system/health?ts='+Date.now()`,
		`function repairHealthAction(action,button)`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("System Health UI is missing %q", required)
		}
	}
	for _, forbidden := range []string{`id="systemHealthSidebar"`, `id="healthRunSideBtn"`, `id="healthCopySideBtn"`, `id="systemNav"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("System Health should be a standalone page, but sidebar/dropdown UI remains: %q", forbidden)
		}
	}
	now := strings.Index(text, `data-module="now-playing"`)
	chat := strings.Index(text, `data-module="chat-overlay"`)
	stream := strings.Index(text, `data-module="stream-settings"`)
	countdown := strings.Index(text, `data-module="countdown-pro"`)
	health := strings.Index(text, `id="staticSystemHealthButton"`)
	help := strings.Index(text, `id="staticHelpButton"`)
	if now < 0 || chat < 0 || stream < 0 || countdown < 0 || health < 0 || help < 0 || !(now < chat && chat < stream && stream < countdown && countdown < health && health < help) {
		t.Fatalf("sidebar order must be Now Playing -> Chat Overlay -> Stream Dashboard -> Countdown Pro -> System Health -> Help: now=%d chat=%d stream=%d countdown=%d health=%d help=%d", now, chat, stream, countdown, health, help)
	}
}

func TestHomeDashboardAfterSplash(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, required := range []string{
		`id="homeNavButton"`,
		`data-module-icon="home"`,
		`class="homeTabIcon" src="/assets/home-tab.png"`,
		`data-module="home"`,
		`id="homeSidebar" data-module-panel="home"`,
		`id="homeWorkspace" data-workspace-panel="home"`,
		`id="homeUpdateSectionTitle">Updates</h3><span>Stable channel</span>`,
		`id="homeGlanceKickCard"`,
		`id="homeGlanceRelayCard"`,
		`id="homeGlanceNowCard"`,
		`id="homeCopyObsUrlsBtn"`,
		`id="homeObsUrlsMenu"`,
		`data-obs-copy-url="http://127.0.0.1:17891/overlay"`,
		`data-obs-copy-url="http://127.0.0.1:17891/chat"`,
		`data-obs-copy-url="http://127.0.0.1:17891/countdown"`,
		`class="homeObsDropdown"`,
		`id="homeHelpBtn"`,
		`id="homeOpenFolderBtn"`,
		`id="homeBackupBtn"`,
		`SleepySource 1.0 Beta <span aria-hidden="true">•</span> Made by SleepyKev`,
		`id="appStartupPage" data-key="startup_page"`,
		`<option value="last_module">Last Used Module</option>`,
		`function applyStartupPage()`,
		`settings.startup_page==='last_module'`,
		`settings.last_module=valid`,
		`setActiveModule('home',false)`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("home command center is missing %q", required)
		}
	}
	for _, removed := range []string{`id="homeCopyAllObsUrlsBtn"`, `class="homeObsCopyAll"`, `Copy All URLs`, `Stream tools, all in one place.`, `One click to open`, `homePrimarySection`, `homeModuleGridPrimary`, `data-home-card=`, `id="homeNowCardStatus"`, `id="homeKickCardStatus"`, `id="homeAlertCardStatus"`, `id="homeStreamCardStatus"`, `id="homeConnectionsCardStatus"`, `id="homeCountdownCardStatus"`} {
		if strings.Contains(text, removed) {
			t.Fatalf("home OBS URL menu should not contain %q", removed)
		}
	}
	homeStart := strings.Index(text, `id="homeWorkspace"`)
	homeEnd := strings.Index(text, `id="nowPlayingWorkspace"`)
	if homeStart < 0 || homeEnd <= homeStart {
		t.Fatal("could not isolate home workspace")
	}
	homeHTML := text[homeStart:homeEnd]
	if strings.Contains(homeHTML, `data-key="startup_page"`) {
		t.Fatal("startup-page preference must live in Settings, not on Home")
	}
	settingsIndex := strings.Index(text, `<summary>Settings</summary>`)
	startupIndex := strings.Index(text, `id="appStartupPage" data-key="startup_page"`)
	if settingsIndex < 0 || startupIndex < settingsIndex {
		t.Fatal("startup-page preference is not located in the Settings controls")
	}
	defaults := defaultSettings()
	if defaults.StartupPage != "home" || defaults.LastModule != "now-playing" {
		t.Fatalf("home startup defaults are wrong: startup=%q last=%q", defaults.StartupPage, defaults.LastModule)
	}
}

func TestHome12UpdateAndGlanceLayout(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	kick := strings.Index(text, `id="homeGlanceKickValue"`)
	relay := strings.Index(text, `id="homeGlanceRelayValue"`)
	nowPlaying := strings.Index(text, `id="homeGlanceNowValue"`)
	if kick < 0 || relay < 0 || nowPlaying < 0 || !(kick < relay && relay < nowPlaying) {
		t.Fatalf("At a Glance order must be Kick -> Relay -> Now Playing: kick=%d relay=%d now=%d", kick, relay, nowPlaying)
	}
	for _, removed := range []string{`id="homeGlanceCountdownValue"`, `id="homeGlanceProfileValue"`, `id="homeGlanceVersion"`} {
		if strings.Contains(text, removed) {
			t.Fatalf("trimmed Home sidebar should not contain %q", removed)
		}
	}
	for _, required := range []string{
		`id="moduleToolsLabel"></div>`,
		`id="homeGlanceNowCard"`,
		`id="homeGlanceNowMessage">Media detection is ready.</div>`,
		`class="homeQuickBar homeQuickCompact homeQuickStandalone"`,
		`.homeQuickSection{margin-top:0}`,
		`id="homeUpdateSectionTitle">Updates</h3><span>Stable channel</span>`,
		`id="helpAboutTitle">About SleepySource™</h3>`,
		`id="headerUpdateIndicator"`,
		`Manual checks only. SleepySource never installs updates automatically.`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("SleepySource 1.3.2 Home/update layout is missing %q", required)
		}
	}
	if strings.Contains(text, `id="healthNavBadge"`) || strings.Contains(text, `class="healthNavBadge`) {
		t.Fatal("System Health navigation must not show the removed status-circle badge")
	}
	if strings.Contains(text, `checkForUpdates(false)`) {
		t.Fatal("SleepySource 1.3.2 must not check for updates automatically at startup")
	}
	for _, retired := range []string{`id="homeResumeBtn"`, `id="homeLastToolValue"`, `id="homeThemeValue"`, `homeContinuePanel`, `homeContinueBody`} {
		if strings.Contains(text, retired) {
			t.Fatalf("removed Continue workspace card unexpectedly remains: %q", retired)
		}
	}
	updatesSection := strings.Index(text, `<section class="homeSection homeDashboardGrid">`)
	quick := strings.Index(text, `<section class="homeSection homeQuickSection">`)
	footer := strings.Index(text, `<footer class="homeFooter">`)
	if updatesSection < 0 || quick < 0 || footer < 0 || !(updatesSection < quick && quick < footer) {
		t.Fatalf("Home dashboard order must be Updates -> Quick Actions -> footer: updates=%d quick=%d footer=%d", updatesSection, quick, footer)
	}
	if strings.Contains(text, `<section class="homeSection homeStatusSection">`) {
		t.Fatal("Home System Status section should be removed")
	}
}

func TestDesignerSidebarSizingGuards(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, required := range []string{
		"grid-template-columns:repeat(2,minmax(0,1fr))",
		"grid-template-columns:repeat(3,minmax(0,1fr))",
		".row>*,.row3>*,.toolbarGrid>*,.chatTestRow>*,.chatStatusGrid>*,.countdownStatusGrid>*{min-width:0}",
		"input,select,button{font:inherit;min-width:0;max-width:100%}",
		"input[type=file]{width:100%;min-width:0;max-width:100%",
		"input[type=range]{display:block;width:100%;min-width:0;margin:0}",
		".moduleButton>span:last-child{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}",
		"@media(max-width:760px)",
		"@media(max-width:520px)",
		"clamp(Number(safeStorageGet(sidebarWidthKey,'400')||400),340,650)",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("designer sizing guard is missing %q", required)
		}
	}
	if got := strings.Count(text, `class="card accordion"`); got != 20 {
		t.Fatalf("designer accordion count=%d, want 20", got)
	}
	if !strings.Contains(text, "function safeStorageGet(key,fallback='')") || !strings.Contains(text, "function safeStorageSet(key,value)") {
		t.Fatal("designer sidebar storage access is not guarded")
	}
	presetPos := strings.Index(text, "const countdownPresets={")
	syncPos := strings.Index(text, "function syncCountdownInputs()")
	listenerPos := strings.Index(text, "addEventListener('click',applyCountdownPreset)")
	if presetPos < 0 || syncPos < 0 || listenerPos < 0 || presetPos > syncPos || syncPos > listenerPos {
		t.Fatal("countdown preset functions must be defined at top level before startup listeners are registered")
	}
	for _, label := range []string{"Now Playing", "Chat Overlay", "Alert Studio", "Countdown Pro"} {
		if !strings.Contains(text, `data-module-label="`+label+`"`) {
			t.Fatalf("module switcher is missing %q", label)
		}
	}
	if !strings.Contains(text, `id="staticSystemHealthButton"`) {
		t.Fatal("standalone System Health bar is missing")
	}
	for _, required := range []string{
		`id="fontUpload" class="filePickerInput"`,
		`id="fontChooseBtn"`,
		`id="fontFileName"`,
		`id="fontRemoveActions" hidden`,
		`id="chatFontRemoveActions" hidden`,
		`id="countdownFontRemoveActions" hidden`,
		`bindFilePicker('#fontUpload','#fontChooseBtn','#fontFileName')`,
		`$('#fontUpload')?.addEventListener('change',()=>uploadFontImmediately`,
		`$('#chatFontUpload')?.addEventListener('change',()=>uploadFontImmediately`,
		`$('#countdownFontUpload')?.addEventListener('change',()=>uploadFontImmediately`,
		`syncFontRemoveControls()`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("font picker behavior is missing %q", required)
		}
	}
	for _, retired := range []string{`Upload &amp; Use Font`, `id="fontUploadBtn"`, `id="chatFontUploadBtn"`, `id="countdownFontUploadBtn"`} {
		if strings.Contains(text, retired) {
			t.Fatalf("retired two-step font uploader marker %q is still present", retired)
		}
	}
}

func TestChatConnectionControlsStayBesideCredentials(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	connections := strings.Index(text, `id="connectionsWorkspace" data-workspace-panel="connections"`)
	secret := strings.Index(text, `id="chatClientSecret"`)
	connect := strings.Index(text, `id="chatSetTokenBtn"`)
	disconnect := strings.Index(text, `id="chatClearTokenBtn"`)
	forget := strings.Index(text, `id="chatForgetLoginBtn"`)
	webhook := strings.Index(text, `id="cfWebhookUrl"`)
	reregister := strings.Index(text, `id="chatReregisterBtn"`)
	if connections < 0 || secret < 0 || connect < 0 || disconnect < 0 || forget < 0 || webhook < 0 || reregister < 0 {
		t.Fatal("Connections page Kick control markers are missing")
	}
	if !(connections < secret && secret < connect && connect < webhook && webhook < reregister) {
		t.Fatal("Kick account and webhook controls should be grouped together on Connections")
	}
	if !(connect < disconnect && disconnect < forget) {
		t.Fatal("Kick connection actions are not kept together")
	}
	for _, required := range []string{
		`data-module="connections"`,
		`data-module-label="Connections"`,
		`id="chatOpenConnectionsBtn"`,
		`id="chatKickActionStatus"`,
		`id="chatSavedLoginStatus"`,
		`if(disconnectBtn)disconnectBtn.hidden=!lookupReady&&!connected`,
		`if(forgetBtn)forgetBtn.hidden=!savedLogin`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Connections behavior is missing %q", required)
		}
	}
}

func TestRemoveCustomFontResetsEveryModuleUsingIt(t *testing.T) {
	app := newTestApp(t)
	fontName := "Shared_Custom.ttf"
	fontPath := filepath.Join(app.fontDir, fontName)
	if err := os.WriteFile(fontPath, []byte{0, 1, 0, 0}, 0644); err != nil {
		t.Fatal(err)
	}
	family := fontFamilyForFile(fontName)
	app.settings.TextFont = family
	chatSettings := app.chat.state().Settings
	chatSettings.FontFamily = family
	if err := app.chat.setSettings(chatSettings); err != nil {
		t.Fatal(err)
	}
	countdownSettings := app.countdown.state(nil).Settings
	countdownSettings.FontFamily = family
	if err := app.countdown.applySettings(countdownSettings); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/api/remove-font", strings.NewReader(`{"family":"`+family+`"}`))
	req.Host = "127.0.0.1:17891"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove font status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(fontPath); !os.IsNotExist(err) {
		t.Fatalf("font file still exists after removal: %v", err)
	}
	if app.settings.TextFont != "Segoe UI" {
		t.Fatalf("Now Playing font=%q, want Segoe UI", app.settings.TextFont)
	}
	if got := app.chat.state().Settings.FontFamily; got != "Segoe UI" {
		t.Fatalf("Chat font=%q, want Segoe UI", got)
	}
	if got := app.countdown.state(nil).Settings.FontFamily; got != "Segoe UI" {
		t.Fatalf("Countdown font=%q, want Segoe UI", got)
	}
}

func TestWindowsTitleBarOmitsVersion(t *testing.T) {
	data, err := os.ReadFile("webview2_embedded_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `utf16("SleepySource")`) {
		t.Fatal("native Windows title bar is not set to SleepySource")
	}
	if strings.Contains(text, `utf16("SleepySource "+appVersion)`) {
		t.Fatal("native Windows title bar still includes the version")
	}
}

func TestEmbeddedHTMLHasNoDuplicateIDs(t *testing.T) {
	idPattern := regexp.MustCompile(`\sid="([^"]+)"`)
	for _, name := range []string{"web/index.html", "web/enter.html", "web/overlay.html", "web/chat.html", "web/countdown.html"} {
		data, err := embedded.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		seen := make(map[string]bool)
		for _, match := range idPattern.FindAllSubmatch(data, -1) {
			id := string(match[1])
			if seen[id] {
				t.Fatalf("%s contains duplicate id=%q", name, id)
			}
			seen[id] = true
		}
	}
}

func TestHomeTabUsesRecoloredHouseAsset(t *testing.T) {
	data, err := os.ReadFile("assets/home-tab.png")
	if err != nil {
		t.Fatalf("read home tab icon: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode home tab icon: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 512 || bounds.Dy() != 512 {
		t.Fatalf("home tab icon dimensions = %dx%d, want 512x512", bounds.Dx(), bounds.Dy())
	}
	found := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A == 0 {
				continue
			}
			found = true
			if c.R != 147 || c.G != 163 || c.B != 187 {
				t.Fatalf("home tab icon visible pixel at %d,%d = #%02x%02x%02x, want #93A3BB", x, y, c.R, c.G, c.B)
			}
		}
	}
	if !found {
		t.Fatal("home tab icon has no visible pixels")
	}
}

func TestNowPlayingEffectSettingsNormalize(t *testing.T) {
	s := defaultSettings()
	s.MediaBorderWidth = 99
	s.MediaBorderColor = "bad"
	s.MediaGlowColor = "#abcdef"
	s.MediaGlowSize = 999
	s.ArtworkAnimation = "warp"
	s.ArtworkAnimationMS = 1
	s.TextAnimation = "sparkle"
	s.TextEffect = "laser"
	s.OverlayAnimation = "shake"
	s.OverlayAnimationMS = 99999
	s.BackgroundMotion = "spin"
	normalizeSettings(&s)
	if s.SchemaVersion != 69 || s.MediaBorderWidth != 20 || s.MediaBorderColor != "#3AA7FF" || s.MediaGlowColor != "#ABCDEF" || s.MediaGlowSize != 80 {
		t.Fatalf("media effect bounds = %+v", s)
	}
	if s.ArtworkAnimation != "none" || s.ArtworkAnimationMS != 800 || s.TextAnimation != "none" || s.TextEffect != "none" || s.OverlayAnimation != "none" || s.OverlayAnimationMS != 20000 || s.BackgroundMotion != "none" {
		t.Fatalf("effect normalization = %+v", s)
	}
}

func TestSleepySource12QualityOfLifeUI(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, required := range []string{
		`id="globalSaveBar"`,
		`id="globalSaveText"`,
		`id="newProfileBtn"`,
		`id="duplicateProfileBtn"`,
		`id="renameProfileBtn"`,
		`id="defaultProfileBtn"`,
		`The default profile loads automatically when SleepySource starts.`,
		`sleepysource-ui-state-v1`,
		`restoreDetailUIState()`,
		`Updates are checked only when you choose Check for Updates.`,
		`sleepysource-update-cache-v1`,
		`id="homeCopyObsSetupBtn"`,
		`SleepySource OBS Setup`,
		`http://127.0.0.1:17891/overlay`,
		`http://127.0.0.1:17891/chat`,
		`http://127.0.0.1:17891/countdown`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("SleepySource 1.3.2 QoL UI is missing %q", required)
		}
	}
	if strings.Contains(text, `id="healthNavBadge"`) || strings.Contains(text, `class="healthNavBadge`) {
		t.Fatal("System Health navigation must not show the removed status-circle badge")
	}
	if strings.Contains(text, `checkForUpdates(false)`) {
		t.Fatal("update checks must remain manual in SleepySource 1.3.2")
	}
	if strings.Contains(text, `poll();runSystemHealth`) || strings.Contains(text, `runSystemHealth();setInterval`) {
		t.Fatal("System Health must not run automatically at startup")
	}
}

func TestProfileDuplicateRenameAndDefaultStartup(t *testing.T) {
	app := newTestApp(t)
	if err := os.MkdirAll(app.profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	app.settings.TextX = 321
	if err := app.saveProfile("Main"); err != nil {
		t.Fatal(err)
	}
	copyName, err := app.duplicateProfile("Main", "")
	if err != nil {
		t.Fatal(err)
	}
	if copyName != "Main Copy" {
		t.Fatalf("duplicate name=%q, want Main Copy", copyName)
	}
	if err := app.renameProfile(copyName, "Stream Night"); err != nil {
		t.Fatal(err)
	}
	if err := app.setDefaultProfile("Stream Night"); err != nil {
		t.Fatal(err)
	}
	profiles := app.listProfiles()
	foundDefault := false
	for _, p := range profiles {
		if p.Name == "Stream Night" && p.Default {
			foundDefault = true
		}
	}
	if !foundDefault {
		t.Fatalf("default profile marker missing: %+v", profiles)
	}
	app.settings.TextX = 12
	app.loadDefaultProfileAtStartup()
	if app.settings.TextX != 321 || app.settings.DefaultProfile != "Stream Night" {
		t.Fatalf("default startup profile did not load/preserve preference: %+v", app.settings)
	}
	if err := app.deleteProfile("Stream Night"); err != nil {
		t.Fatal(err)
	}
	if app.settings.DefaultProfile != "" {
		t.Fatalf("deleting default profile should clear preference, got %q", app.settings.DefaultProfile)
	}
}

func TestChatOverlayCopyURLButtonsBound(t *testing.T) {
	index, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, required := range []string{
		`id="chatCopyUrlBtn"`,
		`id="chatCopyUrlMainBtn"`,
		`for(const sel of ['#chatCopyUrlBtn','#chatCopyUrlMainBtn'])$(sel)?.addEventListener('click',copyChatURL)`,
		`async function copyChatURL()`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("chat overlay Copy URL wiring is missing %q", required)
		}
	}
}
