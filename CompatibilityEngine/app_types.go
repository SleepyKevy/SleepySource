package main

import (
	"context"
	"net/http"
	"sync"
)

const (
	appTitle              = "SleepySource"
	appVersion            = "1.3.2"
	displayVersion        = "1.0 Beta"
	listenAddr            = "127.0.0.1:17891"
	dashboardURL          = "http://127.0.0.1:17891/"
	overlayURL            = "http://127.0.0.1:17891/overlay"
	maxUploadBytes        = 50 << 20
	maxFontBytes          = 20 << 20
	maxProfileBundleBytes = 160 << 20
)

type Settings struct {
	SchemaVersion        int     `json:"schema_version"`
	Format               string  `json:"format"`
	Template             string  `json:"template"`
	CanvasWidth          int     `json:"canvas_width"`
	CanvasHeight         int     `json:"canvas_height"`
	TextX                int     `json:"text_x"`
	TextY                int     `json:"text_y"`
	TextWidth            int     `json:"text_width"`
	TextSize             int     `json:"text_size"`
	TextWeight           int     `json:"text_weight"`
	TextFont             string  `json:"text_font"`
	TextColor            string  `json:"text_color"`
	TextAlign            string  `json:"text_align"`
	TextLineHeight       float64 `json:"text_line_height"`
	TextShadow           bool    `json:"text_shadow"`
	ImageMode            string  `json:"image_mode"`
	ImageX               int     `json:"image_x"`
	ImageY               int     `json:"image_y"`
	ImageWidth           int     `json:"image_width"`
	ImageHeight          int     `json:"image_height"`
	ImageOpacity         int     `json:"image_opacity"`
	MediaFit             string  `json:"media_fit"`
	MediaPositionX       int     `json:"media_position_x"`
	MediaPositionY       int     `json:"media_position_y"`
	MediaZoom            int     `json:"media_zoom"`
	MediaRadius          int     `json:"media_radius"`
	MediaShadow          bool    `json:"media_shadow"`
	MediaBrightness      int     `json:"media_brightness"`
	MediaContrast        int     `json:"media_contrast"`
	MediaSaturation      int     `json:"media_saturation"`
	MediaBorderWidth     int     `json:"media_border_width"`
	MediaBorderColor     string  `json:"media_border_color"`
	MediaGlow            bool    `json:"media_glow"`
	MediaGlowColor       string  `json:"media_glow_color"`
	MediaGlowSize        int     `json:"media_glow_size"`
	ArtworkAnimation     string  `json:"artwork_animation"`
	ArtworkAnimationMS   int     `json:"artwork_animation_ms"`
	TextAnimation        string  `json:"text_animation"`
	TextEffect           string  `json:"text_effect"`
	OverlayAnimation     string  `json:"overlay_animation"`
	OverlayAnimationMS   int     `json:"overlay_animation_ms"`
	BackgroundMotion     string  `json:"background_motion"`
	CustomImage          string  `json:"custom_image"`
	ShowProgress         bool    `json:"show_progress"`
	ProgressMode         string  `json:"progress_mode"`
	ProgressX            int     `json:"progress_x"`
	ProgressY            int     `json:"progress_y"`
	ProgressWidth        int     `json:"progress_width"`
	ProgressHeight       int     `json:"progress_height"`
	ProgressColor        string  `json:"progress_color"`
	ProgressTrackColor   string  `json:"progress_track_color"`
	ProgressRadius       int     `json:"progress_radius"`
	ShowRemainingTime    bool    `json:"show_remaining_time"`
	ProgressTextColor    string  `json:"progress_text_color"`
	ProgressTextSize     int     `json:"progress_text_size"`
	TimeX                int     `json:"time_x"`
	TimeY                int     `json:"time_y"`
	TimeWidth            int     `json:"time_width"`
	TimeAlign            string  `json:"time_align"`
	BackgroundMode       string  `json:"background_mode"`
	BackgroundColor      string  `json:"background_color"`
	BackgroundOpacity    int     `json:"background_opacity"`
	BackgroundFit        string  `json:"background_fit"`
	BackgroundPositionX  int     `json:"background_position_x"`
	BackgroundPositionY  int     `json:"background_position_y"`
	BackgroundZoom       int     `json:"background_zoom"`
	BackgroundRadius     int     `json:"background_radius"`
	BackgroundBrightness int     `json:"background_brightness"`
	BackgroundContrast   int     `json:"background_contrast"`
	BackgroundSaturation int     `json:"background_saturation"`
	BackgroundBlur       int     `json:"background_blur"`
	CustomBackground     string  `json:"custom_background"`
	HideWhenPaused       bool    `json:"hide_when_paused"`
	ShowWhenIdle         bool    `json:"show_when_idle"`
	ProgressStyle        string  `json:"progress_style"`
	TransitionStyle      string  `json:"transition_style"`
	TransitionMS         int     `json:"transition_ms"`
	TransitionEasing     string  `json:"transition_easing"`
	SnapEnabled          bool    `json:"snap_enabled"`
	GridSize             int     `json:"grid_size"`
	OnboardingComplete   bool    `json:"onboarding_complete"`
	DesignerTheme        string  `json:"designer_theme"`
	StartupPage          string  `json:"startup_page"`
	LastModule           string  `json:"last_module"`
	DefaultProfile       string  `json:"default_profile"`
	MediaSourceMode      string  `json:"media_source_mode"`
	MediaSourceInclude   string  `json:"media_source_include"`
	MediaSourceExclude   string  `json:"media_source_exclude"`
}

type Track struct {
	Found       bool   `json:"found"`
	Artist      string `json:"artist"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Source      string `json:"source"`
	PositionMS  int64  `json:"position_ms"`
	DurationMS  int64  `json:"duration_ms"`
	ArtStatus   string `json:"art_status"`
	SampledAtMS int64  `json:"sampled_at_ms"`
}

type FontInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Family string `json:"family"`
	URL    string `json:"url"`
}

type ProfileInfo struct {
	Name       string `json:"name"`
	ModifiedAt int64  `json:"modified_at"`
	Default    bool   `json:"default,omitempty"`
}

type ProfileBundleManifest struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	Name    string `json:"name"`
}

type MediaDiagnostics struct {
	Found          bool   `json:"found"`
	Source         string `json:"source"`
	Status         string `json:"status"`
	HasTimeline    bool   `json:"has_timeline"`
	PositionMS     int64  `json:"position_ms"`
	DurationMS     int64  `json:"duration_ms"`
	SampleAgeMS    int64  `json:"sample_age_ms"`
	Detector       string `json:"detector"`
	DataDirectory  string `json:"data_directory"`
	OverlayAddress string `json:"overlay_address"`
}

type AppState struct {
	Version          string           `json:"version"`
	Track            Track            `json:"track"`
	DisplayText      string           `json:"display_text"`
	Settings         Settings         `json:"settings"`
	ImageURL         string           `json:"image_url"`
	ImageName        string           `json:"image_name"`
	MediaKind        string           `json:"media_kind"`
	BackgroundURL    string           `json:"background_url"`
	BackgroundName   string           `json:"background_name"`
	BackgroundKind   string           `json:"background_kind"`
	Visible          bool             `json:"visible"`
	UpdatedAt        int64            `json:"updated_at"`
	Detector         string           `json:"detector"`
	OverlayFPS       float64          `json:"overlay_fps"`
	OverlayFrameMS   float64          `json:"overlay_frame_ms"`
	OverlayMetricsAt int64            `json:"overlay_metrics_at"`
	Fonts            []FontInfo       `json:"fonts"`
	Profiles         []ProfileInfo    `json:"profiles"`
	Diagnostics      MediaDiagnostics `json:"diagnostics"`
}

type App struct {
	mu               sync.RWMutex
	settings         Settings
	track            Track
	updatedAt        int64
	detectorStatus   string
	exeDir           string
	dataDir          string
	settingsPath     string
	outputPath       string
	customDir        string
	fontDir          string
	profileDir       string
	customPath       string
	backgroundPath   string
	server           *http.Server
	detectorCancel   context.CancelFunc
	overlayFPS       float64
	overlayFrameMS   float64
	overlayMetricsAt int64
	outputMu         sync.Mutex
	lastOutput       string
	wg               sync.WaitGroup
	chat             *ChatManager
	streamAuth       *KickUserAuthManager
	cloudflare       *CloudflareTunnelManager
	countdown        *CountdownManager
	alerts           *AlertManager
}
