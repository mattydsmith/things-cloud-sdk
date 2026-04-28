# Installation Guide

Self-host the Things Cloud MCP server to connect Claude (or any MCP-compatible AI assistant) to your Things 3 task manager.

## Prerequisites

- A [Things 3](https://culturedcode.com/things/) account with Things Cloud sync enabled
- [Go 1.24+](https://go.dev/dl/) (for building from source)
- [Fly.io CLI](https://fly.io/docs/flyctl/install/) (`brew install flyctl`)
- A free [Fly.io account](https://fly.io)

## 1. Clone the repository

```bash
git clone https://github.com/mattydsmith/things-cloud-sdk.git
cd things-cloud-sdk
```

## 2. Build locally (optional, to verify)

```bash
go build -v ./...
go test -v ./...
```

Run the server locally to confirm everything works:

```bash
export THINGS_USERNAME='your-things-email'
export THINGS_PASSWORD='your-things-password'
mkdir -p /data
go run ./server/
```

The server starts at `http://localhost:8080`. Verify with:

```bash
curl http://localhost:8080/
# {"service":"things-cloud-api","status":"ok"}
```

## 3. Deploy to Fly.io

### Create the app

```bash
fly launch
```

When prompted, choose a region close to you. The included `fly.toml` configures a shared CPU VM with 1 GB RAM and a persistent volume for the SQLite database.

### Set your credentials

```bash
fly secrets set THINGS_USERNAME='your-things-email' THINGS_PASSWORD='your-things-password'
```

Optional: set an API key if you want to protect `/api/*` for scripts or direct REST clients:

```bash
fly secrets set API_KEY='your-chosen-api-key'
```

Important:

- Claude.ai web and Claude Code currently connect to `/mcp` with no auth
- `API_KEY` affects only `/api/*` in the current stable server
- `AUTH_SECRET` is part of the planned web UI work, not the current installation flow

### Deploy

```bash
fly deploy
```

The first deploy creates the persistent volume automatically. The container image is ~14 MB.

### Verify the deployment

```bash
curl https://your-app-name.fly.dev/
# {"service":"things-cloud-api","status":"ok"}
```

## 4. Connect to Claude

### Claude.ai (web)

1. Go to **Settings > Connectors > Add custom connector**
2. Set the URL to `https://your-app-name.fly.dev/mcp`
3. Save

Then ask Claude: *"What's on my Things today?"*

No auth field is required. The current stable MCP endpoint is open.

### Claude Code (CLI)

Add the server to your Claude Code MCP config (`~/.claude/mcp.json` or project-level):

```json
{
  "mcpServers": {
    "things": {
      "type": "url",
      "url": "https://your-app-name.fly.dev/mcp"
    }
  }
}
```

No headers are needed for Claude Code either, because `/mcp` is currently open. If you want to script against `/api/*`, use `API_KEY` there instead.

### Direct REST clients

If you do set `API_KEY`, send it on `/api/*` like this:

```bash
curl https://your-app-name.fly.dev/api/verify \
  -H "Authorization: Bearer your-api-key"
```

## 5. Available tools

Once connected, Claude has access to 36 tools:

**Read** — List tasks by view (today, inbox, anytime, someday, upcoming, all), by project, by area. Get individual tasks, areas, tags. List completed tasks with date filters. List checklist items.

**Write** — Create tasks, projects, areas, tags, headings, and checklist items. Edit tasks (title, notes, dates, project, area, tags, recurrence). Complete, uncomplete, trash, and restore tasks.

**Search** — Case-insensitive substring search across task titles and notes.

**Diagnostic** — Built-in smoke test that exercises the full create/read/edit/complete/trash cycle.

## Infrastructure notes

- The server scales to zero when idle and auto-starts on the first request (cold start takes a few seconds)
- SQLite with WAL mode is stored on a persistent Fly.io volume at `/data`
- The server syncs incrementally from Things Cloud before every read, so changes made in the Things app are immediately visible
- Writes use event-sourced sync with automatic retry on 409 conflicts (race with the Things app)

## Privacy and credentials

The server needs your Things Cloud email and password to sync your tasks. Since you're deploying this on your own Fly.io account, your credentials are stored as encrypted secrets on infrastructure you control — they're not shared with anyone.

Today the stable behavior is simple:

- `/mcp` is open for Claude.ai web and Claude Code
- `/api/*` is open unless you set `API_KEY`
- if you do set `API_KEY`, it protects only the REST API

## Cost

Fly.io doesn't bill you if your monthly usage is under $5. The server scales to zero when idle and uses minimal resources when running (shared CPU, 1 GB RAM). After 2+ months of active development and daily use, the author has not been billed.

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `THINGS_USERNAME` | Yes | Your Things account email |
| `THINGS_PASSWORD` | Yes | Your Things account password |
| `API_KEY` | No | Optional static bearer token for `/api/*`. It does not affect `/mcp` in the current stable server. |
| `PORT` | No | Server port (default: `8080`) |
| `DEBUG` | No | Set to `true` for verbose HTTP logging |

## Updating

```bash
git pull
fly deploy
```

## Migrating Existing Deployments

### If you previously tested MCP auth changes

If you temporarily added `AUTH_SECRET` while testing an auth-hardened branch, remove it. The current stable server does not use `AUTH_SECRET`:

```bash
fly secrets unset AUTH_SECRET -a your-app-name
```

If you want the current fully open prod/dev behavior for both `/mcp` and `/api/*`, remove `API_KEY` too:

```bash
fly secrets unset API_KEY -a your-app-name
```

If Claude still shows the old connector error after rollback, delete the connector and add it again with just the MCP URL.

### If you use REST scripts

You can keep using static bearer auth on `/api/*`:

- keep `API_KEY` set
- send `Authorization: Bearer <token>`
- leave your Claude connector unchanged, because `/mcp` stays open

## Troubleshooting

**Server won't start:** Check your credentials with `fly logs`. The most common issue is incorrect Things Cloud credentials.

**Claude connector says it can't reach the MCP server:** Use the plain MCP URL with no auth settings. If you previously edited the connector during auth testing, delete it and recreate it.

**Sync errors:** The server retries sync automatically. If issues persist, check `fly logs` for details. You can trigger a manual sync via `curl https://your-app-name.fly.dev/api/sync`; if `API_KEY` is enabled, include `Authorization: Bearer <API_KEY>`.

**Cold start latency:** The first request after idle may take a few seconds as Fly.io starts the machine and the server performs an initial sync. Subsequent requests are fast.
