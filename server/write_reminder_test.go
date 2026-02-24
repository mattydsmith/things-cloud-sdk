package main

import "testing"

func TestParseReminder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input   string
		wantSec int
		wantOK  bool
	}{
		{"09:00", 32400, true},   // 9h * 3600
		{"14:30", 52200, true},   // 14*3600 + 30*60
		{"00:00", 0, true},       // midnight
		{"23:59", 86340, true},   // 23*3600 + 59*60
		{"9:00", 32400, true},    // single-digit hour
		{"24:00", 0, false},      // hour out of range
		{"12:60", 0, false},      // minute out of range
		{"-1:00", 0, false},      // negative hour
		{"noon", 0, false},       // not HH:MM
		{"", 0, false},           // empty
		{"12", 0, false},         // no colon
		{"12:00:00", 0, false},   // extra colon — Atoi("00:00") fails
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, ok := parseReminder(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("parseReminder(%q): ok=%v, want %v", tc.input, ok, tc.wantOK)
			}
			if ok && got != tc.wantSec {
				t.Fatalf("parseReminder(%q) = %d, want %d", tc.input, got, tc.wantSec)
			}
		})
	}
}

func TestReminderOnTaskUpdate(t *testing.T) {
	t.Parallel()
	u := newTaskUpdate()
	u.reminder(32400) // 09:00

	fields := u.build()
	ato, ok := fields["ato"]
	if !ok {
		t.Fatal("expected ato field in update")
	}
	if ato.(int) != 32400 {
		t.Fatalf("expected ato=32400, got %v", ato)
	}
}

func TestClearReminderOnTaskUpdate(t *testing.T) {
	t.Parallel()
	u := newTaskUpdate()
	u.reminder(32400)
	u.clearReminder()

	fields := u.build()
	ato, ok := fields["ato"]
	if !ok {
		t.Fatal("expected ato field in update (should be nil)")
	}
	if ato != nil {
		t.Fatalf("expected ato=nil after clearReminder, got %v", ato)
	}
}
