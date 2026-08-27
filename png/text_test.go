package png

import (
	"math"
	"testing"
	"time"

	"github.com/wellsjo/agentusage"
)

func TestDurationText(t *testing.T) {
	cases := map[time.Duration]string{
		-time.Hour:                   "0m",
		0:                            "0m",
		30 * time.Second:             "1m",
		time.Minute + time.Second:    "2m",
		90 * time.Minute:             "1h 30m",
		2 * time.Hour:                "2h",
		25 * time.Hour:               "1d 1h",
		48 * time.Hour:               "2d",
		5*24*time.Hour + 2*time.Hour: "5d 2h",
		5*24*time.Hour + 2*time.Hour + 59*time.Minute: "5d 2h",
	}
	for d, want := range cases {
		if got := durationText(d); got != want {
			t.Errorf("durationText(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestFormatPercent(t *testing.T) {
	cases := map[float64]string{
		0:            "0%",
		12:           "12%",
		12.5:         "12.5%",
		12.04:        "12.0%",
		12.25:        "12.3%",
		33.333:       "33.3%",
		99.96:        "100.0%",
		100:          "100%",
		150:          "100%",
		-5:           "0%",
		math.NaN():   "0%",
		math.Inf(1):  "0%",
		math.Inf(-1): "0%",
	}
	for value, want := range cases {
		if got := formatPercent(value); got != want {
			t.Errorf("formatPercent(%v) = %q, want %q", value, got, want)
		}
	}
}

func TestElapsedPercent(t *testing.T) {
	inOneHour := testNow.Add(time.Hour)
	past := testNow.Add(-time.Minute)
	cases := []struct {
		name   string
		window agentusage.Window
		want   float64
	}{
		{"four fifths through", agentusage.Window{WindowSeconds: 5 * 60 * 60, ResetsAt: &inOneHour}, 80},
		{"no reset time", agentusage.Window{WindowSeconds: 5 * 60 * 60}, 0},
		{"no duration", agentusage.Window{ResetsAt: &inOneHour}, 0},
		{"reset in the past", agentusage.Window{WindowSeconds: 60, ResetsAt: &past}, 100},
	}
	for _, tc := range cases {
		if got := elapsedPercent(tc.window, testNow); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: elapsedPercent = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestProviderViews(t *testing.T) {
	reset := testNow.Add(100 * time.Minute)
	snapshot := agentusage.Snapshot{Providers: []agentusage.Provider{
		{ID: agentusage.ProviderIDClaude, Name: "Claude  Code", Windows: []agentusage.Window{
			{Label: "5h window", Scope: "Fable", UsedPercent: 12.5, WindowSeconds: 5 * 60 * 60, ResetsAt: &reset},
			{UsedPercent: 7},
		}, Stale: true, Error: "HTTP 429"},
		{ID: "gemini", Windows: []agentusage.Window{}},
		{},
	}}
	views := providerViews(snapshot, testNow)
	if len(views) != 3 {
		t.Fatalf("views = %d, want 3", len(views))
	}

	claude := views[0]
	if claude.title != "CLAUDE CODE" {
		t.Errorf("title = %q, want CLAUDE CODE", claude.title)
	}
	if len(claude.windows) != 2 {
		t.Fatalf("windows = %d, want 2", len(claude.windows))
	}
	if got := claude.windows[0].description; got != "5h window · Fable · resets in 1h 40m" {
		t.Errorf("description = %q", got)
	}
	if got := claude.windows[0].used; got != "12.5% used" {
		t.Errorf("used = %q", got)
	}
	if got := claude.windows[0].elapsed; math.Abs(got-200.0/3) > 1e-9 {
		t.Errorf("elapsed = %v, want 66.67", got)
	}
	if got := claude.windows[1].description; got != "Usage" {
		t.Errorf("description without label or reset = %q, want Usage", got)
	}
	if claude.status != "Stale data · HTTP 429" || !claude.isError {
		t.Errorf("status = (%q, %v), want a stale error", claude.status, claude.isError)
	}

	gemini := views[1]
	if gemini.title != "GEMINI" {
		t.Errorf("title without a name = %q, want the ID", gemini.title)
	}
	if gemini.status != "No usage windows available." || gemini.isError {
		t.Errorf("status = (%q, %v), want the empty-windows text", gemini.status, gemini.isError)
	}

	if blank := views[2]; blank.title != "PROVIDER" {
		t.Errorf("title without name or ID = %q, want PROVIDER", blank.title)
	}
}
