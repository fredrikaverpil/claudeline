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
| `-debug`              | `false` | Write warnings/errors to `/tmp/claudeline/debug.log` |
| `-cwd`                | `false` | Show working directory name in the status line       |
| `-cwd-max-len`        | `30`    | Max display length for working directory name        |
| `-git-branch`         | `false` | Show git branch in the status line                   |
| `-git-branch-max-len` | `30`    | Max display length for git branch                    |
| `-usage-file`         |         | Read usage data from file instead of API             |
| `-status-file`        |         | Read status data from file instead of API            |
| `-update-file`        |         | Read update data from file instead of API            |
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

## Architecture

Single-binary design with `main.go` orchestrating `internal/` packages.

**Data flow:** stdin JSON → parse input + read credentials → fetch usage
(cached) + fetch status (cached) + check update (cached) → render ANSI output →
stdout

Key components:

- **Credential resolution:** macOS Keychain (`security find-generic-password`)
  first, falls back to `~/.claude/.credentials.json`. Works on any platform via
  the file fallback. Failure is non-fatal (usage bars are omitted).
- **Usage API:** `GET https://api.anthropic.com/api/oauth/usage` with OAuth
  bearer token. 5-second HTTP timeout.
- **File-based cache:** `/tmp/claudeline/usage.json` with 60s TTL on success,
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
  when $0, red at 80%+ of limit). A `⚡️` prefix appears on the 5-hour bar during
  peak hours (weekdays 13:00–19:00 UTC) for Pro and Max plans, when the 5-hour
  session limit
  [burns faster than normal](https://xcancel.com/trq212/status/2037254607001559305#m).
- **Compaction warning:** A yellow `⚠️` appears on the context bar when usage is
  within 5% of the auto-compaction threshold (85% by default, configurable via
  `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`).
- **Extended context indicator:** A `🥵` appears on the context bar when
  `exceeds_200k_tokens` is true, signaling the session has entered extended
  context territory where model quality may degrade.
- **Update check:** Fetches
  `https://api.github.com/repos/fredrikaverpil/claudeline/releases/latest`
  (GitHub API, no auth required). Release tag is cached in
  `/tmp/claudeline/update.json` with 24h OK TTL, 15s fail TTL. Shows a green `↑`
  indicator when a newer version is available. Hidden when already on the latest
  version or when the version cannot be determined (e.g. `(devel)`,
  `(unknown)`).
- **Service status:** Fetches `https://status.claude.com/api/v2/status.json`
  (Atlassian Statuspage API, no auth required). Cached in
  `/tmp/claudeline/status.json` with 2min OK TTL, 30s fail TTL. Shows an orange
  fire icon with severity bars when there is a disruption: `🔥▂` (minor), `🔥▄▂`
  (major), `🔥▆▄▂` (critical). Hidden when all systems are operational.
- **Working directory:** Last path segment from `cwd` in stdin JSON, opt-in with
  `-cwd`.
- **Git info:** Branch name read from `.git/HEAD` (no subprocess), opt-in with
  `-git-branch`.
- **Custom .claude folder**: Support `CLAUDE_CONFIG_DIR`.
- **Debug mode:** Pass `-debug` to write warnings and errors to
  `/tmp/claudeline/debug.log`. Set the statusline command to
  `claudeline -debug`, then `tail -f /tmp/claudeline/debug.log` in another
  terminal.

## Development

This project uses [Pocket](https://github.com/fredrikaverpil/pocket), a
Makefile-like task runner. Run `./pok` to execute linting, formatting, and
tests.

### Capturing and rendering

Claudeline can be tested fully offline using a two-step workflow:

1. **Capture** (requires credentials + network): Start Claude Code with
   `claudeline -debug`, make sure claudeline shows up. In a new terminal, run
   `./pok capture` (or `./pok capture -config-dir ~/.claude-work` for a custom
   profile). This reads credentials, calls the usage and status APIs, and writes
   the responses to testdata files under `internal/stdin/testdata/`,
   `internal/creds/testdata/`, `internal/usage/testdata/`,
   `internal/status/testdata/`, and `internal/update/testdata/`.
2. **Render** (100% offline): Run `./pok render` to render the statusline from
   the captured files. No credentials or network access needed. Use
   `./pok render -json internal/stdin/testdata/stdin_pro_opus.json` to render a
   specific stdin payload.

### Stdin payload schema

The `internal/stdin/testdata/stdin_*.json` files are named as
`stdin_<plan>_<model>.json` (version is stored in the JSON payload itself). A
comprehensive `payload` struct in `internal/stdin/stdin_test.go` maps every
known field. The `TestPayloadSchema` test uses `DisallowUnknownFields` to detect
when Claude Code adds new fields — if the test fails, update the `payload`
struct and re-run `./pok capture` to refresh the testdata.

### Credentials schema

The `internal/creds/testdata/creds_*.json` files are sanitized snapshots of
Claude Code's OAuth credentials (tokens replaced with `"sanitized"`). A
`credentials` struct in `internal/creds/creds_test.go` maps every known field.
The `TestCredentialsSchema` test uses `DisallowUnknownFields` to detect schema
changes — if the test fails, update the `credentials` struct and re-run
`./pok capture` to refresh the testdata.

### Usage API schema

The `internal/usage/testdata/usage_*.json` files are snapshots of the OAuth
usage API response (`api.anthropic.com/api/oauth/usage`), named as
`usage_<plan>.json`. A `usageResponse` struct in `internal/usage/usage_test.go`
maps every known field. The `TestUsageResponseSchema` test uses
`DisallowUnknownFields` to detect schema changes — if the test fails, update the
`usageResponse` struct and re-run `./pok capture` to refresh the testdata.

### Status API schema

The `internal/status/testdata/status.json` file is a snapshot of the Atlassian
Statuspage API response (`status.claude.com/api/v2/status.json`). A
`statusResponse` struct in `internal/status/status_test.go` maps every known
field. The `TestStatusResponseSchema` test uses `DisallowUnknownFields` to
detect schema changes — if the test fails, update the `statusResponse` struct
and re-run `./pok capture` to refresh the testdata.

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
