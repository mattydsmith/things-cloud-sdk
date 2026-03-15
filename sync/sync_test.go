package sync

import (
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
