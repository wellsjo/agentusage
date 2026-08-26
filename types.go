// Package agentusage reads local Codex and Claude Code credentials and fetches
// their current subscription rate-limit windows.
//
// Credentials never leave this package. Callers receive only percentages,
// reset times, and sanitized provider errors.
package agentusage

import (
	"context"
	"slices"
	"time"
)

// Window is one provider quota window.
type Window struct {
	ID            string     `json:"id"`
	Label         string     `json:"label"`
	Scope         string     `json:"scope,omitempty"`
	UsedPercent   float64    `json:"used_percent"`
	WindowSeconds int64      `json:"window_seconds"`
	ResetsAt      *time.Time `json:"resets_at,omitempty"`
}

// Provider is one AI account and its available quota windows. FetchedAt is
// nil until one fetch for this provider succeeds.
type Provider struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Windows   []Window   `json:"windows"`
	FetchedAt *time.Time `json:"fetched_at,omitempty"`
	Stale     bool       `json:"stale,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// Snapshot is the normalized response shared by Go and browser consumers.
type Snapshot struct {
	Providers []Provider `json:"providers"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Source supplies usage snapshots.
type Source interface {
	Snapshot(context.Context) Snapshot
}

// HasData reports whether at least one provider has a quota window.
func (s Snapshot) HasData() bool {
	for _, provider := range s.Providers {
		if len(provider.Windows) > 0 {
			return true
		}
	}
	return false
}

// cloneSnapshot copies every slice and every shared pointer, so a caller can
// change the result without an effect on the cache. It keeps an empty (not
// nil) Windows slice empty, so the JSON stays "windows": [].
func cloneSnapshot(snapshot Snapshot) Snapshot {
	clone := snapshot
	clone.Providers = slices.Clone(snapshot.Providers)
	for i := range clone.Providers {
		provider := &clone.Providers[i]
		if provider.FetchedAt != nil {
			fetchedAt := *provider.FetchedAt
			provider.FetchedAt = &fetchedAt
		}
		provider.Windows = slices.Clone(provider.Windows)
		for j := range provider.Windows {
			if provider.Windows[j].ResetsAt != nil {
				resetsAt := *provider.Windows[j].ResetsAt
				provider.Windows[j].ResetsAt = &resetsAt
			}
		}
	}
	return clone
}
