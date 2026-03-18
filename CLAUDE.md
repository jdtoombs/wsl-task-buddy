# task-buddy

Fullscreen TUI task tracker with vim motions and date-based navigation.

## Tech Stack
- Go 1.24+
- Bubble Tea (charmbracelet/bubbletea) - TUI framework
- Lip Gloss (charmbracelet/lipgloss) - terminal styling

## Build & Run
```
go build -o task-buddy .
./task-buddy
```

## Architecture
Root-level `main.go` using the Elm architecture via Bubble Tea, with `store/` package for data types and persistence:
- `model` holds all state: tasks, cursor position, current date, input mode
- `store.TaskData` holds the persisted task collection
- `Update()` handles key events, delegates to `updateNormal()` / `updateInsert()` based on mode
- `View()` renders fullscreen with date header, task list, and bottom help bar
- Three modes: normal (vim navigation), insert (typing a new task title), and edit (renaming a task)
- Timer support: tracks time per task via `store.TimeEntry`, with start/stop in normal mode
- Reminder system: tasks with `@HH:MMam/pm` in the title trigger WSL notifications when due

## Data
- Tasks persist to `~/.tasks.json` as a flat JSON array
- Each task has: id, title, done (bool), date (YYYY-MM-DD), notified (bool), entries (time tracking)
- Tasks are filtered by the currently viewed date
- Time entries track start/end timestamps per task for timer functionality

## Keybindings
- `j/k` or arrows: move cursor
- `h/l` or left/right: navigate days
- `t`: jump to today
- `g/G`: jump to top/bottom
- `+` or `o`: add task (enter insert mode)
- `i`: edit task name
- `enter` or `space`: toggle done
- `s`: start/stop timer on selected task
- `d` or `x`: delete task (with confirmation)
- `n`: send notification for selected task
- `?`: toggle help overlay
- `q` or `ctrl+c`: quit

## Limitations
- No file locking: concurrent instances can overwrite each other's changes
- Notifications are WSL-specific (uses `powershell.exe`)
