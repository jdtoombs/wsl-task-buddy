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

Tasks are stored in `~/.tasks.json`.

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` / arrows | Move cursor up/down |
| `h` / `l` / left/right | Navigate days |
| `t` | Jump to today |
| `g` / `G` | Jump to top/bottom |
| `+` / `a` | Add task |
| `enter` / `space` | Toggle done |
| `d` / `x` | Delete task |
| `q` / `ctrl+c` | Quit |
