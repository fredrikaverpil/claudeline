package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	runtimedebug "runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fredrikaverpil/claudeline/internal/creds"
	"github.com/fredrikaverpil/claudeline/internal/policy"
	"github.com/fredrikaverpil/claudeline/internal/render"
	"github.com/fredrikaverpil/claudeline/internal/status"
	"github.com/fredrikaverpil/claudeline/internal/stdin"
	"github.com/fredrikaverpil/claudeline/internal/update"
	"github.com/fredrikaverpil/claudeline/internal/usage"
)

// version and commit are set via ldflags by goreleaser.
// When empty, the version falls back to runtime/debug.ReadBuildInfo.
var (
	version string
	commit  string
)

var debugLogFile = debugLogFilePath()

func main() {
	os.Exit(runMain())
}

// buildVersion returns the version string. It prefers the ldflags-injected
// version (set by goreleaser), falling back to runtime/debug.ReadBuildInfo
// (set by go install/run and local builds).
func buildVersion() string {
	v := currentVersion()
	if v == "" {
		v = "(unknown)"
	}
	if commit != "" {
		v += " (" + commit + ")"
	}
	return v
}

// currentVersion returns the bare semver string for the running binary.
// It prefers the ldflags-injected version (set by goreleaser), falling back
// to runtime/debug.ReadBuildInfo (set by go install). Returns "" when the
// version cannot be determined (e.g. local dev builds).
func currentVersion() string {
	if version != "" {
		return version
	}
	if info, ok := runtimedebug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return ""
}

// config holds CLI configuration.
type config struct {
	showGitBranch   bool
	gitBranchMaxLen int
	showCwd         bool
	cwdMaxLen       int

	// debug options
	debug      bool
	usageFile  string
	statusFile string
	updateFile string
}

func runMain() int {
	showVersion := flag.Bool("version", false, "print version and exit")
	debug := flag.Bool("debug", false, "write warnings and errors to "+debugLogFile)
	showGitBranch := flag.Bool("git-branch", false, "show git branch in the status line")
	gitBranchMaxLen := flag.Int("git-branch-max-len", 30, "max display length for git branch")
	showCwd := flag.Bool("cwd", false, "show working directory name in the status line")
	cwdMaxLen := flag.Int("cwd-max-len", 30, "max display length for working directory name")
	usageFile := flag.String("usage-file", "", "read usage data from file instead of API")
	statusFile := flag.String("status-file", "", "read status data from file instead of API")
	updateFile := flag.String("update-file", "", "read update data from file instead of API")
	flag.Parse()

	if *showVersion {
		if _, err := fmt.Fprintln(os.Stdout, buildVersion()); err != nil {
			return 1
		}
		return 0
	}

	log.SetPrefix("claudeline: ")
	log.SetFlags(log.Ldate | log.Ltime)
	_ = os.MkdirAll(cacheDir(), 0o700)
	if *debug {
		// Truncate if over 1MB to prevent unbounded growth.
		if info, err := os.Stat(debugLogFile); err == nil && info.Size() > 1024*1024 {
			_ = os.Truncate(debugLogFile, 0)
		}
		f, err := os.OpenFile(debugLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err == nil {
			log.SetOutput(f)
			defer func() { _ = f.Close() }()
		}
	} else {
		log.SetOutput(io.Discard)
	}

	cfg := config{
		debug:           *debug,
		showGitBranch:   *showGitBranch,
		gitBranchMaxLen: *gitBranchMaxLen,
		showCwd:         *showCwd,
		cwdMaxLen:       *cwdMaxLen,
		usageFile:       *usageFile,
		statusFile:      *statusFile,
		updateFile:      *updateFile,
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "claudeline: %v\n", err)
		return 1
	}
	return 0
}

func run(cfg config) error {
	ctx := context.Background()

	// Read stdin JSON.
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	if cfg.debug {
		_ = os.WriteFile(stdinFilePath(), input, 0o600)
	}
	data, err := stdin.Parse(input)
	if err != nil {
		return fmt.Errorf("parse stdin: %w", err)
	}

	// Read credentials (skipped when file overrides are set).
	var cred creds.Credentials
	var plan string
	if cfg.usageFile != "" && cfg.statusFile != "" {
		plan = "Debug"
	} else {
		cred, err = creds.Read(ctx, os.Getenv("CLAUDE_CONFIG_DIR"), keychainServiceName())
		if err != nil {
			log.Printf("credentials: %v", err)
		}
		plan = creds.PlanName(cred.ClaudeAiOauth.SubscriptionType)
	}

	// Build identity segment.
	identity := render.Identity(data.Model.DisplayName, plan)

	// Context bar.
	contextPct := 0
	if data.ContextWindow.UsedPercentage != nil {
		contextPct = int(math.Round(*data.ContextWindow.UsedPercentage))
	}
	// Warn when context is near auto-compaction threshold.
	compactPct := 85
	if v, err := strconv.Atoi(os.Getenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE")); err == nil && v > 0 && v <= 100 {
		compactPct = v
	}
	warnPct := compactPct - 5
	contextBar := render.Bar(contextPct, render.ContextColorFunc(warnPct))
	if contextPct >= warnPct {
		contextBar += " ⚠️"
	}
	if data.Exceeds200kTokens {
		contextBar += " 🥵"
	}

	// Fetch usage data, service status, and update check concurrently.
	var usageResp *usage.Response
	var statusResp *status.Response
	var updateResp *update.Response
	var wg sync.WaitGroup

	token := cred.ClaudeAiOauth.AccessToken
	subType := cred.ClaudeAiOauth.SubscriptionType

	wg.Go(func() {
		if cfg.usageFile != "" {
			resp, err := usage.ReadResponse(cfg.usageFile)
			if err != nil {
				log.Printf("usage: read file: %v", err)
			}
			usageResp = resp
		} else {
			switch {
			case token == "":
				log.Printf("usage: no access token found")
			case plan == "":
				log.Printf(
					"usage: unknown subscription type %q, expected pro/max/team/enterprise",
					subType,
				)
			default:
				resp, err := usage.Fetch(ctx, token, cacheFilePath())
				if err != nil && !errors.Is(err, usage.ErrCachedRateLimited) &&
					!errors.Is(err, usage.ErrCachedFailure) {
					log.Printf("usage: %v", err)
				}
				usageResp = resp
			}
		}
	})

	wg.Go(func() {
		if cfg.statusFile != "" {
			resp, err := status.ReadResponse(cfg.statusFile)
			if err != nil {
				log.Printf("status: read file: %v", err)
			}
			statusResp = resp
		} else {
			resp, err := status.Fetch(ctx, statusCacheFilePath())
			if err != nil {
				log.Printf("status: %v", err)
			}
			statusResp = resp
		}
	})

	wg.Go(func() {
		if cfg.updateFile != "" {
			resp, err := update.ReadResponse(cfg.updateFile)
			if err != nil {
				log.Printf("update: read file: %v", err)
			}
			updateResp = resp
		} else {
			resp, err := update.Fetch(ctx, currentVersion(), updateCacheFilePath())
			if err != nil {
				log.Printf("update: %v", err)
			}
			updateResp = resp
		}
	})

	wg.Wait()

	// Usage bars.
	var usage5h, usage7d, usageExtra string
	if usageResp != nil {
		now := time.Now()
		// 5-hour bar (null on enterprise).
		if usageResp.FiveHour != nil {
			pct5 := int(math.Round(usageResp.FiveHour.Utilization))
			usage5h = render.Bar(pct5, render.QuotaColor)
			if reset := render.ResetTime(usageResp.FiveHour.ResetsAt, now); reset != "" {
				usage5h += " (" + reset + ")"
			}
			if policy.IsPeakHours(now, subType) {
				usage5h = "⚡️" + usage5h
			}
		}

		// 7-day bar, plus per-model sub-bars (null on enterprise).
		if usageResp.SevenDay != nil {
			pct7 := int(math.Round(usageResp.SevenDay.Utilization))
			usage7d = render.Bar(pct7, render.QuotaColor)
			if reset := render.ResetTime(usageResp.SevenDay.ResetsAt, now); reset != "" {
				usage7d += " (" + reset + ")"
			}
			subSep := render.Dim + " · " + render.Reset
			for _, sub := range []struct {
				q     *usage.QuotaLimit
				label string
			}{
				{usageResp.SevenDaySonnet, "sonnet"},
				{usageResp.SevenDayOpus, "opus"},
				{usageResp.SevenDayCowork, "cowork"},
				{usageResp.SevenDayOAuthApp, "oauth"},
			} {
				if sub.q != nil {
					pct := int(math.Round(sub.q.Utilization))
					usage7d += subSep + render.QuotaSubBar(pct, sub.label, render.ResetTime(sub.q.ResetsAt, now))
				}
			}
		}

		// Extra usage.
		if e := usageResp.ExtraUsage; e != nil && e.IsEnabled && e.MonthlyLimit != nil && e.UsedCredits != nil {
			usageExtra = render.ExtraUsage(int(*e.UsedCredits)/100, int(*e.MonthlyLimit)/100)
		}
	}

	// Service status.
	var statusStr string
	if statusResp != nil {
		statusStr = render.StatusIndicator(statusResp.Status.Indicator)
	}

	// Update indicator.
	var updateStr string
	if updateResp != nil {
		updateStr = render.UpdateIndicator(updateResp.TagName)
	}

	// Render output.
	var cwdStr, branchStr string
	if cfg.showCwd {
		if name := cwdName(data.Cwd, cfg.cwdMaxLen); name != "" {
			cwdStr = render.Yellow + name + render.Reset
		}
	}
	if cfg.showGitBranch {
		if branch := compactName(getBranch(), cfg.gitBranchMaxLen); branch != "" {
			branchStr = render.Magenta + branch + render.Reset
		}
	}

	sep := render.Dim + " │ " + render.Reset
	identityFull := identity
	if cwdStr != "" {
		identityFull += sep + cwdStr
	}
	if branchStr != "" {
		identityFull += sep + branchStr
	}

	output := render.Output(identityFull, contextBar, usage5h, usage7d, usageExtra, statusStr, updateStr)

	// Leading reset clears stale ANSI state from previous renders.
	// Non-breaking spaces prevent the terminal from collapsing whitespace.
	output = render.Reset + strings.ReplaceAll(output, " ", "\u00A0")
	_, err = fmt.Fprintln(os.Stdout, output)
	return err
}

// keychainServiceName returns the macOS Keychain service name used by Claude Code.
// When CLAUDE_CONFIG_DIR is set, Claude Code appends a hash suffix to the service name.
func keychainServiceName() string {
	return "Claude Code-credentials" + configDirSuffix()
}

// cacheDir returns the directory for claudeline cache and log files.
// Uses /tmp/claudeline on Unix and os.TempDir()/claudeline on Windows.
func cacheDir() string {
	base := "/tmp"
	if runtime.GOOS == "windows" {
		base = os.TempDir()
	}
	return filepath.Join(base, "claudeline")
}

// configDirSuffix returns a hash-based suffix when CLAUDE_CONFIG_DIR is set,
// or an empty string when it is unset. This avoids collisions between profiles.
func configDirSuffix() string {
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		return ""
	}
	h := sha256.Sum256([]byte(configDir))
	return fmt.Sprintf("-%x", h[:4])
}

// debugLogFilePath returns the file path for the debug log.
// When CLAUDE_CONFIG_DIR is set, a hash suffix is appended to avoid collisions between profiles.
func debugLogFilePath() string {
	return filepath.Join(cacheDir(), "debug"+configDirSuffix()+".log")
}

// cacheFilePath returns the file path for the usage cache.
// When CLAUDE_CONFIG_DIR is set, a hash suffix is appended to avoid collisions between profiles.
func cacheFilePath() string {
	return filepath.Join(cacheDir(), "usage"+configDirSuffix()+".json")
}

// statusCacheFilePath returns the file path for the status cache.
func statusCacheFilePath() string {
	return filepath.Join(cacheDir(), "status"+configDirSuffix()+".json")
}

// updateCacheFilePath returns the file path for the update check cache.
func updateCacheFilePath() string {
	return filepath.Join(cacheDir(), "update"+configDirSuffix()+".json")
}

// stdinFilePath returns the file path for the latest stdin payload snapshot.
func stdinFilePath() string {
	return filepath.Join(cacheDir(), "stdin"+configDirSuffix()+".json")
}

// getBranch returns the current git branch name, or "" if not in a git repo.
func getBranch() string {
	data, err := os.ReadFile(".git/HEAD")
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	if after, ok := strings.CutPrefix(s, "ref: refs/heads/"); ok {
		return after
	}
	return "" // detached HEAD or bare repo
}

// cwdName extracts the last path segment from cwd as the folder name.
func cwdName(cwd string, maxLen int) string {
	// Normalize separators for cross-platform support.
	name := filepath.Base(strings.ReplaceAll(cwd, `\`, "/"))
	switch {
	case name == "." || name == "/" || name == `\`:
		return ""
	case len(name) == 2 && name[1] == ':':
		// Bare Windows drive letter (e.g. "C:") — root of a drive.
		return ""
	}
	return compactName(name, maxLen)
}

// compactName truncates a name to maxLen runes using a Unicode ellipsis.
func compactName(name string, maxLen int) string {
	runes := []rune(name)
	if len(runes) <= maxLen {
		return name
	}
	half := (maxLen - 1) / 2
	return string(runes[:half]) + "…" + string(runes[len(runes)-(maxLen-1-half):])
}
