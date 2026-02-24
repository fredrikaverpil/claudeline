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
	cyan          = "\033[36m"
	brightBlue    = "\033[94m"
	brightMagenta = "\033[95m"
	dim           = "\033[2m"
	ansiReset     = "\033[0m"
)

const (
	cacheTTLOK   = 60 * time.Second
	cacheTTLFail = 15 * time.Second
	usageURL     = "https://api.anthropic.com/api/oauth/usage"
	httpTimeout  = 5 * time.Second
	barWidth     = 5
)

var debugLogFile = filepath.Join(os.TempDir(), "claudeline-debug.log")

// stdinData is the JSON structure received from Claude Code via stdin.
type stdinData struct {
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
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
	OK        bool            `json:"ok"`
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
	showGitTag   bool
	gitTagMaxLen int
}

func runMain() int {
	showVersion := flag.Bool("version", false, "print version and exit")
	debug := flag.Bool("debug", false, "write warnings and errors to "+debugLogFile)
	showGitTag := flag.Bool("git-tag", false, "show git tag in the status line")
	gitTagMaxLen := flag.Int("git-tag-max-len", 30, "max display length for git tag")
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
		showGitTag:   *showGitTag,
		gitTagMaxLen: *gitTagMaxLen,
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "claudeline: %v\n", err)
		return 1
	}
	return 0
}

func run(cfg config) error {
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
	creds, err := readCredentials()
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
		usage, fetchErr := fetchUsage(token)
		if fetchErr != nil {
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
	if branch := compactName(getBranch(), 30); branch != "" {
		output += sep + dim + branch + ansiReset
	}
	if cfg.showGitTag {
		if tag := compactName(getTag(), cfg.gitTagMaxLen); tag != "" {
			output += sep + yellow + tag + ansiReset
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

// contextColorFunc returns a color function for context usage that turns red
// at the given warning percentage (compaction threshold minus margin).
func contextColorFunc(warnPct int) func(int) string {
	return func(pct int) string {
		switch {
		case pct >= warnPct:
			return red
		case pct >= 70:
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

// cacheFilePath returns the file path for the usage cache.
// When CLAUDE_CONFIG_DIR is set, a hash suffix is appended to avoid collisions between profiles.
func cacheFilePath() string {
	base := filepath.Join(os.TempDir(), "claudeline-usage")
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		return base + ".json"
	}
	h := sha256.Sum256([]byte(configDir))
	return fmt.Sprintf("%s-%x.json", base, h[:4])
}

// readCredentials reads OAuth credentials from keychain or file.
func readCredentials() (credentials, error) {
	// Try macOS keychain first.
	if runtime.GOOS == "darwin" {
		serviceName := keychainServiceName()
		out, err := exec.Command(
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

// getTag returns the tag pointing at HEAD, or "" if HEAD is not tagged.
func getTag() string {
	out, err := exec.Command("git", "tag", "--points-at", "HEAD").Output()
	if err != nil {
		return ""
	}
	// git tag --points-at can return multiple tags; take the first.
	tag, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return tag
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
func fetchUsage(token string) (*usageResponse, error) {
	// Check cache.
	if cached, err := readCache(); err == nil {
		return cached, nil
	}

	// Fetch from API.
	usage, err := fetchUsageAPI(token)
	if err != nil {
		writeCache(nil, false)
		return nil, fmt.Errorf("fetch usage API: %w", err)
	}

	writeCache(usage, true)
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
	if !entry.OK && age < cacheTTLFail {
		return nil, errors.New("cached failure")
	}

	return nil, errors.New("cache expired")
}

// writeCache writes usage data to the cache file.
func writeCache(usage *usageResponse, ok bool) {
	entry := cacheEntry{
		Timestamp: time.Now().Unix(),
		OK:        ok,
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
func fetchUsageAPI(token string) (*usageResponse, error) {
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Anthropic-Beta", "oauth-2025-04-20")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var usage usageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &usage, nil
}
