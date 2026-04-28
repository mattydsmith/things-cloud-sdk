package main

import (
	"encoding/json"
	"testing"

	things "github.com/arthursoares/things-cloud-sdk"
)

func TestBuildTaskDetailResponse(t *testing.T) {
	t.Parallel()

	task := &things.Task{
		UUID:   "task-1",
		Title:  "Example",
		Status: things.TaskStatusPending,
	}

	t.Run("includes zero checklist count", func(t *testing.T) {
		t.Parallel()

		resp := buildTaskDetailResponse(task, nil)
		if resp.ChecklistCount != 0 {
			t.Fatalf("ChecklistCount = %d, want 0", resp.ChecklistCount)
		}

		b, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var got map[string]any
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if got["ChecklistCount"] != float64(0) {
			t.Fatalf("ChecklistCount = %v, want 0", got["ChecklistCount"])
		}
	})

	t.Run("includes non-zero checklist count", func(t *testing.T) {
		t.Parallel()

		resp := buildTaskDetailResponse(task, []*things.CheckListItem{
			{UUID: "c1"},
			{UUID: "c2"},
		})
		if resp.ChecklistCount != 2 {
			t.Fatalf("ChecklistCount = %d, want 2", resp.ChecklistCount)
		}
	})
}
