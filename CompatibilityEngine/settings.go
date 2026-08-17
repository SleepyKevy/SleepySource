package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func defaultSettings() Settings {
	return Settings{
		SchemaVersion:        69,
		Format:               "artist_song",
		Template:             "{artist} - {title}",
		CanvasWidth:          900,
		CanvasHeight:         180,
		TextX:                190,
		TextY:                58,
		TextWidth:            680,
		TextSize:             40,
		TextWeight:           700,
		TextFont:             "Segoe UI",
		TextColor:            "#FFFFFF",
		TextAlign:            "left",
		TextLineHeight:       1.15,
		TextShadow:           false,
		ImageMode:            "fallback",
		ImageX:               15,
		ImageY:               15,
		ImageWidth:           150,
		ImageHeight:          150,
		ImageOpacity:         100,
		MediaFit:             "cover",
		MediaPositionX:       50,
		MediaPositionY:       50,
		MediaZoom:            100,
		MediaRadius:          12,
		MediaShadow:          false,
		MediaBrightness:      100,
		MediaContrast:        105,
		MediaSaturation:      105,
		MediaBorderWidth:     0,
		MediaBorderColor:     "#3AA7FF",
		MediaGlow:            false,
		MediaGlowColor:       "#3AA7FF",
		MediaGlowSize:        18,
		ArtworkAnimation:     "none",
		ArtworkAnimationMS:   5000,
		TextAnimation:        "none",
		TextEffect:           "none",
		OverlayAnimation:     "none",
		OverlayAnimationMS:   6000,
		BackgroundMotion:     "none",
		ShowProgress:         true,
		ProgressMode:         "elapsed",
		ProgressX:            190,
		ProgressY:            126,
		ProgressWidth:        680,
		ProgressHeight:       10,
		ProgressColor:        "#3AA7FF",
		ProgressTrackColor:   "#26384F",
		ProgressRadius:       6,
		ShowRemainingTime:    true,
		ProgressTextColor:    "#DCEBFF",
		ProgressTextSize:     13,
		TimeX:                190,
		TimeY:                140,
		TimeWidth:            680,
		TimeAlign:            "right",
		BackgroundMode:       "transparent",
		BackgroundColor:      "#000000",
		BackgroundOpacity:    100,
		BackgroundFit:        "cover",
		BackgroundPositionX:  50,
		BackgroundPositionY:  50,
		BackgroundZoom:       100,
		BackgroundRadius:     0,
		BackgroundBrightness: 100,
		BackgroundContrast:   100,
		BackgroundSaturation: 100,
		BackgroundBlur:       0,
		HideWhenPaused:       false,
		ShowWhenIdle:         false,
		ProgressStyle:        "rounded",
		TransitionStyle:      "fade",
		TransitionMS:         300,
		TransitionEasing:     "smooth",
		SnapEnabled:          true,
		GridSize:             10,
		OnboardingComplete:   false,
		DesignerTheme:        "blue",
		StartupPage:          "home",
		LastModule:           "now-playing",
		DefaultProfile:       "",
		MediaSourceMode:      "spotify",
		MediaSourceInclude:   "",
		MediaSourceExclude:   "",
	}
}

func (a *App) backupCorruptSettings(data []byte) string {
	stamp := time.Now().Format("20060102-150405")
	path := filepath.Join(a.dataDir, "settings.corrupt-"+stamp+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("could not back up corrupt settings: %v", err)
		return ""
	}
	return path
}

func (a *App) loadSettings() {
	recoveredBackup := false
	data, err := os.ReadFile(a.settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Recover a previous atomic-save backup if one exists; otherwise create
			// a clean settings file immediately so portable data is self-documenting.
			if backup, backupErr := os.ReadFile(a.settingsPath + ".bak"); backupErr == nil {
				data = backup
				recoveredBackup = true
			} else {
				if saveErr := a.saveSettingsLocked(); saveErr != nil {
					log.Printf("could not create default settings: %v", saveErr)
				}
				return
			}
		} else {
			log.Printf("could not read settings: %v", err)
			return
		}
	}

	// Merge over defaults so older settings files upgrade without losing newer defaults.
	merged := defaultSettings()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		backup := a.backupCorruptSettings(data)
		if backup != "" {
			log.Printf("invalid settings JSON was backed up to %s", backup)
		}
		a.settings = defaultSettings()
		if saveErr := a.saveSettingsLocked(); saveErr != nil {
			log.Printf("could not reset invalid settings: %v", saveErr)
		}
		return
	}
	if err := json.Unmarshal(data, &merged); err == nil {
		// Legacy v4.2.2 only stored `format`. If there was no explicit template,
		// derive the template from that legacy format on first upgrade.
		if _, hadTemplate := raw["template"]; !hadTemplate {
			merged.Template = ""
		}
		if _, hadImageMode := raw["image_mode"]; !hadImageMode {
			if strings.TrimSpace(merged.CustomImage) != "" {
				merged.ImageMode = "custom"
			} else {
				merged.ImageMode = "fallback"
			}
		}

		// v5.8 changed the clean-layout default to no shadows. v5.9 added an
		// independently positioned playback-time element. v5.10 changes the visible
		// time from remaining-only to elapsed/total (for example 1:50/3:15) without
		// changing its saved position or styling.
		storedSchemaVersion := 0
		if versionRaw, ok := raw["schema_version"]; ok {
			_ = json.Unmarshal(versionRaw, &storedSchemaVersion)
		}
		migrated := false
		if storedSchemaVersion < 58 {
			merged.TextShadow = false
			merged.MediaShadow = false
			migrated = true
		}
		if storedSchemaVersion < 59 {
			// Match the old time placement exactly: it used the progress bar's X/width
			// and sat four pixels below the bar. From v5.9 onward it can move alone.
			merged.TimeX = merged.ProgressX
			merged.TimeY = merged.ProgressY + merged.ProgressHeight + 4
			merged.TimeWidth = merged.ProgressWidth
			merged.TimeAlign = "right"
			migrated = true
		}
		if storedSchemaVersion < 60 {
			// v5.10 changes only the rendered time format. Keep all v5.9 time
			// positioning, color, size, and alignment exactly as the user saved them.
			merged.SchemaVersion = 60
			migrated = true
		}
		if storedSchemaVersion < 61 {
			// v5.12 adds settings export/import and portable custom web fonts.
			// No visual values need changing; only advance the schema marker.
			merged.SchemaVersion = 61
			migrated = true
		}
		if storedSchemaVersion < 62 {
			// v5.13 adds an optional canvas background layer. Keep existing overlays
			// transparent so upgrading never changes the user's OBS appearance.
			merged.BackgroundMode = "transparent"
			merged.SchemaVersion = 62
			migrated = true
		}
		if storedSchemaVersion < 63 {
			// v5.15 moves portable runtime data into SleepySource_Data beside the EXE.
			// No visual settings need changing.
			merged.SchemaVersion = 63
			migrated = true
		}
		if storedSchemaVersion < 64 {
			// Historical schema step. Security-clean public builds no longer
			// adjust OBS process priority or power-throttling state.
			merged.SchemaVersion = 64
			migrated = true
		}
		if storedSchemaVersion < 65 {
			// Historical schema step removes the automatic album-art artwork mode. Preserve
			// custom artwork when configured; otherwise use the built-in fallback.
			if merged.ImageMode == "album" {
				if strings.TrimSpace(merged.CustomImage) != "" {
					merged.ImageMode = "custom"
				} else {
					merged.ImageMode = "fallback"
				}
			}
			merged.SchemaVersion = 65
			migrated = true
		}
		if storedSchemaVersion < 66 {
			// Public v1.0 baseline adds Designer workflow tools, profiles, transitions,
			// progress styles, onboarding, diagnostics, and editor workflow tools.
			merged.SchemaVersion = 66
			migrated = true
		}
		if storedSchemaVersion < 67 {
			// Public v1.0 feature-complete baseline: portable profile bundles,
			// profile font packaging, advanced transitions/alignment, profile
			// and configurable media-session source filtering.
			merged.SchemaVersion = 67
			migrated = true
		}
		if storedSchemaVersion < 68 {
			// Visual-effects expansion. All new motion/effect settings default to off,
			// so upgrading preserves the exact appearance of existing overlays.
			merged.SchemaVersion = 68
			migrated = true
		}
		if storedSchemaVersion < 69 {
			// SleepySource 1.2 adds a preferred/default Now Playing profile.
			// Existing installs remain unchanged until the user chooses one.
			merged.SchemaVersion = 69
			migrated = true
		}

		normalizeSettings(&merged)
		a.settings = merged
		if migrated || recoveredBackup {
			if err := a.saveSettingsLocked(); err != nil {
				log.Printf("could not persist migrated/recovered settings: %v", err)
			}
		}
	}
}

var hexColorRE = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func normalizeColor(value, fallback string) string {
	value = strings.TrimSpace(value)
	if !hexColorRE.MatchString(value) {
		return fallback
	}
	return strings.ToUpper(value)
}

func clampIntValue(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clampPosition(v int) int { return clampIntValue(v, -7680, 7680) }
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func normalizeSettings(s *Settings) {
	if s.SchemaVersion < 69 {
		s.SchemaVersion = 69
	}
	if s.Format != "song_artist" {
		s.Format = "artist_song"
	}
	s.Template = strings.TrimSpace(s.Template)
	if s.Template == "" {
		if s.Format == "song_artist" {
			s.Template = "{title} • {artist}"
		} else {
			s.Template = "{artist} - {title}"
		}
	}
	if templateRunes := []rune(s.Template); len(templateRunes) > 512 {
		s.Template = string(templateRunes[:512])
	}
	if s.CanvasWidth < 100 {
		s.CanvasWidth = 900
	}
	if s.CanvasHeight < 50 {
		s.CanvasHeight = 180
	}
	s.CanvasWidth = clampIntValue(s.CanvasWidth, 100, 7680)
	s.CanvasHeight = clampIntValue(s.CanvasHeight, 50, 4320)
	s.TextX = clampPosition(s.TextX)
	s.TextY = clampPosition(s.TextY)
	s.ImageX = clampPosition(s.ImageX)
	s.ImageY = clampPosition(s.ImageY)
	s.ProgressX = clampPosition(s.ProgressX)
	s.ProgressY = clampPosition(s.ProgressY)
	s.TimeX = clampPosition(s.TimeX)
	s.TimeY = clampPosition(s.TimeY)
	if s.TextWidth < 40 {
		s.TextWidth = 680
	}
	s.TextWidth = clampIntValue(s.TextWidth, 40, 7680)
	if s.TextSize < 8 {
		s.TextSize = 40
	}
	if s.TextSize > 200 {
		s.TextSize = 200
	}
	if s.TextWeight < 100 || s.TextWeight > 900 {
		s.TextWeight = 700
	}
	s.TextFont = strings.TrimSpace(s.TextFont)
	if s.TextFont == "" || len(s.TextFont) > 128 {
		s.TextFont = "Segoe UI"
	}
	s.TextColor = normalizeColor(s.TextColor, "#FFFFFF")
	switch s.TextAlign {
	case "center", "right":
	default:
		s.TextAlign = "left"
	}
	if s.TextLineHeight < 0.7 || s.TextLineHeight > 3 {
		s.TextLineHeight = 1.15
	}
	switch s.ImageMode {
	case "custom", "fallback":
	default:
		s.ImageMode = "fallback"
	}
	if s.ImageWidth < 1 {
		s.ImageWidth = 150
	}
	if s.ImageHeight < 1 {
		s.ImageHeight = 150
	}
	s.ImageWidth = clampIntValue(s.ImageWidth, 1, 7680)
	s.ImageHeight = clampIntValue(s.ImageHeight, 1, 4320)
	if s.ImageOpacity < 0 {
		s.ImageOpacity = 0
	}
	if s.ImageOpacity > 100 {
		s.ImageOpacity = 100
	}
	switch s.MediaFit {
	case "contain", "fill":
	default:
		s.MediaFit = "cover"
	}
	if s.MediaPositionX < 0 {
		s.MediaPositionX = 0
	}
	if s.MediaPositionX > 100 {
		s.MediaPositionX = 100
	}
	if s.MediaPositionY < 0 {
		s.MediaPositionY = 0
	}
	if s.MediaPositionY > 100 {
		s.MediaPositionY = 100
	}
	if s.MediaZoom < 50 || s.MediaZoom > 250 {
		s.MediaZoom = 100
	}
	if s.MediaRadius < 0 {
		s.MediaRadius = 0
	}
	if s.MediaRadius > 100 {
		s.MediaRadius = 100
	}
	if s.MediaBrightness < 25 || s.MediaBrightness > 200 {
		s.MediaBrightness = 100
	}
	if s.MediaContrast < 25 || s.MediaContrast > 200 {
		s.MediaContrast = 105
	}
	if s.MediaSaturation < 0 || s.MediaSaturation > 250 {
		s.MediaSaturation = 105
	}
	s.MediaBorderWidth = clampIntValue(s.MediaBorderWidth, 0, 20)
	s.MediaBorderColor = normalizeColor(s.MediaBorderColor, "#3AA7FF")
	s.MediaGlowColor = normalizeColor(s.MediaGlowColor, "#3AA7FF")
	s.MediaGlowSize = clampIntValue(s.MediaGlowSize, 0, 80)
	s.ArtworkAnimationMS = clampIntValue(s.ArtworkAnimationMS, 800, 20000)
	s.OverlayAnimationMS = clampIntValue(s.OverlayAnimationMS, 800, 20000)
	switch s.ArtworkAnimation {
	case "float", "pulse", "breathe", "tilt", "slow-rotate":
	default:
		s.ArtworkAnimation = "none"
	}
	switch s.TextAnimation {
	case "float", "pulse", "breathe", "shimmer":
	default:
		s.TextAnimation = "none"
	}
	switch s.TextEffect {
	case "soft-glow", "neon", "outline":
	default:
		s.TextEffect = "none"
	}
	switch s.OverlayAnimation {
	case "float", "pulse", "breathe":
	default:
		s.OverlayAnimation = "none"
	}
	switch s.BackgroundMotion {
	case "slow-zoom", "pan-left", "pan-right":
	default:
		s.BackgroundMotion = "none"
	}
	switch s.BackgroundMode {
	case "solid", "custom":
	default:
		s.BackgroundMode = "transparent"
	}
	s.BackgroundColor = normalizeColor(s.BackgroundColor, "#000000")
	if s.BackgroundOpacity < 0 {
		s.BackgroundOpacity = 0
	}
	if s.BackgroundOpacity > 100 {
		s.BackgroundOpacity = 100
	}
	switch s.BackgroundFit {
	case "contain", "fill":
	default:
		s.BackgroundFit = "cover"
	}
	if s.BackgroundPositionX < 0 {
		s.BackgroundPositionX = 0
	}
	if s.BackgroundPositionX > 100 {
		s.BackgroundPositionX = 100
	}
	if s.BackgroundPositionY < 0 {
		s.BackgroundPositionY = 0
	}
	if s.BackgroundPositionY > 100 {
		s.BackgroundPositionY = 100
	}
	if s.BackgroundZoom < 50 || s.BackgroundZoom > 250 {
		s.BackgroundZoom = 100
	}
	if s.BackgroundRadius < 0 {
		s.BackgroundRadius = 0
	}
	if s.BackgroundRadius > 200 {
		s.BackgroundRadius = 200
	}
	if s.BackgroundBrightness < 25 || s.BackgroundBrightness > 200 {
		s.BackgroundBrightness = 100
	}
	if s.BackgroundContrast < 25 || s.BackgroundContrast > 200 {
		s.BackgroundContrast = 100
	}
	if s.BackgroundSaturation < 0 || s.BackgroundSaturation > 250 {
		s.BackgroundSaturation = 100
	}
	if s.BackgroundBlur < 0 {
		s.BackgroundBlur = 0
	}
	if s.BackgroundBlur > 40 {
		s.BackgroundBlur = 40
	}
	if s.ProgressMode != "remaining" {
		s.ProgressMode = "elapsed"
	}
	if s.ProgressWidth < 20 {
		s.ProgressWidth = 680
	}
	s.ProgressWidth = clampIntValue(s.ProgressWidth, 20, 7680)
	if s.ProgressHeight < 2 {
		s.ProgressHeight = 10
	}
	if s.ProgressHeight > 80 {
		s.ProgressHeight = 80
	}
	s.ProgressColor = normalizeColor(s.ProgressColor, "#3AA7FF")
	s.ProgressTrackColor = normalizeColor(s.ProgressTrackColor, "#26384F")
	if s.ProgressRadius < 0 {
		s.ProgressRadius = 0
	}
	if s.ProgressRadius > 40 {
		s.ProgressRadius = 40
	}
	s.ProgressTextColor = normalizeColor(s.ProgressTextColor, "#DCEBFF")
	if s.ProgressTextSize < 8 || s.ProgressTextSize > 80 {
		s.ProgressTextSize = 13
	}
	if s.TimeWidth < 20 {
		s.TimeWidth = 680
	}
	s.TimeWidth = clampIntValue(s.TimeWidth, 20, 7680)
	switch s.TimeAlign {
	case "left", "center":
	default:
		s.TimeAlign = "right"
	}
	switch s.ProgressStyle {
	case "square", "pill", "glow", "segmented", "gradient":
	default:
		s.ProgressStyle = "rounded"
	}
	switch s.TransitionStyle {
	case "none", "slide-left", "slide-right", "slide-up", "slide-down", "scale", "zoom-in", "zoom-out", "blur", "flip":
	default:
		s.TransitionStyle = "fade"
	}
	s.TransitionMS = clampIntValue(s.TransitionMS, 0, 5000)
	switch s.TransitionEasing {
	case "linear", "ease", "ease-in", "ease-out", "ease-in-out", "snappy", "spring":
	default:
		s.TransitionEasing = "smooth"
	}
	if s.GridSize < 1 || s.GridSize > 200 {
		s.GridSize = 10
	}
	switch s.DesignerTheme {
	case "midnight":
		s.DesignerTheme = "blue"
	case "violet":
		s.DesignerTheme = "purple"
	case "forest":
		s.DesignerTheme = "green"
	case "blue", "red", "purple", "green", "pink":
	default:
		s.DesignerTheme = "blue"
	}
	switch s.StartupPage {
	case "last_module":
	default:
		s.StartupPage = "home"
	}
	switch s.LastModule {
	case "chat-overlay", "stream-settings", "connections", "countdown-pro":
	default:
		s.LastModule = "now-playing"
	}
	s.DefaultProfile = sanitizeProfileName(s.DefaultProfile)
	switch s.MediaSourceMode {
	case "any", "custom":
	default:
		s.MediaSourceMode = "spotify"
	}
	s.MediaSourceInclude = strings.TrimSpace(s.MediaSourceInclude)
	s.MediaSourceExclude = strings.TrimSpace(s.MediaSourceExclude)
	if len(s.MediaSourceInclude) > 512 {
		s.MediaSourceInclude = s.MediaSourceInclude[:512]
	}
	if len(s.MediaSourceExclude) > 512 {
		s.MediaSourceExclude = s.MediaSourceExclude[:512]
	}
}

func (a *App) saveSettingsLocked() error {
	normalizeSettings(&a.settings)
	data, err := json.MarshalIndent(a.settings, "", "  ")
	if err != nil {
		return err
	}
	return writeSettingsFile(a.settingsPath, data)
}

// writeSettingsFile is deliberately defensive on Windows. Color pickers and
// sliders can generate many edits in a short period, while antivirus/indexing
// software may briefly hold settings.json open. A unique temp file plus short
// retries prevents those transient locks from surfacing as a user-visible
// "Save error" and avoids collisions with an older .tmp file.
func commitTempFile(tmpName, target string, attempts int) error {
	if attempts < 1 {
		attempts = 1
	}
	backup := target + ".bak"
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		// Fast path. On platforms/filesystems that support replacement, this is
		// atomic and never exposes a missing destination.
		if err := os.Rename(tmpName, target); err == nil {
			_ = os.Remove(backup)
			return nil
		} else {
			lastErr = err
		}

		// Conservative Windows fallback: move the old file aside, commit the new
		// one, and roll the old file back if the second rename fails.
		_ = os.Remove(backup)
		if _, err := os.Stat(target); err == nil {
			if err := os.Rename(target, backup); err == nil {
				if err := os.Rename(tmpName, target); err == nil {
					_ = os.Remove(backup)
					return nil
				} else {
					lastErr = err
					_ = os.Rename(backup, target)
				}
			} else {
				lastErr = err
			}
		} else if os.IsNotExist(err) {
			if err := os.Rename(tmpName, target); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
		time.Sleep(time.Duration(25*(attempt+1)) * time.Millisecond)
	}
	return lastErr
}

func writeSettingsFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(dir, ".sleepysource-settings-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	_ = tmpFile.Chmod(0644)
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := commitTempFile(tmpName, path, 8); err == nil {
		cleanup = false
		return nil
	} else {
		return fmt.Errorf("settings save failed after retries: %w", err)
	}
}
