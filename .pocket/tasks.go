package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fredrikaverpil/pocket/pk"
	"github.com/fredrikaverpil/pocket/pk/repopath"
	"github.com/fredrikaverpil/pocket/pk/run"
)

// RenderFlags holds flags for the Render task.
type RenderFlags struct {
	JSON       string `flag:"json" usage:"path to a specific testdata JSON file"`
	UsageFile  string `flag:"usage-file" usage:"path to usage testdata JSON file"`
	StatusFile string `flag:"status-file" usage:"path to status testdata JSON file"`
}

// Render runs testdata payloads through claudeline for manual inspection.
// With -json, renders a single file. Without it, renders all testdata/stdin_*.json files.
var Render = &pk.Task{
	Name:  "render",
	Usage: "render statusline from testdata stdin payloads",
	Flags: RenderFlags{
		UsageFile:  "internal/usage/testdata/usage_pro.json",
		StatusFile: "internal/status/testdata/status.json",
	},
	Do: func(ctx context.Context) error {
		flags := run.GetFlags[RenderFlags](ctx)
		dir := repopath.FromGitRoot("")

		var files []string
		if flags.JSON != "" {
			files = []string{flags.JSON}
		} else {
			var err error
			files, err = filepath.Glob(filepath.Join(dir, "internal", "stdin", "testdata", "stdin_*.json"))
			if err != nil {
				return fmt.Errorf("glob testdata: %w", err)
			}
			if len(files) == 0 {
				return fmt.Errorf("no internal/stdin/testdata/stdin_*.json files found")
			}
		}

		// Build once, then reuse the binary for each render.
		bin := filepath.Join(dir, ".pocket", "claudeline-render")
		buildCmd := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
		buildCmd.Dir = dir
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("build: %w", err)
		}
		defer os.Remove(bin)

		for _, f := range files {
			run.Printf(ctx, ":: %s\n", filepath.Base(f))

			// Use captured testdata files so render is fully offline.
			// Try plan-specific usage file first, fall back to the flag default.
			statusFile := filepath.Join(dir, flags.StatusFile)
			plan := extractPlanFromFilename(filepath.Base(f))
			usageFile := filepath.Join(dir, "internal", "usage", "testdata", "usage_"+plan+".json")
			if _, err := os.Stat(usageFile); err != nil {
				usageFile = filepath.Join(dir, flags.UsageFile)
			}
			args := fmt.Sprintf("cat %s | %s -status-file %s -usage-file %s", f, bin, statusFile, usageFile)

			cmd := exec.CommandContext(ctx, "sh", "-c", args)
			cmd.Dir = dir
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return err
			}
			fmt.Println()
		}
		return nil
	},
}

// CaptureFlags holds flags for the Capture task.
type CaptureFlags struct {
	ConfigDir string `flag:"config-dir" usage:"Claude config directory (e.g. ~/.claude-work)"`
}

// Capture reads the latest stdin snapshot from a profile and saves it as a testdata file.
//
// The config-dir flag determines which profile to capture from:
//   - Default (~/.claude): stdin at /tmp/claudeline/stdin.json
//   - Custom (e.g. ~/.claude-work): stdin with a hash suffix,
//     e.g. /tmp/claudeline/stdin-fbb791ba.json
//
// For each profile, the task:
//  1. Reads the stdin snapshot (written by claudeline -debug on each render)
//  2. Resolves the subscription plan from the profile's credentials (Keychain → file fallback)
//  3. Parses the model from the payload
//  4. Sanitizes sensitive fields (paths, cost)
//  5. Saves the payload as internal/stdin/testdata/stdin_<plan>_<model>.json
//
// Prerequisites: claudeline must be running with -debug enabled for the profile
// you want to capture.
var Capture = &pk.Task{
	Name:  "capture",
	Usage: "save latest stdin snapshot as testdata",
	Flags: CaptureFlags{ConfigDir: "~/.claude"},
	Do: func(ctx context.Context) error {
		flags := run.GetFlags[CaptureFlags](ctx)
		dir := repopath.FromGitRoot("")

		// Expand ~ in the config dir path.
		configDir := expandHome(flags.ConfigDir)

		// Determine the file suffix. The default ~/.claude profile
		// has no CLAUDE_CONFIG_DIR set, so it uses no suffix.
		var suffix string
		defaultDir := filepath.Join(mustUserHomeDir(), ".claude")
		if configDir != defaultDir {
			h := sha256.Sum256([]byte(configDir))
			suffix = fmt.Sprintf("-%x", h[:4])
		}

		// Read the stdin snapshot written by claudeline -debug.
		stdinPath := filepath.Join(claudelineCacheDir(), "stdin"+suffix+".json")
		rawJSONBytes, err := os.ReadFile(stdinPath)
		if err != nil {
			return fmt.Errorf("read stdin snapshot %s: %w — run claudeline with -debug first", stdinPath, err)
		}
		rawJSON := string(rawJSONBytes)

		// Parse model.
		var payload struct {
			Model struct {
				ID string `json:"id"`
			} `json:"model"`
		}
		if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
			return fmt.Errorf("parse stdin JSON: %w", err)
		}

		// Resolve plan from this profile's credentials.
		plan, err := resolvePlanName(ctx, configDir)
		if err != nil {
			return fmt.Errorf("resolve plan: %w", err)
		}

		// Sanitize and pretty-print.
		var m map[string]any
		if err := json.Unmarshal([]byte(rawJSON), &m); err != nil {
			return fmt.Errorf("parse JSON: %w", err)
		}
		sanitizePayload(m)
		formatted, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return fmt.Errorf("format JSON: %w", err)
		}

		model := modelShortName(payload.Model.ID)
		filename := fmt.Sprintf("stdin_%s_%s.json", plan, model)
		stdinTestdata := filepath.Join(dir, "internal", "stdin", "testdata")
		outPath := filepath.Join(stdinTestdata, filename)
		if err := os.MkdirAll(stdinTestdata, 0o755); err != nil {
			return fmt.Errorf("create testdata dir: %w", err)
		}
		if err := os.WriteFile(outPath, append(formatted, '\n'), 0o644); err != nil {
			return fmt.Errorf("write testdata: %w", err)
		}

		run.Printf(ctx, "saved %s\n", outPath)

		// Also capture sanitized credentials.
		credsJSON, credsErr := readCredentials(ctx, configDir)
		if credsErr != nil {
			run.Printf(ctx, "skipping credentials capture: %v\n", credsErr)
			return nil
		}

		// Extract access token before sanitization replaces it.
		var accessToken string
		if oauth, ok := credsJSON["claudeAiOauth"].(map[string]any); ok {
			accessToken, _ = oauth["accessToken"].(string)
		}

		sanitizeCredentials(credsJSON)
		formattedCreds, err := json.MarshalIndent(credsJSON, "", "  ")
		if err != nil {
			return fmt.Errorf("format credentials JSON: %w", err)
		}
		credsFile := fmt.Sprintf("creds_%s.json", plan)
		credsTestdata := filepath.Join(dir, "internal", "creds", "testdata")
		if err := os.MkdirAll(credsTestdata, 0o755); err != nil {
			return fmt.Errorf("create creds testdata dir: %w", err)
		}
		credsPath := filepath.Join(credsTestdata, credsFile)
		if err := os.WriteFile(credsPath, append(formattedCreds, '\n'), 0o644); err != nil {
			return fmt.Errorf("write credentials testdata: %w", err)
		}
		run.Printf(ctx, "saved %s\n", credsPath)

		// Also capture usage API response.
		if accessToken == "" {
			run.Printf(ctx, "skipping usage capture: no access token found\n")
		} else {
			usageJSON, usageErr := fetchUsageJSON(ctx, accessToken)
			if usageErr != nil {
				run.Printf(ctx, "skipping usage capture: %v\n", usageErr)
			} else {
				formattedUsage, err := json.MarshalIndent(usageJSON, "", "  ")
				if err != nil {
					return fmt.Errorf("format usage JSON: %w", err)
				}
				usageTestdata := filepath.Join(dir, "internal", "usage", "testdata")
				if err := os.MkdirAll(usageTestdata, 0o755); err != nil {
					return fmt.Errorf("create usage testdata dir: %w", err)
				}
				usageFile := fmt.Sprintf("usage_%s.json", plan)
				usagePath := filepath.Join(usageTestdata, usageFile)
				if err := os.WriteFile(usagePath, append(formattedUsage, '\n'), 0o644); err != nil {
					return fmt.Errorf("write usage testdata: %w", err)
				}
				run.Printf(ctx, "saved %s\n", usagePath)
			}
		}

		// Also capture status API response.
		statusJSON, statusErr := fetchStatusJSON(ctx)
		if statusErr != nil {
			run.Printf(ctx, "skipping status capture: %v\n", statusErr)
			return nil
		}
		formattedStatus, err := json.MarshalIndent(statusJSON, "", "  ")
		if err != nil {
			return fmt.Errorf("format status JSON: %w", err)
		}
		statusTestdata := filepath.Join(dir, "internal", "status", "testdata")
		if err := os.MkdirAll(statusTestdata, 0o755); err != nil {
			return fmt.Errorf("create status testdata dir: %w", err)
		}
		statusPath := filepath.Join(statusTestdata, "status.json")
		if err := os.WriteFile(statusPath, append(formattedStatus, '\n'), 0o644); err != nil {
			return fmt.Errorf("write status testdata: %w", err)
		}
		run.Printf(ctx, "saved %s\n", statusPath)

		return nil
	},
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		return filepath.Join(mustUserHomeDir(), path[1:])
	}
	return path
}

// mustUserHomeDir returns the user's home directory or panics.
func mustUserHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintf("get home dir: %v", err))
	}
	return home
}

// sanitizePayload removes or replaces sensitive fields in a stdin payload map
// before writing to testdata. Mutates the map in place.
func sanitizePayload(m map[string]any) {
	// Identity.
	setNestedString(m, "sanitized-session-id", "session_id")

	// Paths.
	setNestedString(m, "/sanitized/transcript.jsonl", "transcript_path")
	setNestedString(m, "/sanitized/project", "cwd")
	if ws, ok := m["workspace"].(map[string]any); ok {
		setNestedString(ws, "/sanitized/project", "current_dir")
		setNestedString(ws, "/sanitized/project", "project_dir")
	}

	// Cost — cumulative spending and session timing.
	if cost, ok := m["cost"].(map[string]any); ok {
		cost["total_cost_usd"] = 0
		cost["total_duration_ms"] = 0
		cost["total_api_duration_ms"] = 0
		cost["total_lines_added"] = 0
		cost["total_lines_removed"] = 0
	}
}

// setNestedString sets a key in a map to the given string value.
func setNestedString(m map[string]any, value, key string) {
	if _, ok := m[key]; ok {
		m[key] = value
	}
}

// claudelineCacheDir mirrors claudeline's cacheDir() — /tmp/claudeline on non-Windows.
func claudelineCacheDir() string {
	base := "/tmp"
	if runtime.GOOS == "windows" {
		base = os.TempDir()
	}
	return filepath.Join(base, "claudeline")
}

// resolvePlanName reads credentials for a specific config dir and returns the
// lowercase plan name. It tries the macOS Keychain first, falling back to the
// credentials file on disk.
func resolvePlanName(ctx context.Context, configDir string) (string, error) {
	type creds struct {
		ClaudeAiOauth struct {
			SubscriptionType string `json:"subscriptionType"`
		} `json:"claudeAiOauth"`
	}

	// Compute the keychain suffix for this config dir.
	var keychainSuffix string
	defaultDir := filepath.Join(mustUserHomeDir(), ".claude")
	if configDir != defaultDir {
		h := sha256.Sum256([]byte(configDir))
		keychainSuffix = fmt.Sprintf("-%x", h[:4])
	}

	// Try macOS keychain first.
	if runtime.GOOS == "darwin" {
		serviceName := "Claude Code-credentials" + keychainSuffix
		ctx, cancel := context.WithTimeout(ctx, 5_000_000_000) // 5s
		defer cancel()
		out, err := exec.CommandContext(ctx,
			"/usr/bin/security", "find-generic-password",
			"-s", serviceName, "-w",
		).Output()
		if err == nil {
			var c creds
			if err := json.Unmarshal(out, &c); err == nil {
				if plan := planName(c.ClaudeAiOauth.SubscriptionType); plan != "" {
					return plan, nil
				}
			}
		}
	}

	// File fallback.
	data, err := os.ReadFile(filepath.Join(configDir, ".credentials.json"))
	if err != nil {
		return "", fmt.Errorf("read credentials: %w", err)
	}
	var c creds
	if err := json.Unmarshal(data, &c); err != nil {
		return "", fmt.Errorf("parse credentials: %w", err)
	}
	plan := planName(c.ClaudeAiOauth.SubscriptionType)
	if plan == "" {
		return "", fmt.Errorf("unknown subscription type: %q", c.ClaudeAiOauth.SubscriptionType)
	}
	return plan, nil
}

// planName mirrors claudeline's planName() but returns lowercase for filenames.
// TODO: add case for "free" plan (we don't know the key yet)
func planName(subType string) string {
	lower := strings.ToLower(subType)
	switch {
	case strings.Contains(lower, "max"):
		return "max"
	case strings.Contains(lower, "pro"):
		return "pro"
	case strings.Contains(lower, "team"):
		return "team"
	case strings.Contains(lower, "enterprise"):
		return "enterprise"
	default:
		return ""
	}
}

// readCredentials reads the raw credentials JSON as a generic map.
// Tries macOS Keychain first, falls back to the credentials file.
func readCredentials(ctx context.Context, configDir string) (map[string]any, error) {
	// Compute the keychain suffix for this config dir.
	var keychainSuffix string
	defaultDir := filepath.Join(mustUserHomeDir(), ".claude")
	if configDir != defaultDir {
		h := sha256.Sum256([]byte(configDir))
		keychainSuffix = fmt.Sprintf("-%x", h[:4])
	}

	// Try macOS keychain first.
	if runtime.GOOS == "darwin" {
		serviceName := "Claude Code-credentials" + keychainSuffix
		ctx, cancel := context.WithTimeout(ctx, 5_000_000_000) // 5s
		defer cancel()
		out, err := exec.CommandContext(ctx,
			"/usr/bin/security", "find-generic-password",
			"-s", serviceName, "-w",
		).Output()
		if err == nil {
			var m map[string]any
			if err := json.Unmarshal(out, &m); err == nil {
				return m, nil
			}
		}
	}

	// File fallback.
	data, err := os.ReadFile(filepath.Join(configDir, ".credentials.json"))
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	return m, nil
}

// sanitizeCredentials replaces sensitive values in the credentials map.
// Mutates the map in place.
func sanitizeCredentials(m map[string]any) {
	if oauth, ok := m["claudeAiOauth"].(map[string]any); ok {
		setNestedString(oauth, "sanitized", "accessToken")
		setNestedString(oauth, "sanitized", "refreshToken")
		if _, ok := oauth["expiresAt"]; ok {
			oauth["expiresAt"] = 0
		}
	}
	if mcp, ok := m["mcpOAuth"].(map[string]any); ok {
		// Replace all server entries with a single sanitized entry
		// to avoid leaking server IDs (e.g. "github|a1b2c3d4e5f60718").
		sanitized := make(map[string]any, len(mcp))
		i := 0
		for _, v := range mcp {
			if server, ok := v.(map[string]any); ok {
				setNestedString(server, "sanitized", "accessToken")
				setNestedString(server, "sanitized", "serverName")
				setNestedString(server, "https://sanitized.example.com", "serverUrl")
				if _, ok := server["expiresAt"]; ok {
					server["expiresAt"] = 0
				}
				if ds, ok := server["discoveryState"].(map[string]any); ok {
					setNestedString(ds, "https://sanitized.example.com/oauth", "authorizationServerUrl")
					setNestedString(ds, "https://sanitized.example.com/.well-known", "resourceMetadataUrl")
				}
				sanitized[fmt.Sprintf("server|sanitized_%d", i)] = server
				i++
			}
		}
		m["mcpOAuth"] = sanitized
	}
}

// fetchUsageJSON fetches the usage API response as a generic map.
func fetchUsageJSON(ctx context.Context, token string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5_000_000_000) // 5s
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, "https://api.anthropic.com/api/oauth/usage", nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Anthropic-Beta", "oauth-2025-04-20")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return m, nil
}

// extractPlanFromFilename extracts the plan name from a stdin testdata filename.
// e.g. "stdin_pro_opus.json" → "pro".
func extractPlanFromFilename(name string) string {
	name = strings.TrimPrefix(name, "stdin_")
	name = strings.TrimSuffix(name, ".json")
	parts := strings.SplitN(name, "_", 2)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}

// fetchStatusJSON fetches the status API response as a generic map.
func fetchStatusJSON(ctx context.Context) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5_000_000_000) // 5s
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, "https://status.claude.com/api/v2/status.json", nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return m, nil
}

// modelShortName extracts a short model name from a Claude model ID.
// e.g. "claude-opus-4-6[1m]" → "opus", "claude-sonnet-4-6" → "sonnet".
func modelShortName(id string) string {
	for _, name := range []string{"opus", "sonnet", "haiku"} {
		if strings.Contains(id, name) {
			return name
		}
	}
	s := strings.ReplaceAll(id, "[", "")
	s = strings.ReplaceAll(s, "]", "")
	return s
}
