package main

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func fontAllowedExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".ttf", ".otf", ".woff", ".woff2":
		return true
	default:
		return false
	}
}

func sanitizeFontBase(name string) string {
	name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_-")
	if out == "" {
		out = "CustomFont"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func fontFamilyForFile(name string) string {
	return "NPF_" + sanitizeFontBase(name)
}

func (a *App) listFonts() []FontInfo {
	entries, err := os.ReadDir(a.fontDir)
	if err != nil {
		return []FontInfo{}
	}
	fonts := make([]FontInfo, 0, len(entries))
	sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name()) })
	for _, e := range entries {
		if e.IsDir() || !fontAllowedExt(filepath.Ext(e.Name())) {
			continue
		}
		info, _ := e.Info()
		v := int64(0)
		if info != nil {
			v = info.ModTime().UnixNano()
		}
		name := e.Name()
		fonts = append(fonts, FontInfo{
			ID:     name,
			Name:   strings.TrimSuffix(name, filepath.Ext(name)),
			Family: fontFamilyForFile(name),
			URL:    "/font?name=" + url.QueryEscape(name) + fmt.Sprintf("&v=%d", v),
		})
	}
	return fonts
}

func validateFont(file multipart.File, header *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !fontAllowedExt(ext) {
		return "", errors.New("use a TTF, OTF, WOFF, or WOFF2 font file")
	}
	buf := make([]byte, 16)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("could not read font file")
	}
	buf = buf[:n]
	valid := false
	switch ext {
	case ".ttf":
		valid = len(buf) >= 4 && ((buf[0] == 0x00 && buf[1] == 0x01 && buf[2] == 0x00 && buf[3] == 0x00) || string(buf[:4]) == "true")
	case ".otf":
		valid = len(buf) >= 4 && string(buf[:4]) == "OTTO"
	case ".woff":
		valid = len(buf) >= 4 && string(buf[:4]) == "wOFF"
	case ".woff2":
		valid = len(buf) >= 4 && string(buf[:4]) == "wOF2"
	}
	if !valid {
		return "", errors.New("the selected file does not look like a valid " + strings.TrimPrefix(strings.ToUpper(ext), ".") + " font")
	}
	return ext, nil
}

func (a *App) handleUploadFont(w http.ResponseWriter, r *http.Request) {
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
	tmpFile, err := os.CreateTemp(a.fontDir, ".font-upload-*.tmp")
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
	// Only remove alternate extensions after the new font is safely committed.
	entries, _ := os.ReadDir(a.fontDir)
	for _, e := range entries {
		if e.IsDir() || e.Name() == name || strings.HasPrefix(e.Name(), ".font-upload-") {
			continue
		}
		if sanitizeFontBase(e.Name()) == base && fontAllowedExt(filepath.Ext(e.Name())) {
			_ = os.Remove(filepath.Join(a.fontDir, e.Name()))
		}
	}
	family := fontFamilyForFile(name)
	a.mu.Lock()
	a.settings.TextFont = family
	a.updatedAt = time.Now().UnixMilli()
	err = a.saveSettingsLocked()
	a.mu.Unlock()
	if err != nil {
		http.Error(w, "font saved, but settings could not be updated", http.StatusInternalServerError)
		return
	}
	writeJSON(w, a.snapshot())
}

func (a *App) handleRemoveFont(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Family string `json:"family"`
	}
	if err := decodeSingleJSON(r.Body, &req, 32<<10); err != nil {
		http.Error(w, "invalid font request", http.StatusBadRequest)
		return
	}
	var target string
	for _, f := range a.listFonts() {
		if f.Family == req.Family {
			target = filepath.Join(a.fontDir, filepath.Base(f.ID))
			break
		}
	}
	if target == "" {
		http.Error(w, "select an uploaded custom font first", http.StatusBadRequest)
		return
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		http.Error(w, "could not remove font file", http.StatusInternalServerError)
		return
	}
	var updateErr error
	a.mu.Lock()
	if a.settings.TextFont == req.Family {
		a.settings.TextFont = "Segoe UI"
	}
	a.updatedAt = time.Now().UnixMilli()
	if err := a.saveSettingsLocked(); err != nil {
		updateErr = err
	}
	a.mu.Unlock()

	chatSettings := a.chat.state().Settings
	if chatSettings.FontFamily == req.Family {
		chatSettings.FontFamily = "Segoe UI"
		if err := a.chat.setSettings(chatSettings); err != nil && updateErr == nil {
			updateErr = err
		}
	}
	countdownSettings := a.countdown.state(nil).Settings
	if countdownSettings.FontFamily == req.Family {
		countdownSettings.FontFamily = "Segoe UI"
		if err := a.countdown.applySettings(countdownSettings); err != nil && updateErr == nil {
			updateErr = err
		}
	}
	if updateErr != nil {
		http.Error(w, "font removed, but one or more settings files could not be updated", http.StatusInternalServerError)
		return
	}
	writeJSON(w, a.snapshot())
}

func (a *App) handleFont(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := filepath.Base(r.URL.Query().Get("name"))
	if name == "." || name == "" || !fontAllowedExt(filepath.Ext(name)) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(a.fontDir, name)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ttf":
		w.Header().Set("Content-Type", "font/ttf")
	case ".otf":
		w.Header().Set("Content-Type", "font/otf")
	case ".woff":
		w.Header().Set("Content-Type", "font/woff")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	}
	http.ServeFile(w, r, path)
}
