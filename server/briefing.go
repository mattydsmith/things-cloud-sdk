package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	things "github.com/arthursoares/things-cloud-sdk"
	"github.com/arthursoares/things-cloud-sdk/sync"
	_ "modernc.org/sqlite"
)

const (
	briefingRetention = 30 * 24 * time.Hour
	thingsDBPath      = "/data/things.db"
)

var briefingWriteMu gosync.Mutex
var errBriefingNotFound = errors.New("briefing not found")

type dailyBriefing struct {
	Type               string                  `json:"type"`
	Date               string                  `json:"date"`
	GeneratedAt        string                  `json:"generated_at"`
	Calendar           dailyCalendar           `json:"calendar"`
	TodayTasks         dailyTodayTasks         `json:"today_tasks"`
	UpcomingLookahead  []dailyUpcomingItem     `json:"upcoming_lookahead"`
	Overdue            []dailyOverdueItem      `json:"overdue"`
	Inbox              []dailyInboxSuggestion  `json:"inbox"`
	CompletedToday     dailyCompletedSummary   `json:"completed_today"`
	NowWithoutDates    []weeklyTaskSummary     `json:"now_without_dates"`
	TagSignals         dailyTagSignals         `json:"tag_signals"`
	CapacityAssessment dailyCapacityAssessment `json:"capacity_assessment"`
}

type dailyCalendar struct {
	Events                  []dailyCalendarEvent `json:"events"`
	CalendarCounts          map[string]int       `json:"calendar_counts"`
	Warnings                []string             `json:"warnings"`
	TotalMeetingHours       float64              `json:"total_meeting_hours"`
	FreeBlocks              []calendarFreeBlock  `json:"free_blocks"`
	FocusTimeAvailableHours float64              `json:"focus_time_available_hours"`
}

type dailyCalendarEvent struct {
	Title      string `json:"title"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Calendar   string `json:"calendar"`
	Location   string `json:"location"`
	PrepNeeded bool   `json:"prep_needed"`
	PrepNote   string `json:"prep_note"`
}

type calendarFreeBlock struct {
	Start   string `json:"start"`
	End     string `json:"end"`
	Minutes int    `json:"minutes"`
}

type dailyTodayTasks struct {
	Tasks              []dailyTodayTask `json:"tasks"`
	DeadlineCountToday int              `json:"deadline_count_today"`
	DeadlineWarning    bool             `json:"deadline_warning"`
}

type dailyTodayTask struct {
	UUID                  string   `json:"uuid"`
	Title                 string   `json:"title"`
	Project               string   `json:"project"`
	Tags                  []string `json:"tags"`
	Deadline              *string  `json:"deadline"`
	ScheduledFor          *string  `json:"scheduled_for"`
	DaysOnToday           int      `json:"days_on_today"`
	Stale                 bool     `json:"stale"`
	StaleReason           string   `json:"stale_reason"`
	ImprovementSuggestion string   `json:"improvement_suggestion"`
	EstimatedEffort       string   `json:"estimated_effort"`
}

type dailyOverdueItem struct {
	UUID           string  `json:"uuid"`
	Title          string  `json:"title"`
	ScheduledFor   *string `json:"scheduled_for"`
	DaysOverdue    int     `json:"days_overdue"`
	Recommendation string  `json:"recommendation"`
}

type dailyUpcomingItem struct {
	UUID         string   `json:"uuid"`
	Title        string   `json:"title"`
	Project      string   `json:"project"`
	Tags         []string `json:"tags"`
	ScheduledFor *string  `json:"scheduled_for"`
	Deadline     *string  `json:"deadline"`
}

type dailyInboxSuggestion struct {
	UUID                 string   `json:"uuid"`
	Title                string   `json:"title"`
	SuggestedProject     string   `json:"suggested_project,omitempty"`
	SuggestedProjectUUID string   `json:"suggested_project_uuid,omitempty"`
	SuggestedTags        []string `json:"suggested_tags"`
	SuggestedTagUUIDs    string   `json:"suggested_tag_uuids,omitempty"`
	SuggestedWhen        string   `json:"suggested_when"`
	Reasoning            string   `json:"reasoning"`
}

type dailyCompletedSummary struct {
	Count int                  `json:"count"`
	Tasks []dailyCompletedTask `json:"tasks"`
}

type dailyCompletedTask struct {
	UUID        string `json:"uuid"`
	Title       string `json:"title"`
	Project     string `json:"project"`
	CompletedAt string `json:"completed_at"`
}

type dailyCapacityAssessment struct {
	TotalTasks         int                    `json:"total_tasks"`
	EstimatedTaskHours float64                `json:"estimated_task_hours"`
	AvailableHours     float64                `json:"available_hours"`
	Overloaded         bool                   `json:"overloaded"`
	DeferSuggestions   []dailyDeferSuggestion `json:"defer_suggestions"`
}

type dailyDeferSuggestion struct {
	UUID             string `json:"uuid"`
	Title            string `json:"title"`
	Reason           string `json:"reason"`
	SuggestedNewDate string `json:"suggested_new_date"`
}

type dailyTagSignals struct {
	DeepTaskCount   int             `json:"deep_task_count"`
	MeetingHeavyDay bool            `json:"meeting_heavy_day"`
	DeepTaskWarning string          `json:"deep_task_warning,omitempty"`
	SuggestedAI     []reviewTaskCue `json:"suggested_ai"`
	RewriteCues     []reviewTaskCue `json:"rewrite_cues"`
}

type weeklyBriefing struct {
	Type                  string               `json:"type"`
	Week                  string               `json:"week"`
	GeneratedAt           string               `json:"generated_at"`
	ReviewPeriod          weeklyReviewPeriod   `json:"review_period"`
	CompletedTasks        weeklyCompletedTasks `json:"completed_tasks"`
	Rollovers             []weeklyRollover     `json:"rollovers"`
	StaleProjects         []weeklyStaleProject `json:"stale_projects"`
	ProjectHealth         weeklyProjectHealth  `json:"project_health"`
	UpcomingWeek          weeklyUpcomingWeek   `json:"upcoming_week"`
	NowTaggedWithoutDates []weeklyTaskSummary  `json:"now_tagged_without_dates"`
	TagAudit              weeklyTagAudit       `json:"tag_audit"`
	InboxBacklog          int                  `json:"inbox_backlog"`
}

type weeklyReviewPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type weeklyCompletedTasks struct {
	Count      int            `json:"count"`
	ByProject  map[string]int `json:"by_project"`
	Highlights []string       `json:"highlights"`
}

type weeklyRollover struct {
	UUID             string `json:"uuid"`
	Title            string `json:"title"`
	TimesRescheduled int    `json:"times_rescheduled"`
	Recommendation   string `json:"recommendation"`
}

type weeklyStaleProject struct {
	Project        string `json:"project"`
	LastCompletion string `json:"last_completion,omitempty"`
	OpenTasks      int    `json:"open_tasks"`
	Note           string `json:"note"`
}

type weeklyUpcomingWeek struct {
	CalendarSummary map[string]any `json:"calendar_summary"`
	DeadlineDays    map[string]int `json:"deadline_days"`
	HeaviestDay     string         `json:"heaviest_day"`
}

type weeklyProjectHealth struct {
	ProjectsWithoutNextAction []weeklyProjectStatus `json:"projects_without_next_action"`
}

type weeklyProjectStatus struct {
	Project       string `json:"project"`
	ProjectUUID   string `json:"project_uuid"`
	OpenTaskCount int    `json:"open_task_count"`
	Note          string `json:"note"`
}

type weeklyTaskSummary struct {
	UUID    string `json:"uuid"`
	Title   string `json:"title"`
	Project string `json:"project"`
}

type reviewTaskCue struct {
	UUID    string `json:"uuid"`
	Title   string `json:"title"`
	Project string `json:"project"`
	Reason  string `json:"reason,omitempty"`
}

type weeklyTagAudit struct {
	MissingHorizonTags     []weeklyTaskSummary `json:"missing_horizon_tags"`
	ConflictingHorizonTags []weeklyTagIssue    `json:"conflicting_horizon_tags"`
	MissingContextTags     []weeklyTaskSummary `json:"missing_context_tags"`
	ConflictingContextTags []weeklyTagIssue    `json:"conflicting_context_tags"`
	SuggestedAI            []reviewTaskCue     `json:"suggested_ai"`
	LaterTasks             []weeklyTaskSummary `json:"later_tasks"`
}

type weeklyTagIssue struct {
	UUID    string   `json:"uuid"`
	Title   string   `json:"title"`
	Project string   `json:"project"`
	Tags    []string `json:"tags"`
	Note    string   `json:"note"`
}

type reviewFeedbackEntry struct {
	ID         int64  `json:"id"`
	CreatedAt  string `json:"created_at"`
	ReviewType string `json:"review_type"`
	TargetKey  string `json:"target_key,omitempty"`
	IssueType  string `json:"issue_type,omitempty"`
	Feedback   string `json:"feedback"`
	Expected   string `json:"expected,omitempty"`
	Actual     string `json:"actual,omitempty"`
	Source     string `json:"source,omitempty"`
}

type briefingStore struct {
	dbPath string
	now    func() time.Time
}

type calendarProvider interface {
	Daily(context.Context, time.Time) (dailyCalendar, error)
	WeeklySummary(context.Context, time.Time, time.Time) (map[string]any, error)
}

type noopCalendarProvider struct{}

type icsCalendarProvider struct {
	client *http.Client
	feeds  []icsCalendarFeed
}

type icsCalendarFeed struct {
	URL   string
	Label string
}

type icsCalendarData struct {
	Name     string
	Timezone *time.Location
	Events   []icsEvent
}

type icsEvent struct {
	Summary     string
	Location    string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Transparent bool
	RRule       string
	ExDates     []time.Time
}

type briefingService struct {
	store    *briefingStore
	calendar calendarProvider
	now      func() time.Time
}

func (noopCalendarProvider) Daily(context.Context, time.Time) (dailyCalendar, error) {
	return dailyCalendar{
		Events:                  []dailyCalendarEvent{},
		CalendarCounts:          map[string]int{},
		Warnings:                []string{},
		FreeBlocks:              []calendarFreeBlock{},
		TotalMeetingHours:       0,
		FocusTimeAvailableHours: 0,
	}, nil
}

func (noopCalendarProvider) WeeklySummary(context.Context, time.Time, time.Time) (map[string]any, error) {
	return map[string]any{}, nil
}

func newICSCalendarProvider(feeds []icsCalendarFeed) *icsCalendarProvider {
	if len(feeds) == 0 {
		return nil
	}
	return &icsCalendarProvider{
		client: &http.Client{Timeout: 30 * time.Second},
		feeds:  feeds,
	}
}

func resolveICSCalendarFeeds() []icsCalendarFeed {
	feeds := make([]icsCalendarFeed, 0, 2)
	for i := 1; i <= 2; i++ {
		url := strings.TrimSpace(os.Getenv(fmt.Sprintf("CALENDAR_%d_ICS_URL", i)))
		if url == "" {
			continue
		}
		feeds = append(feeds, icsCalendarFeed{
			URL:   url,
			Label: strings.TrimSpace(os.Getenv(fmt.Sprintf("CALENDAR_%d_NAME", i))),
		})
	}
	return feeds
}

func resolveBriefingDBPath() string {
	if path := strings.TrimSpace(os.Getenv("BRIEFING_DB_PATH")); path != "" {
		return path
	}
	return thingsDBPath
}

func newBriefingStore(dbPath string, now func() time.Time) *briefingStore {
	return &briefingStore{dbPath: dbPath, now: now}
}

func newBriefingService() *briefingService {
	now := time.Now
	calendar := calendarProvider(noopCalendarProvider{})
	if provider := newICSCalendarProvider(resolveICSCalendarFeeds()); provider != nil {
		calendar = provider
	}
	return &briefingService{
		store:    newBriefingStore(resolveBriefingDBPath(), now),
		calendar: calendar,
		now:      now,
	}
}

func (s *briefingStore) openDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *briefingStore) migrate(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS briefings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL,
			target_key TEXT NOT NULL,
			generated_at TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			payload_json TEXT NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_briefings_kind_target_generated ON briefings(kind, target_key, generated_at DESC, id DESC)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_briefings_created_at ON briefings(created_at)`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS review_feedback (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TEXT NOT NULL,
			created_unix INTEGER NOT NULL,
			review_type TEXT NOT NULL,
			target_key TEXT NOT NULL,
			issue_type TEXT NOT NULL,
			feedback TEXT NOT NULL,
			expected TEXT NOT NULL,
			actual TEXT NOT NULL,
			source TEXT NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_review_feedback_created_unix ON review_feedback(created_unix DESC, id DESC)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_review_feedback_type_target ON review_feedback(review_type, target_key, created_unix DESC, id DESC)`); err != nil {
		return err
	}
	return nil
}

func (s *briefingStore) write(kind, targetKey, generatedAt string, payload any) error {
	db, err := s.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`
		INSERT INTO briefings (kind, target_key, generated_at, created_at, payload_json)
		VALUES (?, ?, ?, ?, ?)
	`, kind, targetKey, generatedAt, s.now().UTC().Unix(), string(data)); err != nil {
		return err
	}
	return s.cleanupDB(db)
}

func (s *briefingStore) cleanupDB(db *sql.DB) error {
	cutoff := s.now().UTC().Add(-briefingRetention).Unix()
	_, err := db.Exec(`DELETE FROM briefings WHERE created_at < ?`, cutoff)
	return err
}

func (s *briefingStore) readLatest(kind, targetKey string, dst any) error {
	db, err := s.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	var payload string
	err = db.QueryRow(`
		SELECT payload_json
		FROM briefings
		WHERE kind = ? AND target_key = ?
		ORDER BY generated_at DESC, id DESC
		LIMIT 1
	`, kind, targetKey).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return errBriefingNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(payload), dst)
}

func (s *briefingStore) readDaily(date string) (*dailyBriefing, error) {
	var out dailyBriefing
	if err := s.readLatest("daily", date, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *briefingStore) readWeekly(week string) (*weeklyBriefing, error) {
	var out weeklyBriefing
	if err := s.readLatest("weekly", week, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *briefingStore) listLatestDailyInRange(start, end time.Time) ([]*dailyBriefing, error) {
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT target_key, payload_json
		FROM briefings
		WHERE kind = 'daily' AND target_key >= ? AND target_key <= ?
		ORDER BY target_key ASC, generated_at DESC, id DESC
	`, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*dailyBriefing{}
	seen := map[string]struct{}{}
	for rows.Next() {
		var (
			targetKey string
			payload   string
		)
		if err := rows.Scan(&targetKey, &payload); err != nil {
			return nil, err
		}
		if _, ok := seen[targetKey]; ok {
			continue
		}
		seen[targetKey] = struct{}{}
		var briefing dailyBriefing
		if err := json.Unmarshal([]byte(payload), &briefing); err != nil {
			return nil, err
		}
		out = append(out, &briefing)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *briefingStore) writeReviewFeedback(entry reviewFeedbackEntry) (reviewFeedbackEntry, error) {
	db, err := s.openDB()
	if err != nil {
		return reviewFeedbackEntry{}, err
	}
	defer db.Close()

	if strings.TrimSpace(entry.CreatedAt) == "" {
		entry.CreatedAt = s.now().UTC().Format(time.RFC3339)
	}
	result, err := db.Exec(`
		INSERT INTO review_feedback (
			created_at, created_unix, review_type, target_key, issue_type, feedback, expected, actual, source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		entry.CreatedAt,
		s.now().UTC().Unix(),
		strings.TrimSpace(entry.ReviewType),
		strings.TrimSpace(entry.TargetKey),
		strings.TrimSpace(entry.IssueType),
		strings.TrimSpace(entry.Feedback),
		strings.TrimSpace(entry.Expected),
		strings.TrimSpace(entry.Actual),
		strings.TrimSpace(entry.Source),
	)
	if err != nil {
		return reviewFeedbackEntry{}, err
	}
	if id, err := result.LastInsertId(); err == nil {
		entry.ID = id
	}
	return entry, nil
}

func (s *briefingStore) listReviewFeedback(limit int, reviewType string) ([]reviewFeedbackEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `
		SELECT id, created_at, review_type, target_key, issue_type, feedback, expected, actual, source
		FROM review_feedback
	`
	args := []any{}
	if strings.TrimSpace(reviewType) != "" {
		query += ` WHERE review_type = ?`
		args = append(args, strings.TrimSpace(reviewType))
	}
	query += ` ORDER BY created_unix DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []reviewFeedbackEntry{}
	for rows.Next() {
		var entry reviewFeedbackEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.CreatedAt,
			&entry.ReviewType,
			&entry.TargetKey,
			&entry.IssueType,
			&entry.Feedback,
			&entry.Expected,
			&entry.Actual,
			&entry.Source,
		); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *icsCalendarProvider) Daily(ctx context.Context, date time.Time) (dailyCalendar, error) {
	dayStart := utcDay(date)
	dayEnd := dayStart.Add(24 * time.Hour)
	events := make([]renderedCalendarEvent, 0)
	calendarCounts := map[string]int{}
	warnings := []string{}

	fetchStart := dayStart.Add(-24 * time.Hour)
	fetchEnd := dayEnd.Add(24 * time.Hour)
	for _, feed := range p.feeds {
		data, err := p.fetchCalendar(ctx, feed)
		if err != nil {
			name := fallbackString(feed.Label, "calendar")
			calendarCounts[name] = 0
			warnings = append(warnings, fmt.Sprintf("Calendar feed %s unavailable: %v", name, err))
			continue
		}
		rendered := renderCalendarEvents(data, fetchStart, fetchEnd, dayStart, dayEnd)
		name := fallbackString(data.Name, feed.Label)
		calendarCounts[name] = len(rendered)
		if len(rendered) == 0 {
			warnings = append(warnings, fmt.Sprintf("No events found on %s", name))
		}
		events = append(events, rendered...)
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].DateKey != events[j].DateKey {
			return events[i].DateKey < events[j].DateKey
		}
		if events[i].Start != events[j].Start {
			return events[i].Start < events[j].Start
		}
		return events[i].Title < events[j].Title
	})

	calendarEvents := make([]dailyCalendarEvent, 0, len(events))
	totalMeetingHours := 0.0
	busy := make([]timeInterval, 0, len(events))
	for _, event := range events {
		calendarEvents = append(calendarEvents, event.dailyCalendarEvent)
		if event.Start == "All day" {
			continue
		}
		start, err := parseClock(dayStart, event.Start)
		if err != nil {
			continue
		}
		end, err := parseClock(dayStart, event.End)
		if err != nil || !end.After(start) {
			continue
		}
		busy = append(busy, timeInterval{Start: start, End: end})
		totalMeetingHours += end.Sub(start).Hours()
	}

	freeBlocks := computeFreeBlocks(dayStart, busy, 8, 20)
	focusHours := 0.0
	for _, block := range freeBlocks {
		if block.Minutes >= 30 {
			focusHours += float64(block.Minutes) / 60
		}
	}

	return dailyCalendar{
		Events:                  calendarEvents,
		CalendarCounts:          calendarCounts,
		Warnings:                warnings,
		TotalMeetingHours:       round1(totalMeetingHours),
		FreeBlocks:              freeBlocks,
		FocusTimeAvailableHours: round1(focusHours),
	}, nil
}

func (p *icsCalendarProvider) WeeklySummary(ctx context.Context, start, end time.Time) (map[string]any, error) {
	windowEnd := utcDay(end).Add(24 * time.Hour)
	events, warnings := p.eventsInRange(ctx, utcDay(start), windowEnd)

	hoursByDay := map[string]float64{}
	countByDay := map[string]int{}
	countByCalendar := map[string]int{}
	busiestDay := ""
	maxMinutes := -1
	for day := utcDay(start); day.Before(windowEnd); day = day.Add(24 * time.Hour) {
		key := day.Format("2006-01-02")
		hoursByDay[key] = 0
		countByDay[key] = 0
	}

	for _, event := range events {
		key := event.DateKey
		countByDay[key]++
		countByCalendar[event.Calendar]++
		if event.Start == "All day" {
			continue
		}
		startTime, err := parseClock(mustParseDate(key), event.Start)
		if err != nil {
			continue
		}
		endTime, err := parseClock(mustParseDate(key), event.End)
		if err != nil || !endTime.After(startTime) {
			continue
		}
		hoursByDay[key] += endTime.Sub(startTime).Hours()
	}

	for key := range hoursByDay {
		hoursByDay[key] = round1(hoursByDay[key])
		minutes := int(hoursByDay[key] * 60)
		if minutes > maxMinutes {
			maxMinutes = minutes
			busiestDay = mustParseDate(key).Weekday().String()
		}
	}

	return map[string]any{
		"event_count_by_day":      countByDay,
		"event_count_by_calendar": countByCalendar,
		"meeting_hours_by_day":    hoursByDay,
		"busiest_day":             busiestDay,
		"calendar_feed_count":     len(p.feeds),
		"warnings":                warnings,
	}, nil
}

type renderedCalendarEvent struct {
	dailyCalendarEvent
	DateKey string
}

type timeInterval struct {
	Start time.Time
	End   time.Time
}

func (p *icsCalendarProvider) eventsInRange(ctx context.Context, start, end time.Time) ([]renderedCalendarEvent, []string) {
	events := make([]renderedCalendarEvent, 0)
	warnings := []string{}
	fetchStart := start.Add(-24 * time.Hour)
	fetchEnd := end.Add(24 * time.Hour)
	for _, feed := range p.feeds {
		data, err := p.fetchCalendar(ctx, feed)
		if err != nil {
			name := fallbackString(feed.Label, "calendar")
			warnings = append(warnings, fmt.Sprintf("Calendar feed %s unavailable: %v", name, err))
			continue
		}
		events = append(events, renderCalendarEvents(data, fetchStart, fetchEnd, start, end)...)
	}
	return events, warnings
}

func (p *icsCalendarProvider) fetchCalendar(ctx context.Context, feed icsCalendarFeed) (*icsCalendarData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch ICS feed %q: %w", feed.Label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch ICS feed %q: unexpected status %d", feed.Label, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, err
	}
	data, err := parseICSCalendar(body, feed.Label)
	if err != nil {
		return nil, fmt.Errorf("parse ICS feed %q: %w", feed.Label, err)
	}
	return data, nil
}

func parseICSCalendar(body []byte, label string) (*icsCalendarData, error) {
	lines := unfoldICSLines(string(body))
	data := &icsCalendarData{
		Name:     label,
		Timezone: time.UTC,
		Events:   []icsEvent{},
	}

	var current *icsEvent
	for _, line := range lines {
		name, params, value, ok := parseICSProperty(line)
		if !ok {
			continue
		}
		switch name {
		case "BEGIN":
			if value == "VEVENT" {
				current = &icsEvent{}
			}
		case "END":
			if value == "VEVENT" && current != nil {
				if !current.Start.IsZero() {
					if current.End.IsZero() {
						if current.AllDay {
							current.End = current.Start.Add(24 * time.Hour)
						} else {
							current.End = current.Start.Add(time.Hour)
						}
					}
					data.Events = append(data.Events, *current)
				}
				current = nil
			}
		case "X-WR-CALNAME":
			if data.Name == "" {
				data.Name = unescapeICSString(value)
			}
		case "X-WR-TIMEZONE":
			if loc, err := time.LoadLocation(value); err == nil {
				data.Timezone = loc
			}
		default:
			if current == nil {
				continue
			}
			switch name {
			case "SUMMARY":
				current.Summary = unescapeICSString(value)
			case "LOCATION":
				current.Location = unescapeICSString(value)
			case "DTSTART":
				t, allDay, err := parseICSDateTime(value, params, data.Timezone)
				if err == nil {
					current.Start = t
					current.AllDay = allDay
				}
			case "DTEND":
				t, _, err := parseICSDateTime(value, params, data.Timezone)
				if err == nil {
					current.End = t
				}
			case "RRULE":
				current.RRule = value
			case "EXDATE":
				current.ExDates = append(current.ExDates, parseICSDateList(value, params, data.Timezone)...)
			case "TRANSP":
				current.Transparent = strings.EqualFold(value, "TRANSPARENT")
			}
		}
	}

	if data.Name == "" {
		data.Name = "calendar"
	}
	return data, nil
}

func unfoldICSLines(body string) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	scanner := bufio.NewScanner(strings.NewReader(body))
	lines := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func parseICSProperty(line string) (string, map[string]string, string, bool) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return "", nil, "", false
	}
	head := line[:idx]
	value := line[idx+1:]
	parts := strings.Split(head, ";")
	name := strings.ToUpper(parts[0])
	params := map[string]string{}
	for _, part := range parts[1:] {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		params[strings.ToUpper(kv[0])] = kv[1]
	}
	return name, params, value, true
}

func parseICSDateTime(value string, params map[string]string, defaultLoc *time.Location) (time.Time, bool, error) {
	if strings.EqualFold(params["VALUE"], "DATE") || len(value) == 8 {
		t, err := time.ParseInLocation("20060102", value, defaultLoc)
		return utcDay(t), true, err
	}
	if strings.HasSuffix(value, "Z") {
		t, err := time.Parse("20060102T150405Z", value)
		if err != nil {
			return time.Time{}, false, err
		}
		return t.UTC(), false, nil
	}
	loc := defaultLoc
	if tzid := strings.TrimSpace(params["TZID"]); tzid != "" {
		if parsed, err := time.LoadLocation(tzid); err == nil {
			loc = parsed
		}
	}
	layout := "20060102T150405"
	if len(value) == len("20060102T1504") {
		layout = "20060102T1504"
	}
	t, err := time.ParseInLocation(layout, value, loc)
	if err != nil {
		return time.Time{}, false, err
	}
	return t.UTC(), false, nil
}

func parseICSDateList(value string, params map[string]string, defaultLoc *time.Location) []time.Time {
	parts := strings.Split(value, ",")
	out := make([]time.Time, 0, len(parts))
	for _, part := range parts {
		t, _, err := parseICSDateTime(strings.TrimSpace(part), params, defaultLoc)
		if err == nil {
			out = append(out, t.UTC())
		}
	}
	return out
}

func unescapeICSString(value string) string {
	replacer := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	return replacer.Replace(value)
}

func renderCalendarEvents(data *icsCalendarData, expandStart, expandEnd, includeStart, includeEnd time.Time) []renderedCalendarEvent {
	rendered := []renderedCalendarEvent{}
	calendarName := data.Name
	for _, event := range data.Events {
		occurrences := expandICSEvent(event, expandStart, expandEnd)
		for _, occurrence := range occurrences {
			localStart := occurrence.Start.In(data.Timezone)
			localDate := localDay(localStart)
			if localDate.Before(localDay(includeStart.In(data.Timezone))) || !localDate.Before(localDay(includeEnd.In(data.Timezone))) {
				continue
			}
			rendered = append(rendered, renderedCalendarEvent{
				dailyCalendarEvent: dailyCalendarEvent{
					Title:      fallbackString(occurrence.Summary, "(Untitled)"),
					Start:      formatEventClock(occurrence.Start.In(data.Timezone), occurrence.AllDay),
					End:        formatEventClock(occurrence.End.In(data.Timezone), occurrence.AllDay),
					Calendar:   calendarName,
					Location:   occurrence.Location,
					PrepNeeded: needsPrep(occurrence.Summary),
					PrepNote:   prepNote(occurrence.Summary),
				},
				DateKey: localDate.Format("2006-01-02"),
			})
		}
	}
	return rendered
}

func expandICSEvent(event icsEvent, windowStart, windowEnd time.Time) []icsEvent {
	if event.Transparent {
		return nil
	}
	if event.RRule == "" {
		if eventOverlapsWindow(event.Start, event.End, windowStart, windowEnd) {
			return []icsEvent{event}
		}
		return nil
	}

	rule := parseRRULE(event.RRule)
	if rule["FREQ"] == "" {
		if eventOverlapsWindow(event.Start, event.End, windowStart, windowEnd) {
			return []icsEvent{event}
		}
		return nil
	}

	switch rule["FREQ"] {
	case "DAILY":
		return expandDailyRecurring(event, rule, windowStart, windowEnd)
	case "WEEKLY":
		return expandWeeklyRecurring(event, rule, windowStart, windowEnd)
	default:
		if eventOverlapsWindow(event.Start, event.End, windowStart, windowEnd) {
			return []icsEvent{event}
		}
		return nil
	}
}

func parseRRULE(raw string) map[string]string {
	rule := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			rule[strings.ToUpper(kv[0])] = strings.ToUpper(kv[1])
		}
	}
	return rule
}

func expandDailyRecurring(event icsEvent, rule map[string]string, windowStart, windowEnd time.Time) []icsEvent {
	interval := parsePositiveInt(rule["INTERVAL"], 1)
	until := parseRRULEUntil(rule["UNTIL"], event.Start.Location())
	countLimit := parsePositiveInt(rule["COUNT"], 0)
	duration := event.End.Sub(event.Start)
	current := event.Start
	if countLimit == 0 && windowStart.After(current) {
		days := int(windowStart.Sub(current).Hours() / 24)
		jump := days / interval
		current = current.AddDate(0, 0, jump*interval)
		for current.Before(windowStart) {
			current = current.AddDate(0, 0, interval)
		}
	}

	out := []icsEvent{}
	count := 0
	for !current.After(windowEnd) {
		if countLimit > 0 && count >= countLimit {
			break
		}
		count++
		if !until.IsZero() && current.After(until) {
			break
		}
		if !containsTime(event.ExDates, current) && eventOverlapsWindow(current, current.Add(duration), windowStart, windowEnd) {
			next := event
			next.Start = current
			next.End = current.Add(duration)
			out = append(out, next)
		}
		current = current.AddDate(0, 0, interval)
	}
	return out
}

func expandWeeklyRecurring(event icsEvent, rule map[string]string, windowStart, windowEnd time.Time) []icsEvent {
	interval := parsePositiveInt(rule["INTERVAL"], 1)
	until := parseRRULEUntil(rule["UNTIL"], event.Start.Location())
	countLimit := parsePositiveInt(rule["COUNT"], 0)
	duration := event.End.Sub(event.Start)
	weekdays := parseByDay(rule["BYDAY"])
	if len(weekdays) == 0 {
		weekdays = []time.Weekday{event.Start.In(time.UTC).Weekday()}
	}

	baseLocal := event.Start.In(time.UTC)
	anchorWeek := startOfWeek(baseLocal, time.Monday)
	currentWeek := anchorWeek
	if countLimit == 0 && windowStart.After(anchorWeek) {
		weeks := int(windowStart.Sub(anchorWeek).Hours() / (24 * 7))
		jump := weeks / interval
		currentWeek = currentWeek.AddDate(0, 0, jump*7*interval)
		for currentWeek.Before(windowStart.AddDate(0, 0, -7)) {
			currentWeek = currentWeek.AddDate(0, 0, 7*interval)
		}
	}

	out := []icsEvent{}
	count := 0
	for !currentWeek.After(windowEnd) {
		for _, weekday := range weekdays {
			occurrenceStart := combineWeekdayAndTime(currentWeek, weekday, baseLocal)
			if occurrenceStart.Before(event.Start) {
				continue
			}
			if countLimit > 0 && count >= countLimit {
				return out
			}
			if !until.IsZero() && occurrenceStart.After(until) {
				return out
			}
			count++
			if containsTime(event.ExDates, occurrenceStart) {
				continue
			}
			occurrenceEnd := occurrenceStart.Add(duration)
			if eventOverlapsWindow(occurrenceStart, occurrenceEnd, windowStart, windowEnd) {
				next := event
				next.Start = occurrenceStart
				next.End = occurrenceEnd
				out = append(out, next)
			}
		}
		currentWeek = currentWeek.AddDate(0, 0, 7*interval)
	}
	return out
}

func parseRRULEUntil(value string, defaultLoc *time.Location) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, _, err := parseICSDateTime(value, map[string]string{}, defaultLoc)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseByDay(value string) []time.Weekday {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]time.Weekday, 0, len(parts))
	for _, part := range parts {
		switch strings.TrimSpace(part) {
		case "MO":
			out = append(out, time.Monday)
		case "TU":
			out = append(out, time.Tuesday)
		case "WE":
			out = append(out, time.Wednesday)
		case "TH":
			out = append(out, time.Thursday)
		case "FR":
			out = append(out, time.Friday)
		case "SA":
			out = append(out, time.Saturday)
		case "SU":
			out = append(out, time.Sunday)
		}
	}
	return out
}

func startOfWeek(day time.Time, weekStart time.Weekday) time.Time {
	day = utcDay(day)
	offset := (int(day.Weekday()) - int(weekStart) + 7) % 7
	return day.AddDate(0, 0, -offset)
}

func combineWeekdayAndTime(weekStart time.Time, weekday time.Weekday, template time.Time) time.Time {
	offset := (int(weekday) - int(time.Monday) + 7) % 7
	day := weekStart.AddDate(0, 0, offset)
	return time.Date(day.Year(), day.Month(), day.Day(), template.Hour(), template.Minute(), template.Second(), template.Nanosecond(), time.UTC)
}

func eventOverlapsWindow(start, end, windowStart, windowEnd time.Time) bool {
	return start.Before(windowEnd) && end.After(windowStart)
}

func containsTime(values []time.Time, want time.Time) bool {
	want = want.UTC()
	for _, value := range values {
		if value.UTC().Equal(want) {
			return true
		}
	}
	return false
}

func formatEventClock(t time.Time, allDay bool) string {
	if allDay {
		return "All day"
	}
	return t.Format("15:04")
}

func needsPrep(title string) bool {
	return containsAny(strings.ToLower(title), "review", "presentation", "demo", "prep")
}

func prepNote(title string) string {
	if needsPrep(title) {
		return "Title suggests prep may be needed"
	}
	return ""
}

func computeFreeBlocks(day time.Time, busy []timeInterval, startHour, endHour int) []calendarFreeBlock {
	windowStart := time.Date(day.Year(), day.Month(), day.Day(), startHour, 0, 0, 0, time.UTC)
	windowEnd := time.Date(day.Year(), day.Month(), day.Day(), endHour, 0, 0, 0, time.UTC)
	merged := mergeIntervals(clampIntervals(busy, windowStart, windowEnd))
	free := []calendarFreeBlock{}
	cursor := windowStart
	for _, interval := range merged {
		if interval.Start.After(cursor) {
			free = append(free, newFreeBlock(cursor, interval.Start))
		}
		if interval.End.After(cursor) {
			cursor = interval.End
		}
	}
	if cursor.Before(windowEnd) {
		free = append(free, newFreeBlock(cursor, windowEnd))
	}
	return free
}

func clampIntervals(intervals []timeInterval, start, end time.Time) []timeInterval {
	out := make([]timeInterval, 0, len(intervals))
	for _, interval := range intervals {
		if !interval.End.After(start) || !interval.Start.Before(end) {
			continue
		}
		if interval.Start.Before(start) {
			interval.Start = start
		}
		if interval.End.After(end) {
			interval.End = end
		}
		if interval.End.After(interval.Start) {
			out = append(out, interval)
		}
	}
	return out
}

func mergeIntervals(intervals []timeInterval) []timeInterval {
	if len(intervals) == 0 {
		return nil
	}
	sort.Slice(intervals, func(i, j int) bool {
		if !intervals[i].Start.Equal(intervals[j].Start) {
			return intervals[i].Start.Before(intervals[j].Start)
		}
		return intervals[i].End.Before(intervals[j].End)
	})
	merged := []timeInterval{intervals[0]}
	for _, interval := range intervals[1:] {
		last := &merged[len(merged)-1]
		if interval.Start.After(last.End) {
			merged = append(merged, interval)
			continue
		}
		if interval.End.After(last.End) {
			last.End = interval.End
		}
	}
	return merged
}

func newFreeBlock(start, end time.Time) calendarFreeBlock {
	return calendarFreeBlock{
		Start:   start.Format("15:04"),
		End:     end.Format("15:04"),
		Minutes: int(end.Sub(start).Minutes()),
	}
}

func parseClock(day time.Time, value string) (time.Time, error) {
	t, err := time.Parse("15:04", value)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC), nil
}

func mustParseDate(value string) time.Time {
	t, _ := parseBriefingDate(value)
	return t
}

func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func (svc *briefingService) GenerateDaily(ctx context.Context, date time.Time) (*dailyBriefing, error) {
	if err := syncForRead(); err != nil {
		return nil, err
	}

	date = utcDay(date)
	state := syncer.State()
	allTasks, err := state.AllTasks(sync.QueryOpts{})
	if err != nil {
		return nil, err
	}
	inboxTasks, err := state.TasksInInbox(sync.QueryOpts{})
	if err != nil {
		return nil, err
	}
	upcomingTasks, err := state.TasksInUpcoming(sync.QueryOpts{Limit: 20})
	if err != nil {
		return nil, err
	}
	allProjects, err := state.AllProjects(sync.QueryOpts{})
	if err != nil {
		return nil, err
	}
	allTags, err := state.AllTags()
	if err != nil {
		return nil, err
	}
	calendar, err := svc.calendar.Daily(ctx, date)
	if err != nil {
		return nil, err
	}

	tagTitles, tagByTitle := buildTagMaps(allTags)
	projectSuggestions := buildProjectSuggestions(allProjects)
	dayStart := utcDay(date)
	nextDay := dayStart.Add(24 * time.Hour)
	recentCompletionWindowStart := dayStart.Add(-48 * time.Hour)
	completedTodayTasks, err := state.CompletedTasksInRange(20, &dayStart, &nextDay)
	if err != nil {
		return nil, err
	}
	recentlyCompletedTasks, err := state.CompletedTasksInRange(200, &recentCompletionWindowStart, &nextDay)
	if err != nil {
		return nil, err
	}
	recentlyCompletedKeys := buildRecentCompletionKeys(state, recentlyCompletedTasks)

	todayTasks := make([]dailyTodayTask, 0)
	overdueItems := make([]dailyOverdueItem, 0)
	deadlineCount := 0

	for _, task := range allTasks {
		if task == nil || task.Type != things.TaskTypeTask || task.Status != things.TaskStatusPending || task.InTrash {
			continue
		}
		if shouldExcludeRoutineTaskWithState(state, task) {
			continue
		}
		if hasDeadlineOnDay(task, dayStart, nextDay) {
			deadlineCount++
		}
		if isTaskOnDate(task, dayStart, nextDay) {
			todayTasks = append(todayTasks, svc.buildDailyTodayTask(state, task, dayStart, tagTitles))
			continue
		}
		if isOverdueAnytimeTask(task, dayStart, nextDay) {
			if shouldSuppressOverdueAsRecentRepeat(state, task, recentlyCompletedKeys) {
				continue
			}
			overdueItems = append(overdueItems, buildOverdueItem(task, dayStart))
		}
	}

	sort.Slice(todayTasks, func(i, j int) bool {
		if todayTasks[i].EstimatedEffort != todayTasks[j].EstimatedEffort {
			return effortRank(todayTasks[i].EstimatedEffort) > effortRank(todayTasks[j].EstimatedEffort)
		}
		return todayTasks[i].Title < todayTasks[j].Title
	})
	sort.Slice(overdueItems, func(i, j int) bool {
		if overdueItems[i].DaysOverdue != overdueItems[j].DaysOverdue {
			return overdueItems[i].DaysOverdue > overdueItems[j].DaysOverdue
		}
		return overdueItems[i].Title < overdueItems[j].Title
	})

	inboxSuggestions := make([]dailyInboxSuggestion, 0, len(inboxTasks))
	for _, task := range inboxTasks {
		if task == nil {
			continue
		}
		inboxSuggestions = append(inboxSuggestions, buildInboxSuggestion(task, projectSuggestions, tagByTitle, dayStart))
	}

	lookahead := buildUpcomingLookahead(state, upcomingTasks, tagTitles)
	completedToday := buildDailyCompletedSummary(state, completedTodayTasks)
	nowWithoutDates := buildNowTaggedWithoutDates(state, allTasks)
	tagSignals := buildDailyTagSignals(todayTasks, overdueItems, calendar)

	capacity := buildCapacityAssessment(dayStart, calendar.FocusTimeAvailableHours, todayTasks, overdueItems)
	briefing := &dailyBriefing{
		Type:              "daily",
		Date:              dayStart.Format("2006-01-02"),
		GeneratedAt:       svc.now().UTC().Format(time.RFC3339),
		Calendar:          calendar,
		UpcomingLookahead: lookahead,
		TodayTasks: dailyTodayTasks{
			Tasks:              todayTasks,
			DeadlineCountToday: deadlineCount,
			DeadlineWarning:    deadlineCount >= 3 || (deadlineCount > 0 && capacity.Overloaded),
		},
		Overdue:            overdueItems,
		Inbox:              inboxSuggestions,
		CompletedToday:     completedToday,
		NowWithoutDates:    nowWithoutDates,
		TagSignals:         tagSignals,
		CapacityAssessment: capacity,
	}

	briefingWriteMu.Lock()
	defer briefingWriteMu.Unlock()
	if err := svc.store.write("daily", briefing.Date, briefing.GeneratedAt, briefing); err != nil {
		return nil, err
	}
	return briefing, nil
}

func (svc *briefingService) GenerateWeekly(ctx context.Context, week string) (*weeklyBriefing, error) {
	if err := syncForRead(); err != nil {
		return nil, err
	}

	targetWeekStart, err := parseISOWeek(week)
	if err != nil {
		return nil, err
	}
	reviewStart := targetWeekStart.AddDate(0, 0, -7)
	reviewEnd := targetWeekStart.AddDate(0, 0, -1)
	targetWeekEnd := targetWeekStart.AddDate(0, 0, 7)

	state := syncer.State()
	allTasks, err := state.AllTasks(sync.QueryOpts{})
	if err != nil {
		return nil, err
	}
	allProjects, err := state.AllProjects(sync.QueryOpts{})
	if err != nil {
		return nil, err
	}
	allTags, err := state.AllTags()
	if err != nil {
		return nil, err
	}
	inboxTasks, err := state.TasksInInbox(sync.QueryOpts{})
	if err != nil {
		return nil, err
	}
	completed, err := state.CompletedTasksInRange(500, &reviewStart, &targetWeekStart)
	if err != nil {
		return nil, err
	}
	allCompleted, err := state.CompletedTasksInRange(1000, nil, nil)
	if err != nil {
		return nil, err
	}
	weeklyCalendar, err := svc.calendar.WeeklySummary(ctx, targetWeekStart, targetWeekEnd.Add(-time.Nanosecond))
	if err != nil {
		return nil, err
	}

	byProject := map[string]int{}
	for _, task := range completed {
		if task == nil || shouldExcludeRoutineTaskWithState(state, task) {
			continue
		}
		name := projectDisplayName(state, task)
		if name == "" {
			name = "Uncategorised"
		}
		byProject[name]++
	}
	highlights := weeklyHighlights(byProject)
	rollovers := svc.rolloversForPeriod(reviewStart, reviewEnd)
	staleProjects := buildWeeklyStaleProjects(state, allTasks, allCompleted, targetWeekStart)
	projectHealth := buildWeeklyProjectHealth(state, allProjects)
	deadlineDays, heaviestDay := buildUpcomingWeekLoad(state, allTasks, targetWeekStart, targetWeekEnd)
	nowWithoutDates := buildNowTaggedWithoutDates(state, allTasks)
	tagTitles, _ := buildTagMaps(allTags)
	tagAudit := buildWeeklyTagAudit(state, allTasks, tagTitles)

	briefing := &weeklyBriefing{
		Type:        "weekly",
		Week:        week,
		GeneratedAt: svc.now().UTC().Format(time.RFC3339),
		ReviewPeriod: weeklyReviewPeriod{
			Start: reviewStart.Format("2006-01-02"),
			End:   reviewEnd.Format("2006-01-02"),
		},
		CompletedTasks: weeklyCompletedTasks{
			Count:      len(completed),
			ByProject:  byProject,
			Highlights: highlights,
		},
		Rollovers:     rollovers,
		StaleProjects: staleProjects,
		ProjectHealth: projectHealth,
		UpcomingWeek: weeklyUpcomingWeek{
			CalendarSummary: weeklyCalendar,
			DeadlineDays:    deadlineDays,
			HeaviestDay:     heaviestDay,
		},
		NowTaggedWithoutDates: nowWithoutDates,
		TagAudit:              tagAudit,
		InboxBacklog:          len(inboxTasks),
	}

	briefingWriteMu.Lock()
	defer briefingWriteMu.Unlock()
	if err := svc.store.write("weekly", week, briefing.GeneratedAt, briefing); err != nil {
		return nil, err
	}
	return briefing, nil
}

func buildTagMaps(tags []*things.Tag) (map[string]string, map[string]*things.Tag) {
	byID := make(map[string]string, len(tags))
	byTitle := make(map[string]*things.Tag, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		byID[tag.UUID] = tag.Title
		byTitle[strings.ToLower(strings.TrimSpace(tag.Title))] = tag
	}
	return byID, byTitle
}

type projectSuggestion struct {
	uuid   string
	title  string
	tokens []string
}

func buildProjectSuggestions(projects []*things.Task) []projectSuggestion {
	out := make([]projectSuggestion, 0, len(projects))
	for _, project := range projects {
		if project == nil {
			continue
		}
		out = append(out, projectSuggestion{
			uuid:   project.UUID,
			title:  project.Title,
			tokens: titleTokens(project.Title),
		})
	}
	return out
}

func (svc *briefingService) buildDailyTodayTask(state *sync.State, task *things.Task, dayStart time.Time, tagTitles map[string]string) dailyTodayTask {
	tagNames := tagNamesForTask(task, tagTitles)
	scheduled := taskScheduleString(task)
	deadline := dateStringPtr(task.DeadlineDate)
	daysOnToday := daysSinceAnchor(taskScheduleAnchor(task), dayStart)
	stale := daysOnToday >= 2
	return dailyTodayTask{
		UUID:                  task.UUID,
		Title:                 task.Title,
		Project:               projectDisplayName(state, task),
		Tags:                  tagNames,
		Deadline:              deadline,
		ScheduledFor:          scheduled,
		DaysOnToday:           daysOnToday,
		Stale:                 stale,
		StaleReason:           staleReason(stale, daysOnToday),
		ImprovementSuggestion: improvementSuggestion(task.Title, estimateEffort(tagNames, task.Title)),
		EstimatedEffort:       estimateEffort(tagNames, task.Title),
	}
}

func buildOverdueItem(task *things.Task, dayStart time.Time) dailyOverdueItem {
	days := daysSinceAnchor(task.ScheduledDate, dayStart)
	return dailyOverdueItem{
		UUID:           task.UUID,
		Title:          task.Title,
		ScheduledFor:   dateStringPtr(task.ScheduledDate),
		DaysOverdue:    days,
		Recommendation: overdueRecommendation(days),
	}
}

func buildUpcomingLookahead(state *sync.State, tasks []*things.Task, tagTitles map[string]string) []dailyUpcomingItem {
	out := make([]dailyUpcomingItem, 0, min(7, len(tasks)))
	for _, task := range tasks {
		if task == nil || shouldExcludeRoutineTaskWithState(state, task) {
			continue
		}
		out = append(out, dailyUpcomingItem{
			UUID:         task.UUID,
			Title:        task.Title,
			Project:      projectDisplayName(state, task),
			Tags:         tagNamesForTask(task, tagTitles),
			ScheduledFor: taskScheduleString(task),
			Deadline:     dateStringPtr(task.DeadlineDate),
		})
		if len(out) == 7 {
			break
		}
	}
	return out
}

func buildDailyCompletedSummary(state *sync.State, tasks []*things.Task) dailyCompletedSummary {
	out := make([]dailyCompletedTask, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || shouldExcludeRoutineTaskWithState(state, task) || task.CompletionDate == nil {
			continue
		}
		out = append(out, dailyCompletedTask{
			UUID:        task.UUID,
			Title:       task.Title,
			Project:     projectDisplayName(state, task),
			CompletedAt: task.CompletionDate.UTC().Format(time.RFC3339),
		})
	}
	return dailyCompletedSummary{Count: len(out), Tasks: out}
}

func buildDailyTagSignals(todayTasks []dailyTodayTask, overdue []dailyOverdueItem, calendar dailyCalendar) dailyTagSignals {
	deepCount := 0
	aiSuggestions := []reviewTaskCue{}
	for _, task := range todayTasks {
		if task.EstimatedEffort == "deep" {
			deepCount++
		}
		if containsString(task.Tags, "AI") {
			continue
		}
		if reason := aiSuggestionReason(task.Title); reason != "" {
			aiSuggestions = append(aiSuggestions, reviewTaskCue{
				UUID:    task.UUID,
				Title:   task.Title,
				Project: task.Project,
				Reason:  reason,
			})
		}
	}
	rewriteCues := []reviewTaskCue{}
	for _, item := range overdue {
		if item.Recommendation != "rewrite" {
			continue
		}
		rewriteCues = append(rewriteCues, reviewTaskCue{
			UUID:   item.UUID,
			Title:  item.Title,
			Reason: "Overdue long enough that the wording or scope probably needs rewriting",
		})
	}

	meetingHeavyDay := calendar.TotalMeetingHours >= 3
	deepWarning := ""
	switch {
	case meetingHeavyDay && deepCount > 1:
		deepWarning = "Meeting-heavy day with multiple deep tasks; reduce scope or defer one."
	case !meetingHeavyDay && deepCount > 2:
		deepWarning = "Too many deep tasks for one day; narrow the plan."
	}

	return dailyTagSignals{
		DeepTaskCount:   deepCount,
		MeetingHeavyDay: meetingHeavyDay,
		DeepTaskWarning: deepWarning,
		SuggestedAI:     limitReviewTaskCues(aiSuggestions, 8),
		RewriteCues:     limitReviewTaskCues(rewriteCues, 6),
	}
}

func buildInboxSuggestion(task *things.Task, projects []projectSuggestion, tagByTitle map[string]*things.Tag, dayStart time.Time) dailyInboxSuggestion {
	projectTitle, projectUUID := suggestProject(task.Title, projects)
	suggestedWhen := suggestWhen(task.Title, dayStart)
	suggestedTags, tagUUIDs := suggestTags(task.Title, suggestedWhen, dayStart, tagByTitle)
	return dailyInboxSuggestion{
		UUID:                 task.UUID,
		Title:                task.Title,
		SuggestedProject:     projectTitle,
		SuggestedProjectUUID: projectUUID,
		SuggestedTags:        suggestedTags,
		SuggestedTagUUIDs:    strings.Join(tagUUIDs, ","),
		SuggestedWhen:        suggestedWhen,
		Reasoning:            inboxReasoning(task.Title, projectTitle, suggestedTags, suggestedWhen, dayStart),
	}
}

func buildWeeklyTagAudit(state *sync.State, tasks []*things.Task, tagTitles map[string]string) weeklyTagAudit {
	missingHorizon := []weeklyTaskSummary{}
	conflictingHorizon := []weeklyTagIssue{}
	missingContext := []weeklyTaskSummary{}
	conflictingContext := []weeklyTagIssue{}
	suggestedAI := []reviewTaskCue{}
	laterTasks := []weeklyTaskSummary{}

	for _, task := range tasks {
		if task == nil || task.Type != things.TaskTypeTask || task.Status != things.TaskStatusPending || task.InTrash || shouldExcludeRoutineTaskWithState(state, task) {
			continue
		}

		project := projectDisplayName(state, task)
		summary := weeklyTaskSummary{
			UUID:    task.UUID,
			Title:   task.Title,
			Project: project,
		}
		displayTags := tagNamesForTask(task, tagTitles)
		horizonTags := matchingExclusiveTags(displayTags, "Now", "Next", "Later")
		contextTags := matchingExclusiveTags(displayTags, "Work", "Personal")

		switch len(horizonTags) {
		case 0:
			missingHorizon = append(missingHorizon, summary)
		case 1:
			if containsString(horizonTags, "Later") {
				laterTasks = append(laterTasks, summary)
			}
		default:
			conflictingHorizon = append(conflictingHorizon, weeklyTagIssue{
				UUID:    task.UUID,
				Title:   task.Title,
				Project: project,
				Tags:    horizonTags,
				Note:    "Task should have exactly one of Now, Next, or Later",
			})
		}

		switch len(contextTags) {
		case 0:
			missingContext = append(missingContext, summary)
		case 1:
		default:
			conflictingContext = append(conflictingContext, weeklyTagIssue{
				UUID:    task.UUID,
				Title:   task.Title,
				Project: project,
				Tags:    contextTags,
				Note:    "Task should have only one context tag: Work or Personal",
			})
		}

		if !containsString(displayTags, "AI") {
			if reason := aiSuggestionReason(task.Title); reason != "" {
				suggestedAI = append(suggestedAI, reviewTaskCue{
					UUID:    task.UUID,
					Title:   task.Title,
					Project: project,
					Reason:  reason,
				})
			}
		}
	}

	sortWeeklyTaskSummaries(missingHorizon)
	sortWeeklyTaskSummaries(missingContext)
	sortWeeklyTaskSummaries(laterTasks)
	sortWeeklyTagIssues(conflictingHorizon)
	sortWeeklyTagIssues(conflictingContext)
	sortReviewTaskCues(suggestedAI)

	return weeklyTagAudit{
		MissingHorizonTags:     limitWeeklyTaskSummaries(missingHorizon, 20),
		ConflictingHorizonTags: limitWeeklyTagIssues(conflictingHorizon, 20),
		MissingContextTags:     limitWeeklyTaskSummaries(missingContext, 20),
		ConflictingContextTags: limitWeeklyTagIssues(conflictingContext, 20),
		SuggestedAI:            limitReviewTaskCues(suggestedAI, 20),
		LaterTasks:             limitWeeklyTaskSummaries(laterTasks, 40),
	}
}

func buildCapacityAssessment(dayStart time.Time, availableHours float64, todayTasks []dailyTodayTask, overdue []dailyOverdueItem) dailyCapacityAssessment {
	totalHours := 0.0
	for _, task := range todayTasks {
		totalHours += effortHours(task.EstimatedEffort)
	}
	for _, item := range overdue {
		totalHours += overdueEffortHours(item.DaysOverdue)
	}

	overloaded := availableHours > 0 && totalHours > availableHours
	deferSuggestions := []dailyDeferSuggestion{}
	if overloaded {
		for _, task := range todayTasks {
			if task.EstimatedEffort != "deep" {
				continue
			}
			deferSuggestions = append(deferSuggestions, dailyDeferSuggestion{
				UUID:             task.UUID,
				Title:            task.Title,
				Reason:           "Deep task, but the day has limited focus capacity",
				SuggestedNewDate: dayStart.AddDate(0, 0, 2).Format("2006-01-02"),
			})
			if len(deferSuggestions) == 3 {
				break
			}
		}
	}

	return dailyCapacityAssessment{
		TotalTasks:         len(todayTasks) + len(overdue),
		EstimatedTaskHours: round1(totalHours),
		AvailableHours:     round1(availableHours),
		Overloaded:         overloaded,
		DeferSuggestions:   deferSuggestions,
	}
}

func buildWeeklyStaleProjects(state *sync.State, openTasks []*things.Task, completed []*things.Task, now time.Time) []weeklyStaleProject {
	openCounts := map[string]int{}
	lastCompletion := map[string]time.Time{}

	for _, task := range openTasks {
		if task == nil || task.Type != things.TaskTypeTask || shouldExcludeRoutineTaskWithState(state, task) {
			continue
		}
		name := projectDisplayName(state, task)
		if name == "" {
			continue
		}
		openCounts[name]++
	}
	for _, task := range completed {
		if task == nil || task.CompletionDate == nil || shouldExcludeRoutineTaskWithState(state, task) {
			continue
		}
		name := projectDisplayName(state, task)
		if name == "" {
			continue
		}
		if last, ok := lastCompletion[name]; !ok || task.CompletionDate.After(last) {
			lastCompletion[name] = *task.CompletionDate
		}
	}

	cutoff := now.AddDate(0, 0, -14)
	out := make([]weeklyStaleProject, 0)
	for project, count := range openCounts {
		last, ok := lastCompletion[project]
		if ok && !last.Before(cutoff) {
			continue
		}
		item := weeklyStaleProject{
			Project:   project,
			OpenTasks: count,
			Note:      "No activity in 2+ weeks",
		}
		if ok {
			item.LastCompletion = last.Format("2006-01-02")
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OpenTasks != out[j].OpenTasks {
			return out[i].OpenTasks > out[j].OpenTasks
		}
		return out[i].Project < out[j].Project
	})
	return out
}

func buildWeeklyProjectHealth(state *sync.State, projects []*things.Task) weeklyProjectHealth {
	out := []weeklyProjectStatus{}
	for _, project := range projects {
		if project == nil || shouldExcludeRoutineProject(project) {
			continue
		}
		tasks, err := state.TasksInProject(project.UUID, sync.QueryOpts{})
		if err != nil {
			continue
		}
		if projectHasNextAction(state, tasks) {
			continue
		}
		out = append(out, weeklyProjectStatus{
			Project:       project.Title,
			ProjectUUID:   project.UUID,
			OpenTaskCount: countIncludedTasks(state, tasks),
			Note:          "No clear next action found",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OpenTaskCount != out[j].OpenTaskCount {
			return out[i].OpenTaskCount < out[j].OpenTaskCount
		}
		return out[i].Project < out[j].Project
	})
	return weeklyProjectHealth{ProjectsWithoutNextAction: out}
}

func buildUpcomingWeekLoad(state *sync.State, tasks []*things.Task, start, end time.Time) (map[string]int, string) {
	dayLoads := map[string]int{}
	dayNames := map[string]string{}
	for t := start; t.Before(end); t = t.Add(24 * time.Hour) {
		key := t.Format("2006-01-02")
		dayLoads[key] = 0
		dayNames[key] = t.Weekday().String()
	}

	for _, task := range tasks {
		if task == nil || task.Status != things.TaskStatusPending || shouldExcludeRoutineTaskWithState(state, task) {
			continue
		}
		if task.DeadlineDate != nil && !task.DeadlineDate.Before(start) && task.DeadlineDate.Before(end) {
			key := utcDay(*task.DeadlineDate).Format("2006-01-02")
			dayLoads[key]++
		}
	}

	deadlineDays := map[string]int{}
	heaviestDay := ""
	maxLoad := -1
	for key, count := range dayLoads {
		if count > 0 {
			deadlineDays[key] = count
		}
		if count > maxLoad {
			maxLoad = count
			heaviestDay = dayNames[key]
		}
	}
	if maxLoad <= 0 {
		heaviestDay = ""
	}
	return deadlineDays, heaviestDay
}

func buildNowTaggedWithoutDates(state *sync.State, tasks []*things.Task) []weeklyTaskSummary {
	out := []weeklyTaskSummary{}
	for _, task := range tasks {
		if task == nil || task.Status != things.TaskStatusPending || shouldExcludeRoutineTaskWithState(state, task) {
			continue
		}
		if task.ScheduledDate != nil || task.TodayIndexReference != nil {
			continue
		}
		if !hasTagTitle(state, task, "now") {
			continue
		}
		out = append(out, weeklyTaskSummary{
			UUID:    task.UUID,
			Title:   task.Title,
			Project: projectDisplayName(state, task),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

var excludedRoutineTitles = map[string]struct{}{
	"Morning Review": {},
	"Take tablet":    {},
	"Evening Review": {},
	"Weekly Review":  {},
}

func shouldExcludeRoutineTask(task *things.Task) bool {
	return task != nil && shouldExcludeRoutineTitle(task.Title)
}

func shouldExcludeRoutineTitle(title string) bool {
	_, ok := excludedRoutineTitles[strings.TrimSpace(title)]
	return ok
}

func shouldExcludeRoutineTaskWithState(state *sync.State, task *things.Task) bool {
	if shouldExcludeRoutineTask(task) {
		return true
	}
	if task == nil || state == nil {
		return false
	}
	if projectUUID := widgetRootProjectUUID(state, task); projectUUID == widgetExcludedProjectUUID {
		return true
	}
	return shouldExcludeRoutineProjectTitle(projectDisplayName(state, task))
}

func shouldExcludeRoutineProject(project *things.Task) bool {
	if project == nil {
		return false
	}
	if project.UUID == widgetExcludedProjectUUID {
		return true
	}
	return shouldExcludeRoutineProjectTitle(project.Title)
}

func shouldExcludeRoutineProjectTitle(title string) bool {
	return normalizedRoutineProjectTitle(title) == "routines"
}

func normalizedRoutineProjectTitle(title string) string {
	fields := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	return strings.Join(fields, " ")
}

func buildRecentCompletionKeys(state *sync.State, tasks []*things.Task) map[string]struct{} {
	keys := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		if task == nil || task.CompletionDate == nil || shouldExcludeRoutineTaskWithState(state, task) {
			continue
		}
		project := ""
		if state != nil {
			project = projectDisplayName(state, task)
		}
		if key := recentCompletionKey(task.Title, project); key != "" {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func shouldSuppressOverdueAsRecentRepeat(state *sync.State, task *things.Task, recent map[string]struct{}) bool {
	if task == nil || len(recent) == 0 {
		return false
	}
	project := ""
	if state != nil {
		project = projectDisplayName(state, task)
	}
	_, ok := recent[recentCompletionKey(task.Title, project)]
	return ok
}

func recentCompletionKey(title, project string) string {
	normalizedTitle := normalizeRecentCompletionPart(title)
	if normalizedTitle == "" {
		return ""
	}
	normalizedProject := normalizeRecentCompletionPart(project)
	if normalizedProject == "" {
		return normalizedTitle
	}
	return normalizedProject + "::" + normalizedTitle
}

func normalizeRecentCompletionPart(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func hasTagTitle(state *sync.State, task *things.Task, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, tagID := range task.TagIDs {
		tag, err := state.Tag(tagID)
		if err != nil || tag == nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(tag.Title)) == want {
			return true
		}
	}
	return false
}

func projectHasNextAction(state *sync.State, tasks []*things.Task) bool {
	for _, task := range tasks {
		if task == nil || shouldExcludeRoutineTaskWithState(state, task) || task.Status != things.TaskStatusPending || task.InTrash {
			continue
		}
		if task.ScheduledDate != nil || task.TodayIndexReference != nil {
			return true
		}
		if task.Schedule == things.TaskScheduleAnytime {
			return true
		}
	}
	return false
}

func countIncludedTasks(state *sync.State, tasks []*things.Task) int {
	count := 0
	for _, task := range tasks {
		if task == nil || shouldExcludeRoutineTaskWithState(state, task) || task.Status != things.TaskStatusPending || task.InTrash {
			continue
		}
		count++
	}
	return count
}

func weeklyHighlights(byProject map[string]int) []string {
	type kv struct {
		key   string
		value int
	}
	pairs := make([]kv, 0, len(byProject))
	for key, value := range byProject {
		pairs = append(pairs, kv{key: key, value: value})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].value != pairs[j].value {
			return pairs[i].value > pairs[j].value
		}
		return pairs[i].key < pairs[j].key
	})
	out := make([]string, 0, min(3, len(pairs)))
	for i := 0; i < len(pairs) && i < 3; i++ {
		out = append(out, fmt.Sprintf("%s drove %d completed tasks", pairs[i].key, pairs[i].value))
	}
	return out
}

func (svc *briefingService) rolloversForPeriod(start, end time.Time) []weeklyRollover {
	type rolloverState struct {
		title string
		dates map[string]struct{}
	}
	seen := map[string]*rolloverState{}
	briefings, err := svc.store.listLatestDailyInRange(start, end)
	if err != nil {
		return nil
	}
	for _, briefing := range briefings {
		if briefing == nil {
			continue
		}
		dateKey := briefing.Date
		for _, task := range briefing.TodayTasks.Tasks {
			item := seen[task.UUID]
			if item == nil {
				item = &rolloverState{title: task.Title, dates: map[string]struct{}{}}
				seen[task.UUID] = item
			}
			item.title = task.Title
			item.dates[dateKey] = struct{}{}
		}
		for _, task := range briefing.Overdue {
			item := seen[task.UUID]
			if item == nil {
				item = &rolloverState{title: task.Title, dates: map[string]struct{}{}}
				seen[task.UUID] = item
			}
			item.title = task.Title
			item.dates[dateKey] = struct{}{}
		}
	}

	out := make([]weeklyRollover, 0)
	for uuid, item := range seen {
		if len(item.dates) < 2 {
			continue
		}
		out = append(out, weeklyRollover{
			UUID:             uuid,
			Title:            item.title,
			TimesRescheduled: len(item.dates),
			Recommendation:   rolloverRecommendation(len(item.dates)),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TimesRescheduled != out[j].TimesRescheduled {
			return out[i].TimesRescheduled > out[j].TimesRescheduled
		}
		return out[i].Title < out[j].Title
	})
	return out
}

func isTaskOnDate(task *things.Task, dayStart, nextDay time.Time) bool {
	if task == nil {
		return false
	}
	return (task.TodayIndexReference != nil && !task.TodayIndexReference.Before(dayStart) && task.TodayIndexReference.Before(nextDay)) ||
		(task.ScheduledDate != nil && !task.ScheduledDate.Before(dayStart) && task.ScheduledDate.Before(nextDay))
}

func isOverdueAnytimeTask(task *things.Task, dayStart, nextDay time.Time) bool {
	return task != nil &&
		task.Schedule == things.TaskScheduleAnytime &&
		task.Status == things.TaskStatusPending &&
		task.ScheduledDate != nil &&
		task.ScheduledDate.Before(dayStart) &&
		!isTaskOnDate(task, dayStart, nextDay)
}

func hasDeadlineOnDay(task *things.Task, dayStart, nextDay time.Time) bool {
	return task != nil &&
		task.Status == things.TaskStatusPending &&
		task.DeadlineDate != nil &&
		!task.DeadlineDate.Before(dayStart) &&
		task.DeadlineDate.Before(nextDay)
}

func taskScheduleAnchor(task *things.Task) *time.Time {
	switch {
	case task == nil:
		return nil
	case task.ScheduledDate != nil:
		return task.ScheduledDate
	default:
		return task.TodayIndexReference
	}
}

func taskScheduleString(task *things.Task) *string {
	if anchor := taskScheduleAnchor(task); anchor != nil {
		return dateStringPtr(anchor)
	}
	return nil
}

func staleReason(stale bool, days int) string {
	if !stale {
		return ""
	}
	return fmt.Sprintf("Scheduled %d days ago, no progress visible", days)
}

func improvementSuggestion(title, effort string) string {
	switch effort {
	case "deep":
		return "Break this into a first deliverable and a 60-minute focused block"
	case "mini":
		return "Make the next visible action explicit and clear it in one pass"
	default:
		if strings.Contains(strings.ToLower(title), "review") || strings.Contains(strings.ToLower(title), "plan") {
			return "Define the concrete output before starting"
		}
		return "Break into: 1) clarify scope 2) do the first concrete step"
	}
}

func overdueRecommendation(days int) string {
	switch {
	case days >= 4:
		return "rewrite"
	case days >= 2:
		return "reschedule"
	default:
		return "tackle_today"
	}
}

func aiSuggestionReason(title string) string {
	lower := strings.ToLower(title)
	switch {
	case containsAny(lower, "draft", "write", "rewrite", "summary", "summarise", "outline"):
		return "Looks like drafting or rewriting work that Claude can help execute."
	case containsAny(lower, "research", "analyse", "analyze", "overview", "business case", "report", "proposal"):
		return "Looks like analysis or synthesis work that Claude can help execute."
	case containsAny(lower, "prepare", "presentation", "plan", "ideas", "brainstorm"):
		return "Looks like preparation work where Claude can help produce a first pass."
	default:
		return ""
	}
}

func matchingExclusiveTags(tags []string, allowed ...string) []string {
	out := []string{}
	for _, allow := range allowed {
		if containsString(tags, allow) {
			out = append(out, allow)
		}
	}
	return out
}

func sortWeeklyTaskSummaries(items []weeklyTaskSummary) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Project != items[j].Project {
			return items[i].Project < items[j].Project
		}
		return items[i].Title < items[j].Title
	})
}

func sortWeeklyTagIssues(items []weeklyTagIssue) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Project != items[j].Project {
			return items[i].Project < items[j].Project
		}
		return items[i].Title < items[j].Title
	})
}

func sortReviewTaskCues(items []reviewTaskCue) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Project != items[j].Project {
			return items[i].Project < items[j].Project
		}
		return items[i].Title < items[j].Title
	})
}

func limitWeeklyTaskSummaries(items []weeklyTaskSummary, limit int) []weeklyTaskSummary {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func limitWeeklyTagIssues(items []weeklyTagIssue, limit int) []weeklyTagIssue {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func limitReviewTaskCues(items []reviewTaskCue, limit int) []reviewTaskCue {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func estimateEffort(tags []string, title string) string {
	lowerTitle := strings.ToLower(title)
	for _, tag := range tags {
		lower := strings.ToLower(tag)
		if strings.Contains(lower, "deep") || strings.Contains(lower, "focus") {
			return "deep"
		}
		if strings.Contains(lower, "mini") || strings.Contains(lower, "quick") {
			return "mini"
		}
	}
	switch {
	case containsAny(lowerTitle, "design", "write", "draft", "build", "research", "presentation"):
		return "deep"
	case containsAny(lowerTitle, "reply", "email", "text", "book", "pay", "send"):
		return "mini"
	default:
		return "standard"
	}
}

func effortRank(effort string) int {
	switch effort {
	case "deep":
		return 3
	case "standard":
		return 2
	case "mini":
		return 1
	default:
		return 0
	}
}

func effortHours(effort string) float64 {
	switch effort {
	case "deep":
		return 1.5
	case "mini":
		return 0.25
	default:
		return 0.5
	}
}

func overdueEffortHours(days int) float64 {
	switch {
	case days >= 4:
		return 0.5
	case days >= 2:
		return 0.75
	default:
		return 0.5
	}
}

func suggestProject(title string, projects []projectSuggestion) (string, string) {
	titleTokens := titleTokens(title)
	bestScore := 0
	bestTitle := ""
	bestUUID := ""
	for _, project := range projects {
		score := overlapScore(titleTokens, project.tokens)
		if score > bestScore {
			bestScore = score
			bestTitle = project.title
			bestUUID = project.uuid
		}
	}
	if bestScore > 0 {
		return bestTitle, bestUUID
	}

	lower := strings.ToLower(title)
	switch {
	case containsAny(lower, "home", "flat", "house", "laundry", "kitchen"):
		for _, project := range projects {
			if strings.Contains(strings.ToLower(project.title), "home") {
				return project.title, project.uuid
			}
		}
	case containsAny(lower, "work", "client", "meeting", "deploy"):
		for _, project := range projects {
			if strings.Contains(strings.ToLower(project.title), "work") || strings.Contains(strings.ToLower(project.title), "modulaire") {
				return project.title, project.uuid
			}
		}
	}
	return "", ""
}

func suggestTags(title, when string, dayStart time.Time, tagByTitle map[string]*things.Tag) ([]string, []string) {
	lower := strings.ToLower(title)
	outTitles := []string{}
	outUUIDs := []string{}
	add := func(name string) {
		tag := tagByTitle[strings.ToLower(name)]
		outTitles = append(outTitles, name)
		if tag != nil {
			outUUIDs = append(outUUIDs, tag.UUID)
		}
	}

	switch {
	case containsAny(lower, "meeting", "client", "deploy", "review", "prod", "bug"):
		add("Work")
	case containsAny(lower, "home", "groceries", "clean", "family", "personal"):
		add("Personal")
	}

	if shouldUseNowTagForSuggestion(when, dayStart) {
		add("Now")
	} else if containsAny(lower, "next", "follow up", "reply", "book", "pay", "send") {
		add("Next")
	}

	if !containsString(outTitles, "Now") && len(outTitles) == 0 {
		if _, ok := tagByTitle["next"]; ok {
			add("Next")
		}
	}
	return dedupeStrings(outTitles), dedupeStrings(outUUIDs)
}

func suggestWhen(title string, dayStart time.Time) string {
	lower := strings.ToLower(title)
	switch {
	case containsAny(lower, "today", "urgent", "asap", "reply", "meeting"):
		return dayStart.Format("2006-01-02")
	case containsAny(lower, "weekend", "someday"):
		return dayStart.AddDate(0, 0, 5).Format("2006-01-02")
	default:
		return dayStart.AddDate(0, 0, 3).Format("2006-01-02")
	}
}

func shouldUseNowTagForSuggestion(when string, dayStart time.Time) bool {
	if when == "" {
		return false
	}
	scheduled, err := parseBriefingDate(when)
	if err != nil {
		return false
	}
	scheduled = utcDay(scheduled)
	dayStart = utcDay(dayStart)
	if scheduled.Before(dayStart) {
		return false
	}
	weekStart := startOfWeek(dayStart, time.Monday)
	weekEnd := weekStart.AddDate(0, 0, 7)
	return scheduled.Before(weekEnd)
}

func inboxReasoning(title, project string, tags []string, when string, dayStart time.Time) string {
	lower := strings.ToLower(title)
	if project != "" && containsAny(lower, "home", "house", "laundry", "bill") {
		return "Looks like home/admin work and can be triaged into an existing project"
	}
	if containsAny(lower, "meeting", "demo", "review") {
		return "Time-bound work item; keep it close to the calendar date"
	}
	if when != dayStart.Format("2006-01-02") {
		return "Looks important enough to keep, but not urgent enough for today"
	}
	if len(tags) > 0 {
		return fmt.Sprintf("Fits the %s context and should stay visible", strings.Join(tags, "/"))
	}
	return "Needs quick triage into a concrete project and date"
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func rolloverRecommendation(count int) string {
	switch {
	case count >= 4:
		return "drop"
	case count == 3:
		return "break_down"
	case count == 2:
		return "rewrite"
	default:
		return "demote"
	}
}

func projectDisplayName(state *sync.State, task *things.Task) string {
	if task == nil {
		return ""
	}
	if projectUUID := widgetRootProjectUUID(state, task); projectUUID != "" {
		project, err := state.Task(projectUUID)
		if err == nil && project != nil && project.Title != "" {
			return project.Title
		}
	}
	if len(task.AreaIDs) > 0 {
		area, err := state.Area(task.AreaIDs[0])
		if err == nil && area != nil {
			return area.Title
		}
	}
	return ""
}

func tagNamesForTask(task *things.Task, tagTitles map[string]string) []string {
	out := make([]string, 0, len(task.TagIDs))
	for _, id := range task.TagIDs {
		title := tagTitles[id]
		if title == "" {
			continue
		}
		out = append(out, title)
	}
	sort.Strings(out)
	return out
}

func titleTokens(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	filtered := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) <= 2 {
			continue
		}
		filtered = append(filtered, field)
	}
	return dedupeStrings(filtered)
}

func overlapScore(a, b []string) int {
	set := map[string]struct{}{}
	for _, value := range a {
		set[value] = struct{}{}
	}
	score := 0
	for _, value := range b {
		if _, ok := set[value]; ok {
			score++
		}
	}
	return score
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func dateStringPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	value := utcDay(*t).Format("2006-01-02")
	return &value
}

func daysSinceAnchor(anchor *time.Time, dayStart time.Time) int {
	if anchor == nil {
		return 0
	}
	diff := int(dayStart.Sub(utcDay(*anchor)).Hours() / 24)
	if diff < 0 {
		return 0
	}
	return diff
}

func utcDay(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func localDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func parseBriefingDate(raw string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, fmt.Errorf("date must be YYYY-MM-DD")
	}
	return utcDay(date), nil
}

func parseISOWeek(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, "-W")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("week must be YYYY-Www")
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("week must be YYYY-Www")
	}
	week, err := strconv.Atoi(parts[1])
	if err != nil || week < 1 || week > 53 {
		return time.Time{}, fmt.Errorf("week must be YYYY-Www")
	}

	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	weekday := int(jan4.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	firstISOWeekStart := jan4.AddDate(0, 0, -(weekday - 1))
	weekStart := firstISOWeekStart.AddDate(0, 0, (week-1)*7)
	gotYear, gotWeek := weekStart.ISOWeek()
	if gotYear != year || gotWeek != week {
		return time.Time{}, fmt.Errorf("week must be YYYY-Www")
	}
	return weekStart, nil
}

func formatISOWeek(t time.Time) string {
	year, week := utcDay(t).ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func reviewTargetForDate(date time.Time) (kind string, key string) {
	day := utcDay(date)
	switch day.Weekday() {
	case time.Saturday, time.Sunday:
		return "weekly", formatISOWeek(nextMonday(day))
	default:
		return "daily", day.Format("2006-01-02")
	}
}

func nextMonday(day time.Time) time.Time {
	day = utcDay(day)
	offset := (int(time.Monday) - int(day.Weekday()) + 7) % 7
	if offset == 0 {
		offset = 7
	}
	return day.AddDate(0, 0, offset)
}

func handlePutDailyBriefing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	date, err := parseBriefingDate(r.PathValue("date"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	briefing, err := newBriefingService().GenerateDaily(r.Context(), date)
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to generate daily briefing: %v", err), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, briefing)
}

func handlePutWeeklyBriefing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	week := strings.TrimSpace(r.PathValue("week"))
	if _, err := parseISOWeek(week); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	briefing, err := newBriefingService().GenerateWeekly(r.Context(), week)
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to generate weekly briefing: %v", err), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, briefing)
}

func readCachedBriefing(kind, key string) (any, error) {
	store := newBriefingStore(resolveBriefingDBPath(), time.Now)
	switch kind {
	case "daily":
		return store.readDaily(key)
	case "weekly":
		return store.readWeekly(key)
	default:
		return nil, errBriefingNotFound
	}
}
