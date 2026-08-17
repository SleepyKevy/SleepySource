package main

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) handleAlertState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	consume := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("consumer")), "overlay")
	writeJSON(w, a.alerts.state(consume))
}

func (a *App) handleAlertSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("defaults")), "1") || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("defaults")), "true") {
			writeJSON(w, defaultAlertSettings())
			return
		}
		writeJSON(w, a.alerts.settingsSnapshot())
	case http.MethodPost:
		var settings AlertSettings
		if err := decodeSingleJSON(r.Body, &settings, 2<<20); err != nil {
			http.Error(w, "invalid alert settings", http.StatusBadRequest)
			return
		}
		if err := a.alerts.setSettings(settings); err != nil {
			http.Error(w, "could not save alert settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, a.alerts.settingsSnapshot())
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleAlertTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Type        string `json:"type"`
		Username    string `json:"username"`
		Amount      int    `json:"amount"`
		Count       int    `json:"count"`
		Months      int    `json:"months"`
		Tier        string `json:"tier"`
		GiftName    string `json:"gift_name"`
		RewardTitle string `json:"reward_title"`
		UserInput   string `json:"user_input"`
	}
	if err := decodeSingleJSON(r.Body, &in, 1<<20); err != nil {
		http.Error(w, "invalid test alert", http.StatusBadRequest)
		return
	}
	alertType := strings.TrimSpace(in.Type)
	if !isAlertType(alertType) {
		http.Error(w, "unknown alert type", http.StatusBadRequest)
		return
	}
	now := time.Now().UnixMilli()
	event := sampleAlertEvent(alertType, now)
	if value := strings.TrimSpace(in.Username); value != "" {
		event.Username = value
	}
	if in.Amount > 0 {
		event.Amount = in.Amount
	}
	if in.Count > 0 {
		event.Count = in.Count
	}
	if in.Months > 0 {
		event.Months = in.Months
	}
	if value := strings.TrimSpace(in.Tier); value != "" {
		event.Tier = value
	}
	if value := strings.TrimSpace(in.GiftName); value != "" {
		event.GiftName = value
	}
	if value := strings.TrimSpace(in.RewardTitle); value != "" {
		event.RewardTitle = value
	}
	if value := strings.TrimSpace(in.UserInput); value != "" {
		event.UserInput = value
	}
	if !a.alerts.enqueue(event, "") {
		http.Error(w, "alert is disabled or the queue is full", http.StatusConflict)
		return
	}
	writeJSON(w, a.alerts.state(false))
}

func sampleAlertEvent(alertType string, now int64) AlertEvent {
	event := AlertEvent{
		ID:          fmt.Sprintf("test-%s-%d", alertType, now),
		Type:        alertType,
		Source:      "test",
		Username:    "SleepyViewer",
		Tier:        "Kick Sub",
		CreatedAtMS: now,
	}
	switch alertType {
	case "subscription-new":
		event.Months = 1
	case "subscription-renewal":
		event.Months = 6
	case "subscription-gift":
		event.Count = 5
	case "kicks":
		event.Amount = 500
		event.GiftName = "Rage Quit"
	case "reward":
		event.RewardTitle = "Hydrate"
		event.UserInput = "Water break!"
	}
	return event
}

func (a *App) handleAlertControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if err := decodeSingleJSON(r.Body, &in, 1<<20); err != nil {
		http.Error(w, "invalid alert control request", http.StatusBadRequest)
		return
	}
	if err := a.alerts.control(in.Action); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, a.alerts.state(false))
}

func (a *App) handleAlertUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	alertType := strings.TrimSpace(r.URL.Query().Get("type"))
	kind := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("kind")))
	if !isAlertType(alertType) || (kind != "visual" && kind != "sound") {
		http.Error(w, "invalid alert media target", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(maxUploadBytes + (1 << 20)); err != nil {
		http.Error(w, "alert media file is too large (50 MB max)", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "choose a media file first", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > maxUploadBytes {
		http.Error(w, "alert media file is too large (50 MB max)", http.StatusBadRequest)
		return
	}

	var ext string
	if kind == "visual" {
		ext, err = validateMedia(file, header)
	} else {
		ext, err = validateAlertSound(file, header)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "could not read alert media file", http.StatusBadRequest)
		return
	}

	typeDir := filepath.Join(a.alerts.mediaDir, alertType)
	if err := os.MkdirAll(typeDir, 0755); err != nil {
		http.Error(w, "could not prepare alert media folder", http.StatusInternalServerError)
		return
	}
	name := kind + ext
	target := filepath.Join(typeDir, name)
	tmp, err := os.CreateTemp(typeDir, ".alert-media-*.tmp")
	if err != nil {
		http.Error(w, "could not save alert media file", http.StatusInternalServerError)
		return
	}
	tmpName := tmp.Name()
	_, copyErr := io.Copy(tmp, file)
	syncErr := tmp.Sync()
	closeErr := tmp.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		http.Error(w, "could not save alert media file", http.StatusInternalServerError)
		return
	}
	if err := commitTempFile(tmpName, target, 8); err != nil {
		_ = os.Remove(tmpName)
		http.Error(w, "could not finalize alert media file", http.StatusInternalServerError)
		return
	}

	entries, _ := os.ReadDir(typeDir)
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == name || strings.HasPrefix(entry.Name(), ".alert-media-") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(entry.Name()), kind+".") {
			_ = os.Remove(filepath.Join(typeDir, entry.Name()))
		}
	}

	settings := a.alerts.settingsSnapshot()
	style := settings.Types[alertType]
	stamp := time.Now().UnixMilli()
	if kind == "visual" {
		style.VisualFile = name
		style.VisualUpdatedAt = stamp
		if style.DisplayMode == "card" || style.DisplayMode == "" || style.DisplayMode == "text-only" {
			style.DisplayMode = "custom"
			style.MediaX = 0
			style.MediaY = 0
			style.MediaWidth = style.Width
			style.MediaHeight = style.Height
			style.MediaFit = "contain"
			style.MediaOpacity = 100
			style.TitleX = 40
			style.TitleY = maxInt(10, style.Height-165)
			style.TitleWidth = maxInt(80, style.Width-80)
			style.TitleHeight = 100
			style.TitleAlign = "center"
			style.MessageX = 40
			style.MessageY = maxInt(10, style.Height-62)
			style.MessageWidth = maxInt(80, style.Width-80)
			style.MessageHeight = 52
			style.MessageAlign = "center"
		}
	} else {
		style.SoundFile = name
		style.SoundUpdatedAt = stamp
	}
	settings.Types[alertType] = style
	if err := a.alerts.setSettings(settings); err != nil {
		http.Error(w, "media saved, but alert settings could not be updated", http.StatusInternalServerError)
		return
	}
	writeJSON(w, a.alerts.state(false))
}

func (a *App) handleAlertRemoveMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	alertType := strings.TrimSpace(r.URL.Query().Get("type"))
	kind := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("kind")))
	if !isAlertType(alertType) || (kind != "visual" && kind != "sound") {
		http.Error(w, "invalid alert media target", http.StatusBadRequest)
		return
	}
	settings := a.alerts.settingsSnapshot()
	style := settings.Types[alertType]
	var old string
	if kind == "visual" {
		old = style.VisualFile
		style.VisualFile = ""
		style.VisualUpdatedAt = time.Now().UnixMilli()
	} else {
		old = style.SoundFile
		style.SoundFile = ""
		style.SoundUpdatedAt = time.Now().UnixMilli()
	}
	settings.Types[alertType] = style
	if err := a.alerts.setSettings(settings); err != nil {
		http.Error(w, "could not save alert settings", http.StatusInternalServerError)
		return
	}
	if old != "" {
		_ = os.Remove(filepath.Join(a.alerts.mediaDir, alertType, filepath.Base(old)))
	}
	writeJSON(w, a.alerts.state(false))
}

func (a *App) handleAlertMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	alertType := strings.TrimSpace(r.URL.Query().Get("type"))
	kind := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("kind")))
	if !isAlertType(alertType) || (kind != "visual" && kind != "sound") {
		http.NotFound(w, r)
		return
	}
	settings := a.alerts.settingsSnapshot()
	style := settings.Types[alertType]
	name := style.VisualFile
	if kind == "sound" {
		name = style.SoundFile
	}
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(a.alerts.mediaDir, alertType, name)
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

func validateAlertSound(file multipart.File, header *multipart.FileHeader) (string, error) {
	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("could not read sound file")
	}
	data := buf[:n]
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE" && ext == ".wav" {
		return ".wav", nil
	}
	if len(data) >= 4 && string(data[:4]) == "OggS" && ext == ".ogg" {
		return ".ogg", nil
	}
	if len(data) >= 3 && string(data[:3]) == "ID3" && ext == ".mp3" {
		return ".mp3", nil
	}
	if len(data) >= 2 && data[0] == 0xFF && data[1]&0xE0 == 0xE0 && ext == ".mp3" {
		return ".mp3", nil
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" && (ext == ".m4a" || ext == ".mp4") {
		return ".m4a", nil
	}
	return "", errors.New("use an MP3, WAV, OGG, or M4A sound file")
}
