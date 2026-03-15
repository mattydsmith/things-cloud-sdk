package main

import (
	"errors"
	"testing"

	"github.com/arthursoares/things-cloud-sdk/sync"
)

type stubInitialSyncer struct {
	changes []sync.Change
	err     error
}

func (s stubInitialSyncer) Sync() ([]sync.Change, error) {
	return s.changes, s.err
}

func TestRunInitialSync(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		if err := runInitialSync(stubInitialSyncer{changes: []sync.Change{}}); err != nil {
			t.Fatalf("runInitialSync returned error: %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()

		err := runInitialSync(stubInitialSyncer{err: errors.New("boom")})
		if err == nil {
			t.Fatal("expected error")
		}
		if got, want := err.Error(), "initial sync failed: boom"; got != want {
			t.Fatalf("runInitialSync error = %q, want %q", got, want)
		}
	})
}
