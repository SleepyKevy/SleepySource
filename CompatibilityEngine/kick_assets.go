package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func kickLookupHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "client id"), strings.Contains(msg, "client secret"), strings.Contains(msg, "connect kick"):
		return http.StatusPreconditionFailed
	case strings.Contains(msg, "required"), strings.Contains(msg, "not found"):
		return http.StatusBadRequest
	case strings.Contains(msg, "rejected"):
		return http.StatusUnauthorized
	default:
		return http.StatusBadGateway
	}
}

func (a *App) handleKickChannelLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings := a.chat.state().Settings
	channel := normalizeKickChannelSlug(r.URL.Query().Get("channel"))
	if channel == "" {
		channel = settings.KickChannel
	}
	token, err := a.chat.ensureKickAccessToken()
	if err != nil {
		http.Error(w, err.Error(), kickLookupHTTPStatus(err))
		return
	}
	userID, slug, err := resolveKickBroadcasterUserID(channel, token)
	if err != nil {
		http.Error(w, err.Error(), kickLookupHTTPStatus(err))
		return
	}
	a.chat.setResolvedChannel(slug, userID)
	writeJSON(w, map[string]any{"channel": slug, "broadcaster_user_id": userID})
}

func fetchSevenTVJSON(endpoint string) (json.RawMessage, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("7TV request failed")
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("7TV returned %s", resp.Status)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("7TV returned invalid JSON")
	}
	return json.RawMessage(data), nil
}

func (a *App) handleSevenTV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := a.chat.state()
	if !state.Settings.SevenTVEnabled {
		writeJSON(w, map[string]any{"enabled": false, "channel": map[string]any{"emotes": []any{}}, "global": map[string]any{"emotes": []any{}}})
		return
	}
	setID := strings.TrimSpace(r.URL.Query().Get("emote_set_id"))
	if setID == "" {
		setID = state.Settings.SevenTVEmoteSetID
	}
	var endpoint string
	if setID != "" {
		endpoint = "https://7tv.io/v3/emote-sets/" + url.PathEscape(setID)
	} else {
		settings := state.Settings
		channel := normalizeKickChannelSlug(r.URL.Query().Get("kick_channel"))
		if channel == "" {
			channel = settings.KickChannel
		}
		userID := strings.TrimSpace(state.BroadcasterUserID)
		if userID == "" || (state.ConnectedChannel != "" && !strings.EqualFold(state.ConnectedChannel, channel)) {
			token, err := a.chat.ensureKickAccessToken()
			if err != nil {
				http.Error(w, err.Error(), kickLookupHTTPStatus(err))
				return
			}
			resolvedID, slug, err := resolveKickBroadcasterUserID(channel, token)
			if err != nil {
				http.Error(w, err.Error(), kickLookupHTTPStatus(err))
				return
			}
			userID = resolvedID
			a.chat.setResolvedChannel(slug, userID)
		}
		endpoint = "https://7tv.io/v3/users/kick/" + url.PathEscape(userID)
	}

	channelData, err := fetchSevenTVJSON(endpoint)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	// 7TV global emotes live in a separate set. Return both payloads and let the
	// overlay merge them, with the channel set taking precedence on name clashes.
	globalData, globalErr := fetchSevenTVJSON("https://7tv.io/v3/emote-sets/global")
	if globalErr != nil {
		globalData = json.RawMessage(`{"emotes":[]}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"enabled": true, "channel": channelData, "global": globalData})
}

func (a *App) handleSevenTVImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if len(id) < 3 || len(id) > 80 {
		http.Error(w, "invalid 7TV emote ID", http.StatusBadRequest)
		return
	}
	for _, ch := range id {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			http.Error(w, "invalid 7TV emote ID", http.StatusBadRequest)
			return
		}
	}
	endpoint := "https://cdn.7tv.app/emote/" + url.PathEscape(id) + "/2x.webp"
	client := &http.Client{Timeout: 8 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "7TV emote request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, "7TV emote returned "+resp.Status, resp.StatusCode)
		return
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "invalid 7TV emote response", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 8<<20))
}

func isValidKickEmoteID(id string) bool {
	id = strings.TrimSpace(id)
	if len(id) < 1 || len(id) > 20 {
		return false
	}
	for _, ch := range id {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (a *App) handleKickEmoteImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !isValidKickEmoteID(id) {
		http.Error(w, "invalid Kick emote ID", http.StatusBadRequest)
		return
	}
	endpoint := "https://files.kick.com/emotes/" + url.PathEscape(id) + "/fullsize"
	if err := proxyImageResponse(w, endpoint, true); err != nil {
		http.Error(w, "Kick emote request failed", http.StatusBadGateway)
	}
}

func normalizeKickBadgeType(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	t = strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(t)
	switch t {
	case "broadcaster", "channel_host", "channel_owner", "owner":
		return "broadcaster"
	case "moderator", "mod":
		return "moderator"
	case "vip":
		return "vip"
	case "og":
		return "og"
	case "founder":
		return "founder"
	case "subscriber", "sub":
		return "subscriber"
	case "sub_gifter", "subgifter", "gifter":
		return "sub_gifter"
	case "verified":
		return "verified"
	case "staff", "kick_staff":
		return "staff"
	case "sidekick":
		return "sidekick"
	default:
		if len(t) > 48 {
			t = t[:48]
		}
		return t
	}
}

type kickSubscriberBadge struct {
	Months int    `json:"months"`
	URL    string `json:"url"`
}

func fetchKickSubscriberBadges(channel string) ([]kickSubscriberBadge, error) {
	channel = normalizeKickChannelSlug(channel)
	if channel == "" {
		return nil, fmt.Errorf("Kick channel is required")
	}
	endpoint := "https://kick.com/api/v2/channels/" + url.PathEscape(channel)
	client := &http.Client{Timeout: 8 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Kick badge catalog request failed")
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Kick badge catalog returned %s", resp.Status)
	}
	var payload struct {
		SubscriberBadges []struct {
			Months     int `json:"months"`
			BadgeImage struct {
				Src string `json:"src"`
			} `json:"badge_image"`
		} `json:"subscriber_badges"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("Kick returned an invalid badge catalog")
	}
	out := make([]kickSubscriberBadge, 0, len(payload.SubscriberBadges))
	for _, b := range payload.SubscriberBadges {
		if b.Months < 1 {
			continue
		}
		if _, ok := isTrustedKickImageURL(b.BadgeImage.Src); !ok {
			continue
		}
		out = append(out, kickSubscriberBadge{Months: b.Months, URL: strings.TrimSpace(b.BadgeImage.Src)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Months < out[j].Months })
	return out, nil
}

func (a *App) handleKickBadgeCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	channel := normalizeKickChannelSlug(r.URL.Query().Get("channel"))
	if channel == "" {
		channel = a.chat.state().Settings.KickChannel
	}
	badges, err := fetchKickSubscriberBadges(channel)
	if err != nil {
		// Role badges still work even if Kick's channel badge catalog is temporarily unavailable.
		writeJSON(w, map[string]any{"channel": channel, "subscriber_badges": []kickSubscriberBadge{}, "warning": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"channel": channel, "subscriber_badges": badges})
}

var kickRoleBadgeFiles = map[string]string{
	"broadcaster": "broadcaster.svg", "moderator": "moderator.svg", "vip": "vip.svg", "og": "og.svg",
	"founder": "founder.svg", "subscriber": "subscriber.svg", "verified": "verified.svg", "staff": "staff.svg",
	"sidekick": "sidekick.svg", "sub_gifter": "subGifter.svg", "sub_gifter_25": "subGifter25.svg",
	"sub_gifter_50": "subGifter50.svg", "sub_gifter_100": "subGifter100.svg", "sub_gifter_200": "subGifter200.svg",
}

func kickRoleBadgeVariant(role string, count int) string {
	role = normalizeKickBadgeType(role)
	if role != "sub_gifter" {
		return role
	}
	switch {
	case count >= 200:
		return "sub_gifter_200"
	case count >= 100:
		return "sub_gifter_100"
	case count >= 50:
		return "sub_gifter_50"
	case count >= 25:
		return "sub_gifter_25"
	default:
		return "sub_gifter"
	}
}

func proxyImageURLAllowed(raw string, trustedKickOnly bool, allowedHosts ...string) bool {
	if trustedKickOnly {
		_, ok := isTrustedKickImageURL(raw)
		return ok
	}
	if len(allowedHosts) == 0 {
		return true
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	for _, allowed := range allowedHosts {
		if host == strings.ToLower(strings.TrimSuffix(strings.TrimSpace(allowed), ".")) {
			return true
		}
	}
	return false
}

func proxyImageResponse(w http.ResponseWriter, endpoint string, trustedKickOnly bool, allowedHosts ...string) error {
	if !proxyImageURLAllowed(endpoint, trustedKickOnly, allowedHosts...) {
		return fmt.Errorf("untrusted image URL")
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return http.ErrUseLastResponse
		}
		if !proxyImageURLAllowed(req.URL.String(), trustedKickOnly, allowedHosts...) {
			return fmt.Errorf("untrusted redirect")
		}
		return nil
	}}
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("badge image returned %s", resp.Status)
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.HasPrefix(ct, "image/") {
		return fmt.Errorf("invalid badge image response")
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, err = io.Copy(w, io.LimitReader(resp.Body, 2<<20))
	return err
}

func (a *App) handleKickBadgeImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if rawURL := strings.TrimSpace(r.URL.Query().Get("url")); rawURL != "" {
		if err := proxyImageResponse(w, rawURL, true); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	role := kickRoleBadgeVariant(r.URL.Query().Get("role"), func() int { n, _ := strconv.Atoi(r.URL.Query().Get("count")); return n }())
	file, ok := kickRoleBadgeFiles[role]
	if !ok {
		http.Error(w, "unknown Kick badge", http.StatusNotFound)
		return
	}
	// Kick does not expose a documented public endpoint for the global role badge SVGs.
	// These two mirrors carry the current Kick badge art; try both and keep a text fallback in the overlay if unavailable.
	mirrors := []string{"https://www.kickdatabase.com/kickBadges/" + file, "https://cpwemotes.co.uk/kick/kickBadges/" + file}
	for _, endpoint := range mirrors {
		if err := proxyImageResponse(w, endpoint, false, "www.kickdatabase.com", "cpwemotes.co.uk"); err == nil {
			return
		}
	}
	http.Error(w, "Kick badge art unavailable", http.StatusBadGateway)
}

func isTrustedKickImageURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil {
		return nil, false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host != "kick.com" && !strings.HasSuffix(host, ".kick.com") {
		return nil, false
	}
	return u, true
}

func (a *App) handleChatAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	u, ok := isTrustedKickImageURL(r.URL.Query().Get("url"))
	if !ok {
		http.Error(w, "untrusted Kick avatar URL", http.StatusBadRequest)
		return
	}
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			if _, ok := isTrustedKickImageURL(req.URL.String()); !ok {
				return fmt.Errorf("untrusted redirect")
			}
			return nil
		},
	}
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Kick avatar request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, "Kick avatar returned "+resp.Status, resp.StatusCode)
		return
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "invalid Kick avatar response", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=600")
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 6<<20))
}
