package main

import (
	"encoding/json"
	"net/http"
	"time"
)

const maxCalendarWindow = 90 * 24 * time.Hour

// CalendarEvent is the wire shape for a single calendar event.
// PrepNeeded and PrepNote from dailyCalendarEvent are intentionally omitted.
type CalendarEvent struct {
	Title    string `json:"title"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Calendar string `json:"calendar"`
	Location string `json:"location"`
}

// CalendarEventsResponse is the JSON response body for GET /api/calendar/events.
type CalendarEventsResponse struct {
	Events   []CalendarEvent `json:"events"`
	Warnings []string        `json:"warnings"`
}

func handleCalendarEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sinceStr := q.Get("since")
	untilStr := q.Get("until")
	if sinceStr == "" || untilStr == "" {
		jsonError(w, "since and until are required ISO 8601 timestamps", http.StatusBadRequest)
		return
	}
	since, err := time.Parse(time.RFC3339, sinceStr)
	if err != nil {
		jsonError(w, "since must be RFC 3339: "+err.Error(), http.StatusBadRequest)
		return
	}
	until, err := time.Parse(time.RFC3339, untilStr)
	if err != nil {
		jsonError(w, "until must be RFC 3339: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !until.After(since) {
		jsonError(w, "until must be after since", http.StatusBadRequest)
		return
	}
	if until.Sub(since) > maxCalendarWindow {
		jsonError(w, "window must be <= 90 days", http.StatusBadRequest)
		return
	}

	resp := CalendarEventsResponse{Events: []CalendarEvent{}, Warnings: []string{}}

	feeds := resolveICSCalendarFeeds()
	if len(feeds) == 0 {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	provider := newICSCalendarProvider(feeds)
	events, warnings := provider.eventsInRange(r.Context(), since, until)
	resp.Warnings = warnings
	for _, e := range events {
		resp.Events = append(resp.Events, CalendarEvent{
			Title:    e.Title,
			Start:    e.Start,
			End:      e.End,
			Calendar: e.Calendar,
			Location: e.Location,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
