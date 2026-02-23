package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
			want:            filepath.Join(os.TempDir(), "claudeline-usage.json"),
		},
		{
			name:            "custom config dir claude-personal",
			claudeConfigDir: "/Users/oa/.claude-personal",
			want:            filepath.Join(os.TempDir(), "claudeline-usage-81c94270.json"),
		},
		{
			name:            "custom config dir claude-work",
			claudeConfigDir: "/Users/oa/.claude-work",
			want:            filepath.Join(os.TempDir(), "claudeline-usage-1ef5702c.json"),
		},
		{
			name:            "windows config dir claude-personal",
			claudeConfigDir: `C:\Users\oa\.claude-personal`,
			want:            filepath.Join(os.TempDir(), "claudeline-usage-9b705f7c.json"),
		},
		{
			name:            "windows config dir claude-work",
			claudeConfigDir: `C:\Users\oa\.claude-work`,
			want:            filepath.Join(os.TempDir(), "claudeline-usage-34fd078b.json"),
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

func TestCompactBranch(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		maxLen int
		want   string
	}{
		{
			name:   "short name unchanged",
			branch: "main",
			maxLen: 30,
			want:   "main",
		},
		{
			name:   "exactly at limit",
			branch: strings.Repeat("a", 30),
			maxLen: 30,
			want:   strings.Repeat("a", 30),
		},
		{
			name:   "truncated with ellipsis",
			branch: "backup/feat-support-claudeline-progress-tracker",
			maxLen: 30,
			want:   "backup/feat-su…rogress-tracker",
		},
		{
			name:   "empty string",
			branch: "",
			maxLen: 30,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compactBranch(tt.branch, tt.maxLen)
			if got != tt.want {
				t.Errorf("compactBranch(%q, %d) = %q, want %q", tt.branch, tt.maxLen, got, tt.want)
			}
			if len([]rune(got)) > tt.maxLen {
				t.Errorf("compactBranch(%q, %d) rune length = %d, exceeds maxLen", tt.branch, tt.maxLen, len([]rune(got)))
			}
		})
	}
}

func TestGetBranch(t *testing.T) {
	// In this repo, getBranch should return a non-empty branch name.
	branch := getBranch()
	if branch == "" {
		t.Fatal("getBranch() returned empty string, expected a branch name")
	}
	// Verify it matches git output directly.
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	want := strings.TrimSpace(string(out))
	if branch != want {
		t.Errorf("getBranch() = %q, want %q", branch, want)
	}
}
