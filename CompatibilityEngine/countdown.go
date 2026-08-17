package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const countdownOverlayURL = "http://127.0.0.1:17891/countdown"

type CountdownSettings struct {
	SchemaVersion int `json:"schema_version"`

	Mode          string `json:"mode"`
	Hours         int    `json:"hours"`
	Minutes       int    `json:"minutes"`
	Seconds       int    `json:"seconds"`
	Format        string `json:"format"`
	CustomFormat  string `json:"custom_format"`
	Prefix        string `json:"prefix"`
	Suffix        string `json:"suffix"`
	FinishedText  string `json:"finished_text"`
	BlankOnFinish bool   `json:"blank_on_finish"`
	Loop          bool   `json:"loop"`
	Overtime      bool   `json:"overtime"`
	StartBehavior string `json:"start_behavior"`
	RestartOnLoad bool   `json:"restart_on_load"`
	ResetOnUnload bool   `json:"reset_on_unload"`

	CanvasWidth       int    `json:"canvas_width"`
	CanvasHeight      int    `json:"canvas_height"`
	CanvasTransparent bool   `json:"canvas_transparent"`
	CanvasColor       string `json:"canvas_color"`
	CanvasOpacity     int    `json:"canvas_opacity"`

	TimerX         int     `json:"timer_x"`
	TimerY         int     `json:"timer_y"`
	TimerWidth     int     `json:"timer_width"`
	FontFamily     string  `json:"font_family"`
	FontSize       int     `json:"font_size"`
	FontWeight     int     `json:"font_weight"`
	TextColor      string  `json:"text_color"`
	TextOpacity    int     `json:"text_opacity"`
	Align          string  `json:"align"`
	LetterSpacing  float64 `json:"letter_spacing"`
	LineHeight     float64 `json:"line_height"`
	TextShadow     bool    `json:"text_shadow"`
	Outline        bool    `json:"outline"`
	OutlineSize    int     `json:"outline_size"`
	OutlineColor   string  `json:"outline_color"`
	OutlineOpacity int     `json:"outline_opacity"`

	PanelEnabled  bool   `json:"panel_enabled"`
	PanelColor    string `json:"panel_color"`
	PanelOpacity  int    `json:"panel_opacity"`
	PanelRadius   int    `json:"panel_radius"`
	PanelPadding  int    `json:"panel_padding"`
	BorderWidth   int    `json:"border_width"`
	BorderColor   string `json:"border_color"`
	BorderOpacity int    `json:"border_opacity"`

	TimerAnimation   string `json:"timer_animation"`
	TickAnimation    string `json:"tick_animation"`
	PanelAnimation   string `json:"panel_animation"`
	OverlayAnimation string `json:"overlay_animation"`
	AnimationMS      int    `json:"animation_ms"`
}

type CountdownState struct {
	Settings    CountdownSettings `json:"settings"`
	CurrentMS   int64             `json:"current_ms"`
	DurationMS  int64             `json:"duration_ms"`
	Running     bool              `json:"running"`
	Paused      bool              `json:"paused"`
	Finished    bool              `json:"finished"`
	HasStarted  bool              `json:"has_started"`
	DisplayText string            `json:"display_text"`
	ServerNowMS int64             `json:"server_now_ms"`
	UpdatedAtMS int64             `json:"updated_at_ms"`
	OverlayURL  string            `json:"overlay_url"`
	Fonts       []FontInfo        `json:"fonts,omitempty"`
	Profiles    []ProfileInfo     `json:"profiles,omitempty"`
}

type CountdownManager struct {
	mu           sync.Mutex
	settings     CountdownSettings
	settingsPath string
	profileDir   string
	baseMS       int64
	startAt      time.Time
	running      bool
	paused       bool
	finished     bool
	hasStarted   bool
	updatedAtMS  int64
}

func defaultCountdownSettings() CountdownSettings {
	return CountdownSettings{
		SchemaVersion:     2,
		Mode:              "countdown",
		Hours:             0,
		Minutes:           5,
		Seconds:           0,
		Format:            "auto",
		CustomFormat:      "{hh}:{mm}:{ss}",
		Prefix:            "",
		Suffix:            "",
		FinishedText:      "STARTING NOW",
		BlankOnFinish:     false,
		Loop:              false,
		Overtime:          false,
		StartBehavior:     "manual",
		RestartOnLoad:     false,
		CanvasWidth:       900,
		CanvasHeight:      220,
		CanvasTransparent: true,
		CanvasColor:       "#000000",
		CanvasOpacity:     100,
		TimerX:            50,
		TimerY:            45,
		TimerWidth:        800,
		FontFamily:        "Segoe UI",
		FontSize:          96,
		FontWeight:        700,
		TextColor:         "#FFFFFF",
		TextOpacity:       100,
		Align:             "center",
		LetterSpacing:     0,
		LineHeight:        1,
		TextShadow:        true,
		Outline:           true,
		OutlineSize:       3,
		OutlineColor:      "#000000",
		OutlineOpacity:    100,
		PanelEnabled:      false,
		PanelColor:        "#07111F",
		PanelOpacity:      70,
		PanelRadius:       18,
		PanelPadding:      18,
		BorderWidth:       0,
		BorderColor:       "#3AA7FF",
		BorderOpacity:     100,
		TimerAnimation:    "none",
		TickAnimation:     "none",
		PanelAnimation:    "none",
		OverlayAnimation:  "none",
		AnimationMS:       1800,
	}
}

func normalizeHexColor(value, fallback string) string {
	value = strings.TrimSpace(value)
	if len(value) == 7 && value[0] == '#' {
		for _, ch := range value[1:] {
			if !(ch >= '0' && ch <= '9') && !(ch >= 'a' && ch <= 'f') && !(ch >= 'A' && ch <= 'F') {
				return fallback
			}
		}
		return strings.ToUpper(value)
	}
	return fallback
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func normalizeCountdownSettings(s *CountdownSettings) {
	if s == nil {
		return
	}
	s.SchemaVersion = 2
	switch s.Mode {
	case "countdown", "stopwatch":
	default:
		s.Mode = "countdown"
	}
	s.Hours = clampInt(s.Hours, 0, 999)
	s.Minutes = clampInt(s.Minutes, 0, 59)
	s.Seconds = clampInt(s.Seconds, 0, 59)
	switch s.Format {
	case "auto", "hhmmss", "mmss", "seconds", "custom":
	default:
		s.Format = "auto"
	}
	if len(s.CustomFormat) > 256 {
		s.CustomFormat = s.CustomFormat[:256]
	}
	if strings.TrimSpace(s.CustomFormat) == "" {
		s.CustomFormat = "{hh}:{mm}:{ss}"
	}
	if len(s.Prefix) > 128 {
		s.Prefix = s.Prefix[:128]
	}
	if len(s.Suffix) > 128 {
		s.Suffix = s.Suffix[:128]
	}
	if len(s.FinishedText) > 256 {
		s.FinishedText = s.FinishedText[:256]
	}
	switch s.StartBehavior {
	case "manual", "app-start", "overlay-load":
	default:
		s.StartBehavior = "manual"
	}

	s.CanvasWidth = clampInt(s.CanvasWidth, 100, 3840)
	s.CanvasHeight = clampInt(s.CanvasHeight, 60, 2160)
	s.CanvasColor = normalizeHexColor(s.CanvasColor, "#000000")
	s.CanvasOpacity = clampInt(s.CanvasOpacity, 0, 100)

	s.TimerWidth = clampInt(s.TimerWidth, 40, 3840)
	s.TimerX = clampInt(s.TimerX, -3840, 3840)
	s.TimerY = clampInt(s.TimerY, -2160, 2160)
	if strings.TrimSpace(s.FontFamily) == "" || len(s.FontFamily) > 128 {
		s.FontFamily = "Segoe UI"
	}
	s.FontSize = clampInt(s.FontSize, 8, 400)
	switch s.FontWeight {
	case 100, 200, 300, 400, 500, 600, 700, 800, 900:
	default:
		s.FontWeight = 700
	}
	s.TextColor = normalizeHexColor(s.TextColor, "#FFFFFF")
	s.TextOpacity = clampInt(s.TextOpacity, 0, 100)
	switch s.Align {
	case "left", "center", "right":
	default:
		s.Align = "center"
	}
	s.LetterSpacing = clampFloat(s.LetterSpacing, -10, 50)
	s.LineHeight = clampFloat(s.LineHeight, 0.6, 3)
	s.OutlineSize = clampInt(s.OutlineSize, 0, 20)
	s.OutlineColor = normalizeHexColor(s.OutlineColor, "#000000")
	s.OutlineOpacity = clampInt(s.OutlineOpacity, 0, 100)

	s.PanelColor = normalizeHexColor(s.PanelColor, "#07111F")
	s.PanelOpacity = clampInt(s.PanelOpacity, 0, 100)
	s.PanelRadius = clampInt(s.PanelRadius, 0, 200)
	s.PanelPadding = clampInt(s.PanelPadding, 0, 120)
	s.BorderWidth = clampInt(s.BorderWidth, 0, 20)
	s.BorderColor = normalizeHexColor(s.BorderColor, "#3AA7FF")
	s.BorderOpacity = clampInt(s.BorderOpacity, 0, 100)
	s.AnimationMS = clampInt(s.AnimationMS, 250, 12000)
	switch s.TimerAnimation {
	case "float", "pulse", "breathe", "glow", "tilt":
	default:
		s.TimerAnimation = "none"
	}
	switch s.TickAnimation {
	case "pop", "flip", "slide", "pulse":
	default:
		s.TickAnimation = "none"
	}
	switch s.PanelAnimation {
	case "breathe", "glow", "shimmer", "pulse":
	default:
		s.PanelAnimation = "none"
	}
	switch s.OverlayAnimation {
	case "fade", "slide-up", "zoom":
	default:
		s.OverlayAnimation = "none"
	}
}

func countdownDurationMS(s CountdownSettings) int64 {
	return (int64(s.Hours)*3600 + int64(s.Minutes)*60 + int64(s.Seconds)) * 1000
}

func newCountdownManager(dataDir string) *CountdownManager {
	m := &CountdownManager{
		settings:     defaultCountdownSettings(),
		settingsPath: filepath.Join(dataDir, "countdown_settings.json"),
		profileDir:   filepath.Join(dataDir, "CountdownProfiles"),
	}
	if err := os.MkdirAll(m.profileDir, 0755); err != nil {
		log.Printf("could not create countdown profile folder: %v", err)
	}
	m.reloadFromDisk()
	m.mu.Lock()
	if m.settings.StartBehavior == "app-start" {
		m.startLocked(time.Now())
	}
	m.mu.Unlock()
	return m
}

func (m *CountdownManager) backupCorrupt(data []byte) {
	path := filepath.Join(filepath.Dir(m.settingsPath), "countdown_settings.corrupt-"+time.Now().Format("20060102-150405")+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("could not back up corrupt countdown settings: %v", err)
	}
}

func (m *CountdownManager) reloadFromDisk() {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := defaultCountdownSettings()
	data, err := os.ReadFile(m.settingsPath)
	needsSave := err != nil || len(data) == 0
	if err == nil {
		var raw map[string]json.RawMessage
		if json.Unmarshal(data, &raw) != nil || raw == nil || json.Unmarshal(data, &next) != nil {
			m.backupCorrupt(data)
			next = defaultCountdownSettings()
			needsSave = true
		}
	}
	normalizeCountdownSettings(&next)
	m.settings = next
	m.running = false
	m.paused = false
	m.finished = false
	m.hasStarted = false
	if next.Mode == "countdown" {
		m.baseMS = countdownDurationMS(next)
	} else {
		m.baseMS = 0
	}
	m.startAt = time.Now()
	m.updatedAtMS = time.Now().UnixMilli()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("could not read countdown settings: %v", err)
	}
	if needsSave {
		if saveErr := m.saveLocked(); saveErr != nil {
			log.Printf("could not save recovered countdown settings: %v", saveErr)
		}
	}
}

func (m *CountdownManager) saveLocked() error {
	normalizeCountdownSettings(&m.settings)
	data, err := json.MarshalIndent(m.settings, "", "  ")
	if err != nil {
		return err
	}
	return writeSettingsFile(m.settingsPath, data)
}

func (m *CountdownManager) currentMSLocked(now time.Time) int64 {
	if !m.running {
		return m.baseMS
	}
	elapsed := now.Sub(m.startAt).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}
	if m.settings.Mode == "countdown" {
		return m.baseMS - elapsed
	}
	return m.baseMS + elapsed
}

func (m *CountdownManager) settleLocked(now time.Time) int64 {
	value := m.currentMSLocked(now)
	if m.settings.Mode != "countdown" || !m.running || m.settings.Overtime || value > 0 {
		return value
	}
	duration := countdownDurationMS(m.settings)
	if m.settings.Loop && duration > 0 {
		cycles := ((-value) / duration) + 1
		value += cycles * duration
		m.baseMS = value
		m.startAt = now
		m.finished = false
		return value
	}
	m.baseMS = 0
	m.running = false
	m.paused = false
	m.finished = true
	m.updatedAtMS = now.UnixMilli()
	return 0
}

func (m *CountdownManager) resetLocked(now time.Time, clearStarted bool) {
	m.running = false
	m.paused = false
	m.finished = false
	if clearStarted {
		m.hasStarted = false
	}
	if m.settings.Mode == "countdown" {
		m.baseMS = countdownDurationMS(m.settings)
	} else {
		m.baseMS = 0
	}
	m.startAt = now
	m.updatedAtMS = now.UnixMilli()
}

func (m *CountdownManager) startLocked(now time.Time) {
	if m.running {
		return
	}
	if m.settings.Mode == "countdown" && m.finished {
		m.baseMS = countdownDurationMS(m.settings)
		m.finished = false
	}
	m.startAt = now
	m.running = true
	m.paused = false
	m.hasStarted = true
	m.updatedAtMS = now.UnixMilli()
}

func (m *CountdownManager) pauseLocked(now time.Time) {
	if !m.running {
		return
	}
	m.baseMS = m.settleLocked(now)
	if m.finished {
		return
	}
	m.running = false
	m.paused = true
	m.updatedAtMS = now.UnixMilli()
}

func (m *CountdownManager) stopLocked(now time.Time) {
	if !m.running {
		if !m.paused && !m.hasStarted {
			return
		}
		m.paused = false
		m.updatedAtMS = now.UnixMilli()
		return
	}
	m.baseMS = m.settleLocked(now)
	if m.finished {
		return
	}
	m.running = false
	m.paused = false
	m.hasStarted = true
	m.startAt = now
	m.updatedAtMS = now.UnixMilli()
}

func (m *CountdownManager) adjustLocked(now time.Time, deltaMS int64) {
	value := m.settleLocked(now) + deltaMS
	if !m.settings.Overtime && value < 0 {
		value = 0
	}
	m.baseMS = value
	m.startAt = now
	m.finished = false
	m.updatedAtMS = now.UnixMilli()
}

func (m *CountdownManager) applySettings(next CountdownSettings) error {
	normalizeCountdownSettings(&next)
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	current := m.settleLocked(now)
	oldMode := m.settings.Mode
	oldDuration := countdownDurationMS(m.settings)
	wasRunning := m.running
	wasPaused := m.paused
	hadStarted := m.hasStarted
	m.settings = next
	newDuration := countdownDurationMS(next)

	if oldMode != next.Mode {
		m.resetLocked(now, true)
	} else if next.Mode == "countdown" && !wasRunning && oldDuration != newDuration {
		m.baseMS = newDuration
		m.finished = false
	} else {
		m.baseMS = current
		m.startAt = now
		m.running = wasRunning
		m.paused = wasPaused
		m.hasStarted = hadStarted
	}
	m.updatedAtMS = now.UnixMilli()
	return m.saveLocked()
}

func countdownDisplaySeconds(mode string, ms int64) int64 {
	if mode == "countdown" {
		if ms > 0 {
			return (ms + 999) / 1000
		}
		if ms < 0 {
			return -(((-ms) + 999) / 1000)
		}
		return 0
	}
	return ms / 1000
}

func appendPadded(b *strings.Builder, value uint64, width int) { fmt.Fprintf(b, "%0*d", width, value) }

func formatCountdownCustom(format string, seconds int64) string {
	negative := seconds < 0
	var abs uint64
	if negative {
		abs = uint64(-(seconds + 1)) + 1
	} else {
		abs = uint64(seconds)
	}
	days := abs / 86400
	totalHours := abs / 3600
	hours := totalHours % 24
	minutes := (abs / 60) % 60
	secs := abs % 60
	var b strings.Builder
	for i := 0; i < len(format); {
		rest := format[i:]
		switch {
		case strings.HasPrefix(rest, "{sign}"):
			if negative {
				b.WriteByte('-')
			}
			i += 6
		case strings.HasPrefix(rest, "{hh}"):
			appendPadded(&b, hours, 2)
			i += 4
		case strings.HasPrefix(rest, "{mm}"):
			appendPadded(&b, minutes, 2)
			i += 4
		case strings.HasPrefix(rest, "{ss}"):
			appendPadded(&b, secs, 2)
			i += 4
		case strings.HasPrefix(rest, "{d}"):
			fmt.Fprintf(&b, "%d", days)
			i += 3
		case strings.HasPrefix(rest, "{h}"):
			fmt.Fprintf(&b, "%d", hours)
			i += 3
		case strings.HasPrefix(rest, "{m}"):
			fmt.Fprintf(&b, "%d", minutes)
			i += 3
		case strings.HasPrefix(rest, "{s}"):
			fmt.Fprintf(&b, "%d", secs)
			i += 3
		default:
			b.WriteByte(format[i])
			i++
		}
	}
	return b.String()
}

func formatCountdownBody(s CountdownSettings, seconds int64) string {
	negative := seconds < 0
	var abs uint64
	if negative {
		abs = uint64(-(seconds + 1)) + 1
	} else {
		abs = uint64(seconds)
	}
	sign := ""
	if negative {
		sign = "-"
	}
	days := abs / 86400
	totalHours := abs / 3600
	hourComponent := totalHours % 24
	totalMinutes := abs / 60
	minutes := totalMinutes % 60
	secs := abs % 60
	switch s.Format {
	case "hhmmss":
		return fmt.Sprintf("%s%02d:%02d:%02d", sign, totalHours, minutes, secs)
	case "mmss":
		return fmt.Sprintf("%s%02d:%02d", sign, totalMinutes, secs)
	case "seconds":
		return fmt.Sprintf("%s%d", sign, abs)
	case "custom":
		return formatCountdownCustom(s.CustomFormat, seconds)
	default:
		if abs >= 86400 {
			return fmt.Sprintf("%s%d:%02d:%02d:%02d", sign, days, hourComponent, minutes, secs)
		}
		if abs >= 3600 {
			return fmt.Sprintf("%s%d:%02d:%02d", sign, totalHours, minutes, secs)
		}
		return fmt.Sprintf("%s%02d:%02d", sign, totalMinutes, secs)
	}
}

func countdownDisplayText(s CountdownSettings, ms int64, finished bool) string {
	body := ""
	if finished {
		if s.BlankOnFinish {
			return ""
		}
		if s.FinishedText != "" {
			body = s.FinishedText
		} else {
			body = formatCountdownBody(s, 0)
		}
	} else {
		body = formatCountdownBody(s, countdownDisplaySeconds(s.Mode, ms))
	}
	return s.Prefix + body + s.Suffix
}

func (m *CountdownManager) state(fonts []FontInfo) CountdownState {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	current := m.settleLocked(now)
	return CountdownState{
		Settings:    m.settings,
		CurrentMS:   current,
		DurationMS:  countdownDurationMS(m.settings),
		Running:     m.running,
		Paused:      m.paused,
		Finished:    m.finished,
		HasStarted:  m.hasStarted,
		DisplayText: countdownDisplayText(m.settings, current, m.finished),
		ServerNowMS: now.UnixMilli(),
		UpdatedAtMS: m.updatedAtMS,
		OverlayURL:  countdownOverlayURL,
		Fonts:       fonts,
		Profiles:    m.listProfiles(),
	}
}

func (m *CountdownManager) listProfiles() []ProfileInfo {
	entries, err := os.ReadDir(m.profileDir)
	if err != nil {
		return []ProfileInfo{}
	}
	profiles := make([]ProfileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		profiles = append(profiles, ProfileInfo{
			Name:       strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
			ModifiedAt: info.ModTime().UnixMilli(),
		})
	}
	sort.Slice(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	return profiles
}

func (m *CountdownManager) saveProfile(name string) error {
	name = sanitizeProfileName(name)
	if name == "" {
		return errors.New("enter a profile name")
	}
	m.mu.Lock()
	next := m.settings
	m.mu.Unlock()
	normalizeCountdownSettings(&next)
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	return writeSettingsFile(filepath.Join(m.profileDir, name+".json"), data)
}

func (m *CountdownManager) loadProfile(name string) error {
	name = sanitizeProfileName(name)
	if name == "" {
		return errors.New("choose a profile")
	}
	data, err := os.ReadFile(filepath.Join(m.profileDir, name+".json"))
	if err != nil {
		return err
	}
	next := defaultCountdownSettings()
	if err := json.Unmarshal(data, &next); err != nil {
		return errors.New("profile is not valid JSON")
	}
	normalizeCountdownSettings(&next)
	if err := m.applySettings(next); err != nil {
		return err
	}
	return m.control("reset")
}

func (m *CountdownManager) deleteProfile(name string) error {
	name = sanitizeProfileName(name)
	if name == "" {
		return errors.New("choose a profile")
	}
	err := os.Remove(filepath.Join(m.profileDir, name+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (m *CountdownManager) control(action string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	switch action {
	case "start":
		m.startLocked(now)
	case "pause":
		m.pauseLocked(now)
	case "stop":
		m.stopLocked(now)
	case "toggle":
		if m.running {
			m.pauseLocked(now)
		} else {
			m.startLocked(now)
		}
	case "reset":
		m.resetLocked(now, false)
	case "add60":
		m.adjustLocked(now, 60000)
	case "sub60":
		m.adjustLocked(now, -60000)
	case "add10":
		m.adjustLocked(now, 10000)
	case "sub10":
		m.adjustLocked(now, -10000)
	case "overlay_loaded":
		if m.settings.StartBehavior == "overlay-load" {
			if m.settings.RestartOnLoad {
				m.resetLocked(now, false)
				m.startLocked(now)
			} else if !m.hasStarted && !m.running {
				m.startLocked(now)
			}
		}
	case "overlay_unloaded":
		if m.settings.ResetOnUnload {
			m.resetLocked(now, false)
		}
	default:
		return fmt.Errorf("unknown countdown action %q", action)
	}
	return nil
}

func (a *App) handleCountdownProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		writeJSON(w, a.countdown.state(a.listFonts()))
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := decodeSingleJSON(r.Body, &req, 32<<10); err != nil {
		http.Error(w, "invalid countdown profile request", http.StatusBadRequest)
		return
	}
	var err error
	switch strings.TrimSpace(req.Action) {
	case "save":
		err = a.countdown.saveProfile(req.Name)
	case "load":
		err = a.countdown.loadProfile(req.Name)
	case "delete":
		err = a.countdown.deleteProfile(req.Name)
	default:
		err = errors.New("unknown countdown profile action")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, a.countdown.state(a.listFonts()))
}

func (a *App) handleCountdownUploadFont(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFontBytes+(1<<20))
	if err := r.ParseMultipartForm(maxFontBytes + (1 << 20)); err != nil {
		http.Error(w, "font file is too large (20 MB max)", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("font")
	if err != nil {
		http.Error(w, "choose a font file first", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > maxFontBytes {
		http.Error(w, "font file is too large (20 MB max)", http.StatusBadRequest)
		return
	}
	ext, err := validateFont(file, header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "could not read font file", http.StatusBadRequest)
		return
	}
	base := sanitizeFontBase(header.Filename)
	name := base + ext
	target := filepath.Join(a.fontDir, name)
	tmpFile, err := os.CreateTemp(a.fontDir, ".countdown-font-upload-*.tmp")
	if err != nil {
		http.Error(w, "could not save font file", http.StatusInternalServerError)
		return
	}
	tmpName := tmpFile.Name()
	_, copyErr := io.Copy(tmpFile, file)
	syncErr := tmpFile.Sync()
	closeErr := tmpFile.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		http.Error(w, "could not save font file", http.StatusInternalServerError)
		return
	}
	if err := commitTempFile(tmpName, target, 8); err != nil {
		_ = os.Remove(tmpName)
		http.Error(w, "could not finalize font file", http.StatusInternalServerError)
		return
	}
	entries, _ := os.ReadDir(a.fontDir)
	for _, e := range entries {
		if e.IsDir() || e.Name() == name || strings.HasPrefix(e.Name(), ".countdown-font-upload-") {
			continue
		}
		if sanitizeFontBase(e.Name()) == base && fontAllowedExt(filepath.Ext(e.Name())) {
			_ = os.Remove(filepath.Join(a.fontDir, e.Name()))
		}
	}
	family := fontFamilyForFile(name)
	next := a.countdown.state(nil).Settings
	next.FontFamily = family
	if err := a.countdown.applySettings(next); err != nil {
		http.Error(w, "font saved, but countdown settings could not be updated", http.StatusInternalServerError)
		return
	}
	writeJSON(w, a.countdown.state(a.listFonts()))
}

func (a *App) handleCountdownState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, a.countdown.state(a.listFonts()))
}

func (a *App) handleCountdownSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, a.countdown.state(a.listFonts()))
	case http.MethodPost:
		next := a.countdown.state(nil).Settings
		if err := decodeSingleJSON(r.Body, &next, 1<<20); err != nil {
			http.Error(w, "invalid countdown settings", http.StatusBadRequest)
			return
		}
		if err := a.countdown.applySettings(next); err != nil {
			log.Printf("countdown settings save failed: %v", err)
			http.Error(w, "could not save countdown settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, a.countdown.state(a.listFonts()))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleCountdownControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := decodeSingleJSON(r.Body, &req, 16<<10); err != nil {
		http.Error(w, "invalid countdown control", http.StatusBadRequest)
		return
	}
	if err := a.countdown.control(strings.TrimSpace(req.Action)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, a.countdown.state(a.listFonts()))
}
