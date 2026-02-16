# Things Cloud

A Go server and SDK for the [Things 3](https://culturedcode.com/things/) cloud API. Exposes your Things tasks via REST and [MCP](https://modelcontextprotocol.io/) (Model Context Protocol), connecting Claude and other AI assistants directly to your task manager.

Built on a reverse-engineered, unofficial SDK — there is no official API documentation from Cultured Code.

## Architecture

```
┌──────────────┐     ┌──────────────────┐     ┌──────────────┐
│  Things App  │────▶│  Things Cloud    │◀────│  API Server  │
│  (Mac/iOS)   │     │  (Cultured Code) │     │  (Fly.io)    │
└──────────────┘     └──────────────────┘     └──────┬───────┘
                                                     │
                                              ┌──────┴───────┐
                                              │              │
                                         /api/*         /mcp
                                        REST API    MCP Endpoint
                                              │              │
                                         curl/apps    Claude.ai
                                                     Connector
```

The server syncs task data from Things Cloud into a local SQLite database and exposes it through two interfaces:

- **REST API** (`/api/*`) — Bearer token auth, for scripts and apps
- **MCP Endpoint** (`/mcp`) — Streamable HTTP (JSON-RPC 2.0), for AI assistants like Claude

### Sync model

The server maintains two sync channels with Things Cloud:

1. **Read sync** (`sync.Syncer`) — SQLite-backed incremental sync that pulls task state. Called before every read to ensure fresh data.

2. **Write sync** (`things.History`) — Event-sourced write channel using Things Cloud's history API. Each write syncs the history first to get the latest ancestor index, then commits. Retries once on 409 conflict (race with the Things app).

You can freely make changes in the Things app and immediately use the API/MCP tools — no server restart required.

### Key files

| Path | Purpose |
|------|---------|
| `server/main.go` | HTTP server, routing, auth middleware |
| `server/mcp.go` | MCP server with 26 tool definitions and handlers |
| `server/write.go` | Write operations shared between REST and MCP |
| `sync/` | Persistent SQLite sync engine with semantic change detection |

## Infrastructure

The server runs on [Fly.io](https://fly.io):

| Component | Detail |
|-----------|--------|
| **Region** | `lhr` (London) |
| **VM** | Shared CPU, 1 GB RAM |
| **Storage** | Persistent volume at `/data` (SQLite WAL mode) |
| **Container** | Multi-stage Alpine build, ~14 MB image |
| **Auto-scaling** | Scales to zero when idle, auto-starts on request |
| **URL** | `https://things-cloud-mttsmth.fly.dev` |

### Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `THINGS_USERNAME` | Yes | Things account email |
| `THINGS_PASSWORD` | Yes | Things account password |
| `API_KEY` | No | Bearer token for `/api/*` endpoints. If unset, no auth. |
| `PORT` | No | Server port (default: `8080`) |

## MCP Endpoint

The `/mcp` endpoint implements the [Model Context Protocol](https://modelcontextprotocol.io/) using Streamable HTTP transport (JSON-RPC 2.0 over HTTP POST). No authentication — designed for use with Claude.ai custom connectors.

### Tools (27)

#### Read tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `things_list_today` | Tasks scheduled for today | — |
| `things_list_inbox` | Tasks in the inbox | — |
| `things_list_all_tasks` | All open tasks | — |
| `things_list_projects` | All projects | — |
| `things_list_areas` | All areas | — |
| `things_list_tags` | All tags | — |
| `things_list_project_tasks` | Tasks in a project | `project_uuid` |
| `things_list_area_tasks` | Tasks in an area | `area_uuid` |
| `things_list_checklist_items` | Checklist items for a task | `task_uuid` |
| `things_get_task` | Get a single task | `uuid` |
| `things_get_area` | Get a single area | `uuid` |
| `things_get_tag` | Get a single tag | `uuid` |

#### Write tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `things_create_task` | Create a task | `title` (req), `note`, `when`, `deadline`, `project`, `tags` |
| `things_create_project` | Create a project | `title` (req), `note`, `when`, `deadline`, `area` |
| `things_create_heading` | Create a heading in a project | `title` (req), `project` |
| `things_create_area` | Create an area | `title` (req), `tags` |
| `things_create_tag` | Create a tag | `title` (req), `shorthand`, `parent` |
| `things_edit_task` | Edit a task | `uuid` (req), `title`, `note`, `when`, `deadline`, `project`, `tags` |
| `things_complete_task` | Complete a task | `uuid` |
| `things_uncomplete_task` | Reopen a completed task | `uuid` |
| `things_trash_task` | Move to trash | `uuid` |
| `things_untrash_task` | Restore from trash | `uuid` |
| `things_move_to_today` | Schedule for today | `uuid` |
| `things_move_to_anytime` | Move to anytime | `uuid` |
| `things_move_to_someday` | Move to someday | `uuid` |
| `things_move_to_inbox` | Move to inbox | `uuid` |

#### Diagnostic tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `things_smoke_test` | Run a smoke test: create, read, edit, complete, trash | — |

### Claude.ai setup

1. Go to **Settings > Connectors > Add custom connector**
2. Set the URL to `https://things-cloud-mttsmth.fly.dev/mcp`
3. No OAuth configuration needed — leave auth fields empty

Then ask Claude: *"What's on my Things today?"* or *"Add a task to buy milk"*.

## REST API

All `/api/*` endpoints require `Authorization: Bearer <API_KEY>` when `API_KEY` is set.

### Read endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Health check — `{"service":"things-cloud-api","status":"ok"}` |
| `GET /api/verify` | Verify Things Cloud credentials |
| `GET /api/sync` | Trigger sync, returns change count |
| `GET /api/tasks/today` | Tasks scheduled for today |
| `GET /api/tasks/inbox` | Tasks in the inbox |
| `GET /api/projects` | All projects |
| `GET /api/areas` | All areas |
| `GET /api/tags` | All tags |

### Write endpoints

| Endpoint | Body | Description |
|----------|------|-------------|
| `POST /api/tasks/create` | `{"title":"...","note":"...","when":"today","deadline":"2025-12-31","project":"uuid","tags":"uuid1,uuid2"}` | Create a task |
| `POST /api/tasks/edit` | `{"uuid":"...","title":"...","note":"...","when":"today"}` | Edit a task |
| `POST /api/tasks/complete` | `{"uuid":"..."}` | Complete a task |
| `POST /api/tasks/trash` | `{"uuid":"..."}` | Trash a task |

## Testing

92 integration tests across 5 test suites, all running against the live deployment.

```bash
# Daily smoke test (11 checks, ~15s) — core read/write workflow
./tests/test-smoke.sh

# Full MCP write tools (23 checks, ~45s) — all write operations end-to-end
./tests/test-mcp.sh 010

# MCP read tools (29 checks, ~30s) — all read-only tools
./tests/test-mcp-read.sh

# MCP protocol (11 checks, ~10s) — JSON-RPC handshake and error handling
./tests/test-mcp-protocol.sh

# REST API (18 checks, ~20s) — all /api/* endpoints with auth
API_KEY=your-key ./tests/test-api.sh
```

Results are appended to `tests/test-results.log`:

```
2026-02-16 21:03:40 UTC  test-mcp    cycle=009  PASS  23 passed, 0 failed
2026-02-16 21:10:23 UTC  test-smoke             PASS  11 passed, 0 failed
2026-02-16 21:16:47 UTC  test-mcp-protocol      PASS  11 passed, 0 failed
2026-02-16 21:17:00 UTC  test-mcp-read          PASS  29 passed, 0 failed
2026-02-16 21:17:14 UTC  test-api               PASS  18 passed, 0 failed
```

See [`tests/TEST_PLAN.md`](tests/TEST_PLAN.md) for check-by-check detail.

## Deployment

```bash
# First time
fly launch

# Subsequent deploys
fly deploy

# Set secrets
fly secrets set THINGS_USERNAME='...' THINGS_PASSWORD='...' API_KEY='...'

# View logs
fly logs
```

## SDK

The underlying Go SDK can also be used directly as a library. See [`docs/sdk.md`](docs/sdk.md) for the full SDK documentation including:

- Getting started and quick start guide
- CLI tool (`things-cli`) with create, edit, complete, trash, batch commands
- Working with histories and items
- Persistent sync engine with 40+ semantic change types
- State queries (inbox, today, projects, areas, tags)
- Wire format notes from reverse engineering

## Local development

```bash
# Build the server
go build -v -o things-server ./server/

# Run locally
export THINGS_USERNAME='...' THINGS_PASSWORD='...'
mkdir -p /data
./things-server

# Run SDK tests
go test -v ./...
```
