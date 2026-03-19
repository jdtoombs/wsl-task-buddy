// store provides shared task data types and persistence for the tasks app
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type TimeEntry struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end,omitempty"`
}

type Task struct {
	ID             int         `json:"id"`
	Title          string      `json:"title"`
	Done           bool        `json:"done"`
	Date           string      `json:"date"`
	Notified       bool        `json:"notified,omitempty"`
	Entries        []TimeEntry `json:"entries,omitempty"`
	CarriedFromID  int         `json:"carried_from_id,omitempty"`
}

func (t Task) TotalTime() time.Duration {
	var total time.Duration
	for _, e := range t.Entries {
		if e.End != nil {
			total += e.End.Sub(e.Start)
		} else {
			total += time.Since(e.Start)
		}
	}
	return total
}

func (t Task) IsRunning() bool {
	return len(t.Entries) > 0 && t.Entries[len(t.Entries)-1].End == nil
}

type TaskData struct {
	NextID int    `json:"next_id"`
	Tasks  []Task `json:"tasks"`
}

func DataPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".tasks.json"), nil
}

func Load(path string) (TaskData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TaskData{NextID: 1}, nil
		}
		return TaskData{}, fmt.Errorf("read %s: %w", path, err)
	}
	var s TaskData
	if err := json.Unmarshal(data, &s); err != nil {
		return TaskData{}, fmt.Errorf("corrupt tasks file: %w", err)
	}
	changed := false
	if s.NextID == 0 {
		s.NextID = 1
		changed = true
	}
	for i := range s.Tasks {
		if s.Tasks[i].ID == 0 {
			s.Tasks[i].ID = s.NextID
			s.NextID++
			changed = true
		}
	}
	if changed {
		if err := Save(path, s); err != nil {
			return s, fmt.Errorf("backfill IDs: %w", err)
		}
	}
	return s, nil
}

func Save(path string, s TaskData) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func TasksForDate(s TaskData, date string) []Task {
	var tasks []Task
	for _, t := range s.Tasks {
		if t.Date == date {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

func TaskByID(s TaskData, id int) (Task, int, bool) {
	for i, t := range s.Tasks {
		if t.ID == id {
			return t, i, true
		}
	}
	return Task{}, -1, false
}

func AddTask(s *TaskData, title, date string) Task {
	t := Task{
		ID:    s.NextID,
		Title: title,
		Date:  date,
	}
	s.NextID++
	s.Tasks = append(s.Tasks, t)
	return t
}

func ToggleDone(s *TaskData, id int) (Task, error) {
	_, idx, ok := TaskByID(*s, id)
	if !ok {
		return Task{}, fmt.Errorf("task %d not found", id)
	}
	s.Tasks[idx].Done = !s.Tasks[idx].Done
	return s.Tasks[idx], nil
}

func DeleteTask(s *TaskData, id int) error {
	_, idx, ok := TaskByID(*s, id)
	if !ok {
		return fmt.Errorf("task %d not found", id)
	}
	s.Tasks = append(s.Tasks[:idx], s.Tasks[idx+1:]...)
	return nil
}

func StartTimer(s *TaskData, id int) (Task, error) {
	// stop any running timer first
	StopAllTimers(s)
	_, idx, ok := TaskByID(*s, id)
	if !ok {
		return Task{}, fmt.Errorf("task %d not found", id)
	}
	s.Tasks[idx].Entries = append(s.Tasks[idx].Entries, TimeEntry{Start: time.Now()})
	return s.Tasks[idx], nil
}

func StopTimer(s *TaskData, id int) (Task, error) {
	_, idx, ok := TaskByID(*s, id)
	if !ok {
		return Task{}, fmt.Errorf("task %d not found", id)
	}
	t := &s.Tasks[idx]
	if !t.IsRunning() {
		return *t, fmt.Errorf("task %d has no running timer", id)
	}
	now := time.Now()
	t.Entries[len(t.Entries)-1].End = &now
	return *t, nil
}

func StopAllTimers(s *TaskData) {
	now := time.Now()
	for i := range s.Tasks {
		if s.Tasks[i].IsRunning() {
			s.Tasks[i].Entries[len(s.Tasks[i].Entries)-1].End = &now
		}
	}
}

func FindRunningTimerID(s TaskData) int {
	for _, t := range s.Tasks {
		if t.IsRunning() {
			return t.ID
		}
	}
	return -1
}

// CarryForwardTasks copies incomplete tasks from past dates into today,
// marking the originals as done so they won't be carried again.
// Uses CarriedFromID to track origin, avoiding duplicates even after renames.
func CarryForwardTasks(s *TaskData, today string) bool {
	// Build set of source IDs already carried into today
	carriedIDs := make(map[int]bool)
	for _, t := range s.Tasks {
		if t.Date == today && t.CarriedFromID > 0 {
			carriedIDs[t.CarriedFromID] = true
		}
	}
	changed := false
	// range snapshots len; appended tasks won't be revisited
	for i, t := range s.Tasks {
		if t.Date >= today || t.Done {
			continue
		}
		if carriedIDs[t.ID] {
			continue
		}
		newTask := Task{
			ID:            s.NextID,
			Title:         t.Title,
			Date:          today,
			CarriedFromID: t.ID,
		}
		s.NextID++
		s.Tasks = append(s.Tasks, newTask)
		// Mark original as done so it won't be carried again
		s.Tasks[i].Done = true
		carriedIDs[t.ID] = true
		changed = true
	}
	return changed
}

func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
