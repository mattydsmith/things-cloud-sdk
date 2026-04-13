package main

import (
	"errors"
	"testing"
	"time"

	things "github.com/arthursoares/things-cloud-sdk"
	"github.com/arthursoares/things-cloud-sdk/sync"
)

type stubWidgetLookup struct {
	tasks map[string]*things.Task
	areas map[string]*things.Area
	err   error
}

func (s stubWidgetLookup) Task(uuid string) (*things.Task, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.tasks[uuid], nil
}

func (s stubWidgetLookup) Area(uuid string) (*things.Area, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.areas[uuid], nil
}

func TestFormatWidgetTodayItem(t *testing.T) {
	t.Parallel()

	t.Run("uses parent task title when present", func(t *testing.T) {
		t.Parallel()

		item := formatWidgetTodayItem(stubWidgetLookup{
			tasks: map[string]*things.Task{
				"parent-1": {UUID: "parent-1", Title: "Home"},
			},
		}, &things.Task{
			UUID:          "task-1",
			Title:         "Sort out HBO Max and sky",
			ParentTaskIDs: []string{"parent-1"},
			Status:        things.TaskStatusPending,
		})

		if item.ProjectName != "Home" {
			t.Fatalf("ProjectName = %q, want %q", item.ProjectName, "Home")
		}
		if item.IsCompleted {
			t.Fatal("expected IsCompleted to be false")
		}
		if item.Deadline != "" {
			t.Fatalf("Deadline = %q, want empty", item.Deadline)
		}
	})

	t.Run("falls back to area title", func(t *testing.T) {
		t.Parallel()

		item := formatWidgetTodayItem(stubWidgetLookup{
			areas: map[string]*things.Area{
				"area-1": {UUID: "area-1", Title: "Work"},
			},
		}, &things.Task{
			UUID:    "task-2",
			Title:   "Reply to notes",
			AreaIDs: []string{"area-1"},
			Status:  things.TaskStatusCompleted,
		})

		if item.ProjectName != "Work" {
			t.Fatalf("ProjectName = %q, want %q", item.ProjectName, "Work")
		}
		if !item.IsCompleted {
			t.Fatal("expected IsCompleted to be true")
		}
	})

	t.Run("returns empty project name when lookups fail", func(t *testing.T) {
		t.Parallel()

		item := formatWidgetTodayItem(stubWidgetLookup{
			err: errors.New("boom"),
		}, &things.Task{
			UUID:          "task-3",
			Title:         "Loose task",
			ParentTaskIDs: []string{"parent-2"},
			AreaIDs:       []string{"area-2"},
			Status:        things.TaskStatusPending,
		})

		if item.ProjectName != "" {
			t.Fatalf("ProjectName = %q, want empty", item.ProjectName)
		}
	})

	t.Run("includes formatted deadline when present", func(t *testing.T) {
		t.Parallel()

		deadline := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)
		item := formatWidgetTodayItem(stubWidgetLookup{}, &things.Task{
			UUID:         "task-4",
			Title:        "Has deadline",
			Status:       things.TaskStatusPending,
			DeadlineDate: &deadline,
		})

		if item.Deadline != "2026-04-13" {
			t.Fatalf("Deadline = %q, want %q", item.Deadline, "2026-04-13")
		}
	})
}

func TestIsOverdueOpenTask(t *testing.T) {
	t.Parallel()

	todayStart := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	yesterday := todayStart.Add(-24 * time.Hour)
	tomorrow := todayStart.Add(24 * time.Hour)

	tests := []struct {
		name string
		task *things.Task
		want bool
	}{
		{
			name: "open task scheduled before today is overdue",
			task: &things.Task{Status: things.TaskStatusPending, ScheduledDate: &yesterday},
			want: true,
		},
		{
			name: "open task scheduled today is not overdue",
			task: &things.Task{Status: things.TaskStatusPending, ScheduledDate: &todayStart},
			want: false,
		},
		{
			name: "open task scheduled in future is not overdue",
			task: &things.Task{Status: things.TaskStatusPending, ScheduledDate: &tomorrow},
			want: false,
		},
		{
			name: "completed task is not overdue for widget",
			task: &things.Task{Status: things.TaskStatusCompleted, ScheduledDate: &yesterday},
			want: false,
		},
		{
			name: "task without scheduled date is not overdue",
			task: &things.Task{Status: things.TaskStatusPending},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isOverdueOpenTask(tt.task, todayStart); got != tt.want {
				t.Fatalf("isOverdueOpenTask() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPaginateWidgetTodayItems(t *testing.T) {
	t.Parallel()

	items := []widgetTodayItem{
		{UUID: "1"},
		{UUID: "2"},
		{UUID: "3"},
	}

	got := paginateWidgetTodayItems(items, sync.QueryOpts{Offset: 1, Limit: 1})
	if len(got) != 1 || got[0].UUID != "2" {
		t.Fatalf("unexpected paginated items: %+v", got)
	}

	got = paginateWidgetTodayItems(items, sync.QueryOpts{Offset: 10, Limit: 5})
	if len(got) != 0 {
		t.Fatalf("expected empty page, got %+v", got)
	}
}

func TestHasDueDeadline(t *testing.T) {
	t.Parallel()

	todayStart := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	tomorrowStart := todayStart.Add(24 * time.Hour)
	todayDeadline := todayStart
	oldDeadline := todayStart.Add(-24 * time.Hour)
	futureDeadline := tomorrowStart

	tests := []struct {
		name string
		task *things.Task
		want bool
	}{
		{"deadline today", &things.Task{Status: things.TaskStatusPending, DeadlineDate: &todayDeadline}, true},
		{"deadline overdue", &things.Task{Status: things.TaskStatusPending, DeadlineDate: &oldDeadline}, true},
		{"deadline future", &things.Task{Status: things.TaskStatusPending, DeadlineDate: &futureDeadline}, false},
		{"completed", &things.Task{Status: things.TaskStatusCompleted, DeadlineDate: &oldDeadline}, false},
		{"none", &things.Task{Status: things.TaskStatusPending}, false},
	}

	for _, tt := range tests {
		if got := hasDueDeadline(tt.task, tomorrowStart); got != tt.want {
			t.Fatalf("%s: hasDueDeadline() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSortWidgetTasks(t *testing.T) {
	t.Parallel()

	todayStart := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	yesterday := todayStart.Add(-24 * time.Hour)
	tomorrow := todayStart.Add(24 * time.Hour)

	tasks := []*things.Task{
		{UUID: "today", ScheduledDate: &todayStart, Index: 5, Status: things.TaskStatusPending},
		{UUID: "overdue", ScheduledDate: &yesterday, Index: 1, Status: things.TaskStatusPending},
		{UUID: "deadline-today", ScheduledDate: &todayStart, DeadlineDate: &todayStart, Index: 9, Status: things.TaskStatusPending},
		{UUID: "deadline-overdue", ScheduledDate: &yesterday, DeadlineDate: &yesterday, Index: 7, Status: things.TaskStatusPending},
		{UUID: "future-deadline", ScheduledDate: &todayStart, DeadlineDate: &tomorrow, Index: 0, Status: things.TaskStatusPending},
	}

	sortWidgetTasksAt(tasks, todayStart)
	got := []string{tasks[0].UUID, tasks[1].UUID, tasks[2].UUID, tasks[3].UUID, tasks[4].UUID}
	want := []string{"deadline-overdue", "deadline-today", "overdue", "future-deadline", "today"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted UUIDs = %v, want %v", got, want)
		}
	}
}

func TestWidgetIncludeTask(t *testing.T) {
	t.Parallel()

	t.Run("includes non-routines project tasks", func(t *testing.T) {
		t.Parallel()

		lookup := stubWidgetLookup{
			tasks: map[string]*things.Task{
				"proj-1": {UUID: "proj-1", Type: things.TaskTypeProject, Title: "Home"},
			},
		}

		if !widgetIncludeTask(lookup, &things.Task{
			UUID:          "task-1",
			ParentTaskIDs: []string{"proj-1"},
		}) {
			t.Fatal("expected non-routines task to be included")
		}
	})

	t.Run("excludes direct routines project tasks", func(t *testing.T) {
		t.Parallel()

		lookup := stubWidgetLookup{
			tasks: map[string]*things.Task{
				widgetExcludedProjectUUID: {UUID: widgetExcludedProjectUUID, Type: things.TaskTypeProject, Title: "Routines"},
			},
		}

		if widgetIncludeTask(lookup, &things.Task{
			UUID:          "task-2",
			ParentTaskIDs: []string{widgetExcludedProjectUUID},
		}) {
			t.Fatal("expected routines project task to be excluded")
		}
	})

	t.Run("excludes subtasks under routines project", func(t *testing.T) {
		t.Parallel()

		lookup := stubWidgetLookup{
			tasks: map[string]*things.Task{
				"parent-task":             {UUID: "parent-task", ParentTaskIDs: []string{widgetExcludedProjectUUID}},
				widgetExcludedProjectUUID: {UUID: widgetExcludedProjectUUID, Type: things.TaskTypeProject, Title: "Routines"},
			},
		}

		if widgetIncludeTask(lookup, &things.Task{
			UUID:          "task-3",
			ParentTaskIDs: []string{"parent-task"},
		}) {
			t.Fatal("expected nested routines task to be excluded")
		}
	})

	t.Run("excludes tasks in routines heading", func(t *testing.T) {
		t.Parallel()

		lookup := stubWidgetLookup{
			tasks: map[string]*things.Task{
				"heading-1":               {UUID: "heading-1", Type: things.TaskTypeHeading, ParentTaskIDs: []string{widgetExcludedProjectUUID}},
				widgetExcludedProjectUUID: {UUID: widgetExcludedProjectUUID, Type: things.TaskTypeProject, Title: "Routines"},
			},
		}

		if widgetIncludeTask(lookup, &things.Task{
			UUID:           "task-4",
			ActionGroupIDs: []string{"heading-1"},
		}) {
			t.Fatal("expected routines heading task to be excluded")
		}
	})
}
