package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	kickUserOAuthRedirectURI = "http://127.0.0.1:17891/oauth/kick/callback"
	kickUserOAuthScopes      = "user:read channel:read channel:write"
)

type KickUserAuthState struct {
	Authorized        bool   `json:"authorized"`
	Scope             string `json:"scope,omitempty"`
	ExpiresAt         int64  `json:"expires_at,omitempty"`
	RedirectURI       string `json:"redirect_uri"`
	Pending           bool   `json:"pending"`
	LastError         string `json:"last_error,omitempty"`
	HasAppCredentials bool   `json:"has_app_credentials"`
}

type storedKickUserAuth struct {
	Version       int    `json:"version"`
	ProtectedData string `json:"protected_data"`
}

type kickUserTokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresAt    int64  `json:"expires_at"`
}

type KickUserAuthManager struct {
	mu              sync.Mutex
	path            string
	accessToken     string
	refreshToken    string
	tokenType       string
	scope           string
	expiresAt       time.Time
	pendingState    string
	pendingVerifier string
	pendingAt       time.Time
	lastError       string
}

func newKickUserAuthManager(dataDir string) *KickUserAuthManager {
	m := &KickUserAuthManager{path: filepath.Join(dataDir, "kick_user_authorization.json")}
	m.load()
	return m
}

func randomOAuthValue(bytesLen int) (string, error) {
	if bytesLen < 32 {
		bytesLen = 32
	}
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func oauthCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (m *KickUserAuthManager) load() {
	if m == nil || !secureCredentialStorageAvailable() {
		return
	}
	data, err := os.ReadFile(m.path)
	if err != nil {
		return
	}
	var stored storedKickUserAuth
	if json.Unmarshal(data, &stored) != nil || strings.TrimSpace(stored.ProtectedData) == "" {
		return
	}
	ciphertext, err := base64.StdEncoding.DecodeString(stored.ProtectedData)
	if err != nil {
		return
	}
	plain, err := unprotectCredential(ciphertext)
	if err != nil {
		return
	}
	var token kickUserTokenData
	if json.Unmarshal(plain, &token) != nil || strings.TrimSpace(token.RefreshToken) == "" {
		return
	}
	m.mu.Lock()
	m.accessToken = strings.TrimSpace(token.AccessToken)
	m.refreshToken = strings.TrimSpace(token.RefreshToken)
	m.tokenType = strings.TrimSpace(token.TokenType)
	m.scope = strings.TrimSpace(token.Scope)
	if token.ExpiresAt > 0 {
		m.expiresAt = time.Unix(token.ExpiresAt, 0)
	}
	m.mu.Unlock()
}

func (m *KickUserAuthManager) saveLocked() error {
	if m == nil {
		return fmt.Errorf("stream authorization manager unavailable")
	}
	if !secureCredentialStorageAvailable() {
		return fmt.Errorf("secure Kick user authorization storage is unavailable on this platform")
	}
	token := kickUserTokenData{
		AccessToken:  strings.TrimSpace(m.accessToken),
		RefreshToken: strings.TrimSpace(m.refreshToken),
		TokenType:    strings.TrimSpace(m.tokenType),
		Scope:        strings.TrimSpace(m.scope),
	}
	if !m.expiresAt.IsZero() {
		token.ExpiresAt = m.expiresAt.Unix()
	}
	plain, err := json.Marshal(token)
	if err != nil {
		return err
	}
	protected, err := protectCredential(plain)
	if err != nil {
		return err
	}
	stored := storedKickUserAuth{Version: 1, ProtectedData: base64.StdEncoding.EncodeToString(protected)}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.path), ".kick-user-auth-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_ = tmp.Chmod(0600)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	_ = tmp.Sync()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := commitTempFile(tmpName, m.path, 8); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func (m *KickUserAuthManager) clear() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.accessToken = ""
	m.refreshToken = ""
	m.tokenType = ""
	m.scope = ""
	m.expiresAt = time.Time{}
	m.pendingState = ""
	m.pendingVerifier = ""
	m.pendingAt = time.Time{}
	m.lastError = ""
	m.mu.Unlock()
	_ = os.Remove(m.path)
}

func (m *KickUserAuthManager) state(hasAppCredentials bool) KickUserAuthState {
	if m == nil {
		return KickUserAuthState{RedirectURI: kickUserOAuthRedirectURI, HasAppCredentials: hasAppCredentials}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	pending := m.pendingState != "" && time.Since(m.pendingAt) < 10*time.Minute
	if !pending && m.pendingState != "" {
		m.pendingState = ""
		m.pendingVerifier = ""
		m.pendingAt = time.Time{}
	}
	authorized := strings.TrimSpace(m.refreshToken) != "" || (strings.TrimSpace(m.accessToken) != "" && (m.expiresAt.IsZero() || time.Now().Before(m.expiresAt)))
	state := KickUserAuthState{
		Authorized:        authorized,
		Scope:             strings.TrimSpace(m.scope),
		RedirectURI:       kickUserOAuthRedirectURI,
		Pending:           pending,
		LastError:         strings.TrimSpace(m.lastError),
		HasAppCredentials: hasAppCredentials,
	}
	if !m.expiresAt.IsZero() {
		state.ExpiresAt = m.expiresAt.UnixMilli()
	}
	return state
}

func (m *KickUserAuthManager) begin(clientID string) (string, error) {
	if m == nil {
		return "", fmt.Errorf("stream authorization manager unavailable")
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return "", fmt.Errorf("connect Kick on the Connections page first so SleepySource has your Developer App Client ID")
	}
	verifier, err := randomOAuthValue(48)
	if err != nil {
		return "", err
	}
	state, err := randomOAuthValue(32)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.pendingVerifier = verifier
	m.pendingState = state
	m.pendingAt = time.Now()
	m.lastError = ""
	m.mu.Unlock()

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	// Kick documents a frontend workaround for 127.0.0.1 redirect URIs: a
	// sacrificial redirect query value must appear before redirect_uri so the
	// actual callback remains unchanged by their NextJS layer.
	q.Set("redirect", "127.0.0.1")
	q.Set("redirect_uri", kickUserOAuthRedirectURI)
	q.Set("scope", kickUserOAuthScopes)
	q.Set("code_challenge", oauthCodeChallenge(verifier))
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	return kickOAuthBase + "/oauth/authorize?" + q.Encode(), nil
}

type kickOAuthTokenResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    interface{} `json:"expires_in"`
	Scope        string      `json:"scope"`
}

func parseExpiresIn(v interface{}) time.Duration {
	seconds := int64(3600)
	switch n := v.(type) {
	case float64:
		if n > 0 {
			seconds = int64(n)
		}
	case json.Number:
		if parsed, err := n.Int64(); err == nil && parsed > 0 {
			seconds = parsed
		}
	case string:
		var parsed int64
		if _, err := fmt.Sscan(strings.TrimSpace(n), &parsed); err == nil && parsed > 0 {
			seconds = parsed
		}
	}
	if seconds < 60 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func requestKickUserToken(form url.Values) (kickOAuthTokenResponse, error) {
	req, err := http.NewRequest(http.MethodPost, kickOAuthBase+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return kickOAuthTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SleepySource/"+appVersion)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return kickOAuthTokenResponse{}, fmt.Errorf("could not reach Kick OAuth: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return kickOAuthTokenResponse{}, fmt.Errorf("Kick OAuth returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var token kickOAuthTokenResponse
	if err := dec.Decode(&token); err != nil {
		return kickOAuthTokenResponse{}, fmt.Errorf("Kick OAuth returned invalid token data")
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return kickOAuthTokenResponse{}, fmt.Errorf("Kick OAuth did not return a user access token")
	}
	return token, nil
}

func (m *KickUserAuthManager) finishAuthorization(code, state, clientID, clientSecret string) error {
	if m == nil {
		return fmt.Errorf("stream authorization manager unavailable")
	}
	code = strings.TrimSpace(code)
	state = strings.TrimSpace(state)
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if code == "" {
		return fmt.Errorf("Kick did not return an authorization code")
	}
	m.mu.Lock()
	pendingState := m.pendingState
	verifier := m.pendingVerifier
	pendingAt := m.pendingAt
	m.mu.Unlock()
	if pendingState == "" || verifier == "" || state == "" || state != pendingState || time.Since(pendingAt) > 10*time.Minute {
		return fmt.Errorf("Kick authorization session expired or did not match. Start authorization again")
	}
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("saved Kick Developer App credentials are unavailable; reconnect Kick in Chat Overlay")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("redirect_uri", kickUserOAuthRedirectURI)
	form.Set("code_verifier", verifier)
	token, err := requestKickUserToken(form)
	if err != nil {
		m.mu.Lock()
		m.lastError = err.Error()
		m.mu.Unlock()
		return err
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return fmt.Errorf("Kick OAuth did not return a refresh token")
	}
	m.mu.Lock()
	m.accessToken = strings.TrimSpace(token.AccessToken)
	m.refreshToken = strings.TrimSpace(token.RefreshToken)
	m.tokenType = strings.TrimSpace(token.TokenType)
	m.scope = strings.TrimSpace(token.Scope)
	m.expiresAt = time.Now().Add(parseExpiresIn(token.ExpiresIn))
	m.pendingState = ""
	m.pendingVerifier = ""
	m.pendingAt = time.Time{}
	m.lastError = ""
	err = m.saveLocked()
	m.mu.Unlock()
	return err
}

func (m *KickUserAuthManager) ensureUserAccessToken(clientID, clientSecret string) (string, error) {
	if m == nil {
		return "", fmt.Errorf("Authorize Stream Controls first")
	}
	m.mu.Lock()
	if strings.TrimSpace(m.accessToken) != "" && (m.expiresAt.IsZero() || time.Now().Add(45*time.Second).Before(m.expiresAt)) {
		token := m.accessToken
		m.mu.Unlock()
		return token, nil
	}
	refreshToken := strings.TrimSpace(m.refreshToken)
	m.mu.Unlock()
	if refreshToken == "" {
		return "", fmt.Errorf("Authorize Stream Controls first")
	}
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
		return "", fmt.Errorf("Kick Developer App credentials are unavailable; reconnect Kick in Chat Overlay")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", strings.TrimSpace(clientID))
	form.Set("client_secret", strings.TrimSpace(clientSecret))
	form.Set("refresh_token", refreshToken)
	token, err := requestKickUserToken(form)
	if err != nil {
		m.mu.Lock()
		m.lastError = "Stream Controls authorization expired. Authorize again."
		m.accessToken = ""
		m.refreshToken = ""
		m.expiresAt = time.Time{}
		_ = os.Remove(m.path)
		m.mu.Unlock()
		return "", fmt.Errorf("Stream Controls authorization expired. Authorize again")
	}
	m.mu.Lock()
	m.accessToken = strings.TrimSpace(token.AccessToken)
	if strings.TrimSpace(token.RefreshToken) != "" {
		m.refreshToken = strings.TrimSpace(token.RefreshToken)
	}
	m.tokenType = strings.TrimSpace(token.TokenType)
	if strings.TrimSpace(token.Scope) != "" {
		m.scope = strings.TrimSpace(token.Scope)
	}
	m.expiresAt = time.Now().Add(parseExpiresIn(token.ExpiresIn))
	m.lastError = ""
	saveErr := m.saveLocked()
	accessToken := m.accessToken
	m.mu.Unlock()
	if saveErr != nil {
		return "", saveErr
	}
	return accessToken, nil
}

func (m *KickUserAuthManager) localRefreshToken() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return strings.TrimSpace(m.refreshToken)
}

func (m *KickUserAuthManager) setError(err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if err == nil {
		m.lastError = ""
	} else {
		m.lastError = strings.TrimSpace(err.Error())
	}
	m.mu.Unlock()
}

func (m *ChatManager) appCredentials() (string, string, bool) {
	if m == nil {
		return "", "", false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	clientID := strings.TrimSpace(m.clientID)
	clientSecret := strings.TrimSpace(m.clientSecret)
	return clientID, clientSecret, clientID != "" && clientSecret != ""
}

func (a *App) handleKickUserAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, _, hasCredentials := a.chat.appCredentials()
	state := a.streamAuth.state(hasCredentials)
	writeJSON(w, map[string]any{
		"authorized":          state.Authorized,
		"scope":               state.Scope,
		"expires_at":          state.ExpiresAt,
		"redirect_uri":        state.RedirectURI,
		"pending":             state.Pending,
		"last_error":          state.LastError,
		"has_app_credentials": state.HasAppCredentials,
		"connected_channel":   a.chat.state().ConnectedChannel,
	})
}

func (a *App) handleKickUserAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clientID, _, ok := a.chat.appCredentials()
	chatState := a.chat.state()
	if !ok || strings.TrimSpace(chatState.ConnectedChannel) == "" {
		http.Error(w, "Connect Kick Channel on the Connections page first", http.StatusPreconditionFailed)
		return
	}
	authURL, err := a.streamAuth.begin(clientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	go openBrowser(authURL)
	writeJSON(w, map[string]any{
		"ok":           true,
		"pending":      true,
		"redirect_uri": kickUserOAuthRedirectURI,
		"message":      "Opening Kick authorization in your browser",
	})
}

func (a *App) handleKickUserAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if oauthErr := strings.TrimSpace(r.URL.Query().Get("error")); oauthErr != "" {
		description := strings.TrimSpace(r.URL.Query().Get("error_description"))
		if description == "" {
			description = oauthErr
		}
		a.streamAuth.setError(fmt.Errorf("Kick authorization was not completed: %s", description))
		serveKickOAuthResult(w, false)
		return
	}
	clientID, clientSecret, ok := a.chat.appCredentials()
	if !ok {
		a.streamAuth.setError(fmt.Errorf("Kick Developer App credentials are unavailable; reconnect Kick in Chat Overlay"))
		serveKickOAuthResult(w, false)
		return
	}
	if err := a.streamAuth.finishAuthorization(r.URL.Query().Get("code"), r.URL.Query().Get("state"), clientID, clientSecret); err != nil {
		a.streamAuth.setError(err)
		serveKickOAuthResult(w, false)
		return
	}
	serveKickOAuthResult(w, true)
}

func serveKickOAuthResult(w http.ResponseWriter, success bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if success {
		_, _ = io.WriteString(w, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>SleepySource</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#06080d;color:#eef5ff;font-family:Segoe UI,sans-serif}.card{width:min(520px,calc(100vw - 40px));padding:28px;border:1px solid #203047;border-radius:16px;background:#0d1119;text-align:center;box-shadow:0 18px 60px rgba(0,0,0,.35)}.ok{font-size:38px;color:#56d99a}.muted{color:#8fa3bc;line-height:1.6}</style></head><body><div class="card"><div class="ok">✓</div><h1>Stream Controls Authorized</h1><p class="muted">Kick authorization is complete. Return to SleepySource and you can update your stream title and category.</p></div></body></html>`)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>SleepySource</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#06080d;color:#eef5ff;font-family:Segoe UI,sans-serif}.card{width:min(520px,calc(100vw - 40px));padding:28px;border:1px solid #203047;border-radius:16px;background:#0d1119;text-align:center}.warn{font-size:38px;color:#ffbd5f}.muted{color:#8fa3bc;line-height:1.6}</style></head><body><div class="card"><div class="warn">!</div><h1>Authorization Not Completed</h1><p class="muted">Return to SleepySource for the specific error and try Authorize Stream Controls again.</p></div></body></html>`)
}

func (a *App) handleKickUserAuthDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	refreshToken := a.streamAuth.localRefreshToken()
	a.streamAuth.clear()
	if refreshToken != "" {
		go func(token string) {
			q := url.Values{}
			q.Set("token", token)
			q.Set("token_hint_type", "refresh_token")
			req, err := http.NewRequest(http.MethodPost, kickOAuthBase+"/oauth/revoke?"+q.Encode(), nil)
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("User-Agent", "SleepySource/"+appVersion)
			resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
			if err == nil && resp != nil {
				resp.Body.Close()
			}
		}(refreshToken)
	}
	writeJSON(w, map[string]any{"ok": true, "authorized": false})
}
