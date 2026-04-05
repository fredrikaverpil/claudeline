package creds

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fredrikaverpil/claudeline/internal/paths"
)

const ioTimeout = 5 * time.Second

// API provider names returned by Provider().
const (
	ProviderBedrock = "Bedrock"
	ProviderVertex  = "Vertex"
	ProviderFoundry = "Foundry"
	ProviderAPI     = "API" // Anthropic's API
)

// Subscription display names returned by SubscriptionType().
const (
	SubFree       = "Free"
	SubPro        = "Pro"
	SubMax        = "Max"
	SubTeam       = "Team"
	SubEnterprise = "Enterprise"
	SubDebug      = "Debug" // Only used by claudeline while debugging
	SubUnknown    = "Unknown subscription type"
)

// thirdPartyProviders are providers that use non-Anthropic infrastructure
// (AWS, GCP, Azure). status.claude.com is not relevant for these.
var thirdPartyProviders = map[string]bool{
	ProviderBedrock: true,
	ProviderVertex:  true,
	ProviderFoundry: true,
}

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
		configDir = paths.DefaultConfigDir()
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

// Resolve determines the subscription/provider/API (login) type and credentials from environment
// variables and local credential stores. API providers (Bedrock, Vertex,
// Foundry, API key) skip credential resolution entirely. When debugMode is
// true, the "Debug" is returned without any credential lookup.
func Resolve(ctx context.Context, debugMode bool, configDir string) (Credentials, string, bool) {
	if debugMode {
		return Credentials{}, SubDebug, false
	}
	loginType := Provider()
	if loginType != "" {
		return Credentials{}, loginType, true
	}
	cred, err := Read(ctx, configDir, KeychainServiceName(configDir))
	if err != nil {
		log.Printf("credentials: %v", err)
		return Credentials{}, ProviderAPI, false
	}
	loginType = SubscriptionType(cred.ClaudeAiOauth.SubscriptionType)
	if loginType == "" {
		log.Printf("unknown subscription type: subscription_type=%q", cred.ClaudeAiOauth.SubscriptionType)
		loginType = SubUnknown
	}
	return cred, loginType, false
}

// KeychainServiceName returns the macOS Keychain service name used by Claude Code.
// When configDir is non-empty, a hash suffix is appended to avoid collisions between profiles.
func KeychainServiceName(configDir string) string {
	return "Claude Code-credentials" + paths.ConfigDirSuffix(configDir)
}

// SubscriptionType maps a subscription type to a display name.
func SubscriptionType(subType string) string {
	lower := strings.ToLower(subType)
	switch {
	case strings.Contains(lower, "free"):
		return SubFree
	case strings.Contains(lower, "pro"):
		return SubPro
	case strings.Contains(lower, "max"):
		return SubMax
	case strings.Contains(lower, "team"):
		return SubTeam
	case strings.Contains(lower, "enterprise"):
		return SubEnterprise
	default:
		return ""
	}
}
