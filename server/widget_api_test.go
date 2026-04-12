package main

import (
	"errors"
	"testing"

	things "github.com/arthursoares/things-cloud-sdk"
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
