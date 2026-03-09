package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	runtimedebug "runtime/debug"
	"strconv"
	"strings"
	"time"
)

// version and commit are set via ldflags by goreleaser.
// When empty, the version falls back to runtime/debug.ReadBuildInfo.
var (
	version string
	commit  string
)

// ANSI color constants.
const (
	green         = "\033[32m"
	yellow        = "\033[33m"
	red           = "\033[31m"
	magenta       = "\033[35m"
	cyan          = "\033[36m"
	brightBlue    = "\033[94m"
	brightMagenta = "\033[95m"
	orange        = "\033[38;5;208m"
	dim           = "\033[2m"
	ansiReset     = "\033[0m"
)

const (
	cacheTTLOK                  = 60 * time.Second
	cacheTTLFail                = 15 * time.Second
	cacheTTLRateLimitDefault    = 5 * time.Minute
	cacheTTLRateLimitMaxBackoff = 30 * time.Minute
	usageURL                    = "https://api.anthropic.com/api/oauth/usage"
	ioTimeout                   = 5 * time.Second
	barWidth                    = 5
)

var (
	debugLogFile         = debugLogFilePath()
	errRateLimited       = errors.New("rate limited")
	errCachedRateLimited = errors.New("cached rate limit")
	errCachedFailure     = errors.New("cached failure")
)

// stdinData is the JSON structure received from Claude Code via stdin.
type stdinData struct {
	Cwd   string `json:"cwd"`
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		UsedPercentage *float64 `json:"used_percentage"`
	} `json:"context_window"`
}

// credentials is the OAuth credentials structure.
type credentials struct {
	ClaudeAiOauth struct {
		AccessToken      string `json:"accessToken"`
		SubscriptionType string `json:"subscriptionType"`
	} `json:"claudeAiOauth"`
}

// usageResponse is the API response from the usage endpoint.
type usageResponse struct {
	FiveHour struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"five_hour"`
	SevenDay struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"seven_day"`
}

// cacheEntry is the file-based cache structure.
type cacheEntry struct {
	Data        json.RawMessage `json:"data"`
	Timestamp   int64           `json:"timestamp"`
	OK          bool            `json:"ok"`
	RateLimited bool            `json:"rate_limited,omitempty"`
	RetryAfter  int64           `json:"retry_after,omitempty"` // Unix timestamp; retry allowed after this time.
}

func main() {
	os.Exit(runMain())
}

// buildVersion returns the version string. It prefers the ldflags-injected
// version (set by goreleaser), falling back to runtime/debug.ReadBuildInfo
// (set by go install/run and local builds).
func buildVersion() string {
	v := version
	if v == "" {
		if info, ok := runtimedebug.ReadBuildInfo(); ok {
			v = info.Main.Version
		}
	}
	if v == "" {
		v = "(unknown)"
	}
	if commit != "" {
		v += " (" + commit + ")"
	}
	return v
}

// config holds CLI configuration.
type config struct {
	showGitBranch   bool
	gitBranchMaxLen int
	showCwd         bool
	cwdMaxLen       int
}

func runMain() int {
	showVersion := flag.Bool("version", false, "print version and exit")
	debug := flag.Bool("debug", false, "write warnings and errors to "+debugLogFile)
	showGitBranch := flag.Bool("git-branch", false, "show git branch in the status line")
	gitBranchMaxLen := flag.Int("git-branch-max-len", 30, "max display length for git branch")
	showCwd := flag.Bool("cwd", false, "show working directory name in the status line")
	cwdMaxLen := flag.Int("cwd-max-len", 30, "max display length for working directory name")
	flag.Parse()

	if *showVersion {
		if _, err := fmt.Fprintln(os.Stdout, buildVersion()); err != nil {
			return 1
		}
		return 0
	}

	log.SetPrefix("claudeline: ")
	log.SetFlags(log.Ldate | log.Ltime)
	if *debug {
		f, err := os.OpenFile(debugLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err == nil {
			log.SetOutput(f)
			defer func() { _ = f.Close() }()
		}
	} else {
		log.SetOutput(io.Discard)
	}

	cfg := config{
		showGitBranch:   *showGitBranch,
		gitBranchMaxLen: *gitBranchMaxLen,
		showCwd:         *showCwd,
		cwdMaxLen:       *cwdMaxLen,
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "claudeline: %v\n", err)
		return 1
	}
	return 0
}

func run(cfg config) error {
	ctx := context.Background()

	// Read stdin JSON.
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	var data stdinData
	if err := json.Unmarshal(input, &data); err != nil {
		return fmt.Errorf("parse stdin JSON: %w", err)
	}

	// Read credentials.
	creds, err := readCredentials(ctx)
	if err != nil {
		log.Printf("credentials: %v", err)
		creds = credentials{}
	}

	// Determine plan name.
	plan := planName(creds.ClaudeAiOauth.SubscriptionType)

	// Build identity segment.
	identity := buildIdentity(data.Model.DisplayName, plan)

	// Context bar.
	contextPct := 0
	if data.ContextWindow.UsedPercentage != nil {
		contextPct = int(math.Round(*data.ContextWindow.UsedPercentage))
	}
	// Warn when context is near auto-compaction threshold.
	compactPct := 85
	if v, err := strconv.Atoi(os.Getenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE")); err == nil && v > 0 && v <= 100 {
		compactPct = v
	}
	warnPct := compactPct - 5
	contextBar := bar(contextPct, contextColorFunc(warnPct))
	if contextPct >= warnPct {
		contextBar += " " + yellow + "⚠" + ansiReset
	}

	// Usage bars.
	var usage5h, usage7d string
	token := creds.ClaudeAiOauth.AccessToken
	if token == "" {
		log.Printf("usage: no access token found")
	} else if plan == "" {
		log.Printf("usage: unknown subscription type %q, expected pro/max/team", creds.ClaudeAiOauth.SubscriptionType)
	}
	if token != "" && plan != "" {
		usage, fetchErr := fetchUsage(ctx, token)
		if fetchErr != nil && !errors.Is(fetchErr, errCachedRateLimited) && !errors.Is(fetchErr, errCachedFailure) {
			log.Printf("usage: %v", fetchErr)
		}
		if fetchErr == nil && usage != nil {
			pct5 := int(math.Round(usage.FiveHour.Utilization))
			usage5h = bar(pct5, quotaColor)
			if reset := formatLocalTime(usage.FiveHour.ResetsAt, "15:04"); reset != "" {
				usage5h += " (" + reset + ")"
			}

			pct7 := int(math.Round(usage.SevenDay.Utilization))
			usage7d = bar(pct7, quotaColor)
			if reset := formatLocalTime(usage.SevenDay.ResetsAt, "Mon 15:04"); reset != "" {
				usage7d += " (" + reset + ")"
			}
		}
	}

	// Render output.
	sep := dim + " │ " + ansiReset
	output := identity
	if cfg.showCwd {
		if name := cwdName(data.Cwd, cfg.cwdMaxLen); name != "" {
			output += sep + yellow + name + ansiReset
		}
	}
	if cfg.showGitBranch {
		if branch := compactName(getBranch(), cfg.gitBranchMaxLen); branch != "" {
			output += sep + magenta + branch + ansiReset
		}
	}
	output += sep + contextBar
	if usage5h != "" {
		output += sep + usage5h
	}
	if usage7d != "" {
		output += sep + usage7d
	}

	// Leading reset clears stale ANSI state from previous renders.
	// Non-breaking spaces prevent the terminal from collapsing whitespace.
	output = ansiReset + strings.ReplaceAll(output, " ", "\u00A0")
	_, err = fmt.Fprintln(os.Stdout, output)
	return err
}

// buildIdentity returns the "[Model | Plan]" segment.
func buildIdentity(model, plan string) string {
	switch {
	case model != "" && plan != "":
		return cyan + "[" + model + " | " + plan + "]" + ansiReset
	case model != "":
		return cyan + "[" + model + "]" + ansiReset
	default:
		return ""
	}
}

// planName maps a subscription type to a display name.
func planName(subType string) string {
	lower := strings.ToLower(subType)
	switch {
	case strings.Contains(lower, "max"):
		return "Max"
	case strings.Contains(lower, "pro"):
		return "Pro"
	case strings.Contains(lower, "team"):
		return "Team"
	default:
		return ""
	}
}

// contextColorFunc returns a color function for context window usage zones:
//   - Smart (green):  0–40%  — model performs at full capability
//   - Dumb (yellow):  41–60% — quality starts to degrade
//   - Danger (orange): 61%–warnPct — significant quality loss
//   - Near compaction (red): ≥warnPct — approaching auto-compaction
func contextColorFunc(warnPct int) func(int) string {
	return func(pct int) string {
		switch {
		case pct >= warnPct:
			return red
		case pct > 60:
			return orange
		case pct > 40:
			return yellow
		default:
			return green
		}
	}
}

// quotaColor returns the ANSI color for a quota usage percentage.
func quotaColor(pct int) string {
	switch {
	case pct >= 90:
		return red
	case pct >= 75:
		return brightMagenta
	default:
		return brightBlue
	}
}

// bar renders a progress bar with ANSI colors.
func bar(pct int, colorFn func(int) string) string {
	pct = max(0, min(100, pct))
	filled := pct * barWidth / 100
	empty := barWidth - filled
	color := colorFn(pct)

	return fmt.Sprintf(
		"%s%s%s%s%s %d%%",
		color, strings.Repeat("█", filled),
		dim, strings.Repeat("░", empty),
		ansiReset, pct,
	)
}

// formatLocalTime parses an ISO 8601 timestamp and formats it in the local timezone.
func formatLocalTime(iso, layout string) string {
	if iso == "" {
		return ""
	}
	target, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	return target.Local().Format(layout)
}

// keychainServiceName returns the macOS Keychain service name used by Claude Code.
// When CLAUDE_CONFIG_DIR is set, Claude Code appends a hash suffix to the service name.
func keychainServiceName() string {
	const base = "Claude Code-credentials"
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		return base
	}
	h := sha256.Sum256([]byte(configDir))
	return fmt.Sprintf("%s-%x", base, h[:4])
}

// tempDir returns /tmp on Unix systems and the OS temp directory on Windows.
func tempDir() string {
	if runtime.GOOS == "windows" {
		return os.TempDir()
	}
	return "/tmp"
}

// debugLogFilePath returns the file path for the debug log.
// When CLAUDE_CONFIG_DIR is set, a hash suffix is appended to avoid collisions between profiles.
func debugLogFilePath() string {
	base := filepath.Join(tempDir(), "claudeline-debug")
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		return base + ".log"
	}
	h := sha256.Sum256([]byte(configDir))
	return fmt.Sprintf("%s-%x.log", base, h[:4])
}

// cacheFilePath returns the file path for the usage cache.
// When CLAUDE_CONFIG_DIR is set, a hash suffix is appended to avoid collisions between profiles.
func cacheFilePath() string {
	base := filepath.Join(tempDir(), "claudeline-usage")
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		return base + ".json"
	}
	h := sha256.Sum256([]byte(configDir))
	return fmt.Sprintf("%s-%x.json", base, h[:4])
}

// readCredentials reads OAuth credentials from keychain or file.
func readCredentials(ctx context.Context) (credentials, error) {
	// Try macOS keychain first.
	if runtime.GOOS == "darwin" {
		serviceName := keychainServiceName()
		ctx, cancel := context.WithTimeout(ctx, ioTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx,
			"/usr/bin/security", "find-generic-password",
			"-s", serviceName, "-w",
		).Output()
		if err == nil {
			var creds credentials
			if err := json.Unmarshal(out, &creds); err != nil {
				return credentials{}, fmt.Errorf("parse keychain credentials: %w", err)
			}
			return creds, nil
		}
	}

	// File fallback.
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return credentials{}, fmt.Errorf("get home dir: %w", err)
		}
		configDir = filepath.Join(home, ".claude")
	}
	data, err := os.ReadFile(filepath.Join(configDir, ".credentials.json"))
	if err != nil {
		return credentials{}, fmt.Errorf("read credentials file: %w", err)
	}
	var creds credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return credentials{}, fmt.Errorf("parse credentials file: %w", err)
	}
	return creds, nil
}

// getBranch returns the current git branch name, or "" if not in a git repo.
func getBranch() string {
	data, err := os.ReadFile(".git/HEAD")
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	if after, ok := strings.CutPrefix(s, "ref: refs/heads/"); ok {
		return after
	}
	return "" // detached HEAD or bare repo
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

// fetchUsage fetches usage data from the API with file-based caching.
func fetchUsage(ctx context.Context, token string) (*usageResponse, error) {
	// Check cache.
	cached, err := readCache()
	if err == nil {
		return cached, nil
	}
	// Respect cached rate limit and failure TTLs — don't re-fetch
	// during the cooldown window, as that would reset the TTL on
	// each failed attempt and prevent recovery.
	if errors.Is(err, errCachedRateLimited) || errors.Is(err, errCachedFailure) {
		return nil, err
	}

	// Fetch from API.
	usage, retryAfter, err := fetchUsageAPI(ctx, token)
	if err != nil {
		writeCache(nil, false, retryAfter)
		return nil, fmt.Errorf("fetch usage API: %w", err)
	}

	writeCache(usage, true, 0)
	return usage, nil
}

// readCache reads and validates the cached usage data.
func readCache() (*usageResponse, error) {
	data, err := os.ReadFile(cacheFilePath())
	if err != nil {
		return nil, err
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}

	age := time.Since(time.Unix(entry.Timestamp, 0))
	if entry.OK && age < cacheTTLOK {
		var usage usageResponse
		if err := json.Unmarshal(entry.Data, &usage); err != nil {
			return nil, err
		}
		return &usage, nil
	}
	if !entry.OK && entry.RateLimited {
		if entry.RetryAfter > 0 && time.Now().Unix() < entry.RetryAfter {
			return nil, errCachedRateLimited
		}
		// Fallback for cache entries without RetryAfter (e.g. written by older versions).
		if entry.RetryAfter == 0 && age < cacheTTLRateLimitDefault {
			return nil, errCachedRateLimited
		}
		// Deadline passed or fallback TTL expired — allow re-fetch.
		return nil, errors.New("cache expired")
	}
	if !entry.OK && age < cacheTTLFail {
		return nil, errCachedFailure
	}

	return nil, errors.New("cache expired")
}

// writeCache writes usage data to the cache file.
func writeCache(usage *usageResponse, ok bool, retryAfter time.Duration) {
	entry := cacheEntry{
		Timestamp:   time.Now().Unix(),
		OK:          ok,
		RateLimited: retryAfter > 0,
	}
	if retryAfter > 0 {
		entry.RetryAfter = time.Now().Add(retryAfter).Unix()
	}
	if usage != nil {
		data, err := json.Marshal(usage)
		if err != nil {
			return
		}
		entry.Data = data
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = os.WriteFile(cacheFilePath(), data, 0o600)
}

// fetchUsageAPI makes the HTTP request to the usage API.
// On rate limit (429), retryAfter contains the duration from the retry-after header.
//
// NOTE: The undocumented OAuth usage API (/api/oauth/usage) has been observed
// to return "Retry-After: 0" on 429 responses. Per the HTTP spec, 0 means
// "retry now", but blindly doing so would hammer the API. To distinguish a
// genuine "retry now" from a bad/unset header, we perform a single immediate
// retry. If the retry also returns 429, we treat "0" as a bad signal and
// fall back to the conservative default TTL (cacheTTLRateLimitDefault).
func fetchUsageAPI(ctx context.Context, token string) (_ *usageResponse, retryAfter time.Duration, _ error) {
	usage, rawRetryAfter, err := doUsageRequest(ctx, token)
	if err == nil {
		return usage, 0, nil
	}
	if !errors.Is(err, errRateLimited) {
		return nil, 0, err
	}

	// If Retry-After is "0", the API claims we can retry immediately.
	// Try once more — if it fails again, treat it as a bad signal.
	if rawRetryAfter != "0" {
		ra := parseRetryAfter(rawRetryAfter)
		return nil, ra, err
	}
	log.Printf("retry-after=0, retrying once to verify")
	usage, _, err = doUsageRequest(ctx, token)
	if err == nil {
		return usage, 0, nil
	}
	if !errors.Is(err, errRateLimited) {
		return nil, 0, err
	}
	// Second attempt also failed — "0" was a bad signal.
	log.Printf("retry-after=0 on second attempt, treating as bad signal, using default TTL")
	return nil, cacheTTLRateLimitDefault, err
}

// doUsageRequest performs a single HTTP request to the usage API.
// On 429, it returns errRateLimited along with the raw Retry-After header value.
func doUsageRequest(ctx context.Context, token string) (_ *usageResponse, rawRetryAfter string, _ error) {
	ctx, cancel := context.WithTimeout(ctx, ioTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Anthropic-Beta", "oauth-2025-04-20")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		raw := resp.Header.Get("Retry-After")
		log.Printf("rate limited: status=%d retry-after=%q body=%s", resp.StatusCode, raw, body)
		return nil, raw, fmt.Errorf("unexpected status %d: %w", resp.StatusCode, errRateLimited)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var usage usageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, "", fmt.Errorf("decode response: %w", err)
	}
	return &usage, "", nil
}

// parseRetryAfter parses the Retry-After header value as seconds (integer)
// or as an HTTP-date (RFC1123). Returns cacheTTLRateLimitDefault if the
// header is missing, zero, or unparseable, clamped to cacheTTLRateLimitMaxBackoff.
// See https://platform.claude.com/docs/en/api/rate-limits for details.
//
// NOTE: Values <= 0 return the default TTL. The caller (fetchUsageAPI)
// handles the "Retry-After: 0" case separately with a single retry.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return cacheTTLRateLimitDefault
	}
	// Try as seconds first (most common for APIs).
	// Requires secs > 0 to avoid treating "0" as "retry immediately".
	if secs, err := strconv.Atoi(value); err == nil && secs > 0 {
		d := time.Duration(secs) * time.Second
		return min(d, cacheTTLRateLimitMaxBackoff)
	}
	// Try as HTTP-date (RFC1123).
	if t, err := time.Parse(time.RFC1123, value); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return cacheTTLRateLimitDefault
		}
		return min(d, cacheTTLRateLimitMaxBackoff)
	}
	return cacheTTLRateLimitDefault
}
