package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDirSuffix(t *testing.T) {
	tests := []struct {
		name            string
		claudeConfigDir string
		wantSuffix      string
	}{
		{
			name:            "no CLAUDE_CONFIG_DIR set",
			claudeConfigDir: "",
			wantSuffix:      "",
		},
		{
			name:            "custom config dir claude-personal",
			claudeConfigDir: "/Users/oa/.claude-personal",
			wantSuffix:      "-81c94270",
		},
		{
			name:            "custom config dir claude-work",
			claudeConfigDir: "/Users/oa/.claude-work",
			wantSuffix:      "-1ef5702c",
		},
		{
			name:            "windows config dir claude-personal",
			claudeConfigDir: `C:\Users\oa\.claude-personal`,
			wantSuffix:      "-9b705f7c",
		},
		{
			name:            "windows config dir claude-work",
			claudeConfigDir: `C:\Users\oa\.claude-work`,
			wantSuffix:      "-34fd078b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", tt.claudeConfigDir)
			got := configDirSuffix()
			if got != tt.wantSuffix {
				t.Errorf("configDirSuffix() = %q, want %q", got, tt.wantSuffix)
			}
		})
	}

	// Verify the suffix is correctly wired into each path/name function.
	t.Run("wiring", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", "/Users/oa/.claude-work")
		suffix := configDirSuffix()

		if got := cacheFilePath(); got != filepath.Join(cacheDir(), "usage"+suffix+".json") {
			t.Errorf("cacheFilePath() = %q, want suffix %q", got, suffix)
		}
		if got := debugLogFilePath(); got != filepath.Join(cacheDir(), "debug"+suffix+".log") {
			t.Errorf("debugLogFilePath() = %q, want suffix %q", got, suffix)
		}
		if got := statusCacheFilePath(); got != filepath.Join(cacheDir(), "status"+suffix+".json") {
			t.Errorf("statusCacheFilePath() = %q, want suffix %q", got, suffix)
		}
		if got := keychainServiceName(); got != "Claude Code-credentials"+suffix {
			t.Errorf("keychainServiceName() = %q, want suffix %q", got, suffix)
		}
	})
}

func TestCompactName(t *testing.T) {
	t.Parallel()

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
		{
			name:   "multibyte unicode",
			input:  "日本語テスト文字列",
			maxLen: 5,
			want:   "日本…字列",
		},
		{
			name:   "maxLen 3",
			input:  "abcdef",
			maxLen: 3,
			want:   "a…f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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
	t.Parallel()

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
		{
			name:   "bare windows drive letter",
			cwd:    "C:",
			maxLen: 30,
			want:   "",
		},
		{
			name:   "dot cwd",
			cwd:    ".",
			maxLen: 30,
			want:   "",
		},
		{
			name:   "backslash only",
			cwd:    `\`,
			maxLen: 30,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cwdName(tt.cwd, tt.maxLen)
			if got != tt.want {
				t.Errorf("cwdName(%q, %d) = %q, want %q", tt.cwd, tt.maxLen, got, tt.want)
			}
		})
	}
}

func BenchmarkRun(b *testing.B) {
	// Use testdata files so the benchmark is fully offline.
	stdinFile := "internal/stdin/testdata/stdin_pro_opus.json"
	usageFile := "internal/usage/testdata/usage_pro.json"
	statusFile := "internal/status/testdata/status.json"

	stdinData, err := os.ReadFile(stdinFile)
	if err != nil {
		b.Fatalf("read stdin testdata: %v", err)
	}

	cfg := config{
		usageFile:       usageFile,
		statusFile:      statusFile,
		gitBranchMaxLen: 30,
		cwdMaxLen:       30,
	}

	// Discard stdout to avoid benchmark noise.
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer devNull.Close()
	origStdout := os.Stdout
	os.Stdout = devNull
	defer func() { os.Stdout = origStdout }()

	b.ResetTimer()
	for b.Loop() {
		r, w, err := os.Pipe()
		if err != nil {
			b.Fatal(err)
		}
		if _, err := w.Write(stdinData); err != nil {
			b.Fatal(err)
		}
		w.Close()

		os.Stdin = r
		if err := run(cfg); err != nil {
			b.Fatalf("run: %v", err)
		}
		r.Close()
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
		cmd := exec.CommandContext(t.Context(), "git", args...)
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

	t.Run("no git directory", func(t *testing.T) {
		noGit := t.TempDir()
		if err := os.Chdir(noGit); err != nil {
			t.Fatal(err)
		}
		// Chdir back before TempDir cleanup — Windows can't remove the cwd.
		t.Cleanup(func() { _ = os.Chdir(orig) })
		got := getBranch()
		if got != "" {
			t.Errorf("getBranch() = %q, want empty string", got)
		}
	})
}
