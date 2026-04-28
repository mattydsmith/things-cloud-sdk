# Web UI Design Feedback

Review of the design spec, implementation plan, and HTML mockups (2026-03-28).

## 1. Sidebar: inconsistent area links

In the mockups, "Work" is an `<a>` linking to `area.html` but "Personal" is a `<span>` with no link. Both should be clickable links to their area detail view.

**Files:** all mockups with sidebar (`today.html`, `inbox.html`, `anytime.html`, `someday.html`, `upcoming.html`, `project.html`)

## 2. Login page label says "API Key"

The login mockup uses `<label>API Key</label>` and `placeholder="Enter your API key"`. The implementation plan calls this `AUTH_SECRET` and describes it to users as "any password you choose." The label should say "Password" (or similar) to match the deployment guide and avoid confusing non-technical users.

**File:** `mockups/login.html`

## 3. Inbox triage: add "Anytime" button

Inbox task rows show "Today" and "Someday" quick-action buttons. Someday rows show "Today" and "Anytime". Triaging from Inbox to Anytime is arguably more common than Inbox-to-Someday — add an "Anytime" button to the Inbox row actions.

**File:** `mockups/inbox.html`, `IMPLEMENTATION.md` (scope table)

## 4. New API endpoints need request/response details

The API gap table lists new endpoints but doesn't specify request bodies or query parameters:

- `POST /api/tasks/move-to-today` — body is presumably `{"uuid": "..."}`?
- `POST /api/tasks/move-to-anytime` — same question
- `GET /api/tasks/search` — what query params? `?q=...`?
- `/api/tasks/:uuid/checklist` — GET only, or also POST/PATCH/DELETE for add/complete/delete?

These details are needed for Phase 2 when building the frontend API client.

**File:** `IMPLEMENTATION.md` (API Gaps to Fill section)

## 5. Shared CSS across mockups

Each HTML mockup duplicates ~300 lines of identical sidebar and shared component CSS. Fine for now, but if iterating on the mockups, extracting into a `shared.css` would reduce churn.

**Files:** `mockups/*.html`

## 6. Command palette (`Cmd+K`) in spec but not in plan

The design spec's keyboard shortcuts table lists `Cmd+K` for a command palette. This feature isn't mentioned in the implementation plan's scope or phases. Either remove it from the spec or add it to the plan (probably Phase 5 or post-v1).

**Files:** `2026-03-28-web-ui-design-spec.md` (Keyboard Shortcuts), `IMPLEMENTATION.md` (Scope / Phases)

