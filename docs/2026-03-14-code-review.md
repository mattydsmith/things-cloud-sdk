# Code Review — 2026-03-14

Full codebase review of `things-cloud-sdk`: core SDK, sync engine, MCP server, state packages, and CLI tools.

**Test status at review time**: All tests pass. `go vet` clean.

**Status note**: The first two transport-layer findings below were fixed on `main` in the same change set that added this review file. They are kept here because they were valid review findings, but they are now resolved.

---

## High Priority

### 1. Error checked after use — potential nil panics (resolved on `main`)

Several places called `http.NewRequest` but used the returned `req` before checking the error. If `NewRequest` failed, `req` would be nil and the subsequent `req.URL.Query()` or `req.Header.Set()` calls would panic.

**Original locations:**
- `items.go:40-46` — `Items()` called `req.URL.Query()` before checking `err`
- `histories.go:260-273` — `Write()` called `req.Header.Add` before checking `err`
- `verify.go:30-32` — `Verify()` called `req.Header.Set` before checking `err`

**Fix:** Move the `if err != nil` check immediately after `http.NewRequest`.

### 2. Swallowed JSON unmarshal errors (resolved on `main`)

Multiple places ignored the error from `json.Unmarshal`, silently proceeding with zero-value structs. If the server returned unexpected JSON (API change, error body, truncated response), these would produce wrong results with no indication of failure.

**Original locations:**
- `histories.go:56` — `Sync()` ignored unmarshal error
- `histories.go:175` — `Histories()` ignored unmarshal error
- `histories.go:215` — `CreateHistory()` ignored unmarshal error
- `verify.go:51` — `Verify()` ignored unmarshal error
- `histories.go:291` — `Write()` ignored unmarshal error

**Fix:** Check and return all unmarshal errors.

### 3. N+1 query pattern in sync state queries

`sync/state.go:259-276` — `scanTaskUUIDs` fetches one task at a time via `getTask()`, which itself runs a second query for tags. For a Today view with 30 tasks, that's 60 database queries. For `AllTasks` with hundreds, this becomes a real bottleneck.

**Fix:** Add a batch `getTasks(uuids []string)` method that joins tasks + tags in one or two queries, or a `scanFullTasks` that reads all columns directly from the listing query.

---

## Medium Priority

### 4. Time-dependent change detection is non-deterministic

`sync/detect.go:138-161` — `taskLocation()` and `isToday()` call `time.Now()` directly. This means:
- The same sync data can produce different change events depending on when it runs
- These functions are difficult to unit test reliably (tests that run near midnight can flake)

**Fix:** Pass a `now time.Time` parameter through `detectTaskChanges` -> `taskLocation` -> `isToday`/`isFutureAt`.

### 5. Global mutable state in server package

`server/write.go` relies on package-level `historyMu`, `syncer`, `history`, and `client` variables. This makes the server code:
- Awkward to integration-test at the handler level with isolated dependencies
- Unsafe to run multiple instances in-process
- Harder to reason about concurrency

**Fix:** Bundle these into a struct (e.g. `ThingsServer`) and pass it to handlers. This would also enable cleaner integration testing with mock clients.

### 6. Recursive `hasArea` has no cycle protection

`state/memory/memory.go:339-355` — `hasArea()` recurses through `ParentTaskIDs` with no visited set. If task data ever contains a cycle (data corruption, import bug), this causes a stack overflow.

**Fix:** Add a `visited map[string]bool` parameter, or limit recursion depth.

### 7. Hardcoded Host header is misleading and likely unnecessary

`client.go:107` sets `req.Header.Set("Host", "cloud.culturedcode.com")` on every request. In Go, that does not override the destination selected from `c.Endpoint` unless `req.Host` is set, so the current code does not actually redirect requests away from a test server. The problem is that the header assignment is misleading, production-specific, and may not do what the code appears to intend.

**Fix:** Remove the header assignment, only set it when talking to the production endpoint, or set `req.Host` explicitly if a custom Host value is genuinely required.

---

## Low Priority

### 8. Stale / incorrect comments

- `types.go:217` — Comment says `"hm, not sure what tir stands for"`. This is now well-understood as "today index reference date" (documented in `CLAUDE.md`). Update the comment.
- `types.go:348` — Godoc for `TagActionItemPayload` says "payload for modifying Areas" — should say "Tags".

**Fix:** Update both comments.

### 9. `lastDayOfMonth` returns 23:00 instead of midnight

`repeat.go:206-208` — Subtracts one hour from the first of the next month. This gives `{last_day}T23:00:00` rather than `{last_day}T00:00:00`. Works for date-only comparisons but is technically wrong for anything that uses the time component.

**Fix:** Use `.AddDate(0, 1, -1)` on the first of the month instead, or subtract `time.Nanosecond` if end-of-day is intended.

### 10. Duplicate title assignment

`state/memory/memory.go:118` — Sets `t.Title` a second time (already set at line 43). Harmless but redundant.

**Fix:** Remove the duplicate on line 118.

### 11. Missing test coverage for CLI tools

14 CLI tools under `cmd/` have no test files. While these are mostly thin wrappers, the larger ones (`things-cli` at 1478 lines, `thingsync` at 973 lines) contain non-trivial logic that could benefit from basic smoke tests.

**Fix:** Add tests for core parsing/formatting logic in the larger CLI tools, or extract shared logic into testable packages.

---

## What's Working Well

- **Event-sourcing model** is clean with proper separation (items -> state -> changes)
- **Change detection** is thorough — 40+ semantic change types with compile-time interface checks
- **Transaction batching** in `processItems` gives good sync performance
- **Nullable date handling** via `Has*Date()` sentinel pattern is well thought out
- **MCP tool descriptions** are clear and precise (good LLM-facing documentation)
- **Test infrastructure** with tape-based HTTP mocking is solid
- **Base58 UUID generation** correctly matches Things' format
- **Retry logic** with exponential backoff handles transient server errors gracefully
