# Setup claude-status-line

Configure the claude-status-line plugin as your Claude Code statusline.

## Steps

1. Build the Go binary (if not already built):

```bash
cd ~/code/public/claude-status-line && go build -o claude-status-line .
```

2. Read the current `~/.claude/settings.json` and update the `statusLine` field to:

```json
{
  "statusLine": {
    "type": "command",
    "command": "/Users/fredrik/code/public/claude-status-line/claude-status-line"
  }
}
```

3. Confirm the change was made and tell the user to restart their Claude Code session for the statusline to take effect.
