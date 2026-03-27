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

// PlanName maps a subscription type to a display name.
// TODO: add case for "free" plan (we don't know the key yet).
func PlanName(subType string) string {
	lower := strings.ToLower(subType)
	switch {
	case strings.Contains(lower, "max"):
		return "Max"
	case strings.Contains(lower, "pro"):
		return "Pro"
	case strings.Contains(lower, "team"):
		return "Team"
	case strings.Contains(lower, "enterprise"):
		return "Enterprise"
	default:
		return ""
	}
}
