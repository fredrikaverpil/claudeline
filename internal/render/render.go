package render

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fredrikaverpil/claudeline/internal/policy"
	"github.com/fredrikaverpil/claudeline/internal/status"
	"github.com/fredrikaverpil/claudeline/internal/stdin"
	"github.com/fredrikaverpil/claudeline/internal/update"
	"github.com/fredrikaverpil/claudeline/internal/usage"
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

// Params holds all data needed to build the statusline.
type Params struct {
	Sub                string
	Model              string
	ContextUsedPct     *float64 // nil when unavailable
	CompactPctOverride string   // raw CLAUDE_AUTOCOMPACT_PCT_OVERRIDE value
	Exceeds200kTokens  bool
	Usage              *usage.Response
	StdinRateLimits    *struct {
		FiveHour *stdin.RateLimit `json:"five_hour"`
		SevenDay *stdin.RateLimit `json:"seven_day"`
	}
	SubscriptionType string // raw subscription type for peak hours check
	Status           *status.Response
	Update           *update.Response
	ShowCwd          bool
	Cwd              string // raw working directory path
	CwdMaxLen        int
	ShowBranch       bool
	Branch           string // current git branch name
	BranchMaxLen     int
	ShowCost         bool
	CostUSD          float64
}

// Build assembles the complete statusline string from all collected data.
func Build(p Params) string {
	// Identity.
	identity := Identity(p.Model, p.Sub)

	// Context bar.
	contextPct := 0
	if p.ContextUsedPct != nil {
		contextPct = int(math.Round(*p.ContextUsedPct))
	}
	compactPct := 85
	if v, err := strconv.Atoi(p.CompactPctOverride); err == nil && v > 0 && v <= 100 {
		compactPct = v
	}
	warnPct := compactPct - 5
	contextBar := Bar(contextPct, ContextColorFunc(warnPct))
	if contextPct >= warnPct {
		contextBar += " ⚠️"
	}
	if p.Exceeds200kTokens {
		contextBar += " 🥵"
	}

	// Usage bars.
	// Aggregate 5h/7d bars come from stdin rate_limits (instant, no network).
	// Per-model sub-bars and extra usage come from the usage API (when available).
	var usage5h, usage7d, usageExtra string
	now := time.Now()

	// 5-hour bar from stdin.
	if p.StdinRateLimits != nil && p.StdinRateLimits.FiveHour != nil &&
		p.StdinRateLimits.FiveHour.UsedPercentage != nil {
		pct5 := int(math.Round(*p.StdinRateLimits.FiveHour.UsedPercentage))
		usage5h = Bar(pct5, QuotaColor)
		if reset := ResetTimeUnix(p.StdinRateLimits.FiveHour.ResetsAt, now); reset != "" {
			usage5h += " (" + reset + ")"
		}
		if policy.IsPeakHours(now, p.SubscriptionType) {
			usage5h = "⚡️" + usage5h
		}
	}

	// 7-day bar from stdin.
	if p.StdinRateLimits != nil && p.StdinRateLimits.SevenDay != nil &&
		p.StdinRateLimits.SevenDay.UsedPercentage != nil {
		pct7 := int(math.Round(*p.StdinRateLimits.SevenDay.UsedPercentage))
		usage7d = Bar(pct7, QuotaColor)
		if reset := ResetTimeUnix(p.StdinRateLimits.SevenDay.ResetsAt, now); reset != "" {
			usage7d += " (" + reset + ")"
		}
	}

	// Usage API: fall back for aggregate bars when stdin doesn't provide them,
	// plus per-model sub-bars and extra usage.
	if p.Usage != nil {
		if usage5h == "" && p.Usage.FiveHour != nil {
			pct5 := int(math.Round(p.Usage.FiveHour.Utilization))
			usage5h = Bar(pct5, QuotaColor)
			if reset := ResetTime(p.Usage.FiveHour.ResetsAt, now); reset != "" {
				usage5h += " (" + reset + ")"
			}
			if policy.IsPeakHours(now, p.SubscriptionType) {
				usage5h = "⚡️" + usage5h
			}
		}
		if usage7d == "" && p.Usage.SevenDay != nil {
			pct7 := int(math.Round(p.Usage.SevenDay.Utilization))
			usage7d = Bar(pct7, QuotaColor)
			if reset := ResetTime(p.Usage.SevenDay.ResetsAt, now); reset != "" {
				usage7d += " (" + reset + ")"
			}
		}

		// Sub-bars for 7d usage limits.
		subSep := Dim + " · " + Reset
		if usage7d == "" {
			subSep = " " + Reset
		}
		for _, model := range []struct {
			q     *usage.QuotaLimit
			label string
		}{
			{p.Usage.SevenDaySonnet, "sonnet"},
			{p.Usage.SevenDayOpus, "opus"},
			{p.Usage.SevenDayCowork, "cowork"},
			{p.Usage.SevenDayOAuthApp, "oauth"},
		} {
			if model.q != nil {
				pct := int(math.Round(model.q.Utilization))
				usage7d += subSep + QuotaSubBar(
					pct, model.label, ResetTime(model.q.ResetsAt, now),
				)
			}
		}

		if e := p.Usage.ExtraUsage; e != nil && e.IsEnabled && e.MonthlyLimit != nil && e.UsedCredits != nil {
			usageExtra = ExtraUsage(int(*e.UsedCredits)/100, int(*e.MonthlyLimit)/100)
		}
	}

	// Service status.
	var statusStr string
	if p.Status != nil {
		statusStr = StatusIndicator(p.Status.Status.Indicator)
	}

	// Update indicator.
	var updateStr string
	if p.Update != nil {
		updateStr = UpdateIndicator(p.Update.TagName)
	}

	// Working directory and git branch.
	sep := Dim + " │ " + Reset
	identityFull := identity
	if p.ShowCwd {
		if name := cwdName(p.Cwd, p.CwdMaxLen); name != "" {
			identityFull += sep + Yellow + name + Reset
		}
	}
	if p.ShowBranch {
		if name := compactName(p.Branch, p.BranchMaxLen); name != "" {
			identityFull += sep + Magenta + name + Reset
		}
	}

	var costStr string
	if p.ShowCost && p.CostUSD > 0 {
		costStr = Cost(p.CostUSD)
	}

	out := Output(identityFull, contextBar, usage5h, usage7d, costStr, usageExtra, statusStr, updateStr)
	// Leading reset clears stale ANSI state from previous renders.
	// Non-breaking spaces prevent the terminal from collapsing whitespace.
	return Reset + strings.ReplaceAll(out, " ", "\u00A0")
}

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
func Output(identity, contextBar, usage5h, usage7d, cost, usageExtra, statusIndicator, updateIndicator string) string {
	sep := Dim + " │ " + Reset

	out := identity + sep + contextBar
	if usage5h != "" {
		out += sep + usage5h
	}
	if usage7d != "" {
		out += sep + usage7d
	}
	if cost != "" {
		out += sep + cost
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
// The arrow is an OSC 8 hyperlink to the GitHub release page.
// Returns "" when tag is empty.
func UpdateIndicator(tag string) string {
	if tag == "" {
		return ""
	}
	url := "https://github.com/fredrikaverpil/claudeline/releases/tag/" + tag
	// OSC 8 hyperlink: \033]8;;URL\a TEXT \033]8;;\a
	return "\033]8;;" + url + "\a" + Green + "↑" + Reset + "\033]8;;\a"
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

// ResetTimeUnix formats a Unix timestamp reset time, showing just the time if
// it's today, or the day and time if it's a different day. Returns "" when ts is nil.
func ResetTimeUnix(ts *float64, now time.Time) string {
	if ts == nil {
		return ""
	}
	local := time.Unix(int64(*ts), 0).Local()
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

// Cost formats a USD cost value for display (e.g. "$1.23").
func Cost(usd float64) string {
	return fmt.Sprintf("$%.2f", usd)
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

// cwdName extracts the last path segment from cwd as the folder name.
func cwdName(cwd string, maxLen int) string {
	// Normalize separators for cross-platform support.
	name := filepath.Base(strings.ReplaceAll(cwd, `\`, "/"))
	switch {
	case name == "." || name == "/" || name == `\`:
		return ""
	case len(name) == 2 && name[1] == ':':
		// Bare Windows drive letter (e.g. "C:") — root of a drive.
		return ""
	}
	return compactName(name, maxLen)
}

// compactName truncates a name to maxLen runes using a Unicode ellipsis.
func compactName(name string, maxLen int) string {
	runes := []rune(name)
	if len(runes) <= maxLen {
		return name
	}
	half := (maxLen - 1) / 2
	return string(runes[:half]) + "…" + string(runes[len(runes)-(maxLen-1-half):])
}
