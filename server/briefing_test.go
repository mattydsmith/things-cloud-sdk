package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	things "github.com/arthursoares/things-cloud-sdk"
)

func TestBriefingStoreWriteAndCleanup(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir() + "/briefings.db"
	now := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)
	store := newBriefingStore(dbPath, func() time.Time { return now })

	briefing := &dailyBriefing{
		Type:        "daily",
		Date:        "2026-04-15",
		GeneratedAt: now.Format(time.RFC3339),
		Calendar: dailyCalendar{
			Events:     []dailyCalendarEvent{},
			FreeBlocks: []calendarFreeBlock{},
		},
		TodayTasks: dailyTodayTasks{Tasks: []dailyTodayTask{}},
		Overdue:    []dailyOverdueItem{},
		Inbox:      []dailyInboxSuggestion{},
		CapacityAssessment: dailyCapacityAssessment{
			DeferSuggestions: []dailyDeferSuggestion{},
		},
	}
	db, err := store.openDB()
	if err != nil {
		t.Fatalf("openDB failed: %v", err)
	}
	oldPayload, err := json.Marshal(map[string]string{"type": "daily"})
	if err != nil {
		t.Fatalf("marshal old payload: %v", err)
	}
	oldCreatedAt := now.Add(-31 * 24 * time.Hour).Unix()
	if _, err := db.Exec(`
		INSERT INTO briefings (kind, target_key, generated_at, created_at, payload_json)
		VALUES ('daily', '2026-03-01', '2026-03-01T09:00:00Z', ?, ?)
	`, oldCreatedAt, string(oldPayload)); err != nil {
		t.Fatalf("insert old briefing: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	if err := store.write("daily", briefing.Date, briefing.GeneratedAt, briefing); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	checkDB, err := store.openDB()
	if err != nil {
		t.Fatalf("reopen db failed: %v", err)
	}
	defer checkDB.Close()
	var oldCount int
	if err := checkDB.QueryRow(`SELECT COUNT(*) FROM briefings WHERE target_key = '2026-03-01'`).Scan(&oldCount); err != nil {
		t.Fatalf("count old rows: %v", err)
	}
	if oldCount != 0 {
		t.Fatalf("expected old briefing row to be removed, count = %d", oldCount)
	}
	if _, err := store.readDaily("2026-04-15"); err != nil {
		t.Fatalf("expected fresh daily briefing to be readable: %v", err)
	}
}

func TestReviewTargetForDate(t *testing.T) {
	t.Parallel()

	weekday := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	kind, key := reviewTargetForDate(weekday)
	if kind != "daily" || key != "2026-04-15" {
		t.Fatalf("weekday target = (%q, %q), want (daily, 2026-04-15)", kind, key)
	}

	saturday := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	kind, key = reviewTargetForDate(saturday)
	if kind != "weekly" || key != "2026-W16" {
		t.Fatalf("weekend target = (%q, %q), want (weekly, 2026-W16)", kind, key)
	}
}

func TestParseISOWeek(t *testing.T) {
	t.Parallel()

	got, err := parseISOWeek("2026-W16")
	if err != nil {
		t.Fatalf("parseISOWeek returned error: %v", err)
	}
	if got.Format("2006-01-02") != "2026-04-13" {
		t.Fatalf("week start = %s, want 2026-04-13", got.Format("2006-01-02"))
	}

	if _, err := parseISOWeek("2026-16"); err == nil {
		t.Fatal("expected invalid week format to fail")
	}
}

func TestRolloversForPeriod(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir() + "/briefings.db"
	now := func() time.Time { return time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC) }
	store := newBriefingStore(dbPath, now)
	svc := &briefingService{store: store, calendar: noopCalendarProvider{}, now: now}

	days := map[string][]dailyTodayTask{
		"2026-04-06": {
			{UUID: "task-1", Title: "Rewrite landing page"},
			{UUID: "task-2", Title: "Pay gas bill"},
		},
		"2026-04-07": {
			{UUID: "task-1", Title: "Rewrite landing page"},
		},
		"2026-04-08": {
			{UUID: "task-1", Title: "Rewrite landing page"},
		},
	}

	for date, tasks := range days {
		briefing := &dailyBriefing{
			Type:        "daily",
			Date:        date,
			GeneratedAt: now().Format(time.RFC3339),
			Calendar: dailyCalendar{
				Events:     []dailyCalendarEvent{},
				FreeBlocks: []calendarFreeBlock{},
			},
			TodayTasks: dailyTodayTasks{Tasks: tasks},
			Overdue:    []dailyOverdueItem{},
			Inbox:      []dailyInboxSuggestion{},
			CapacityAssessment: dailyCapacityAssessment{
				DeferSuggestions: []dailyDeferSuggestion{},
			},
		}
		if err := store.write("daily", date, briefing.GeneratedAt, briefing); err != nil {
			t.Fatalf("write daily briefing %s: %v", date, err)
		}
	}

	rollovers := svc.rolloversForPeriod(
		time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
	)
	if len(rollovers) != 1 {
		t.Fatalf("expected one rollover, got %d", len(rollovers))
	}
	if rollovers[0].UUID != "task-1" || rollovers[0].TimesRescheduled != 3 {
		t.Fatalf("unexpected rollover payload: %+v", rollovers[0])
	}
	if rollovers[0].Recommendation != "break_down" {
		t.Fatalf("unexpected recommendation: %+v", rollovers[0])
	}
}

func TestReviewFeedbackStore(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir() + "/briefings.db"
	now := func() time.Time { return time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC) }
	store := newBriefingStore(dbPath, now)

	first, err := store.writeReviewFeedback(reviewFeedbackEntry{
		ReviewType: "daily",
		TargetKey:  "2026-04-15",
		IssueType:  "missing_context",
		Feedback:   "It ignored my calendar load.",
		Source:     "claude",
	})
	if err != nil {
		t.Fatalf("writeReviewFeedback(first) failed: %v", err)
	}
	second, err := store.writeReviewFeedback(reviewFeedbackEntry{
		ReviewType: "weekly",
		TargetKey:  "2026-W16",
		IssueType:  "bad_prioritisation",
		Feedback:   "It over-focused on overdue items.",
		Source:     "claude",
	})
	if err != nil {
		t.Fatalf("writeReviewFeedback(second) failed: %v", err)
	}
	if first.ID == 0 || second.ID == 0 {
		t.Fatalf("expected inserted feedback ids, got %d and %d", first.ID, second.ID)
	}

	all, err := store.listReviewFeedback(10, "")
	if err != nil {
		t.Fatalf("listReviewFeedback(all) failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 feedback entries, got %d", len(all))
	}

	daily, err := store.listReviewFeedback(10, "daily")
	if err != nil {
		t.Fatalf("listReviewFeedback(daily) failed: %v", err)
	}
	if len(daily) != 1 || daily[0].ReviewType != "daily" || daily[0].TargetKey != "2026-04-15" {
		t.Fatalf("unexpected filtered feedback: %+v", daily)
	}
}

func TestBearerOnlyAuthMiddleware(t *testing.T) {
	protected := bearerOnlyAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("requires configured secret", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "")
		t.Setenv("API_KEY", "")

		req := httptest.NewRequest(http.MethodPut, "/briefing/daily/2026-04-15", nil)
		rec := httptest.NewRecorder()
		protected(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
		}
	})

	t.Run("requires bearer auth specifically", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("API_KEY", "")

		req := httptest.NewRequest(http.MethodPut, "/briefing/daily/2026-04-15", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: newSessionCookieValue("test-secret", time.Now())})
		rec := httptest.NewRecorder()
		protected(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
		}
	})

	t.Run("accepts bearer auth", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("API_KEY", "")

		req := httptest.NewRequest(http.MethodPut, "/briefing/daily/2026-04-15", nil)
		req.Header.Set("Authorization", "Bearer test-secret")
		rec := httptest.NewRecorder()
		protected(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
		}
	})
}

func TestShouldExcludeRoutineTitle(t *testing.T) {
	t.Parallel()

	if !shouldExcludeRoutineTitle("Morning Review") {
		t.Fatal("expected Morning Review to be excluded")
	}
	if !shouldExcludeRoutineTitle("Weekly Review") {
		t.Fatal("expected Weekly Review to be excluded")
	}
	if !shouldExcludeRoutineProjectTitle("☀️ Routines") {
		t.Fatal("expected emoji-prefixed Routines project to be excluded")
	}
	if shouldExcludeRoutineProjectTitle("Home & Admin") {
		t.Fatal("did not expect non-routines project to be excluded")
	}
}

func TestShouldSuppressOverdueAsRecentRepeat(t *testing.T) {
	t.Parallel()

	recentlyCompleted := map[string]struct{}{
		recentCompletionKey("Water house plants", ""): {},
	}
	task := &things.Task{Title: "Water house plants"}
	if !shouldSuppressOverdueAsRecentRepeat(nil, task, recentlyCompleted) {
		t.Fatal("expected matching recent completion to suppress overdue repeat")
	}
	if shouldSuppressOverdueAsRecentRepeat(nil, &things.Task{Title: "Pay gas bill"}, recentlyCompleted) {
		t.Fatal("did not expect unmatched task to be suppressed")
	}
}

func TestSuggestTagsUsesNowForThisWeek(t *testing.T) {
	t.Parallel()

	dayStart := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)
	tagByTitle := map[string]*things.Tag{
		"now":      {UUID: "tag-now", Title: "Now"},
		"next":     {UUID: "tag-next", Title: "Next"},
		"work":     {UUID: "tag-work", Title: "Work"},
		"personal": {UUID: "tag-personal", Title: "Personal"},
	}

	tags, uuids := suggestTags("Reply to Emma", "2026-04-15", dayStart, tagByTitle)
	if !containsString(tags, "Now") {
		t.Fatalf("expected Now tag for today suggestion, got %v", tags)
	}
	if containsString(tags, "Next") {
		t.Fatalf("did not expect Next tag for today suggestion, got %v", tags)
	}
	if !containsString(uuids, "tag-now") {
		t.Fatalf("expected now uuid, got %v", uuids)
	}

	tags, _ = suggestTags("Book train", "2026-04-20", dayStart, tagByTitle)
	if containsString(tags, "Now") {
		t.Fatalf("did not expect Now tag for next-week suggestion, got %v", tags)
	}
	if !containsString(tags, "Next") {
		t.Fatalf("expected Next tag for next-week suggestion, got %v", tags)
	}
}

func TestOverdueRecommendationPrefersRewriteForLongOverdue(t *testing.T) {
	t.Parallel()

	if got := overdueRecommendation(1); got != "tackle_today" {
		t.Fatalf("days=1 recommendation = %q, want tackle_today", got)
	}
	if got := overdueRecommendation(3); got != "reschedule" {
		t.Fatalf("days=3 recommendation = %q, want reschedule", got)
	}
	if got := overdueRecommendation(4); got != "rewrite" {
		t.Fatalf("days=4 recommendation = %q, want rewrite", got)
	}
}

func TestShouldUseNowTagForSuggestion(t *testing.T) {
	t.Parallel()

	dayStart := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)
	if !shouldUseNowTagForSuggestion("2026-04-15", dayStart) {
		t.Fatal("expected today suggestion to use Now")
	}
	if !shouldUseNowTagForSuggestion("2026-04-18", dayStart) {
		t.Fatal("expected same-week suggestion to use Now")
	}
	if shouldUseNowTagForSuggestion("2026-04-20", dayStart) {
		t.Fatal("did not expect next-week suggestion to use Now")
	}
}

func TestICSCalendarProviderDaily(t *testing.T) {
	t.Parallel()

	ics := `BEGIN:VCALENDAR
VERSION:2.0
X-WR-CALNAME:Personal
X-WR-TIMEZONE:Europe/London
BEGIN:VEVENT
DTSTART;TZID=Europe/London:20260415T090000
DTEND;TZID=Europe/London:20260415T100000
SUMMARY:Planning
LOCATION:Office
END:VEVENT
BEGIN:VEVENT
DTSTART;TZID=Europe/London:20260415T150000
DTEND;TZID=Europe/London:20260415T160000
SUMMARY:Demo review
END:VEVENT
END:VCALENDAR
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(ics))
	}))
	defer srv.Close()

	provider := newICSCalendarProvider([]icsCalendarFeed{{URL: srv.URL}})
	got, err := provider.Daily(context.Background(), time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Daily() returned error: %v", err)
	}
	if len(got.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got.Events))
	}
	if got.Events[0].Calendar != "Personal" {
		t.Fatalf("calendar = %q, want Personal", got.Events[0].Calendar)
	}
	if got.CalendarCounts["Personal"] != 2 {
		t.Fatalf("calendar count = %d, want 2", got.CalendarCounts["Personal"])
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", got.Warnings)
	}
	if got.Events[0].Title != "Planning" || got.Events[0].Start != "09:00" || got.Events[0].End != "10:00" {
		t.Fatalf("unexpected first event: %+v", got.Events[0])
	}
	if got.Events[1].PrepNeeded != true {
		t.Fatalf("expected demo review event to require prep: %+v", got.Events[1])
	}
	if got.TotalMeetingHours != 2 {
		t.Fatalf("TotalMeetingHours = %v, want 2", got.TotalMeetingHours)
	}
	if got.FocusTimeAvailableHours != 10 {
		t.Fatalf("FocusTimeAvailableHours = %v, want 10", got.FocusTimeAvailableHours)
	}
}

func TestICSCalendarProviderWeeklyRecurringEvent(t *testing.T) {
	t.Parallel()

	ics := `BEGIN:VCALENDAR
VERSION:2.0
X-WR-CALNAME:Modulaire
X-WR-TIMEZONE:Europe/London
BEGIN:VEVENT
DTSTART;TZID=Europe/London:20260408T110000
DTEND;TZID=Europe/London:20260408T120000
RRULE:FREQ=WEEKLY;BYDAY=WE
SUMMARY:Weekly standup
END:VEVENT
END:VCALENDAR
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(ics))
	}))
	defer srv.Close()

	provider := newICSCalendarProvider([]icsCalendarFeed{{URL: srv.URL, Label: "Work"}})
	got, err := provider.Daily(context.Background(), time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Daily() returned error: %v", err)
	}
	if len(got.Events) != 1 {
		t.Fatalf("expected 1 recurring event, got %d", len(got.Events))
	}
	if got.Events[0].Calendar != "Work" {
		t.Fatalf("calendar label override = %q, want Work", got.Events[0].Calendar)
	}
	if got.Events[0].Title != "Weekly standup" || got.Events[0].Start != "11:00" || got.Events[0].End != "12:00" {
		t.Fatalf("unexpected recurring event: %+v", got.Events[0])
	}
}

func TestICSCalendarProviderDoesNotReviveExpiredCountRecurrence(t *testing.T) {
	t.Parallel()

	ics := `BEGIN:VCALENDAR
VERSION:2.0
X-WR-CALNAME:Lump
X-WR-TIMEZONE:Europe/London
BEGIN:VEVENT
DTSTART;TZID=Europe/London:20150311T083000
DTEND;TZID=Europe/London:20150311T084500
RRULE:FREQ=DAILY;COUNT=22
SUMMARY:Stuby Tablet
END:VEVENT
END:VCALENDAR
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(ics))
	}))
	defer srv.Close()

	provider := newICSCalendarProvider([]icsCalendarFeed{{URL: srv.URL, Label: "Lump"}})
	got, err := provider.Daily(context.Background(), time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Daily() returned error: %v", err)
	}
	if len(got.Events) != 0 {
		t.Fatalf("expected expired count recurrence to stay expired, got %+v", got.Events)
	}
	if got.CalendarCounts["Lump"] != 0 {
		t.Fatalf("calendar count = %d, want 0", got.CalendarCounts["Lump"])
	}
}

func TestICSCalendarProviderWarnsOnEmptyCalendar(t *testing.T) {
	t.Parallel()

	ics := `BEGIN:VCALENDAR
VERSION:2.0
X-WR-CALNAME:Lump
X-WR-TIMEZONE:Europe/London
END:VCALENDAR
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(ics))
	}))
	defer srv.Close()

	provider := newICSCalendarProvider([]icsCalendarFeed{{URL: srv.URL}})
	got, err := provider.Daily(context.Background(), time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Daily() returned error: %v", err)
	}
	if got.CalendarCounts["Lump"] != 0 {
		t.Fatalf("calendar count = %d, want 0", got.CalendarCounts["Lump"])
	}
	if len(got.Warnings) != 1 {
		t.Fatalf("expected one warning, got %+v", got.Warnings)
	}
}
