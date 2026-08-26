package agentusage

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"sync"
	"time"
)

const (
	defaultCacheTTL       = 2 * time.Minute
	defaultRequestTimeout = 10 * time.Second
)

// Config overrides host integration settings. Zero values use local CLI and
// operating-system defaults.
type Config struct {
	CacheTTL     time.Duration
	HTTPClient   *http.Client
	HomeDir      string
	CodexHome    string
	CodexCommand string
}

type commandRunner func(context.Context, []byte, string, ...string) ([]byte, error)

// Fetcher serializes and caches remote reads. This prevents multiple browser
// tabs from polling provider APIs independently.
type Fetcher struct {
	homeDir        string
	codexHome      string
	codexCommand   string
	startCodex     func(context.Context) *exec.Cmd
	client         *http.Client
	run            commandRunner
	now            func() time.Time
	cacheTTL       time.Duration
	requestTimeout time.Duration
	claudeURL      string
	tokenURL       string
	username       string

	mu       sync.Mutex
	cache    Snapshot
	cachedAt time.Time
	retryAt  map[string]time.Time
}

// New creates a Fetcher for the current operating-system user.
func New() *Fetcher {
	return NewWithConfig(Config{})
}

// NewWithConfig creates a Fetcher with explicit host integration settings.
func NewWithConfig(config Config) *Fetcher {
	home := config.HomeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	codexHome := config.CodexHome
	if codexHome == "" {
		codexHome = os.Getenv("CODEX_HOME")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultRequestTimeout}
	}
	cacheTTL := config.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	codexCommand := config.CodexCommand
	if codexCommand == "" {
		codexCommand = "codex"
	}
	username := os.Getenv("USER")
	if current, err := user.Current(); err == nil && current.Username != "" {
		username = current.Username
	}
	return &Fetcher{
		homeDir:        home,
		codexHome:      codexHome,
		codexCommand:   codexCommand,
		client:         client,
		run:            outputCommand,
		now:            time.Now,
		cacheTTL:       cacheTTL,
		requestTimeout: defaultRequestTimeout,
		claudeURL:      claudeUsageURL,
		tokenURL:       claudeTokenURL,
		username:       username,
		retryAt:        make(map[string]time.Time),
	}
}

func outputCommand(ctx context.Context, input []byte, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	return command.Output()
}

// Snapshot returns both providers even when one is unavailable. A failed
// refresh keeps the last good values and marks them stale.
func (f *Fetcher) Snapshot(ctx context.Context) Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := f.now()
	if !f.cachedAt.IsZero() && now.Sub(f.cachedAt) < f.cacheTTL {
		return cloneSnapshot(f.cache)
	}

	type result struct {
		provider Provider
		err      error
	}
	results := make(chan result, 2)
	codexRetryAt := f.retryAt["codex"]
	claudeRetryAt := f.retryAt["claude"]
	go func() {
		if now.Before(codexRetryAt) {
			results <- result{provider: Provider{ID: "codex", Name: "Codex"}, err: retryBlocked(codexRetryAt, now)}
			return
		}
		provider, err := f.fetchCodex(ctx)
		results <- result{provider: provider, err: err}
	}()
	go func() {
		if now.Before(claudeRetryAt) {
			results <- result{provider: Provider{ID: "claude", Name: "Claude Code"}, err: retryBlocked(claudeRetryAt, now)}
			return
		}
		provider, err := f.fetchClaude(ctx)
		results <- result{provider: provider, err: err}
	}()

	previous := make(map[string]Provider, len(f.cache.Providers))
	for _, provider := range f.cache.Providers {
		previous[provider.ID] = provider
	}
	providers := make(map[string]Provider, 2)
	for range 2 {
		res := <-results
		if res.err == nil {
			delete(f.retryAt, res.provider.ID)
			res.provider.FetchedAt = now
			providers[res.provider.ID] = res.provider
			continue
		}
		var statusErr *httpStatusError
		if errors.As(res.err, &statusErr) && statusErr.RetryAfter > 0 {
			f.retryAt[res.provider.ID] = now.Add(statusErr.RetryAfter)
		}
		if old, ok := previous[res.provider.ID]; ok && len(old.Windows) > 0 {
			old.Stale = true
			old.Error = res.err.Error()
			providers[old.ID] = old
			continue
		}
		res.provider.Windows = []Window{}
		res.provider.Error = res.err.Error()
		providers[res.provider.ID] = res.provider
	}

	f.cache = Snapshot{
		Providers: []Provider{providers["codex"], providers["claude"]},
		UpdatedAt: now,
	}
	f.cachedAt = now
	return cloneSnapshot(f.cache)
}
