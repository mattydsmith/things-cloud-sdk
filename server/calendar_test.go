package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// canned ICS with one VEVENT on 2026-04-28 09:00–09:30 UTC
const testICS = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//
BEGIN:VEVENT
UID:test-event-1
SUMMARY:Test Meeting
DTSTART:20260428T090000Z
DTEND:20260428T093000Z
LOCATION:Zoom
END:VEVENT
END:VCALENDAR
`

// clearCalendarEnv wipes all CALENDAR_N env vars so each test starts clean.
func clearCalendarEnv(t *testing.T) {
	t.Helper()
	for i := 1; i <= 10; i++ {
		t.Setenv(fmt.Sprintf("CALENDAR_%d_ICS_URL", i), "")
		t.Setenv(fmt.Sprintf("CALENDAR_%d_NAME", i), "")
	}
}

func doGet(t *testing.T, since, until string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/calendar/events"
	if since != "" || until != "" {
		path += "?since=" + since + "&until=" + until
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	handleCalendarEvents(w, req)
	return w
}

func TestCalendarEvents_MissingSinceUntil(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/calendar/events", nil)
	w := httptest.NewRecorder()
	handleCalendarEvents(w, req)
	if got, want := w.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] == "" {
		t.Fatal("expected non-empty error field")
	}
}

func TestCalendarEvents_BadSince(t *testing.T) {
	w := doGet(t, "not-a-date", "2026-04-28T10:00:00Z")
	if got, want := w.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCalendarEvents_UntilNotAfterSince(t *testing.T) {
	// until == since
	w := doGet(t, "2026-04-28T09:00:00Z", "2026-04-28T09:00:00Z")
	if got, want := w.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCalendarEvents_WindowOver90Days(t *testing.T) {
	w := doGet(t, "2026-01-01T00:00:00Z", "2026-04-10T00:00:00Z") // 99 days
	if got, want := w.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCalendarEvents_NoFeeds(t *testing.T) {
	clearCalendarEnv(t)
	w := doGet(t, "2026-04-28T00:00:00Z", "2026-04-29T00:00:00Z")
	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var resp CalendarEventsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(resp.Events))
	}
	if len(resp.Warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d", len(resp.Warnings))
	}
}

func TestCalendarEvents_ReturnsEventsFromFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		fmt.Fprint(w, testICS)
	}))
	defer srv.Close()

	clearCalendarEnv(t)
	t.Setenv("CALENDAR_1_ICS_URL", srv.URL+"/cal.ics")
	t.Setenv("CALENDAR_1_NAME", "Work")

	w := doGet(t, "2026-04-28T00:00:00Z", "2026-04-29T00:00:00Z")
	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body: %s", got, want, w.Body.String())
	}
	var resp CalendarEventsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Events) == 0 {
		t.Fatal("expected at least one event, got 0")
	}
	if got, want := resp.Events[0].Title, "Test Meeting"; got != want {
		t.Fatalf("Events[0].Title = %q, want %q", got, want)
	}
	if resp.Events[0].Start == "" {
		t.Fatal("Events[0].Start should not be empty")
	}
}

func TestCalendarEvents_WarningsOnFeed500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	clearCalendarEnv(t)
	t.Setenv("CALENDAR_1_ICS_URL", srv.URL+"/cal.ics")
	t.Setenv("CALENDAR_1_NAME", "BadFeed")

	w := doGet(t, "2026-04-28T00:00:00Z", "2026-04-29T00:00:00Z")
	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var resp CalendarEventsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(resp.Events))
	}
	if len(resp.Warnings) == 0 {
		t.Fatal("expected at least one warning for 500 feed")
	}
}

func TestCalendarEvents_NoPrepFieldsInJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		fmt.Fprint(w, testICS)
	}))
	defer srv.Close()

	clearCalendarEnv(t)
	t.Setenv("CALENDAR_1_ICS_URL", srv.URL+"/cal.ics")

	w := doGet(t, "2026-04-28T00:00:00Z", "2026-04-29T00:00:00Z")
	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	// Decode into raw map to check key names
	var raw map[string]any
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	events, ok := raw["events"].([]any)
	if !ok || len(events) == 0 {
		// No events means nothing to check — skip prep check if feed yielded nothing
		return
	}
	for i, ev := range events {
		m, ok := ev.(map[string]any)
		if !ok {
			t.Fatalf("events[%d] is not an object", i)
		}
		if _, found := m["prep_needed"]; found {
			t.Fatalf("events[%d] contains prep_needed, expected it stripped", i)
		}
		if _, found := m["prep_note"]; found {
			t.Fatalf("events[%d] contains prep_note, expected it stripped", i)
		}
	}
}
