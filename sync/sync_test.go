package sync

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	things "github.com/arthursoares/things-cloud-sdk"
)

func TestOpen(t *testing.T) {
	t.Parallel()

	t.Run("creates new database", func(t *testing.T) {
		t.Parallel()
		dbPath := filepath.Join(t.TempDir(), "test.db")

		syncer, err := Open(dbPath, nil)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer syncer.Close()

		// Verify file was created
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Fatal("Database file was not created")
		}

		// Verify schema was applied by checking tables exist
		var tableName string
		err = syncer.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&tableName)
		if err != nil {
			t.Fatalf("tasks table not created: %v", err)
		}
	})

	t.Run("reopens existing database", func(t *testing.T) {
		t.Parallel()
		dbPath := filepath.Join(t.TempDir(), "test.db")

		// Create and close
		syncer1, err := Open(dbPath, nil)
		if err != nil {
			t.Fatalf("First Open failed: %v", err)
		}

		// Insert test data
		_, err = syncer1.db.Exec("INSERT INTO areas (uuid, title) VALUES ('test-uuid', 'Test Area')")
		if err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
		syncer1.Close()

		// Reopen
		syncer2, err := Open(dbPath, nil)
		if err != nil {
			t.Fatalf("Second Open failed: %v", err)
		}
		defer syncer2.Close()

		// Verify data persisted
		var title string
		err = syncer2.db.QueryRow("SELECT title FROM areas WHERE uuid = 'test-uuid'").Scan(&title)
		if err != nil {
			t.Fatalf("Data not persisted: %v", err)
		}
		if title != "Test Area" {
			t.Fatalf("Expected 'Test Area', got %q", title)
		}
	})

	t.Run("migrates v3 cache by forcing a clean resync", func(t *testing.T) {
		t.Parallel()
		dbPath := filepath.Join(t.TempDir(), "test.db")

		syncer, err := Open(dbPath, nil)
		if err != nil {
			t.Fatalf("initial Open failed: %v", err)
		}
		if err := syncer.saveTask(&things.Task{
			UUID:     "task-stale-cache",
			Title:    "Task",
			Type:     things.TaskTypeTask,
			Status:   things.TaskStatusPending,
			Schedule: things.TaskScheduleAnytime,
		}); err != nil {
			t.Fatalf("saveTask failed: %v", err)
		}
		if _, err := syncer.db.Exec(`INSERT INTO areas (uuid, title) VALUES ('area-1', 'Area')`); err != nil {
			t.Fatalf("insert area failed: %v", err)
		}
		if _, err := syncer.db.Exec(`INSERT INTO tags (uuid, title) VALUES ('tag-1', 'Tag')`); err != nil {
			t.Fatalf("insert tag failed: %v", err)
		}
		if _, err := syncer.db.Exec(`INSERT INTO checklist_items (uuid, title) VALUES ('check-1', 'Item')`); err != nil {
			t.Fatalf("insert checklist item failed: %v", err)
		}
		if _, err := syncer.db.Exec(`INSERT INTO task_tags (task_uuid, tag_uuid) VALUES ('task-stale-cache', 'tag-1')`); err != nil {
			t.Fatalf("insert task_tags failed: %v", err)
		}
		if _, err := syncer.db.Exec(`INSERT INTO area_tags (area_uuid, tag_uuid) VALUES ('area-1', 'tag-1')`); err != nil {
			t.Fatalf("insert area_tags failed: %v", err)
		}
		if _, err := syncer.db.Exec(`INSERT INTO change_log (server_index, synced_at, change_type, entity_type, entity_uuid, payload) VALUES (7, 1, 'TaskCreated', 'task', 'task-stale-cache', '{}')`); err != nil {
			t.Fatalf("insert change_log failed: %v", err)
		}
		if err := syncer.saveSyncState("history-1", 7); err != nil {
			t.Fatalf("saveSyncState failed: %v", err)
		}
		if _, err := syncer.db.Exec(`UPDATE schema_version SET version = 3`); err != nil {
			t.Fatalf("downgrade schema version failed: %v", err)
		}
		if err := syncer.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}

		syncer, err = Open(dbPath, nil)
		if err != nil {
			t.Fatalf("migration Open failed: %v", err)
		}
		defer syncer.Close()

		var version int
		if err := syncer.db.QueryRow(`SELECT version FROM schema_version`).Scan(&version); err != nil {
			t.Fatalf("schema version lookup failed: %v", err)
		}
		if version != schemaVersion {
			t.Fatalf("expected schema version %d, got %d", schemaVersion, version)
		}

		for _, table := range []string{"tasks", "areas", "tags", "checklist_items", "task_tags", "area_tags", "change_log"} {
			var count int
			if err := syncer.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
				t.Fatalf("count %s failed: %v", table, err)
			}
			if count != 0 {
				t.Fatalf("expected %s to be cleared, got %d rows", table, count)
			}
		}

		var historyID string
		var serverIndex int
		err = syncer.db.QueryRow(`SELECT history_id, server_index FROM sync_state WHERE id = 1`).Scan(&historyID, &serverIndex)
		if err != sql.ErrNoRows {
			t.Fatalf("expected sync_state to be cleared, got history_id=%q server_index=%d err=%v", historyID, serverIndex, err)
		}
	})
}

func TestReadQueriesWaitForSyncLock(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	if err := syncer.saveTask(&things.Task{
		UUID:     "task-1",
		Title:    "Task 1",
		Type:     things.TaskTypeTask,
		Status:   things.TaskStatusPending,
		Schedule: things.TaskScheduleAnytime,
	}); err != nil {
		t.Fatalf("saveTask failed: %v", err)
	}

	state := syncer.State()
	syncer.mu.Lock()

	taskDone := make(chan error, 1)
	go func() {
		_, err := state.Task("task-1")
		taskDone <- err
	}()

	changeDone := make(chan error, 1)
	go func() {
		_, err := syncer.ChangesSinceIndex(-1)
		changeDone <- err
	}()

	select {
	case err := <-taskDone:
		t.Fatalf("State().Task should block while sync lock is held, returned early with %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	select {
	case err := <-changeDone:
		t.Fatalf("ChangesSinceIndex should block while sync lock is held, returned early with %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	syncer.mu.Unlock()

	select {
	case err := <-taskDone:
		if err != nil {
			t.Fatalf("State().Task failed after lock release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("State().Task did not resume after lock release")
	}

	select {
	case err := <-changeDone:
		if err != nil {
			t.Fatalf("ChangesSinceIndex failed after lock release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ChangesSinceIndex did not resume after lock release")
	}
}

func TestRawChangesSinceID(t *testing.T) {
	t.Parallel()

	syncer, err := Open(":memory:", nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	change := TaskCreated{
		taskChange: taskChange{
			baseChange: baseChange{serverIndex: 100, timestamp: time.Now()},
			Task:       &things.Task{UUID: "task-uuid-1", Title: "T"},
		},
	}
	if err := syncer.logChange(100, change, `{"tt":1,"e":{"tp":0,"tt":"T"}}`); err != nil {
		t.Fatalf("logChange failed: %v", err)
	}
	if err := syncer.logChange(101, change, `{"tt":1,"e":{"tp":0,"tt":"T2"}}`); err != nil {
		t.Fatalf("logChange failed: %v", err)
	}

	rows, err := syncer.RawChangesSinceID(0, 100)
	if err != nil {
		t.Fatalf("RawChangesSinceID failed: %v", err)
	}
	if got, want := len(rows), 2; got != want {
		t.Fatalf("len(rows) = %d, want %d", got, want)
	}
	if rows[0].ID == 0 {
		t.Fatal("rows[0].ID should be the AUTOINCREMENT id, got 0")
	}
	if rows[0].ID >= rows[1].ID {
		t.Fatalf("rows must be ordered by id ASC: rows[0].ID=%d rows[1].ID=%d", rows[0].ID, rows[1].ID)
	}
	if got, want := rows[0].ChangeType, "TaskCreated"; got != want {
		t.Fatalf("rows[0].ChangeType = %q, want %q", got, want)
	}
	if got, want := rows[0].EntityType, "Task"; got != want {
		t.Fatalf("rows[0].EntityType = %q, want %q", got, want)
	}
	if got, want := rows[0].EntityUUID, "task-uuid-1"; got != want {
		t.Fatalf("rows[0].EntityUUID = %q, want %q", got, want)
	}
	if rows[0].Payload == "" {
		t.Fatal("rows[0].Payload should not be empty")
	}

	limited, err := syncer.RawChangesSinceID(0, 1)
	if err != nil {
		t.Fatalf("RawChangesSinceID with limit failed: %v", err)
	}
	if got, want := len(limited), 1; got != want {
		t.Fatalf("limited len(rows) = %d, want %d", got, want)
	}

	exclusive, err := syncer.RawChangesSinceID(rows[0].ID, 100)
	if err != nil {
		t.Fatalf("RawChangesSinceID (exclusive) failed: %v", err)
	}
	if got, want := len(exclusive), 1; got != want {
		t.Fatalf("exclusive len(rows) = %d, want %d", got, want)
	}
	if got, want := exclusive[0].ID, rows[1].ID; got != want {
		t.Fatalf("exclusive[0].ID = %d, want %d", got, want)
	}
}
