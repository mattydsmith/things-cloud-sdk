package main

import (
	"testing"

	thingscloud "github.com/arthursoares/things-cloud-sdk"
)

func TestTaskUpdateForMoveToToday_PreservesInboxSchedule(t *testing.T) {
	t.Parallel()

	task := &thingscloud.Task{Schedule: thingscloud.TaskScheduleInbox}
	update := taskUpdateForMoveToToday(task)
	today := todayMidnightUTC()

	if got := update.fields["st"]; got != 0 {
		t.Fatalf("expected inbox schedule 0, got %#v", got)
	}
	if got := update.fields["sr"]; got != nil {
		t.Fatalf("expected nil scheduled date for inbox task, got %#v", got)
	}
	if got := update.fields["tir"]; got != today {
		t.Fatalf("expected today-index reference %d, got %#v", today, got)
	}
}

func TestTaskUpdateForMoveToToday_TriagesNonInboxTask(t *testing.T) {
	t.Parallel()

	task := &thingscloud.Task{Schedule: thingscloud.TaskScheduleAnytime}
	update := taskUpdateForMoveToToday(task)
	today := todayMidnightUTC()

	if got := update.fields["st"]; got != 1 {
		t.Fatalf("expected anytime schedule 1, got %#v", got)
	}
	if got := update.fields["sr"]; got != today {
		t.Fatalf("expected scheduled date %d, got %#v", today, got)
	}
	if got := update.fields["tir"]; got != today {
		t.Fatalf("expected today-index reference %d, got %#v", today, got)
	}
}

func TestTaskUpdateForRemoveFromToday_PreservesInboxSchedule(t *testing.T) {
	t.Parallel()

	task := &thingscloud.Task{Schedule: thingscloud.TaskScheduleInbox}
	update := taskUpdateForRemoveFromToday(task)

	if got := update.fields["st"]; got != 0 {
		t.Fatalf("expected inbox schedule 0, got %#v", got)
	}
	if got := update.fields["sr"]; got != nil {
		t.Fatalf("expected nil scheduled date for inbox task, got %#v", got)
	}
	if got := update.fields["tir"]; got != nil {
		t.Fatalf("expected nil today-index reference for inbox task, got %#v", got)
	}
}

func TestTaskUpdateForRemoveFromToday_TriagesToAnytime(t *testing.T) {
	t.Parallel()

	task := &thingscloud.Task{Schedule: thingscloud.TaskScheduleAnytime}
	update := taskUpdateForRemoveFromToday(task)

	if got := update.fields["st"]; got != 1 {
		t.Fatalf("expected anytime schedule 1, got %#v", got)
	}
	if got := update.fields["sr"]; got != nil {
		t.Fatalf("expected nil scheduled date, got %#v", got)
	}
	if got := update.fields["tir"]; got != nil {
		t.Fatalf("expected nil today-index reference, got %#v", got)
	}
}
