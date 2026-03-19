# Setup claudeline

Install and configure the claudeline plugin as your Claude Code statusline.

Re-running this command will update claudeline to the latest version.

## Steps

1. **Check if claudeline is already installed.**

Check if a `claudeline` binary already exists on the system:

```bash
which claudeline
```

If found, note the path — the update should replace the binary at that location.

If not found, check these paths:

- `~/.local/bin/claudeline`
- `~/go/bin/claudeline`

2. **Download the latest pre-built binary from GitHub releases.**

Detect OS and architecture:

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
esac
```

Download and install to the target directory (the directory from step 1):

```bash
mkdir -p <target_directory>
curl -fsSL "https://github.com/fredrikaverpil/claudeline/releases/latest/download/claudeline_${OS}_${ARCH}.tar.gz" | tar -xz -C <target_directory> claudeline
chmod +x <target_directory>/claudeline
```

If the download fails (e.g. `curl` is not available, no internet, or the
platform is unsupported), fall back to `go install`:

```bash
go install github.com/fredrikaverpil/claudeline@latest
```

This requires Go to be installed with `$GOPATH/bin` in `$PATH`.

3. **Verify the binary is available.**

```bash
<target_directory>/claudeline -version
```

4. **Configure `settings.json` if not already set.**

Determine the settings file path: use `$CLAUDE_CONFIG_DIR/settings.json` if the
`CLAUDE_CONFIG_DIR` environment variable is set, otherwise
`~/.claude/settings.json`.

Read the settings file. If `statusLine` is already configured and its `command`
contains `claudeline`, **skip this step** — the existing configuration is valid.

Otherwise, set the `statusLine` field. If the target directory is in the user's
`$PATH`, use just the binary name:

```json
{
  "statusLine": {
    "type": "command",
    "command": "claudeline"
  }
}
```

Otherwise, use the full path:

```json
{
  "statusLine": {
    "type": "command",
    "command": "<target_directory>/claudeline"
  }
}
```

Preserve all other fields in the file.

5. **Confirm** the change was made and tell the user to restart their Claude
   Code session for the statusline to take effect.
