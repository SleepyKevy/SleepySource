package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	updateRepositoryURL = "https://github.com/SleepyKevy/SleepySource"
	updateReleaseURL    = updateRepositoryURL + "/releases/latest"
)

var (
	githubLatestReleaseAPI = "https://api.github.com/repos/SleepyKevy/SleepySource/releases/latest"
	updateHTTPClient       = &http.Client{Timeout: 6 * time.Second}
	versionNumberPattern   = regexp.MustCompile(`\d+`)
)

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
}

type UpdateStatus struct {
	Status          string `json:"status"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	ReleaseName     string `json:"release_name,omitempty"`
	ReleaseURL      string `json:"release_url,omitempty"`
	ReleaseNotes    string `json:"release_notes,omitempty"`
	PublishedAt     string `json:"published_at,omitempty"`
	CheckedAt       string `json:"checked_at"`
	UpdateAvailable bool   `json:"update_available"`
	Message         string `json:"message"`
}

func versionParts(version string) ([]int, error) {
	matches := versionNumberPattern.FindAllString(strings.TrimSpace(version), -1)
	if len(matches) == 0 {
		return nil, errors.New("version does not contain a number")
	}
	parts := make([]int, 0, len(matches))
	for _, match := range matches {
		value, err := strconv.Atoi(match)
		if err != nil {
			return nil, err
		}
		parts = append(parts, value)
	}
	return parts, nil
}

func compareVersions(a, b string) (int, error) {
	left, err := versionParts(a)
	if err != nil {
		return 0, err
	}
	right, err := versionParts(b)
	if err != nil {
		return 0, err
	}
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	for i := 0; i < length; i++ {
		lv, rv := 0, 0
		if i < len(left) {
			lv = left[i]
		}
		if i < len(right) {
			rv = right[i]
		}
		if lv < rv {
			return -1, nil
		}
		if lv > rv {
			return 1, nil
		}
	}
	return 0, nil
}

func cleanReleaseNotes(notes string) string {
	notes = strings.TrimSpace(strings.ReplaceAll(notes, "\r\n", "\n"))
	const maxRunes = 12000
	runes := []rune(notes)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "\n\n…View the full release notes on GitHub."
	}
	return notes
}

func fetchUpdateStatus(ctx context.Context) UpdateStatus {
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	result := UpdateStatus{
		Status:         "error",
		CurrentVersion: displayVersion,
		CheckedAt:      checkedAt,
		Message:        "Unable to check for updates.",
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestReleaseAPI, nil)
	if err != nil {
		return result
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			result.Message = "No published SleepySource release was found on GitHub."
		} else if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			result.Message = "GitHub temporarily refused the update check. Try again later."
		}
		return result
	}

	var release githubRelease
	dec := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := dec.Decode(&release); err != nil {
		return result
	}
	latest := strings.TrimSpace(release.TagName)
	if latest == "" {
		latest = strings.TrimSpace(release.Name)
	}
	cmp, err := compareVersions(appVersion, latest)
	if err != nil {
		result.Message = "GitHub returned an update version SleepySource could not understand."
		return result
	}

	result.LatestVersion = strings.TrimLeft(latest, "vV")
	result.ReleaseName = strings.TrimSpace(release.Name)
	result.ReleaseURL = strings.TrimSpace(release.HTMLURL)
	if result.ReleaseURL == "" {
		result.ReleaseURL = updateReleaseURL
	}
	result.ReleaseNotes = cleanReleaseNotes(release.Body)
	result.PublishedAt = strings.TrimSpace(release.PublishedAt)
	result.UpdateAvailable = cmp < 0
	switch {
	case cmp < 0:
		result.Status = "available"
		result.Message = fmt.Sprintf("SleepySource %s is available.", result.LatestVersion)
	case cmp > 0:
		result.Status = "ahead"
		result.Message = "This SleepySource build is newer than the latest published stable release."
	default:
		result.Status = "up_to_date"
		result.Message = "SleepySource is up to date."
	}
	return result
}

func (a *App) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()
	writeJSON(w, fetchUpdateStatus(ctx))
}

func (a *App) handleUpdateOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go openBrowser(updateRepositoryURL)
	w.WriteHeader(http.StatusNoContent)
}
