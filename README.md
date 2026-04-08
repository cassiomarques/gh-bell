# gh-bell 🔔

A terminal UI for managing GitHub notifications. Built as a `gh` CLI extension — no PAT needed.

![Go](https://img.shields.io/badge/Go-1.25-blue)

## Install

```bash
gh extension install cassiomarques/gh-bell
```

## Usage

```bash
gh bell
```

## Features

- **Vim-style navigation** — `j`/`k`, `gg`, `G`
- **Three views** — Unread (`1`), All (`2`), Participating (`3`)
- **Filter by repo** — press `/`, type to search, `Enter` to apply
- **Filter by reason** — press `f` to cycle (review, mention, assign, etc.)
- **Actions** — `r` mark read, `R` mark all, `m` mute, `u` unsubscribe
- **Open in browser** — `Enter` opens the notification in your default browser
- **Preview pane** — shows notification details alongside the list
- **Auto-refresh** — polls for new notifications every 60 seconds
- **Catppuccin theme** — beautiful terminal colors out of the box

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `gg` | Jump to top |
| `G` | Jump to bottom |
| `Enter` | Open in browser |
| `r` | Mark as read |
| `R` | Mark all as read |
| `m` | Mute thread |
| `u` | Unsubscribe |
| `1` / `2` / `3` | Switch view |
| `/` | Filter by repo |
| `f` | Cycle reason filter |
| `Esc` | Clear filters |
| `Tab` | Switch focus (list ↔ preview) |
| `?` | Help |
| `q` | Quit |

## How It Works

gh-bell uses the [GitHub Notifications REST API](https://docs.github.com/en/rest/activity/notifications) and inherits authentication from `gh auth login` via the [go-gh](https://github.com/cli/go-gh) library.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) v2, following the Elm Architecture (Model → Update → View).

## Development

```bash
go build -o gh-bell .
go test ./...
```

## License

MIT
