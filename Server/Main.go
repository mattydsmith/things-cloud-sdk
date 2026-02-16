package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	things "github.com/arthursoares/things-cloud-sdk"
	"github.com/arthursoares/things-cloud-sdk/sync"
)

var (
	client *things.Client
	syncer *sync.Syncer
)

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func handleVerify(w http.ResponseWriter, r *http.Request) {
	resp, err := client.Verify()
	if err != nil {
		jsonError(w, fmt.Sprintf("verification failed: %v", err), 401)
		return
	}
	jsonResponse(w, resp)
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	changes, err := syncer.Sync()
	if err != nil {
		jsonError(w, fmt.Sprintf("sync failed: %v", err), 500)
		return
	}
	result := make([]map[string]interface{}, 0, len(changes))
	for _, c := range changes {
		result = append(result, map[string]interface{}{
			"type":   fmt.Sprintf("%T", c),
			"change": c,
		})
	}
	jsonResponse(w, map[string]interface{}{
		"changes_count": len(changes),
		"changes":       result,
	})
}

func handleInbox(w http.ResponseWriter, r *http.Request) {
	syncer.Sync()
	state := syncer.State()
	tasks, err := state.TasksInInbox(sync.QueryOpts{})
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get inbox: %v", err), 500)
		return
	}
	jsonResponse(w, tasks)
}

func handleToday(w http.ResponseWriter, r *http.Request) {
	syncer.Sync()
	state := syncer.State()
	tasks, err := state.TasksInToday(sync.QueryOpts{})
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get today: %v", err), 500)
		return
	}
	jsonResponse(w, tasks)
}

func handleAnytime(w http.ResponseWriter, r *http.Request) {
	syncer.Sync()
	state := syncer.State()
	tasks, err := state.TasksInAnytime(sync.QueryOpts{})
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get anytime: %v", err), 500)
		return
	}
	jsonResponse(w, tasks)
}

func handleProjects(w http.ResponseWriter, r *http.Request) {
	syncer.Sync()
	state := syncer.State()
	projects, err := state.AllProjects(sync.QueryOpts{})
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get projects: %v", err), 500)
		return
	}
	jsonResponse(w, projects)
}

func handleAreas(w http.ResponseWriter, r *http.Request) {
	syncer.Sync()
	state := syncer.State()
	areas, err := state.AllAreas(sync.QueryOpts{})
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get areas: %v", err), 500)
		return
	}
	jsonResponse(w, areas)
}

func handleTags(w http.ResponseWriter, r *http.Request) {
	syncer.Sync()
	state := syncer.State()
	tags, err := state.AllTags(sync.QueryOpts{})
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get tags: %v", err), 500)
		return
	}
	jsonResponse(w, tags)
}

func handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title    string `json:"title"`
		When     string `json:"when"`
		Project  string `json:"project"`
		Area     string `json:"area"`
		Notes    string `json:"notes"`
		Deadline string `json:"deadline"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}
	if req.Title == "" {
		jsonError(w, "title is required", 400)
		return
	}

	task := things.Task{
		UUID:  things.GenerateUUID(),
		Title: things.String(req.Title),
	}

	switch strings.ToLower(req.When) {
	case "today":
		task.Schedule = things.Schedule(things.TaskScheduleAnytime)
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		todayUnix := today.Unix()
		task.TodayIndexReferenceDate = &todayUnix
	case "anytime":
		task.Schedule = things.Schedule(things.TaskScheduleAnytime)
	case "someday":
		task.Schedule = things.Schedule(things.TaskScheduleSomeday)
	default:
		task.Schedule = things.Schedule(things.TaskScheduleInbox)
	}

	if req.Project != "" {
		task.ProjectID = things.String(req.Project)
		if task.Schedule == nil || *task.Schedule == things.TaskScheduleInbox {
			task.Schedule = things.Schedule(things.TaskScheduleAnytime)
		}
	}
	if req.Area != "" {
		task.AreaID = things.String(req.Area)
	}
	if req.Notes != "" {
		task.Note = things.NewFullNote(req.Notes)
	}
	if req.Deadline != "" {
		task.Deadline = things.String(req.Deadline)
	}

	histories, err := client.Histories()
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get histories: %v", err), 500)
		return
	}
	var historyID string
	if len(histories) > 0 {
		historyID = histories[0].ID
	} else {
		hist, err := client.CreateHistory()
		if err != nil {
			jsonError(w, fmt.Sprintf("failed to create history: %v", err), 500)
			return
		}
		historyID = hist.ID
	}

	items := []things.Item{things.NewCreateTaskItem(task)}
	if err := client.Write(historyID, items, -1); err != nil {
		jsonError(w, fmt.Sprintf("failed to write task: %v", err), 500)
		return
	}

	jsonResponse(w, map[string]string{
		"status": "created",
		"uuid":   task.UUID,
		"title":  *task.Title,
	})
}

func handleCompleteTask(w http.ResponseWriter, r *http.Request) {
	uuid := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	uuid = strings.TrimSuffix(uuid, "/complete")
	if uuid == "" {
		jsonError(w, "uuid is required", 400)
		return
	}

	histories, err := client.Histories()
	if err != nil || len(histories) == 0 {
		jsonError(w, "failed to get histories", 500)
		return
	}

	task := things.Task{
		UUID:   uuid,
		Status: things.Status(things.TaskStatusCompleted),
	}
	items := []things.Item{things.NewModifyTaskItem(task)}
	if err := client.Write(histories[0].ID, items, -1); err != nil {
		jsonError(w, fmt.Sprintf("failed to complete task: %v", err), 500)
		return
	}
	jsonResponse(w, map[string]string{"status": "completed", "uuid": uuid})
}

func handleTrashTask(w http.ResponseWriter, r *http.Request) {
	uuid := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	uuid = strings.TrimSuffix(uuid, "/trash")
	if uuid == "" {
		jsonError(w, "uuid is required", 400)
		return
	}

	histories, err := client.Histories()
	if err != nil || len(histories) == 0 {
		jsonError(w, "failed to get histories", 500)
		return
	}

	task := things.Task{
		UUID:   uuid,
		Status: things.Status(things.TaskStatusCanceled),
	}
	items := []things.Item{things.NewModifyTaskItem(task)}
	if err := client.Write(histories[0].ID, items, -1); err != nil {
		jsonError(w, fmt.Sprintf("failed to trash task: %v", err), 500)
		return
	}
	jsonResponse(w, map[string]string{"status": "trashed", "uuid": uuid})
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := os.Getenv("API_KEY")
		if apiKey == "" {
			next(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+apiKey {
			jsonError(w, "unauthorized", 401)
			return
		}
		next(w, r)
	}
}

func main() {
	username := os.Getenv("THINGS_USERNAME")
	password := os.Getenv("THINGS_PASSWORD")
	if username == "" || password == "" {
		log.Fatal("THINGS_USERNAME and THINGS_PASSWORD must be set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	client = things.New(things.APIEndpoint, username, password)

	var err error
	syncer, err = sync.Open("/data/things.db", client)
	if err != nil {
		log.Fatalf("failed to open sync database: %v", err)
	}
	defer syncer.Close()

	log.Println("Performing initial sync...")
	changes, err := syncer.Sync()
	if err != nil {
		log.Printf("Warning: initial sync failed: %v", err)
	} else {
		log.Printf("Initial sync complete: %d changes", len(changes))
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]string{"service": "things-cloud-api", "status": "ok"})
	})

	http.HandleFunc("/api/verify", authMiddleware(handleVerify))
	http.HandleFunc("/api/sync", authMiddleware(handleSync))
	http.HandleFunc("/api/tasks/inbox", authMiddleware(handleInbox))
	http.HandleFunc("/api/tasks/today", authMiddleware(handleToday))
	http.HandleFunc("/api/tasks/anytime", authMiddleware(handleAnytime))
	http.HandleFunc("/api/projects", authMiddleware(handleProjects))
	http.HandleFunc("/api/areas", authMiddleware(handleAreas))
	http.HandleFunc("/api/tags", authMiddleware(handleTags))

	http.HandleFunc("/api/tasks", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			handleCreateTask(w, r)
		} else {
			jsonError(w, "method not allowed", 405)
		}
	}))

	http.HandleFunc("/api/tasks/", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonError(w, "method not allowed", 405)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/complete") {
			handleCompleteTask(w, r)
		} else if strings.HasSuffix(r.URL.Path, "/trash") {
			handleTrashTask(w, r)
		} else {
			jsonError(w, "unknown action", 404)
		}
	}))

	log.Printf("Starting server on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
