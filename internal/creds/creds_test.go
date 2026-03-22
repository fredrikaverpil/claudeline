package creds

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// credentials is the complete JSON schema stored in the macOS Keychain
// or ~/.claude/.credentials.json. This struct documents every known field
// and is used in tests with DisallowUnknownFields to detect when Claude Code
// adds new fields. Update this struct and testdata/creds_*.json when the
// schema changes.
type credentials struct {
	ClaudeAiOauth struct {
		AccessToken      string   `json:"accessToken"`
		RefreshToken     string   `json:"refreshToken"`
		ExpiresAt        int64    `json:"expiresAt"`
		Scopes           []string `json:"scopes"`
		SubscriptionType string   `json:"subscriptionType"`
		RateLimitTier    string   `json:"rateLimitTier"`
	} `json:"claudeAiOauth"`
	// McpOAuth is present only when MCP servers with OAuth are configured.
	// It is not plan-specific — it may be absent in any testdata file.
	McpOAuth map[string]struct {
		ServerName     string `json:"serverName"`
		ServerURL      string `json:"serverUrl"`
		AccessToken    string `json:"accessToken"`
		ExpiresAt      int64  `json:"expiresAt"`
		DiscoveryState *struct {
			AuthorizationServerURL string `json:"authorizationServerUrl"`
			ResourceMetadataURL    string `json:"resourceMetadataUrl"`
		} `json:"discoveryState"`
	} `json:"mcpOAuth"`
}

func TestCredentialsSchema(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("testdata/creds_*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no testdata/creds_*.json files found — run ./pok capture to generate")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			// Strict unmarshal: fails if Claude Code added fields we haven't mapped.
			var c credentials
			dec := json.NewDecoder(strings.NewReader(string(data)))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&c); err != nil {
				t.Fatalf(
					"unknown or changed fields in credentials: %v\n"+
						"Update credentials struct and testdata to match the new schema.",
					err,
				)
			}

			// Sanity checks on required fields.
			if c.ClaudeAiOauth.SubscriptionType == "" {
				t.Error("claudeAiOauth.subscriptionType is empty")
			}
			if c.ClaudeAiOauth.AccessToken == "" {
				t.Error("claudeAiOauth.accessToken is empty")
			}
		})
	}
}

func TestRead(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("valid credentials file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		//nolint:gosec // test data
		creds := `{"claudeAiOauth":{"accessToken":"test-token","subscriptionType":"claude_pro_monthly"}}`
		if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(creds), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := Read(ctx, dir, "unused-service")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ClaudeAiOauth.AccessToken != "test-token" {
			t.Errorf("AccessToken = %q, want %q", got.ClaudeAiOauth.AccessToken, "test-token")
		}
		if got.ClaudeAiOauth.SubscriptionType != "claude_pro_monthly" {
			t.Errorf("SubscriptionType = %q, want %q", got.ClaudeAiOauth.SubscriptionType, "claude_pro_monthly")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		_, err := Read(ctx, dir, "unused-service")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "read credentials file") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "read credentials file")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte("{not json}"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := Read(ctx, dir, "unused-service")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "parse credentials file") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "parse credentials file")
		}
	})
}

func TestPlanName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		subType string
		want    string
	}{
		{name: "max", subType: "claude_max_monthly", want: "Max"},
		{name: "pro", subType: "claude_pro_monthly", want: "Pro"},
		{name: "team", subType: "team_monthly", want: "Team"},
		{name: "enterprise", subType: "enterprise_annual", want: "Enterprise"},
		{name: "empty", subType: "", want: ""},
		{name: "unknown", subType: "free", want: ""},
		{name: "case_insensitive", subType: "CLAUDE_PRO_MONTHLY", want: "Pro"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := PlanName(tt.subType)
			if got != tt.want {
				t.Errorf("PlanName(%q) = %q, want %q", tt.subType, got, tt.want)
			}
		})
	}
}
