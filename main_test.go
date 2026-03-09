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
