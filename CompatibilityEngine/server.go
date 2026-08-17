package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	webFS, _ := fs.Sub(embedded, "web")
	assetFS, _ := fs.Sub(embedded, "assets")
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assetFS))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		serveEmbeddedFile(w, webFS, "enter.html", "text/html; charset=utf-8")
	})
	mux.HandleFunc("/designer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		serveEmbeddedFile(w, webFS, "index.html", "text/html; charset=utf-8")
	})
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		serveEmbeddedFile(w, webFS, "chat.html", "text/html; charset=utf-8")
	})
	mux.HandleFunc("/overlay", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		serveEmbeddedFile(w, webFS, "overlay.html", "text/html; charset=utf-8")
	})
	mux.HandleFunc("/countdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		serveEmbeddedFile(w, webFS, "countdown.html", "text/html; charset=utf-8")
	})
	mux.HandleFunc("/alerts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		serveEmbeddedFile(w, webFS, "alerts.html", "text/html; charset=utf-8")
	})
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/settings", a.handleSettings)
	mux.HandleFunc("/api/export-settings", a.handleExportSettings)
	mux.HandleFunc("/api/import-settings", a.handleImportSettings)
	mux.HandleFunc("/api/backup/export", a.handleBackupExport)
	mux.HandleFunc("/api/backup/import", a.handleBackupImport)
	mux.HandleFunc("/api/upload", a.handleUpload)
	mux.HandleFunc("/api/remove-image", a.handleRemoveImage)
	mux.HandleFunc("/api/upload-background", a.handleUploadBackground)
	mux.HandleFunc("/api/remove-background", a.handleRemoveBackground)
	mux.HandleFunc("/api/upload-font", a.handleUploadFont)
	mux.HandleFunc("/api/remove-font", a.handleRemoveFont)
	mux.HandleFunc("/font", a.handleFont)
	mux.HandleFunc("/media/custom", a.handleCustomImage)
	mux.HandleFunc("/media/background", a.handleCustomBackground)
	mux.HandleFunc("/api/open-folder", a.handleOpenFolder)
	mux.HandleFunc("/api/update", a.handleUpdateCheck)
	mux.HandleFunc("/api/update/open", a.handleUpdateOpen)
	mux.HandleFunc("/api/overlay-metrics", a.handleOverlayMetrics)
	mux.HandleFunc("/api/profiles", a.handleProfiles)
	mux.HandleFunc("/api/profile-export", a.handleProfileExport)
	mux.HandleFunc("/api/profile-import", a.handleProfileImport)
	mux.HandleFunc("/api/countdown/state", a.handleCountdownState)
	mux.HandleFunc("/api/countdown/settings", a.handleCountdownSettings)
	mux.HandleFunc("/api/countdown/control", a.handleCountdownControl)
	mux.HandleFunc("/api/countdown/profiles", a.handleCountdownProfiles)
	mux.HandleFunc("/api/countdown/upload-font", a.handleCountdownUploadFont)
	mux.HandleFunc("/api/alerts/state", a.handleAlertState)
	mux.HandleFunc("/api/alerts/settings", a.handleAlertSettings)
	mux.HandleFunc("/api/alerts/test", a.handleAlertTest)
	mux.HandleFunc("/api/alerts/control", a.handleAlertControl)
	mux.HandleFunc("/api/alerts/upload", a.handleAlertUpload)
	mux.HandleFunc("/api/alerts/remove-media", a.handleAlertRemoveMedia)
	mux.HandleFunc("/media/alerts", a.handleAlertMedia)
	mux.HandleFunc("/api/chat/state", a.handleChatState)
	mux.HandleFunc("/api/chat/settings", a.handleChatSettings)
	mux.HandleFunc("/api/chat/upload-font", a.handleChatUploadFont)
	mux.HandleFunc("/api/chat/test", a.handleChatTest)
	mux.HandleFunc("/api/chat/clear", a.handleChatClear)
	mux.HandleFunc("/api/chat/ingest", a.handleChatIngest)
	mux.HandleFunc("/api/chat/kick-webhook", a.handleKickWebhook)
	mux.HandleFunc("/api/chat/auth", a.handleChatAuth)
	mux.HandleFunc("/api/chat/connect", a.handleKickConnect)
	mux.HandleFunc("/api/chat/reregister", a.handleKickReregister)
	mux.HandleFunc("/api/chat/channel", a.handleKickChannelLookup)
	mux.HandleFunc("/api/stream/metadata", a.handleKickStreamMetadata)
	mux.HandleFunc("/api/stream/categories", a.handleKickStreamCategories)
	mux.HandleFunc("/api/stream/update", a.handleKickStreamUpdate)
	mux.HandleFunc("/api/stream/auth/status", a.handleKickUserAuthStatus)
	mux.HandleFunc("/api/stream/auth/start", a.handleKickUserAuthStart)
	mux.HandleFunc("/api/stream/auth/disconnect", a.handleKickUserAuthDisconnect)
	mux.HandleFunc("/oauth/kick/callback", a.handleKickUserAuthCallback)
	mux.HandleFunc("/api/chat/7tv", a.handleSevenTV)
	mux.HandleFunc("/api/chat/7tv-image", a.handleSevenTVImage)
	mux.HandleFunc("/api/chat/kick-emote", a.handleKickEmoteImage)
	mux.HandleFunc("/api/chat/avatar", a.handleChatAvatar)
	mux.HandleFunc("/api/chat/badges", a.handleKickBadgeCatalog)
	mux.HandleFunc("/api/chat/badge-image", a.handleKickBadgeImage)
	mux.HandleFunc("/api/cloudflare/status", a.handleCloudflareStatus)
	mux.HandleFunc("/api/cloudflare/start", a.handleCloudflareStart)
	mux.HandleFunc("/api/cloudflare/stop", a.handleCloudflareStop)
	mux.HandleFunc("/api/system/health", a.handleSystemHealth)
	mux.HandleFunc("/api/relay-health", a.handleRelayHealth)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok")
	})

	return securityHeaders(mux)
}

func isLocalHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func requestOriginAllowed(r *http.Request) bool {
	// Kick must call this route through a public HTTPS URL/tunnel. The handler
	// verifies Kick's RSA signature before accepting any payload. The relay-health
	// route is a deliberately content-free GET/HEAD probe used only to confirm that
	// an active Quick Tunnel reaches this local SleepySource instance end to end.
	if r.URL.Path == "/api/chat/kick-webhook" && r.Method == http.MethodPost {
		return true
	}
	if r.URL.Path == "/api/relay-health" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		return true
	}
	if !isLocalHost(r.Host) {
		return false
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Native/local clients such as curl do not always send Origin. The TCP
		// listener is loopback-only, so those requests are still local.
		return true
	}
	if origin == "null" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || !isLocalHost(u.Host) {
		return false
	}
	port := u.Port()
	return port == "17891"
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requestOriginAllowed(r) {
			http.Error(w, "local SleepySource request required", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; font-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'self'")

		path := r.URL.Path
		switch {
		case strings.HasPrefix(path, "/api/"), path == "/", path == "/designer", path == "/overlay", path == "/chat", path == "/countdown", path == "/alerts":
			w.Header().Set("Cache-Control", "no-store")
		case strings.HasPrefix(path, "/assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasPrefix(path, "/media/"), path == "/font":
			if r.URL.Query().Get("v") != "" {
				w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "private, max-age=60")
			}
		default:
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func serveEmbeddedFile(w http.ResponseWriter, f fs.FS, name, contentType string) {
	data, err := fs.ReadFile(f, name)
	if err != nil {
		http.Error(w, "embedded file missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, a.snapshot())
}

func decodeSingleJSON(r io.Reader, dst any, maxBytes int64) error {
	dec := json.NewDecoder(io.LimitReader(r, maxBytes))
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected data after JSON document")
		}
		return err
	}
	return nil
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, a.snapshot())
	case http.MethodPost:
		a.mu.RLock()
		next := a.settings
		a.mu.RUnlock()
		if err := decodeSingleJSON(r.Body, &next, 1<<20); err != nil {
			http.Error(w, "invalid settings", http.StatusBadRequest)
			return
		}
		normalizeSettings(&next)
		a.mu.Lock()
		// Uploaded media filenames are managed only by their upload/remove endpoints.
		next.CustomImage = a.settings.CustomImage
		next.CustomBackground = a.settings.CustomBackground
		a.settings = next
		a.updatedAt = time.Now().UnixMilli()
		err := a.saveSettingsLocked()
		current := a.track
		a.mu.Unlock()
		if err != nil {
			log.Printf("settings save failed: %v", err)
			http.Error(w, "could not save settings", http.StatusInternalServerError)
			return
		}
		a.writeOutput(current)
		writeJSON(w, a.snapshot())
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleExportSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.mu.RLock()
	s := a.settings
	a.mu.RUnlock()
	normalizeSettings(&s)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		http.Error(w, "could not export settings", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="SleepySource_Settings.json"`)
	_, _ = w.Write(data)
}

func (a *App) handleImportSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		http.Error(w, "settings file is too large", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, _, err := r.FormFile("settings")
	if err != nil {
		http.Error(w, "choose a settings JSON file first", http.StatusBadRequest)
		return
	}
	defer file.Close()
	next := defaultSettings()
	data, err := io.ReadAll(io.LimitReader(file, 1<<20))
	if err != nil || json.Unmarshal(data, &next) != nil {
		http.Error(w, "invalid settings JSON", http.StatusBadRequest)
		return
	}
	normalizeSettings(&next)
	a.mu.Lock()
	// Layout presets should not orphan the user's uploaded media files.
	next.CustomImage = a.settings.CustomImage
	next.CustomBackground = a.settings.CustomBackground
	a.settings = next
	a.updatedAt = time.Now().UnixMilli()
	err = a.saveSettingsLocked()
	current := a.track
	a.mu.Unlock()
	if err != nil {
		http.Error(w, "settings loaded, but could not be saved", http.StatusInternalServerError)
		return
	}
	a.writeOutput(current)
	writeJSON(w, a.snapshot())
}

func (a *App) handleOverlayMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var m struct {
		FPS     float64 `json:"fps"`
		FrameMS float64 `json:"frame_ms"`
	}
	if err := decodeSingleJSON(r.Body, &m, 16<<10); err != nil {
		http.Error(w, "invalid metrics", http.StatusBadRequest)
		return
	}
	if m.FPS < 0 {
		m.FPS = 0
	}
	if m.FPS > 1000 {
		m.FPS = 1000
	}
	if m.FrameMS < 0 {
		m.FrameMS = 0
	}
	if m.FrameMS > 10000 {
		m.FrameMS = 10000
	}
	a.mu.Lock()
	a.overlayFPS = m.FPS
	a.overlayFrameMS = m.FrameMS
	a.overlayMetricsAt = time.Now().UnixMilli()
	a.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func (a *App) startServer() error {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	a.server = &http.Server{
		Addr:              listenAddr,
		Handler:           a.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		err := a.server.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server: %v", err)
		}
	}()
	return nil
}
