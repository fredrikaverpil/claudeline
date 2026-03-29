package creds

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const ioTimeout = 5 * time.Second

// Credentials is the OAuth credentials structure.
type Credentials struct {
	ClaudeAiOauth struct {
		AccessToken      string `json:"accessToken"`
		SubscriptionType string `json:"subscriptionType"`
	} `json:"claudeAiOauth"`
}

// Read reads OAuth credentials from keychain or file.
// configDir is the Claude config directory (falls back to ~/.claude if empty).
// keychainService is the macOS Keychain service name.
func Read(ctx context.Context, configDir, keychainService string) (Credentials, error) {
	// Try macOS keychain first.
	if runtime.GOOS == "darwin" {
		ctx, cancel := context.WithTimeout(ctx, ioTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx,
			"/usr/bin/security", "find-generic-password",
			"-s", keychainService, "-w",
		).Output()
		if err == nil {
			var creds Credentials
			if err := json.Unmarshal(out, &creds); err != nil {
				return Credentials{}, fmt.Errorf("parse keychain credentials: %w", err)
			}
			return creds, nil
		}
	}

	// File fallback.
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Credentials{}, fmt.Errorf("get home dir: %w", err)
		}
		configDir = filepath.Join(home, ".claude")
	}
	data, err := os.ReadFile(
		filepath.Join(configDir, ".credentials.json"),
	)
	if err != nil {
		return Credentials{}, fmt.Errorf("read credentials file: %w", err)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("parse credentials file: %w", err)
	}
	return creds, nil
}

// API provider names returned by Provider().
const (
	ProviderBedrock = "Bedrock"
	ProviderVertex  = "Vertex"
	ProviderFoundry = "Foundry"
	ProviderAPI     = "API" // Anthropic's API
)

// thirdPartyProviders are providers that use non-Anthropic infrastructure
// (AWS, GCP, Azure). status.claude.com is not relevant for these.
var thirdPartyProviders = map[string]bool{
	ProviderBedrock: true,
	ProviderVertex:  true,
	ProviderFoundry: true,
}

// Provider returns the API provider name based on environment variables.
// Returns empty string if no API provider is detected (subscription mode).
// Precedence follows Claude Code's authentication order:
// Bedrock > Vertex > Foundry > API key/bearer token.
func Provider() string {
	switch {
	case os.Getenv("CLAUDE_CODE_USE_BEDROCK") == "1":
		return ProviderBedrock
	case os.Getenv("CLAUDE_CODE_USE_VERTEX") == "1":
		return ProviderVertex
	case os.Getenv("CLAUDE_CODE_USE_FOUNDRY") == "1":
		return ProviderFoundry
	case os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != "":
		return ProviderAPI
	default:
		return ""
	}
}

// IsThirdPartyProvider reports whether the provider uses non-Anthropic infrastructure.
func IsThirdPartyProvider(provider string) bool {
	return thirdPartyProviders[provider]
}

// PlanName maps a subscription type to a display name.
func PlanName(subType string) string {
	lower := strings.ToLower(subType)
	switch {
	case strings.Contains(lower, "free"):
		return "Free"
	case strings.Contains(lower, "pro"):
		return "Pro"
	case strings.Contains(lower, "max"):
		return "Max"
	case strings.Contains(lower, "team"):
		return "Team"
	case strings.Contains(lower, "enterprise"):
		return "Enterprise"
	default:
		return ""
	}
}
