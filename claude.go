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
)

func (f *Fetcher) fetchClaude(ctx context.Context) (Provider, error) {
	provider := Provider{ID: "claude", Name: "Claude Code"}
	token, err := f.claudeToken(ctx, false, "")
	if err != nil {
		return provider, err
	}
	payload, err := f.fetchClaudeUsage(ctx, token)
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) && statusErr.Code == http.StatusUnauthorized {
		token, refreshErr := f.claudeToken(ctx, true, token)
		if refreshErr != nil {
			return provider, fmt.Errorf("Claude usage returned HTTP 401 and token refresh failed: %w", refreshErr)
		}
		payload, err = f.fetchClaudeUsage(ctx, token)
	}
	if err != nil {
		return provider, fmt.Errorf("Claude usage: %w", err)
	}
	provider.Windows = claudeWindows(payload)
	if len(provider.Windows) == 0 {
		return provider, fmt.Errorf("Claude returned no usage windows")
	}
	return provider, nil
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

type claudeCredentials struct {
	root         map[string]json.RawMessage
	oauth        map[string]json.RawMessage
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
}

type claudeCredentialSource struct {
	path string
}

func (f *Fetcher) claudeToken(ctx context.Context, force bool, rejectedToken string) (string, error) {
	auth, source, err := f.loadClaudeCredentials(ctx)
	if err != nil {
		return "", err
	}
	if rejectedToken != "" && auth.AccessToken != rejectedToken && f.claudeTokenUsable(auth) {
		return auth.AccessToken, nil
	}
	if !force && f.claudeTokenUsable(auth) {
		return auth.AccessToken, nil
	}
	if auth.RefreshToken == "" {
		return "", fmt.Errorf("Claude OAuth refresh token is missing (run `claude auth login`)")
	}

	accessToken, refreshToken, expiresAt, err := f.refreshClaudeToken(ctx, auth.RefreshToken)
	if err != nil {
		// Claude Code can refresh and rotate the credential while this request is
		// in flight. Prefer its new credential over a false error.
		latest, _, loadErr := f.loadClaudeCredentials(ctx)
		if loadErr == nil && credentialsChanged(auth, latest) && f.claudeTokenUsable(latest) {
			return latest.AccessToken, nil
		}
		return "", err
	}
	if refreshToken == "" {
		refreshToken = auth.RefreshToken
	}

	// Avoid overwriting a credential that Claude Code rotated after this
	// refresh started.
	latest, _, loadErr := f.loadClaudeCredentials(ctx)
	if loadErr == nil && credentialsChanged(auth, latest) && f.claudeTokenUsable(latest) {
		return latest.AccessToken, nil
	}

	raw, err := auth.updated(accessToken, refreshToken, expiresAt)
	if err != nil {
		return "", fmt.Errorf("encode refreshed Claude credentials: %w", err)
	}
	if err := f.saveClaudeCredentials(ctx, source, raw); err != nil {
		return "", fmt.Errorf("persist refreshed Claude credentials: %w", err)
	}
	return accessToken, nil
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

func (f *Fetcher) loadClaudeCredentials(ctx context.Context) (claudeCredentials, claudeCredentialSource, error) {
	path := filepath.Join(f.homeDir, ".claude", ".credentials.json")
	raw, err := os.ReadFile(path)
	source := claudeCredentialSource{path: path}
	if errors.Is(err, os.ErrNotExist) {
		source.path = ""
		args := []string{"find-generic-password", "-s", claudeKeychainItem, "-w"}
		if f.username != "" {
			args = append(args, "-a", f.username)
		}
		raw, err = f.run(ctx, nil, "/usr/bin/security", args...)
	}
	if err != nil {
		return claudeCredentials{}, source, fmt.Errorf("Claude credentials not found")
	}
	if len(raw) > maxResponse {
		return claudeCredentials{}, source, fmt.Errorf("Claude credentials are invalid")
	}
	auth, err := parseClaudeCredentials(raw)
	if err != nil {
		return claudeCredentials{}, source, err
	}
	return auth, source, nil
}

func parseClaudeCredentials(raw []byte) (claudeCredentials, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return claudeCredentials{}, fmt.Errorf("Claude OAuth credentials are invalid")
	}
	var oauth map[string]json.RawMessage
	if err := json.Unmarshal(root["claudeAiOauth"], &oauth); err != nil {
		return claudeCredentials{}, fmt.Errorf("Claude OAuth credentials are incomplete")
	}
	var values struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
	}
	if err := json.Unmarshal(root["claudeAiOauth"], &values); err != nil || values.AccessToken == "" {
		return claudeCredentials{}, fmt.Errorf("Claude OAuth credentials are incomplete")
	}
	return claudeCredentials{
		root:         root,
		oauth:        oauth,
		AccessToken:  values.AccessToken,
		RefreshToken: values.RefreshToken,
		ExpiresAt:    values.ExpiresAt,
	}, nil
}

func (credentials claudeCredentials) updated(accessToken, refreshToken string, expiresAt int64) ([]byte, error) {
	for key, value := range map[string]any{
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
		"expiresAt":    expiresAt,
	} {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		credentials.oauth[key] = raw
	}
	oauth, err := json.Marshal(credentials.oauth)
	if err != nil {
		return nil, err
	}
	credentials.root["claudeAiOauth"] = oauth
	return json.Marshal(credentials.root)
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

func (f *Fetcher) saveClaudeCredentials(ctx context.Context, source claudeCredentialSource, raw []byte) error {
	if source.path != "" {
		return writePrivateFile(source.path, raw)
	}
	if f.username == "" {
		return fmt.Errorf("current username is unavailable")
	}
	// security(1) warns that -w and -X command arguments expose secrets. Its
	// interactive mode accepts the command over stdin, matching Claude Code.
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
	appendWindow := func(id, label, scope string, seconds int64, window *claudeUsageWindow) {
		if window == nil {
			return
		}
		windows = append(windows, Window{
			ID:            id,
			Label:         label,
			Scope:         scope,
			UsedPercent:   clampPercent(window.Utilization),
			WindowSeconds: seconds,
			ResetsAt:      parseTime(window.ResetsAt),
		})
	}
	appendWindow("five-hour", "5h window", "", 5*60*60, payload.FiveHour)
	appendWindow("seven-day", "1w window", "all models", 7*24*60*60, payload.SevenDay)

	seenScopes := map[string]bool{}
	for _, limit := range payload.Limits {
		if limit.Kind != "weekly_scoped" || limit.Scope == nil || limit.Scope.Model == nil {
			continue
		}
		scope := strings.TrimSpace(limit.Scope.Model.DisplayName)
		if scope == "" || seenScopes[strings.ToLower(scope)] {
			continue
		}
		seenScopes[strings.ToLower(scope)] = true
		windows = append(windows, Window{
			ID:            "weekly-" + slug(scope),
			Label:         "1w window",
			Scope:         scope,
			UsedPercent:   clampPercent(limit.Percent),
			WindowSeconds: 7 * 24 * 60 * 60,
			ResetsAt:      parseTime(limit.ResetsAt),
		})
	}
	if payload.SevenDaySonnet != nil && !seenScopes["sonnet"] {
		appendWindow("weekly-sonnet", "1w window", "Sonnet", 7*24*60*60, payload.SevenDaySonnet)
	}
	if payload.SevenDayOpus != nil && !seenScopes["opus"] {
		appendWindow("weekly-opus", "1w window", "Opus", 7*24*60*60, payload.SevenDayOpus)
	}
	return windows
}
