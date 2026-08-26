package agentusage

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	claudeUsageURL      = "https://api.anthropic.com/api/oauth/usage"
	claudeTokenURL      = "https://platform.claude.com/v1/oauth/token"
	claudeOAuthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	claudeKeychainItem  = "Claude Code-credentials"
	claudeOAuthBeta     = "oauth-2025-04-20"
	refreshBeforeExpiry = 5 * time.Minute
	weekSeconds         = int64(7 * 24 * 60 * 60)
)

func (f *Fetcher) fetchClaude(ctx context.Context) ([]Window, error) {
	token, err := f.claudeToken(ctx, "")
	if err != nil {
		return nil, err
	}
	payload, err := f.fetchClaudeUsage(ctx, token)
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) && statusErr.Code == http.StatusUnauthorized {
		token, err = f.claudeToken(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("Claude usage returned HTTP 401 and token refresh failed: %w", err)
		}
		payload, err = f.fetchClaudeUsage(ctx, token)
	}
	if err != nil {
		return nil, fmt.Errorf("Claude usage: %w", err)
	}
	windows := claudeWindows(payload)
	if len(windows) == 0 {
		return nil, fmt.Errorf("Claude returned no usage windows")
	}
	return windows, nil
}

func (f *Fetcher) fetchClaudeUsage(ctx context.Context, token string) (claudeUsage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, f.claudeURL, nil)
	if err != nil {
		return claudeUsage{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("anthropic-beta", claudeOAuthBeta)
	var payload claudeUsage
	if err := f.fetchJSON(request, &payload); err != nil {
		return claudeUsage{}, err
	}
	return payload, nil
}

// claudeCredentials holds the raw credential document plus the parsed OAuth
// fields. The raw bytes keep every unknown field intact for a later save.
type claudeCredentials struct {
	raw          []byte
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
}

// claudeToken returns a usable access token. It refreshes and persists the
// credentials when needed. rejectedToken names a token that the API rejected;
// the function never returns that token again.
//
// The Fetcher keeps the last credentials in memory. This avoids a store read
// on every poll, and it keeps a rotated token safe when a save fails.
func (f *Fetcher) claudeToken(ctx context.Context, rejectedToken string) (string, error) {
	f.credMu.Lock()
	defer f.credMu.Unlock()

	if f.claudeLoaded && f.claudeAuth.AccessToken != rejectedToken && f.claudeTokenUsable(f.claudeAuth) {
		f.saveDirtyCredentials(ctx)
		return f.claudeAuth.AccessToken, nil
	}

	stored, path, err := f.loadClaudeCredentials(ctx)
	if err == nil {
		f.adoptStoredCredentials(stored, path)
	} else if !f.claudeLoaded {
		return "", err
	}
	auth := f.claudeAuth
	if auth.AccessToken != rejectedToken && f.claudeTokenUsable(auth) {
		return auth.AccessToken, nil
	}
	if auth.RefreshToken == "" {
		return "", fmt.Errorf("Claude OAuth refresh token is missing (run `claude auth login`)")
	}

	accessToken, refreshToken, expiresAt, err := f.refreshClaudeToken(ctx, auth.RefreshToken)
	if err != nil {
		// Claude Code can refresh and rotate the credential while this request
		// is in flight. Prefer its new credential over a false error.
		if latest, ok := f.rotatedCredentials(ctx, auth); ok {
			return latest.AccessToken, nil
		}
		return "", err
	}
	if refreshToken == "" {
		refreshToken = auth.RefreshToken
	}

	// Do not overwrite a credential that Claude Code rotated after this
	// refresh started.
	if latest, ok := f.rotatedCredentials(ctx, auth); ok {
		return latest.AccessToken, nil
	}

	raw, err := auth.updated(accessToken, refreshToken, expiresAt)
	if err != nil {
		return "", fmt.Errorf("encode refreshed Claude credentials: %w", err)
	}
	f.claudeAuth = claudeCredentials{
		raw:          raw,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}
	f.claudeLoaded = true
	f.claudeDirty = true
	// A failed save is not fatal: the rotated credential stays in memory, and
	// the save runs again on the next poll.
	f.saveDirtyCredentials(ctx)
	return accessToken, nil
}

// adoptStoredCredentials replaces the in-memory credentials with the stored
// ones, unless the memory holds a newer un-persisted rotation. The caller
// must hold f.credMu.
func (f *Fetcher) adoptStoredCredentials(stored claudeCredentials, path string) {
	f.claudePath = path
	if f.claudeDirty && !f.claudeTokenUsable(stored) {
		return
	}
	f.claudeAuth = stored
	f.claudeLoaded = true
	f.claudeDirty = false
}

// rotatedCredentials reloads the store and adopts a usable credential that
// another process rotated after previous was read. The caller must hold
// f.credMu.
func (f *Fetcher) rotatedCredentials(ctx context.Context, previous claudeCredentials) (claudeCredentials, bool) {
	latest, path, err := f.loadClaudeCredentials(ctx)
	if err != nil || !credentialsChanged(previous, latest) || !f.claudeTokenUsable(latest) {
		return claudeCredentials{}, false
	}
	f.claudePath = path
	f.claudeAuth = latest
	f.claudeLoaded = true
	f.claudeDirty = false
	return latest, true
}

// saveDirtyCredentials persists an un-persisted rotation. A failure keeps the
// dirty flag, so the save runs again later. The caller must hold f.credMu.
func (f *Fetcher) saveDirtyCredentials(ctx context.Context) {
	if !f.claudeDirty {
		return
	}
	if err := f.saveClaudeCredentials(ctx, f.claudePath, f.claudeAuth.raw); err == nil {
		f.claudeDirty = false
	}
}

func credentialsChanged(previous, current claudeCredentials) bool {
	return previous.AccessToken != current.AccessToken || previous.RefreshToken != current.RefreshToken
}

func (f *Fetcher) claudeTokenUsable(auth claudeCredentials) bool {
	if auth.AccessToken == "" {
		return false
	}
	if auth.ExpiresAt == 0 {
		return true
	}
	return auth.ExpiresAt > f.now().Add(refreshBeforeExpiry).UnixMilli()
}

// loadClaudeCredentials reads the credential file, or the macOS Keychain item
// when the file does not exist. An empty path means the Keychain.
func (f *Fetcher) loadClaudeCredentials(ctx context.Context) (claudeCredentials, string, error) {
	path := filepath.Join(f.homeDir, ".claude", ".credentials.json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		path = ""
		args := []string{"find-generic-password", "-s", claudeKeychainItem, "-w"}
		if f.username != "" {
			args = append(args, "-a", f.username)
		}
		raw, err = f.run(ctx, nil, "/usr/bin/security", args...)
	}
	if err != nil {
		return claudeCredentials{}, path, fmt.Errorf("Claude credentials not found")
	}
	if len(raw) > maxResponse {
		return claudeCredentials{}, path, fmt.Errorf("Claude credentials are invalid")
	}
	auth, err := parseClaudeCredentials(raw)
	if err != nil {
		return claudeCredentials{}, path, err
	}
	return auth, path, nil
}

func parseClaudeCredentials(raw []byte) (claudeCredentials, error) {
	var document struct {
		OAuth struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresAt    int64  `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return claudeCredentials{}, fmt.Errorf("Claude OAuth credentials are invalid")
	}
	if document.OAuth.AccessToken == "" {
		return claudeCredentials{}, fmt.Errorf("Claude OAuth credentials are incomplete")
	}
	return claudeCredentials{
		raw:          raw,
		AccessToken:  document.OAuth.AccessToken,
		RefreshToken: document.OAuth.RefreshToken,
		ExpiresAt:    document.OAuth.ExpiresAt,
	}, nil
}

// updated returns the credential document with new OAuth tokens. It rebuilds
// the document from the raw bytes, so every unknown field stays intact.
func (credentials claudeCredentials) updated(accessToken, refreshToken string, expiresAt int64) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(credentials.raw, &root); err != nil {
		return nil, err
	}
	oauth := map[string]json.RawMessage{}
	if rawOAuth, ok := root["claudeAiOauth"]; ok {
		if err := json.Unmarshal(rawOAuth, &oauth); err != nil {
			return nil, err
		}
	}
	for key, value := range map[string]any{
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
		"expiresAt":    expiresAt,
	} {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		oauth[key] = raw
	}
	rawOAuth, err := json.Marshal(oauth)
	if err != nil {
		return nil, err
	}
	root["claudeAiOauth"] = rawOAuth
	return json.Marshal(root)
}

func (f *Fetcher) refreshClaudeToken(ctx context.Context, refreshToken string) (string, string, int64, error) {
	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     claudeOAuthClientID,
	})
	if err != nil {
		return "", "", 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", "", 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("anthropic-beta", claudeOAuthBeta)
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := f.fetchJSON(request, &token); err != nil {
		return "", "", 0, fmt.Errorf("Claude token refresh: %w (run `claude auth login` if the refresh token was revoked)", err)
	}
	if token.AccessToken == "" || token.ExpiresIn <= 0 {
		return "", "", 0, fmt.Errorf("Claude token refresh returned incomplete credentials")
	}
	expiresAt := f.now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli()
	return token.AccessToken, token.RefreshToken, expiresAt, nil
}

// saveClaudeCredentials writes the credential document to the given file
// path, or to the macOS Keychain when the path is empty.
func (f *Fetcher) saveClaudeCredentials(ctx context.Context, path string, raw []byte) error {
	if path != "" {
		return writePrivateFile(path, raw)
	}
	if f.username == "" {
		return fmt.Errorf("current username is unavailable")
	}
	// security(1) warns that -w and -X command arguments expose secrets. Its
	// interactive mode accepts the command over stdin; Claude Code uses the
	// same mode.
	command := fmt.Sprintf("add-generic-password -U -a %q -s %q -X %q\n",
		f.username, claudeKeychainItem, hex.EncodeToString(raw))
	if _, err := f.run(ctx, []byte(command), "/usr/bin/security", "-i"); err != nil {
		return fmt.Errorf("update Claude Code Keychain item: %w", err)
	}
	return nil
}

func writePrivateFile(path string, raw []byte) (returnErr error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if returnErr != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

type claudeUsageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type claudeLimit struct {
	Kind     string       `json:"kind"`
	Percent  float64      `json:"percent"`
	ResetsAt string       `json:"resets_at"`
	Scope    *claudeScope `json:"scope"`
}

type claudeScope struct {
	Model *claudeModel `json:"model"`
}

type claudeModel struct {
	DisplayName string `json:"display_name"`
}

type claudeUsage struct {
	FiveHour       *claudeUsageWindow `json:"five_hour"`
	SevenDay       *claudeUsageWindow `json:"seven_day"`
	SevenDaySonnet *claudeUsageWindow `json:"seven_day_sonnet"`
	SevenDayOpus   *claudeUsageWindow `json:"seven_day_opus"`
	Limits         []claudeLimit      `json:"limits"`
}

func claudeWindows(payload claudeUsage) []Window {
	var windows []Window
	add := func(id, label, scope string, seconds int64, percent float64, resetsAt string) {
		windows = append(windows, Window{
			ID:            id,
			Label:         label,
			Scope:         scope,
			UsedPercent:   clampPercent(percent),
			WindowSeconds: seconds,
			ResetsAt:      parseTime(resetsAt),
		})
	}
	if window := payload.FiveHour; window != nil {
		add("five-hour", "5h window", "", 5*60*60, window.Utilization, window.ResetsAt)
	}
	if window := payload.SevenDay; window != nil {
		add("seven-day", "1w window", "all models", weekSeconds, window.Utilization, window.ResetsAt)
	}

	seenScopes := map[string]bool{}
	for _, limit := range payload.Limits {
		if limit.Kind != "weekly_scoped" || limit.Scope == nil || limit.Scope.Model == nil {
			continue
		}
		scope := strings.TrimSpace(limit.Scope.Model.DisplayName)
		key := strings.ToLower(scope)
		if scope == "" || seenScopes[key] {
			continue
		}
		seenScopes[key] = true
		add("weekly-"+slug(scope), "1w window", scope, weekSeconds, limit.Percent, limit.ResetsAt)
	}

	// A scoped limit for the same model family supersedes the legacy field,
	// whatever the exact display name is ("Sonnet", "Claude Sonnet 4.5", ...).
	scopedFamily := func(family string) bool {
		for key := range seenScopes {
			if strings.Contains(key, family) {
				return true
			}
		}
		return false
	}
	if window := payload.SevenDaySonnet; window != nil && !scopedFamily("sonnet") {
		add("weekly-sonnet", "1w window", "Sonnet", weekSeconds, window.Utilization, window.ResetsAt)
	}
	if window := payload.SevenDayOpus; window != nil && !scopedFamily("opus") {
		add("weekly-opus", "1w window", "Opus", weekSeconds, window.Utilization, window.ResetsAt)
	}
	return windows
}
