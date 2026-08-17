package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func newApp() (*App, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exeDir := filepath.Dir(exe)
	dataDir := filepath.Join(exeDir, "SleepySource_Data")
	mediaDir := filepath.Join(dataDir, "Media")
	// Portable layout: all generated/user data lives in SleepySource_Data
	// beside the EXE. Nothing is written to AppData.
	app := &App{
		settings:       defaultSettings(),
		exeDir:         exeDir,
		dataDir:        dataDir,
		settingsPath:   filepath.Join(dataDir, "settings.json"),
		outputPath:     filepath.Join(dataDir, "now_playing.txt"),
		customDir:      mediaDir,
		fontDir:        filepath.Join(mediaDir, "fonts"),
		profileDir:     filepath.Join(dataDir, "Profiles"),
		updatedAt:      time.Now().UnixMilli(),
		detectorStatus: "Starting Windows media-session detector…",
	}
	for _, dir := range []string{app.dataDir, app.customDir, app.fontDir, app.profileDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("SleepySource could not create its portable data folder %q: %w", dir, err)
		}
	}
	migrateLegacyPortableData(exeDir, app.dataDir, app.customDir)
	app.loadSettings()
	app.loadDefaultProfileAtStartup()
	app.findCustomImage()
	app.findCustomBackground()
	app.chat = newChatManager(app.dataDir)
	app.streamAuth = newKickUserAuthManager(app.dataDir)
	app.cloudflare = newCloudflareTunnelManager(app.exeDir)
	app.countdown = newCountdownManager(app.dataDir)
	app.alerts = newAlertManager(app.dataDir)
	return app, nil
}

func copyFileIfMissing(src, dst string) {
	if _, err := os.Stat(dst); err == nil {
		return
	}
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(dst)
	}
}

func copyDirContentsIfMissing(srcDir, dstDir string) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return
	}
	_ = os.MkdirAll(dstDir, 0755)
	for _, entry := range entries {
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(dstDir, entry.Name())
		if entry.IsDir() {
			copyDirContentsIfMissing(src, dst)
			continue
		}
		copyFileIfMissing(src, dst)
	}
}

func migrateLegacyPortableData(exeDir, dataDir, mediaDir string) {
	// Older portable builds wrote these directly beside the EXE. Copy only files
	// missing from the new folder so existing SleepySource_Data always wins.
	copyFileIfMissing(filepath.Join(exeDir, "settings.json"), filepath.Join(dataDir, "settings.json"))
	copyFileIfMissing(filepath.Join(exeDir, "now_playing.txt"), filepath.Join(dataDir, "now_playing.txt"))
	copyDirContentsIfMissing(filepath.Join(exeDir, "now_playing_assets"), mediaDir)
}
