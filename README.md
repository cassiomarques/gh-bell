# gh-bell 🔔

A terminal UI for managing GitHub notifications. Built as a `gh` CLI extension.

![Go](https://img.shields.io/badge/Go-1.25-blue)

## Install

```bash
gh extension install cassiomarques/gh-bell
```

## Usage

```bash
gh bell
```

### Authentication

gh-bell requires a [classic GitHub Personal Access Token (PAT)](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#creating-a-personal-access-token-classic) with `notifications` and `repo` scopes. GitHub's Notifications API returns persistent 502 errors when called with the OAuth tokens issued by `gh auth login`.

1. [Create a classic PAT](https://github.com/settings/tokens/new) — select `notifications` and `repo` scopes
2. Export it in your shell profile (e.g. `~/.zshrc` or `~/.bashrc`):

```bash
export GH_BELL_TOKEN=ghp_your_token
```

3. Run:

```bash
gh bell
```

Token resolution order: `GH_BELL_TOKEN` → `GH_TOKEN` → `GITHUB_TOKEN` → `gh auth login` keyring.

### Configuration

| Environment variable | Description | Default |
|---------------------|-------------|---------|
| `GH_BELL_TOKEN` | Classic PAT for authentication | (use `gh auth login`) |
| `GH_BELL_REFRESH` | Auto-refresh interval in seconds | `60` |

## Features

- **Vim-style navigation** — `j`/`k`, `gg`/`G` in both list and preview pane
- **Three views** — Unread (`1`), All (`2`), Participating (`3`)
- **Rich filtering** — repo (`/`), title search (`s`), reason (`f`), type (`t`), org (`o`), age (`a`), participating (`p`) — all combinable
- **Full-text search** — `S` searches notification titles, bodies, comments, labels, and more using [Bleve](https://blevesearch.com/)
- **Actions** — `r` mark read, `R` mark all, `m` mute, `u` unsubscribe
- **Open in browser** — `Enter` opens the notification and marks it as read
- **Preview pane** — shows notification details with markdown rendering (via [glamour](https://github.com/charmbracelet/glamour))
- **Local persistence** — SQLite cache for fast startup + persistent mutes; Bleve index for offline full-text search
- **Remembered preferences** — last active view is restored on next launch
- **New notification indicators** — `•` prefix and green tint for items that appeared since last refresh
- **Color-coded reasons** — each notification reason (review, mention, assign, etc.) has a distinct color
- **Auto-refresh** — configurable polling interval (default 60s, set `GH_BELL_REFRESH`)
- **Notification count** — status bar shows filtered/total count
- **Catppuccin Mocha theme** — beautiful terminal colors out of the box

## Keybindings

### Navigation

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `gg` | Jump to top |
| `G` | Jump to bottom |
| `Tab` | Switch focus (list ↔ preview) |

### Actions

| Key | Action |
|-----|--------|
| `Enter` | Open in browser (also marks as read) |
| `r` | Mark as read |
| `R` | Mark all as read |
| `m` | Mute thread |
| `u` | Unsubscribe |

### Filters & Views

| Key | Action |
|-----|--------|
| `1` / `2` / `3` | Unread / All / Participating view |
| `/` | Filter by repo (live search) |
| `s` | Search titles (live search) |
| `S` | Full-text search (bodies, comments, labels) |

#### Full-text search syntax (`S`)

| Query | Meaning |
|-------|---------|
| `foo bar` | Both words, any order (AND) |
| `"foo bar"` | Exact phrase |
| `foo OR bar` | Either word |
| `+foo -bar` | Must contain foo, must not contain bar |

Search covers titles, issue/PR bodies, comments, labels, repo names, and notification reasons. Results are ranked by relevance.

| Key | Action |
|-----|--------|
| `f` | Cycle reason filter |
| `t` | Cycle type filter (Issue, PR, Release, etc.) |
| `o` | Cycle org/owner filter |
| `a` | Cycle age filter (24h, 7d, 30d) |
| `p` | Toggle participating-only |
| `A` | Toggle assigned to me |
| `Esc` | Clear all filters |

### General

| Key | Action |
|-----|--------|
| `Ctrl+R` | Refresh notifications |
| `?` | Toggle help overlay |
| `q` | Quit |

> All action keys (`r`, `m`, `u`, `Enter`) work from both the list and preview panes.

## How It Works

gh-bell uses the [GitHub Notifications REST API](https://docs.github.com/en/rest/activity/notifications) and inherits authentication from `gh auth login` via the [go-gh](https://github.com/cli/go-gh) library.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) v2, following the Elm Architecture (Model → Update → View).

### Data Directory

gh-bell stores local data in `~/.gh-bell/`:

| File | Purpose |
|------|---------|
| `meta.db` | SQLite database — cached notifications, thread details, mutes, preferences |
| `search.bleve/` | Bleve full-text search index |
| `gh-bell.log` | Debug log (overwritten each run) |

The cache speeds up startup (cached notifications display instantly while fresh data loads in the background) and enables offline full-text search across notification titles, issue/PR bodies, comments, and labels.

## Development

```bash
go build -o gh-bell .
go test ./...
```

## License

MIT
