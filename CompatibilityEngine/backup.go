package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxBackupUploadBytes int64 = 500 << 20
const maxBackupExtractBytes int64 = 1 << 30
const maxBackupFiles = 10000

type backupManifest struct {
	Product   string `json:"product"`
	Version   string `json:"version"`
	CreatedAt string `json:"created_at"`
}

func cleanBackupRelative(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	if strings.HasPrefix(name, "/") || name == "" {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return clean, true
}

func skipBackupRelative(rel string) bool {
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(rel)))
	if clean == "alerts" || strings.HasPrefix(clean, "alerts/") ||
		clean == "kick" || strings.HasPrefix(clean, "kick/") ||
		clean == "desktopruntime" || strings.HasPrefix(clean, "desktopruntime/") ||
		clean == "app.ico" || clean == "kick_credentials.json" || clean == "kick_user_authorization.json" {
		return true
	}
	base := filepath.Base(clean)
	return strings.HasSuffix(base, ".tmp") ||
		strings.HasPrefix(base, ".sleepysource-settings-") ||
		strings.HasPrefix(base, ".chat-settings-") ||
		strings.HasPrefix(base, ".kick-credentials-") ||
		strings.HasPrefix(base, ".kick-user-auth-") ||
		strings.HasPrefix(base, ".font-upload-") ||
		strings.HasPrefix(base, ".countdown-font-upload-") ||
		strings.HasPrefix(base, ".chat-font-upload-") ||
		strings.HasPrefix(base, ".artwork-upload-") ||
		strings.HasPrefix(base, ".background-upload-") ||
		strings.HasPrefix(base, ".icon-") ||
		strings.HasPrefix(base, ".profile-copy-") ||
		strings.HasPrefix(base, ".bundle-")
}

func preserveRestoreRelative(rel string) bool {
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(rel)))
	return clean == "desktopruntime" || strings.HasPrefix(clean, "desktopruntime/") ||
		clean == "app.ico" || clean == "kick_credentials.json" || clean == "kick_user_authorization.json"
}

func copyBackupFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func (a *App) handleBackupExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := "SleepySource_Backup_" + time.Now().Format("20060102_150405") + ".sleepysource"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	zw := zip.NewWriter(w)
	manifest, _ := json.MarshalIndent(backupManifest{Product: "SleepySource", Version: displayVersion, CreatedAt: time.Now().UTC().Format(time.RFC3339)}, "", "  ")
	mh, _ := zw.Create("SleepySource_Backup.json")
	_, _ = mh.Write(manifest)
	walkErr := filepath.WalkDir(a.dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == a.dataDir {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(a.dataDir, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if skipBackupRelative(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(filepath.Join("SleepySource_Data", rel))
		h.Method = zip.Deflate
		entry, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(entry, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if walkErr != nil {
		_ = zw.Close()
		return
	}
	_ = zw.Close()
}

func clearRestorableData(dataDir string) error {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if preserveRestoreRelative(entry.Name()) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dataDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyRestoreTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyBackupFile(path, target)
	})
}

// snapshotRestorableData copies the data that a restore is allowed to replace.
// The snapshot is created before any current data is removed so a failed restore
// can be rolled back instead of leaving the user's setup only partially restored.
func snapshotRestorableData(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if preserveRestoreRelative(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("cannot safely snapshot symbolic link %q", rel)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyBackupFile(path, target)
	})
}

func restoreSnapshot(dataDir, snapshotDir string) error {
	if err := clearRestorableData(dataDir); err != nil {
		return err
	}
	return copyRestoreTree(snapshotDir, dataDir)
}

func (a *App) reloadRestoredData() error {
	a.mu.Lock()
	a.settings = defaultSettings()
	a.customPath = ""
	a.backgroundPath = ""
	a.loadSettings()
	a.updatedAt = time.Now().UnixMilli()
	a.mu.Unlock()
	a.findCustomImage()
	a.findCustomBackground()
	if a.chat != nil {
		a.chat.reloadFromDisk()
	}
	if a.countdown != nil {
		a.countdown.reloadFromDisk()
	}
	return nil
}

func (a *App) handleBackupImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupUploadBytes+(2<<20))
	if err := r.ParseMultipartForm(maxBackupUploadBytes + (2 << 20)); err != nil {
		http.Error(w, "backup file is too large", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("backup")
	if err != nil {
		http.Error(w, "choose a SleepySource backup first", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > maxBackupUploadBytes {
		http.Error(w, "backup file is too large", http.StatusBadRequest)
		return
	}
	tmpZip, err := os.CreateTemp(a.exeDir, ".sleepysource-restore-*.zip")
	if err != nil {
		http.Error(w, "could not prepare restore", http.StatusInternalServerError)
		return
	}
	tmpZipName := tmpZip.Name()
	defer os.Remove(tmpZipName)
	if _, err := io.Copy(tmpZip, io.LimitReader(file, maxBackupUploadBytes)); err != nil {
		_ = tmpZip.Close()
		http.Error(w, "could not read backup", http.StatusBadRequest)
		return
	}
	if err := tmpZip.Close(); err != nil {
		http.Error(w, "could not read backup", http.StatusBadRequest)
		return
	}
	zr, err := zip.OpenReader(tmpZipName)
	if err != nil {
		http.Error(w, "the selected file is not a valid SleepySource backup", http.StatusBadRequest)
		return
	}
	defer zr.Close()
	if len(zr.File) == 0 || len(zr.File) > maxBackupFiles {
		http.Error(w, "backup contains an invalid number of files", http.StatusBadRequest)
		return
	}
	restoreDir, err := os.MkdirTemp(a.exeDir, ".SleepySource_Restore-")
	if err != nil {
		http.Error(w, "could not prepare restore folder", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(restoreDir)
	var total int64
	foundData := false
	for _, zf := range zr.File {
		clean, ok := cleanBackupRelative(zf.Name)
		if !ok {
			http.Error(w, "backup contains an unsafe file path", http.StatusBadRequest)
			return
		}
		parts := strings.Split(filepath.ToSlash(clean), "/")
		if len(parts) == 1 && strings.EqualFold(parts[0], "SleepySource_Backup.json") {
			continue
		}
		if len(parts) < 2 || !strings.EqualFold(parts[0], "SleepySource_Data") {
			continue
		}
		rel := filepath.Join(parts[1:]...)
		if rel == "." || rel == "" {
			continue
		}
		// Legacy Alerts/KICK-alert data from older backups is deliberately ignored.
		// Current Now Playing, Chat Overlay, and Countdown Pro settings remain part of the backup.
		if skipBackupRelative(rel) {
			continue
		}
		foundData = true
		if zf.UncompressedSize64 > uint64(maxBackupExtractBytes) {
			http.Error(w, "backup contains an oversized file", http.StatusBadRequest)
			return
		}
		total += int64(zf.UncompressedSize64)
		if total > maxBackupExtractBytes {
			http.Error(w, "backup expands beyond the allowed size", http.StatusBadRequest)
			return
		}
		target := filepath.Join(restoreDir, rel)
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				http.Error(w, "could not extract backup", http.StatusInternalServerError)
				return
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			http.Error(w, "could not extract backup", http.StatusInternalServerError)
			return
		}
		rc, err := zf.Open()
		if err != nil {
			http.Error(w, "could not extract backup", http.StatusBadRequest)
			return
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			_ = rc.Close()
			http.Error(w, "could not extract backup", http.StatusInternalServerError)
			return
		}
		_, copyErr := io.Copy(out, io.LimitReader(rc, int64(zf.UncompressedSize64)+1))
		closeOutErr := out.Close()
		closeInErr := rc.Close()
		if copyErr != nil || closeOutErr != nil || closeInErr != nil {
			http.Error(w, "could not extract backup", http.StatusBadRequest)
			return
		}
	}
	if !foundData {
		http.Error(w, "backup does not contain SleepySource_Data", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(filepath.Join(restoreDir, "settings.json")); os.IsNotExist(err) {
		http.Error(w, "backup does not contain SleepySource settings", http.StatusBadRequest)
		return
	}
	// WebView2 keeps DesktopRuntime files open while SleepySource is running,
	// and saved Kick credentials are intentionally local-only. Preserve those
	// items while replacing the portable settings/media from the backup.
	//
	// Before replacing anything, take a rollback snapshot. A disk-full error,
	// antivirus/file-lock conflict, or other mid-copy failure must not leave a
	// public build with only half of the user's previous setup remaining.
	rollbackDir, err := os.MkdirTemp(a.exeDir, ".SleepySource_Rollback-")
	if err != nil {
		http.Error(w, "could not prepare restore rollback", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(rollbackDir)
	if err := snapshotRestorableData(a.dataDir, rollbackDir); err != nil {
		http.Error(w, "could not prepare a safe restore rollback", http.StatusInternalServerError)
		return
	}
	rollback := func() error {
		if err := restoreSnapshot(a.dataDir, rollbackDir); err != nil {
			return err
		}
		return a.reloadRestoredData()
	}
	if err := clearRestorableData(a.dataDir); err != nil {
		if rollbackErr := rollback(); rollbackErr != nil {
			http.Error(w, "restore could not replace current data and automatic rollback also failed", http.StatusInternalServerError)
			return
		}
		http.Error(w, "could not replace current data; the previous setup was restored", http.StatusInternalServerError)
		return
	}
	if err := copyRestoreTree(restoreDir, a.dataDir); err != nil {
		if rollbackErr := rollback(); rollbackErr != nil {
			http.Error(w, "backup restore failed and automatic rollback also failed", http.StatusInternalServerError)
			return
		}
		http.Error(w, "backup restore failed; the previous setup was restored", http.StatusInternalServerError)
		return
	}
	if err := a.reloadRestoredData(); err != nil {
		if rollbackErr := rollback(); rollbackErr != nil {
			http.Error(w, "backup files could not be reloaded and automatic rollback also failed", http.StatusInternalServerError)
			return
		}
		http.Error(w, "backup files could not be reloaded; the previous setup was restored", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": "SleepySource backup restored"})
}
