package main

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(maxUploadBytes + (1 << 20)); err != nil {
		http.Error(w, "media file is too large (50 MB max)", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "missing media file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > maxUploadBytes {
		http.Error(w, "media file is too large (50 MB max)", http.StatusBadRequest)
		return
	}

	ext, err := validateMedia(file, header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "could not read media file", http.StatusBadRequest)
		return
	}

	name := "custom_now_playing" + ext
	target := filepath.Join(a.customDir, name)
	out, err := os.CreateTemp(a.customDir, ".artwork-upload-*.tmp")
	if err != nil {
		http.Error(w, "could not save media file", http.StatusInternalServerError)
		return
	}
	tmp := out.Name()
	_, copyErr := io.Copy(out, file)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		http.Error(w, "could not save media file", http.StatusInternalServerError)
		return
	}
	if err := commitTempFile(tmp, target, 8); err != nil {
		_ = os.Remove(tmp)
		http.Error(w, "could not finalize media file", http.StatusInternalServerError)
		return
	}
	// Remove the previous custom artwork only after the replacement is safe.
	entries, _ := os.ReadDir(a.customDir)
	for _, e := range entries {
		if e.IsDir() || e.Name() == name || strings.HasPrefix(e.Name(), ".artwork-upload-") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(e.Name()), "custom_now_playing.") {
			_ = os.Remove(filepath.Join(a.customDir, e.Name()))
		}
	}

	a.mu.Lock()
	a.customPath = target
	a.settings.CustomImage = name
	a.settings.ImageMode = "custom"
	a.updatedAt = time.Now().UnixMilli()
	saveErr := a.saveSettingsLocked()
	a.mu.Unlock()
	if saveErr != nil {
		http.Error(w, "media saved, but settings could not be updated", http.StatusInternalServerError)
		return
	}
	writeJSON(w, a.snapshot())
}

func (a *App) handleUploadBackground(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(maxUploadBytes + (1 << 20)); err != nil {
		http.Error(w, "background file is too large (50 MB max)", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("background")
	if err != nil {
		http.Error(w, "missing background media file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > maxUploadBytes {
		http.Error(w, "background file is too large (50 MB max)", http.StatusBadRequest)
		return
	}
	ext, err := validateMedia(file, header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "could not read background media file", http.StatusBadRequest)
		return
	}
	name := "custom_background" + ext
	target := filepath.Join(a.customDir, name)
	out, err := os.CreateTemp(a.customDir, ".background-upload-*.tmp")
	if err != nil {
		http.Error(w, "could not save background media file", http.StatusInternalServerError)
		return
	}
	tmp := out.Name()
	_, copyErr := io.Copy(out, file)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		http.Error(w, "could not save background media file", http.StatusInternalServerError)
		return
	}
	if err := commitTempFile(tmp, target, 8); err != nil {
		_ = os.Remove(tmp)
		http.Error(w, "could not finalize background media file", http.StatusInternalServerError)
		return
	}
	entries, _ := os.ReadDir(a.customDir)
	for _, e := range entries {
		if e.IsDir() || e.Name() == name || strings.HasPrefix(e.Name(), ".background-upload-") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(e.Name()), "custom_background.") {
			_ = os.Remove(filepath.Join(a.customDir, e.Name()))
		}
	}
	a.mu.Lock()
	a.backgroundPath = target
	a.settings.CustomBackground = name
	a.settings.BackgroundMode = "custom"
	a.updatedAt = time.Now().UnixMilli()
	saveErr := a.saveSettingsLocked()
	a.mu.Unlock()
	if saveErr != nil {
		http.Error(w, "background saved, but settings could not be updated", http.StatusInternalServerError)
		return
	}
	writeJSON(w, a.snapshot())
}

func (a *App) handleRemoveBackground(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.mu.Lock()
	old := a.backgroundPath
	a.backgroundPath = ""
	a.settings.CustomBackground = ""
	if a.settings.BackgroundMode == "custom" {
		a.settings.BackgroundMode = "transparent"
	}
	a.updatedAt = time.Now().UnixMilli()
	err := a.saveSettingsLocked()
	a.mu.Unlock()
	if old != "" {
		_ = os.Remove(old)
	}
	if err != nil {
		http.Error(w, "could not save settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, a.snapshot())
}

func (a *App) handleCustomBackground(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.mu.RLock()
	p := a.backgroundPath
	a.mu.RUnlock()
	if p == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}

func validateMedia(file multipart.File, header *multipart.FileHeader) (string, error) {
	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("could not read media file")
	}
	data := buf[:n]
	mimeType := http.DetectContentType(data)
	switch mimeType {
	case "image/png":
		return ".png", nil
	case "image/jpeg":
		return ".jpg", nil
	case "image/gif":
		return ".gif", nil
	case "image/webp":
		return ".webp", nil
	case "video/webm":
		return ".webm", nil
	}

	// Go's built-in MIME sniffer does not identify every valid WebM file.
	// WebM is an EBML container and starts with the EBML header 1A 45 DF A3.
	// Require both that signature and a .webm filename before accepting it.
	nameExt := strings.ToLower(filepath.Ext(header.Filename))
	if nameExt == ".webm" && len(data) >= 4 && data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3 {
		return ".webm", nil
	}
	return "", errors.New("use a PNG, JPG, GIF, WEBP, or WEBM file")
}

func (a *App) handleRemoveImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.mu.Lock()
	old := a.customPath
	a.customPath = ""
	a.settings.CustomImage = ""
	a.settings.ImageMode = "fallback"
	a.updatedAt = time.Now().UnixMilli()
	err := a.saveSettingsLocked()
	a.mu.Unlock()
	if old != "" {
		_ = os.Remove(old)
	}
	if err != nil {
		http.Error(w, "could not save settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, a.snapshot())
}

func (a *App) handleCustomImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.mu.RLock()
	p := a.customPath
	a.mu.RUnlock()
	if p == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}

func (a *App) handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go openFolder(a.dataDir)
	w.WriteHeader(http.StatusNoContent)
}
