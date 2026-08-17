package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ChatSettings struct {
	SchemaVersion                int    `json:"schema_version"`
	CanvasWidth                  int    `json:"canvas_width"`
	CanvasHeight                 int    `json:"canvas_height"`
	BoxX                         int    `json:"box_x"`
	BoxY                         int    `json:"box_y"`
	BoxWidth                     int    `json:"box_width"`
	BoxHeight                    int    `json:"box_height"`
	FontFamily                   string `json:"font_family"`
	FontSize                     int    `json:"font_size"`
	UsernameSize                 int    `json:"username_size"`
	MessageColor                 string `json:"message_color"`
	UsernameColor                string `json:"username_color"`
	BackgroundColor              string `json:"background_color"`
	BackgroundOpacity            int    `json:"background_opacity"`
	BoxBackgroundTransparent     bool   `json:"box_background_transparent"`
	BorderColor                  string `json:"border_color"`
	BorderWidth                  int    `json:"border_width"`
	Radius                       int    `json:"radius"`
	Padding                      int    `json:"padding"`
	MessageGap                   int    `json:"message_gap"`
	EmoteSize                    int    `json:"emote_size"`
	BadgeSize                    int    `json:"badge_size"`
	MaxMessages                  int    `json:"max_messages"`
	CompactMode                  bool   `json:"compact_mode"`
	MessageBackgroundColor       string `json:"message_background_color"`
	MessageBackgroundOpacity     int    `json:"message_background_opacity"`
	MessageBackgroundTransparent bool   `json:"message_background_transparent"`
	TextShadow                   bool   `json:"text_shadow"`
	UseKickUsernameColor         bool   `json:"use_kick_username_color"`
	ShowBadges                   bool   `json:"show_badges"`
	ShowTimestamps               bool   `json:"show_timestamps"`
	ShowAvatars                  bool   `json:"show_avatars"`
	HideCommands                 bool   `json:"hide_commands"`
	Direction                    string `json:"direction"`
	Animation                    string `json:"animation"`
	AnimationMS                  int    `json:"animation_ms"`
	Theme                        string `json:"theme"`
	AnimationEasing              string `json:"animation_easing"`
	MessageBorderWidth           int    `json:"message_border_width"`
	MessageBorderColor           string `json:"message_border_color"`
	MessageRadius                int    `json:"message_radius"`
	BoxBlur                      int    `json:"box_blur"`
	UsernameWeight               int    `json:"username_weight"`
	SevenTVEnabled               bool   `json:"seventv_enabled"`
	RememberKickLogin            bool   `json:"remember_kick_login"`
	KickChannel                  string `json:"kick_channel"`
	SevenTVEmoteSetID            string `json:"seventv_emote_set_id"`
}

type ChatBadge struct {
	Text  string `json:"text,omitempty"`
	Type  string `json:"type,omitempty"`
	Count int    `json:"count,omitempty"`
}

type ChatMessage struct {
	ID           string      `json:"id"`
	UserID       string      `json:"user_id"`
	Username     string      `json:"username"`
	Color        string      `json:"color"`
	Text         string      `json:"text"`
	Badges       []string    `json:"badges"`
	BadgeDetails []ChatBadge `json:"badge_details,omitempty"`
	AvatarURL    string      `json:"avatar_url"`
	CreatedAt    int64       `json:"created_at"`
	IsMod        bool        `json:"is_mod"`
}

type ChatState struct {
	Settings              ChatSettings  `json:"settings"`
	Messages              []ChatMessage `json:"messages"`
	UpdatedAt             int64         `json:"updated_at"`
	AuthReady             bool          `json:"auth_ready"`
	AuthMode              string        `json:"auth_mode,omitempty"`
	ConnectedChannel      string        `json:"connected_channel,omitempty"`
	BroadcasterUserID     string        `json:"broadcaster_user_id,omitempty"`
	TokenExpiresAt        int64         `json:"token_expires_at,omitempty"`
	LiveChatConnected     bool          `json:"live_chat_connected"`
	LiveChatStatus        string        `json:"live_chat_status,omitempty"`
	ChatroomID            string        `json:"chatroom_id,omitempty"`
	WebhookSubscribed     bool          `json:"webhook_subscribed"`
	WebhookSubscriptionID string        `json:"webhook_subscription_id,omitempty"`
	WebhookLastEventAt    int64         `json:"webhook_last_event_at,omitempty"`
	WebhookRequestCount   int64         `json:"webhook_request_count"`
	WebhookVerifiedCount  int64         `json:"webhook_verified_count"`
	WebhookAcceptedCount  int64         `json:"webhook_accepted_count"`
	WebhookRejectedCount  int64         `json:"webhook_rejected_count"`
	WebhookLastRequestAt  int64         `json:"webhook_last_request_at,omitempty"`
	WebhookLastEventType  string        `json:"webhook_last_event_type,omitempty"`
	WebhookLastError      string        `json:"webhook_last_error,omitempty"`
	SavedClientID         string        `json:"saved_client_id,omitempty"`
	CredentialsSaved      bool          `json:"credentials_saved"`
	CredentialStorage     string        `json:"credential_storage,omitempty"`
}

type ChatManager struct {
	mu                    sync.RWMutex
	authMu                sync.Mutex
	liveMu                sync.Mutex
	settings              ChatSettings
	messages              []ChatMessage
	settingsPath          string
	credentialsPath       string
	clientID              string
	clientSecret          string
	accessToken           string
	accessTokenExpiresAt  time.Time
	authMode              string
	connectedChannel      string
	broadcasterUserID     string
	chatroomID            string
	liveChatConnected     bool
	liveChatStatus        string
	webhookSubscribed     bool
	webhookSubscriptionID string
	webhookLastEventAt    int64
	webhookRequestCount   int64
	webhookVerifiedCount  int64
	webhookAcceptedCount  int64
	webhookRejectedCount  int64
	webhookLastRequestAt  int64
	webhookLastEventType  string
	webhookLastError      string
	webhookSeenMessageIDs map[string]int64
	updatedAt             int64
}

func defaultChatSettings() ChatSettings {
	return ChatSettings{
		SchemaVersion:                6,
		CanvasWidth:                  900,
		CanvasHeight:                 600,
		BoxX:                         30,
		BoxY:                         30,
		BoxWidth:                     520,
		BoxHeight:                    520,
		FontFamily:                   "Segoe UI",
		FontSize:                     24,
		UsernameSize:                 22,
		MessageColor:                 "#FFFFFF",
		UsernameColor:                "#55B7FF",
		BackgroundColor:              "#07111F",
		BackgroundOpacity:            72,
		BoxBackgroundTransparent:     false,
		BorderColor:                  "#2F78B7",
		BorderWidth:                  1,
		Radius:                       14,
		Padding:                      16,
		MessageGap:                   10,
		EmoteSize:                    32,
		BadgeSize:                    20,
		MaxMessages:                  12,
		CompactMode:                  true,
		MessageBackgroundColor:       "#07111F",
		MessageBackgroundOpacity:     22,
		MessageBackgroundTransparent: false,
		TextShadow:                   true,
		UseKickUsernameColor:         true,
		ShowBadges:                   true,
		ShowTimestamps:               false,
		ShowAvatars:                  false,
		HideCommands:                 false,
		Direction:                    "bottom-up",
		Animation:                    "slide-up",
		AnimationMS:                  240,
		Theme:                        "midnight",
		AnimationEasing:              "smooth",
		MessageBorderWidth:           0,
		MessageBorderColor:           "#2F78B7",
		MessageRadius:                9,
		BoxBlur:                      2,
		UsernameWeight:               800,
		SevenTVEnabled:               true,
		RememberKickLogin:            true,
	}
}

func newChatManager(dataDir string) *ChatManager {
	m := &ChatManager{
		settings:              defaultChatSettings(),
		settingsPath:          filepath.Join(dataDir, "chat_settings.json"),
		credentialsPath:       filepath.Join(dataDir, "kick_credentials.json"),
		webhookSeenMessageIDs: make(map[string]int64),
		updatedAt:             time.Now().UnixMilli(),
	}
	m.load()
	return m
}

func normalizeChatSettings(s *ChatSettings) {
	if s.SchemaVersion < 6 {
		s.SchemaVersion = 6
	}
	s.CanvasWidth = clampIntValue(s.CanvasWidth, 320, 3840)
	s.CanvasHeight = clampIntValue(s.CanvasHeight, 240, 2160)
	s.BoxWidth = clampIntValue(s.BoxWidth, 180, 3840)
	s.BoxHeight = clampIntValue(s.BoxHeight, 120, 2160)
	s.BoxX = clampIntValue(s.BoxX, -3840, 3840)
	s.BoxY = clampIntValue(s.BoxY, -2160, 2160)
	s.FontFamily = strings.TrimSpace(s.FontFamily)
	if s.FontFamily == "" {
		s.FontFamily = "Segoe UI"
	}
	s.FontSize = clampIntValue(s.FontSize, 10, 96)
	s.UsernameSize = clampIntValue(s.UsernameSize, 10, 96)
	s.MessageColor = normalizeColor(s.MessageColor, "#FFFFFF")
	s.UsernameColor = normalizeColor(s.UsernameColor, "#55B7FF")
	s.BackgroundColor = normalizeColor(s.BackgroundColor, "#07111F")
	s.BorderColor = normalizeColor(s.BorderColor, "#2F78B7")
	s.BackgroundOpacity = clampIntValue(s.BackgroundOpacity, 0, 100)
	s.BorderWidth = clampIntValue(s.BorderWidth, 0, 12)
	s.Radius = clampIntValue(s.Radius, 0, 100)
	s.Padding = clampIntValue(s.Padding, 0, 80)
	s.MessageGap = clampIntValue(s.MessageGap, 0, 60)
	s.EmoteSize = clampIntValue(s.EmoteSize, 16, 96)
	s.BadgeSize = clampIntValue(s.BadgeSize, 12, 64)
	s.MaxMessages = clampIntValue(s.MaxMessages, 1, 50)
	s.MessageBackgroundColor = normalizeColor(s.MessageBackgroundColor, "#07111F")
	s.MessageBackgroundOpacity = clampIntValue(s.MessageBackgroundOpacity, 0, 100)
	s.AnimationMS = clampIntValue(s.AnimationMS, 0, 3000)
	s.MessageBorderWidth = clampIntValue(s.MessageBorderWidth, 0, 12)
	s.MessageBorderColor = normalizeColor(s.MessageBorderColor, "#2F78B7")
	s.MessageRadius = clampIntValue(s.MessageRadius, 0, 80)
	s.BoxBlur = clampIntValue(s.BoxBlur, 0, 40)
	s.UsernameWeight = clampIntValue(s.UsernameWeight, 100, 900)
	switch s.Theme {
	case "midnight", "glass", "neon", "minimal", "bubblegum", "custom":
	default:
		s.Theme = "midnight"
	}
	switch s.AnimationEasing {
	case "linear", "ease", "ease-in", "ease-out", "ease-in-out", "snappy", "spring":
	default:
		s.AnimationEasing = "smooth"
	}
	if s.Direction != "top-down" {
		s.Direction = "bottom-up"
	}
	switch s.Animation {
	case "fade", "slide-left", "slide-right", "slide-up", "pop", "zoom", "bounce", "blur", "flip", "none":
	default:
		s.Animation = "slide-up"
	}
	s.KickChannel = normalizeKickChannelSlug(s.KickChannel)
	s.SevenTVEmoteSetID = strings.TrimSpace(s.SevenTVEmoteSetID)
}

func (m *ChatManager) load() {
	defer m.loadSavedCredentials()
	data, err := os.ReadFile(m.settingsPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("could not read chat settings: %v", err)
		}
		if saveErr := m.saveLocked(); saveErr != nil {
			log.Printf("could not save default chat settings: %v", saveErr)
		}
		return
	}
	s := defaultChatSettings()
	if err := json.Unmarshal(data, &s); err != nil {
		backup := filepath.Join(filepath.Dir(m.settingsPath), "chat_settings.corrupt-"+time.Now().Format("20060102-150405")+".json")
		if writeErr := os.WriteFile(backup, data, 0644); writeErr != nil {
			log.Printf("could not back up corrupt chat settings: %v", writeErr)
		}
		m.settings = defaultChatSettings()
		if saveErr := m.saveLocked(); saveErr != nil {
			log.Printf("could not save recovered chat settings: %v", saveErr)
		}
		return
	}
	normalizeChatSettings(&s)
	m.settings = s
}

type storedKickCredentials struct {
	Version         int    `json:"version"`
	ClientID        string `json:"client_id"`
	ProtectedSecret string `json:"protected_secret"`
}

func (m *ChatManager) loadSavedCredentials() {
	if !m.settings.RememberKickLogin || !secureCredentialStorageAvailable() {
		return
	}
	data, err := os.ReadFile(m.credentialsPath)
	if err != nil {
		return
	}
	var stored storedKickCredentials
	if json.Unmarshal(data, &stored) != nil || strings.TrimSpace(stored.ClientID) == "" || strings.TrimSpace(stored.ProtectedSecret) == "" {
		return
	}
	ciphertext, err := base64.StdEncoding.DecodeString(stored.ProtectedSecret)
	if err != nil {
		return
	}
	plain, err := unprotectCredential(ciphertext)
	if err != nil || strings.TrimSpace(string(plain)) == "" {
		return
	}
	m.mu.Lock()
	m.clientID = strings.TrimSpace(stored.ClientID)
	m.clientSecret = strings.TrimSpace(string(plain))
	m.authMode = "app"
	m.updatedAt = time.Now().UnixMilli()
	m.mu.Unlock()
}

func (m *ChatManager) saveCredentialsSnapshot(clientID, clientSecret string, remember bool) error {
	if !remember || strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
		_ = os.Remove(m.credentialsPath)
		return nil
	}
	if !secureCredentialStorageAvailable() {
		return nil
	}
	protected, err := protectCredential([]byte(strings.TrimSpace(clientSecret)))
	if err != nil {
		return err
	}
	stored := storedKickCredentials{Version: 1, ClientID: strings.TrimSpace(clientID), ProtectedSecret: base64.StdEncoding.EncodeToString(protected)}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(m.credentialsPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".kick-credentials-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_ = tmp.Chmod(0600)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	_ = tmp.Sync()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := commitTempFile(tmpName, m.credentialsPath, 8); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func (m *ChatManager) deleteSavedCredentials() { _ = os.Remove(m.credentialsPath) }

func (m *ChatManager) saveLocked() error {
	normalizeChatSettings(&m.settings)
	data, err := json.MarshalIndent(m.settings, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(m.settingsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".chat-settings-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	_ = tmp.Sync()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := commitTempFile(tmpName, m.settingsPath, 8); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func (m *ChatManager) state() ChatState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msgs := append([]ChatMessage(nil), m.messages...)
	validToken := strings.TrimSpace(m.accessToken) != "" && (m.accessTokenExpiresAt.IsZero() || time.Now().Before(m.accessTokenExpiresAt))
	ready := validToken || (m.authMode == "app" && strings.TrimSpace(m.clientID) != "" && strings.TrimSpace(m.clientSecret) != "")
	expiresAt := int64(0)
	if !m.accessTokenExpiresAt.IsZero() {
		expiresAt = m.accessTokenExpiresAt.Unix()
	}
	return ChatState{
		Settings: m.settings, Messages: msgs, UpdatedAt: m.updatedAt, AuthReady: ready,
		AuthMode: m.authMode, ConnectedChannel: m.connectedChannel, BroadcasterUserID: m.broadcasterUserID, TokenExpiresAt: expiresAt,
		LiveChatConnected: m.liveChatConnected, LiveChatStatus: m.liveChatStatus, ChatroomID: m.chatroomID,
		WebhookSubscribed: m.webhookSubscribed, WebhookSubscriptionID: m.webhookSubscriptionID, WebhookLastEventAt: m.webhookLastEventAt,
		WebhookRequestCount: m.webhookRequestCount, WebhookVerifiedCount: m.webhookVerifiedCount, WebhookAcceptedCount: m.webhookAcceptedCount, WebhookRejectedCount: m.webhookRejectedCount,
		WebhookLastRequestAt: m.webhookLastRequestAt, WebhookLastEventType: m.webhookLastEventType, WebhookLastError: m.webhookLastError,
		SavedClientID: m.clientID, CredentialsSaved: m.settings.RememberKickLogin && strings.TrimSpace(m.clientID) != "" && strings.TrimSpace(m.clientSecret) != "" && secureCredentialStorageAvailable(),
		CredentialStorage: func() string {
			if secureCredentialStorageAvailable() {
				return "Windows encrypted storage"
			}
			return "memory only"
		}(),
	}
}

func (m *ChatManager) setSettings(s ChatSettings) error {
	normalizeChatSettings(&s)
	m.mu.Lock()
	m.settings = s
	m.updatedAt = time.Now().UnixMilli()
	clientID, clientSecret := m.clientID, m.clientSecret
	err := m.saveLocked()
	m.mu.Unlock()
	if err != nil {
		return err
	}
	return m.saveCredentialsSnapshot(clientID, clientSecret, s.RememberKickLogin)
}

func (m *ChatManager) addMessage(msg ChatMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(msg.ID) == "" {
		msg.ID = fmt.Sprintf("local-%d", time.Now().UnixNano())
	}
	msg.ID = strings.TrimSpace(msg.ID)
	if len(msg.ID) > 128 {
		msg.ID = msg.ID[:128]
	}
	msg.UserID = strings.TrimSpace(msg.UserID)
	if len(msg.UserID) > 64 {
		msg.UserID = msg.UserID[:64]
	}
	msg.Username = strings.TrimSpace(msg.Username)
	if msg.Username == "" {
		msg.Username = "Viewer"
	}
	if len(msg.Username) > 80 {
		msg.Username = msg.Username[:80]
	}
	msg.Text = strings.TrimSpace(msg.Text)
	if len(msg.Text) > 4000 {
		msg.Text = msg.Text[:4000]
	}
	msg.AvatarURL = strings.TrimSpace(msg.AvatarURL)
	if len(msg.AvatarURL) > 2048 {
		msg.AvatarURL = ""
	}
	if len(msg.Badges) > 12 {
		msg.Badges = msg.Badges[:12]
	}
	for i := range msg.Badges {
		msg.Badges[i] = strings.TrimSpace(msg.Badges[i])
		if len(msg.Badges[i]) > 48 {
			msg.Badges[i] = msg.Badges[i][:48]
		}
	}
	if len(msg.BadgeDetails) > 12 {
		msg.BadgeDetails = msg.BadgeDetails[:12]
	}
	for i := range msg.BadgeDetails {
		msg.BadgeDetails[i].Text = strings.TrimSpace(msg.BadgeDetails[i].Text)
		msg.BadgeDetails[i].Type = normalizeKickBadgeType(msg.BadgeDetails[i].Type)
		if len(msg.BadgeDetails[i].Text) > 64 {
			msg.BadgeDetails[i].Text = msg.BadgeDetails[i].Text[:64]
		}
		if msg.BadgeDetails[i].Count < 0 {
			msg.BadgeDetails[i].Count = 0
		}
	}
	if msg.CreatedAt <= 0 {
		msg.CreatedAt = time.Now().UnixMilli()
	}
	msg.Color = normalizeColor(msg.Color, "#55B7FF")
	if m.settings.HideCommands && strings.HasPrefix(msg.Text, "!") {
		return
	}
	// Kick history polling, webhooks, and reconnects can occasionally surface the
	// same message more than once. Message IDs are stable, so suppress duplicates
	// before they can flash twice in OBS.
	for i := len(m.messages) - 1; i >= 0 && i >= len(m.messages)-100; i-- {
		if m.messages[i].ID == msg.ID {
			return
		}
	}
	m.messages = append(m.messages, msg)
	limit := m.settings.MaxMessages
	if limit < 1 {
		limit = 12
	}
	// Keep a little extra history for smooth overlay transitions and editor inspection.
	keep := limit * 3
	if keep < 30 {
		keep = 30
	}
	if len(m.messages) > keep {
		m.messages = append([]ChatMessage(nil), m.messages[len(m.messages)-keep:]...)
	}
	m.updatedAt = time.Now().UnixMilli()
}

func (m *ChatManager) clearMessages() {
	m.mu.Lock()
	m.messages = nil
	m.updatedAt = time.Now().UnixMilli()
	m.mu.Unlock()
}

func (m *ChatManager) reloadFromDisk() {
	m.stopLiveChat()
	settings := defaultChatSettings()
	if data, err := os.ReadFile(m.settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			backup := filepath.Join(filepath.Dir(m.settingsPath), "chat_settings.corrupt-"+time.Now().Format("20060102-150405")+".json")
			if writeErr := os.WriteFile(backup, data, 0644); writeErr != nil {
				log.Printf("could not back up corrupt restored chat settings: %v", writeErr)
			}
			settings = defaultChatSettings()
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("could not read restored chat settings: %v", err)
	}
	normalizeChatSettings(&settings)
	m.mu.Lock()
	m.settings = settings
	m.messages = nil
	m.clientID = ""
	m.clientSecret = ""
	m.accessToken = ""
	m.accessTokenExpiresAt = time.Time{}
	m.authMode = ""
	m.connectedChannel = ""
	m.broadcasterUserID = ""
	m.chatroomID = ""
	m.liveChatConnected = false
	m.liveChatStatus = ""
	m.webhookSubscribed = false
	m.webhookSubscriptionID = ""
	m.webhookLastEventAt = 0
	m.webhookRequestCount = 0
	m.webhookVerifiedCount = 0
	m.webhookAcceptedCount = 0
	m.webhookRejectedCount = 0
	m.webhookLastRequestAt = 0
	m.webhookLastEventType = ""
	m.webhookLastError = ""
	m.webhookSeenMessageIDs = make(map[string]int64)
	m.updatedAt = time.Now().UnixMilli()
	if err := m.saveLocked(); err != nil {
		log.Printf("could not save reloaded chat settings: %v", err)
	}
	m.mu.Unlock()
	m.loadSavedCredentials()
}

func (m *ChatManager) setAccessToken(token string) {
	m.stopLiveChat()
	m.mu.Lock()
	m.clientID = ""
	m.clientSecret = ""
	m.accessToken = strings.TrimSpace(token)
	m.accessTokenExpiresAt = time.Time{}
	m.webhookSubscribed = false
	m.webhookSubscriptionID = ""
	m.webhookLastEventAt = 0
	if m.accessToken != "" {
		m.authMode = "legacy-user-token"
	} else {
		m.authMode = ""
		m.connectedChannel = ""
		m.broadcasterUserID = ""
		m.chatroomID = ""
	}
	m.updatedAt = time.Now().UnixMilli()
	m.mu.Unlock()
}

func (m *ChatManager) setAppCredentials(clientID, clientSecret string) error {
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	m.mu.RLock()
	remember := m.settings.RememberKickLogin
	m.mu.RUnlock()
	if err := m.saveCredentialsSnapshot(clientID, clientSecret, remember); err != nil {
		return err
	}
	m.stopLiveChat()
	m.mu.Lock()
	m.clientID = clientID
	m.clientSecret = clientSecret
	m.accessToken = ""
	m.accessTokenExpiresAt = time.Time{}
	m.authMode = "app"
	m.connectedChannel = ""
	m.broadcasterUserID = ""
	m.chatroomID = ""
	m.webhookSubscribed = false
	m.webhookSubscriptionID = ""
	m.webhookLastEventAt = 0
	m.updatedAt = time.Now().UnixMilli()
	m.mu.Unlock()
	return nil
}

func (m *ChatManager) setKickAppAccessToken(token string, expiresAt time.Time) {
	m.mu.Lock()
	m.accessToken = strings.TrimSpace(token)
	m.accessTokenExpiresAt = expiresAt
	m.authMode = "app"
	m.updatedAt = time.Now().UnixMilli()
	m.mu.Unlock()
}

func (m *ChatManager) disconnectAuth() {
	m.stopLiveChat()
	m.mu.Lock()
	m.accessToken = ""
	m.accessTokenExpiresAt = time.Time{}
	m.connectedChannel = ""
	m.broadcasterUserID = ""
	m.chatroomID = ""
	m.liveChatConnected = false
	m.liveChatStatus = ""
	m.webhookSubscribed = false
	m.webhookSubscriptionID = ""
	m.webhookLastEventAt = 0
	if strings.TrimSpace(m.clientID) != "" && strings.TrimSpace(m.clientSecret) != "" {
		m.authMode = "app"
	} else {
		m.authMode = ""
	}
	m.updatedAt = time.Now().UnixMilli()
	m.mu.Unlock()
}

func (m *ChatManager) clearAuth() {
	m.stopLiveChat()
	m.deleteSavedCredentials()
	m.mu.Lock()
	m.clientID = ""
	m.clientSecret = ""
	m.accessToken = ""
	m.accessTokenExpiresAt = time.Time{}
	m.authMode = ""
	m.connectedChannel = ""
	m.broadcasterUserID = ""
	m.chatroomID = ""
	m.liveChatConnected = false
	m.liveChatStatus = ""
	m.webhookSubscribed = false
	m.webhookSubscriptionID = ""
	m.webhookLastEventAt = 0
	m.webhookRequestCount = 0
	m.webhookVerifiedCount = 0
	m.webhookAcceptedCount = 0
	m.webhookRejectedCount = 0
	m.webhookLastRequestAt = 0
	m.webhookLastEventType = ""
	m.webhookLastError = ""
	m.webhookSeenMessageIDs = make(map[string]int64)
	m.updatedAt = time.Now().UnixMilli()
	m.mu.Unlock()
}

func (m *ChatManager) hasAppCredentials() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return strings.TrimSpace(m.clientID) != "" && strings.TrimSpace(m.clientSecret) != ""
}

func (m *ChatManager) setResolvedChannel(channel, broadcasterUserID string) {
	m.mu.Lock()
	m.connectedChannel = normalizeKickChannelSlug(channel)
	m.broadcasterUserID = strings.TrimSpace(broadcasterUserID)
	m.updatedAt = time.Now().UnixMilli()
	m.mu.Unlock()
}

func (m *ChatManager) setWebhookSubscription(subscriptionID, status string) {
	m.mu.Lock()
	m.chatroomID = ""
	m.webhookSubscribed = true
	m.webhookSubscriptionID = strings.TrimSpace(subscriptionID)
	m.liveChatConnected = false
	m.liveChatStatus = strings.TrimSpace(status)
	m.updatedAt = time.Now().UnixMilli()
	m.mu.Unlock()
}

func (m *ChatManager) markWebhookRequest(eventType string) {
	now := time.Now().UnixMilli()
	m.mu.Lock()
	m.webhookRequestCount++
	m.webhookLastRequestAt = now
	m.webhookLastEventType = strings.TrimSpace(eventType)
	m.updatedAt = now
	m.mu.Unlock()
}

func (m *ChatManager) markWebhookVerified(eventType string) {
	now := time.Now().UnixMilli()
	m.mu.Lock()
	m.webhookVerifiedCount++
	m.webhookLastEventType = strings.TrimSpace(eventType)
	m.webhookLastError = ""
	m.updatedAt = now
	m.mu.Unlock()
}

func (m *ChatManager) acceptWebhookMessageID(messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	now := time.Now().UnixMilli()
	cutoff := now - int64((24*time.Hour)/time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.webhookSeenMessageIDs == nil {
		m.webhookSeenMessageIDs = make(map[string]int64)
	}
	for id, seenAt := range m.webhookSeenMessageIDs {
		if seenAt < cutoff {
			delete(m.webhookSeenMessageIDs, id)
		}
	}
	if _, exists := m.webhookSeenMessageIDs[messageID]; exists {
		return false
	}
	if len(m.webhookSeenMessageIDs) >= 4096 {
		oldestID := ""
		oldestAt := now
		for id, seenAt := range m.webhookSeenMessageIDs {
			if oldestID == "" || seenAt < oldestAt {
				oldestID = id
				oldestAt = seenAt
			}
		}
		if oldestID != "" {
			delete(m.webhookSeenMessageIDs, oldestID)
		}
	}
	m.webhookSeenMessageIDs[messageID] = now
	return true
}

func (m *ChatManager) markWebhookRejected(message string) {
	now := time.Now().UnixMilli()
	m.mu.Lock()
	m.webhookRejectedCount++
	m.webhookLastError = strings.TrimSpace(message)
	m.updatedAt = now
	m.mu.Unlock()
}

func (m *ChatManager) markWebhookAccepted(eventType string, chatEvent bool) {
	now := time.Now().UnixMilli()
	m.mu.Lock()
	m.webhookSubscribed = true
	m.webhookAcceptedCount++
	m.webhookLastEventAt = now
	m.webhookLastEventType = strings.TrimSpace(eventType)
	if chatEvent {
		m.liveChatConnected = true
		m.liveChatStatus = "Receiving verified Kick webhook chat"
	}
	m.updatedAt = now
	m.mu.Unlock()
}

func (m *ChatManager) markWebhookEvent() {
	m.markWebhookAccepted("chat.message.sent", true)
}

func (m *ChatManager) stopLiveChat() {
	m.mu.Lock()
	m.liveChatConnected = false
	m.liveChatStatus = ""
	m.chatroomID = ""
	m.updatedAt = time.Now().UnixMilli()
	m.mu.Unlock()
}
