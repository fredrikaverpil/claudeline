// Package git provides lightweight git helpers.
package git

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

const ioTimeout = 5 * time.Second

// Branch returns the current git branch name for the given directory,
// or "" if not in a git repo or HEAD is detached.
// Delegates to `git -C <dir> symbolic-ref --short HEAD` so subdirectories,
// worktrees, and submodules all work correctly.
func Branch(ctx context.Context, dir string) string {
	ctx, cancel := context.WithTimeout(ctx, ioTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
