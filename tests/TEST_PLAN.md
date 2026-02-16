# Things Cloud API — Test Plan

## Overview

All tests live in the `tests/` directory and run against the live deployed service at `https://things-cloud-mttsmth.fly.dev`. All tests use real Things Cloud sync — there are no mocks. A 1-second delay between calls avoids Things Cloud 429 rate limiting.

## Test Scripts

| Script | Scope | Usage |
|--------|-------|-------|
| `test-smoke.sh` | Daily smoke test — core workflow | `./test-smoke.sh [base_url]` |
| `test-mcp.sh` | MCP write tools + read verification | `./test-mcp.sh [cycle] [base_url]` |
| `test-mcp-read.sh` | MCP read-only tools | `./test-mcp-read.sh [base_url]` |
| `test-mcp-protocol.sh` | MCP JSON-RPC protocol | `./test-mcp-protocol.sh [base_url]` |
| `test-api.sh` | REST API endpoints | `./test-api.sh [base_url] [api_key]` |

Each script exits with code 0 on success, 1 on any failure.

## Test Results Log

All scripts append results to `tests/test-results.log`. Each line records:

```
<timestamp>  <script>  cycle=<id>  <PASS|FAIL>  <N> passed, <N> failed
```

Failed checks are logged on indented lines below the summary. Example:

```
2026-02-16 14:30:00 UTC  test-mcp  cycle=009  PASS  23 passed, 0 failed
2026-02-16 15:00:00 UTC  test-mcp  cycle=010  FAIL  21 passed, 2 failed
    FAIL  trash ok (got: , want: trashed)
    FAIL  untrash ok (got: , want: restored)
```

---

## 1. Smoke Test (`test-smoke.sh`)

**Status: Implemented — 11/11 passing**

Lightweight daily test covering the core read/write workflow. Runs in ~15 seconds. Designed to catch Things Cloud API breaking changes early.

| # | Step | Tool | Checks |
|---|------|------|--------|
| 1 | Health | `GET /` | Service returns status=ok |
| 2 | Read today | `things_list_today` | Returns valid JSON array |
| 3 | Read projects | `things_list_projects` | Returns valid JSON array |
| 4 | Read tags | `things_list_tags` | Returns valid JSON array |
| 5 | Create task | `things_create_task` | UUID returned |
| 6 | Get task | `things_get_task` | Title and status=open match |
| 7 | Edit task | `things_edit_task` | status=updated |
| 8 | Complete task | `things_complete_task` | status=completed; re-get confirms |
| 9 | Trash task | `things_trash_task` | status=trashed (cleanup) |

Leaves no residual data — the test task is trashed at the end.

---

## 2. MCP Full Write Tools (`test-mcp.sh`)

**Status: Implemented — 23/23 passing**

Each cycle creates test entities with a `[test-NNN]` prefix, runs checks, then cleans up.

### Setup (create entities)

| # | Tool | Check | Notes |
|---|------|-------|-------|
| 1 | `things_create_tag` | UUID returned | Creates `[test-NNN] Tag` with shorthand |
| 2 | `things_create_area` | UUID returned | Creates `[test-NNN] Area` |
| 3 | `things_create_project` | UUID returned | In area, with note, when=anytime, deadline |
| 4 | `things_create_task` | UUID returned | In project, with tag, note, when=today, deadline |

### Get (verify created entities)

| # | Tool | Check |
|---|------|-------|
| 5 | `things_get_task` | Title, status=open, project_id match |
| 6 | `things_get_area` | Title matches |
| 7 | `things_get_tag` | Title matches |

### Modify

| # | Tool | Check |
|---|------|-------|
| 8 | `things_edit_task` | Response status=updated; re-get confirms new title |

### Move

| # | Tool | Check |
|---|------|-------|
| 9 | `things_move_to_someday` | status=moved_to_someday |
| 10 | `things_move_to_anytime` | status=moved_to_anytime |
| 11 | `things_move_to_inbox` | status=moved_to_inbox |
| 12 | `things_move_to_today` | status=moved_to_today |

### Complete / Uncomplete

| # | Tool | Check |
|---|------|-------|
| 13 | `things_complete_task` | Response status=completed; re-get confirms status=completed |
| 14 | `things_uncomplete_task` | Response status=uncompleted; re-get confirms status=open |

### Trash / Untrash

| # | Tool | Check |
|---|------|-------|
| 15 | `things_trash_task` | status=trashed |
| 16 | `things_untrash_task` | status=restored |

### List verification

| # | Tool | Check |
|---|------|-------|
| 17 | `things_list_today` | Created task UUID appears in results |
| 18 | `things_list_project_tasks` | Created task UUID appears in results |

### Cleanup

- Trashes the test task and project
- Areas and tags cannot be trashed via API (logged as note)

---

## 3. MCP Read Tools (`test-mcp-read.sh`)

**Status: Implemented — 29/29 passing**

These tests verify all read-only MCP tools return valid responses. They don't create test data — they rely on existing Things data (plus anything left from a write test cycle).

### Basic list tools (return JSON arrays)

| # | Tool | Checks |
|---|------|--------|
| 1 | `things_list_today` | Returns array; each item has uuid, title, status fields |
| 2 | `things_list_inbox` | Returns array; items have uuid, title, status |
| 3 | `things_list_all_tasks` | Returns non-empty array (account has tasks) |
| 4 | `things_list_projects` | Returns array; items have uuid, title |
| 5 | `things_list_areas` | Returns array; items have uuid, title |
| 6 | `things_list_tags` | Returns array; items have uuid, title |

### Parameterised list tools

| # | Tool | Checks |
|---|------|--------|
| 7 | `things_list_project_tasks` | Pick first project UUID from #4; returns array of tasks |
| 8 | `things_list_area_tasks` | Pick first area UUID from #5; returns array of tasks |
| 9 | `things_list_checklist_items` | Find a task with checklists (or skip); returns array |

### Get tools (return single objects)

| # | Tool | Checks |
|---|------|--------|
| 10 | `things_get_task` | Pick first task UUID from #1 or #3; returns object with uuid, title, status |
| 11 | `things_get_area` | Pick first area UUID from #5; returns object with uuid, title |
| 12 | `things_get_tag` | Pick first tag UUID from #6; returns object with uuid, title |

### Error handling

| # | Tool | Input | Expected |
|---|------|-------|----------|
| 13 | `things_get_task` | uuid=`nonexistent-uuid` | MCP error or empty result |
| 14 | `things_list_project_tasks` | project_uuid=`nonexistent-uuid` | Empty array or error |
| 15 | `things_list_checklist_items` | task_uuid=`nonexistent-uuid` | Empty array or error |

---

## 4. REST API Endpoints (`test-api.sh`)

**Status: Implemented — 18/18 passing**

Tests the `/api/*` endpoints which require Bearer token auth. Pass API key as second arg or via `API_KEY` env var.

### Health & Auth

| # | Endpoint | Method | Check |
|---|----------|--------|-------|
| 1 | `GET /` | — | Returns `{"service":"things-cloud-api","status":"ok"}` |
| 2 | `GET /api/verify` | No auth | Returns 401 |
| 3 | `GET /api/verify` | With auth | Returns 200 with account info |

### Read endpoints

| # | Endpoint | Check |
|---|----------|-------|
| 4 | `GET /api/tasks/today` | Returns JSON array of tasks |
| 5 | `GET /api/tasks/inbox` | Returns JSON array of tasks |
| 6 | `GET /api/projects` | Returns JSON array of projects |
| 7 | `GET /api/areas` | Returns JSON array of areas |
| 8 | `GET /api/tags` | Returns JSON array of tags |
| 9 | `GET /api/sync` | Returns `{"changes_count": N}` |

### Write endpoints

| # | Endpoint | Body | Check |
|---|----------|------|-------|
| 10 | `POST /api/tasks/create` | `{"title":"[api-test] Task"}` | Returns uuid, status=created |
| 11 | `POST /api/tasks/edit` | `{"uuid":"...","title":"..."}` | Returns status=updated |
| 12 | `POST /api/tasks/complete` | `{"uuid":"..."}` | Returns status=completed |
| 13 | `POST /api/tasks/trash` | `{"uuid":"..."}` | Returns status=trashed |

### Write endpoint validation

| # | Endpoint | Body | Expected |
|---|----------|------|----------|
| 14 | `POST /api/tasks/create` | `{}` (no title) | 400 "title is required" |
| 15 | `POST /api/tasks/complete` | `{}` (no uuid) | 400 "uuid is required" |
| 16 | `GET /api/tasks/create` | Wrong method | 405 "method not allowed" |

---

## 5. MCP Protocol (`test-mcp-protocol.sh`)

**Status: Implemented — 11/11 passing**

Tests the JSON-RPC protocol layer independently from tool logic.

### Initialize handshake

| # | Check |
|---|-------|
| 1 | Server name is "Things Cloud" |
| 2 | Server version is "1.0.0" |
| 3 | Capabilities include tools |

### List tools

| # | Check |
|---|-------|
| 4 | Returns 27 tools |
| 5 | Includes `things_create_task` |
| 6 | Includes `things_list_today` |
| 7 | Includes `things_create_heading` |
| 8 | All tools have `inputSchema` |

### Error handling

| # | Check |
|---|-------|
| 9 | Unknown tool name returns error |
| 10 | Missing required `title` param returns error |
| 11 | Missing required `uuid` param returns error |

---

## Known Limitations

- **Areas and tags can't be deleted** — the Things Cloud API has no trash/delete endpoint for these. Test cycles leave orphaned `[test-NNN] Area` and `[test-NNN] Tag` entries.
- **Rate limiting** — Things Cloud returns HTTP 429 under rapid writes. All test scripts must include a delay (currently 1s) between MCP calls.
- **No mocked tests** — all tests hit the live service. They require valid `THINGS_USERNAME`/`THINGS_PASSWORD` env vars on the server and a running deployment.
- **Checklist items** — no write tool exists for creating checklist items, so `things_list_checklist_items` can only be tested if the account already has tasks with checklists.
