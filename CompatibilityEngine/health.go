package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type HealthCheck struct {
	ID           string `json:"id"`
	Group        string `json:"group"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	RepairAction string `json:"repair_action,omitempty"`
	RepairLabel  string `json:"repair_label,omitempty"`
}

type HealthReport struct {
	Version       string        `json:"version"`
	CheckedAt     int64         `json:"checked_at"`
	OverallStatus string        `json:"overall_status"`
	Summary       string        `json:"summary"`
	Checks        []HealthCheck `json:"checks"`
}

func healthCheck(id, group, name, status, message string) HealthCheck {
	return HealthCheck{ID: id, Group: group, Name: name, Status: status, Message: message}
}

func healthCheckRepair(id, group, name, status, message, action, label string) HealthCheck {
	c := healthCheck(id, group, name, status, message)
	c.RepairAction = action
	c.RepairLabel = label
	return c
}

func (a *App) runHealthCheck() HealthReport {
	checks := make([]HealthCheck, 0, 18)

	checks = append(checks, healthCheck("local-server", "Core System", "Local Server", "pass", "SleepySource is responding on 127.0.0.1:17891."))

	if err := testWritableDirectory(a.dataDir); err != nil {
		checks = append(checks, healthCheck("data-storage", "Core System", "Settings & Data Storage", "fail", "SleepySource cannot write to its data folder: "+err.Error()))
	} else {
		checks = append(checks, healthCheck("data-storage", "Core System", "Settings & Data Storage", "pass", "Settings and portable data storage are writable."))
	}

	if err := testWritableDirectory(a.customDir); err != nil {
		checks = append(checks, healthCheck("media-storage", "Core System", "Media Storage", "fail", "SleepySource cannot write to its custom media folder: "+err.Error()))
	} else {
		checks = append(checks, healthCheck("media-storage", "Core System", "Media Storage", "pass", "Custom artwork, backgrounds, and uploaded media storage are writable."))
	}

	if a.alerts != nil {
		if err := testWritableDirectory(a.alerts.mediaDir); err != nil {
			checks = append(checks, healthCheck("alert-media-storage", "Alert Studio", "Alert Media Storage", "fail", "SleepySource cannot write to its Alert Studio media folder: "+err.Error()))
		} else {
			checks = append(checks, healthCheck("alert-media-storage", "Alert Studio", "Alert Media Storage", "pass", "Alert Studio image, video, and sound storage is writable."))
		}
	}

	if err := testWritableDirectory(a.profileDir); err != nil {
		checks = append(checks, healthCheck("profiles", "Profiles", "Profile Storage", "fail", "SleepySource cannot write to its profile folder: "+err.Error()))
	} else {
		a.mu.RLock()
		defaultProfile := strings.TrimSpace(a.settings.DefaultProfile)
		a.mu.RUnlock()
		if defaultProfile == "" {
			checks = append(checks, healthCheck("profiles", "Profiles", "Profile Storage", "pass", "Profile storage is writable; no startup default profile is set."))
		} else if _, err := os.Stat(filepath.Join(a.profileDir, defaultProfile, "profile.json")); err != nil {
			checks = append(checks, healthCheck("profiles", "Profiles", "Default Profile", "warn", "The selected startup default profile could not be found: "+defaultProfile+"."))
		} else {
			checks = append(checks, healthCheck("profiles", "Profiles", "Default Profile", "pass", "Startup default profile is ready: "+defaultProfile+"."))
		}
	}

	a.mu.RLock()
	detector := strings.TrimSpace(a.detectorStatus)
	track := a.track
	a.mu.RUnlock()
	if containsHealthError(detector) {
		checks = append(checks, healthCheck("now-playing", "Core System", "Now Playing Detector", "warn", detector))
	} else if track.Found {
		label := strings.TrimSpace(strings.TrimSpace(track.Artist + " — " + track.Title))
		label = strings.Trim(label, " —")
		checks = append(checks, healthCheck("now-playing", "Core System", "Now Playing Detector", "pass", "Media detection is active"+healthSuffix(label)+"."))
	} else {
		checks = append(checks, healthCheck("now-playing", "Core System", "Now Playing Detector", "pass", "Media detector is running; nothing is currently playing."))
	}

	for _, route := range []struct {
		id, name, path string
	}{
		{"overlay-now-playing", "Now Playing Overlay", "/overlay"},
		{"overlay-chat", "Chat Overlay", "/chat"},
		{"overlay-alerts", "Alert Studio Overlay", "/alerts"},
		{"overlay-countdown", "Countdown Pro Overlay", "/countdown"},
	} {
		if err := probeLocalRoute(route.path); err != nil {
			checks = append(checks, healthCheck(route.id, "OBS Overlays", route.name, "fail", err.Error()))
		} else {
			checks = append(checks, healthCheck(route.id, "OBS Overlays", route.name, "pass", route.path+" is responding normally."))
		}
	}

	chatState := ChatState{}
	if a.chat != nil {
		chatState = a.chat.state()
	}
	if chatState.AuthReady && strings.TrimSpace(chatState.ConnectedChannel) != "" {
		checks = append(checks, healthCheck("kick-auth", "Kick Connection", "Kick Authentication", "pass", "Connected to @"+chatState.ConnectedChannel+"."))
	} else if chatState.AuthReady {
		checks = append(checks, healthCheckRepair("kick-auth", "Kick Connection", "Kick Authentication", "warn", "Kick credentials are available, but no channel is connected.", "open-connections", "Open Connections"))
	} else {
		checks = append(checks, healthCheckRepair("kick-auth", "Kick Connection", "Kick Authentication", "warn", "Kick is not connected. This only affects Kick-powered features.", "open-connections", "Connect Kick"))
	}

	if strings.TrimSpace(chatState.ConnectedChannel) == "" {
		checks = append(checks, healthCheck("kick-webhook", "Kick Connection", "Kick Webhook Subscription", "info", "Connect a Kick channel to enable official chat and Alert Studio events."))
	} else if chatState.WebhookSubscribed {
		checks = append(checks, healthCheck("kick-webhook", "Kick Connection", "Kick Webhook Subscription", "pass", "Official Kick chat and Alert Studio events are registered."))
	} else {
		msg := "The connected Kick channel does not currently have confirmed chat and Alert Studio webhook subscriptions."
		if strings.TrimSpace(chatState.WebhookLastError) != "" {
			msg += " Last error: " + chatState.WebhookLastError
		}
		checks = append(checks, healthCheckRepair("kick-webhook", "Kick Connection", "Kick Webhook Subscription", "fail", msg, "reregister-kick", "Repair Subscription"))
	}

	if chatState.WebhookRequestCount == 0 {
		checks = append(checks, healthCheck("kick-activity", "Kick Connection", "Webhook Activity", "info", "No webhook requests have been received yet. Send a Kick chat message or trigger a supported alert event after connecting to verify delivery."))
	} else {
		message := fmt.Sprintf("%d requests • %d verified • %d accepted • %d rejected.", chatState.WebhookRequestCount, chatState.WebhookVerifiedCount, chatState.WebhookAcceptedCount, chatState.WebhookRejectedCount)
		status := "pass"
		if chatState.WebhookRejectedCount > 0 && chatState.WebhookAcceptedCount == 0 {
			status = "warn"
		}
		checks = append(checks, healthCheck("kick-activity", "Kick Connection", "Webhook Activity", status, message))
	}

	relayState := cloudflareTunnelState{}
	if a.cloudflare != nil {
		relayState = a.cloudflare.state()
	}
	if relayState.RuntimeReady {
		checks = append(checks, healthCheck("relay-runtime", "Relay", "Cloudflare Runtime", "pass", "Managed Cloudflare runtime "+relayState.RuntimeVersion+" is ready."))
	} else if relayState.Running {
		checks = append(checks, healthCheck("relay-runtime", "Relay", "Cloudflare Runtime", "warn", "Relay is starting, but the managed runtime is not yet reported ready."))
	} else {
		checks = append(checks, healthCheck("relay-runtime", "Relay", "Cloudflare Runtime", "info", "Managed Cloudflare runtime will be downloaded and verified automatically when the relay starts."))
	}

	if relayState.Running && strings.TrimSpace(relayState.PublicURL) != "" {
		checks = append(checks, healthCheck("relay-process", "Relay", "Relay Process", "pass", "Cloudflare Quick Tunnel is running and has a public URL."))
		if err := probeRelayURL(relayState.PublicURL); err != nil {
			checks = append(checks, healthCheckRepair("relay-end-to-end", "Relay", "End-to-End Tunnel", "warn", "The relay is running, but the public probe did not reach SleepySource: "+err.Error(), "restart-relay", "Restart Relay"))
		} else {
			checks = append(checks, healthCheck("relay-end-to-end", "Relay", "End-to-End Tunnel", "pass", "The public Cloudflare URL reaches this SleepySource instance."))
		}
	} else if relayState.Running {
		message := "Relay process is running but is still waiting for a public URL."
		if relayState.LastError != "" {
			message = "Relay needs attention: " + relayState.LastError
		}
		checks = append(checks, healthCheckRepair("relay-process", "Relay", "Relay Process", "warn", message, "restart-relay", "Restart Relay"))
	} else if relayState.LastError != "" {
		checks = append(checks, healthCheckRepair("relay-process", "Relay", "Relay Process", "warn", "Relay is stopped. Last error: "+relayState.LastError, "start-relay", "Start Relay"))
	} else {
		checks = append(checks, healthCheckRepair("relay-process", "Relay", "Relay Process", "info", "Relay is stopped. Start it when Kick webhook delivery is needed.", "start-relay", "Start Relay"))
	}

	overall := "healthy"
	for _, c := range checks {
		if c.Status == "fail" {
			overall = "problem"
			break
		}
		if c.Status == "warn" {
			overall = "attention"
		}
	}
	summary := "All checked systems are ready."
	if overall == "attention" {
		summary = "SleepySource is running, but one or more items need attention."
	} else if overall == "problem" {
		summary = "One or more checks failed and may affect streaming features."
	}
	return HealthReport{Version: displayVersion, CheckedAt: time.Now().UnixMilli(), OverallStatus: overall, Summary: summary, Checks: checks}
}

func containsHealthError(value string) bool {
	value = strings.ToLower(value)
	for _, word := range []string{"could not", "missing", "unavailable", "failed", "error"} {
		if strings.Contains(value, word) {
			return true
		}
	}
	return false
}

func healthSuffix(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return ": " + value
}

func testWritableDirectory(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("data directory is not configured")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".sleepysource-health-*")
	if err != nil {
		return err
	}
	name := f.Name()
	if _, err := f.WriteString("ok"); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func probeLocalRoute(path string) error {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	req, err := http.NewRequest(http.MethodHead, "http://"+listenAddr+path, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s did not respond: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("%s returned HTTP %d", path, resp.StatusCode)
	}
	return nil
}

func probeRelayURL(publicURL string) error {
	publicURL = strings.TrimSpace(strings.TrimSuffix(publicURL, "/"))
	if !quickTunnelURLPattern.MatchString(publicURL) {
		return fmt.Errorf("public relay URL is not a recognized Quick Tunnel address")
	}
	client := &http.Client{Timeout: 4 * time.Second}
	req, err := http.NewRequest(http.MethodGet, publicURL+"/api/relay-health", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "SleepySource/"+appVersion+" health-check")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("public probe returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (a *App) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, a.runHealthCheck())
}

func (a *App) handleRelayHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte("ok"))
	}
}
