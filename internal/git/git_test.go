package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func initRepo(t *testing.T, dir string) {
	t.Helper()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "Test")
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestBranch(t *testing.T) {
	tests := []struct {
		name string
		// setup prepares a scenario rooted at repo and returns the directory
		// to pass to Branch().
		setup func(t *testing.T, repo string) string
		want  string
	}{
		{
			name: "default branch",
			setup: func(t *testing.T, repo string) string {
				initRepo(t, repo)
				return repo
			},
			want: "main",
		},
		{
			name: "branch with slashes",
			setup: func(t *testing.T, repo string) string {
				initRepo(t, repo)
				gitRun(t, repo, "switch", "-c", "feat/my-feature")
				return repo
			},
			want: "feat/my-feature",
		},
		{
			name: "subdirectory",
			setup: func(t *testing.T, repo string) string {
				initRepo(t, repo)
				sub := filepath.Join(repo, "nested", "deep")
				mkdir(t, sub)
				return sub
			},
			want: "main",
		},
		{
			name: "worktree root",
			setup: func(t *testing.T, repo string) string {
				initRepo(t, repo)
				gitRun(t, repo, "commit", "--allow-empty", "-m", "init")
				wt := filepath.Join(t.TempDir(), "wt")
				gitRun(t, repo, "worktree", "add", "-b", "wt-branch", wt)
				return wt
			},
			want: "wt-branch",
		},
		{
			name: "worktree subdirectory",
			setup: func(t *testing.T, repo string) string {
				initRepo(t, repo)
				gitRun(t, repo, "commit", "--allow-empty", "-m", "init")
				wt := filepath.Join(t.TempDir(), "wt")
				gitRun(t, repo, "worktree", "add", "-b", "wt-branch", wt)
				sub := filepath.Join(wt, "nested", "deep")
				mkdir(t, sub)
				return sub
			},
			want: "wt-branch",
		},
		{
			name: "detached HEAD",
			setup: func(t *testing.T, repo string) string {
				initRepo(t, repo)
				gitRun(t, repo, "commit", "--allow-empty", "-m", "init")
				gitRun(t, repo, "switch", "--detach")
				return repo
			},
			want: "",
		},
		{
			name: "no git directory",
			setup: func(_ *testing.T, repo string) string {
				return repo
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t, t.TempDir())
			got := Branch(t.Context(), dir)
			if got != tt.want {
				t.Errorf("Branch() = %q, want %q", got, tt.want)
			}
		})
	}
}
