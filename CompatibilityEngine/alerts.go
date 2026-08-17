package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const alertOverlayURL = "http://127.0.0.1:17891/alerts"

var alertTypeOrder = []string{
	"follow",
	"subscription-new",
	"subscription-renewal",
	"subscription-gift",
	"kicks",
	"reward",
}

var kickAlertEvents = map[string]string{
	"channel.followed":                  "follow",
	"channel.subscription.new":          "subscription-new",
	"channel.subscription.renewal":      "subscription-renewal",
	"channel.subscription.gifts":        "subscription-gift",
	"kicks.gifted":                      "kicks",
	"channel.reward.redemption.updated": "reward",
}

type AlertStyle struct {
	Enabled              bool   `json:"enabled"`
	DisplayMode          string `json:"display_mode"`
	ShowTitle            bool   `json:"show_title"`
	ShowMessage          bool   `json:"show_message"`
	TitleTemplate        string `json:"title_template"`
	MessageTemplate      string `json:"message_template"`
	DurationMS           int    `json:"duration_ms"`
	EnterAnimation       string `json:"enter_animation"`
	ExitAnimation        string `json:"exit_animation"`
	EnterDurationMS      int    `json:"enter_duration_ms"`
	ExitDurationMS       int    `json:"exit_duration_ms"`
	Animation            string `json:"animation,omitempty"` // v1 migration compatibility
	Layout               string `json:"layout,omitempty"`    // v1 migration compatibility
	X                    int    `json:"x"`
	Y                    int    `json:"y"`
	Width                int    `json:"width"`
	Height               int    `json:"height"`
	SnapEnabled          bool   `json:"snap_enabled"`
	MediaX               int    `json:"media_x"`
	MediaY               int    `json:"media_y"`
	MediaWidth           int    `json:"media_width"`
	MediaHeight          int    `json:"media_height"`
	MediaFit             string `json:"media_fit"`
	MediaOpacity         int    `json:"media_opacity"`
	MediaRotation        int    `json:"media_rotation"`
	MediaFlipHorizontal  bool   `json:"media_flip_horizontal"`
	MediaFlipVertical    bool   `json:"media_flip_vertical"`
	MediaAboveText       bool   `json:"media_above_text"`
	TitleX               int    `json:"title_x"`
	TitleY               int    `json:"title_y"`
	TitleWidth           int    `json:"title_width"`
	TitleHeight          int    `json:"title_height"`
	TitleFontFamily      string `json:"title_font_family"`
	TitleSize            int    `json:"title_size"`
	TitleWeight          int    `json:"title_weight"`
	TitleColor           string `json:"title_color"`
	TitleAlign           string `json:"title_align"`
	TitleOutlineColor    string `json:"title_outline_color"`
	TitleOutlineWidth    int    `json:"title_outline_width"`
	TitleLetterSpacing   int    `json:"title_letter_spacing"`
	TitleLineHeight      int    `json:"title_line_height"`
	TitleShadow          bool   `json:"title_shadow"`
	MessageX             int    `json:"message_x"`
	MessageY             int    `json:"message_y"`
	MessageWidth         int    `json:"message_width"`
	MessageHeight        int    `json:"message_height"`
	MessageFontFamily    string `json:"message_font_family"`
	MessageSize          int    `json:"message_size"`
	MessageWeight        int    `json:"message_weight"`
	MessageColor         string `json:"message_color"`
	MessageAlign         string `json:"message_align"`
	MessageOutlineColor  string `json:"message_outline_color"`
	MessageOutlineWidth  int    `json:"message_outline_width"`
	MessageLetterSpacing int    `json:"message_letter_spacing"`
	MessageLineHeight    int    `json:"message_line_height"`
	MessageShadow        bool   `json:"message_shadow"`
	BackgroundColor      string `json:"background_color"`
	BackgroundOpacity    int    `json:"background_opacity"`
	AccentColor          string `json:"accent_color"`
	TextColor            string `json:"text_color,omitempty"` // v1 migration compatibility
	Radius               int    `json:"radius"`
	BorderWidth          int    `json:"border_width"`
	Shadow               bool   `json:"shadow"`
	VisualFile           string `json:"visual_file,omitempty"`
	VisualUpdatedAt      int64  `json:"visual_updated_at,omitempty"`
	SoundFile            string `json:"sound_file,omitempty"`
	SoundUpdatedAt       int64  `json:"sound_updated_at,omitempty"`
	SoundVolume          int    `json:"sound_volume"`
	SoundDelayMS         int    `json:"sound_delay_ms"`
}

type AlertSettings struct {
	SchemaVersion int                   `json:"schema_version"`
	CanvasWidth   int                   `json:"canvas_width"`
	CanvasHeight  int                   `json:"canvas_height"`
	QueueLimit    int                   `json:"queue_limit"`
	Types         map[string]AlertStyle `json:"types"`
}

type AlertEvent struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Source      string `json:"source"`
	Username    string `json:"username"`
	Amount      int    `json:"amount,omitempty"`
	Count       int    `json:"count,omitempty"`
	Months      int    `json:"months,omitempty"`
	Tier        string `json:"tier,omitempty"`
	GiftName    string `json:"gift_name,omitempty"`
	RewardTitle string `json:"reward_title,omitempty"`
	UserInput   string `json:"user_input,omitempty"`
	CreatedAtMS int64  `json:"created_at_ms"`
}

type AlertPresentation struct {
	AlertEvent
	Style       AlertStyle `json:"style"`
	Title       string     `json:"title"`
	Message     string     `json:"message"`
	StartedAtMS int64      `json:"started_at_ms"`
	EndsAtMS    int64      `json:"ends_at_ms"`
	VisualURL   string     `json:"visual_url,omitempty"`
	SoundURL    string     `json:"sound_url,omitempty"`
}

type AlertState struct {
	Settings    AlertSettings      `json:"settings"`
	Current     *AlertPresentation `json:"current,omitempty"`
	Queue       []AlertEvent       `json:"queue"`
	QueueDepth  int                `json:"queue_depth"`
	Dropped     int64              `json:"dropped"`
	LastEvent   *AlertEvent        `json:"last_event,omitempty"`
	OverlayURL  string             `json:"overlay_url"`
	UpdatedAtMS int64              `json:"updated_at_ms"`
}

type AlertManager struct {
	mu           sync.Mutex
	settings     AlertSettings
	settingsPath string
	mediaDir     string
	queue        []AlertEvent
	current      *AlertPresentation
	seen         map[string]int64
	dropped      int64
	updatedAtMS  int64
	sequence     uint64
	lastEvent    *AlertEvent
}

func defaultAlertStyle(alertType string) AlertStyle {
	style := AlertStyle{
		Enabled:              true,
		DisplayMode:          "card",
		ShowTitle:            true,
		ShowMessage:          true,
		DurationMS:           4200,
		EnterAnimation:       "pop",
		ExitAnimation:        "fade",
		EnterDurationMS:      420,
		ExitDurationMS:       320,
		Animation:            "pop",
		Layout:               "card",
		X:                    610,
		Y:                    365,
		Width:                700,
		Height:               350,
		SnapEnabled:          true,
		MediaX:               34,
		MediaY:               90,
		MediaWidth:           170,
		MediaHeight:          170,
		MediaFit:             "contain",
		MediaOpacity:         100,
		TitleX:               235,
		TitleY:               105,
		TitleWidth:           425,
		TitleHeight:          82,
		TitleFontFamily:      "Segoe UI",
		TitleSize:            42,
		TitleWeight:          900,
		TitleColor:           "#FFFFFF",
		TitleAlign:           "left",
		TitleOutlineColor:    "#000000",
		TitleOutlineWidth:    0,
		TitleLetterSpacing:   0,
		TitleLineHeight:      108,
		TitleShadow:          true,
		MessageX:             235,
		MessageY:             175,
		MessageWidth:         425,
		MessageHeight:        78,
		MessageFontFamily:    "Segoe UI",
		MessageSize:          24,
		MessageWeight:        650,
		MessageColor:         "#FFFFFF",
		MessageAlign:         "left",
		MessageOutlineColor:  "#000000",
		MessageOutlineWidth:  0,
		MessageLetterSpacing: 0,
		MessageLineHeight:    128,
		MessageShadow:        true,
		BackgroundColor:      "#07111F",
		BackgroundOpacity:    92,
		AccentColor:          "#3AA7FF",
		TextColor:            "#FFFFFF",
		Radius:               24,
		BorderWidth:          2,
		Shadow:               true,
		SoundVolume:          75,
		SoundDelayMS:         0,
	}
	switch alertType {
	case "follow":
		style.TitleTemplate = "{user} followed!"
		style.MessageTemplate = "Welcome to the stream."
	case "subscription-new":
		style.TitleTemplate = "Thanks for the sub, {user}!"
		style.MessageTemplate = "Welcome to the community!"
		style.AccentColor = "#7C8CFF"
	case "subscription-renewal":
		style.TitleTemplate = "{user} resubscribed!"
		style.MessageTemplate = "{months} month subscription"
		style.AccentColor = "#8D79FF"
	case "subscription-gift":
		style.TitleTemplate = "Gift subs!"
		style.MessageTemplate = "{user} gifted {count} subscription{plural}!"
		style.AccentColor = "#C77DFF"
	case "kicks":
		style.TitleTemplate = "{gift}"
		style.MessageTemplate = "{user} sent {amount} Kicks!"
		style.AccentColor = "#53E58C"
	case "reward":
		style.TitleTemplate = "{reward}"
		style.MessageTemplate = "Redeemed by {user}{input_suffix}"
		style.AccentColor = "#FFB85C"
	}
	return style
}

func defaultAlertSettings() AlertSettings {
	types := make(map[string]AlertStyle, len(alertTypeOrder))
	for _, alertType := range alertTypeOrder {
		types[alertType] = defaultAlertStyle(alertType)
	}
	return AlertSettings{
		SchemaVersion: 2,
		CanvasWidth:   1920,
		CanvasHeight:  1080,
		QueueLimit:    40,
		Types:         types,
	}
}

func isAlertType(value string) bool {
	value = strings.TrimSpace(value)
	for _, alertType := range alertTypeOrder {
		if value == alertType {
			return true
		}
	}
	return false
}

func normalizeAlertFont(value, fallback string) string {
	value = strings.TrimSpace(value)
	allowed := map[string]bool{
		"Segoe UI": true, "Arial": true, "Verdana": true, "Tahoma": true,
		"Trebuchet MS": true, "Georgia": true, "Times New Roman": true, "Impact": true,
	}
	if !allowed[value] {
		return fallback
	}
	return value
}

func normalizeAlertAlign(value, fallback string) string {
	switch strings.TrimSpace(value) {
	case "left", "center", "right":
		return strings.TrimSpace(value)
	default:
		return fallback
	}
}

func normalizeAlertAnimation(value, fallback string, allowPop bool) string {
	value = strings.TrimSpace(value)
	switch value {
	case "none", "fade", "slide-up", "slide-down", "slide-left", "slide-right", "zoom":
		return value
	case "pop":
		if allowPop {
			return value
		}
	}
	return fallback
}

func migrateAlertStyleV1(style AlertStyle, fallback AlertStyle) AlertStyle {
	style.DisplayMode = "card"
	style.ShowTitle = true
	style.ShowMessage = true
	if strings.TrimSpace(style.Animation) != "" {
		style.EnterAnimation = style.Animation
	} else {
		style.EnterAnimation = fallback.EnterAnimation
	}
	style.ExitAnimation = fallback.ExitAnimation
	style.EnterDurationMS = fallback.EnterDurationMS
	style.ExitDurationMS = fallback.ExitDurationMS
	style.SnapEnabled = true
	style.MediaFit = "contain"
	style.MediaOpacity = 100
	style.MediaX = 34
	style.MediaY = maxInt(18, (style.Height-style.MediaHeight)/2)
	style.TitleX = 235
	style.TitleY = 105
	style.TitleWidth = maxInt(160, style.Width-275)
	style.TitleHeight = fallback.TitleHeight
	style.TitleFontFamily = fallback.TitleFontFamily
	style.TitleWeight = fallback.TitleWeight
	style.TitleColor = normalizeHexColor(style.TextColor, fallback.TitleColor)
	style.TitleAlign = fallback.TitleAlign
	style.TitleOutlineColor = fallback.TitleOutlineColor
	style.TitleLineHeight = fallback.TitleLineHeight
	style.TitleShadow = style.Shadow
	style.MessageX = 235
	style.MessageY = 175
	style.MessageWidth = maxInt(160, style.Width-275)
	style.MessageHeight = fallback.MessageHeight
	style.MessageFontFamily = fallback.MessageFontFamily
	style.MessageWeight = fallback.MessageWeight
	style.MessageColor = normalizeHexColor(style.TextColor, fallback.MessageColor)
	style.MessageAlign = fallback.MessageAlign
	style.MessageOutlineColor = fallback.MessageOutlineColor
	style.MessageLineHeight = fallback.MessageLineHeight
	style.MessageShadow = style.Shadow
	return style
}

func normalizeAlertStyle(style AlertStyle, fallback AlertStyle, canvasWidth, canvasHeight int) AlertStyle {
	if strings.TrimSpace(style.TitleTemplate) == "" {
		style.TitleTemplate = fallback.TitleTemplate
	}
	if len(style.TitleTemplate) > 256 {
		style.TitleTemplate = style.TitleTemplate[:256]
	}
	if strings.TrimSpace(style.MessageTemplate) == "" {
		style.MessageTemplate = fallback.MessageTemplate
	}
	if len(style.MessageTemplate) > 512 {
		style.MessageTemplate = style.MessageTemplate[:512]
	}
	switch style.DisplayMode {
	case "card", "custom", "media-only", "text-only":
	default:
		style.DisplayMode = fallback.DisplayMode
	}
	style.DurationMS = clampInt(style.DurationMS, 500, 30000)
	style.EnterAnimation = normalizeAlertAnimation(style.EnterAnimation, fallback.EnterAnimation, true)
	style.ExitAnimation = normalizeAlertAnimation(style.ExitAnimation, fallback.ExitAnimation, false)
	style.EnterDurationMS = clampInt(style.EnterDurationMS, 0, 5000)
	style.ExitDurationMS = clampInt(style.ExitDurationMS, 0, 5000)
	style.Width = clampInt(style.Width, 120, canvasWidth)
	style.Height = clampInt(style.Height, 80, canvasHeight)
	style.X = clampInt(style.X, -canvasWidth, canvasWidth)
	style.Y = clampInt(style.Y, -canvasHeight, canvasHeight)
	style.MediaX = clampInt(style.MediaX, -style.Width, style.Width)
	style.MediaY = clampInt(style.MediaY, -style.Height, style.Height)
	style.MediaWidth = clampInt(style.MediaWidth, 20, canvasWidth)
	style.MediaHeight = clampInt(style.MediaHeight, 20, canvasHeight)
	switch style.MediaFit {
	case "contain", "cover", "fill", "none":
	default:
		style.MediaFit = fallback.MediaFit
	}
	style.MediaOpacity = clampInt(style.MediaOpacity, 0, 100)
	style.MediaRotation = clampInt(style.MediaRotation, -180, 180)
	style.TitleX = clampInt(style.TitleX, -style.Width, style.Width)
	style.TitleY = clampInt(style.TitleY, -style.Height, style.Height)
	style.TitleWidth = clampInt(style.TitleWidth, 40, canvasWidth)
	style.TitleHeight = clampInt(style.TitleHeight, 20, canvasHeight)
	style.TitleFontFamily = normalizeAlertFont(style.TitleFontFamily, fallback.TitleFontFamily)
	style.TitleSize = clampInt(style.TitleSize, 8, 200)
	style.TitleWeight = clampInt(style.TitleWeight, 100, 900)
	style.TitleColor = normalizeHexColor(style.TitleColor, fallback.TitleColor)
	style.TitleAlign = normalizeAlertAlign(style.TitleAlign, fallback.TitleAlign)
	style.TitleOutlineColor = normalizeHexColor(style.TitleOutlineColor, fallback.TitleOutlineColor)
	style.TitleOutlineWidth = clampInt(style.TitleOutlineWidth, 0, 12)
	style.TitleLetterSpacing = clampInt(style.TitleLetterSpacing, -10, 30)
	style.TitleLineHeight = clampInt(style.TitleLineHeight, 70, 220)
	style.MessageX = clampInt(style.MessageX, -style.Width, style.Width)
	style.MessageY = clampInt(style.MessageY, -style.Height, style.Height)
	style.MessageWidth = clampInt(style.MessageWidth, 40, canvasWidth)
	style.MessageHeight = clampInt(style.MessageHeight, 20, canvasHeight)
	style.MessageFontFamily = normalizeAlertFont(style.MessageFontFamily, fallback.MessageFontFamily)
	style.MessageSize = clampInt(style.MessageSize, 8, 160)
	style.MessageWeight = clampInt(style.MessageWeight, 100, 900)
	style.MessageColor = normalizeHexColor(style.MessageColor, fallback.MessageColor)
	style.MessageAlign = normalizeAlertAlign(style.MessageAlign, fallback.MessageAlign)
	style.MessageOutlineColor = normalizeHexColor(style.MessageOutlineColor, fallback.MessageOutlineColor)
	style.MessageOutlineWidth = clampInt(style.MessageOutlineWidth, 0, 12)
	style.MessageLetterSpacing = clampInt(style.MessageLetterSpacing, -10, 30)
	style.MessageLineHeight = clampInt(style.MessageLineHeight, 70, 220)
	style.BackgroundColor = normalizeHexColor(style.BackgroundColor, fallback.BackgroundColor)
	style.BackgroundOpacity = clampInt(style.BackgroundOpacity, 0, 100)
	style.AccentColor = normalizeHexColor(style.AccentColor, fallback.AccentColor)
	style.Radius = clampInt(style.Radius, 0, 120)
	style.BorderWidth = clampInt(style.BorderWidth, 0, 20)
	style.SoundVolume = clampInt(style.SoundVolume, 0, 100)
	style.SoundDelayMS = clampInt(style.SoundDelayMS, 0, 10000)
	style.VisualFile = filepath.Base(strings.TrimSpace(style.VisualFile))
	style.SoundFile = filepath.Base(strings.TrimSpace(style.SoundFile))
	style.Animation = style.EnterAnimation
	style.TextColor = style.TitleColor
	return style
}

func normalizeAlertSettings(settings AlertSettings) AlertSettings {
	defaults := defaultAlertSettings()
	oldVersion := settings.SchemaVersion
	settings.SchemaVersion = 2
	settings.CanvasWidth = clampInt(settings.CanvasWidth, 640, 3840)
	settings.CanvasHeight = clampInt(settings.CanvasHeight, 360, 2160)
	settings.QueueLimit = clampInt(settings.QueueLimit, 1, 100)
	if settings.Types == nil {
		settings.Types = map[string]AlertStyle{}
	}
	for _, alertType := range alertTypeOrder {
		fallback := defaults.Types[alertType]
		style, ok := settings.Types[alertType]
		if !ok {
			style = fallback
		} else if oldVersion < 2 {
			style = migrateAlertStyleV1(style, fallback)
		}
		style = normalizeAlertStyle(style, fallback, settings.CanvasWidth, settings.CanvasHeight)
		settings.Types[alertType] = style
	}
	for key := range settings.Types {
		if !isAlertType(key) {
			delete(settings.Types, key)
		}
	}
	return settings
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func newAlertManager(dataDir string) *AlertManager {
	m := &AlertManager{
		settings:     defaultAlertSettings(),
		settingsPath: filepath.Join(dataDir, "alert_settings.json"),
		mediaDir:     filepath.Join(dataDir, "Alerts"),
		seen:         make(map[string]int64),
		updatedAtMS:  time.Now().UnixMilli(),
	}
	_ = os.MkdirAll(m.mediaDir, 0755)
	m.load()
	return m
}

func (m *AlertManager) load() {
	data, err := os.ReadFile(m.settingsPath)
	if err != nil {
		return
	}
	var settings AlertSettings
	if json.Unmarshal(data, &settings) != nil {
		stamp := time.Now().UTC().Format("20060102-150405")
		_ = os.WriteFile(filepath.Join(filepath.Dir(m.settingsPath), "alert_settings.corrupt-"+stamp+".json"), data, 0600)
		return
	}
	m.settings = normalizeAlertSettings(settings)
}

func (m *AlertManager) saveLocked() error {
	data, err := json.MarshalIndent(m.settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.settingsPath), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.settingsPath), ".alert-settings-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := commitTempFile(tmpName, m.settingsPath, 8); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func (m *AlertManager) setSettings(settings AlertSettings) error {
	settings = normalizeAlertSettings(settings)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = settings
	m.updatedAtMS = time.Now().UnixMilli()
	return m.saveLocked()
}

func (m *AlertManager) settingsSnapshot() AlertSettings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneAlertSettings(m.settings)
}

func cloneAlertSettings(settings AlertSettings) AlertSettings {
	out := settings
	out.Types = make(map[string]AlertStyle, len(settings.Types))
	for key, style := range settings.Types {
		out.Types[key] = style
	}
	return out
}

func (m *AlertManager) cleanupSeenLocked(now int64) {
	cutoff := now - int64((24*time.Hour)/time.Millisecond)
	for key, seenAt := range m.seen {
		if seenAt < cutoff {
			delete(m.seen, key)
		}
	}
	if len(m.seen) <= 8192 {
		return
	}
	for key := range m.seen {
		delete(m.seen, key)
		if len(m.seen) <= 4096 {
			break
		}
	}
}

func (m *AlertManager) enqueue(event AlertEvent, dedupeKey string) bool {
	if !isAlertType(event.Type) {
		return false
	}
	event.Source = strings.TrimSpace(event.Source)
	if event.Source == "" {
		event.Source = "local"
	}
	event.Username = strings.TrimSpace(event.Username)
	if event.Username == "" {
		event.Username = "Anonymous"
	}
	if event.CreatedAtMS <= 0 {
		event.CreatedAtMS = time.Now().UnixMilli()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UnixMilli()
	m.cleanupSeenLocked(now)
	dedupeKey = strings.TrimSpace(dedupeKey)
	if dedupeKey != "" {
		if _, exists := m.seen[dedupeKey]; exists {
			return false
		}
		m.seen[dedupeKey] = now
	}
	style := m.settings.Types[event.Type]
	if !style.Enabled {
		return false
	}
	m.sequence++
	if strings.TrimSpace(event.ID) == "" {
		event.ID = fmt.Sprintf("alert-%d-%d", now, m.sequence)
	}
	if len(m.queue) >= m.settings.QueueLimit {
		m.dropped++
		m.updatedAtMS = now
		return false
	}
	m.queue = append(m.queue, event)
	copyEvent := event
	m.lastEvent = &copyEvent
	m.updatedAtMS = now
	return true
}

func (m *AlertManager) startNextLocked(now int64) {
	for len(m.queue) > 0 {
		event := m.queue[0]
		m.queue = m.queue[1:]
		style, ok := m.settings.Types[event.Type]
		if !ok || !style.Enabled {
			continue
		}
		presentation := AlertPresentation{
			AlertEvent:  event,
			Style:       style,
			Title:       renderAlertTemplate(style.TitleTemplate, event),
			Message:     renderAlertTemplate(style.MessageTemplate, event),
			StartedAtMS: now,
			EndsAtMS:    now + int64(style.EnterDurationMS+style.DurationMS+style.ExitDurationMS),
		}
		if style.VisualFile != "" {
			presentation.VisualURL = fmt.Sprintf("/media/alerts?type=%s&kind=visual&v=%d", event.Type, style.VisualUpdatedAt)
		}
		if style.SoundFile != "" {
			presentation.SoundURL = fmt.Sprintf("/media/alerts?type=%s&kind=sound&v=%d", event.Type, style.SoundUpdatedAt)
		}
		m.current = &presentation
		m.updatedAtMS = now
		return
	}
}

func (m *AlertManager) advanceLocked(now int64) {
	if m.current != nil && now >= m.current.EndsAtMS {
		m.current = nil
		m.updatedAtMS = now
	}
	if m.current == nil {
		m.startNextLocked(now)
	}
}

func (m *AlertManager) state(consume bool) AlertState {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UnixMilli()
	if consume {
		m.advanceLocked(now)
	}
	settings := cloneAlertSettings(m.settings)
	queue := append([]AlertEvent(nil), m.queue...)
	var current *AlertPresentation
	if m.current != nil {
		copyCurrent := *m.current
		current = &copyCurrent
	}
	var last *AlertEvent
	if m.lastEvent != nil {
		copyLast := *m.lastEvent
		last = &copyLast
	}
	return AlertState{
		Settings: settings, Current: current, Queue: queue, QueueDepth: len(queue),
		Dropped: m.dropped, LastEvent: last, OverlayURL: alertOverlayURL, UpdatedAtMS: m.updatedAtMS,
	}
}

func (m *AlertManager) control(action string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UnixMilli()
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "skip":
		m.current = nil
		m.updatedAtMS = now
	case "clear":
		m.current = nil
		m.queue = nil
		m.updatedAtMS = now
	default:
		return errors.New("unknown alert control action")
	}
	return nil
}

func renderAlertTemplate(template string, event AlertEvent) string {
	plural := "s"
	if event.Count == 1 {
		plural = ""
	}
	inputSuffix := ""
	if strings.TrimSpace(event.UserInput) != "" {
		inputSuffix = ": " + strings.TrimSpace(event.UserInput)
	}
	values := map[string]string{
		"{user}":         event.Username,
		"{count}":        fmt.Sprintf("%d", event.Count),
		"{plural}":       plural,
		"{amount}":       fmt.Sprintf("%d", event.Amount),
		"{months}":       fmt.Sprintf("%d", event.Months),
		"{tier}":         event.Tier,
		"{gift}":         event.GiftName,
		"{reward}":       event.RewardTitle,
		"{input}":        event.UserInput,
		"{input_suffix}": inputSuffix,
	}
	out := template
	for key, value := range values {
		out = strings.ReplaceAll(out, key, value)
	}
	return strings.TrimSpace(out)
}

func alertTypeDisplayName(alertType string) string {
	switch alertType {
	case "follow":
		return "Follow"
	case "subscription-new":
		return "New Subscription"
	case "subscription-renewal":
		return "Subscription Renewal"
	case "subscription-gift":
		return "Gifted Subscriptions"
	case "kicks":
		return "Kicks Gift"
	case "reward":
		return "Reward Redemption"
	default:
		return "Alert"
	}
}
