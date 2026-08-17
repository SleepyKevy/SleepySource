package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *App) handleChatState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, a.chat.state())
}

func (a *App) handleChatSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		writeJSON(w, a.chat.state().Settings)
	case http.MethodPost:
		var s ChatSettings
		if err := decodeSingleJSON(r.Body, &s, 1<<20); err != nil {
			http.Error(w, "invalid settings", http.StatusBadRequest)
			return
		}
		if err := a.chat.setSettings(s); err != nil {
			http.Error(w, "could not save chat settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, a.chat.state().Settings)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleChatUploadFont(w http.ResponseWriter, r *http.Request) {
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
	tmpFile, err := os.CreateTemp(a.fontDir, ".chat-font-upload-*.tmp")
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
		if e.IsDir() || e.Name() == name || strings.HasPrefix(e.Name(), ".chat-font-upload-") {
			continue
		}
		if sanitizeFontBase(e.Name()) == base && fontAllowedExt(filepath.Ext(e.Name())) {
			_ = os.Remove(filepath.Join(a.fontDir, e.Name()))
		}
	}
	family := fontFamilyForFile(name)
	next := a.chat.state().Settings
	next.FontFamily = family
	if err := a.chat.setSettings(next); err != nil {
		http.Error(w, "font saved, but chat settings could not be updated", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"settings": a.chat.state().Settings,
		"fonts":    a.listFonts(),
	})
}

func (a *App) handleChatTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Username string `json:"username"`
		Text     string `json:"text"`
		UserID   string `json:"user_id"`
		IsMod    bool   `json:"is_mod"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in)
	if strings.TrimSpace(in.Username) == "" {
		in.Username = "SleepyViewer"
	}
	if strings.TrimSpace(in.Text) == "" {
		in.Text = "This is a test chat message OMEGALUL"
	}
	if strings.TrimSpace(in.UserID) == "" {
		in.UserID = "123456"
	}
	a.chat.addMessage(ChatMessage{UserID: in.UserID, Username: in.Username, Text: in.Text, Color: "#55B7FF", IsMod: true, Badges: []string{"Moderator", "Subscriber"}, BadgeDetails: []ChatBadge{{Text: "Moderator", Type: "moderator"}, {Text: "Subscriber", Type: "subscriber", Count: 3}}})
	writeJSON(w, a.chat.state())
}

func (a *App) handleChatClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.chat.clearMessages()
	w.WriteHeader(http.StatusNoContent)
}

// handleChatIngest accepts normalized messages from a trusted local relay on the same
// machine. Official Kick webhooks should use /api/chat/kick-webhook so signatures are
// verified before the message reaches the overlay.
func (a *App) handleChatIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(data) == 0 {
		http.Error(w, "invalid chat message", http.StatusBadRequest)
		return
	}
	if msg, ok := parseKickWebhookChatMessage(data); ok {
		a.chat.addMessage(msg)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var msg ChatMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		http.Error(w, "invalid chat message", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(msg.Text) == "" {
		http.Error(w, "message text required", http.StatusBadRequest)
		return
	}
	a.chat.addMessage(msg)
	w.WriteHeader(http.StatusNoContent)
}

func parseKickWebhookChatMessage(data []byte) (ChatMessage, bool) {
	var raw struct {
		MessageID   string `json:"message_id"`
		Content     string `json:"content"`
		CreatedAt   string `json:"created_at"`
		Broadcaster struct {
			UserID int64 `json:"user_id"`
		} `json:"broadcaster"`
		Sender struct {
			UserID         int64  `json:"user_id"`
			Username       string `json:"username"`
			ProfilePicture string `json:"profile_picture"`
			Identity       *struct {
				UsernameColor string `json:"username_color"`
				Badges        []struct {
					Text  string `json:"text"`
					Type  string `json:"type"`
					Count int    `json:"count"`
				} `json:"badges"`
			} `json:"identity"`
		} `json:"sender"`
	}
	if json.Unmarshal(data, &raw) != nil || strings.TrimSpace(raw.MessageID) == "" || strings.TrimSpace(raw.Content) == "" {
		return ChatMessage{}, false
	}
	msg := ChatMessage{
		ID:        strings.TrimSpace(raw.MessageID),
		UserID:    strconv.FormatInt(raw.Sender.UserID, 10),
		Username:  strings.TrimSpace(raw.Sender.Username),
		Text:      strings.TrimSpace(raw.Content),
		AvatarURL: strings.TrimSpace(raw.Sender.ProfilePicture),
	}
	if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw.CreatedAt)); err == nil {
		msg.CreatedAt = parsed.UnixMilli()
	}
	seenTypes := map[string]bool{}
	if raw.Sender.Identity != nil {
		msg.Color = strings.TrimSpace(raw.Sender.Identity.UsernameColor)
		for _, badge := range raw.Sender.Identity.Badges {
			typ := normalizeKickBadgeType(badge.Type)
			label := strings.TrimSpace(badge.Text)
			if label == "" {
				label = typ
			}
			if label != "" {
				msg.Badges = append(msg.Badges, label)
			}
			if typ != "" {
				msg.BadgeDetails = append(msg.BadgeDetails, ChatBadge{Text: label, Type: typ, Count: badge.Count})
				seenTypes[typ] = true
			}
			if typ == "moderator" {
				msg.IsMod = true
			}
		}
	}
	if raw.Broadcaster.UserID != 0 && raw.Broadcaster.UserID == raw.Sender.UserID && !seenTypes["broadcaster"] {
		msg.Badges = append([]string{"Broadcaster"}, msg.Badges...)
		msg.BadgeDetails = append([]ChatBadge{{Text: "Broadcaster", Type: "broadcaster"}}, msg.BadgeDetails...)
	}
	return msg, true
}
