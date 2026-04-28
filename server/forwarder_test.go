package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arthursoares/things-cloud-sdk/sync"
)

func setupForwarderTestSyncer(t *testing.T) *sync.Syncer {
	t.Helper()
	dir := t.TempDir()
	s, err := sync.Open(filepath.Join(dir, "things.db"), nil)
	if err != nil {
		t.Fatalf("sync.Open failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestForwardOnce_PostsRowsAndAdvancesCursor(t *testing.T) {
	syncer := setupForwarderTestSyncer(t)

	if err := syncer.LogChangeForTest(100, "TaskCreated", "Task", "uuid-1", `{"tt":1}`); err != nil {
		t.Fatalf("LogChangeForTest failed: %v", err)
	}
	if err := syncer.LogChangeForTest(101, "TaskCompleted", "Task", "uuid-1", `{"tt":1}`); err != nil {
		t.Fatalf("LogChangeForTest failed: %v", err)
	}

	var posted []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer test-token"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		posted = append(posted, m)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-uuid"}`))
	}))
	defer srv.Close()

	target := ForwardTarget{URL: srv.URL, AuthToken: "test-token", BatchSize: 100}

	report, err := forwardOnce(context.Background(), syncer, target)
	if err != nil {
		t.Fatalf("forwardOnce failed: %v", err)
	}
	if got, want := report.Forwarded, 2; got != want {
		t.Fatalf("Forwarded = %d, want %d", got, want)
	}
	if got, want := len(posted), 2; got != want {
		t.Fatalf("len(posted) = %d, want %d", got, want)
	}
	if got, want := posted[0]["source"], "things"; got != want {
		t.Fatalf("posted[0].source = %v, want %v", got, want)
	}
	if got, want := posted[0]["kind"], "TaskCreated"; got != want {
		t.Fatalf("posted[0].kind = %v, want %v", got, want)
	}
	payload0 := posted[0]["payload"].(map[string]any)
	if got, want := payload0["entityType"], "Task"; got != want {
		t.Fatalf("payload.entityType = %v, want %v", got, want)
	}
	if got, want := payload0["entityUUID"], "uuid-1"; got != want {
		t.Fatalf("payload.entityUUID = %v, want %v", got, want)
	}

	cursor, err := syncer.GetForwardCursor()
	if err != nil {
		t.Fatalf("GetForwardCursor failed: %v", err)
	}
	if cursor < 2 {
		t.Fatalf("cursor = %d, want >= 2", cursor)
	}
}

func TestForwardOnce_NoOpWhenCaughtUp(t *testing.T) {
	syncer := setupForwarderTestSyncer(t)

	var posted int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posted, 1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	report, err := forwardOnce(context.Background(), syncer, ForwardTarget{
		URL: srv.URL, AuthToken: "t", BatchSize: 100,
	})
	if err != nil {
		t.Fatalf("forwardOnce failed: %v", err)
	}
	if report.Forwarded != 0 {
		t.Fatalf("Forwarded = %d, want 0", report.Forwarded)
	}
	if atomic.LoadInt32(&posted) != 0 {
		t.Fatal("expected zero POSTs when no rows past cursor")
	}
}

func TestForwardOnce_HaltsOnPostFailure(t *testing.T) {
	syncer := setupForwarderTestSyncer(t)

	for i := 0; i < 3; i++ {
		if err := syncer.LogChangeForTest(100+i, "TaskCreated", "Task", "u", `{}`); err != nil {
			t.Fatal(err)
		}
	}

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	_, err := forwardOnce(context.Background(), syncer, ForwardTarget{
		URL: srv.URL, AuthToken: "t", BatchSize: 100,
	})
	if err == nil {
		t.Fatal("expected error from forwardOnce; got nil")
	}

	cursor, _ := syncer.GetForwardCursor()
	if cursor != 1 {
		t.Fatalf("cursor = %d, want 1 (advanced for the 1 successful POST, halted before the failing one)", cursor)
	}
}

func TestForwardOnce_PaginatesBatches(t *testing.T) {
	syncer := setupForwarderTestSyncer(t)

	const total = 7
	for i := 0; i < total; i++ {
		if err := syncer.LogChangeForTest(100+i, "TaskCreated", "Task", "u", `{}`); err != nil {
			t.Fatal(err)
		}
	}

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	report, err := forwardOnce(context.Background(), syncer, ForwardTarget{
		URL: srv.URL, AuthToken: "t", BatchSize: 3,
	})
	if err != nil {
		t.Fatalf("forwardOnce failed: %v", err)
	}
	if got, want := report.Forwarded, total; got != want {
		t.Fatalf("Forwarded = %d, want %d", got, want)
	}
	if got, want := atomic.LoadInt32(&calls), int32(total); got != want {
		t.Fatalf("POSTs = %d, want %d", got, want)
	}
}

func TestForwardOnce_RespectsContextCancellation(t *testing.T) {
	syncer := setupForwarderTestSyncer(t)
	for i := 0; i < 5; i++ {
		_ = syncer.LogChangeForTest(100+i, "TaskCreated", "Task", "u", `{}`)
	}

	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel() // cancel after the first POST starts
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	_, err := forwardOnce(ctx, syncer, ForwardTarget{URL: srv.URL, AuthToken: "t", BatchSize: 10})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// Ensure RunForwardLoop is importable / compiles (smoke test only; no goroutine leak).
var _ = func() {
	_ = RunForwardLoop
	_ = time.Second // prevent unused import if test file is the only importer
}
