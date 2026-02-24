# Things MCP — API Guide & Quirks

Reference for Claude when using the Things MCP tools. This is the authoritative guide for the `things_*` tool family.

## Quick Reference — All 38 Tools

### Reading

| Tool | What it does | Key params |
|------|-------------|------------|
| `things_list_today` | Tasks in Today view | — |
| `things_list_inbox` | Tasks in Inbox | — |
| `things_list_anytime` | Triaged tasks with no date | — |
| `things_list_someday` | Deferred tasks | — |
| `things_list_upcoming` | Tasks with future scheduled dates | — |
| `things_list_all_tasks` | All open tasks | — |
| `things_list_projects` | All projects | — |
| `things_list_areas` | All areas | — |
| `things_list_tags` | All tags | — |
| `things_list_project_tasks` | Tasks inside a project | `project_uuid` |
| `things_list_area_tasks` | Tasks assigned to an area | `area_uuid` |
| `things_list_completed` | Recently completed | `limit` (default 50) |
| `things_list_checklist_items` | Checklist items in a task | `task_uuid` |
| `things_get_task` | Single task by UUID | `uuid` |
| `things_get_area` | Single area by UUID | `uuid` |
| `things_get_tag` | Single tag by UUID | `uuid` |
| `things_search_tasks` | Search titles + notes | `query` |

### Creating

| Tool | What it does | Required | Optional |
|------|-------------|----------|----------|
| `things_create_task` | New task | `title` | `note`, `when`, `deadline`, `reminder`, `project`, `parent_task`, `tags`, `repeat` |
| `things_create_project` | New project | `title` | `note`, `when`, `deadline`, `area` |
| `things_create_heading` | Heading in a project | `title` | `project` |
| `things_create_area` | New area | `title` | `tags` |
| `things_create_tag` | New tag | `title` | `shorthand`, `parent` |
| `things_create_checklist_item` | Checkbox in a task | `title`, `task_uuid` | — |

### Editing

| Tool | What it does | Required | Optional |
|------|-------------|----------|----------|
| `things_edit_task` | Edit a task | `uuid` | `title`, `note`, `when`, `deadline`, `reminder`, `project`, `parent_task`, `heading`, `area`, `tags`, `repeat`, `index`, `today_index` |
| `things_edit_area` | Rename area / change tags | `uuid` | `title`, `tags` |
| `things_edit_tag` | Rename / reparent tag | `uuid` | `title`, `shorthand`, `parent` |
| `things_edit_checklist_item` | Rename checkbox | `uuid`, `title` | — |

### Status Changes

| Tool | What it does |
|------|-------------|
| `things_complete_task` | Mark done |
| `things_cancel_task` | Mark abandoned (different from complete) |
| `things_uncomplete_task` | Reopen completed or canceled task |
| `things_trash_task` | Move to trash |
| `things_untrash_task` | Restore from trash |
| `things_complete_checklist_item` | Check off a checkbox |
| `things_uncomplete_checklist_item` | Uncheck a checkbox |
| `things_delete_checklist_item` | Remove a checkbox entirely |

### Moving & Organising

| Tool | What it does |
|------|-------------|
| `things_move_to_today` | Schedule for today |
| `things_move_to_anytime` | Triaged, no date |
| `things_move_to_someday` | Defer |
| `things_move_to_inbox` | Back to inbox |
| `things_reorder_task` | Change sort position (`index` and/or `today_index`) |

### Diagnostics

| Tool | What it does |
|------|-------------|
| `things_smoke_test` | End-to-end create/read/edit/complete/trash cycle |
| `things_delete_area` | Permanently delete an area |
| `things_delete_tag` | Permanently delete a tag |

---

## Parameter Reference

### `when` — Scheduling (most date requests use this)

| Value | Things view | Notes |
|-------|------------|-------|
| `today` | Today | Scheduled for today's date |
| `anytime` | Anytime | Triaged, no specific date |
| `someday` | Someday | Deferred indefinitely |
| `inbox` | Inbox | Default for new tasks; unprocessed |
| `YYYY-MM-DD` | Upcoming or Today | Future date → Upcoming (auto-surfaces on that day). Today's date or past → Today. |
| `none` | *(edit only)* | Strips dates without moving the task out of its project/area |

**Key distinction**: `when` controls which *view* the task appears in. It's the right choice for "schedule this for Tuesday", "move to someday", "put in inbox", etc.

### `deadline` — Hard due dates (use sparingly)

| Value | Effect |
|-------|--------|
| `YYYY-MM-DD` | Set a hard deadline (shown with a red badge in Things) |
| `none` | *(edit only)* Clear the deadline |

**Only use `deadline`** when the user explicitly says "deadline", "due date", or "due by". For general scheduling ("do this on Friday", "remind me Tuesday"), use `when` instead. A task can have both a `when` date and a `deadline`.

### `reminder` — Alarm time

| Value | Effect |
|-------|--------|
| `HH:MM` | Set reminder at this time on the task's scheduled date (e.g. `09:00`, `14:30`) |
| `none` | *(edit only)* Clear existing reminder |

The reminder fires at the specified time on whatever date the task is scheduled for (`when`). If the task has no scheduled date, the reminder won't fire until one is set.

### `note` — Task notes

| Value | Effect |
|-------|--------|
| *any text* | Set the notes |
| `none` | *(edit only)* Clear existing notes |

### `repeat` — Recurring tasks

| Value | Example |
|-------|---------|
| `daily` | Every day |
| `weekly` | Every week (same weekday as start) |
| `monthly` | Every month (same day of month) |
| `yearly` | Same date each year |
| `every N days/weeks/months/years` | `every 2 weeks`, `every 3 days` |
| `... until YYYY-MM-DD` | `weekly until 2026-06-01` |
| `... after completion` | `daily after completion` |
| `none` | *(edit only)* Clear recurrence |

Modifiers can be combined: `every 2 weeks until 2026-12-31 after completion`

### `area` — Area assignment *(edit only)*

| Value | Effect |
|-------|--------|
| *area UUID* | Assign to area |
| `none` | Remove from area |

### `heading` — Heading placement *(edit only)*

| Value | Effect |
|-------|--------|
| *heading UUID* | Place under heading (task must be in the heading's project) |
| `none` | Remove from heading |

### `tags` — Tag assignment

Pass comma-separated UUIDs. On edit, **replaces** existing tags entirely. To add a tag, you need to read existing tags first, then pass the full set.

### `index` / `today_index` — Sort position

Both are integers (negative values valid). Lower values sort first. `index` controls ordering in inbox, project, anytime, and someday lists. `today_index` controls ordering within the Today view.

---

## Quirks & Gotchas

### 1. Repeating tasks cannot live in Inbox

Things requires repeating tasks to be triaged. If you try to create a repeating task with `when: "inbox"` (or omit `when`), the API will return an error. Use `when: "anytime"`, `when: "today"`, `when: "someday"`, or a specific date.

### 2. Moving to a project/area/heading auto-triages

When you set `project`, `area`, or `heading` on a task (via create or edit), the task automatically moves out of Inbox to Anytime (`st=1`) unless you explicitly provide a `when` value. This matches Things.app behavior — items placed inside structural containers are considered "triaged".

### 3. Subtasks vs Checklist Items — two different things

| Feature | Subtask (`parent_task`) | Checklist item |
|---------|------------------------|----------------|
| Own dates/deadlines | Yes | No |
| Own tags | Yes | No |
| Own notes | Yes | No |
| Own reminders | Yes | No |
| Appears in Today/Inbox | Yes | No |
| Nesting | Full task under parent | Checkbox inside task detail |
| Create tool | `things_create_task` with `parent_task` | `things_create_checklist_item` |
| List tool | `things_list_project_tasks` (with parent UUID) | `things_list_checklist_items` |

**When to use which**: Checklist items are lightweight checkboxes for "steps within a task" (like a packing list). Subtasks are independent tasks that happen to be grouped under a parent (like project milestones).

### 4. Tags are set by UUID, not name

To tag a task, you need the tag's UUID. Workflow:
1. `things_list_tags` → find the UUID
2. `things_create_task` with `tags: "uuid1,uuid2"`

Same for projects, areas, and headings — always pass UUIDs, never names.

### 5. `things_list_project_tasks` works for subtasks too

Despite the name, passing a regular task's UUID (not a project) returns its subtasks. Useful for inspecting a task's child tasks.

### 6. Editing tags replaces, doesn't append

`things_edit_task` with `tags: "uuid1"` will **remove** all existing tags and set only `uuid1`. To add a tag to existing ones, read the task first, merge the tag lists, and send the full set.

### 7. Deadlines cannot be in the past

The API rejects deadlines before today. This is enforced server-side.

### 8. `cancel` vs `complete` vs `trash`

- **Complete** = task is done (counts toward completion stats, shows in Logbook)
- **Cancel** = task was abandoned (shows in Logbook with strikethrough, doesn't count as "done")
- **Trash** = task deleted (hidden from all views, can be restored with `untrash`)

All three can be undone: `uncomplete` reopens both completed and canceled tasks; `untrash` restores trashed tasks.

### 9. The Today view is `st=1` + date, not its own state

Under the hood, Things has three schedule states: Inbox (0), Anytime (1), Someday (2). "Today" is just Anytime with a scheduled date set to today. "Upcoming" is Someday with a future date. The `when` parameter abstracts this away.

### 10. Search is basic substring matching

`things_search_tasks` does case-insensitive substring matching on title and note fields. It returns all open tasks that match. There's no fuzzy matching or ranking.

---

## Common Workflows

### Add a task for today with a morning reminder
```
things_create_task(title: "Stand-up meeting", when: "today", reminder: "09:00")
```

### Create a task in a project under a heading
```
# First, find the project and heading UUIDs
things_list_projects()  →  find project UUID
things_list_project_tasks(project_uuid: "...")  →  find heading UUID (type: "heading")

things_create_task(title: "Write tests", project: "<project-uuid>", heading: "<heading-uuid>")
```

### Schedule a task for next Tuesday
```
things_create_task(title: "Review PR", when: "2026-03-03")
```
This goes to Upcoming and auto-surfaces in Today on March 3rd.

### Create a recurring task
```
things_create_task(title: "Weekly review", when: "today", repeat: "weekly")
```

### Create a task with a checklist
```
# Create the task
things_create_task(title: "Pack for trip", when: "today")  →  get UUID

# Add checklist items
things_create_checklist_item(title: "Passport", task_uuid: "...")
things_create_checklist_item(title: "Charger", task_uuid: "...")
things_create_checklist_item(title: "Toothbrush", task_uuid: "...")
```

### Move a batch of inbox tasks to a project
```
# Read inbox
things_list_inbox()  →  find task UUIDs

# Edit each one (sets project + auto-triages to Anytime)
things_edit_task(uuid: "task1", project: "<project-uuid>")
things_edit_task(uuid: "task2", project: "<project-uuid>")
```

### Defer a task to someday and clear its deadline
```
things_edit_task(uuid: "...", when: "someday", deadline: "none")
```

### Reorganize Today list order
```
things_reorder_task(uuid: "task1", today_index: 0)   # first
things_reorder_task(uuid: "task2", today_index: 1)   # second
things_reorder_task(uuid: "task3", today_index: 2)   # third
```
