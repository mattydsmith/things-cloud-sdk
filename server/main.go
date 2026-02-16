package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	things "github.com/arthursoares/things-cloud-sdk"
	"github.com/arthursoares/things-cloud-sdk/sync"
)

var (
	client  *things.Client
	syncer  *sync.Syncer
	history *things.History
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
	jsonResponse(w, map[string]interface{}{
		"changes_count": len(changes),
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
	if tasks == nil {
		jsonResponse(w, []*things.Task{})
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
	if tasks == nil {
		jsonResponse(w, []*things.Task{})
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
	if projects == nil {
		jsonResponse(w, []*things.Task{})
		return
	}
	jsonResponse(w, projects)
}

func handleAreas(w http.ResponseWriter, r *http.Request) {
	syncer.Sync()
	state := syncer.State()
	areas, err := state.AllAreas()
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get areas: %v", err), 500)
		return
	}
	if areas == nil {
		jsonResponse(w, []*things.Area{})
		return
	}
	jsonResponse(w, areas)
}

func handleTags(w http.ResponseWriter, r *http.Request) {
	syncer.Sync()
	state := syncer.State()
	tags, err := state.AllTags()
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to get tags: %v", err), 500)
		return
	}
	if tags == nil {
		jsonResponse(w, []*things.Tag{})
		return
	}
	jsonResponse(w, tags)
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

func authHandlerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := os.Getenv("API_KEY")
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			jsonError(w, "unauthorized", 401)
			return
		}
		next.ServeHTTP(w, r)
	})
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

	// Get history for write operations
	history, err = client.OwnHistory()
	if err != nil {
		log.Fatalf("failed to get history: %v", err)
	}
	if err := history.Sync(); err != nil {
		log.Fatalf("failed to sync history: %v", err)
	}
	log.Println("History ready for writes")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			jsonError(w, "not found", 404)
			return
		}
		jsonResponse(w, map[string]string{"service": "things-cloud-api", "status": "ok"})
	})

	http.HandleFunc("/api/verify", authMiddleware(handleVerify))
	http.HandleFunc("/api/sync", authMiddleware(handleSync))
	http.HandleFunc("/api/tasks/inbox", authMiddleware(handleInbox))
	http.HandleFunc("/api/tasks/today", authMiddleware(handleToday))
	http.HandleFunc("/api/projects", authMiddleware(handleProjects))
	http.HandleFunc("/api/areas", authMiddleware(handleAreas))
	http.HandleFunc("/api/tags", authMiddleware(handleTags))

	// Write endpoints
	http.HandleFunc("/api/tasks/create", authMiddleware(handleCreateTask))
	http.HandleFunc("/api/tasks/complete", authMiddleware(handleCompleteTask))
	http.HandleFunc("/api/tasks/trash", authMiddleware(handleTrashTask))
	http.HandleFunc("/api/tasks/edit", authMiddleware(handleEditTask))

	// MCP endpoint (no bearer auth — claude.ai connectors use OAuth which we don't implement)
	http.Handle("/mcp", newMCPHandler())

	log.Printf("Starting server on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
