package main

import (
	"testing"
	"time"
)

func TestParseDateOrRFC3339(t *testing.T) {
	t.Run("parses yyyy-mm-dd as utc midnight", func(t *testing.T) {
		got, err := parseDateOrRFC3339("2026-03-05", "completed_after")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
		if got == nil || !got.Equal(want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
	})

	t.Run("parses rfc3339 and normalizes to utc", func(t *testing.T) {
		got, err := parseDateOrRFC3339("2026-03-05T10:30:00+01:00", "completed_before")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(2026, 3, 5, 9, 30, 0, 0, time.UTC)
		if got == nil || !got.Equal(want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
	})

	t.Run("empty value returns nil", func(t *testing.T) {
		got, err := parseDateOrRFC3339("", "completed_after")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("invalid value returns error", func(t *testing.T) {
		if _, err := parseDateOrRFC3339("tomorrow-ish", "completed_after"); err == nil {
			t.Fatal("expected error for invalid date format")
		}
	})
}
