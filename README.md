# claudeline

A minimal, opinionated and non-configurable, Claude Code status line.

```sh
[Opus 4.6 | Team] │ ████░ 80% ⚠ │ ███░░ 74% (13:00) │ █░░░░ 30% (Thu 10:00)
```

## Installation

### Prerequisites

- [Go](https://go.dev/dl/) 1.26+

### Via Claude Code plugin

1. Inside Claude Code, add the plugin marketplace and install:

```
/plugin marketplace add fredrikaverpil/claudeline
/plugin install claudeline@claudeline
```

2. Run `/claudeline:setup` inside Claude Code
3. Restart Claude Code

### Manual

1. Install the binary:

```bash
go install github.com/fredrikaverpil/claudeline@latest
```

2. Add the statusline to `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "~/go/bin/claudeline"
  }
}
```

3. Restart Claude Code

> [!NOTE]
>
> If you have a custom `$GOPATH`, replace `~/go/bin` with `$GOPATH/bin`.

## About

A Claude Code statusline plugin written in Go. It displays the current AI model,
subscription plan, context window usage, and 5-hour/7-day quota usage as
ANSI-colored progress bars. Zero external dependencies (stdlib only).

The binary reads JSON from stdin (provided by Claude Code) and writes a single
ANSI-colored line to stdout.

> [!NOTE]
>
> The 5-hour and 7-day quota bars require a Claude Code subscription (Pro, Max,
> or Team). They are not available for free tier or API key users.

## Architecture

Single-file (`main.go`), single-package (`main`) design.

**Data flow:** stdin JSON → parse input + read credentials → fetch usage
(cached) → render ANSI output → stdout

Key components:

- **Credential resolution:** macOS Keychain (`security find-generic-password`)
  first, falls back to `~/.claude/.credentials.json`. Works on any platform via
  the file fallback. Failure is non-fatal (usage bars are omitted).
- **Usage API:** `GET https://api.anthropic.com/api/oauth/usage` with OAuth
  bearer token. 5-second HTTP timeout.
- **File-based cache:** `/tmp/claudeline-usage.json` with 60s TTL on success,
  15s TTL on failure.
- **Progress bars:** 5-char width using `█`/`░` with color thresholds
  (green/yellow/red for context; blue/magenta/red for quota).
- **Compaction warning:** A yellow `⚠` appears on the context bar when usage is
  within 5% of the auto-compaction threshold (85% by default, configurable via
  `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`).
- **Custom .claude folder**: Support `CLAUDE_CONFIG_DIR`.
- **Debug mode:** Pass `-debug` to write warnings and errors to
  `/tmp/claudeline-debug.log`. Set the statusline command to
  `claudeline -debug`, then `tail -f /tmp/claudeline-debug.log` in another
  terminal.

## Development

This project uses [Pocket](https://github.com/fredrikaverpil/pocket), a
Makefile-like task runner. Run `./pok` to execute linting, formatting, and
tests.

## References

- [claude-hud](https://github.com/jarrodwatts/claude-hud) — inspiration for this
  project
- [Create Claude plugins](https://code.claude.com/docs/en/plugins)
- [Customize your status line](https://code.claude.com/docs/en/statusline)
- [Costs and context window](https://code.claude.com/docs/en/costs)
