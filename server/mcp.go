package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	things "github.com/arthursoares/things-cloud-sdk"
	"github.com/arthursoares/things-cloud-sdk/sync"
)

// ---------------------------------------------------------------------------
// Output types for JSON serialization
// ---------------------------------------------------------------------------

type taskOutput struct {
	UUID         string   `json:"uuid"`
	Title        string   `json:"title"`
	Note         string   `json:"note,omitempty"`
	Status       string   `json:"status"`
	Type         string   `json:"type"`
	Deadline     string   `json:"deadline,omitempty"`
	ScheduledFor string   `json:"scheduled_for,omitempty"`
	ProjectID    string   `json:"project_id,omitempty"`
	AreaID       string   `json:"area_id,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

type areaOutput struct {
	UUID  string `json:"uuid"`
	Title string `json:"title"`
}

type tagOutput struct {
	UUID      string `json:"uuid"`
	Title     string `json:"title"`
	Shorthand string `json:"shorthand,omitempty"`
}

type checklistOutput struct {
	UUID   string `json:"uuid"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// ---------------------------------------------------------------------------
// Format helpers
// ---------------------------------------------------------------------------

func formatTask(t *things.Task) taskOutput {
	o := taskOutput{
		UUID:  t.UUID,
		Title: t.Title,
		Note:  t.Note,
		Tags:  t.TagIDs,
	}
	switch t.Status {
	case things.TaskStatusPending:
		o.Status = "open"
	case things.TaskStatusCompleted:
		o.Status = "completed"
	case things.TaskStatusCanceled:
		o.Status = "canceled"
	}
	switch t.Type {
	case things.TaskTypeTask:
		o.Type = "task"
	case things.TaskTypeProject:
		o.Type = "project"
	case things.TaskTypeHeading:
		o.Type = "heading"
	}
	if t.DeadlineDate != nil {
		o.Deadline = t.DeadlineDate.Format("2006-01-02")
	}
	if t.ScheduledDate != nil {
		o.ScheduledFor = t.ScheduledDate.Format("2006-01-02")
	}
	if len(t.ParentTaskIDs) > 0 {
		o.ProjectID = t.ParentTaskIDs[0]
	}
	if len(t.AreaIDs) > 0 {
		o.AreaID = t.AreaIDs[0]
	}
	return o
}

func tasksResult(tasks []*things.Task) *mcp.CallToolResult {
	outputs := make([]taskOutput, len(tasks))
	for i, t := range tasks {
		outputs[i] = formatTask(t)
	}
	b, _ := json.MarshalIndent(outputs, "", "  ")
	return mcp.NewToolResultText(string(b))
}

func areasResult(areas []*things.Area) *mcp.CallToolResult {
	out := make([]areaOutput, len(areas))
	for i, a := range areas {
		out[i] = areaOutput{UUID: a.UUID, Title: a.Title}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(b))
}

func tagsResult(tags []*things.Tag) *mcp.CallToolResult {
	out := make([]tagOutput, len(tags))
	for i, t := range tags {
		out[i] = tagOutput{UUID: t.UUID, Title: t.Title, Shorthand: t.ShortHand}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(b))
}

func checklistResult(items []*things.CheckListItem) *mcp.CallToolResult {
	out := make([]checklistOutput, len(items))
	for i, c := range items {
		s := "open"
		if c.Status == things.TaskStatusCompleted {
			s = "completed"
		}
		out[i] = checklistOutput{UUID: c.UUID, Title: c.Title, Status: s}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(b))
}

func writeResult(fields map[string]string) *mcp.CallToolResult {
	b, _ := json.Marshal(fields)
	return mcp.NewToolResultText(string(b))
}

// ---------------------------------------------------------------------------
// Read tool handlers
// ---------------------------------------------------------------------------

func mcpListToday(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	syncer.Sync()
	tasks, err := syncer.State().TasksInToday(sync.QueryOpts{})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return tasksResult(tasks), nil
}

func mcpListInbox(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	syncer.Sync()
	tasks, err := syncer.State().TasksInInbox(sync.QueryOpts{})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return tasksResult(tasks), nil
}

func mcpListProjects(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	syncer.Sync()
	projects, err := syncer.State().AllProjects(sync.QueryOpts{})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return tasksResult(projects), nil
}

func mcpListAreas(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	syncer.Sync()
	areas, err := syncer.State().AllAreas()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return areasResult(areas), nil
}

func mcpListTags(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	syncer.Sync()
	tags, err := syncer.State().AllTags()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return tagsResult(tags), nil
}

func mcpListAllTasks(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	syncer.Sync()
	tasks, err := syncer.State().AllTasks(sync.QueryOpts{})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return tasksResult(tasks), nil
}

func mcpListProjectTasks(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectUUID, err := req.RequireString("project_uuid")
	if err != nil {
		return mcp.NewToolResultError("project_uuid is required"), nil
	}
	syncer.Sync()
	tasks, err := syncer.State().TasksInProject(projectUUID, sync.QueryOpts{})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return tasksResult(tasks), nil
}

func mcpListAreaTasks(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	areaUUID, err := req.RequireString("area_uuid")
	if err != nil {
		return mcp.NewToolResultError("area_uuid is required"), nil
	}
	syncer.Sync()
	tasks, err := syncer.State().TasksInArea(areaUUID, sync.QueryOpts{})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return tasksResult(tasks), nil
}

func mcpListChecklistItems(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskUUID, err := req.RequireString("task_uuid")
	if err != nil {
		return mcp.NewToolResultError("task_uuid is required"), nil
	}
	syncer.Sync()
	items, err := syncer.State().ChecklistItems(taskUUID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return checklistResult(items), nil
}

func mcpGetTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	syncer.Sync()
	task, err := syncer.State().Task(uuid)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if task == nil {
		return mcp.NewToolResultError("task not found"), nil
	}
	b, _ := json.MarshalIndent(formatTask(task), "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func mcpGetArea(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	syncer.Sync()
	area, err := syncer.State().Area(uuid)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if area == nil {
		return mcp.NewToolResultError("area not found"), nil
	}
	b, _ := json.MarshalIndent(areaOutput{UUID: area.UUID, Title: area.Title}, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func mcpGetTag(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	syncer.Sync()
	tag, err := syncer.State().Tag(uuid)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if tag == nil {
		return mcp.NewToolResultError("tag not found"), nil
	}
	b, _ := json.MarshalIndent(tagOutput{UUID: tag.UUID, Title: tag.Title, Shorthand: tag.ShortHand}, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

// ---------------------------------------------------------------------------
// Write tool handlers
// ---------------------------------------------------------------------------

func mcpCreateTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := req.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError("title is required"), nil
	}
	uuid, err := createTask(CreateTaskRequest{
		Title:      title,
		Note:       req.GetString("note", ""),
		When:       req.GetString("when", ""),
		Deadline:   req.GetString("deadline", ""),
		Project:    req.GetString("project", ""),
		ParentTask: req.GetString("parent_task", ""),
		Tags:       req.GetString("tags", ""),
		Repeat:     req.GetString("repeat", ""),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "created", "uuid": uuid, "title": title}), nil
}

func mcpCompleteTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := completeTask(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "completed", "uuid": uuid}), nil
}

func mcpEditTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := editTask(EditTaskRequest{
		UUID:       uuid,
		Title:      req.GetString("title", ""),
		Note:       req.GetString("note", ""),
		When:       req.GetString("when", ""),
		Deadline:   req.GetString("deadline", ""),
		Project:    req.GetString("project", ""),
		ParentTask: req.GetString("parent_task", ""),
		Tags:       req.GetString("tags", ""),
		Repeat:     req.GetString("repeat", ""),
	}); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "updated", "uuid": uuid}), nil
}

func mcpTrashTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := trashTask(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "trashed", "uuid": uuid}), nil
}

func mcpMoveToToday(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := moveTaskToToday(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "moved_to_today", "uuid": uuid}), nil
}

func mcpMoveToAnytime(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := moveTaskToAnytime(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "moved_to_anytime", "uuid": uuid}), nil
}

func mcpMoveToSomeday(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := moveTaskToSomeday(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "moved_to_someday", "uuid": uuid}), nil
}

func mcpMoveToInbox(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := moveTaskToInbox(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "moved_to_inbox", "uuid": uuid}), nil
}

func mcpUncompleteTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := uncompleteTask(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "uncompleted", "uuid": uuid}), nil
}

func mcpUntrashTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := untrashTask(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "restored", "uuid": uuid}), nil
}

func mcpCreateArea(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := req.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError("title is required"), nil
	}
	var tagUUIDs []string
	if tagStr := req.GetString("tags", ""); tagStr != "" {
		tagUUIDs = strings.Split(tagStr, ",")
	}
	uuid, err := createArea(title, tagUUIDs)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "created", "uuid": uuid, "title": title}), nil
}

func mcpCreateTag(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := req.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError("title is required"), nil
	}
	uuid, err := createTag(title, req.GetString("shorthand", ""), req.GetString("parent", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "created", "uuid": uuid, "title": title}), nil
}

func mcpCreateHeading(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := req.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError("title is required"), nil
	}
	uuid, err := createHeading(title, req.GetString("project", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "created", "uuid": uuid, "title": title}), nil
}

func mcpCreateProject(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := req.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError("title is required"), nil
	}
	uuid, err := createProject(
		title,
		req.GetString("note", ""),
		req.GetString("when", ""),
		req.GetString("deadline", ""),
		req.GetString("area", ""),
	)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "created", "uuid": uuid, "title": title}), nil
}

// ---------------------------------------------------------------------------
// Search handler
// ---------------------------------------------------------------------------

func mcpSearchTasks(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}
	syncer.Sync()
	tasks, err := syncer.State().AllTasks(sync.QueryOpts{})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	q := strings.ToLower(query)
	var matches []*things.Task
	for _, t := range tasks {
		if strings.Contains(strings.ToLower(t.Title), q) || strings.Contains(strings.ToLower(t.Note), q) {
			matches = append(matches, t)
		}
	}
	if matches == nil {
		matches = []*things.Task{}
	}
	return tasksResult(matches), nil
}

// ---------------------------------------------------------------------------
// Checklist item handlers
// ---------------------------------------------------------------------------

func mcpCreateChecklistItem(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := req.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError("title is required"), nil
	}
	taskUUID, err := req.RequireString("task_uuid")
	if err != nil {
		return mcp.NewToolResultError("task_uuid is required"), nil
	}
	uuid, err := createChecklistItem(title, taskUUID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "created", "uuid": uuid, "title": title}), nil
}

func mcpCompleteChecklistItem(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := completeChecklistItem(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "completed", "uuid": uuid}), nil
}

func mcpUncompleteChecklistItem(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := uncompleteChecklistItem(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "uncompleted", "uuid": uuid}), nil
}

func mcpDeleteChecklistItem(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, err := req.RequireString("uuid")
	if err != nil {
		return mcp.NewToolResultError("uuid is required"), nil
	}
	if err := deleteChecklistItem(uuid); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return writeResult(map[string]string{"status": "deleted", "uuid": uuid}), nil
}

// ---------------------------------------------------------------------------
// Smoke test
// ---------------------------------------------------------------------------

type smokeCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "pass" or "fail"
	Detail string `json:"detail,omitempty"`
}

func mcpSmokeTest(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var checks []smokeCheck

	check := func(name string, fn func() error) {
		if err := fn(); err != nil {
			checks = append(checks, smokeCheck{Name: name, Status: "fail", Detail: err.Error()})
		} else {
			checks = append(checks, smokeCheck{Name: name, Status: "pass"})
		}
	}

	// 1. Sync
	check("sync", func() error {
		_, err := syncer.Sync()
		return err
	})

	// 2. List today
	check("list_today", func() error {
		_, err := syncer.State().TasksInToday(sync.QueryOpts{})
		return err
	})

	// 3. List projects
	check("list_projects", func() error {
		_, err := syncer.State().AllProjects(sync.QueryOpts{})
		return err
	})

	// 4. Create task
	var taskUUID string
	check("create_task", func() error {
		uuid, err := createTask(CreateTaskRequest{Title: "[smoke-test] Verify", When: "today"})
		if err != nil {
			return err
		}
		taskUUID = uuid
		return nil
	})

	if taskUUID == "" {
		// Can't continue without the task
		b, _ := json.MarshalIndent(map[string]any{
			"passed": countStatus(checks, "pass"),
			"failed": countStatus(checks, "fail"),
			"checks": checks,
		}, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}

	// 5. Get task
	check("get_task", func() error {
		task, err := syncer.State().Task(taskUUID)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("task not found after create")
		}
		if task.Title != "[smoke-test] Verify" {
			return fmt.Errorf("title mismatch: got %q", task.Title)
		}
		if task.Status != things.TaskStatusPending {
			return fmt.Errorf("status mismatch: got %v, want pending", task.Status)
		}
		return nil
	})

	// 6. Edit task
	check("edit_task", func() error {
		return editTask(EditTaskRequest{UUID: taskUUID, Title: "[smoke-test] Verify (edited)"})
	})

	// 7. Verify edit
	check("verify_edit", func() error {
		task, err := syncer.State().Task(taskUUID)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("task not found after edit")
		}
		if task.Title != "[smoke-test] Verify (edited)" {
			return fmt.Errorf("title mismatch after edit: got %q", task.Title)
		}
		return nil
	})

	// 8. Complete task
	check("complete_task", func() error {
		return completeTask(taskUUID)
	})

	// 9. Verify complete
	check("verify_complete", func() error {
		task, err := syncer.State().Task(taskUUID)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("task not found after complete")
		}
		if task.Status != things.TaskStatusCompleted {
			return fmt.Errorf("status mismatch: got %v, want completed", task.Status)
		}
		return nil
	})

	// 10. Trash task (cleanup)
	check("trash_task", func() error {
		return trashTask(taskUUID)
	})

	b, _ := json.MarshalIndent(map[string]any{
		"passed": countStatus(checks, "pass"),
		"failed": countStatus(checks, "fail"),
		"checks": checks,
	}, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func countStatus(checks []smokeCheck, status string) int {
	n := 0
	for _, c := range checks {
		if c.Status == status {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// MCP server setup
// ---------------------------------------------------------------------------

func newMCPHandler() http.Handler {
	s := server.NewMCPServer("Things Cloud", "1.0.0")

	// --- Read tools ---

	s.AddTool(mcp.NewTool("things_list_today",
		mcp.WithDescription("List tasks scheduled for today in Things"),
		mcp.WithReadOnlyHintAnnotation(true),
	), mcpListToday)

	s.AddTool(mcp.NewTool("things_list_inbox",
		mcp.WithDescription("List tasks in the Things inbox"),
		mcp.WithReadOnlyHintAnnotation(true),
	), mcpListInbox)

	s.AddTool(mcp.NewTool("things_list_projects",
		mcp.WithDescription("List all projects in Things"),
		mcp.WithReadOnlyHintAnnotation(true),
	), mcpListProjects)

	s.AddTool(mcp.NewTool("things_list_areas",
		mcp.WithDescription("List all areas in Things"),
		mcp.WithReadOnlyHintAnnotation(true),
	), mcpListAreas)

	s.AddTool(mcp.NewTool("things_list_tags",
		mcp.WithDescription("List all tags in Things"),
		mcp.WithReadOnlyHintAnnotation(true),
	), mcpListTags)

	s.AddTool(mcp.NewTool("things_list_all_tasks",
		mcp.WithDescription("List all open tasks in Things"),
		mcp.WithReadOnlyHintAnnotation(true),
	), mcpListAllTasks)

	s.AddTool(mcp.NewTool("things_list_project_tasks",
		mcp.WithDescription("List tasks in a specific Things project"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("project_uuid",
			mcp.Required(),
			mcp.Description("UUID of the project"),
		),
	), mcpListProjectTasks)

	s.AddTool(mcp.NewTool("things_list_area_tasks",
		mcp.WithDescription("List tasks in a specific Things area"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("area_uuid",
			mcp.Required(),
			mcp.Description("UUID of the area"),
		),
	), mcpListAreaTasks)

	s.AddTool(mcp.NewTool("things_list_checklist_items",
		mcp.WithDescription("List checklist items for a specific Things task"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("task_uuid",
			mcp.Required(),
			mcp.Description("UUID of the task"),
		),
	), mcpListChecklistItems)

	s.AddTool(mcp.NewTool("things_get_task",
		mcp.WithDescription("Get a single Things task by UUID"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the task"),
		),
	), mcpGetTask)

	s.AddTool(mcp.NewTool("things_get_area",
		mcp.WithDescription("Get a single Things area by UUID"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the area"),
		),
	), mcpGetArea)

	s.AddTool(mcp.NewTool("things_get_tag",
		mcp.WithDescription("Get a single Things tag by UUID"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the tag"),
		),
	), mcpGetTag)

	// --- Write tools ---

	s.AddTool(mcp.NewTool("things_create_task",
		mcp.WithDescription("Create a new task in Things"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Task title"),
		),
		mcp.WithString("note",
			mcp.Description("Task notes"),
		),
		mcp.WithString("when",
			mcp.Description("When to start the task. Use this for most date-related requests. Accepts: 'today', 'anytime' (triaged, no date), 'someday' (deferred), 'inbox' (default), or a YYYY-MM-DD date. A future date puts the task in Upcoming and auto-surfaces it on that day. Today's date or past dates go to Today view."),
		),
		mcp.WithString("deadline",
			mcp.Description("Hard deadline in YYYY-MM-DD format. Only use when the user explicitly mentions a deadline or due date — not for general scheduling. Most date requests should use 'when' instead."),
		),
		mcp.WithString("project",
			mcp.Description("Project UUID to add the task to"),
		),
		mcp.WithString("parent_task",
			mcp.Description("Parent task UUID to create this as a subtask. Takes precedence over project."),
		),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tag UUIDs"),
		),
		mcp.WithString("repeat",
			mcp.Description("Recurrence rule. Accepts: 'daily', 'weekly', 'monthly', 'yearly', or 'every N days/weeks/months/years'. Append ' after completion' for repeat-after-completion mode (e.g. 'daily after completion'). Weekly defaults to the current weekday, monthly to the current day of month."),
		),
	), mcpCreateTask)

	s.AddTool(mcp.NewTool("things_complete_task",
		mcp.WithDescription("Mark a Things task as completed"),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the task to complete"),
		),
	), mcpCompleteTask)

	s.AddTool(mcp.NewTool("things_edit_task",
		mcp.WithDescription("Edit an existing Things task"),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the task to edit"),
		),
		mcp.WithString("title",
			mcp.Description("New title"),
		),
		mcp.WithString("note",
			mcp.Description("New notes"),
		),
		mcp.WithString("when",
			mcp.Description("When to start the task. Use this for most date-related requests. Accepts: 'today', 'anytime' (triaged, no date), 'someday' (deferred), 'inbox' (clears schedule), 'none' (strip dates, keep in project/area), or a YYYY-MM-DD date. A future date puts the task in Upcoming. Today's date or past dates go to Today view."),
		),
		mcp.WithString("deadline",
			mcp.Description("Hard deadline in YYYY-MM-DD format, or 'none' to clear an existing deadline. Only use when the user explicitly mentions a deadline or due date — not for general scheduling."),
		),
		mcp.WithString("project",
			mcp.Description("New project UUID"),
		),
		mcp.WithString("parent_task",
			mcp.Description("Move task under a parent task (make it a subtask). Takes precedence over project."),
		),
		mcp.WithString("tags",
			mcp.Description("New comma-separated tag UUIDs (replaces existing)"),
		),
		mcp.WithString("repeat",
			mcp.Description("Recurrence rule. Accepts: 'daily', 'weekly', 'monthly', 'yearly', 'every N days/weeks/months/years', or 'none' to clear. Append ' after completion' for repeat-after-completion mode."),
		),
	), mcpEditTask)

	s.AddTool(mcp.NewTool("things_trash_task",
		mcp.WithDescription("Move a Things task to the trash"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the task to trash"),
		),
	), mcpTrashTask)

	s.AddTool(mcp.NewTool("things_move_to_today",
		mcp.WithDescription("Move a Things task to the Today view"),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the task to move"),
		),
	), mcpMoveToToday)

	s.AddTool(mcp.NewTool("things_move_to_anytime",
		mcp.WithDescription("Move a Things task to the Anytime view"),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the task to move"),
		),
	), mcpMoveToAnytime)

	s.AddTool(mcp.NewTool("things_move_to_someday",
		mcp.WithDescription("Move a Things task to the Someday view"),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the task to move"),
		),
	), mcpMoveToSomeday)

	s.AddTool(mcp.NewTool("things_move_to_inbox",
		mcp.WithDescription("Move a Things task back to the Inbox"),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the task to move"),
		),
	), mcpMoveToInbox)

	s.AddTool(mcp.NewTool("things_uncomplete_task",
		mcp.WithDescription("Mark a completed Things task as open again"),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the task to uncomplete"),
		),
	), mcpUncompleteTask)

	s.AddTool(mcp.NewTool("things_untrash_task",
		mcp.WithDescription("Restore a Things task from the trash"),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the task to restore"),
		),
	), mcpUntrashTask)

	s.AddTool(mcp.NewTool("things_create_area",
		mcp.WithDescription("Create a new area in Things"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Area title"),
		),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tag UUIDs to associate"),
		),
	), mcpCreateArea)

	s.AddTool(mcp.NewTool("things_create_tag",
		mcp.WithDescription("Create a new tag in Things"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Tag title"),
		),
		mcp.WithString("shorthand",
			mcp.Description("Tag keyboard shortcut"),
		),
		mcp.WithString("parent",
			mcp.Description("Parent tag UUID for nesting"),
		),
	), mcpCreateTag)

	s.AddTool(mcp.NewTool("things_create_heading",
		mcp.WithDescription("Create a heading within a project in Things"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Heading title"),
		),
		mcp.WithString("project",
			mcp.Description("Project UUID to add the heading to"),
		),
	), mcpCreateHeading)

	s.AddTool(mcp.NewTool("things_create_project",
		mcp.WithDescription("Create a new project in Things"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Project title"),
		),
		mcp.WithString("note",
			mcp.Description("Project notes"),
		),
		mcp.WithString("when",
			mcp.Description("Schedule: today, anytime (default), or someday"),
			mcp.Enum("today", "anytime", "someday"),
		),
		mcp.WithString("deadline",
			mcp.Description("Deadline in YYYY-MM-DD format"),
		),
		mcp.WithString("area",
			mcp.Description("Area UUID to assign the project to"),
		),
	), mcpCreateProject)

	// --- Search tools ---

	s.AddTool(mcp.NewTool("things_search_tasks",
		mcp.WithDescription("Search for tasks by title or note content (case-insensitive substring match)"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Text to search for in task titles and notes"),
		),
	), mcpSearchTasks)

	// --- Checklist item tools ---

	s.AddTool(mcp.NewTool("things_create_checklist_item",
		mcp.WithDescription("Add a checklist item to a Things task"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Checklist item text"),
		),
		mcp.WithString("task_uuid",
			mcp.Required(),
			mcp.Description("UUID of the task to add the checklist item to"),
		),
	), mcpCreateChecklistItem)

	s.AddTool(mcp.NewTool("things_complete_checklist_item",
		mcp.WithDescription("Mark a checklist item as completed"),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the checklist item to complete"),
		),
	), mcpCompleteChecklistItem)

	s.AddTool(mcp.NewTool("things_uncomplete_checklist_item",
		mcp.WithDescription("Mark a completed checklist item as open again"),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the checklist item to uncomplete"),
		),
	), mcpUncompleteChecklistItem)

	s.AddTool(mcp.NewTool("things_delete_checklist_item",
		mcp.WithDescription("Delete a checklist item"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("uuid",
			mcp.Required(),
			mcp.Description("UUID of the checklist item to delete"),
		),
	), mcpDeleteChecklistItem)

	// --- Diagnostic tools ---

	s.AddTool(mcp.NewTool("things_smoke_test",
		mcp.WithDescription("Run a smoke test that creates a task, verifies read/edit/complete, then cleans up. Returns pass/fail results for each check."),
	), mcpSmokeTest)

	return server.NewStreamableHTTPServer(s,
		server.WithEndpointPath("/mcp"),
		server.WithStateLess(true),
	)
}
