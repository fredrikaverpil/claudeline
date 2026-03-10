package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCacheFilePath(t *testing.T) {
	tests := []struct {
		name            string
		claudeConfigDir string
		want            string
	}{
		{
			name:            "no CLAUDE_CONFIG_DIR set",
			claudeConfigDir: "",
			want:            filepath.Join(tempDir(), "claudeline-usage.json"),
		},
		{
			name:            "custom config dir claude-personal",
			claudeConfigDir: "/Users/oa/.claude-personal",
			want:            filepath.Join(tempDir(), "claudeline-usage-81c94270.json"),
		},
		{
			name:            "custom config dir claude-work",
			claudeConfigDir: "/Users/oa/.claude-work",
			want:            filepath.Join(tempDir(), "claudeline-usage-1ef5702c.json"),
		},
		{
			name:            "windows config dir claude-personal",
			claudeConfigDir: `C:\Users\oa\.claude-personal`,
			want:            filepath.Join(tempDir(), "claudeline-usage-9b705f7c.json"),
		},
		{
			name:            "windows config dir claude-work",
			claudeConfigDir: `C:\Users\oa\.claude-work`,
			want:            filepath.Join(tempDir(), "claudeline-usage-34fd078b.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", tt.claudeConfigDir)
			got := cacheFilePath()
			if got != tt.want {
				t.Errorf("cacheFilePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDebugLogFilePath(t *testing.T) {
	tests := []struct {
		name            string
		claudeConfigDir string
		want            string
	}{
		{
			name:            "no CLAUDE_CONFIG_DIR set",
			claudeConfigDir: "",
			want:            filepath.Join(tempDir(), "claudeline-debug.log"),
		},
		{
			name:            "custom config dir claude-personal",
			claudeConfigDir: "/Users/oa/.claude-personal",
			want:            filepath.Join(tempDir(), "claudeline-debug-81c94270.log"),
		},
		{
			name:            "custom config dir claude-work",
			claudeConfigDir: "/Users/oa/.claude-work",
			want:            filepath.Join(tempDir(), "claudeline-debug-1ef5702c.log"),
		},
		{
			name:            "windows config dir claude-personal",
			claudeConfigDir: `C:\Users\oa\.claude-personal`,
			want:            filepath.Join(tempDir(), "claudeline-debug-9b705f7c.log"),
		},
		{
			name:            "windows config dir claude-work",
			claudeConfigDir: `C:\Users\oa\.claude-work`,
			want:            filepath.Join(tempDir(), "claudeline-debug-34fd078b.log"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", tt.claudeConfigDir)
			got := debugLogFilePath()
			if got != tt.want {
				t.Errorf("debugLogFilePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKeychainServiceName(t *testing.T) {
	tests := []struct {
		name            string
		claudeConfigDir string
		want            string
	}{
		{
			name:            "no CLAUDE_CONFIG_DIR set",
			claudeConfigDir: "",
			want:            "Claude Code-credentials",
		},
		{
			name:            "custom config dir claude-personal",
			claudeConfigDir: "/Users/oa/.claude-personal",
			want:            "Claude Code-credentials-81c94270",
		},
		{
			name:            "custom config dir claude-work",
			claudeConfigDir: "/Users/oa/.claude-work",
			want:            "Claude Code-credentials-1ef5702c",
		},
		{
			name:            "windows config dir claude-personal",
			claudeConfigDir: `C:\Users\oa\.claude-personal`,
			want:            "Claude Code-credentials-9b705f7c",
		},
		{
			name:            "windows config dir claude-work",
			claudeConfigDir: `C:\Users\oa\.claude-work`,
			want:            "Claude Code-credentials-34fd078b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", tt.claudeConfigDir)
			got := keychainServiceName()
			if got != tt.want {
				t.Errorf("keychainServiceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompactName(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short name unchanged",
			input:  "main",
			maxLen: 30,
			want:   "main",
		},
		{
			name:   "exactly at limit",
			input:  strings.Repeat("a", 30),
			maxLen: 30,
			want:   strings.Repeat("a", 30),
		},
		{
			name:   "truncated with ellipsis",
			input:  "backup/feat-support-claudeline-progress-tracker",
			maxLen: 30,
			want:   "backup/feat-su…rogress-tracker",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 30,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compactName(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("compactName(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
			if len([]rune(got)) > tt.maxLen {
				t.Errorf("compactName(%q, %d) rune length = %d, exceeds maxLen", tt.input, tt.maxLen, len([]rune(got)))
			}
		})
	}
}

func TestCwdName(t *testing.T) {
	tests := []struct {
		name   string
		cwd    string
		maxLen int
		want   string
	}{
		{
			name:   "simple path",
			cwd:    "/Users/fredrik/code/public/claudeline",
			maxLen: 30,
			want:   "claudeline",
		},
		{
			name:   "root path",
			cwd:    "/",
			maxLen: 30,
			want:   "",
		},
		{
			name:   "empty cwd",
			cwd:    "",
			maxLen: 30,
			want:   "",
		},
		{
			name:   "trailing slash",
			cwd:    "/Users/fredrik/code/claudeline/",
			maxLen: 30,
			want:   "claudeline",
		},
		{
			name:   "long name truncated",
			cwd:    "/home/user/my-very-long-project-name-that-exceeds-limit",
			maxLen: 20,
			want:   "my-very-l…eeds-limit",
		},
		{
			name:   "windows path",
			cwd:    `C:\Users\oa\code\claudeline`,
			maxLen: 30,
			want:   "claudeline",
		},
		{
			name:   "home directory",
			cwd:    "/Users/fredrik",
			maxLen: 30,
			want:   "fredrik",
		},
		{
			name:   "windows root C:\\",
			cwd:    `C:\`,
			maxLen: 30,
			want:   "",
		},
		{
			name:   "windows root C:/",
			cwd:    "C:/",
			maxLen: 30,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cwdName(tt.cwd, tt.maxLen)
			if got != tt.want {
				t.Errorf("cwdName(%q, %d) = %q, want %q", tt.cwd, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestContextColorFunc(t *testing.T) {
	colorFn := contextColorFunc(80)

	tests := []struct {
		name string
		pct  int
		want string
	}{
		{name: "smart zone 0%", pct: 0, want: green},
		{name: "smart zone 40%", pct: 40, want: green},
		{name: "dumb zone 41%", pct: 41, want: yellow},
		{name: "dumb zone 60%", pct: 60, want: yellow},
		{name: "danger zone 61%", pct: 61, want: orange},
		{name: "danger zone 79%", pct: 79, want: orange},
		{name: "near compaction 80%", pct: 80, want: red},
		{name: "near compaction 100%", pct: 100, want: red},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorFn(tt.pct)
			if got != tt.want {
				t.Errorf("contextColorFunc(80)(%d) = %q, want %q", tt.pct, got, tt.want)
			}
		})
	}
}

func TestReadCacheRateLimited(t *testing.T) {
	// Use a unique CLAUDE_CONFIG_DIR to isolate the cache file per test.
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	cachePath := cacheFilePath()
	t.Cleanup(func() { os.Remove(cachePath) })

	t.Run("rate limited with future RetryAfter returns sentinel error", func(t *testing.T) {
		entry := cacheEntry{
			Timestamp:   time.Now().Unix(),
			OK:          false,
			RateLimited: true,
			RetryAfter:  time.Now().Add(5 * time.Minute).Unix(),
		}
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cachePath, data, 0o600); err != nil {
			t.Fatal(err)
		}

		_, err = readCache()
		if !errors.Is(err, errCachedRateLimited) {
			t.Errorf("readCache() error = %v, want %v", err, errCachedRateLimited)
		}
	})

	t.Run("rate limited with past RetryAfter returns cache expired", func(t *testing.T) {
		entry := cacheEntry{
			Timestamp:   time.Now().Add(-time.Minute).Unix(),
			OK:          false,
			RateLimited: true,
			RetryAfter:  time.Now().Add(-time.Second).Unix(),
		}
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cachePath, data, 0o600); err != nil {
			t.Fatal(err)
		}

		_, err = readCache()
		if err == nil || errors.Is(err, errCachedRateLimited) {
			t.Errorf("readCache() error = %v, want cache expired", err)
		}
	})

	t.Run("rate limited without RetryAfter uses default TTL fallback", func(t *testing.T) {
		// Simulates cache written by an older version without RetryAfter.
		entry := cacheEntry{
			Timestamp:   time.Now().Unix(),
			OK:          false,
			RateLimited: true,
		}
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cachePath, data, 0o600); err != nil {
			t.Fatal(err)
		}

		_, err = readCache()
		if !errors.Is(err, errCachedRateLimited) {
			t.Errorf("readCache() error = %v, want %v", err, errCachedRateLimited)
		}
	})

	t.Run("rate limited without RetryAfter expired returns cache expired", func(t *testing.T) {
		entry := cacheEntry{
			Timestamp:   time.Now().Add(-cacheTTLRateLimitDefault - time.Second).Unix(),
			OK:          false,
			RateLimited: true,
		}
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cachePath, data, 0o600); err != nil {
			t.Fatal(err)
		}

		_, err = readCache()
		if err == nil || errors.Is(err, errCachedRateLimited) {
			t.Errorf("readCache() error = %v, want cache expired", err)
		}
	})
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{
			name:  "empty returns default",
			value: "",
			want:  cacheTTLRateLimitDefault,
		},
		{
			name:  "integer seconds",
			value: "120",
			want:  120 * time.Second,
		},
		{
			name:  "clamped to max backoff",
			value: "7200",
			want:  cacheTTLRateLimitMaxBackoff,
		},
		{
			name:  "zero returns default",
			value: "0",
			want:  cacheTTLRateLimitDefault,
		},
		{
			name:  "negative returns default",
			value: "-10",
			want:  cacheTTLRateLimitDefault,
		},
		{
			name:  "unparseable returns default",
			value: "not-a-number",
			want:  cacheTTLRateLimitDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRetryAfter(tt.value)
			if got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestUsageResponseUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  usageResponse
	}{
		{
			name: "full response with all fields",
			input: `{
				"five_hour": {"utilization": 8.0, "resets_at": "2026-03-09T11:00:00+00:00"},
				"seven_day": {"utilization": 31.0, "resets_at": "2026-03-15T08:00:00+00:00"},
				"seven_day_sonnet": {"utilization": 12, "resets_at": "2026-03-09T13:00:00+00:00"},
				"seven_day_opus": {"utilization": 45, "resets_at": "2026-03-09T14:00:00+00:00"},
				"seven_day_oauth_apps": null,
				"seven_day_cowork": {"utilization": 5, "resets_at": "2026-03-10T08:00:00+00:00"},
				"iguana_necktie": null,
				"extra_usage": {"is_enabled": true, "monthly_limit": 5000, "used_credits": 1234, "utilization": null}
			}`,
			want: usageResponse{
				FiveHour:       quotaLimit{Utilization: 8.0, ResetsAt: "2026-03-09T11:00:00+00:00"},
				SevenDay:       quotaLimit{Utilization: 31.0, ResetsAt: "2026-03-15T08:00:00+00:00"},
				SevenDaySonnet: &quotaLimit{Utilization: 12, ResetsAt: "2026-03-09T13:00:00+00:00"},
				SevenDayOpus:   &quotaLimit{Utilization: 45, ResetsAt: "2026-03-09T14:00:00+00:00"},
				SevenDayCowork: &quotaLimit{Utilization: 5, ResetsAt: "2026-03-10T08:00:00+00:00"},
				ExtraUsage: &extraUsage{
					IsEnabled:    true,
					MonthlyLimit: new(float64(5000)),
					UsedCredits:  new(float64(1234)),
				},
			},
		},
		{
			name: "minimal response with nulls",
			input: `{
				"five_hour": {"utilization": 0, "resets_at": null},
				"seven_day": {"utilization": 14, "resets_at": "2026-03-13T08:00:00+00:00"},
				"seven_day_sonnet": null,
				"seven_day_opus": null,
				"seven_day_oauth_apps": null,
				"seven_day_cowork": null,
				"iguana_necktie": null,
				"extra_usage": {"is_enabled": false, "monthly_limit": null, "used_credits": null, "utilization": null}
			}`,
			want: usageResponse{
				FiveHour: quotaLimit{Utilization: 0},
				SevenDay: quotaLimit{Utilization: 14, ResetsAt: "2026-03-13T08:00:00+00:00"},
				ExtraUsage: &extraUsage{
					IsEnabled: false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got usageResponse
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if got.FiveHour != tt.want.FiveHour {
				t.Errorf("FiveHour = %+v, want %+v", got.FiveHour, tt.want.FiveHour)
			}
			if got.SevenDay != tt.want.SevenDay {
				t.Errorf("SevenDay = %+v, want %+v", got.SevenDay, tt.want.SevenDay)
			}
			assertQuotaLimitPtr(t, "SevenDaySonnet", got.SevenDaySonnet, tt.want.SevenDaySonnet)
			assertQuotaLimitPtr(t, "SevenDayOpus", got.SevenDayOpus, tt.want.SevenDayOpus)
			assertQuotaLimitPtr(t, "SevenDayCowork", got.SevenDayCowork, tt.want.SevenDayCowork)
			assertExtraUsage(t, got.ExtraUsage, tt.want.ExtraUsage)
		})
	}
}

func assertQuotaLimitPtr(t *testing.T, name string, got, want *quotaLimit) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if (got == nil) != (want == nil) {
		t.Errorf("%s: got %v, want %v", name, got, want)
		return
	}
	if *got != *want {
		t.Errorf("%s = %+v, want %+v", name, *got, *want)
	}
}

func assertExtraUsage(t *testing.T, got, want *extraUsage) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if (got == nil) != (want == nil) {
		t.Errorf("ExtraUsage: got %v, want %v", got, want)
		return
	}
	if got.IsEnabled != want.IsEnabled {
		t.Errorf("ExtraUsage.IsEnabled = %v, want %v", got.IsEnabled, want.IsEnabled)
	}
	assertFloat64Ptr(t, "ExtraUsage.MonthlyLimit", got.MonthlyLimit, want.MonthlyLimit)
	assertFloat64Ptr(t, "ExtraUsage.UsedCredits", got.UsedCredits, want.UsedCredits)
}

func assertFloat64Ptr(t *testing.T, name string, got, want *float64) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Errorf("%s: got %v, want %v", name, got, want)
		return
	}
	if got != nil && *got != *want {
		t.Errorf("%s = %v, want %v", name, *got, *want)
	}
}

func TestFormatExtraUsage(t *testing.T) {
	tests := []struct {
		name  string
		extra *extraUsage
		want  string
	}{
		{
			name:  "nil extra usage",
			extra: nil,
			want:  "",
		},
		{
			name:  "disabled",
			extra: &extraUsage{IsEnabled: false},
			want:  "",
		},
		{
			name:  "enabled with zero usage - hidden",
			extra: &extraUsage{IsEnabled: true, MonthlyLimit: new(float64(5000)), UsedCredits: new(float64(0))},
			want:  "",
		},
		{
			name:  "enabled with usage below threshold",
			extra: &extraUsage{IsEnabled: true, MonthlyLimit: new(float64(5000)), UsedCredits: new(float64(1234))},
			want:  "$12/$50",
		},
		{
			name:  "enabled at 80% - red",
			extra: &extraUsage{IsEnabled: true, MonthlyLimit: new(float64(5000)), UsedCredits: new(float64(4000))},
			want:  red + "$40/$50" + ansiReset,
		},
		{
			name:  "enabled with nil fields",
			extra: &extraUsage{IsEnabled: true, MonthlyLimit: nil, UsedCredits: nil},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExtraUsage(tt.extra)
			if got != tt.want {
				t.Errorf("formatExtraUsage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatQuotaSubBar(t *testing.T) {
	now := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		q       *quotaLimit
		label   string
		wantPct string // expected percentage string in output.
	}{
		{
			name:  "nil quota",
			q:     nil,
			label: "sonnet",
		},
		{
			name:    "sonnet at 12%",
			q:       &quotaLimit{Utilization: 12, ResetsAt: "2026-03-09T13:00:00+00:00"},
			label:   "sonnet",
			wantPct: "12%",
		},
		{
			name:    "opus at 45%",
			q:       &quotaLimit{Utilization: 45, ResetsAt: "2026-03-09T14:00:00+00:00"},
			label:   "opus",
			wantPct: "45%",
		},
		{
			name:    "cowork at 5%",
			q:       &quotaLimit{Utilization: 5, ResetsAt: "2026-03-10T08:00:00+00:00"},
			label:   "cowork",
			wantPct: "5%",
		},
		{
			name:    "oauth at 0%",
			q:       &quotaLimit{Utilization: 0, ResetsAt: "2026-03-10T08:00:00+00:00"},
			label:   "oauth",
			wantPct: "0%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatQuotaSubBar(tt.q, tt.label, now)
			if tt.q == nil {
				if got != "" {
					t.Errorf("formatQuotaSubBar(nil) = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.label) {
				t.Errorf("formatQuotaSubBar() = %q, missing label %q", got, tt.label)
			}
			if !strings.Contains(got, tt.wantPct) {
				t.Errorf("formatQuotaSubBar() = %q, missing percentage %q", got, tt.wantPct)
			}
		})
	}
}

func TestRenderOutput(t *testing.T) {
	subSep := dim + " · " + ansiReset
	sep := dim + " │ " + ansiReset
	ident := cyan + "[Opus 4.6 | Pro]" + ansiReset
	identCwd := ident + sep + yellow + "myproject" + ansiReset
	identBranch := ident + sep + magenta + "feat/foo" + ansiReset
	identCwdBranch := ident + sep + yellow + "myproject" + ansiReset + sep + magenta + "feat/foo" + ansiReset

	tests := []struct {
		name       string
		identity   string
		contextBar string
		usage5h    string
		usage7d    string
		usageExtra string
		want       string
	}{
		// Minimal: identity + context only.
		{
			name:       "identity and context only",
			identity:   ident,
			contextBar: "█░░░░ 23%",
			want:       ident + sep + "█░░░░ 23%",
		},
		// Identity variants with cwd/branch.
		{
			name:       "with cwd",
			identity:   identCwd,
			contextBar: "█░░░░ 23%",
			want:       identCwd + sep + "█░░░░ 23%",
		},
		{
			name:       "with git branch",
			identity:   identBranch,
			contextBar: "█░░░░ 23%",
			want:       identBranch + sep + "█░░░░ 23%",
		},
		{
			name:       "with cwd and git branch",
			identity:   identCwdBranch,
			contextBar: "█░░░░ 23%",
			want:       identCwdBranch + sep + "█░░░░ 23%",
		},
		// Usage bar combinations.
		{
			name:       "5h only",
			identity:   ident,
			contextBar: "█░░░░ 23%",
			usage5h:    "░░░░░ 9% (13:00)",
			want:       ident + sep + "█░░░░ 23%" + sep + "░░░░░ 9% (13:00)",
		},
		{
			name:       "5h and 7d",
			identity:   ident,
			contextBar: "█░░░░ 23%",
			usage5h:    "░░░░░ 9% (13:00)",
			usage7d:    "█░░░░ 31% (Sun 09:00)",
			want: ident + sep + "█░░░░ 23%" + sep + "░░░░░ 9% (13:00)" + sep +
				"█░░░░ 31% (Sun 09:00)",
		},
		{
			name:       "7d with sub-bars",
			identity:   ident,
			contextBar: "█░░░░ 23%",
			usage5h:    "░░░░░ 9% (13:00)",
			usage7d:    "█░░░░ 31% (Sun 09:00)" + subSep + "░░░░░ 12% son (14:00)",
			want: ident + sep + "█░░░░ 23%" + sep + "░░░░░ 9% (13:00)" + sep +
				"█░░░░ 31% (Sun 09:00)" + subSep + "░░░░░ 12% son (14:00)",
		},
		// Extra usage variants.
		{
			name:       "with extra usage",
			identity:   ident,
			contextBar: "█░░░░ 23%",
			usage5h:    "░░░░░ 9% (13:00)",
			usage7d:    "█░░░░ 31% (Sun 09:00)",
			usageExtra: "$40/$50",
			want: ident + sep + "█░░░░ 23%" + sep + "░░░░░ 9% (13:00)" + sep +
				"█░░░░ 31% (Sun 09:00)" + sep + "$40/$50",
		},
		{
			name:       "with sub-bars and extra usage",
			identity:   ident,
			contextBar: "█░░░░ 23%",
			usage5h:    "░░░░░ 9% (13:00)",
			usage7d:    "█░░░░ 31% (Sun 09:00)" + subSep + "░░░░░ 12% son (14:00)",
			usageExtra: red + "$45/$50" + ansiReset,
			want: ident + sep + "█░░░░ 23%" + sep + "░░░░░ 9% (13:00)" + sep +
				"█░░░░ 31% (Sun 09:00)" + subSep + "░░░░░ 12% son (14:00)" + sep +
				red + "$45/$50" + ansiReset,
		},
		// Full combination: cwd + branch + all bars + extra.
		{
			name:       "all segments",
			identity:   identCwdBranch,
			contextBar: "██░░░ 42%",
			usage5h:    "███░░ 62% (15:00)",
			usage7d:    "█░░░░ 27% (Fri 09:00)" + subSep + "░░░░░ 1% son (Tue 08:00)",
			usageExtra: "$12/$50",
			want: identCwdBranch + sep + "██░░░ 42%" + sep + "███░░ 62% (15:00)" + sep +
				"█░░░░ 27% (Fri 09:00)" + subSep + "░░░░░ 1% son (Tue 08:00)" + sep + "$12/$50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderOutput(tt.identity, tt.contextBar, tt.usage5h, tt.usage7d, tt.usageExtra)
			if got != tt.want {
				t.Errorf("renderOutput() =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

func TestFormatResetTime(t *testing.T) {
	// Use a fixed "now" for deterministic tests.
	now := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)

	// Test today: should NOT contain day name.
	todayResult := formatResetTime("2026-03-09T13:00:00+00:00", now)
	if todayResult == "" {
		t.Fatal("formatResetTime returned empty for valid today timestamp")
	}
	if strings.Contains(todayResult, "Mon") || strings.Contains(todayResult, "Sun") ||
		strings.Contains(todayResult, "Tue") {
		t.Errorf("today reset should not contain day name, got %q", todayResult)
	}

	// Test different day: should contain day name.
	futureResult := formatResetTime("2026-03-15T08:00:00+00:00", now)
	if futureResult == "" {
		t.Fatal("formatResetTime returned empty for valid future timestamp")
	}
	if !strings.Contains(futureResult, "Sun") {
		t.Errorf("future reset should contain day name 'Sun', got %q", futureResult)
	}

	// Test empty.
	emptyResult := formatResetTime("", now)
	if emptyResult != "" {
		t.Errorf("formatResetTime('') = %q, want empty", emptyResult)
	}
}

func TestGetBranch(t *testing.T) {
	tmp := t.TempDir()

	// Save and restore working directory.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	// Initialize a real git repo so .git/HEAD is created by git itself.
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = tmp
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	t.Run("default branch", func(t *testing.T) {
		got := getBranch()
		if got != "main" {
			t.Errorf("getBranch() = %q, want %q", got, "main")
		}
	})

	t.Run("branch with slashes", func(t *testing.T) {
		run("switch", "-c", "feat/my-feature")
		got := getBranch()
		if got != "feat/my-feature" {
			t.Errorf("getBranch() = %q, want %q", got, "feat/my-feature")
		}
	})

	t.Run("detached HEAD", func(t *testing.T) {
		// Need a commit to detach from.
		run("commit", "--allow-empty", "-m", "init")
		run("switch", "--detach")
		got := getBranch()
		if got != "" {
			t.Errorf("getBranch() = %q, want empty string", got)
		}
	})
}
