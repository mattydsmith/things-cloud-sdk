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

func TestSortOverdueTasks(t *testing.T) {
	t.Parallel()

	older := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)
	sameDay := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)

	tasks := []*things.Task{
		{UUID: "c", ScheduledDate: &newer, Index: 3},
		{UUID: "a", ScheduledDate: &older, Index: 8},
		{UUID: "b", ScheduledDate: &sameDay, Index: 1},
	}

	sortOverdueTasks(tasks)

	got := []string{tasks[0].UUID, tasks[1].UUID, tasks[2].UUID}
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted UUIDs = %v, want %v", got, want)
		}
	}
}
