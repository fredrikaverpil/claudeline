package render

import (
	"fmt"
	"strings"
	"time"
)

// ANSI color constants.
const (
	Green         = "\033[32m"
	Yellow        = "\033[33m"
	Red           = "\033[31m"
	Magenta       = "\033[35m"
	Cyan          = "\033[36m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	Orange        = "\033[38;5;208m"
	Dim           = "\033[2m"
	Reset         = "\033[0m"
)

const barWidth = 5

// Bar renders a progress bar with ANSI colors.
func Bar(pct int, colorFn func(int) string) string {
	pct = max(0, min(100, pct))
	filled := pct * barWidth / 100
	empty := barWidth - filled
	color := colorFn(pct)

	return fmt.Sprintf(
		"%s%s%s%s%s %d%%",
		color, strings.Repeat("█", filled),
		Dim, strings.Repeat("░", empty),
		Reset, pct,
	)
}

// ContextColorFunc returns a color function for context window usage zones:
//   - Smart (green):  0–40%  — model performs at full capability
//   - Dumb (yellow):  41–60% — quality starts to degrade
//   - Danger (orange): 61%–warnPct — significant quality loss
//   - Near compaction (red): ≥warnPct — approaching auto-compaction
func ContextColorFunc(warnPct int) func(int) string {
	return func(pct int) string {
		switch {
		case pct >= warnPct:
			return Red
		case pct > 60:
			return Orange
		case pct > 40:
			return Yellow
		default:
			return Green
		}
	}
}

// QuotaColor returns the ANSI color for a quota usage percentage.
func QuotaColor(pct int) string {
	switch {
	case pct >= 90:
		return Red
	case pct >= 75:
		return BrightMagenta
	default:
		return BrightBlue
	}
}

// Identity returns the "Plan | Model" segment.
func Identity(model, plan string) string {
	switch {
	case model != "" && plan != "":
		return Cyan + plan + Reset + Dim + " │ " + Reset + Cyan + model + Reset
	case model != "":
		return Cyan + model + Reset
	default:
		return ""
	}
}

// Output assembles all segments into a single-line status output.
func Output(identity, contextBar, usage5h, usage7d, usageExtra, statusIndicator, updateIndicator string) string {
	sep := Dim + " │ " + Reset

	out := identity + sep + contextBar
	if usage5h != "" {
		out += sep + usage5h
	}
	if usage7d != "" {
		out += sep + usage7d
	}
	if usageExtra != "" {
		out += sep + usageExtra
	}
	if statusIndicator != "" {
		out += sep + statusIndicator
	}
	if updateIndicator != "" {
		out += sep + updateIndicator
	}
	return out
}

// UpdateIndicator returns a green arrow when a newer version is available.
// Returns "" when tag is empty.
func UpdateIndicator(tag string) string {
	if tag == "" {
		return ""
	}
	return Green + "↑" + Reset
}

// ResetTime formats a reset timestamp, showing just the time if it's
// today, or the day and time if it's a different day.
func ResetTime(iso string, now time.Time) string {
	if iso == "" {
		return ""
	}
	target, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	local := target.Local()
	y1, m1, d1 := now.Local().Date()
	y2, m2, d2 := local.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return local.Format("15:04")
	}
	return local.Format("Mon 15:04")
}

// StatusIndicator returns a colored fire icon with severity bars for service disruptions.
// Returns "" for "none", unknown indicators, or empty input.
func StatusIndicator(indicator string) string {
	switch indicator {
	case "minor":
		return Orange + "🔥▂" + Reset
	case "major":
		return Orange + "🔥▄▂" + Reset
	case "critical":
		return Orange + "🔥▆▄▂" + Reset
	default:
		return ""
	}
}

// ExtraUsage returns the "$used/$limit" string for pay-as-you-go overage.
// Returns "" when used is zero. Colors red when 80%+ of limit is used.
func ExtraUsage(used, limit int) string {
	if used == 0 {
		return ""
	}
	s := fmt.Sprintf("$%d/$%d", used, limit)
	if limit > 0 && used*100/limit >= 80 {
		return Red + s + Reset
	}
	return s
}

// QuotaSubBar renders a per-model quota bar with a trailing label.
func QuotaSubBar(pct int, label, resetTime string) string {
	s := Bar(pct, QuotaColor) + " " + label
	if resetTime != "" {
		s += " (" + resetTime + ")"
	}
	return s
}
