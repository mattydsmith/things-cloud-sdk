package sync

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	things "github.com/arthursoares/things-cloud-sdk"
)

func TestIntegration(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	t.Run("process task creation", func(t *testing.T) {
		payload := things.TaskActionItemPayload{}
		title := "Buy groceries"
		payload.Title = &title
		tp := things.TaskTypeTask
		payload.Type = &tp

		payloadBytes, _ := json.Marshal(payload)
		item := things.Item{
			UUID:   "task-001",
			Kind:   things.ItemKindTask,
			Action: things.ItemActionCreated,
			P:      payloadBytes,
		}

		changes, err := syncer.processItems([]things.Item{item}, 0)
		if err != nil {
			t.Fatalf("processItems failed: %v", err)
		}

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}

		created, ok := changes[0].(TaskCreated)
		if !ok {
			t.Fatalf("expected TaskCreated, got %T", changes[0])
		}
		if created.Task.Title != "Buy groceries" {
			t.Errorf("expected title 'Buy groceries', got %q", created.Task.Title)
		}

		// Verify task was persisted
		state := syncer.State()
		task, err := state.Task("task-001")
		if err != nil {
			t.Fatalf("Task lookup failed: %v", err)
		}
		if task == nil {
			t.Fatal("task not persisted")
		}
	})

	t.Run("process task completion", func(t *testing.T) {
		payload := things.TaskActionItemPayload{}
		status := things.TaskStatusCompleted
		payload.Status = &status

		payloadBytes, _ := json.Marshal(payload)
		item := things.Item{
			UUID:   "task-001",
			Kind:   things.ItemKindTask,
			Action: things.ItemActionModified,
			P:      payloadBytes,
		}

		changes, err := syncer.processItems([]things.Item{item}, 1)
		if err != nil {
			t.Fatalf("processItems failed: %v", err)
		}

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}

		_, ok := changes[0].(TaskCompleted)
		if !ok {
			t.Fatalf("expected TaskCompleted, got %T", changes[0])
		}
	})

	t.Run("process task with tir stores TodayIndexReference separately", func(t *testing.T) {
		// Create a task with a scheduled date (sr)
		payload := things.TaskActionItemPayload{}
		title := "TIR test task"
		payload.Title = &title
		tp := things.TaskTypeTask
		payload.Type = &tp
		sched := things.TaskScheduleAnytime
		payload.Schedule = &sched
		srDate := things.Timestamp(time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC))
		payload.ScheduledDate = &srDate

		payloadBytes, _ := json.Marshal(payload)
		item := things.Item{
			UUID:   "task-tir",
			Kind:   things.ItemKindTask,
			Action: things.ItemActionCreated,
			P:      payloadBytes,
		}

		_, err := syncer.processItems([]things.Item{item}, 10)
		if err != nil {
			t.Fatalf("processItems (create) failed: %v", err)
		}

		// Now modify the task to set tir (simulating drag to Today in the app)
		modPayload := things.TaskActionItemPayload{}
		tirDate := things.Timestamp(time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC))
		modPayload.TaskIR = &tirDate

		modBytes, _ := json.Marshal(modPayload)
		modItem := things.Item{
			UUID:   "task-tir",
			Kind:   things.ItemKindTask,
			Action: things.ItemActionModified,
			P:      modBytes,
		}

		_, err = syncer.processItems([]things.Item{modItem}, 11)
		if err != nil {
			t.Fatalf("processItems (modify tir) failed: %v", err)
		}

		// Verify both fields are stored independently
		task, err := syncer.State().Task("task-tir")
		if err != nil {
			t.Fatalf("Task lookup failed: %v", err)
		}
		if task == nil {
			t.Fatal("task not found")
		}
		if task.ScheduledDate == nil {
			t.Fatal("ScheduledDate should still be set (sr)")
		}
		if task.ScheduledDate.Day() != 10 {
			t.Errorf("ScheduledDate should be March 10, got %v", task.ScheduledDate)
		}
		if task.TodayIndexReference == nil {
			t.Fatal("TodayIndexReference should be set (tir)")
		}
		if task.TodayIndexReference.Day() != 12 {
			t.Errorf("TodayIndexReference should be March 12, got %v", task.TodayIndexReference)
		}
	})

	t.Run("process area creation", func(t *testing.T) {
		payload := things.AreaActionItemPayload{}
		title := "Work"
		payload.Title = &title

		payloadBytes, _ := json.Marshal(payload)
		item := things.Item{
			UUID:   "area-001",
			Kind:   things.ItemKindArea3,
			Action: things.ItemActionCreated,
			P:      payloadBytes,
		}

		changes, err := syncer.processItems([]things.Item{item}, 2)
		if err != nil {
			t.Fatalf("processItems failed: %v", err)
		}

		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(changes))
		}

		_, ok := changes[0].(AreaCreated)
		if !ok {
			t.Fatalf("expected AreaCreated, got %T", changes[0])
		}

		state := syncer.State()
		areas, _ := state.AllAreas()
		if len(areas) != 1 {
			t.Errorf("expected 1 area, got %d", len(areas))
		}
	})

	t.Run("query change log", func(t *testing.T) {
		// ChangesSinceIndex(0) returns changes with server_index > 0
		// So we should get 2 changes (indexes 1 and 2)
		changes, err := syncer.ChangesSinceIndex(0)
		if err != nil {
			t.Fatalf("ChangesSinceIndex failed: %v", err)
		}

		if len(changes) != 2 {
			t.Errorf("expected 2 changes (indexes 1 and 2), got %d", len(changes))
		}

		// Test ChangesSinceIndex(-1) to get all changes
		allChanges, err := syncer.ChangesSinceIndex(-1)
		if err != nil {
			t.Fatalf("ChangesSinceIndex failed: %v", err)
		}

		if len(allChanges) != 3 {
			t.Errorf("expected 3 total changes, got %d", len(allChanges))
		}
	})
}

func TestTasksInTodayWithTIR(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	nowUTC := time.Now().UTC()
	today := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 12, 0, 0, 0, time.UTC)
	yesterday := today.Add(-24 * time.Hour)

	// Task with only sr (scheduled_date) set to today — classic case
	syncer.saveTask(&things.Task{
		UUID: "sr-only", Title: "SR Only Today", Status: things.TaskStatusPending,
		Schedule: things.TaskScheduleAnytime, ScheduledDate: &today,
	})

	// Task with only tir (today_index_ref) set to today — dragged into Today via app
	syncer.saveTask(&things.Task{
		UUID: "tir-only", Title: "TIR Only Today", Status: things.TaskStatusPending,
		Schedule: things.TaskScheduleAnytime, TodayIndexReference: &today,
	})

	// Task with both sr and tir set to today
	syncer.saveTask(&things.Task{
		UUID: "both-today", Title: "Both Today", Status: things.TaskStatusPending,
		Schedule: things.TaskScheduleAnytime, ScheduledDate: &today, TodayIndexReference: &today,
	})

	// Task with sr=yesterday, tir=today — tir wins, should appear in Today
	syncer.saveTask(&things.Task{
		UUID: "sr-old-tir-today", Title: "SR Old TIR Today", Status: things.TaskStatusPending,
		Schedule: things.TaskScheduleAnytime, ScheduledDate: &yesterday, TodayIndexReference: &today,
	})

	// Task with sr=today, tir=yesterday — sr wins, should appear in Today
	syncer.saveTask(&things.Task{
		UUID: "sr-today-tir-old", Title: "SR Today TIR Old", Status: things.TaskStatusPending,
		Schedule: things.TaskScheduleAnytime, ScheduledDate: &today, TodayIndexReference: &yesterday,
	})

	// Task with both dates in the past — should NOT appear in Today
	syncer.saveTask(&things.Task{
		UUID: "both-old", Title: "Both Old", Status: things.TaskStatusPending,
		Schedule: things.TaskScheduleAnytime, ScheduledDate: &yesterday, TodayIndexReference: &yesterday,
	})

	// Task with neither date set — should NOT appear in Today
	syncer.saveTask(&things.Task{
		UUID: "no-dates", Title: "No Dates", Status: things.TaskStatusPending,
		Schedule: things.TaskScheduleAnytime,
	})

	// Task in Inbox schedule — should NOT appear in Today even with tir set
	syncer.saveTask(&things.Task{
		UUID: "inbox-with-tir", Title: "Inbox TIR", Status: things.TaskStatusPending,
		Schedule: things.TaskScheduleInbox, TodayIndexReference: &today,
	})

	state := syncer.State()
	tasks, err := state.TasksInToday(QueryOpts{})
	if err != nil {
		t.Fatalf("TasksInToday failed: %v", err)
	}

	got := make(map[string]bool)
	for _, task := range tasks {
		got[task.UUID] = true
	}

	expected := []string{"sr-only", "tir-only", "both-today", "sr-old-tir-today", "sr-today-tir-old"}
	for _, uuid := range expected {
		if !got[uuid] {
			t.Errorf("expected task %q in Today, but not found", uuid)
		}
	}

	notExpected := []string{"both-old", "no-dates", "inbox-with-tir"}
	for _, uuid := range notExpected {
		if got[uuid] {
			t.Errorf("task %q should NOT be in Today", uuid)
		}
	}

	if len(tasks) != len(expected) {
		t.Errorf("expected %d tasks in Today, got %d", len(expected), len(tasks))
	}
}

func TestStateQueries(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	// Create test data directly
	completedAtRecent := time.Date(2026, 3, 5, 10, 30, 0, 0, time.UTC)
	completedAtOlder := time.Date(2026, 3, 4, 8, 15, 0, 0, time.UTC)
	syncer.saveTask(&things.Task{UUID: "inbox-1", Title: "Inbox Task", Schedule: things.TaskScheduleInbox, Status: things.TaskStatusPending})
	syncer.saveTask(&things.Task{UUID: "anytime-1", Title: "Anytime Task", Schedule: things.TaskScheduleAnytime, Status: things.TaskStatusPending})
	syncer.saveTask(&things.Task{UUID: "completed-1", Title: "Completed Task", Schedule: things.TaskScheduleAnytime, Status: things.TaskStatusCompleted, CompletionDate: &completedAtRecent})
	syncer.saveTask(&things.Task{UUID: "completed-2", Title: "Older Completed Task", Schedule: things.TaskScheduleAnytime, Status: things.TaskStatusCompleted, CompletionDate: &completedAtOlder})
	syncer.saveTask(&things.Task{UUID: "trashed-1", Title: "Trashed Task", InTrash: true})
	syncer.saveTask(&things.Task{UUID: "project-1", Title: "Test Project", Type: things.TaskTypeProject})

	state := syncer.State()

	t.Run("TasksInInbox excludes completed by default", func(t *testing.T) {
		tasks, err := state.TasksInInbox(QueryOpts{})
		if err != nil {
			t.Fatalf("TasksInInbox failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Errorf("expected 1 inbox task, got %d", len(tasks))
		}
	})

	t.Run("AllTasks excludes trashed by default", func(t *testing.T) {
		tasks, err := state.AllTasks(QueryOpts{})
		if err != nil {
			t.Fatalf("AllTasks failed: %v", err)
		}
		for _, task := range tasks {
			if task.InTrash {
				t.Error("trashed task should be excluded")
			}
		}
	})

	t.Run("AllTasks includes trashed when requested", func(t *testing.T) {
		tasks, err := state.AllTasks(QueryOpts{IncludeTrashed: true})
		if err != nil {
			t.Fatalf("AllTasks failed: %v", err)
		}
		found := false
		for _, task := range tasks {
			if task.UUID == "trashed-1" {
				found = true
				break
			}
		}
		if !found {
			t.Error("trashed task should be included")
		}
	})

	t.Run("AllProjects returns only projects", func(t *testing.T) {
		projects, err := state.AllProjects(QueryOpts{})
		if err != nil {
			t.Fatalf("AllProjects failed: %v", err)
		}
		if len(projects) != 1 {
			t.Errorf("expected 1 project, got %d", len(projects))
		}
		if projects[0].Type != things.TaskTypeProject {
			t.Error("returned task is not a project")
		}
	})

	t.Run("CompletedTasksInRange filters and orders by completion date", func(t *testing.T) {
		after := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
		before := time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)

		tasks, err := state.CompletedTasksInRange(10, &after, &before)
		if err != nil {
			t.Fatalf("CompletedTasksInRange failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task in date range, got %d", len(tasks))
		}
		if tasks[0].UUID != "completed-1" {
			t.Fatalf("expected completed-1, got %s", tasks[0].UUID)
		}

		allCompleted, err := state.CompletedTasksInRange(10, nil, nil)
		if err != nil {
			t.Fatalf("CompletedTasksInRange(all) failed: %v", err)
		}
		if len(allCompleted) < 2 {
			t.Fatalf("expected at least 2 completed tasks, got %d", len(allCompleted))
		}
		if allCompleted[0].UUID != "completed-1" || allCompleted[1].UUID != "completed-2" {
			t.Fatalf("expected completed tasks ordered by completion date desc, got %s then %s", allCompleted[0].UUID, allCompleted[1].UUID)
		}
	})
}
