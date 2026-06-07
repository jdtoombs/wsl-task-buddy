package main

import (
	"testing"
	"time"

	"wsl-task-buddy/store"
)

func TestToggleContext(t *testing.T) {
	m := model{activeContext: store.TaskContextWork, cursor: 3}
	m.toggleContext()
	if m.currentContext() != store.TaskContextPersonal {
		t.Fatalf("expected personal context, got %q", m.currentContext())
	}
	if m.cursor != 0 {
		t.Errorf("expected cursor reset to 0, got %d", m.cursor)
	}
	m.toggleContext()
	if m.currentContext() != store.TaskContextWork {
		t.Fatalf("expected work context, got %q", m.currentContext())
	}
}

func TestModelTasksForDateFiltersByContext(t *testing.T) {
	date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	m := model{
		date:          date,
		activeContext: store.TaskContextPersonal,
		data: store.TaskData{Tasks: []store.Task{
			{ID: 1, Title: "legacy personal", Date: "2025-01-01"},
			{ID: 2, Title: "personal", Date: "2025-01-01", Context: store.TaskContextPersonal},
			{ID: 3, Title: "work", Date: "2025-01-01", Context: store.TaskContextWork},
		}},
	}
	if indices := m.tasksForDate(); len(indices) != 2 {
		t.Fatalf("expected 2 personal tasks, got %d", len(indices))
	}
	m.activeContext = store.TaskContextWork
	indices := m.tasksForDate()
	if len(indices) != 1 {
		t.Fatalf("expected 1 work task, got %d", len(indices))
	}
	if m.data.Tasks[indices[0]].Title != "work" {
		t.Errorf("expected work task, got %q", m.data.Tasks[indices[0]].Title)
	}
}

func TestParseDurationInput(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		// H:MM format
		{"1:30", 1*time.Hour + 30*time.Minute, false},
		{"0:45", 45 * time.Minute, false},
		{"2:00", 2 * time.Hour, false},

		// XhYm format
		{"1h30m", 1*time.Hour + 30*time.Minute, false},
		{"45m", 45 * time.Minute, false},
		{"2h", 2 * time.Hour, false},

		// plain minutes
		{"30", 30 * time.Minute, false},
		{"90", 90 * time.Minute, false},

		// errors
		{"", 0, true},
		{"0:00", 0, true},
		{"-1:30", 0, true},
		{"1:-30", 0, true},
		{"0m", 0, true},
		{"0h", 0, true},
		{"0", 0, true},
		{"-5", 0, true},
		{"xh30m", 0, true},
		{"1hxm", 0, true},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		got, err := parseDurationInput(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseDurationInput(%q) expected error, got %v", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDurationInput(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseDurationInput(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
