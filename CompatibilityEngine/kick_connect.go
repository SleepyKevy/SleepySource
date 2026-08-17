package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var kickAPIBase = "https://api.kick.com/public/v1"
var kickOAuthBase = "https://id.kick.com"

func requestKickAppAccessToken(clientID, clientSecret string) (string, time.Time, error) {
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if clientID == "" || clientSecret == "" {
		return "", time.Time{}, fmt.Errorf("enter your Kick Client ID and Client Secret first")
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	req, err := http.NewRequest(http.MethodPost, kickOAuthBase+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("could not reach Kick OAuth: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return "", time.Time{}, fmt.Errorf("Kick rejected the Client ID or Client Secret")
		}
		var payload map[string]any
		_ = json.Unmarshal(data, &payload)
		msg := strings.TrimSpace(fmt.Sprint(payload["message"]))
		if msg == "" || msg == "<nil>" {
			msg = strings.TrimSpace(fmt.Sprint(payload["error"]))
		}
		if msg == "" || msg == "<nil>" {
			msg = resp.Status
		}
		return "", time.Time{}, fmt.Errorf("Kick OAuth: %s", msg)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", time.Time{}, fmt.Errorf("Kick OAuth returned an invalid response")
	}
	accessToken := strings.TrimSpace(fmt.Sprint(payload["access_token"]))
	if accessToken == "" || accessToken == "<nil>" {
		return "", time.Time{}, fmt.Errorf("Kick OAuth did not return an access token")
	}
	expiresIn := int64(3600)
	switch v := payload["expires_in"].(type) {
	case float64:
		if v > 0 {
			expiresIn = int64(v)
		}
	case string:
		if n, e := strconv.ParseInt(strings.TrimSpace(v), 10, 64); e == nil && n > 0 {
			expiresIn = n
		}
	}
	if expiresIn < 60 {
		expiresIn = 60
	}
	return accessToken, time.Now().Add(time.Duration(expiresIn) * time.Second), nil
}

func (m *ChatManager) ensureKickAccessToken() (string, error) {
	m.authMu.Lock()
	defer m.authMu.Unlock()

	m.mu.RLock()
	token := strings.TrimSpace(m.accessToken)
	expiresAt := m.accessTokenExpiresAt
	clientID := strings.TrimSpace(m.clientID)
	clientSecret := strings.TrimSpace(m.clientSecret)
	authMode := m.authMode
	m.mu.RUnlock()

	if token != "" && (expiresAt.IsZero() || time.Now().Add(45*time.Second).Before(expiresAt)) {
		return token, nil
	}
	if authMode == "legacy-user-token" && token != "" {
		return token, nil
	}
	accessToken, nextExpiry, err := requestKickAppAccessToken(clientID, clientSecret)
	if err != nil {
		return "", err
	}
	m.setKickAppAccessToken(accessToken, nextExpiry)
	return accessToken, nil
}

func (m *ChatManager) lookupAuth() (ChatSettings, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings, m.accessToken
}

func (a *App) handleChatAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		AccessToken  string `json:"access_token"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Forget       bool   `json:"forget"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		http.Error(w, "invalid auth payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(in.AccessToken) != "" {
		// Backward-compatible API support for older local clients. The Designer no longer exposes manual token entry.
		a.chat.setAccessToken(in.AccessToken)
		writeJSON(w, map[string]any{"ready": true, "mode": "legacy-user-token"})
		return
	}
	if in.Forget {
		a.chat.clearAuth()
		writeJSON(w, map[string]any{"ready": false, "forgotten": true})
		return
	}
	if strings.TrimSpace(in.ClientID) == "" && strings.TrimSpace(in.ClientSecret) == "" {
		a.chat.disconnectAuth()
		writeJSON(w, map[string]any{"ready": a.chat.hasAppCredentials(), "saved": a.chat.state().CredentialsSaved})
		return
	}
	if strings.TrimSpace(in.ClientID) == "" || strings.TrimSpace(in.ClientSecret) == "" {
		http.Error(w, "both Kick Client ID and Client Secret are required", http.StatusBadRequest)
		return
	}
	token, expiresAt, err := requestKickAppAccessToken(in.ClientID, in.ClientSecret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.chat.setAppCredentials(in.ClientID, in.ClientSecret); err != nil {
		http.Error(w, "could not securely save Kick login: "+err.Error(), http.StatusInternalServerError)
		return
	}
	a.chat.setKickAppAccessToken(token, expiresAt)
	writeJSON(w, map[string]any{"ready": true, "mode": "app"})
}

func (a *App) handleKickConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Channel      string `json:"channel"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		http.Error(w, "invalid connection payload", http.StatusBadRequest)
		return
	}
	channel := normalizeKickChannelSlug(in.Channel)
	if channel == "" {
		channel = a.chat.state().Settings.KickChannel
	}
	if channel == "" {
		http.Error(w, "Kick channel username required", http.StatusBadRequest)
		return
	}
	clientID := strings.TrimSpace(in.ClientID)
	clientSecret := strings.TrimSpace(in.ClientSecret)
	var token string
	if clientID != "" || clientSecret != "" {
		if clientID == "" || clientSecret == "" {
			http.Error(w, "both Kick Client ID and Client Secret are required", http.StatusBadRequest)
			return
		}
		validatedToken, expiresAt, err := requestKickAppAccessToken(clientID, clientSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if err := a.chat.setAppCredentials(clientID, clientSecret); err != nil {
			http.Error(w, "could not securely save Kick login: "+err.Error(), http.StatusInternalServerError)
			return
		}
		a.chat.setKickAppAccessToken(validatedToken, expiresAt)
		token = validatedToken
	} else {
		if !a.chat.hasAppCredentials() {
			http.Error(w, "enter your Kick Client ID and Client Secret first", http.StatusPreconditionFailed)
			return
		}
		var err error
		token, err = a.chat.ensureKickAccessToken()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	if a.cloudflare != nil {
		if err := a.cloudflare.start(); err != nil {
			http.Error(w, "could not start the automatic secure webhook relay: "+err.Error(), http.StatusBadGateway)
			return
		}
	}
	userID, slug, err := resolveKickBroadcasterUserID(channel, token)
	if err != nil {
		http.Error(w, err.Error(), kickLookupHTTPStatus(err))
		return
	}
	subscriptionID, replacedSubscriptions, err := refreshKickChatWebhookSubscription(token, userID)
	if err != nil {
		http.Error(w, "Kick channel resolved, but Kick event subscription refresh failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	a.chat.stopLiveChat()
	a.chat.setResolvedChannel(slug, userID)
	status := "Official Kick chat + Alert Studio events freshly registered — waiting for the first verified event"
	if replacedSubscriptions > 0 {
		status = fmt.Sprintf("Official Kick chat + Alert Studio events refreshed (%d old subscription removed) — waiting for verified events", replacedSubscriptions)
	}
	a.chat.setWebhookSubscription(subscriptionID, status)
	relayState := cloudflareTunnelState{}
	if a.cloudflare != nil {
		relayState = a.cloudflare.state()
	}
	writeJSON(w, map[string]any{
		"connected":               true,
		"channel":                 slug,
		"broadcaster_user_id":     userID,
		"webhook_subscribed":      true,
		"webhook_subscription_id": subscriptionID,
		"replaced_subscriptions":  replacedSubscriptions,
		"webhook_path":            "/api/chat/kick-webhook",
		"webhook_url":             relayState.WebhookURL,
		"relay_running":           relayState.Running,
		"relay_mode":              relayState.Mode,
		"auth_mode":               "app",
	})
}

func (a *App) handleKickReregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := a.chat.state()
	if strings.TrimSpace(state.BroadcasterUserID) == "" || strings.TrimSpace(state.ConnectedChannel) == "" {
		http.Error(w, "connect your Kick channel first", http.StatusPreconditionFailed)
		return
	}
	token, err := a.chat.ensureKickAccessToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if a.cloudflare != nil {
		if err := a.cloudflare.start(); err != nil {
			http.Error(w, "could not start the automatic secure webhook relay: "+err.Error(), http.StatusBadGateway)
			return
		}
	}
	subscriptionID, replacedSubscriptions, err := refreshKickChatWebhookSubscription(token, state.BroadcasterUserID)
	if err != nil {
		http.Error(w, "Kick event subscription refresh failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	status := "Official Kick chat + Alert Studio events freshly registered — waiting for the first verified event"
	if replacedSubscriptions > 0 {
		status = fmt.Sprintf("Official Kick chat + Alert Studio events refreshed (%d old subscription removed) — waiting for verified events", replacedSubscriptions)
	}
	a.chat.setWebhookSubscription(subscriptionID, status)
	relayState := cloudflareTunnelState{}
	if a.cloudflare != nil {
		relayState = a.cloudflare.state()
	}
	writeJSON(w, map[string]any{
		"ok":                      true,
		"webhook_subscription_id": subscriptionID,
		"replaced_subscriptions":  replacedSubscriptions,
		"webhook_url":             relayState.WebhookURL,
	})
}

func normalizeKickChannelSlug(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "@")
	lower := strings.ToLower(value)
	for _, prefix := range []string{"https://kick.com/", "http://kick.com/", "https://www.kick.com/", "http://www.kick.com/", "kick.com/", "www.kick.com/"} {
		if strings.HasPrefix(lower, prefix) {
			value = value[len(prefix):]
			break
		}
	}
	if i := strings.IndexAny(value, "/?#"); i >= 0 {
		value = value[:i]
	}
	value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
	if len(value) > 80 {
		value = value[:80]
	}
	return value
}

func parseKickChannelResponse(data []byte, requestedChannel string) (string, string, error) {
	var payload struct {
		Data []struct {
			BroadcasterUserID int64  `json:"broadcaster_user_id"`
			Slug              string `json:"slug"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", "", fmt.Errorf("Kick returned an invalid channel response")
	}
	if len(payload.Data) == 0 {
		return "", "", fmt.Errorf("Kick channel %q was not found", requestedChannel)
	}
	match := payload.Data[0]
	for _, item := range payload.Data {
		if strings.EqualFold(strings.TrimSpace(item.Slug), requestedChannel) {
			match = item
			break
		}
	}
	if match.BroadcasterUserID <= 0 {
		return "", "", fmt.Errorf("Kick did not return a broadcaster user ID for %q", requestedChannel)
	}
	resolvedSlug := normalizeKickChannelSlug(match.Slug)
	if resolvedSlug == "" {
		resolvedSlug = requestedChannel
	}
	return strconv.FormatInt(match.BroadcasterUserID, 10), resolvedSlug, nil
}

func kickChannelRequest(endpoint, token, requestedChannel string) (string, string, int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", 0, nil, err
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", "", 0, nil, fmt.Errorf("Kick channel lookup failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", resp.StatusCode, data, nil
	}
	userID, slug, err := parseKickChannelResponse(data, requestedChannel)
	return userID, slug, resp.StatusCode, data, err
}

func resolveKickBroadcasterUserID(channel, token string) (string, string, error) {
	channel = normalizeKickChannelSlug(channel)
	token = strings.TrimSpace(token)
	if channel == "" {
		return "", "", fmt.Errorf("Kick channel username required")
	}
	if token == "" {
		return "", "", fmt.Errorf("connect Kick with a Client ID and Client Secret first")
	}
	endpoint := kickAPIBase + "/channels?slug=" + url.QueryEscape(channel)
	userID, slug, status, data, err := kickChannelRequest(endpoint, token, channel)
	if err != nil {
		return "", "", err
	}
	if status >= 200 && status < 300 {
		return userID, slug, nil
	}
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		msg = http.StatusText(status)
	}
	return "", "", fmt.Errorf("Kick official channel lookup failed (%d): %s", status, msg)
}
