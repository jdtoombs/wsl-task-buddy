// store provides shared task data types and persistence for the tasks app
package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type TimeEntry struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end,omitempty"`
}

const (
	TaskContextPersonal = "personal"
	TaskContextWork     = "work"

	NotesDirEnv = "TASK_BUDDY_NOTES_DIR"
)

func NormalizeTaskContext(context string) string {
	switch strings.ToLower(strings.TrimSpace(context)) {
	case TaskContextWork:
		return TaskContextWork
	default:
		return TaskContextPersonal
	}
}

type Task struct {
	ID            int         `json:"id"`
	Title         string      `json:"title"`
	Done          bool        `json:"done"`
	Date          string      `json:"date"`
	Context       string      `json:"context,omitempty"`
	Notified      bool        `json:"notified,omitempty"`
	Entries       []TimeEntry `json:"entries,omitempty"`
	CarriedFromID int         `json:"carried_from_id,omitempty"`
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

func NotesDir() (string, error) {
	path := strings.TrimSpace(os.Getenv(NotesDirEnv))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		path = filepath.Join(home, ".task-buddy", "notes")
	} else if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimLeft(path[1:], `/\\`))
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve notes directory: %w", err)
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return "", fmt.Errorf("create notes directory: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat notes directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("notes path is not a directory: %s", abs)
	}
	return abs, nil
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

type NoteEntry struct {
	Name    string
	RelPath string
	IsDir   bool
	Size    int64
}

func cleanRelativePath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return "", nil
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", rel)
	}
	cleaned := filepath.Clean(rel)
	if cleaned == "." {
		return "", nil
	}
	for _, part := range strings.Split(cleaned, string(os.PathSeparator)) {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe path segment %q in %s", part, rel)
		}
	}
	return cleaned, nil
}

func pathInsideRoot(root, path string) (bool, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)), nil
}

func ResolveNotePath(root, rel string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("notes root is empty")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve notes root: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("resolve notes root symlinks: %w", err)
	}
	cleanRel, err := cleanRelativePath(rel)
	if err != nil {
		return "", err
	}
	path := filepath.Join(absRoot, cleanRel)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve note path: %w", err)
	}
	inside, err := pathInsideRoot(absRoot, absPath)
	if err != nil {
		return "", fmt.Errorf("compare note path: %w", err)
	}
	if !inside {
		return "", fmt.Errorf("path escapes notes root: %s", rel)
	}

	// If the path or its nearest existing parent contains symlinks, make sure the
	// real filesystem location still remains inside the notes root. This rejects
	// creating or editing notes through symlinks that point outside the root.
	checkPath := absPath
	for {
		realPath, err := filepath.EvalSymlinks(checkPath)
		if err == nil {
			inside, err := pathInsideRoot(realRoot, realPath)
			if err != nil {
				return "", fmt.Errorf("compare real note path: %w", err)
			}
			if !inside {
				return "", fmt.Errorf("path escapes notes root: %s", rel)
			}
			break
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve note path symlinks: %w", err)
		}
		parent := filepath.Dir(checkPath)
		if parent == checkPath {
			break
		}
		checkPath = parent
	}
	return absPath, nil
}

func ListNotesDir(root, relDir string) ([]NoteEntry, error) {
	path, err := ResolveNotePath(root, relDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read notes directory: %w", err)
	}
	cleanDir, err := cleanRelativePath(relDir)
	if err != nil {
		return nil, err
	}
	notes := make([]NoteEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat note entry %s: %w", entry.Name(), err)
		}
		rel := filepath.Join(cleanDir, entry.Name())
		notes = append(notes, NoteEntry{
			Name:    entry.Name(),
			RelPath: rel,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
		})
	}
	sort.Slice(notes, func(i, j int) bool {
		if notes[i].IsDir != notes[j].IsDir {
			return notes[i].IsDir
		}
		return strings.ToLower(notes[i].Name) < strings.ToLower(notes[j].Name)
	})
	return notes, nil
}

func safeChildPath(relDir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("name cannot be empty")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", name)
	}
	cleanName, err := cleanRelativePath(name)
	if err != nil {
		return "", err
	}
	if cleanName == "" {
		return "", fmt.Errorf("name cannot be empty or current directory")
	}
	cleanDir, err := cleanRelativePath(relDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(cleanDir, cleanName), nil
}

func markdownRelPath(relDir, name string) (string, error) {
	rel, err := safeChildPath(relDir, name)
	if err != nil {
		return "", err
	}
	if filepath.Ext(rel) == "" {
		rel += ".md"
	}
	return rel, nil
}

func CreateMarkdownNote(root, relDir, name string) (NoteEntry, error) {
	rel, err := markdownRelPath(relDir, name)
	if err != nil {
		return NoteEntry{}, err
	}
	path, err := ResolveNotePath(root, rel)
	if err != nil {
		return NoteEntry{}, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return NoteEntry{}, fmt.Errorf("create note: %w", err)
	}
	if err := file.Close(); err != nil {
		return NoteEntry{}, fmt.Errorf("close note: %w", err)
	}
	return NoteEntry{Name: filepath.Base(rel), RelPath: rel, IsDir: false}, nil
}

func CreateNotesFolder(root, relDir, name string) (NoteEntry, error) {
	rel, err := safeChildPath(relDir, name)
	if err != nil {
		return NoteEntry{}, err
	}
	path, err := ResolveNotePath(root, rel)
	if err != nil {
		return NoteEntry{}, err
	}
	if err := os.Mkdir(path, 0755); err != nil {
		return NoteEntry{}, fmt.Errorf("create folder: %w", err)
	}
	return NoteEntry{Name: filepath.Base(rel), RelPath: rel, IsDir: true}, nil
}

func ReadNote(root, rel string) (string, error) {
	path, err := ResolveNotePath(root, rel)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat note: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("cannot read folder as note: %s", rel)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read note: %w", err)
	}
	return string(data), nil
}

func NotesFolderNonEmpty(root, rel string) (bool, error) {
	path, err := ResolveNotePath(root, rel)
	if err != nil {
		return false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open folder: %w", err)
	}
	defer file.Close()
	_, err = file.Readdirnames(1)
	if err == nil {
		return true, nil
	}
	if err == io.EOF {
		return false, nil
	}
	return false, fmt.Errorf("read folder: %w", err)
}

func DeleteNoteEntry(root, rel string) error {
	path, err := ResolveNotePath(root, rel)
	if err != nil {
		return err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve notes root: %w", err)
	}
	if path == absRoot {
		return fmt.Errorf("cannot delete notes root")
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("delete note entry: %w", err)
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

func TasksForDateAndContext(s TaskData, date, context string) []Task {
	context = NormalizeTaskContext(context)
	var tasks []Task
	for _, t := range s.Tasks {
		if t.Date == date && NormalizeTaskContext(t.Context) == context {
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

func AddTask(s *TaskData, title, date string, context ...string) Task {
	taskContext := TaskContextPersonal
	if len(context) > 0 {
		taskContext = NormalizeTaskContext(context[0])
	}
	t := Task{
		ID:      s.NextID,
		Title:   title,
		Date:    date,
		Context: taskContext,
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
	// Build set of source IDs and titles already on today to prevent duplicates.
	// Titles dedupe only within the same task context, so work and personal lists
	// can each carry a task with the same title.
	carriedIDs := make(map[int]bool)
	todayTitles := make(map[string]map[string]bool)
	for _, t := range s.Tasks {
		if t.Date == today {
			context := NormalizeTaskContext(t.Context)
			if todayTitles[context] == nil {
				todayTitles[context] = make(map[string]bool)
			}
			todayTitles[context][t.Title] = true
			if t.CarriedFromID > 0 {
				carriedIDs[t.CarriedFromID] = true
			}
		}
	}
	changed := false
	// range snapshots len; appended tasks won't be revisited
	for i, t := range s.Tasks {
		if t.Date >= today || t.Done {
			continue
		}
		context := NormalizeTaskContext(t.Context)
		if todayTitles[context] == nil {
			todayTitles[context] = make(map[string]bool)
		}
		if carriedIDs[t.ID] || todayTitles[context][t.Title] {
			// Still mark the original as done to prevent future carries
			s.Tasks[i].Done = true
			changed = true
			continue
		}
		newTask := Task{
			ID:            s.NextID,
			Title:         t.Title,
			Date:          today,
			Context:       context,
			CarriedFromID: t.ID,
		}
		s.NextID++
		s.Tasks = append(s.Tasks, newTask)
		// Mark original as done so it won't be carried again
		s.Tasks[i].Done = true
		carriedIDs[t.ID] = true
		todayTitles[context][t.Title] = true
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
