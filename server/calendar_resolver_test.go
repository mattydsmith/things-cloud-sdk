package main

import (
	"fmt"
	"testing"
)

func TestResolveICSCalendarFeeds_Empty(t *testing.T) {
	for i := 1; i <= 10; i++ {
		t.Setenv(fmt.Sprintf("CALENDAR_%d_ICS_URL", i), "")
		t.Setenv(fmt.Sprintf("CALENDAR_%d_NAME", i), "")
	}
	got := resolveICSCalendarFeeds()
	if len(got) != 0 {
		t.Fatalf("expected 0 feeds, got %d", len(got))
	}
}

func TestResolveICSCalendarFeeds_ThreeFeeds(t *testing.T) {
	t.Setenv("CALENDAR_1_ICS_URL", "https://a.example/cal.ics")
	t.Setenv("CALENDAR_1_NAME", "Work")
	t.Setenv("CALENDAR_2_ICS_URL", "https://b.example/cal.ics")
	t.Setenv("CALENDAR_2_NAME", "Personal")
	t.Setenv("CALENDAR_3_ICS_URL", "https://c.example/cal.ics")
	t.Setenv("CALENDAR_3_NAME", "Family")
	got := resolveICSCalendarFeeds()
	if got, want := len(got), 3; got != want {
		t.Fatalf("len(feeds) = %d, want %d", got, want)
	}
	if got[2].URL != "https://c.example/cal.ics" {
		t.Fatalf("got[2].URL = %q, want https://c.example/cal.ics", got[2].URL)
	}
}

func TestResolveICSCalendarFeeds_StopsAtFirstGap(t *testing.T) {
	t.Setenv("CALENDAR_1_ICS_URL", "https://a.example/cal.ics")
	t.Setenv("CALENDAR_2_ICS_URL", "")
	t.Setenv("CALENDAR_3_ICS_URL", "https://c.example/cal.ics")
	got := resolveICSCalendarFeeds()
	if got, want := len(got), 1; got != want {
		t.Fatalf("len(feeds) = %d, want %d (must stop at first empty CALENDAR_N_ICS_URL)", got, want)
	}
}
