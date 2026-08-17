package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) findCustomImage() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.settings.CustomImage != "" {
		p := filepath.Join(a.customDir, filepath.Base(a.settings.CustomImage))
		if _, err := os.Stat(p); err == nil {
			a.customPath = p
			return
		}
		a.settings.CustomImage = ""
	}
	entries, _ := os.ReadDir(a.customDir)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(strings.ToLower(entry.Name()), "custom_now_playing.") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" || ext == ".gif" || ext == ".webm" {
			a.customPath = filepath.Join(a.customDir, entry.Name())
			a.settings.CustomImage = entry.Name()
			_ = a.saveSettingsLocked()
			return
		}
	}
}

func (a *App) findCustomBackground() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.settings.CustomBackground != "" {
		p := filepath.Join(a.customDir, filepath.Base(a.settings.CustomBackground))
		if _, err := os.Stat(p); err == nil {
			a.backgroundPath = p
			return
		}
		a.settings.CustomBackground = ""
	}
	entries, _ := os.ReadDir(a.customDir)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(strings.ToLower(entry.Name()), "custom_background.") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" || ext == ".gif" || ext == ".webm" {
			a.backgroundPath = filepath.Join(a.customDir, entry.Name())
			a.settings.CustomBackground = entry.Name()
			_ = a.saveSettingsLocked()
			return
		}
	}
}

func (a *App) renderText(track Track, s Settings) string {
	if !track.Found || (strings.TrimSpace(track.Artist) == "" && strings.TrimSpace(track.Title) == "") {
		if s.ShowWhenIdle {
			return applyTemplate(s.Template, "Artist Name", "Song Title")
		}
		return ""
	}
	return applyTemplate(s.Template, track.Artist, track.Title)
}

func applyTemplate(tmpl, artist, title string) string {
	r := strings.NewReplacer(
		"{artist}", artist,
		"{title}", title,
		"{song}", title,
		"{Artist}", artist,
		"{Title}", title,
	)
	return r.Replace(tmpl)
}

func (a *App) writeOutput(track Track) {
	a.mu.RLock()
	s := a.settings
	a.mu.RUnlock()
	text := a.renderText(track, s)
	if !track.Found && !s.ShowWhenIdle {
		text = ""
	}

	// Timeline samples arrive frequently, but the legacy OBS text only changes
	// when the song/text changes. Avoid needless disk writes on every position tick.
	a.outputMu.Lock()
	defer a.outputMu.Unlock()
	if text == a.lastOutput {
		if _, err := os.Stat(a.outputPath); err == nil {
			return
		}
	}
	data := []byte(text)
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if err := os.WriteFile(a.outputPath, data, 0644); err == nil {
			a.lastOutput = text
			return
		} else {
			lastErr = err
		}
		time.Sleep(time.Duration(15*(attempt+1)) * time.Millisecond)
	}
	if lastErr != nil {
		log.Printf("legacy text output write failed: %v", lastErr)
	}
}

func (a *App) updateTrack(t Track, detector string) {
	t.SampledAtMS = time.Now().UnixMilli()
	if t.PositionMS < 0 {
		t.PositionMS = 0
	}
	if t.DurationMS < 0 {
		t.DurationMS = 0
	}
	if t.DurationMS > 0 && t.PositionMS > t.DurationMS {
		t.PositionMS = t.DurationMS
	}
	a.mu.Lock()
	changed := t != a.track
	a.track = t
	if detector != "" {
		a.detectorStatus = detector
	}
	if changed {
		a.updatedAt = time.Now().UnixMilli()
	}
	a.mu.Unlock()
	a.writeOutput(t)
}

func (a *App) snapshot() AppState {
	a.mu.RLock()
	s := a.settings
	t := a.track
	updated := a.updatedAt
	detector := a.detectorStatus
	overlayFPS := a.overlayFPS
	overlayFrameMS := a.overlayFrameMS
	overlayMetricsAt := a.overlayMetricsAt
	customPath := a.customPath
	backgroundPath := a.backgroundPath
	a.mu.RUnlock()

	display := a.renderText(t, s)
	visible := true
	if !t.Found && !s.ShowWhenIdle {
		visible = false
	}
	if t.Found && s.HideWhenPaused && !strings.EqualFold(t.Status, "Playing") {
		visible = false
	}

	imageURL := "/assets/default.png"
	imageName := "Built-in fallback artwork"
	mediaKind := "image"
	switch s.ImageMode {
	case "custom":
		if customPath != "" {
			imageURL = "/media/custom"
			imageName = filepath.Base(customPath)
			if strings.EqualFold(filepath.Ext(customPath), ".webm") {
				mediaKind = "video"
			}
			if info, err := os.Stat(customPath); err == nil {
				imageURL += fmt.Sprintf("?v=%d", info.ModTime().UnixNano())
			}
		} else {
			imageName = "Custom image not set — using fallback"
		}
	}

	backgroundURL := ""
	backgroundName := "Transparent"
	backgroundKind := "image"
	if s.BackgroundMode == "solid" {
		backgroundName = "Solid color " + s.BackgroundColor
	} else if s.BackgroundMode == "custom" {
		if backgroundPath != "" {
			backgroundURL = "/media/background"
			backgroundName = filepath.Base(backgroundPath)
			if strings.EqualFold(filepath.Ext(backgroundPath), ".webm") {
				backgroundKind = "video"
			}
			if info, err := os.Stat(backgroundPath); err == nil {
				backgroundURL += fmt.Sprintf("?v=%d", info.ModTime().UnixNano())
			}
		} else {
			backgroundName = "Custom background not set — transparent"
		}
	}

	return AppState{
		Version:          displayVersion,
		Track:            t,
		DisplayText:      display,
		Settings:         s,
		ImageURL:         imageURL,
		ImageName:        imageName,
		MediaKind:        mediaKind,
		BackgroundURL:    backgroundURL,
		BackgroundName:   backgroundName,
		BackgroundKind:   backgroundKind,
		Visible:          visible,
		UpdatedAt:        updated,
		Detector:         detector,
		OverlayFPS:       overlayFPS,
		OverlayFrameMS:   overlayFrameMS,
		OverlayMetricsAt: overlayMetricsAt,
		Fonts:            a.listFonts(),
		Profiles:         a.listProfiles(),
		Diagnostics: MediaDiagnostics{
			Found:       t.Found,
			Source:      t.Source,
			Status:      t.Status,
			HasTimeline: t.DurationMS > 0,
			PositionMS:  t.PositionMS,
			DurationMS:  t.DurationMS,
			SampleAgeMS: func() int64 {
				if t.SampledAtMS <= 0 {
					return 0
				}
				return maxInt64(0, time.Now().UnixMilli()-t.SampledAtMS)
			}(),
			Detector:       detector,
			DataDirectory:  a.dataDir,
			OverlayAddress: overlayURL,
		},
	}
}
