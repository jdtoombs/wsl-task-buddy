package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tmpPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "tasks.json")
}

func TestLoadNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	data, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.NextID != 1 {
		t.Errorf("expected NextID=1, got %d", data.NextID)
	}
	if len(data.Tasks) != 0 {
		t.Errorf("expected empty tasks, got %d", len(data.Tasks))
	}
}

func TestLoadCorruptFile(t *testing.T) {
	path := tmpPath(t)
	os.WriteFile(path, []byte("not json"), 0644)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for corrupt file")
	}
}

func TestLoadIDBackfill(t *testing.T) {
	path := tmpPath(t)
	// Write tasks with no IDs (simulating old format)
	os.WriteFile(path, []byte(`{"next_id":0,"tasks":[{"title":"a","date":"2025-01-01"},{"title":"b","date":"2025-01-01"}]}`), 0644)

	data, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Tasks[0].ID == 0 || data.Tasks[1].ID == 0 {
		t.Error("IDs should be backfilled")
	}
	if data.Tasks[0].ID == data.Tasks[1].ID {
		t.Error("IDs should be unique")
	}
	if data.NextID <= data.Tasks[1].ID {
		t.Error("NextID should be greater than all assigned IDs")
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := tmpPath(t)
	data := TaskData{NextID: 3, Tasks: []Task{
		{ID: 1, Title: "task one", Date: "2025-01-01"},
		{ID: 2, Title: "task two", Date: "2025-01-02", Done: true},
	}}
	if err := Save(path, data); err != nil {
		t.Fatalf("save error: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if len(loaded.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(loaded.Tasks))
	}
	if loaded.Tasks[0].Title != "task one" {
		t.Errorf("expected 'task one', got %q", loaded.Tasks[0].Title)
	}
	if !loaded.Tasks[1].Done {
		t.Error("task two should be done")
	}
}

func TestAddTask(t *testing.T) {
	data := TaskData{NextID: 1}
	task := AddTask(&data, "my task", "2025-03-01")
	if task.ID != 1 {
		t.Errorf("expected ID=1, got %d", task.ID)
	}
	if data.NextID != 2 {
		t.Errorf("expected NextID=2, got %d", data.NextID)
	}
	if len(data.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(data.Tasks))
	}
	if data.Tasks[0].Title != "my task" {
		t.Errorf("expected 'my task', got %q", data.Tasks[0].Title)
	}
}

func TestToggleDone(t *testing.T) {
	data := TaskData{NextID: 2, Tasks: []Task{{ID: 1, Title: "t", Date: "2025-01-01"}}}
	task, err := ToggleDone(&data, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !task.Done {
		t.Error("task should be done")
	}
	task, err = ToggleDone(&data, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Done {
		t.Error("task should be undone")
	}
}

func TestToggleDoneNotFound(t *testing.T) {
	data := TaskData{NextID: 1}
	_, err := ToggleDone(&data, 99)
	if err == nil {
		t.Error("expected error for missing task")
	}
}

func TestDeleteTask(t *testing.T) {
	data := TaskData{NextID: 3, Tasks: []Task{
		{ID: 1, Title: "a", Date: "2025-01-01"},
		{ID: 2, Title: "b", Date: "2025-01-01"},
	}}
	if err := DeleteTask(&data, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(data.Tasks))
	}
	if data.Tasks[0].ID != 2 {
		t.Errorf("expected remaining task ID=2, got %d", data.Tasks[0].ID)
	}
}

func TestDeleteTaskNotFound(t *testing.T) {
	data := TaskData{NextID: 1}
	if err := DeleteTask(&data, 99); err == nil {
		t.Error("expected error for missing task")
	}
}

func TestTimerStartStop(t *testing.T) {
	data := TaskData{NextID: 2, Tasks: []Task{{ID: 1, Title: "t", Date: "2025-01-01"}}}
	task, err := StartTimer(&data, 1)
	if err != nil {
		t.Fatalf("start error: %v", err)
	}
	if !task.IsRunning() {
		t.Error("task should be running after start")
	}
	if FindRunningTimerID(data) != 1 {
		t.Error("FindRunningTimerID should return 1")
	}

	task, err = StopTimer(&data, 1)
	if err != nil {
		t.Fatalf("stop error: %v", err)
	}
	if task.IsRunning() {
		t.Error("task should not be running after stop")
	}
	if FindRunningTimerID(data) != -1 {
		t.Error("FindRunningTimerID should return -1")
	}
}

func TestStartTimerStopsExisting(t *testing.T) {
	data := TaskData{NextID: 3, Tasks: []Task{
		{ID: 1, Title: "a", Date: "2025-01-01"},
		{ID: 2, Title: "b", Date: "2025-01-01"},
	}}
	StartTimer(&data, 1)
	StartTimer(&data, 2)

	if data.Tasks[0].IsRunning() {
		t.Error("first task timer should have been stopped")
	}
	if !data.Tasks[1].IsRunning() {
		t.Error("second task should be running")
	}
}

func TestStopTimerNotRunning(t *testing.T) {
	data := TaskData{NextID: 2, Tasks: []Task{{ID: 1, Title: "t", Date: "2025-01-01"}}}
	_, err := StopTimer(&data, 1)
	if err == nil {
		t.Error("expected error when stopping a non-running timer")
	}
}

func TestStopAllTimers(t *testing.T) {
	now := time.Now()
	data := TaskData{NextID: 3, Tasks: []Task{
		{ID: 1, Title: "a", Date: "2025-01-01", Entries: []TimeEntry{{Start: now}}},
		{ID: 2, Title: "b", Date: "2025-01-01", Entries: []TimeEntry{{Start: now}}},
	}}
	StopAllTimers(&data)
	for _, task := range data.Tasks {
		if task.IsRunning() {
			t.Errorf("task %d should not be running", task.ID)
		}
	}
}

func TestTasksForDate(t *testing.T) {
	data := TaskData{NextID: 4, Tasks: []Task{
		{ID: 1, Title: "a", Date: "2025-01-01"},
		{ID: 2, Title: "b", Date: "2025-01-02"},
		{ID: 3, Title: "c", Date: "2025-01-01"},
	}}
	tasks := TasksForDate(data, "2025-01-01")
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestTaskByID(t *testing.T) {
	data := TaskData{NextID: 3, Tasks: []Task{
		{ID: 1, Title: "a", Date: "2025-01-01"},
		{ID: 2, Title: "b", Date: "2025-01-01"},
	}}
	task, idx, ok := TaskByID(data, 2)
	if !ok {
		t.Fatal("expected to find task")
	}
	if idx != 1 {
		t.Errorf("expected index 1, got %d", idx)
	}
	if task.Title != "b" {
		t.Errorf("expected 'b', got %q", task.Title)
	}

	_, _, ok = TaskByID(data, 99)
	if ok {
		t.Error("should not find nonexistent task")
	}
}

func TestTotalTime(t *testing.T) {
	now := time.Now()
	end := now.Add(5 * time.Minute)
	task := Task{
		Entries: []TimeEntry{
			{Start: now, End: &end},
		},
	}
	total := task.TotalTime()
	if total < 4*time.Minute || total > 6*time.Minute {
		t.Errorf("expected ~5m, got %v", total)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"},
		{30 * time.Second, "0:30"},
		{5*time.Minute + 3*time.Second, "5:03"},
		{1*time.Hour + 2*time.Minute + 3*time.Second, "1:02:03"},
		{10*time.Hour + 0*time.Minute + 0*time.Second, "10:00:00"},
	}
	for _, tt := range tests {
		got := FormatDuration(tt.d)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFindRunningTimerIDNone(t *testing.T) {
	data := TaskData{NextID: 2, Tasks: []Task{{ID: 1, Title: "t", Date: "2025-01-01"}}}
	if id := FindRunningTimerID(data); id != -1 {
		t.Errorf("expected -1, got %d", id)
	}
}
