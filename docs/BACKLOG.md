# Things MCP — Backlog

## Medium Priority

### Add area assignment on task edit
The `things_edit_task` tool can set a project but not an area. Add an `area` parameter that sets the area on a task.

### Add notes clearing (`note: "none"`)
Can set notes but not clear them. Add support for `note: "none"` on `things_edit_task` to clear notes, similar to `deadline: "none"`.

### Add completed tasks list tool
No way to see recently completed tasks. Add a `things_list_completed` tool that returns completed tasks, possibly with a date range filter.

## Lower Priority

### ~~Add recurring task support~~ (Done)
Added `repeat` parameter to `things_create_task` and `things_edit_task`. Supports: daily, weekly, monthly, yearly, every N days/weeks/months/years, after completion mode, and "none" to clear.

### Investigate tag/area deletion via Tombstone2
Areas and tags can't currently be deleted. The SDK supports `Tombstone2` entities for explicit deletion — test if writing a Tombstone2 for a tag/area UUID actually deletes it.

### ~~Add subtask support~~ (Done)
Added `parent_task` parameter to `things_create_task` and `things_edit_task`. Sets the `pr` wire field to the parent task UUID.
