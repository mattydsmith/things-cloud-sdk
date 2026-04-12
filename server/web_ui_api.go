package main

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	things "github.com/arthursoares/things-cloud-sdk"
	"github.com/arthursoares/things-cloud-sdk/sync"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"service": "things-cloud-api", "status": "ok"})
}

type widgetTodayItem struct {
	UUID        string `json:"uuid"`
	Title       string `json:"title"`
	ProjectName string `json:"projectName"`
	IsCompleted bool   `json:"isCompleted"`
}

const widgetExcludedProjectUUID = "MBtLPdfaYx3evGuzYDyHEX"

type widgetLookup interface {
	Task(uuid string) (*things.Task, error)
	Area(uuid string) (*things.Area, error)
}

func widgetProjectName(state widgetLookup, task *things.Task) string {
	if len(task.ParentTaskIDs) > 0 {
		parent, err := state.Task(task.ParentTaskIDs[0])
		if err == nil && parent != nil && parent.Title != "" {
			return parent.Title
		}
	}
	if len(task.AreaIDs) > 0 {
		area, err := state.Area(task.AreaIDs[0])
		if err == nil && area != nil && area.Title != "" {
			return area.Title
		}
	}
	return ""
}

func formatWidgetTodayItem(state widgetLookup, task *things.Task) widgetTodayItem {
	return widgetTodayItem{
		UUID:        task.UUID,
		Title:       task.Title,
		ProjectName: widgetProjectName(state, task),
		IsCompleted: task.Status == things.TaskStatusCompleted,
	}
}

func widgetParentProjectUUID(state widgetLookup, task *things.Task, seen map[string]struct{}) string {
	current := task
	for current != nil && len(current.ParentTaskIDs) > 0 {
		parentID := current.ParentTaskIDs[0]
		if parentID == "" {
			return ""
		}
		if _, ok := seen[parentID]; ok {
			return ""
		}
		seen[parentID] = struct{}{}

		parent, err := state.Task(parentID)
		if err != nil || parent == nil {
			return parentID
		}
		if parent.Type == things.TaskTypeProject || len(parent.ParentTaskIDs) == 0 {
			return parent.UUID
		}
		current = parent
	}
	return ""
}

func widgetRootProjectUUID(state widgetLookup, task *things.Task) string {
	seen := map[string]struct{}{}
	if projectUUID := widgetParentProjectUUID(state, task, seen); projectUUID != "" {
		return projectUUID
	}

	if task != nil && len(task.ActionGroupIDs) > 0 {
		headingID := task.ActionGroupIDs[0]
		if headingID != "" {
			if _, ok := seen[headingID]; !ok {
				seen[headingID] = struct{}{}
				heading, err := state.Task(headingID)
				if err == nil && heading != nil {
					if projectUUID := widgetParentProjectUUID(state, heading, seen); projectUUID != "" {
						return projectUUID
					}
				}
			}
		}
	}
	return ""
}

func widgetIncludeTask(state widgetLookup, task *things.Task) bool {
	return widgetRootProjectUUID(state, task) != widgetExcludedProjectUUID
}

type syncStateAccessor struct{}

func (*syncStateAccessor) Task(uuid string) (*things.Task, error) {
	return syncer.State().Task(uuid)
}

func (*syncStateAccessor) Area(uuid string) (*things.Area, error) {
	return syncer.State().Area(uuid)
}

func widgetTodayStartUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func isOverdueOpenTask(task *things.Task, todayStart time.Time) bool {
	return task != nil &&
		task.Status == things.TaskStatusPending &&
		task.ScheduledDate != nil &&
		task.ScheduledDate.Before(todayStart)
}

func sortOverdueTasks(tasks []*things.Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		left := tasks[i]
		right := tasks[j]
		switch {
		case left == nil:
			return false
		case right == nil:
			return true
		case left.ScheduledDate == nil:
			return false
		case right.ScheduledDate == nil:
			return true
		case !left.ScheduledDate.Equal(*right.ScheduledDate):
			return left.ScheduledDate.Before(*right.ScheduledDate)
		case left.Index != right.Index:
			return left.Index < right.Index
		default:
			return left.UUID < right.UUID
		}
	})
}

func widgetMergedTodayTasks(state *sync.State) ([]*things.Task, error) {
	todayTasks, err := state.TasksInToday(sync.QueryOpts{})
	if err != nil {
		return nil, err
	}

	allTasks, err := state.AllTasks(sync.QueryOpts{})
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(todayTasks))
	for _, task := range todayTasks {
		if task == nil {
			continue
		}
		seen[task.UUID] = struct{}{}
	}

	todayStart := widgetTodayStartUTC()
	overdue := make([]*things.Task, 0)
	for _, task := range allTasks {
		if task == nil || !isOverdueOpenTask(task, todayStart) {
			continue
		}
		if _, ok := seen[task.UUID]; ok {
			continue
		}
		seen[task.UUID] = struct{}{}
		overdue = append(overdue, task)
	}

	sortOverdueTasks(overdue)

	merged := make([]*things.Task, 0, len(overdue)+len(todayTasks))
	merged = append(merged, overdue...)
	merged = append(merged, todayTasks...)

	return merged, nil
}

func paginateWidgetTodayItems(items []widgetTodayItem, opts sync.QueryOpts) []widgetTodayItem {
	if opts.Offset >= len(items) {
		return []widgetTodayItem{}
	}

	start := opts.Offset
	if start < 0 {
		start = 0
	}
	end := len(items)
	if opts.Limit > 0 && start+opts.Limit < end {
		end = start + opts.Limit
	}
	return items[start:end]
}

func handleWidgetToday(w http.ResponseWriter, r *http.Request) {
	opts, err := paginationQueryOpts(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := syncForRead(); err != nil {
		jsonError(w, fmt.Sprintf("pre-read sync failed: %v", err), http.StatusServiceUnavailable)
		return
	}

	state := syncer.State()
	tasks, err := widgetMergedTodayTasks(state)
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get today widget items: %v", err), http.StatusInternalServerError)
		return
	}
	if tasks == nil {
		jsonResponse(w, []widgetTodayItem{})
		return
	}

	lookup := &syncStateAccessor{}
	items := make([]widgetTodayItem, 0, len(tasks))
	for _, task := range tasks {
		if !widgetIncludeTask(lookup, task) {
			continue
		}
		items = append(items, formatWidgetTodayItem(lookup, task))
	}

	jsonResponse(w, paginateWidgetTodayItems(items, opts))
}

func handleTaskByUUID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uuid := r.PathValue("uuid")
	if err := validateUUID("uuid", uuid); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := syncForRead(); err != nil {
		jsonError(w, fmt.Sprintf("pre-read sync failed: %v", err), http.StatusServiceUnavailable)
		return
	}

	task, err := syncer.State().Task(uuid)
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get task: %v", err), http.StatusInternalServerError)
		return
	}
	if task == nil {
		jsonError(w, "task not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, task)
}

func handleTaskChecklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uuid := r.PathValue("uuid")
	if err := validateUUID("uuid", uuid); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := syncForRead(); err != nil {
		jsonError(w, fmt.Sprintf("pre-read sync failed: %v", err), http.StatusServiceUnavailable)
		return
	}

	task, err := syncer.State().Task(uuid)
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get task: %v", err), http.StatusInternalServerError)
		return
	}
	if task == nil {
		jsonError(w, "task not found", http.StatusNotFound)
		return
	}

	items, err := syncer.State().ChecklistItems(uuid)
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get checklist items: %v", err), http.StatusInternalServerError)
		return
	}
	if items == nil {
		jsonResponse(w, []any{})
		return
	}

	jsonResponse(w, items)
}

func handleAreaByUUID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uuid := r.PathValue("uuid")
	if err := validateUUID("uuid", uuid); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := syncForRead(); err != nil {
		jsonError(w, fmt.Sprintf("pre-read sync failed: %v", err), http.StatusServiceUnavailable)
		return
	}

	area, err := syncer.State().Area(uuid)
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get area: %v", err), http.StatusInternalServerError)
		return
	}
	if area == nil {
		jsonError(w, "area not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, area)
}

func handleProjectTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uuid := r.PathValue("uuid")
	if err := validateUUID("uuid", uuid); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	opts, err := paginationQueryOpts(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := syncForRead(); err != nil {
		jsonError(w, fmt.Sprintf("pre-read sync failed: %v", err), http.StatusServiceUnavailable)
		return
	}

	tasks, err := syncer.State().TasksInProject(uuid, opts)
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get project tasks: %v", err), http.StatusInternalServerError)
		return
	}
	if tasks == nil {
		jsonResponse(w, []any{})
		return
	}

	jsonResponse(w, tasks)
}

func handleAreaTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uuid := r.PathValue("uuid")
	if err := validateUUID("uuid", uuid); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	opts, err := paginationQueryOpts(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := syncForRead(); err != nil {
		jsonError(w, fmt.Sprintf("pre-read sync failed: %v", err), http.StatusServiceUnavailable)
		return
	}

	tasks, err := syncer.State().TasksInArea(uuid, opts)
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get area tasks: %v", err), http.StatusInternalServerError)
		return
	}
	if tasks == nil {
		jsonResponse(w, []any{})
		return
	}

	jsonResponse(w, tasks)
}
