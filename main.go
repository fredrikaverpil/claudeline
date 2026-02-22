package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
	cacheFile    = "/tmp/claudemeter-usage.json"
	cacheTTLOK   = 60 * time.Second
	cacheTTLFail = 15 * time.Second
	usageURL     = "https://api.anthropic.com/api/oauth/usage"
	httpTimeout  = 5 * time.Second
	barWidth     = 5
)

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
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "claudemeter: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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
		// Non-fatal: we just won't show usage.
		creds = credentials{}
	}

	// Determine plan name.
	plan := planName(creds.ClaudeAiOauth.SubscriptionType)

	// Build identity segment.
	identity := buildIdentity(data.Model.DisplayName, plan)

	// Context bar.
	contextPct := 0
	if data.ContextWindow.UsedPercentage != nil {
		contextPct = int(*data.ContextWindow.UsedPercentage)
	}
	contextBar := bar(contextPct, contextColor)

	// Usage bars.
	var usage5h, usage7d string
	token := creds.ClaudeAiOauth.AccessToken
	if token != "" && plan != "" {
		usage, fetchErr := fetchUsage(token)
		if fetchErr == nil && usage != nil {
			pct5 := int(usage.FiveHour.Utilization)
			usage5h = bar(pct5, quotaColor)
			if reset := formatLocalTime(usage.FiveHour.ResetsAt, "15:04"); reset != "" {
				usage5h += " (" + reset + ")"
			}

			pct7 := int(usage.SevenDay.Utilization)
			usage7d = bar(pct7, quotaColor)
			if reset := formatLocalTime(usage.SevenDay.ResetsAt, "Mon 15:04"); reset != "" {
				usage7d += " (" + reset + ")"
			}
		}
	}

	// Render output.
	sep := dim + " │ " + ansiReset
	output := identity + sep + contextBar
	if usage5h != "" {
		output += sep + usage5h
	}
	if usage7d != "" {
		output += sep + usage7d
	}

	fmt.Println(output)
	return nil
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

// contextColor returns the ANSI color for a context usage percentage.
func contextColor(pct int) string {
	switch {
	case pct >= 85:
		return red
	case pct >= 70:
		return yellow
	default:
		return green
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

// readCredentials reads OAuth credentials from keychain or file.
func readCredentials() (credentials, error) {
	// Try macOS keychain first.
	out, err := exec.Command(
		"/usr/bin/security", "find-generic-password",
		"-s", "Claude Code-credentials", "-w",
	).Output()
	if err == nil {
		var creds credentials
		if err := json.Unmarshal(out, &creds); err != nil {
			return credentials{}, fmt.Errorf("parse keychain credentials: %w", err)
		}
		return creds, nil
	}

	// File fallback.
	home, err := os.UserHomeDir()
	if err != nil {
		return credentials{}, fmt.Errorf("get home dir: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
	if err != nil {
		return credentials{}, fmt.Errorf("read credentials file: %w", err)
	}
	var creds credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return credentials{}, fmt.Errorf("parse credentials file: %w", err)
	}
	return creds, nil
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
	data, err := os.ReadFile(cacheFile)
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
		return nil, fmt.Errorf("cached failure")
	}

	return nil, fmt.Errorf("cache expired")
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
	_ = os.WriteFile(cacheFile, data, 0o600)
}

// fetchUsageAPI makes the HTTP request to the usage API.
func fetchUsageAPI(token string) (*usageResponse, error) {
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest(http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var usage usageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &usage, nil
}
