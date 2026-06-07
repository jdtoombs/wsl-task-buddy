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
	if data.Tasks[0].Context != TaskContextPersonal {
		t.Errorf("expected default context %q, got %q", TaskContextPersonal, data.Tasks[0].Context)
	}
}

func TestAddTaskWithContext(t *testing.T) {
	data := TaskData{NextID: 1}
	task := AddTask(&data, "work task", "2025-03-01", TaskContextWork)
	if task.Context != TaskContextWork {
		t.Errorf("expected context %q, got %q", TaskContextWork, task.Context)
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

func TestNormalizeTaskContext(t *testing.T) {
	tests := []struct {
		context string
		want    string
	}{
		{"", TaskContextPersonal},
		{"personal", TaskContextPersonal},
		{"PERSONAL", TaskContextPersonal},
		{"work", TaskContextWork},
		{" Work ", TaskContextWork},
		{"other", TaskContextPersonal},
	}
	for _, tt := range tests {
		if got := NormalizeTaskContext(tt.context); got != tt.want {
			t.Errorf("NormalizeTaskContext(%q) = %q, want %q", tt.context, got, tt.want)
		}
	}
}

func TestTasksForDateAndContext(t *testing.T) {
	data := TaskData{NextID: 5, Tasks: []Task{
		{ID: 1, Title: "legacy personal", Date: "2025-01-01"},
		{ID: 2, Title: "personal", Date: "2025-01-01", Context: TaskContextPersonal},
		{ID: 3, Title: "work", Date: "2025-01-01", Context: TaskContextWork},
		{ID: 4, Title: "tomorrow", Date: "2025-01-02", Context: TaskContextWork},
	}}

	personalTasks := TasksForDateAndContext(data, "2025-01-01", TaskContextPersonal)
	if len(personalTasks) != 2 {
		t.Fatalf("expected 2 personal tasks, got %d", len(personalTasks))
	}
	workTasks := TasksForDateAndContext(data, "2025-01-01", TaskContextWork)
	if len(workTasks) != 1 {
		t.Fatalf("expected 1 work task, got %d", len(workTasks))
	}
	if workTasks[0].Title != "work" {
		t.Errorf("expected work task, got %q", workTasks[0].Title)
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

func TestCarryForwardBasic(t *testing.T) {
	data := TaskData{NextID: 3, Tasks: []Task{
		{ID: 1, Title: "incomplete", Date: "2025-01-01", Done: false},
		{ID: 2, Title: "done task", Date: "2025-01-01", Done: true},
	}}
	changed := CarryForwardTasks(&data, "2025-01-05")
	if !changed {
		t.Fatal("expected changes")
	}
	if len(data.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(data.Tasks))
	}
	carried := data.Tasks[2]
	if carried.Title != "incomplete" || carried.Date != "2025-01-05" {
		t.Errorf("carried task wrong: %+v", carried)
	}
	if carried.CarriedFromID != 1 {
		t.Errorf("expected CarriedFromID=1, got %d", carried.CarriedFromID)
	}
	// Original should be marked done
	if !data.Tasks[0].Done {
		t.Error("original task should be marked done after carry")
	}
}

func TestCarryForwardSkipsDone(t *testing.T) {
	data := TaskData{NextID: 2, Tasks: []Task{
		{ID: 1, Title: "done", Date: "2025-01-01", Done: true},
	}}
	changed := CarryForwardTasks(&data, "2025-01-05")
	if changed {
		t.Error("should not carry done tasks")
	}
	if len(data.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(data.Tasks))
	}
}

func TestCarryForwardNoDuplicateAfterDelete(t *testing.T) {
	// Simulate: task was carried (original marked done), user deleted the carried copy.
	// On next launch, it should NOT be re-carried because the original is done.
	data := TaskData{NextID: 3, Tasks: []Task{
		{ID: 1, Title: "task", Date: "2025-01-01", Done: true}, // marked done by carry
	}}
	changed := CarryForwardTasks(&data, "2025-01-05")
	if changed {
		t.Error("should not re-carry a task whose original is already done")
	}
}

func TestCarryForwardNoDuplicateByID(t *testing.T) {
	// Task already carried into today (CarriedFromID set) - don't carry again
	data := TaskData{NextID: 4, Tasks: []Task{
		{ID: 1, Title: "old name", Date: "2025-01-01", Done: true},
		{ID: 3, Title: "renamed", Date: "2025-01-05", CarriedFromID: 1},
	}}
	changed := CarryForwardTasks(&data, "2025-01-05")
	if changed {
		t.Error("should not duplicate already-carried task even with different title")
	}
	if len(data.Tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(data.Tasks))
	}
}

func TestCarryForwardDeduplicatesByTitle(t *testing.T) {
	// Same task manually re-entered on two different past dates (no CarriedFromID).
	// Only one copy should be carried to today.
	data := TaskData{NextID: 4, Tasks: []Task{
		{ID: 1, Title: "Bug Fix", Date: "2025-01-01", Done: false},
		{ID: 2, Title: "Bug Fix", Date: "2025-01-02", Done: false},
		{ID: 3, Title: "Other Task", Date: "2025-01-02", Done: false},
	}}
	changed := CarryForwardTasks(&data, "2025-01-05")
	if !changed {
		t.Fatal("expected changes")
	}
	todayTasks := TasksForDate(data, "2025-01-05")
	if len(todayTasks) != 2 {
		t.Errorf("expected 2 tasks on today (Bug Fix + Other Task), got %d", len(todayTasks))
	}
	// Both originals should be marked done
	if !data.Tasks[0].Done || !data.Tasks[1].Done {
		t.Error("both originals should be marked done")
	}
}

func TestCarryForwardDeduplicatesByTitleWithinContext(t *testing.T) {
	data := TaskData{NextID: 3, Tasks: []Task{
		{ID: 1, Title: "Same Title", Date: "2025-01-01", Done: false, Context: TaskContextPersonal},
		{ID: 2, Title: "Same Title", Date: "2025-01-01", Done: false, Context: TaskContextWork},
	}}
	changed := CarryForwardTasks(&data, "2025-01-05")
	if !changed {
		t.Fatal("expected changes")
	}
	personalTasks := TasksForDateAndContext(data, "2025-01-05", TaskContextPersonal)
	if len(personalTasks) != 1 {
		t.Fatalf("expected 1 personal carried task, got %d", len(personalTasks))
	}
	workTasks := TasksForDateAndContext(data, "2025-01-05", TaskContextWork)
	if len(workTasks) != 1 {
		t.Fatalf("expected 1 work carried task, got %d", len(workTasks))
	}
	if personalTasks[0].Context != TaskContextPersonal {
		t.Errorf("expected personal context, got %q", personalTasks[0].Context)
	}
	if workTasks[0].Context != TaskContextWork {
		t.Errorf("expected work context, got %q", workTasks[0].Context)
	}
}

func TestCarryForwardSkipsFutureDates(t *testing.T) {
	data := TaskData{NextID: 2, Tasks: []Task{
		{ID: 1, Title: "future", Date: "2025-12-31", Done: false},
	}}
	changed := CarryForwardTasks(&data, "2025-01-05")
	if changed {
		t.Error("should not carry future tasks")
	}
}

func TestNotesDirDefaultCreatesDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(NotesDirEnv, "")
	got, err := NotesDir()
	if err != nil {
		t.Fatalf("NotesDir error: %v", err)
	}
	want := filepath.Join(home, ".task-buddy", "notes")
	if got != want {
		t.Fatalf("NotesDir = %q, want %q", got, want)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("notes dir was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("notes path is not a directory")
	}
}

func TestNotesDirEnvOverrideCreatesDirectory(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-notes")
	t.Setenv(NotesDirEnv, custom)
	got, err := NotesDir()
	if err != nil {
		t.Fatalf("NotesDir error: %v", err)
	}
	if got != custom {
		t.Fatalf("NotesDir = %q, want %q", got, custom)
	}
	if info, err := os.Stat(got); err != nil || !info.IsDir() {
		t.Fatalf("override directory was not created: info=%v err=%v", info, err)
	}
}

func TestResolveNotePathBlocksEscapes(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveNotePath(root, "../escape.md"); err == nil {
		t.Fatal("expected parent traversal to be rejected")
	}
	if _, err := ResolveNotePath(root, filepath.Join("..", "escape.md")); err == nil {
		t.Fatal("expected filepath parent traversal to be rejected")
	}
	if _, err := ResolveNotePath(root, filepath.Join("folder", "note.md")); err != nil {
		t.Fatalf("expected safe relative path, got %v", err)
	}
}

func TestCreateMarkdownNoteDefaultsExtensionAndRead(t *testing.T) {
	root := t.TempDir()
	entry, err := CreateMarkdownNote(root, "", "daily")
	if err != nil {
		t.Fatalf("CreateMarkdownNote error: %v", err)
	}
	if entry.Name != "daily.md" || entry.RelPath != "daily.md" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	path, err := ResolveNotePath(root, entry.RelPath)
	if err != nil {
		t.Fatalf("ResolveNotePath error: %v", err)
	}
	if err := os.WriteFile(path, []byte("# Daily"), 0644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	content, err := ReadNote(root, entry.RelPath)
	if err != nil {
		t.Fatalf("ReadNote error: %v", err)
	}
	if content != "# Daily" {
		t.Fatalf("ReadNote = %q", content)
	}
}

func TestListNotesDirSortsFoldersFirst(t *testing.T) {
	root := t.TempDir()
	if _, err := CreateMarkdownNote(root, "", "zeta"); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := CreateNotesFolder(root, "", "alpha"); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	entries, err := ListNotesDir(root, "")
	if err != nil {
		t.Fatalf("ListNotesDir error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if !entries[0].IsDir || entries[0].Name != "alpha" {
		t.Fatalf("expected folder first, got %+v", entries[0])
	}
	if entries[1].IsDir || entries[1].Name != "zeta.md" {
		t.Fatalf("expected file second, got %+v", entries[1])
	}
}

func TestNotesFolderNonEmptyAndRecursiveDelete(t *testing.T) {
	root := t.TempDir()
	if _, err := CreateNotesFolder(root, "", "folder"); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	nonEmpty, err := NotesFolderNonEmpty(root, "folder")
	if err != nil {
		t.Fatalf("NotesFolderNonEmpty error: %v", err)
	}
	if nonEmpty {
		t.Fatal("new folder should be empty")
	}
	if _, err := CreateMarkdownNote(root, "folder", "note"); err != nil {
		t.Fatalf("create nested note: %v", err)
	}
	nonEmpty, err = NotesFolderNonEmpty(root, "folder")
	if err != nil {
		t.Fatalf("NotesFolderNonEmpty error: %v", err)
	}
	if !nonEmpty {
		t.Fatal("folder with note should be non-empty")
	}
	if err := DeleteNoteEntry(root, "folder"); err != nil {
		t.Fatalf("DeleteNoteEntry error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "folder")); !os.IsNotExist(err) {
		t.Fatalf("expected folder to be removed, err=%v", err)
	}
}

func TestDeleteNoteEntryRejectsRoot(t *testing.T) {
	root := t.TempDir()
	if err := DeleteNoteEntry(root, ""); err == nil {
		t.Fatal("expected deleting notes root to be rejected")
	}
}
