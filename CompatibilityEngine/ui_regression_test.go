package main

import (
	"os"
	"strings"
	"testing"
)

func TestBetaSidebarStatusAndThemeIconRegression(t *testing.T) {
	data, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	checks := []string{
		".homeSidebarHint{display:none}",
		".homeGlanceStatusCard",
		`id="homeGlanceKickCard"`,
		`id="homeGlanceRelayCard"`,
		`id="homeGlanceNowCard"`,
		`id="homeGlanceKickMessage"`,
		`id="homeGlanceRelayMessage"`,
		`id="homeGlanceNowMessage"`,
		`.homeStatusDot.error`,
		`Connect your Kick account in Connections.`,
		`Ready to start your public webhook relay.`,
		`No media is currently playing.`,
		`Playback is paused or stopped.`,
		`src="/assets/nav-theme.svg"`,
		`id="homeGlanceKickValue"`,
		`id="homeGlanceRelayValue"`,
		`id="homeGlanceNowValue"`,
		`for(const sel of ['#chatCopyUrlBtn','#chatCopyUrlMainBtn'])`,
		`id="relayNotice"`,
		`.relayNotice[data-variant="success"]`,
		`.relayNotice[data-variant="warning"]`,
		`.relayNotice[data-variant="error"]`,
		`function relayNoticeForMessage(message)`,
		`relay|cloudflare|webhook`,
		`Relay Connected`,
		`Public webhook is ready`,
		`function renderRelayInlineStatus(st)`,
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Fatalf("required Beta UI regression guard missing: %q", want)
		}
	}
	if strings.Contains(html, `<h3>System status</h3>`) {
		t.Fatal("system status section should be removed from Home")
	}
	if strings.Contains(html, `Open System Health →`) {
		t.Fatal("home open system health shortcut should be removed")
	}
	if strings.Contains(html, `id="cfTunnelStatus"`) {
		t.Fatal("connections relay inline card should be removed")
	}
	if strings.Contains(html, `Native Windows media-session detector connected.`) {
		t.Fatal("detector status text should not appear in the Home sidebar card")
	}
	if strings.Contains(html, `<span class="moduleIcon" aria-hidden="true">🎨</span>`) {
		t.Fatal("colored theme emoji returned; monochrome theme icon is required")
	}
	if _, err := os.Stat("assets/nav-theme.svg"); err != nil {
		t.Fatalf("monochrome theme asset missing: %v", err)
	}
}
