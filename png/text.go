package png

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/wellsjo/agentusage"
)

// providerView is the text content of one provider section. It mirrors what
// the Web Component's _providerView puts in the DOM.
type providerView struct {
	id      string
	title   string
	windows []windowView
	status  string
	isError bool
}

type windowView struct {
	description string
	used        string
	usedPercent float64
	elapsed     float64
}

func providerViews(snapshot agentusage.Snapshot, now time.Time) []providerView {
	views := make([]providerView, 0, len(snapshot.Providers))
	for _, provider := range snapshot.Providers {
		view := providerView{id: provider.ID, title: providerTitle(provider)}
		for _, window := range provider.Windows {
			view.windows = append(view.windows, windowView{
				description: windowDescription(window, now),
				used:        formatPercent(window.UsedPercent) + " used",
				usedPercent: clampPercent(window.UsedPercent),
				elapsed:     elapsedPercent(window, now),
			})
		}
		if len(provider.Windows) == 0 || provider.Error != "" {
			status := provider.Error
			if status == "" {
				status = "No usage windows available."
			}
			if provider.Stale {
				status = "Stale data · " + status
			}
			view.status = collapseSpace(status)
			view.isError = provider.Error != ""
		}
		views = append(views, view)
	}
	return views
}

// providerTitle returns the heading text after CSS text-transform: uppercase.
func providerTitle(provider agentusage.Provider) string {
	title := provider.Name
	if title == "" {
		title = provider.ID
	}
	if title == "" {
		title = "Provider"
	}
	return strings.ToUpper(collapseSpace(title))
}

func windowDescription(window agentusage.Window, now time.Time) string {
	label := window.Label
	if label == "" {
		label = "Usage"
	}
	if window.Scope != "" {
		label += " · " + window.Scope
	}
	return collapseSpace(label + resetText(window.ResetsAt, now))
}

func resetText(resetsAt *time.Time, now time.Time) string {
	if resetsAt == nil {
		return ""
	}
	return " · resets in " + durationText(max(0, resetsAt.Sub(now)))
}

// durationText rounds up to whole minutes, like the Web Component.
func durationText(d time.Duration) string {
	totalMinutes := int64(math.Ceil(float64(max(0, d)) / float64(time.Minute)))
	days := totalMinutes / 1440
	hours := (totalMinutes % 1440) / 60
	minutes := totalMinutes % 60
	switch {
	case days > 0 && hours > 0:
		return strconv.FormatInt(days, 10) + "d " + strconv.FormatInt(hours, 10) + "h"
	case days > 0:
		return strconv.FormatInt(days, 10) + "d"
	case hours > 0 && minutes > 0:
		return strconv.FormatInt(hours, 10) + "h " + strconv.FormatInt(minutes, 10) + "m"
	case hours > 0:
		return strconv.FormatInt(hours, 10) + "h"
	default:
		return strconv.FormatInt(minutes, 10) + "m"
	}
}

// elapsedPercent returns how far the window has run, for the marker.
func elapsedPercent(window agentusage.Window, now time.Time) float64 {
	if window.ResetsAt == nil || window.WindowSeconds <= 0 {
		return 0
	}
	duration := float64(window.WindowSeconds)
	start := window.ResetsAt.Add(-time.Duration(window.WindowSeconds) * time.Second)
	return clampPercent(now.Sub(start).Seconds() / duration * 100)
}

// formatPercent prints a whole number without decimals and any other value
// with one decimal, like the Web Component.
func formatPercent(value float64) string {
	used := clampPercent(value)
	if used == math.Trunc(used) {
		return strconv.FormatInt(int64(used), 10) + "%"
	}
	return strconv.FormatFloat(math.Round(used*10)/10, 'f', 1, 64) + "%"
}

// clampPercent maps a non-finite value to 0 and clamps the rest to [0, 100].
func clampPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Max(0, math.Min(100, value))
}

// collapseSpace applies HTML white-space collapsing.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
