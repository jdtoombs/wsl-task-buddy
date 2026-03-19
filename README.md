# task-buddy

A fullscreen TUI task tracker with vim motions and date-based navigation.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Install

Requires Go 1.24+.

```sh
go build -o task-buddy .
```

## Usage

```sh
./task-buddy
```

Or run directly without building:

```sh
go run .
```

Tasks are stored in `~/.tasks.json`. Incomplete tasks from past dates are automatically carried forward to today on launch.

## Features

- Vim-style navigation with date-based task filtering
- Timer tracking with start/stop per task
- Manual time entry (e.g. `1h30m`, `45m`, `1:30`)
- Carry-forward of incomplete past tasks to today
- Desktop notifications for tasks with `@HH:MMam/pm` in the title

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` / arrows | Move cursor up/down |
| `h` / `l` / left/right | Navigate days |
| `t` | Jump to today |
| `g` / `G` | Jump to top/bottom |
| `+` / `o` | Add task |
| `i` | Edit task name |
| `enter` / `space` | Toggle done |
| `s` | Start/stop timer |
| `T` | Add time manually |
| `d` / `x` | Delete task (with confirmation) |
| `n` | Send notification for selected task |
| `?` | Toggle help overlay |
| `q` / `ctrl+c` | Quit |
