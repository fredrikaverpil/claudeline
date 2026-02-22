# claudemeter

A minimal, opinionated and non-configurable, Claude Code status line.

```sh
[Opus 4.6 | Team] │ █░░░░ 18% │ ██░░░ 40% (12:59) │ █░░░░ 27% (Thu 10:00)
```

## Installation

### Prerequisites

- [Go](https://go.dev/dl/) 1.26+

### Via Claude Code plugin

1. Inside Claude Code, add the plugin marketplace and install:

```
/plugin marketplace add fredrikaverpil/claudemeter
/plugin install claudemeter@claudemeter
```

2. Run `/claudemeter:setup` inside Claude Code
3. Restart Claude Code

### Manual

1. Install the binary:

```bash
go install github.com/fredrikaverpil/claudemeter@latest
```

2. Add the statusline to `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "claudemeter"
  }
}
```

3. Restart Claude Code

> [!NOTE]
> `$GOPATH/bin` must be in your `$PATH` for the `claudemeter` command to be
> found. If you're unsure, run `go env GOPATH` and add `$(go env GOPATH)/bin` to
> your shell profile.

## About

A Claude Code statusline plugin written in Go. It displays the current AI model,
subscription plan, context window usage, and 5-hour/7-day quota usage as
ANSI-colored progress bars. Zero external dependencies (stdlib only).

The binary reads JSON from stdin (provided by Claude Code) and writes a single
ANSI-colored line to stdout.

## Architecture

Single-file (`main.go`), single-package (`main`) design.

**Data flow:** stdin JSON → parse input + read credentials → fetch usage
(cached) → render ANSI output → stdout

Key components:

- **Credential resolution:** macOS Keychain (`security find-generic-password`)
  first, falls back to `~/.claude/.credentials.json`. Works on any platform
  via the file fallback. Failure is non-fatal (usage bars are omitted).
- **Usage API:** `GET https://api.anthropic.com/api/oauth/usage` with OAuth
  bearer token. 5-second HTTP timeout.
- **File-based cache:** `/tmp/claudemeter-usage.json` with 60s TTL on
  success, 15s TTL on failure.
- **Progress bars:** 5-char width using `█`/`░` with color thresholds
  (green/yellow/red for context; blue/magenta/red for quota).

## References

- [claude-hud](https://github.com/jarrodwatts/claude-hud) — inspiration for this project
- [Create Claude plugins](https://code.claude.com/docs/en/plugins)
- [Customize your status line](https://code.claude.com/docs/en/statusline)
- [Costs and context window](https://code.claude.com/docs/en/costs)
