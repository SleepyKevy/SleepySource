package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func sanitizeProfileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func copyFileSafe(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(dst), ".profile-copy-*.tmp")
	if err != nil {
		return err
	}
	tmp := out.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err = out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = commitTempFile(tmp, dst, 5); err != nil {
		return err
	}
	ok = true
	return nil
}

func (a *App) listProfiles() []ProfileInfo {
	entries, err := os.ReadDir(a.profileDir)
	if err != nil {
		return []ProfileInfo{}
	}
	a.mu.RLock()
	defaultProfile := strings.TrimSpace(a.settings.DefaultProfile)
	a.mu.RUnlock()
	out := make([]ProfileInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(a.profileDir, e.Name(), "profile.json")
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		out = append(out, ProfileInfo{Name: e.Name(), ModifiedAt: info.ModTime().UnixMilli(), Default: strings.EqualFold(defaultProfile, e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func (a *App) fontPathForFamily(family string) string {
	if strings.TrimSpace(family) == "" || !strings.HasPrefix(family, "NPF_") {
		return ""
	}
	entries, err := os.ReadDir(a.fontDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !fontAllowedExt(filepath.Ext(e.Name())) {
			continue
		}
		if fontFamilyForFile(e.Name()) == family {
			return filepath.Join(a.fontDir, e.Name())
		}
	}
	return ""
}

func removeProfileManagedFiles(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if name == "profile.json" || name == "bundle.json" || strings.HasPrefix(name, "artwork.") || strings.HasPrefix(name, "background.") || strings.HasPrefix(name, "font_") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func (a *App) saveProfile(name string) error {
	name = sanitizeProfileName(name)
	if name == "" {
		return errors.New("enter a profile name")
	}
	dir := filepath.Join(a.profileDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	removeProfileManagedFiles(dir)
	a.mu.RLock()
	s := a.settings
	art := a.customPath
	bg := a.backgroundPath
	a.mu.RUnlock()
	normalizeSettings(&s)
	if art != "" {
		ext := strings.ToLower(filepath.Ext(art))
		dst := filepath.Join(dir, "artwork"+ext)
		if err := copyFileSafe(art, dst); err != nil {
			return fmt.Errorf("save profile artwork: %w", err)
		}
		s.CustomImage = filepath.Base(dst)
	}
	if bg != "" {
		ext := strings.ToLower(filepath.Ext(bg))
		dst := filepath.Join(dir, "background"+ext)
		if err := copyFileSafe(bg, dst); err != nil {
			return fmt.Errorf("save profile background: %w", err)
		}
		s.CustomBackground = filepath.Base(dst)
	}
	if fontPath := a.fontPathForFamily(s.TextFont); fontPath != "" {
		fontName := filepath.Base(fontPath)
		dst := filepath.Join(dir, "font_"+fontName)
		if err := copyFileSafe(fontPath, dst); err != nil {
			return fmt.Errorf("save profile font: %w", err)
		}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeSettingsFile(filepath.Join(dir, "profile.json"), data)
}

func (a *App) loadProfile(name string) error {
	name = sanitizeProfileName(name)
	if name == "" {
		return errors.New("select a profile")
	}
	dir := filepath.Join(a.profileDir, name)
	data, err := os.ReadFile(filepath.Join(dir, "profile.json"))
	if err != nil {
		return err
	}
	next := defaultSettings()
	if err := json.Unmarshal(data, &next); err != nil {
		return err
	}
	normalizeSettings(&next)
	a.mu.RLock()
	global := a.settings
	a.mu.RUnlock()
	// Profiles describe the overlay itself. Keep application/editor preferences global.
	next.SnapEnabled = global.SnapEnabled
	next.GridSize = global.GridSize
	next.OnboardingComplete = global.OnboardingComplete
	next.DesignerTheme = global.DesignerTheme
	next.StartupPage = global.StartupPage
	next.LastModule = global.LastModule
	next.DefaultProfile = global.DefaultProfile
	next.MediaSourceMode = global.MediaSourceMode
	next.MediaSourceInclude = global.MediaSourceInclude
	next.MediaSourceExclude = global.MediaSourceExclude
	var artPath, bgPath string
	for _, e := range []struct {
		base         string
		targetPrefix string
		set          func(string)
	}{
		{"artwork", "custom_now_playing", func(v string) { next.CustomImage = v }},
		{"background", "custom_background", func(v string) { next.CustomBackground = v }},
	} {
		matches, _ := filepath.Glob(filepath.Join(dir, e.base+".*"))
		if len(matches) == 0 {
			continue
		}
		ext := strings.ToLower(filepath.Ext(matches[0]))
		target := filepath.Join(a.customDir, e.targetPrefix+ext)
		if err := copyFileSafe(matches[0], target); err != nil {
			return err
		}
		entries, _ := os.ReadDir(a.customDir)
		for _, old := range entries {
			if old.IsDir() || old.Name() == filepath.Base(target) {
				continue
			}
			if strings.HasPrefix(strings.ToLower(old.Name()), strings.ToLower(e.targetPrefix)+".") {
				_ = os.Remove(filepath.Join(a.customDir, old.Name()))
			}
		}
		e.set(filepath.Base(target))
		if e.base == "artwork" {
			artPath = target
		} else {
			bgPath = target
		}
	}
	fontMatches, _ := filepath.Glob(filepath.Join(dir, "font_*"))
	if len(fontMatches) > 0 {
		fontSrc := fontMatches[0]
		fontName := strings.TrimPrefix(filepath.Base(fontSrc), "font_")
		if fontAllowedExt(filepath.Ext(fontName)) {
			fontTarget := filepath.Join(a.fontDir, filepath.Base(fontName))
			if err := copyFileSafe(fontSrc, fontTarget); err != nil {
				return fmt.Errorf("restore profile font: %w", err)
			}
		}
	}
	if strings.HasPrefix(next.TextFont, "NPF_") && a.fontPathForFamily(next.TextFont) == "" {
		next.TextFont = "Segoe UI"
	}
	a.mu.Lock()
	next.CustomImage = func() string {
		if artPath != "" {
			return filepath.Base(artPath)
		}
		return a.settings.CustomImage
	}()
	next.CustomBackground = func() string {
		if bgPath != "" {
			return filepath.Base(bgPath)
		}
		return a.settings.CustomBackground
	}()
	a.settings = next
	if artPath != "" {
		a.customPath = artPath
	}
	if bgPath != "" {
		a.backgroundPath = bgPath
	}
	a.updatedAt = time.Now().UnixMilli()
	err = a.saveSettingsLocked()
	current := a.track
	a.mu.Unlock()
	a.writeOutput(current)
	return err
}

func copyProfileDirectory(srcDir, dstDir string) error {
	if _, err := os.Stat(filepath.Join(srcDir, "profile.json")); err != nil {
		return errors.New("profile not found")
	}
	if _, err := os.Stat(dstDir); err == nil {
		return errors.New("a profile with that name already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(dstDir)
		}
	}()
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !isAllowedProfileBundleEntry(entry.Name()) || strings.EqualFold(entry.Name(), "bundle.json") {
			continue
		}
		if err := copyFileSafe(filepath.Join(srcDir, entry.Name()), filepath.Join(dstDir, entry.Name())); err != nil {
			return err
		}
	}
	ok = true
	return nil
}

func (a *App) duplicateProfile(name, newName string) (string, error) {
	name = sanitizeProfileName(name)
	newName = sanitizeProfileName(newName)
	if name == "" {
		return "", errors.New("select a profile")
	}
	if newName == "" {
		existing := map[string]bool{}
		for _, p := range a.listProfiles() {
			existing[strings.ToLower(p.Name)] = true
		}
		newName = uniqueProfileName(name+" Copy", existing)
	}
	if strings.EqualFold(name, newName) {
		return "", errors.New("choose a different name for the duplicate")
	}
	if err := copyProfileDirectory(filepath.Join(a.profileDir, name), filepath.Join(a.profileDir, newName)); err != nil {
		return "", err
	}
	return newName, nil
}

func (a *App) renameProfile(name, newName string) error {
	name = sanitizeProfileName(name)
	newName = sanitizeProfileName(newName)
	if name == "" || newName == "" {
		return errors.New("select a profile and enter its new name")
	}
	if strings.EqualFold(name, newName) {
		if name == newName {
			return nil
		}
		// Windows profile folders are case-insensitive. Use a temporary hop for
		// capitalization-only renames so the operation remains portable.
		tmp := filepath.Join(a.profileDir, ".rename-"+fmt.Sprint(time.Now().UnixNano()))
		if err := os.Rename(filepath.Join(a.profileDir, name), tmp); err != nil {
			return err
		}
		if err := os.Rename(tmp, filepath.Join(a.profileDir, newName)); err != nil {
			_ = os.Rename(tmp, filepath.Join(a.profileDir, name))
			return err
		}
	} else {
		if _, err := os.Stat(filepath.Join(a.profileDir, newName)); err == nil {
			return errors.New("a profile with that name already exists")
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(filepath.Join(a.profileDir, name), filepath.Join(a.profileDir, newName)); err != nil {
			return err
		}
	}
	a.mu.Lock()
	if strings.EqualFold(a.settings.DefaultProfile, name) {
		a.settings.DefaultProfile = newName
		a.updatedAt = time.Now().UnixMilli()
		if err := a.saveSettingsLocked(); err != nil {
			a.mu.Unlock()
			return err
		}
	}
	a.mu.Unlock()
	return nil
}

func (a *App) setDefaultProfile(name string) error {
	name = sanitizeProfileName(name)
	if name != "" {
		if _, err := os.Stat(filepath.Join(a.profileDir, name, "profile.json")); err != nil {
			return errors.New("profile not found")
		}
	}
	a.mu.Lock()
	a.settings.DefaultProfile = name
	a.updatedAt = time.Now().UnixMilli()
	err := a.saveSettingsLocked()
	a.mu.Unlock()
	return err
}

func (a *App) deleteProfile(name string) error {
	name = sanitizeProfileName(name)
	if name == "" {
		return errors.New("select a profile")
	}
	if err := os.RemoveAll(filepath.Join(a.profileDir, name)); err != nil {
		return err
	}
	a.mu.Lock()
	if strings.EqualFold(a.settings.DefaultProfile, name) {
		a.settings.DefaultProfile = ""
		a.updatedAt = time.Now().UnixMilli()
		if err := a.saveSettingsLocked(); err != nil {
			a.mu.Unlock()
			return err
		}
	}
	a.mu.Unlock()
	return nil
}

func (a *App) loadDefaultProfileAtStartup() {
	a.mu.RLock()
	name := strings.TrimSpace(a.settings.DefaultProfile)
	a.mu.RUnlock()
	if name == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(a.profileDir, name, "profile.json")); err != nil {
		// A removed/moved profile should not cause every future launch to report
		// a startup error. Clear the stale preference and keep the current layout.
		if clearErr := a.setDefaultProfile(""); clearErr != nil {
			fmt.Printf("could not clear missing default profile %q: %v\n", name, clearErr)
		}
		return
	}
	if err := a.loadProfile(name); err != nil {
		fmt.Printf("could not load default profile %q: %v\n", name, err)
	}
}

func safeBundleFileName(name string) string {
	name = sanitizeProfileName(name)
	if name == "" {
		name = "SleepySource_Profile"
	}
	return strings.ReplaceAll(name, " ", "_") + ".sleepyprofile.zip"
}

func isAllowedProfileBundleEntry(name string) bool {
	base := filepath.Base(name)
	lower := strings.ToLower(base)
	if name != base || base == "." || base == "" {
		return false
	}
	if lower == "bundle.json" || lower == "profile.json" {
		return true
	}
	if strings.HasPrefix(lower, "artwork.") || strings.HasPrefix(lower, "background.") {
		ext := strings.ToLower(filepath.Ext(base))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".webm":
			return true
		}
		return false
	}
	if strings.HasPrefix(lower, "font_") {
		return fontAllowedExt(filepath.Ext(base))
	}
	return false
}

func (a *App) handleProfileExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := sanitizeProfileName(r.URL.Query().Get("name"))
	if name == "" {
		http.Error(w, "select a profile to export", http.StatusBadRequest)
		return
	}
	dir := filepath.Join(a.profileDir, name)
	if _, err := os.Stat(filepath.Join(dir, "profile.json")); err != nil {
		http.Error(w, "profile not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeBundleFileName(name)))
	zw := zip.NewWriter(w)
	manifest, _ := json.MarshalIndent(ProfileBundleManifest{Format: "SleepySourceProfileBundle", Version: 1, Name: name}, "", "  ")
	mf, err := zw.Create("bundle.json")
	if err != nil {
		_ = zw.Close()
		return
	}
	_, _ = mf.Write(manifest)
	entries, err := os.ReadDir(dir)
	if err != nil {
		_ = zw.Close()
		return
	}
	for _, e := range entries {
		if e.IsDir() || !isAllowedProfileBundleEntry(e.Name()) || strings.EqualFold(e.Name(), "bundle.json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		in, err := os.Open(path)
		if err != nil {
			continue
		}
		entry, createErr := zw.Create(e.Name())
		if createErr == nil {
			_, _ = io.Copy(entry, io.LimitReader(in, maxProfileBundleBytes))
		}
		_ = in.Close()
	}
	_ = zw.Close()
}

func uniqueProfileName(base string, existing map[string]bool) string {
	base = sanitizeProfileName(base)
	if base == "" {
		base = "Imported Profile"
	}
	if !existing[strings.ToLower(base)] {
		return base
	}
	for i := 2; i <= 99; i++ {
		candidate := sanitizeProfileName(fmt.Sprintf("%s Copy %d", base, i))
		if !existing[strings.ToLower(candidate)] {
			return candidate
		}
	}
	return sanitizeProfileName(base + " Imported")
}

func validateBundleMedia(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := &multipart.FileHeader{Filename: filepath.Base(path)}
	_, err = validateMedia(f, h)
	return err
}

func validateBundleFont(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := &multipart.FileHeader{Filename: strings.TrimPrefix(filepath.Base(path), "font_")}
	_, err = validateFont(f, h)
	return err
}

func (a *App) handleProfileImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileBundleBytes+(2<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "profile bundle is too large or invalid", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("bundle")
	if err != nil {
		http.Error(w, "choose a SleepySource profile bundle first", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > maxProfileBundleBytes {
		http.Error(w, "profile bundle is too large", http.StatusBadRequest)
		return
	}
	tmp, err := os.CreateTemp(a.profileDir, ".bundle-*.zip")
	if err != nil {
		http.Error(w, "could not prepare profile import", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, io.LimitReader(file, maxProfileBundleBytes+1)); err != nil || tmp.Sync() != nil || tmp.Close() != nil {
		_ = tmp.Close()
		http.Error(w, "could not read profile bundle", http.StatusBadRequest)
		return
	}
	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		http.Error(w, "invalid profile bundle ZIP", http.StatusBadRequest)
		return
	}
	defer zr.Close()
	if len(zr.File) == 0 || len(zr.File) > 32 {
		http.Error(w, "profile bundle has an invalid file count", http.StatusBadRequest)
		return
	}
	manifest := ProfileBundleManifest{}
	var total uint64
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() || zf.Mode()&os.ModeSymlink != 0 || !isAllowedProfileBundleEntry(zf.Name) {
			http.Error(w, "profile bundle contains unsupported files", http.StatusBadRequest)
			return
		}
		total += zf.UncompressedSize64
		if total > maxProfileBundleBytes {
			http.Error(w, "profile bundle expands beyond the allowed size", http.StatusBadRequest)
			return
		}
		if strings.EqualFold(zf.Name, "bundle.json") {
			rc, _ := zf.Open()
			data, _ := io.ReadAll(io.LimitReader(rc, 64<<10))
			_ = rc.Close()
			_ = json.Unmarshal(data, &manifest)
		}
	}
	name := sanitizeProfileName(manifest.Name)
	if name == "" {
		base := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
		base = strings.TrimSuffix(base, ".sleepyprofile")
		name = sanitizeProfileName(strings.ReplaceAll(base, "_", " "))
	}
	if name == "" {
		name = "Imported Profile"
	}
	existing := map[string]bool{}
	for _, p := range a.listProfiles() {
		existing[strings.ToLower(p.Name)] = true
	}
	overwrite := r.FormValue("overwrite") == "1"
	if existing[strings.ToLower(name)] && !overwrite {
		http.Error(w, "profile already exists", http.StatusConflict)
		return
	}
	if !overwrite {
		name = uniqueProfileName(name, existing)
	}
	stage, err := os.MkdirTemp(a.profileDir, ".import-*")
	if err != nil {
		http.Error(w, "could not prepare profile import", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(stage)
	for _, zf := range zr.File {
		if strings.EqualFold(zf.Name, "bundle.json") {
			continue
		}
		target := filepath.Join(stage, filepath.Base(zf.Name))
		rc, err := zf.Open()
		if err != nil {
			http.Error(w, "could not read profile bundle entry", http.StatusBadRequest)
			return
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			_ = rc.Close()
			http.Error(w, "could not stage profile bundle", http.StatusInternalServerError)
			return
		}
		_, copyErr := io.Copy(out, io.LimitReader(rc, int64(zf.UncompressedSize64)+1))
		_ = out.Close()
		_ = rc.Close()
		if copyErr != nil {
			http.Error(w, "could not extract profile bundle", http.StatusBadRequest)
			return
		}
	}
	profileData, err := os.ReadFile(filepath.Join(stage, "profile.json"))
	if err != nil {
		http.Error(w, "profile bundle is missing profile.json", http.StatusBadRequest)
		return
	}
	next := defaultSettings()
	if err := json.Unmarshal(profileData, &next); err != nil {
		http.Error(w, "profile bundle contains invalid settings", http.StatusBadRequest)
		return
	}
	normalizeSettings(&next)
	entries, _ := os.ReadDir(stage)
	for _, e := range entries {
		lower := strings.ToLower(e.Name())
		path := filepath.Join(stage, e.Name())
		if strings.HasPrefix(lower, "artwork.") || strings.HasPrefix(lower, "background.") {
			if err := validateBundleMedia(path); err != nil {
				http.Error(w, "profile bundle contains invalid media: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
		if strings.HasPrefix(lower, "font_") {
			if err := validateBundleFont(path); err != nil {
				http.Error(w, "profile bundle contains invalid font: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
	}
	finalDir := filepath.Join(a.profileDir, name)
	if overwrite {
		_ = os.RemoveAll(finalDir)
	}
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		http.Error(w, "could not create imported profile", http.StatusInternalServerError)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFileSafe(filepath.Join(stage, e.Name()), filepath.Join(finalDir, e.Name())); err != nil {
			http.Error(w, "could not finalize imported profile", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, struct {
		Name     string        `json:"name"`
		Profiles []ProfileInfo `json:"profiles"`
	}{Name: name, Profiles: a.listProfiles()})
}

func (a *App) handleProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, a.listProfiles())
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Action  string `json:"action"`
		Name    string `json:"name"`
		NewName string `json:"new_name"`
	}
	if err := decodeSingleJSON(r.Body, &req, 64<<10); err != nil {
		http.Error(w, "invalid profile request", http.StatusBadRequest)
		return
	}
	name := sanitizeProfileName(req.Name)
	var err error
	resultName := name
	switch req.Action {
	case "save":
		err = a.saveProfile(name)
	case "load":
		err = a.loadProfile(name)
	case "duplicate":
		resultName, err = a.duplicateProfile(name, req.NewName)
	case "rename":
		resultName = sanitizeProfileName(req.NewName)
		err = a.renameProfile(name, resultName)
	case "set_default":
		err = a.setDefaultProfile(name)
	case "clear_default":
		resultName = ""
		err = a.setDefaultProfile("")
	case "delete":
		err = a.deleteProfile(name)
	default:
		err = errors.New("unknown profile action")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result := a.snapshot()
	if req.Action == "duplicate" || req.Action == "rename" || req.Action == "set_default" || req.Action == "clear_default" || req.Action == "delete" {
		writeJSON(w, struct {
			State AppState `json:"state"`
			Name  string   `json:"name,omitempty"`
		}{State: result, Name: resultName})
		return
	}
	writeJSON(w, result)
}
