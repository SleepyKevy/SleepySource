package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	bundledCloudflaredVersion = "2026.7.3"
	bundledCloudflaredURL     = "https://github.com/cloudflare/cloudflared/releases/download/2026.7.3/cloudflared-windows-amd64.exe"
	bundledCloudflaredSHA256  = "8635da433b6df8194746e88ed9d2589566c20e38bfc2a80e431a348b7c765841"
	maxCloudflaredDownload    = int64(100 << 20)
	quickTunnelOrigin         = "http://127.0.0.1:17891"
)

var quickTunnelURLPattern = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

type CloudflareTunnelManager struct {
	mu        sync.Mutex
	exeDir    string
	cmd       *exec.Cmd
	running   bool
	startedAt int64
	lastError string
	publicURL string
	runtime   string
}

type cloudflareTunnelState struct {
	Running        bool   `json:"running"`
	StartedAt      int64  `json:"started_at,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	PublicURL      string `json:"public_url,omitempty"`
	WebhookURL     string `json:"webhook_url,omitempty"`
	Binary         string `json:"binary,omitempty"`
	Integrated     bool   `json:"integrated"`
	RuntimeReady   bool   `json:"runtime_ready"`
	RuntimeVersion string `json:"runtime_version"`
	Mode           string `json:"mode"`
	NeedsKickSetup bool   `json:"needs_kick_setup"`
}

func newCloudflareTunnelManager(exeDir string) *CloudflareTunnelManager {
	m := &CloudflareTunnelManager{exeDir: exeDir}
	if managed := cloudflaredRuntimePath(); validCloudflaredRuntime(managed) {
		m.runtime = managed
	}
	return m
}

func cloudflaredRuntimeDir() string {
	base, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(base) == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "SleepySource", "runtime")
}

func cloudflaredRuntimePath() string {
	return filepath.Join(cloudflaredRuntimeDir(), "cloudflared-"+bundledCloudflaredVersion+"-windows-amd64.exe")
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func validCloudflaredRuntime(path string) bool {
	sum, size, err := fileSHA256(path)
	return err == nil && size > 0 && size <= maxCloudflaredDownload && strings.EqualFold(sum, bundledCloudflaredSHA256)
}

func ensureManagedCloudflared() (string, error) {
	dest := cloudflaredRuntimePath()
	if validCloudflaredRuntime(dest) {
		return dest, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return "", fmt.Errorf("could not create secure relay runtime folder: %w", err)
	}

	tmp := dest + ".download"
	_ = os.Remove(tmp)

	client := &http.Client{Timeout: 3 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, bundledCloudflaredURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not download the managed secure relay runtime: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("secure relay runtime download returned HTTP %d", resp.StatusCode)
	}

	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return "", fmt.Errorf("could not create secure relay runtime: %w", err)
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, maxCloudflaredDownload+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("could not save secure relay runtime: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("could not finish secure relay runtime download: %w", closeErr)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if n <= 0 || n > maxCloudflaredDownload || !strings.EqualFold(sum, bundledCloudflaredSHA256) {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("secure relay runtime verification failed; expected the official Cloudflare %s Windows x64 build", bundledCloudflaredVersion)
	}

	_ = os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("could not activate secure relay runtime: %w", err)
	}
	return dest, nil
}

func parseQuickTunnelURL(line string) string {
	return quickTunnelURLPattern.FindString(strings.ToLower(strings.TrimSpace(line)))
}

func replaceEnv(env []string, key, value string) []string {
	prefix := strings.ToLower(key) + "="
	out := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(strings.ToLower(item), prefix) {
			continue
		}
		out = append(out, item)
	}
	return append(out, key+"="+value)
}

func scanTunnelOutput(r io.Reader, lines chan<- string) {
	s := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 512*1024)
	for s.Scan() {
		select {
		case lines <- s.Text():
		default:
		}
	}
}

func runtimeFileAvailable(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0 && info.Size() <= maxCloudflaredDownload
}

func (m *CloudflareTunnelManager) state() cloudflareTunnelState {
	m.mu.Lock()
	defer m.mu.Unlock()
	bin := m.runtime
	// The runtime is SHA-256 verified when it is discovered/downloaded and again
	// before every tunnel start. Status is polled frequently by the UI, so only
	// use a cheap existence/size check here instead of re-hashing the executable.
	ready := runtimeFileAvailable(bin)
	s := cloudflareTunnelState{
		Running: m.running, StartedAt: m.startedAt, LastError: m.lastError,
		PublicURL: m.publicURL, Binary: bin, Integrated: true,
		RuntimeReady: ready, RuntimeVersion: bundledCloudflaredVersion,
		Mode: "quick", NeedsKickSetup: true,
	}
	if m.publicURL != "" {
		s.WebhookURL = strings.TrimSuffix(m.publicURL, "/") + "/api/chat/kick-webhook"
	}
	return s
}

func (m *CloudflareTunnelManager) start() error {
	m.mu.Lock()
	if m.running && m.publicURL != "" {
		m.mu.Unlock()
		return nil
	}
	if m.running && m.cmd != nil {
		cmd := m.cmd
		m.cmd = nil
		m.running = false
		m.mu.Unlock()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	} else {
		m.mu.Unlock()
	}

	managed := cloudflaredRuntimePath()
	bin := ""
	if validCloudflaredRuntime(managed) {
		bin = managed
	} else {
		var err error
		bin, err = ensureManagedCloudflared()
		if err != nil {
			m.mu.Lock()
			m.lastError = err.Error()
			m.mu.Unlock()
			return err
		}
	}

	// Give cloudflared an isolated home so a user's existing .cloudflared/config.yml
	// cannot disable Quick Tunnel mode.
	runtimeHome := filepath.Join(cloudflaredRuntimeDir(), "quick-home")
	if err := os.MkdirAll(runtimeHome, 0700); err != nil {
		return fmt.Errorf("could not prepare secure relay runtime: %w", err)
	}

	cmd := exec.Command(bin, "tunnel", "--no-autoupdate", "--url", quickTunnelOrigin)
	env := os.Environ()
	env = replaceEnv(env, "HOME", runtimeHome)
	env = replaceEnv(env, "USERPROFILE", runtimeHome)
	cmd.Env = env
	prepareBackgroundCommand(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("could not start secure relay output: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("could not start secure relay logging: %w", err)
	}
	if err := cmd.Start(); err != nil {
		m.mu.Lock()
		m.lastError = err.Error()
		m.mu.Unlock()
		return fmt.Errorf("could not start secure relay: %w", err)
	}

	m.mu.Lock()
	m.cmd = cmd
	m.running = true
	m.startedAt = time.Now().UnixMilli()
	m.lastError = ""
	m.publicURL = ""
	m.runtime = bin
	m.mu.Unlock()

	lines := make(chan string, 64)
	go scanTunnelOutput(stdout, lines)
	go scanTunnelOutput(stderr, lines)
	waitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		waitCh <- err
		m.mu.Lock()
		if m.cmd == cmd {
			m.running = false
			m.cmd = nil
			m.publicURL = ""
			m.startedAt = 0
			if err != nil && strings.TrimSpace(m.lastError) == "" {
				m.lastError = err.Error()
			}
		}
		m.mu.Unlock()
	}()

	timer := time.NewTimer(35 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line := <-lines:
			if publicURL := parseQuickTunnelURL(line); publicURL != "" {
				m.mu.Lock()
				if m.cmd == cmd {
					m.publicURL = publicURL
					m.lastError = ""
				}
				m.mu.Unlock()
				return nil
			}
		case err := <-waitCh:
			if err == nil {
				err = fmt.Errorf("secure relay stopped before Cloudflare assigned a public URL")
			}
			m.mu.Lock()
			m.running = false
			m.cmd = nil
			m.lastError = err.Error()
			m.mu.Unlock()
			return fmt.Errorf("secure relay could not start: %w", err)
		case <-timer.C:
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			err := fmt.Errorf("secure relay timed out while waiting for a temporary public URL")
			m.mu.Lock()
			m.running = false
			m.cmd = nil
			m.lastError = err.Error()
			m.mu.Unlock()
			return err
		}
	}
}

func (m *CloudflareTunnelManager) stop() {
	m.mu.Lock()
	cmd := m.cmd
	m.cmd = nil
	m.running = false
	m.publicURL = ""
	m.startedAt = 0
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func (a *App) handleCloudflareStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, a.cloudflare.state())
}

func (a *App) handleCloudflareStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.cloudflare.start(); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, a.cloudflare.state())
}

func (a *App) handleCloudflareStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.cloudflare.stop()
	writeJSON(w, a.cloudflare.state())
}
