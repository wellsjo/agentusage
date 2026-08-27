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
	// errorCacheTTL caps how long a snapshot with no data at all is served.
	// The short cap lets the module recover quickly from a start-up failure.
	errorCacheTTL = 15 * time.Second
)

// Config overrides host integration settings. Zero values use the local CLI
// and operating-system defaults.
type Config struct {
	CacheTTL     time.Duration
	HTTPClient   *http.Client
	HomeDir      string
	CodexHome    string
	CodexCommand string
}

type commandRunner func(context.Context, []byte, string, ...string) ([]byte, error)

// providerSpec binds a provider identity to its fetch function. This table is
// the single source of truth for the provider set and for the output order.
type providerSpec struct {
	id    string
	name  string
	fetch func(*Fetcher, context.Context) ([]Window, error)
}

var providerSpecs = []providerSpec{
	{id: ProviderIDCodex, name: "Codex", fetch: (*Fetcher).fetchCodex},
	{id: ProviderIDClaude, name: "Claude Code", fetch: (*Fetcher).fetchClaude},
}

// Fetcher serializes and caches remote reads. One shared refresh serves all
// concurrent callers, so multiple browser tabs cause one provider read.
type Fetcher struct {
	homeDir        string
	codexHome      string
	startCodex     func(context.Context) *exec.Cmd
	client         *http.Client
	run            commandRunner
	now            func() time.Time
	cacheTTL       time.Duration
	requestTimeout time.Duration
	claudeURL      string
	tokenURL       string
	username       string

	// credMu guards the in-memory copy of the Claude credentials.
	credMu       sync.Mutex
	claudeAuth   claudeCredentials
	claudePath   string
	claudeLoaded bool
	claudeDirty  bool

	mu       sync.Mutex
	cache    Snapshot
	cachedAt time.Time
	retryAt  map[string]time.Time
	refresh  chan struct{}
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
		homeDir:   home,
		codexHome: codexHome,
		startCodex: func(ctx context.Context) *exec.Cmd {
			return exec.CommandContext(ctx, codexCommand, "app-server")
		},
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
//
// Concurrent calls share one refresh. The refresh runs on a detached context
// with its own timeout, so one canceled request cannot poison the cache for
// other callers.
func (f *Fetcher) Snapshot(ctx context.Context) Snapshot {
	f.mu.Lock()
	if f.cacheFresh() {
		snapshot := cloneSnapshot(f.cache)
		f.mu.Unlock()
		return snapshot
	}
	refresh := f.refresh
	if refresh == nil {
		refresh = make(chan struct{})
		f.refresh = refresh
		go f.runRefresh(context.WithoutCancel(ctx), refresh)
	}
	f.mu.Unlock()

	select {
	case <-refresh:
	case <-ctx.Done():
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneSnapshot(f.cache)
}

// cacheFresh reports whether the cache can serve the current request. The
// caller must hold f.mu.
func (f *Fetcher) cacheFresh() bool {
	if f.cachedAt.IsZero() {
		return false
	}
	ttl := f.cacheTTL
	if !f.cache.HasData() {
		ttl = min(ttl, errorCacheTTL)
	}
	return f.now().Sub(f.cachedAt) < ttl
}

// runRefresh fetches every provider once and replaces the cache. It closes
// done so that all callers that wait on this refresh continue.
func (f *Fetcher) runRefresh(ctx context.Context, done chan struct{}) {
	f.mu.Lock()
	now := f.now()
	previous := make(map[string]Provider, len(f.cache.Providers))
	for _, provider := range f.cache.Providers {
		previous[provider.ID] = provider
	}
	blocked := make(map[string]time.Time, len(f.retryAt))
	for id, at := range f.retryAt {
		blocked[id] = at
	}
	f.mu.Unlock()

	type result struct {
		windows []Window
		err     error
	}
	results := make([]result, len(providerSpecs))
	var group sync.WaitGroup
	for i, spec := range providerSpecs {
		if until := blocked[spec.id]; now.Before(until) {
			results[i] = result{err: retryBlocked(until, now)}
			continue
		}
		group.Add(1)
		go func() {
			defer group.Done()
			fetchCtx, cancel := context.WithTimeout(ctx, f.requestTimeout)
			defer cancel()
			windows, err := spec.fetch(f, fetchCtx)
			results[i] = result{windows: windows, err: err}
		}()
	}
	group.Wait()

	f.mu.Lock()
	defer f.mu.Unlock()
	providers := make([]Provider, len(providerSpecs))
	for i, spec := range providerSpecs {
		provider := Provider{ID: spec.id, Name: spec.name}
		res := results[i]
		if res.err == nil {
			fetchedAt := now
			provider.Windows = res.windows
			provider.FetchedAt = &fetchedAt
			delete(f.retryAt, spec.id)
			providers[i] = provider
			continue
		}
		var statusErr *httpStatusError
		if errors.As(res.err, &statusErr) && statusErr.RetryAfter > 0 {
			f.retryAt[spec.id] = now.Add(statusErr.RetryAfter)
		}
		if old, ok := previous[spec.id]; ok && len(old.Windows) > 0 {
			old.Stale = true
			old.Error = res.err.Error()
			providers[i] = old
			continue
		}
		provider.Windows = []Window{}
		provider.Error = res.err.Error()
		providers[i] = provider
	}
	f.cache = Snapshot{Providers: providers, UpdatedAt: now}
	f.cachedAt = now
	f.refresh = nil
	close(done)
}
