# Pi Web

Use this skill when the user wants to view, browse, or inspect their pi coding agent sessions in a web browser.

## When to use

- The user asks to "see my sessions", "browse sessions", "view sessions", "open session viewer", or anything about looking at past pi conversations.
- The user wants a GUI or web interface for their session history.

## How to use

Run the `pi-web` command:

```bash
# Start the viewer and auto-open browser
pi-web -o

# Or start on a specific port (default is 31415)
pi-web -p 31415 -o
```

## What it does

- Scans `~/.pi/agent/sessions/` recursively
- Serves a dark-themed web UI listing all sessions grouped by project
- Shows: message count, token usage, cost, timestamps
- Click any session to view the full conversation (user messages, assistant responses, tool calls, bash output, model changes, compactions, branch summaries)

## Options

| Flag | Description |
|------|-------------|
| `-p <port>` | Port to listen on (default: **31415**) |
| `-o` | Auto-open browser |

The binary is installed at `~/.pi/agent/bin/pi-web`.
