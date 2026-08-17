package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type kickAlertUser struct {
	UserID         int64  `json:"user_id"`
	Username       string `json:"username"`
	ProfilePicture string `json:"profile_picture"`
	IsAnonymous    bool   `json:"is_anonymous"`
}

func parseKickAlertEvent(eventType, messageID string, body []byte) (AlertEvent, string, bool) {
	alertType, supported := kickAlertEvents[strings.ToLower(strings.TrimSpace(eventType))]
	if !supported {
		return AlertEvent{}, "", false
	}
	event := AlertEvent{
		ID:          strings.TrimSpace(messageID),
		Type:        alertType,
		Source:      "kick",
		CreatedAtMS: time.Now().UnixMilli(),
	}
	dedupeKey := "kick:" + strings.TrimSpace(messageID)

	switch alertType {
	case "follow":
		var raw struct {
			Follower kickAlertUser `json:"follower"`
		}
		if json.Unmarshal(body, &raw) != nil {
			return AlertEvent{}, "", false
		}
		event.Username = alertKickUsername(raw.Follower)
	case "subscription-new", "subscription-renewal":
		var raw struct {
			Subscriber kickAlertUser `json:"subscriber"`
			Duration   int           `json:"duration"`
			CreatedAt  string        `json:"created_at"`
		}
		if json.Unmarshal(body, &raw) != nil {
			return AlertEvent{}, "", false
		}
		event.Username = alertKickUsername(raw.Subscriber)
		event.Months = raw.Duration
		if parsed := parseKickEventTime(raw.CreatedAt); parsed > 0 {
			event.CreatedAtMS = parsed
		}
	case "subscription-gift":
		var raw struct {
			Gifter    kickAlertUser   `json:"gifter"`
			Giftees   []kickAlertUser `json:"giftees"`
			CreatedAt string          `json:"created_at"`
		}
		if json.Unmarshal(body, &raw) != nil {
			return AlertEvent{}, "", false
		}
		event.Username = alertKickUsername(raw.Gifter)
		if raw.Gifter.IsAnonymous || strings.TrimSpace(raw.Gifter.Username) == "" {
			event.Username = "Anonymous"
		}
		event.Count = len(raw.Giftees)
		if parsed := parseKickEventTime(raw.CreatedAt); parsed > 0 {
			event.CreatedAtMS = parsed
		}
	case "kicks":
		var raw struct {
			Sender kickAlertUser `json:"sender"`
			Gift   struct {
				Amount  int    `json:"amount"`
				Name    string `json:"name"`
				Message string `json:"message"`
			} `json:"gift"`
			CreatedAt string `json:"created_at"`
		}
		if json.Unmarshal(body, &raw) != nil {
			return AlertEvent{}, "", false
		}
		event.Username = alertKickUsername(raw.Sender)
		event.Amount = raw.Gift.Amount
		event.GiftName = strings.TrimSpace(raw.Gift.Name)
		if event.GiftName == "" {
			event.GiftName = "Kicks Gift"
		}
		if parsed := parseKickEventTime(raw.CreatedAt); parsed > 0 {
			event.CreatedAtMS = parsed
		}
	case "reward":
		var raw struct {
			ID        string        `json:"id"`
			UserInput string        `json:"user_input"`
			Status    string        `json:"status"`
			Redeemed  string        `json:"redeemed_at"`
			Redeemer  kickAlertUser `json:"redeemer"`
			Reward    struct {
				Title string `json:"title"`
			} `json:"reward"`
		}
		if json.Unmarshal(body, &raw) != nil || strings.TrimSpace(raw.ID) == "" {
			return AlertEvent{}, "", false
		}
		event.Username = alertKickUsername(raw.Redeemer)
		event.RewardTitle = strings.TrimSpace(raw.Reward.Title)
		if event.RewardTitle == "" {
			event.RewardTitle = "Channel Reward"
		}
		event.UserInput = strings.TrimSpace(raw.UserInput)
		if parsed := parseKickEventTime(raw.Redeemed); parsed > 0 {
			event.CreatedAtMS = parsed
		}
		// A redemption can later be accepted/rejected and generate another update.
		// Key it by redemption ID so the viewer sees the redemption once, not each
		// status transition.
		dedupeKey = "kick:reward:" + strings.TrimSpace(raw.ID)
		event.ID = strings.TrimSpace(messageID)
	}
	if strings.TrimSpace(event.Username) == "" {
		event.Username = "Anonymous"
	}
	if event.Count < 0 || event.Amount < 0 || event.Months < 0 {
		return AlertEvent{}, "", false
	}
	return event, dedupeKey, true
}

func alertKickUsername(user kickAlertUser) string {
	name := strings.TrimSpace(user.Username)
	if user.IsAnonymous || name == "" {
		return "Anonymous"
	}
	return name
}

func parseKickEventTime(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UnixMilli()
	}
	return 0
}

func broadcasterIDFromKickEvent(body []byte) string {
	var raw struct {
		Broadcaster struct {
			UserID int64 `json:"user_id"`
		} `json:"broadcaster"`
	}
	if json.Unmarshal(body, &raw) != nil || raw.Broadcaster.UserID <= 0 {
		return ""
	}
	return strconv.FormatInt(raw.Broadcaster.UserID, 10)
}

func (a *App) acceptKickAlert(eventType, messageID string, body []byte) (bool, error) {
	if a.alerts == nil {
		return false, nil
	}
	connectedID := ""
	if a.chat != nil {
		connectedID = strings.TrimSpace(a.chat.state().BroadcasterUserID)
	}
	if payloadID := broadcasterIDFromKickEvent(body); connectedID != "" && payloadID != "" && payloadID != connectedID {
		return false, fmt.Errorf("Kick alert broadcaster does not match the connected channel")
	}
	event, dedupeKey, ok := parseKickAlertEvent(eventType, messageID, body)
	if !ok {
		return false, fmt.Errorf("invalid %s alert payload", strings.TrimSpace(eventType))
	}
	return a.alerts.enqueue(event, dedupeKey), nil
}
