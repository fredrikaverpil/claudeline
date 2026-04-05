package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	runtimedebug "runtime/debug"
	"sync"

	"github.com/fredrikaverpil/claudeline/internal/creds"
	"github.com/fredrikaverpil/claudeline/internal/git"
	"github.com/fredrikaverpil/claudeline/internal/paths"
	"github.com/fredrikaverpil/claudeline/internal/render"
	"github.com/fredrikaverpil/claudeline/internal/status"
	"github.com/fredrikaverpil/claudeline/internal/stdin"
	"github.com/fredrikaverpil/claudeline/internal/update"
	"github.com/fredrikaverpil/claudeline/internal/usage"
)

// version and commit are set via ldflags by goreleaser.
// When empty, the version falls back to runtime/debug.ReadBuildInfo.
var (
	version   string
	commit    string
	configDir = os.Getenv("CLAUDE_CONFIG_DIR")
)

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
	showCost        bool

	// debug options
	debug      bool
	usageFile  string
	statusFile string
	updateFile string
}

func runMain() int {
	showVersion := flag.Bool("version", false, "print version and exit")
	debugLogFile := paths.MustCacheFile(configDir, "debug.log")
	debug := flag.Bool("debug", false, "write warnings and errors to "+debugLogFile)
	showGitBranch := flag.Bool("git-branch", false, "show git branch in the status line")
	gitBranchMaxLen := flag.Int("git-branch-max-len", 30, "max display length for git branch")
	showCwd := flag.Bool("cwd", false, "show working directory name in the status line")
	cwdMaxLen := flag.Int("cwd-max-len", 30, "max display length for working directory name")
	showCost := flag.Bool("cost", false, "show estimated session cost in the status line (always on for API key users)")
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
	_ = os.MkdirAll(paths.CacheDir(), 0o700)
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
		showCost:        *showCost,
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

	data, err := readStdin(cfg)
	if err != nil {
		return err
	}
	debugMode := cfg.usageFile != "" && cfg.statusFile != ""
	cred, loginType, isProvider := creds.Resolve(ctx, debugMode, configDir)
	remote := fetchRemoteData(ctx, cfg, cred, loginType, isProvider)

	output := render.Build(render.Params{
		LoginType:          loginType,
		Model:              data.Model.DisplayName,
		ContextUsedPct:     data.ContextWindow.UsedPercentage,
		CompactPctOverride: os.Getenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE"),
		Exceeds200kTokens:  data.Exceeds200kTokens,
		Usage:              remote.usage,
		StdinRateLimits:    data.RateLimits,
		SubscriptionType:   cred.ClaudeAiOauth.SubscriptionType,
		Status:             remote.status,
		Update:             remote.update,
		ShowCwd:            cfg.showCwd,
		Cwd:                data.Cwd,
		CwdMaxLen:          cfg.cwdMaxLen,
		ShowBranch:         cfg.showGitBranch,
		Branch:             git.Branch(),
		BranchMaxLen:       cfg.gitBranchMaxLen,
		ShowCost:           cfg.showCost || loginType == creds.ProviderAPI,
		CostUSD:            data.Cost.TotalCostUSD,
	})

	_, err = fmt.Fprintln(os.Stdout, output)
	return err
}

// readStdin reads and parses the stdin JSON payload.
func readStdin(cfg config) (stdin.Data, error) {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return stdin.Data{}, fmt.Errorf("read stdin: %w", err)
	}
	if cfg.debug {
		_ = os.WriteFile(paths.MustCacheFile(configDir, "stdin.json"), input, 0o600)
	}
	data, err := stdin.Parse(input)
	if err != nil {
		return stdin.Data{}, fmt.Errorf("parse stdin: %w", err)
	}
	return data, nil
}

// remoteData holds responses from concurrent API calls.
type remoteData struct {
	usage  *usage.Response
	status *status.Response
	update *update.Response
}

// fetchRemoteData fetches usage, status, and update data concurrently.
func fetchRemoteData(
	ctx context.Context,
	cfg config,
	cred creds.Credentials,
	loginType string,
	isProvider bool,
) remoteData {
	var rd remoteData
	var wg sync.WaitGroup

	// Providers have no 5h/7d quotas — skip usage API.
	if !isProvider {
		token := cred.ClaudeAiOauth.AccessToken
		switch {
		case cfg.usageFile != "":
			resp, err := usage.ReadResponse(cfg.usageFile)
			if err != nil {
				log.Printf("usage: read file: %v", err)
			}
			rd.usage = resp
		case token == "":
			log.Printf("usage: no access token found")
		default:
			usage.FetchAsync(ctx, token, paths.MustCacheFile(configDir, "usage.json"), &wg, &rd.usage)
		}
	}

	if !creds.IsThirdPartyProvider(loginType) {
		if cfg.statusFile != "" {
			resp, err := status.ReadResponse(cfg.statusFile)
			if err != nil {
				log.Printf("status: read file: %v", err)
			}
			rd.status = resp
		} else {
			status.FetchAsync(ctx, paths.MustCacheFile(configDir, "status.json"), &wg, &rd.status)
		}
	}

	if cfg.updateFile != "" {
		resp, err := update.ReadResponse(cfg.updateFile)
		if err != nil {
			log.Printf("update: read file: %v", err)
		}
		rd.update = resp
	} else {
		update.FetchAsync(ctx, currentVersion(), paths.MustCacheFile(configDir, "update.json"), &wg, &rd.update)
	}

	wg.Wait()
	return rd
}
