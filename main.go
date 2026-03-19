// task-buddy - fullscreen TUI task tracker with vim motions, date navigation, and timers
package main

import (
	"fmt"
	"os"
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"wsl-task-buddy/store"
)

type mode int

const (
	modeNormal mode = iota
	modeInsert
	modeEdit
	modeConfirmDelete
	modeHelp
)

type model struct {
	data        store.TaskData
	cursor      int
	mode        mode
	input       string
	date        time.Time
	width       int
	height      int
	savePath    string
	err         string
	timerTaskID int
	lastChecked time.Time
	editTaskID  int
	editCursor  int
}

type reminderTickMsg time.Time
type timerTickMsg time.Time
type notifyDoneMsg struct{}

func reminderTickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg { return reminderTickMsg(t) })
}

func timerTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return timerTickMsg(t) })
}

var timeTagRegex = regexp.MustCompile(`@(\d{1,2}):(\d{2})(am|pm)`)

func parseTaskTime(title string, date string) (time.Time, bool) {
	matches := timeTagRegex.FindStringSubmatch(title)
	if matches == nil {
		return time.Time{}, false
	}
	var hour int
	fmt.Sscanf(matches[1], "%d", &hour)
	var minute int
	fmt.Sscanf(matches[2], "%d", &minute)
	ampm := matches[3]
	if hour < 1 || hour > 12 || minute > 59 {
		return time.Time{}, false
	}
	if ampm == "pm" && hour != 12 {
		hour += 12
	} else if ampm == "am" && hour == 12 {
		hour = 0
	}
	t, err := time.ParseInLocation("2006-01-02", date, time.Now().Location())
	if err != nil {
		return time.Time{}, false
	}
	t = t.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	return t, true
}

func sendNotification(title, body string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "powershell.exe", "-Command",
			`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.MessageBox]::Show($env:TASKS_BODY, $env:TASKS_TITLE, 'OK', 'Information')`)
		cmd.Env = append(os.Environ(), "TASKS_TITLE="+title, "TASKS_BODY="+body)
		cmd.Run()
		return notifyDoneMsg{}
	}
}

func (m model) tasksForDate() []int {
	dateStr := m.date.Format("2006-01-02")
	var indices []int
	for i, t := range m.data.Tasks {
		if t.Date == dateStr {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m *model) save() {
	if err := store.Save(m.savePath, m.data); err != nil {
		m.err = err.Error()
	} else {
		m.err = ""
	}
}

func (m *model) stopTimer() {
	if m.timerTaskID < 0 {
		return
	}
	if _, err := store.StopTimer(&m.data, m.timerTaskID); err != nil {
		m.err = err.Error()
	}
	m.timerTaskID = -1
}

func (m *model) startTimer(globalIdx int) {
	m.stopTimer()
	t := &m.data.Tasks[globalIdx]
	t.Entries = append(t.Entries, store.TimeEntry{Start: time.Now()})
	m.timerTaskID = t.ID
}

func (m *model) checkReminders() tea.Cmd {
	now := time.Now()
	today := now.Format("2006-01-02")
	var cmds []tea.Cmd
	for i := range m.data.Tasks {
		t := &m.data.Tasks[i]
		if t.Done || t.Notified || t.Date != today {
			continue
		}
		due, ok := parseTaskTime(t.Title, t.Date)
		if !ok {
			continue
		}
		if due.After(m.lastChecked) && !due.After(now) {
			t.Notified = true
			cmds = append(cmds, sendNotification("Task Reminder", t.Title))
		}
	}
	m.lastChecked = now
	if len(cmds) > 0 {
		m.save()
		return tea.Batch(cmds...)
	}
	return nil
}

func initialModel() model {
	path, err := store.DataPath()
	if err != nil {
		return model{err: err.Error(), timerTaskID: -1}
	}
	s, loadErr := store.Load(path)
	today := time.Now().Format("2006-01-02")
	if store.CarryForwardTasks(&s, today) {
		if saveErr := store.Save(path, s); saveErr != nil {
			return model{
				data:        s,
				date:        time.Now(),
				savePath:    path,
				timerTaskID: store.FindRunningTimerID(s),
				lastChecked: time.Now(),
				err:         saveErr.Error(),
			}
		}
	}
	m := model{
		data:        s,
		date:        time.Now(),
		savePath:    path,
		timerTaskID: store.FindRunningTimerID(s),
		lastChecked: time.Now(),
	}
	if loadErr != nil {
		m.err = loadErr.Error()
	}
	return m
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{reminderTickCmd()}
	if m.timerTaskID >= 0 {
		cmds = append(cmds, timerTickCmd())
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case reminderTickMsg:
		cmd := m.checkReminders()
		return m, tea.Batch(cmd, reminderTickCmd())
	case timerTickMsg:
		if m.timerTaskID >= 0 {
			return m, timerTickCmd()
		}
		return m, nil
	case notifyDoneMsg:
		return m, nil
	case tea.KeyMsg:
		m.err = ""
		switch m.mode {
		case modeHelp:
			m.mode = modeNormal
			return m, nil
		case modeInsert:
			return m.updateInsert(msg)
		case modeEdit:
			return m.updateEdit(msg)
		case modeConfirmDelete:
			return m.updateConfirmDelete(msg)
		default:
			return m.updateNormal(msg)
		}
	}
	return m, nil
}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	indices := m.tasksForDate()
	switch msg.String() {
	case "q", "ctrl+c":
		if m.timerTaskID >= 0 {
			m.stopTimer()
			m.save()
		}
		return m, tea.Quit
	case "+", "o":
		m.mode = modeInsert
		m.input = ""
	case "j", "down":
		if len(indices) > 0 && m.cursor < len(indices)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "G":
		if len(indices) > 0 {
			m.cursor = len(indices) - 1
		}
	case "g":
		m.cursor = 0
	case "h", "left":
		m.date = m.date.AddDate(0, 0, -1)
		m.cursor = 0
	case "l", "right":
		m.date = m.date.AddDate(0, 0, 1)
		m.cursor = 0
	case "t":
		m.date = time.Now()
		m.cursor = 0
	case "enter", " ":
		if len(indices) > 0 {
			idx := indices[m.cursor]
			m.data.Tasks[idx].Done = !m.data.Tasks[idx].Done
			m.save()
		}
	case "d", "x":
		if len(indices) > 0 {
			m.mode = modeConfirmDelete
		}
	case "n":
		if len(indices) > 0 {
			idx := indices[m.cursor]
			return m, sendNotification("Task Reminder", m.data.Tasks[idx].Title)
		}
	case "s":
		if len(indices) > 0 {
			globalIdx := indices[m.cursor]
			taskID := m.data.Tasks[globalIdx].ID
			if m.timerTaskID == taskID {
				m.stopTimer()
			} else {
				m.startTimer(globalIdx)
				m.save()
				return m, timerTickCmd()
			}
			m.save()
		}
	case "i":
		if len(indices) > 0 {
			idx := indices[m.cursor]
			m.editTaskID = m.data.Tasks[idx].ID
			m.input = m.data.Tasks[idx].Title
			m.editCursor = utf8.RuneCountInString(m.input)
			m.mode = modeEdit
		}
	case "?":
		m.mode = modeHelp
	}
	return m, nil
}

func (m model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.timerTaskID >= 0 {
			m.stopTimer()
			m.save()
		}
		return m, tea.Quit
	case "y":
		indices := m.tasksForDate()
		if len(indices) > 0 {
			idx := indices[m.cursor]
			if m.data.Tasks[idx].ID == m.timerTaskID {
				m.stopTimer()
			}
			m.data.Tasks = append(m.data.Tasks[:idx], m.data.Tasks[idx+1:]...)
			if m.cursor >= len(indices)-1 && m.cursor > 0 {
				m.cursor--
			}
			m.save()
		}
		m.mode = modeNormal
	default:
		m.mode = modeNormal
	}
	return m, nil
}

func (m model) updateInsert(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.timerTaskID >= 0 {
			m.stopTimer()
			m.save()
		}
		return m, tea.Quit
	case "enter":
		title := strings.TrimSpace(m.input)
		if title != "" {
			store.AddTask(&m.data, title, m.date.Format("2006-01-02"))
			indices := m.tasksForDate()
			m.cursor = len(indices) - 1
			m.save()
		}
		m.mode = modeNormal
		m.input = ""
	case "esc":
		m.mode = modeNormal
		m.input = ""
	case "backspace":
		if len(m.input) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.input)
			m.input = m.input[:len(m.input)-size]
		}
	default:
		if utf8.RuneCountInString(msg.String()) == 1 {
			m.input += msg.String()
		}
	}
	return m, nil
}

func editRuneSlice(s string) []rune {
	return []rune(s)
}

func (m model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	runes := editRuneSlice(m.input)
	switch msg.String() {
	case "ctrl+c":
		if m.timerTaskID >= 0 {
			m.stopTimer()
			m.save()
		}
		return m, tea.Quit
	case "enter":
		title := strings.TrimSpace(m.input)
		if title != "" {
			if _, idx, ok := store.TaskByID(m.data, m.editTaskID); ok {
				m.data.Tasks[idx].Title = title
				m.save()
			}
		}
		m.mode = modeNormal
		m.input = ""
	case "esc":
		m.mode = modeNormal
		m.input = ""
	case "left":
		if m.editCursor > 0 {
			m.editCursor--
		}
	case "right":
		if m.editCursor < len(runes) {
			m.editCursor++
		}
	case "home", "ctrl+a":
		m.editCursor = 0
	case "end", "ctrl+e":
		m.editCursor = len(runes)
	case "backspace":
		if m.editCursor > 0 {
			runes = append(runes[:m.editCursor-1], runes[m.editCursor:]...)
			m.editCursor--
			m.input = string(runes)
		}
	case "delete":
		if m.editCursor < len(runes) {
			runes = append(runes[:m.editCursor], runes[m.editCursor+1:]...)
			m.input = string(runes)
		}
	default:
		if utf8.RuneCountInString(msg.String()) == 1 {
			r := []rune(msg.String())[0]
			runes = append(runes[:m.editCursor], append([]rune{r}, runes[m.editCursor:]...)...)
			m.editCursor++
			m.input = string(runes)
		}
	}
	return m, nil
}

var (
	dateStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	todayStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	dimDateStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	doneStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	inputStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	timerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	timeLogStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
)

func centerText(text string, width int) string {
	textLen := lipgloss.Width(text)
	if textLen >= width {
		return text
	}
	pad := (width - textLen) / 2
	return strings.Repeat(" ", pad) + text
}

func displayWidth(s string) int {
	return runewidth.StringWidth(s)
}

func (m model) View() string {
	var b strings.Builder
	w := m.width
	if w < 1 {
		w = 80
	}

	today := time.Now().Format("2006-01-02")
	viewDate := m.date.Format("2006-01-02")
	isToday := today == viewDate

	prev := m.date.AddDate(0, 0, -1)
	next := m.date.AddDate(0, 0, 1)

	leftArrow := dimDateStyle.Render(fmt.Sprintf("< %s", prev.Format("Mon 02")))
	rightArrow := dimDateStyle.Render(fmt.Sprintf("%s >", next.Format("Mon 02")))

	var centerDate string
	if isToday {
		centerDate = todayStyle.Render(m.date.Format("Monday, January 2, 2006") + "  (today)")
	} else {
		centerDate = dateStyle.Render(m.date.Format("Monday, January 2, 2006"))
	}

	headerLine := fmt.Sprintf("%s    %s    %s", leftArrow, centerDate, rightArrow)
	b.WriteString("\n")
	b.WriteString(centerText(headerLine, w))
	b.WriteString("\n")

	dividerLen := min(w-4, 70)
	if dividerLen < 1 {
		dividerLen = 40
	}
	divider := strings.Repeat("─", dividerLen)
	b.WriteString(centerText(helpStyle.Render(divider), w))
	b.WriteString("\n\n")

	indices := m.tasksForDate()

	if len(indices) == 0 && m.mode == modeNormal {
		b.WriteString(centerText(helpStyle.Render("no tasks for this day — press + to add one"), w))
		b.WriteString("\n")
	}

	maxTitleWidth := 0
	for _, idx := range indices {
		tw := displayWidth(m.data.Tasks[idx].Title)
		if tw > maxTitleWidth {
			maxTitleWidth = tw
		}
	}

	prefixWidth := displayWidth("> ") + displayWidth("[ ] ")
	maxTimeWidth := 0
	hasAnyTime := false
	for _, idx := range indices {
		t := m.data.Tasks[idx]
		total := t.TotalTime()
		if total > 0 || t.IsRunning() {
			hasAnyTime = true
			tw := displayWidth(store.FormatDuration(total))
			if tw > maxTimeWidth {
				maxTimeWidth = tw
			}
		}
	}
	timeColWidth := 0
	if hasAnyTime {
		timeColWidth = 2 + maxTimeWidth + 2
	}
	fullLineWidth := prefixWidth + maxTitleWidth + timeColWidth
	blockPad := (w - fullLineWidth) / 2
	if blockPad < 0 {
		blockPad = 0
	}
	pad := strings.Repeat(" ", blockPad)

	for i, idx := range indices {
		t := m.data.Tasks[idx]

		if m.mode == modeEdit && t.ID == m.editTaskID {
			runes := []rune(m.input)
			before := string(runes[:m.editCursor])
			after := ""
			if m.editCursor < len(runes) {
				after = string(runes[m.editCursor:])
			}
			editLine := fmt.Sprintf("> [ ] %s_%s", before, after)
			b.WriteString(pad + inputStyle.Render(editLine))
			b.WriteString("\n")
			continue
		}

		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		check := "[ ] "
		if t.Done {
			check = "[x] "
		}

		titlePad := maxTitleWidth - displayWidth(t.Title)
		if titlePad < 0 {
			titlePad = 0
		}
		titlePadded := t.Title + strings.Repeat(" ", titlePad)

		var timeSuffix string
		total := t.TotalTime()
		if t.IsRunning() {
			durStr := fmt.Sprintf("%*s", maxTimeWidth, store.FormatDuration(total))
			timeSuffix = "  " + timerStyle.Render(durStr+" ▶")
		} else if total > 0 {
			durStr := fmt.Sprintf("%*s", maxTimeWidth, store.FormatDuration(total))
			timeSuffix = "  " + timeLogStyle.Render(durStr+"  ")
		}

		line := fmt.Sprintf("%s%s%s", cursor, check, titlePadded)
		var rendered string
		if t.Done {
			rendered = doneStyle.Render(line) + timeSuffix
		} else if i == m.cursor {
			rendered = selectedStyle.Render(line) + timeSuffix
		} else {
			rendered = "  " + check + titlePadded + timeSuffix
		}
		b.WriteString(pad + rendered)
		b.WriteString("\n")
	}

	if m.mode == modeInsert {
		b.WriteString("\n")
		b.WriteString(pad + inputStyle.Render(fmt.Sprintf("new task: %s_", m.input)))
		b.WriteString("\n")
	}

	if m.mode == modeConfirmDelete && len(indices) > 0 {
		idx := indices[m.cursor]
		b.WriteString("\n")
		b.WriteString(pad + warnStyle.Render(fmt.Sprintf("delete \"%s\"? (y/n)", m.data.Tasks[idx].Title)))
		b.WriteString("\n")
	}

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(centerText(errStyle.Render("error: "+m.err), w))
		b.WriteString("\n")
	}

	if m.mode == modeHelp {
		b.WriteString("\n")
		helpLines := []string{
			"Keybindings",
			"",
			"+/o     add task",
			"i       edit task name",
			"enter   toggle done",
			"s       start/stop timer",
			"d/x     delete task",
			"n       send notification",
			"j/k     move up/down",
			"g/G     jump to top/bottom",
			"h/l     previous/next day",
			"t       jump to today",
			"?       toggle this help",
			"q       quit",
		}
		for i, line := range helpLines {
			if i == 0 {
				b.WriteString(centerText(selectedStyle.Render(line), w))
			} else {
				b.WriteString(centerText(helpStyle.Render(line), w))
			}
			b.WriteString("\n")
		}
	}

	contentLines := strings.Count(b.String(), "\n")
	padding := m.height - contentLines - 3
	if padding > 0 {
		b.WriteString(strings.Repeat("\n", padding))
	}

	b.WriteString(centerText(helpStyle.Render("? help  q quit"), w))
	b.WriteString("\n")

	return b.String()
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
