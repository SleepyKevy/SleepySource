package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var kickCategoriesAPIBase = "https://api.kick.com/public/v2"

type kickChannelMetadata struct {
	BroadcasterUserID int64  `json:"broadcaster_user_id,omitempty"`
	ChannelSlug       string `json:"channel_slug,omitempty"`
	Title             string `json:"title"`
	CategoryID        int64  `json:"category_id,omitempty"`
	CategoryName      string `json:"category_name,omitempty"`
	IsLive            bool   `json:"is_live"`
}

type kickCategoryOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func parseKickError(resp *http.Response, data []byte, fallback string) error {
	if resp == nil {
		return fmt.Errorf(fallback)
	}
	var payload map[string]any
	_ = json.Unmarshal(data, &payload)
	msg := strings.TrimSpace(fmt.Sprint(payload["message"]))
	if msg == "" || msg == "<nil>" {
		msg = strings.TrimSpace(fmt.Sprint(payload["error"]))
	}
	if msg == "" || msg == "<nil>" {
		msg = strings.TrimSpace(string(data))
	}
	if msg == "" {
		msg = fallback
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("Kick rejected the request. Make sure your connected Kick credentials are allowed to edit this channel")
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("Kick could not find that channel or category")
	}
	return fmt.Errorf("%s", msg)
}

func parseKickChannelMetadata(data []byte, fallbackSlug string) (kickChannelMetadata, error) {
	var payload struct {
		Data []struct {
			BroadcasterUserID int64  `json:"broadcaster_user_id"`
			Slug              string `json:"slug"`
			StreamTitle       string `json:"stream_title"`
			Category          struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"category"`
			Stream struct {
				IsLive bool `json:"is_live"`
			} `json:"stream"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return kickChannelMetadata{}, fmt.Errorf("Kick channel lookup returned invalid data")
	}
	if len(payload.Data) == 0 {
		if strings.TrimSpace(fallbackSlug) != "" {
			return kickChannelMetadata{}, fmt.Errorf("Kick did not return channel data for @%s", normalizeKickChannelSlug(fallbackSlug))
		}
		return kickChannelMetadata{}, fmt.Errorf("Kick did not return channel data for the authorized account")
	}
	item := payload.Data[0]
	return kickChannelMetadata{
		BroadcasterUserID: item.BroadcasterUserID,
		ChannelSlug:       firstNonEmpty(normalizeKickChannelSlug(item.Slug), normalizeKickChannelSlug(fallbackSlug)),
		Title:             strings.TrimSpace(item.StreamTitle),
		CategoryID:        item.Category.ID,
		CategoryName:      strings.TrimSpace(item.Category.Name),
		IsLive:            item.Stream.IsLive,
	}, nil
}

func fetchKickChannelMetadata(slug, token string) (kickChannelMetadata, error) {
	slug = normalizeKickChannelSlug(slug)
	if slug == "" {
		return kickChannelMetadata{}, fmt.Errorf("Kick channel not connected")
	}
	endpoint := kickAPIBase + "/channels?slug=" + url.QueryEscape(slug)
	resp, data, err := kickAPIRequest(http.MethodGet, endpoint, token, nil)
	if err != nil {
		return kickChannelMetadata{}, fmt.Errorf("could not reach Kick: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return kickChannelMetadata{}, parseKickError(resp, data, "Kick channel lookup failed")
	}
	return parseKickChannelMetadata(data, slug)
}

func fetchKickAuthorizedChannelMetadata(token string) (kickChannelMetadata, error) {
	resp, data, err := kickAPIRequest(http.MethodGet, kickAPIBase+"/channels", token, nil)
	if err != nil {
		return kickChannelMetadata{}, fmt.Errorf("could not reach Kick: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return kickChannelMetadata{}, parseKickError(resp, data, "Kick authorized-channel lookup failed")
	}
	return parseKickChannelMetadata(data, "")
}

func fetchKickActiveLivestreamMetadata(broadcasterUserID int64, token string) (kickChannelMetadata, bool, error) {
	if broadcasterUserID <= 0 {
		return kickChannelMetadata{}, false, fmt.Errorf("invalid Kick broadcaster user ID")
	}
	endpoint := kickAPIBase + "/users/livestreams?user_id=" + url.QueryEscape(strconv.FormatInt(broadcasterUserID, 10))
	resp, data, err := kickAPIRequest(http.MethodGet, endpoint, token, nil)
	if err != nil {
		return kickChannelMetadata{}, false, fmt.Errorf("could not reach Kick livestreams: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return kickChannelMetadata{}, false, parseKickError(resp, data, "Kick livestream lookup failed")
	}
	var payload struct {
		Data []struct {
			BroadcasterUser struct {
				ID int64 `json:"id"`
			} `json:"broadcaster_user"`
			Category struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"category"`
			Channel struct {
				Slug string `json:"slug"`
			} `json:"channel"`
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return kickChannelMetadata{}, false, fmt.Errorf("Kick livestream lookup returned invalid data")
	}
	if len(payload.Data) == 0 {
		return kickChannelMetadata{BroadcasterUserID: broadcasterUserID, IsLive: false}, false, nil
	}
	item := payload.Data[0]
	return kickChannelMetadata{
		BroadcasterUserID: firstPositiveInt64(item.BroadcasterUser.ID, broadcasterUserID),
		ChannelSlug:       normalizeKickChannelSlug(item.Channel.Slug),
		Title:             strings.TrimSpace(item.Title),
		CategoryID:        item.Category.ID,
		CategoryName:      strings.TrimSpace(item.Category.Name),
		IsLive:            true,
	}, true, nil
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func searchKickCategories(query, token string) ([]kickCategoryOption, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []kickCategoryOption{}, nil
	}
	values := url.Values{}
	values.Set("limit", "20")
	values.Set("name", query)
	endpoint := kickCategoriesAPIBase + "/categories?" + values.Encode()
	resp, data, err := kickAPIRequest(http.MethodGet, endpoint, token, nil)
	if err != nil {
		return nil, fmt.Errorf("could not reach Kick categories: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseKickError(resp, data, "Kick category search failed")
	}
	var payload struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("Kick categories returned invalid data")
	}
	out := make([]kickCategoryOption, 0, len(payload.Data))
	seen := map[int64]bool{}
	for _, item := range payload.Data {
		if item.ID <= 0 || strings.TrimSpace(item.Name) == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		out = append(out, kickCategoryOption{ID: item.ID, Name: strings.TrimSpace(item.Name)})
	}
	return out, nil
}

func resolveKickCategoryID(categoryID int64, categoryName, token string) (int64, string, error) {
	if categoryID > 0 {
		return categoryID, strings.TrimSpace(categoryName), nil
	}
	categoryName = strings.TrimSpace(categoryName)
	if categoryName == "" {
		return 0, "", nil
	}
	results, err := searchKickCategories(categoryName, token)
	if err != nil {
		return 0, "", err
	}
	if len(results) == 0 {
		return 0, "", fmt.Errorf("Kick could not find a category named %q", categoryName)
	}
	for _, item := range results {
		if strings.EqualFold(strings.TrimSpace(item.Name), categoryName) {
			return item.ID, item.Name, nil
		}
	}
	return results[0].ID, results[0].Name, nil
}

func patchKickChannelMetadata(token, title string, categoryID int64) error {
	body := map[string]any{"stream_title": title}
	if categoryID > 0 {
		body["category_id"] = categoryID
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, data, err := kickAPIRequest(http.MethodPatch, kickAPIBase+"/channels", token, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("could not reach Kick: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("Kick rejected Stream Controls authorization. Reauthorize Stream Controls and make sure channel:write permission was approved")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseKickError(resp, data, "Kick stream update failed")
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (a *App) currentKickStreamChannel() string {
	state := a.chat.state()
	return firstNonEmpty(state.ConnectedChannel, state.Settings.KickChannel)
}

func (a *App) handleKickStreamMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := a.chat.state()
	channel := a.currentKickStreamChannel()
	if strings.TrimSpace(channel) == "" {
		http.Error(w, "Connect Kick Channel first", http.StatusPreconditionFailed)
		return
	}
	appToken, err := a.chat.ensureKickAccessToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	meta, err := fetchKickChannelMetadata(channel, appToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	authState := a.streamAuth.state(a.chat.hasAppCredentials())
	authorizedChannel := kickChannelMetadata{}
	authorizedMatches := false
	if authState.Authorized {
		clientID, clientSecret, ok := a.chat.appCredentials()
		if ok {
			if userToken, tokenErr := a.streamAuth.ensureUserAccessToken(clientID, clientSecret); tokenErr == nil {
				if authorizedMeta, metaErr := fetchKickAuthorizedChannelMetadata(userToken); metaErr == nil {
					authorizedChannel = authorizedMeta
					authorizedMatches = sameKickBroadcaster(state.BroadcasterUserID, meta, authorizedMeta)
					if authorizedMatches {
						meta = authorizedMeta
						if liveMeta, live, liveErr := fetchKickActiveLivestreamMetadata(authorizedMeta.BroadcasterUserID, userToken); liveErr == nil {
							if live {
								liveMeta.ChannelSlug = firstNonEmpty(liveMeta.ChannelSlug, authorizedMeta.ChannelSlug)
								meta = liveMeta
							} else {
								meta.IsLive = false
							}
						}
					}
				}
			}
		}
	}

	writeJSON(w, map[string]any{
		"connected":                      state.AuthReady && strings.TrimSpace(state.ConnectedChannel) != "",
		"auth_ready":                     state.AuthReady,
		"channel":                        meta.ChannelSlug,
		"broadcaster_user_id":            meta.BroadcasterUserID,
		"title":                          meta.Title,
		"category_id":                    meta.CategoryID,
		"category_name":                  meta.CategoryName,
		"is_live":                        meta.IsLive,
		"metadata_readback_available":    meta.IsLive || strings.TrimSpace(meta.Title) != "" || meta.CategoryID > 0 || strings.TrimSpace(meta.CategoryName) != "",
		"update_supported":               true,
		"stream_authorized":              authState.Authorized,
		"authorized_channel":             authorizedChannel.ChannelSlug,
		"authorized_broadcaster_user_id": authorizedChannel.BroadcasterUserID,
		"authorized_channel_matches":     authorizedMatches,
		"connected_channel":              state.ConnectedChannel,
	})
}

func sameKickBroadcaster(connectedID string, connectedMeta, authorizedMeta kickChannelMetadata) bool {
	if authorizedMeta.BroadcasterUserID <= 0 {
		return false
	}
	if id, err := strconv.ParseInt(strings.TrimSpace(connectedID), 10, 64); err == nil && id > 0 {
		return id == authorizedMeta.BroadcasterUserID
	}
	if connectedMeta.BroadcasterUserID > 0 {
		return connectedMeta.BroadcasterUserID == authorizedMeta.BroadcasterUserID
	}
	return strings.EqualFold(strings.TrimSpace(connectedMeta.ChannelSlug), strings.TrimSpace(authorizedMeta.ChannelSlug))
}

func (a *App) handleKickStreamCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, map[string]any{"categories": []kickCategoryOption{}})
		return
	}
	token, err := a.chat.ensureKickAccessToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	categories, err := searchKickCategories(query, token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"categories": categories})
}

func (a *App) handleKickStreamUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := a.chat.state()
	channel := a.currentKickStreamChannel()
	if !state.AuthReady || strings.TrimSpace(state.ConnectedChannel) == "" || strings.TrimSpace(channel) == "" {
		http.Error(w, "Connect Kick Channel first", http.StatusPreconditionFailed)
		return
	}
	var in struct {
		Title        string `json:"title"`
		CategoryID   int64  `json:"category_id"`
		CategoryName string `json:"category_name"`
	}
	if err := decodeSingleJSON(r.Body, &in, 64<<10); err != nil {
		http.Error(w, "invalid stream settings payload", http.StatusBadRequest)
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		http.Error(w, "Stream title is required", http.StatusBadRequest)
		return
	}
	appToken, err := a.chat.ensureKickAccessToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	categoryID, categoryName, err := resolveKickCategoryID(in.CategoryID, in.CategoryName, appToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	clientID, clientSecret, ok := a.chat.appCredentials()
	if !ok {
		http.Error(w, "Kick Developer App credentials are unavailable; reconnect Kick in Chat Overlay", http.StatusPreconditionFailed)
		return
	}
	userToken, err := a.streamAuth.ensureUserAccessToken(clientID, clientSecret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	connectedMeta, err := fetchKickChannelMetadata(channel, appToken)
	if err != nil {
		http.Error(w, "Could not verify the connected Kick channel: "+err.Error(), http.StatusBadGateway)
		return
	}
	authorizedMeta, err := fetchKickAuthorizedChannelMetadata(userToken)
	if err != nil {
		http.Error(w, "Could not verify the Stream Controls account: "+err.Error(), http.StatusBadGateway)
		return
	}
	if !sameKickBroadcaster(state.BroadcasterUserID, connectedMeta, authorizedMeta) {
		http.Error(w, fmt.Sprintf("Stream Controls is authorized for @%s, but Chat Overlay is connected to @%s. Disconnect Stream Controls and authorize the same Kick account before updating.", firstNonEmpty(authorizedMeta.ChannelSlug, "another account"), firstNonEmpty(connectedMeta.ChannelSlug, state.ConnectedChannel)), http.StatusConflict)
		return
	}
	if err := patchKickChannelMetadata(userToken, in.Title, categoryID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	verified := authorizedMeta
	verificationAvailable := false
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 350 * time.Millisecond)
		}
		liveMeta, live, fetchErr := fetchKickActiveLivestreamMetadata(authorizedMeta.BroadcasterUserID, userToken)
		if fetchErr != nil {
			continue
		}
		if !live {
			// Kick's channel read response may expose blank title/category while offline.
			// A successful 204 PATCH is authoritative in this state because there is
			// no reliable public read-back surface until an active livestream exists.
			break
		}
		verificationAvailable = true
		liveMeta.ChannelSlug = firstNonEmpty(liveMeta.ChannelSlug, authorizedMeta.ChannelSlug)
		verified = liveMeta
		titleMatches := strings.EqualFold(strings.TrimSpace(liveMeta.Title), strings.TrimSpace(in.Title))
		categoryMatches := categoryID <= 0 || liveMeta.CategoryID == categoryID
		if titleMatches && categoryMatches {
			break
		}
	}

	if verificationAvailable {
		titleMatches := strings.EqualFold(strings.TrimSpace(verified.Title), strings.TrimSpace(in.Title))
		categoryMatches := categoryID <= 0 || verified.CategoryID == categoryID
		if !titleMatches || !categoryMatches {
			http.Error(w, fmt.Sprintf("Kick accepted the update request, but the active livestream still reports title %q and category %q. Wait a moment, refresh Stream Dashboard, and try once more if it does not update.", verified.Title, verified.CategoryName), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{
			"ok":                  true,
			"verified":            true,
			"channel":             verified.ChannelSlug,
			"broadcaster_user_id": verified.BroadcasterUserID,
			"title":               verified.Title,
			"category_id":         verified.CategoryID,
			"category_name":       firstNonEmpty(verified.CategoryName, categoryName),
			"message":             "Kick stream settings updated and verified on the active livestream",
		})
		return
	}

	writeJSON(w, map[string]any{
		"ok":                  true,
		"verified":            false,
		"verification_status": "accepted_offline",
		"channel":             firstNonEmpty(authorizedMeta.ChannelSlug, connectedMeta.ChannelSlug, state.ConnectedChannel),
		"broadcaster_user_id": authorizedMeta.BroadcasterUserID,
		"title":               in.Title,
		"category_id":         categoryID,
		"category_name":       categoryName,
		"message":             "Kick accepted the stream update. This channel is offline, and Kick does not reliably expose offline title/category for read-back; the new settings will be verified once the channel is live.",
	})
}

// quick sanity check helper used in tests and guards when needed.
func parseCategoryID(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, _ := strconv.ParseInt(raw, 10, 64)
	if n < 0 {
		return 0
	}
	return n
}
