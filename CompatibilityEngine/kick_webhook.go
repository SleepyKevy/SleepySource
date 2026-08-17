package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type kickEventSubscription struct {
	ID                string `json:"id"`
	Event             string `json:"event"`
	Version           int    `json:"version"`
	BroadcasterUserID int64  `json:"broadcaster_user_id"`
}

func kickAPIRequest(method, endpoint, token string, body io.Reader) (*http.Response, []byte, error) {
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return resp, data, nil
}

func refreshKickChatWebhookSubscription(token, broadcasterUserID string) (string, int, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(broadcasterUserID), 10, 64)
	if err != nil || id <= 0 {
		return "", 0, fmt.Errorf("invalid Kick broadcaster user ID")
	}
	desired := []string{
		"chat.message.sent",
		"channel.followed",
		"channel.subscription.new",
		"channel.subscription.renewal",
		"channel.subscription.gifts",
		"kicks.gifted",
		"channel.reward.redemption.updated",
	}
	desiredSet := make(map[string]struct{}, len(desired))
	for _, name := range desired {
		desiredSet[strings.ToLower(name)] = struct{}{}
	}

	endpoint := kickAPIBase + "/events/subscriptions?broadcaster_user_id=" + url.QueryEscape(strconv.FormatInt(id, 10))
	resp, data, err := kickAPIRequest(http.MethodGet, endpoint, token, nil)
	if err != nil {
		return "", 0, fmt.Errorf("could not check Kick event subscriptions: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("Kick event subscription lookup returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var payload struct {
		Data []kickEventSubscription `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", 0, fmt.Errorf("Kick returned an invalid event subscription list")
	}
	ids := make([]string, 0, len(desired))
	for _, sub := range payload.Data {
		_, wanted := desiredSet[strings.ToLower(strings.TrimSpace(sub.Event))]
		if wanted && sub.Version == 1 && strings.TrimSpace(sub.ID) != "" {
			ids = append(ids, strings.TrimSpace(sub.ID))
		}
	}
	if len(ids) > 0 {
		q := url.Values{}
		for _, subscriptionID := range ids {
			q.Add("id", subscriptionID)
		}
		resp, data, err = kickAPIRequest(http.MethodDelete, kickAPIBase+"/events/subscriptions?"+q.Encode(), token, nil)
		if err != nil {
			return "", 0, fmt.Errorf("could not remove the old Kick event subscriptions: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", 0, fmt.Errorf("Kick could not remove the old event subscriptions (%s): %s", resp.Status, strings.TrimSpace(string(data)))
		}
	}
	events := make([]map[string]any, 0, len(desired))
	for _, name := range desired {
		events = append(events, map[string]any{"name": name, "version": 1})
	}
	body, _ := json.Marshal(map[string]any{
		"broadcaster_user_id": id,
		"events":              events,
		"method":              "webhook",
	})
	resp, data, err = kickAPIRequest(http.MethodPost, kickAPIBase+"/events/subscriptions", token, strings.NewReader(string(body)))
	if err != nil {
		return "", len(ids), fmt.Errorf("could not subscribe to Kick chat and alert events: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", len(ids), fmt.Errorf("Kick returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var created struct {
		Data []struct {
			Name           string `json:"name"`
			Version        int    `json:"version"`
			SubscriptionID string `json:"subscription_id"`
			Error          string `json:"error"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &created); err != nil {
		return "", len(ids), fmt.Errorf("Kick returned an invalid event subscription response")
	}
	confirmed := make(map[string]string, len(desired))
	for _, item := range created.Data {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		if _, wanted := desiredSet[name]; !wanted || item.Version != 1 {
			continue
		}
		if strings.TrimSpace(item.Error) != "" {
			return "", len(ids), fmt.Errorf("Kick could not subscribe to %s: %s", strings.TrimSpace(item.Name), strings.TrimSpace(item.Error))
		}
		if strings.TrimSpace(item.SubscriptionID) == "" {
			return "", len(ids), fmt.Errorf("Kick did not return a subscription ID for %s", strings.TrimSpace(item.Name))
		}
		confirmed[name] = strings.TrimSpace(item.SubscriptionID)
	}
	for _, name := range desired {
		if confirmed[strings.ToLower(name)] == "" {
			return "", len(ids), fmt.Errorf("Kick did not confirm the %s subscription", name)
		}
	}
	return confirmed["chat.message.sent"], len(ids), nil
}

var kickPublicKeyCache struct {
	sync.Mutex
	base string
	key  *rsa.PublicKey
	at   time.Time
}

func kickWebhookKeyRefreshAllowedAfterFailure() bool {
	kickPublicKeyCache.Lock()
	defer kickPublicKeyCache.Unlock()
	return kickPublicKeyCache.key != nil && kickPublicKeyCache.base == kickAPIBase && time.Since(kickPublicKeyCache.at) >= 5*time.Minute
}

func parseKickRSAPublicKey(pemText string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("Kick public key is invalid")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("Kick public key is not RSA")
	}
	return key, nil
}

func fetchKickWebhookPublicKey(force bool) (*rsa.PublicKey, error) {
	kickPublicKeyCache.Lock()
	defer kickPublicKeyCache.Unlock()
	if !force && kickPublicKeyCache.key != nil && kickPublicKeyCache.base == kickAPIBase && time.Since(kickPublicKeyCache.at) < 6*time.Hour {
		return kickPublicKeyCache.key, nil
	}
	resp, data, err := kickAPIRequest(http.MethodGet, kickAPIBase+"/public-key", "", nil)
	if err != nil {
		return nil, fmt.Errorf("could not fetch Kick webhook public key: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Kick public-key endpoint returned %s", resp.Status)
	}
	var payload struct {
		Data struct {
			PublicKey string `json:"public_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || strings.TrimSpace(payload.Data.PublicKey) == "" {
		return nil, fmt.Errorf("Kick returned an invalid webhook public key response")
	}
	key, err := parseKickRSAPublicKey(payload.Data.PublicKey)
	if err != nil {
		return nil, err
	}
	kickPublicKeyCache.base = kickAPIBase
	kickPublicKeyCache.key = key
	kickPublicKeyCache.at = time.Now()
	return key, nil
}

func verifyKickWebhookSignature(messageID, timestamp string, body []byte, signatureB64 string) error {
	messageID = strings.TrimSpace(messageID)
	timestamp = strings.TrimSpace(timestamp)
	signatureB64 = strings.TrimSpace(signatureB64)
	if messageID == "" || timestamp == "" || signatureB64 == "" {
		return fmt.Errorf("missing Kick webhook signature headers")
	}
	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("invalid Kick webhook signature encoding")
	}
	signed := []byte(messageID + "." + timestamp + "." + string(body))
	hashed := sha256.Sum256(signed)
	verifyWith := func(force bool) error {
		key, err := fetchKickWebhookPublicKey(force)
		if err != nil {
			return err
		}
		return rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], signature)
	}
	if err := verifyWith(false); err == nil {
		return nil
	}
	if kickWebhookKeyRefreshAllowedAfterFailure() {
		if err := verifyWith(true); err == nil {
			return nil
		}
	}
	return fmt.Errorf("Kick webhook signature verification failed")
}

func (a *App) handleKickWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(time.Now().Add(15 * time.Second)); err == nil {
		defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
	}
	eventType := strings.TrimSpace(r.Header.Get("Kick-Event-Type"))
	a.chat.markWebhookRequest(eventType)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		a.chat.markWebhookRejected("invalid webhook body")
		http.Error(w, "invalid webhook body", http.StatusBadRequest)
		return
	}
	messageID := strings.TrimSpace(r.Header.Get("Kick-Event-Message-Id"))
	if err := verifyKickWebhookSignature(
		messageID,
		r.Header.Get("Kick-Event-Message-Timestamp"),
		body,
		r.Header.Get("Kick-Event-Signature"),
	); err != nil {
		a.chat.markWebhookRejected(err.Error())
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	a.chat.markWebhookVerified(eventType)

	isChat := strings.EqualFold(eventType, "chat.message.sent")
	_, isAlert := kickAlertEvents[strings.ToLower(eventType)]
	if !isChat && !isAlert {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if version := strings.TrimSpace(r.Header.Get("Kick-Event-Version")); version != "" && version != "1" {
		a.chat.markWebhookRejected("unsupported Kick event version")
		http.Error(w, "unsupported Kick event version", http.StatusBadRequest)
		return
	}

	if isChat {
		msg, ok := parseKickWebhookChatMessage(body)
		if !ok {
			a.chat.markWebhookRejected("invalid Kick chat.message.sent payload")
			http.Error(w, "invalid Kick chat.message.sent payload", http.StatusBadRequest)
			return
		}
		if !a.chat.acceptWebhookMessageID(messageID) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		a.chat.addMessage(msg)
		a.chat.markWebhookAccepted(eventType, true)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if _, err := a.acceptKickAlert(eventType, messageID, body); err != nil {
		a.chat.markWebhookRejected(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.chat.markWebhookAccepted(eventType, false)
	w.WriteHeader(http.StatusNoContent)
}
