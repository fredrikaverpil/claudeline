# claudeline

A minimalistic and opinionated Claude Code status line.

<img width="930" height="112" alt="claudeline_pro" src="https://github.com/user-attachments/assets/1c28cf80-c562-47fa-8ae4-2dda6bccd336" />

<img width="930" height="112" alt="claudeline" src="https://github.com/user-attachments/assets/51ab601c-760a-41da-8dca-d682bf7ad138" />

<img width="930" height="112" alt="image" src="https://github.com/user-attachments/assets/8728028b-8cbb-4113-af88-3bbf4e6ab23b" />

It displays the current Anthropic model, subscription plan, context window
usage, and 5-hour/7-day quota usage as ANSI-colored progress bars. Written in Go
with no external dependencies (stdlib only).

> [!NOTE]
>
> The 5-hour and 7-day quota bars require a Claude Code subscription (Pro, Max,
> or Team). They are not available for free tier, Enterprise or API key users.
> The bars may also disappear silently if the usage API is temporarily
> unavailable or rate limited — use `-debug` to diagnose.

## Installation

### Via Claude Code plugin (recommended)

1. Inside Claude Code, add the plugin marketplace and install:

```
/plugin marketplace add fredrikaverpil/claudeline
/plugin install claudeline@claudeline
```

2. Run `/claudeline:setup` inside Claude Code — this downloads the Go binary and
   configures your statusline
3. Restart Claude Code

### Manual

1. Download the latest binary from
   [GitHub releases](https://github.com/fredrikaverpil/claudeline/releases), or
   use `go install github.com/fredrikaverpil/claudeline@latest`.
2. Add the statusline to `~/.claude/settings.json`:

```jsonc
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/claudeline",
  },
}
```

> [!TIP]
>
> If installing via `go install`, set the command to `~/go/bin/claudeline`

3. Restart Claude Code

## Flags

| Flag                  | Default | Description                                          |
| --------------------- | ------- | ---------------------------------------------------- |
| `-debug`              | `false` | Write warnings/errors to `/tmp/claudeline-debug.log` |
| `-cwd`                | `false` | Show working directory name in the status line       |
| `-cwd-max-len`        | `30`    | Max display length for working directory name        |
| `-git-branch`         | `false` | Show git branch in the status line                   |
| `-git-branch-max-len` | `30`    | Max display length for git branch                    |
| `-version`            | `false` | Print version and exit                               |

Example with working directory and git branch enabled:

```json
{
  "statusLine": {
    "type": "command",
    "command": "claudeline -cwd -git-branch"
  }
}
```

## About

## Architecture

Single-file (`main.go`), single-package (`main`) design.

**Data flow:** stdin JSON → parse input + read credentials → fetch usage
(cached) + fetch status (cached) → render ANSI output → stdout

Key components:

- **Credential resolution:** macOS Keychain (`security find-generic-password`)
  first, falls back to `~/.claude/.credentials.json`. Works on any platform via
  the file fallback. Failure is non-fatal (usage bars are omitted).
- **Usage API:** `GET https://api.anthropic.com/api/oauth/usage` with OAuth
  bearer token. 5-second HTTP timeout.
- **File-based cache:** `/tmp/claudeline-usage.json` with 60s TTL on success,
  15s TTL on failure.
- **Context bar:** 5-char width using `█`/`░` with four color zones inspired by
  [Dax Horthy's "dumb zone" theory](https://www.youtube.com/watch?v=rmvDxxNubIg&t=493s)
  on context window quality degradation:
  - **Smart** (green, 0–40%) — model performs at full capability
  - **Dumb** (yellow, 41–60%) — quality starts to degrade
  - **Danger** (orange, 61–80%) — significant quality loss
  - **Near compaction** (red, 80%+) — approaching auto-compaction threshold
- **Quota bars:** 5-char width using `█`/`░` (blue/magenta/red) for 5-hour and
  7-day quotas. Per-model sub-bars (sonnet, opus, cowork, oauth) appended to the
  7-day bar with `·` sub-separator. Extra usage shown as `$used/$limit` (hidden
  when $0, red at 80%+ of limit).
- **Compaction warning:** A yellow `⚠` appears on the context bar when usage is
  within 5% of the auto-compaction threshold (85% by default, configurable via
  `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`).
- **Service status:** Fetches `https://status.claude.com/api/v2/status.json`
  (Atlassian Statuspage API, no auth required). Cached in
  `/tmp/claudeline-status.json` with 2min OK TTL, 30s fail TTL. Shows an orange
  fire icon with severity bars when there is a disruption: `🔥▂` (minor), `🔥▄▂`
  (major), `🔥▆▄▂` (critical). Hidden when all systems are operational.
- **Working directory:** Last path segment from `cwd` in stdin JSON, opt-in with
  `-cwd`.
- **Git info:** Branch name read from `.git/HEAD` (no subprocess), opt-in with
  `-git-branch`.
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

### Usage API

The quota bars use `GET https://api.anthropic.com/api/oauth/usage` with an
`Anthropic-Beta: oauth-2025-04-20` header. This endpoint is undocumented by
Anthropic and is not part of their public API. It was reverse-engineered from
Claude Code's own OAuth flow and is used by several third-party projects:

- [JetBrains ClaudeQuotaService](https://github.com/JetBrains/intellij-community/blob/master/plugins/agent-workbench/sessions/src/claude/ClaudeQuotaService.kt)
- [claude-hud usage-api.ts](https://github.com/jarrodwatts/claude-hud/blob/main/src/usage-api.ts)

Because the endpoint is in beta, it may change or break without notice.
