package agentusage

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var testNow = time.Date(2026, 8, 5, 18, 0, 0, 0, time.UTC)

func TestCodexAppServerHelper(t *testing.T) {
	if os.Getenv("AGENTUSAGE_CODEX_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	expectRPCMethod(t, scanner, "initialize")
	if err := encoder.Encode(map[string]any{
		"id": 1,
		"result": map[string]any{
			"userAgent": "agentusage-test",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Encode(map[string]any{
		"method": "remoteControl/status/changed",
		"params": map[string]any{"status": "disabled"},
	}); err != nil {
		t.Fatal(err)
	}
	expectRPCMethod(t, scanner, "initialized")
	expectRPCMethod(t, scanner, "account/rateLimits/read")
	if err := encoder.Encode(map[string]any{
		"id": 2,
		"result": map[string]any{
			"rateLimits": map[string]any{
				"limitId": "codex",
				"primary": map[string]any{
					"usedPercent": 12.5, "windowDurationMins": 300, "resetsAt": 1785956400,
				},
				"secondary": map[string]any{
					"usedPercent": 19, "windowDurationMins": 10080, "resetsAt": 1786482933,
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func expectRPCMethod(t *testing.T, scanner *bufio.Scanner, want string) {
	t.Helper()
	if !scanner.Scan() {
		t.Fatalf("missing %s request: %v", want, scanner.Err())
	}
	var request struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if request.Method != want {
		t.Fatalf("method = %q, want %q", request.Method, want)
	}
}

func helperCommand(ctx context.Context) *exec.Cmd {
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexAppServerHelper$")
	command.Env = append(os.Environ(), "AGENTUSAGE_CODEX_HELPER=1")
	return command
}

func testFetcher(t *testing.T, handler http.HandlerFunc) (*Fetcher, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		handler(w, request)
	}))
	t.Cleanup(server.Close)

	var codexStarts atomic.Int32
	fetcher := NewWithConfig(Config{HomeDir: t.TempDir()})
	fetcher.claudeURL = server.URL + "/claude"
	fetcher.tokenURL = server.URL + "/token"
	fetcher.startCodex = func(ctx context.Context) *exec.Cmd {
		codexStarts.Add(1)
		return helperCommand(ctx)
	}
	fetcher.run = func(context.Context, []byte, string, ...string) ([]byte, error) {
		return []byte(`{"claudeAiOauth":{"accessToken":"claude-secret"}}`), nil
	}
	fetcher.now = func() time.Time { return testNow }
	return fetcher, &requests, &codexStarts
}

func TestSnapshotMapsProvidersWithoutLeakingCredentials(t *testing.T) {
	fetcher, _, _ := testFetcher(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/claude" {
			http.NotFound(w, request)
			return
		}
		if got := request.Header.Get("Authorization"); got != "Bearer claude-secret" {
			t.Errorf("Claude Authorization = %q", got)
		}
		if got := request.Header.Get("anthropic-beta"); got != claudeOAuthBeta {
			t.Errorf("Claude beta header = %q", got)
		}
		_, _ = w.Write([]byte(`{
		  "five_hour":{"utilization":8,"resets_at":"2026-08-05T19:40:00Z"},
		  "seven_day":{"utilization":19,"resets_at":"2026-08-08T21:00:00Z"},
		  "limits":[{"kind":"weekly_scoped","percent":35,"resets_at":"2026-08-08T21:00:00Z","scope":{"model":{"display_name":"Fable"}}}]
		}`))
	})

	snapshot := fetcher.Snapshot(context.Background())
	if len(snapshot.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(snapshot.Providers))
	}
	codex := snapshot.Providers[0]
	if codex.ID != "codex" || codex.Error != "" || len(codex.Windows) != 2 {
		t.Fatalf("Codex = %+v", codex)
	}
	if codex.Windows[0].Label != "5h window" || codex.Windows[1].Label != "1w window" {
		t.Errorf("Codex windows = %+v", codex.Windows)
	}
	claude := snapshot.Providers[1]
	if claude.ID != "claude" || claude.Error != "" || len(claude.Windows) != 3 {
		t.Fatalf("Claude = %+v", claude)
	}
	if got := claude.Windows[2]; got.Scope != "Fable" || got.UsedPercent != 35 {
		t.Errorf("scoped window = %+v", got)
	}
	if !codex.FetchedAt.Equal(testNow) || !claude.FetchedAt.Equal(testNow) {
		t.Errorf("provider fetched times = %s, %s", codex.FetchedAt, claude.FetchedAt)
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("snapshot leaked credentials: %s", raw)
	}
}

func TestSnapshotCachesAndReturnsCopies(t *testing.T) {
	fetcher, requests, codexStarts := testFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":8,"resets_at":"2026-08-05T19:40:00Z"}}`))
	})
	first := fetcher.Snapshot(context.Background())
	first.Providers[0].Windows[0].UsedPercent = 99
	second := fetcher.Snapshot(context.Background())
	if second.Providers[0].Windows[0].UsedPercent != 12.5 {
		t.Fatalf("caller mutated cache: %+v", second.Providers[0].Windows[0])
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("Claude requests = %d, want 1", got)
	}
	if got := codexStarts.Load(); got != 1 {
		t.Fatalf("Codex app-server starts = %d, want 1", got)
	}
}

func TestSnapshotKeepsLastGoodProviderOnRefreshError(t *testing.T) {
	failClaude := false
	fetcher, _, _ := testFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
		if failClaude {
			http.Error(w, "limited", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":8,"resets_at":"2026-08-05T19:40:00Z"}}`))
	})
	first := fetcher.Snapshot(context.Background())
	if first.Providers[1].Stale {
		t.Fatal("first Claude snapshot is stale")
	}
	failClaude = true
	fetcher.cachedAt = time.Time{}
	second := fetcher.Snapshot(context.Background())
	claude := second.Providers[1]
	if !claude.Stale || len(claude.Windows) != 1 || !strings.Contains(claude.Error, "HTTP 429") {
		t.Fatalf("stale Claude = %+v", claude)
	}
	if !claude.FetchedAt.Equal(testNow) {
		t.Errorf("stale fetched_at changed: %s", claude.FetchedAt)
	}
}

func TestSnapshotHonorsRetryAfter(t *testing.T) {
	var claudeRequests atomic.Int32
	fetcher, _, _ := testFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
		claudeRequests.Add(1)
		w.Header().Set("Retry-After", "1800")
		http.Error(w, "limited", http.StatusTooManyRequests)
	})
	first := fetcher.Snapshot(context.Background())
	if !strings.Contains(first.Providers[1].Error, "HTTP 429") {
		t.Fatalf("first Claude error = %q", first.Providers[1].Error)
	}
	fetcher.cachedAt = time.Time{}
	second := fetcher.Snapshot(context.Background())
	if got := claudeRequests.Load(); got != 1 {
		t.Fatalf("Claude requests = %d, want retry suppression", got)
	}
	if !strings.Contains(second.Providers[1].Error, "provider rate limited") {
		t.Fatalf("second Claude error = %q", second.Providers[1].Error)
	}
}

func TestClaudeRefreshesExpiredTokenAndPersistsKeychainRotation(t *testing.T) {
	stored := []byte(fmt.Sprintf(`{
		"claudeAiOauth":{
			"accessToken":"expired-access",
			"refreshToken":"old-refresh",
			"expiresAt":%d,
			"scopes":["user:inference"],
			"subscriptionType":"max"
		}
	}`, testNow.Add(-time.Hour).UnixMilli()))
	fetcher, _, _ := testFetcher(t, func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["refresh_token"] != "old-refresh" || body["client_id"] != claudeOAuthClientID {
				t.Errorf("token body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
		case "/claude":
			if got := request.Header.Get("Authorization"); got != "Bearer new-access" {
				t.Errorf("Claude Authorization = %q", got)
			}
			_, _ = w.Write([]byte(`{"five_hour":{"utilization":8,"resets_at":"2026-08-05T19:40:00Z"}}`))
		default:
			http.NotFound(w, request)
		}
	})
	fetcher.username = "test-user"
	fetcher.run = func(_ context.Context, input []byte, name string, args ...string) ([]byte, error) {
		if input == nil {
			return stored, nil
		}
		if name != "/usr/bin/security" || len(args) != 1 || args[0] != "-i" {
			t.Fatalf("Keychain write command = %s %v", name, args)
		}
		if strings.Contains(string(input), "new-access") || strings.Contains(string(input), "new-refresh") {
			t.Fatal("Keychain write exposed plaintext tokens")
		}
		stored = decodeKeychainUpdate(t, input)
		return nil, nil
	}

	provider, err := fetcher.fetchClaude(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.Windows) != 1 {
		t.Fatalf("Claude windows = %+v", provider.Windows)
	}
	auth, err := parseClaudeCredentials(stored)
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken != "new-access" || auth.RefreshToken != "new-refresh" {
		t.Fatalf("persisted tokens = access %q refresh %q", auth.AccessToken, auth.RefreshToken)
	}
	if got := string(auth.oauth["subscriptionType"]); got != `"max"` {
		t.Errorf("subscriptionType = %s", got)
	}
}

func TestClaudeDoesNotOverwriteConcurrentRotation(t *testing.T) {
	oldCredentials := []byte(fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"old-access","refreshToken":"old-refresh","expiresAt":%d}}`, testNow.Add(-time.Hour).UnixMilli()))
	newCredentials := []byte(fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"rotated-access","refreshToken":"rotated-refresh","expiresAt":%d}}`, testNow.Add(time.Hour).UnixMilli()))
	var reads int
	fetcher, _, _ := testFetcher(t, func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"our-access","refresh_token":"our-refresh","expires_in":3600}`))
		case "/claude":
			if got := request.Header.Get("Authorization"); got != "Bearer rotated-access" {
				t.Errorf("Claude Authorization = %q", got)
			}
			_, _ = w.Write([]byte(`{"five_hour":{"utilization":8,"resets_at":"2026-08-05T19:40:00Z"}}`))
		default:
			http.NotFound(w, request)
		}
	})
	fetcher.run = func(_ context.Context, input []byte, _ string, _ ...string) ([]byte, error) {
		if input != nil {
			t.Fatal("concurrent rotation was overwritten")
		}
		reads++
		if reads == 1 {
			return oldCredentials, nil
		}
		return newCredentials, nil
	}
	if _, err := fetcher.fetchClaude(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reads != 2 {
		t.Fatalf("Keychain reads = %d, want 2", reads)
	}
}

func TestClaudeRefreshPersistsPrivateCredentialFile(t *testing.T) {
	fetcher, _, _ := testFetcher(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/token" {
			http.NotFound(w, request)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"new-access","expires_in":3600}`))
	})
	credentialsDir := filepath.Join(fetcher.homeDir, ".claude")
	if err := os.MkdirAll(credentialsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	credentialsPath := filepath.Join(credentialsDir, ".credentials.json")
	credentials := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"expired-access","refreshToken":"old-refresh","expiresAt":%d}}`, testNow.Add(-time.Hour).UnixMilli())
	if err := os.WriteFile(credentialsPath, []byte(credentials), 0o644); err != nil {
		t.Fatal(err)
	}
	fetcher.run = func(context.Context, []byte, string, ...string) ([]byte, error) {
		t.Fatal("credential file refresh accessed Keychain")
		return nil, nil
	}

	token, err := fetcher.claudeToken(context.Background(), false, "")
	if err != nil {
		t.Fatal(err)
	}
	if token != "new-access" {
		t.Fatalf("token = %q", token)
	}
	info, err := os.Stat(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("credential mode = %o", got)
	}
}

func decodeKeychainUpdate(t *testing.T, input []byte) []byte {
	t.Helper()
	const marker = ` -X "`
	command := string(input)
	start := strings.Index(command, marker)
	if start < 0 {
		t.Fatalf("Keychain command has no hex payload: %q", command)
	}
	start += len(marker)
	end := strings.Index(command[start:], `"`)
	if end < 0 {
		t.Fatalf("Keychain command has unterminated hex payload: %q", command)
	}
	raw, err := hex.DecodeString(command[start : start+end])
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestHelpersClampAndParseProviderData(t *testing.T) {
	windows := claudeWindows(claudeUsage{
		SevenDaySonnet: &claudeUsageWindow{Utilization: 150},
		SevenDayOpus:   &claudeUsageWindow{Utilization: -2},
		Limits: []claudeLimit{{
			Kind: "weekly_scoped", Percent: 37,
			Scope: &claudeScope{Model: &claudeModel{DisplayName: "Sonnet"}},
		}},
	})
	if len(windows) != 2 || windows[0].UsedPercent != 37 || windows[1].UsedPercent != 0 {
		t.Fatalf("windows = %+v", windows)
	}
	if got := parseRetryAfter("120", testNow); got != 2*time.Minute {
		t.Errorf("retry after = %s", got)
	}
}
