package main

import (
	"fmt"
	"net/http"

	things "github.com/arthursoares/things-cloud-sdk"
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

type syncStateAccessor struct{}

func (*syncStateAccessor) Task(uuid string) (*things.Task, error) {
	return syncer.State().Task(uuid)
}

func (*syncStateAccessor) Area(uuid string) (*things.Area, error) {
	return syncer.State().Area(uuid)
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
	tasks, err := state.TasksInToday(opts)
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get today widget items: %v", err), http.StatusInternalServerError)
		return
	}
	if tasks == nil {
		jsonResponse(w, []widgetTodayItem{})
		return
	}

	lookup := &syncStateAccessor{}
	items := make([]widgetTodayItem, len(tasks))
	for i, task := range tasks {
		items[i] = formatWidgetTodayItem(lookup, task)
	}

	jsonResponse(w, items)
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
