package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseQuickTunnelURL(t *testing.T) {
	cases := map[string]string{
		"INF Your quick Tunnel has been created! Visit it at https://sleepy-blue-raccoon.trycloudflare.com": "https://sleepy-blue-raccoon.trycloudflare.com",
		"https://ABC-123.trycloudflare.com/path":                                                            "https://abc-123.trycloudflare.com",
		"no tunnel here":                                                                                    "",
	}
	for in, want := range cases {
		if got := parseQuickTunnelURL(in); got != want {
			t.Fatalf("parseQuickTunnelURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCloudflaredPinnedMetadata(t *testing.T) {
	const wantVersion = "2026.7.3"
	const wantURL = "https://github.com/cloudflare/cloudflared/releases/download/2026.7.3/cloudflared-windows-amd64.exe"
	const wantSHA256 = "8635da433b6df8194746e88ed9d2589566c20e38bfc2a80e431a348b7c765841"
	if bundledCloudflaredVersion != wantVersion || bundledCloudflaredURL != wantURL || bundledCloudflaredSHA256 != wantSHA256 || maxCloudflaredDownload < 50<<20 {
		t.Fatal("pinned cloudflared metadata is incomplete or unexpected")
	}
	if quickTunnelOrigin != "http://127.0.0.1:17891" {
		t.Fatalf("unexpected quick tunnel origin %q", quickTunnelOrigin)
	}
}

func TestRuntimeFileAvailableUsesCheapFileCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cloudflared.exe")
	if runtimeFileAvailable(path) {
		t.Fatal("missing runtime should not be reported ready")
	}
	if err := os.WriteFile(path, []byte("runtime"), 0600); err != nil {
		t.Fatal(err)
	}
	if !runtimeFileAvailable(path) {
		t.Fatal("non-empty runtime file should be reported available")
	}
}

func TestCloudflareStopClearsPublishedState(t *testing.T) {
	m := &CloudflareTunnelManager{running: true, startedAt: 123, publicURL: "https://old.trycloudflare.com"}
	m.stop()
	state := m.state()
	if state.Running || state.StartedAt != 0 || state.PublicURL != "" || state.WebhookURL != "" {
		t.Fatalf("stale relay state after stop: %+v", state)
	}
}
