package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cassiomarques/gh-bell/internal/github"
	"github.com/cassiomarques/gh-bell/internal/tui/theme"
)

// focusedPane tracks which pane has keyboard focus.
type focusedPane int

const (
	focusList focusedPane = iota
	focusPreview
)

// App is the root Bubble Tea model.
//
// In the Elm Architecture, this struct IS the entire application state.
// Every piece of data the UI needs lives here (or in a sub-component's model).
// State is never mutated in place — Update returns a new App value each time.
// This makes the app predictable: given the same state + message, you always
// get the same result.
type App struct {
	client *github.Client

	// Data
	notifications []github.Notification
	currentView   github.View

	// UI state
	cursor      int
	offset      int // scroll offset for the visible window
	focused     focusedPane
	width       int
	height      int
	loading     bool
	statusText  string
	statusError bool

	// Filters
	repoFilter   string
	reasonFilter string
}

// NewApp creates an App wired to the given GitHub API client.
func NewApp(client *github.Client) App {
	return App{
		client:      client,
		currentView: github.ViewUnread,
		loading:     true,
	}
}

// Init is called once when the program starts. It returns the initial Cmd(s)
// to kick things off — in our case, fetching notifications and starting the
// refresh timer.
//
// This is the "entry point" of the Elm Architecture's event loop:
//   Init → (model, cmd) → runtime executes cmd → msg arrives → Update → View → …
func (a App) Init() tea.Cmd {
	return tea.Batch(
		fetchNotificationsCmd(a.client, a.currentView),
		refreshTickCmd(),
	)
}

// Update is the heart of the Elm Architecture. It receives a message and
// returns an updated model + optional Cmd(s). Bubble Tea calls this every
// time something happens: a keypress, a window resize, an API response, a
// timer tick — everything is a message.
//
// The pattern is always: match on message type → compute new state → return.
// No side effects happen here — those go into Cmd functions (see commands.go).
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case tea.KeyPressMsg:
		return a.handleKey(msg)

	case notificationsLoadedMsg:
		a.notifications = msg.notifications
		a.loading = false
		a.clampCursor()
		return a, nil

	case errorMsg:
		a.loading = false
		a.statusText = fmt.Sprintf("Error: %v", msg.err)
		a.statusError = true
		return a, clearStatusCmd()

	case threadMarkedReadMsg:
		a.removeNotification(msg.threadID)
		a.statusText = "Marked as read"
		a.statusError = false
		return a, clearStatusCmd()

	case allMarkedReadMsg:
		a.notifications = nil
		a.cursor = 0
		a.offset = 0
		a.statusText = "All marked as read"
		a.statusError = false
		return a, clearStatusCmd()

	case threadMutedMsg:
		a.removeNotification(msg.threadID)
		a.statusText = "Thread muted"
		a.statusError = false
		return a, clearStatusCmd()

	case threadUnsubscribedMsg:
		a.removeNotification(msg.threadID)
		a.statusText = "Unsubscribed"
		a.statusError = false
		return a, clearStatusCmd()

	case clearStatusMsg:
		a.statusText = ""
		a.statusError = false
		return a, nil

	case refreshTickMsg:
		a.loading = true
		return a, tea.Batch(
			fetchNotificationsCmd(a.client, a.currentView),
			refreshTickCmd(),
		)

	case statusMsg:
		a.statusText = msg.text
		a.statusError = msg.isError
		return a, clearStatusCmd()
	}

	return a, nil
}

// handleKey routes keypresses to the appropriate handler based on focus.
func (a App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys (work regardless of focus)
	switch key {
	case "q", "ctrl+c":
		return a, tea.Quit
	case "?":
		// TODO: help overlay
		return a, nil
	case "1":
		return a.switchView(github.ViewUnread)
	case "2":
		return a.switchView(github.ViewAll)
	case "3":
		return a.switchView(github.ViewParticipating)
	case "tab":
		if a.focused == focusList {
			a.focused = focusPreview
		} else {
			a.focused = focusList
		}
		return a, nil
	}

	// List-focused keys
	if a.focused == focusList {
		return a.handleListKey(key)
	}

	return a, nil
}

func (a App) handleListKey(key string) (tea.Model, tea.Cmd) {
	filtered := a.filteredNotifications()

	switch key {
	// Navigation
	case "j", "down":
		a.cursor++
		a.clampCursor()
		return a, nil
	case "k", "up":
		a.cursor--
		a.clampCursor()
		return a, nil
	case "g":
		a.cursor = 0
		a.clampScroll()
		return a, nil
	case "G":
		if len(filtered) > 0 {
			a.cursor = len(filtered) - 1
		}
		a.clampScroll()
		return a, nil

	// Actions
	case "r":
		if n := a.selectedNotification(); n != nil {
			return a, markReadCmd(a.client, n.ID)
		}
	case "R":
		return a, markAllReadCmd(a.client)
	case "m":
		if n := a.selectedNotification(); n != nil {
			return a, muteThreadCmd(a.client, n.ID)
		}
	case "u":
		if n := a.selectedNotification(); n != nil {
			return a, unsubscribeCmd(a.client, n.ID)
		}
	case "enter":
		if n := a.selectedNotification(); n != nil {
			webURL := n.WebURL()
			if webURL != "" {
				return a, openBrowserCmd(webURL)
			}
		}
	}

	return a, nil
}

func (a App) switchView(view github.View) (App, tea.Cmd) {
	a.currentView = view
	a.cursor = 0
	a.offset = 0
	a.loading = true
	return a, fetchNotificationsCmd(a.client, view)
}

// View renders the entire UI. In the Elm Architecture, View is a pure function
// of the current state — it never modifies the model. Bubble Tea calls View
// after every Update to repaint the terminal.
func (a App) View() tea.View {
	if a.width == 0 {
		return tea.NewView("Loading...")
	}

	var b strings.Builder

	// Header: view tabs
	b.WriteString(a.renderTabs())
	b.WriteString("\n")

	// Filter indicators
	if a.repoFilter != "" || a.reasonFilter != "" {
		b.WriteString(a.renderFilters())
		b.WriteString("\n")
	}

	// Main content area
	contentHeight := a.contentHeight()
	if a.loading && len(a.notifications) == 0 {
		b.WriteString(a.renderCentered("Loading notifications...", contentHeight))
	} else {
		filtered := a.filteredNotifications()
		if len(filtered) == 0 {
			b.WriteString(a.renderCentered("🔔 No notifications!", contentHeight))
		} else {
			b.WriteString(a.renderNotificationList(filtered, contentHeight))
		}
	}

	// Status bar
	b.WriteString(a.renderStatusBar())

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// --- Rendering helpers ---

func (a App) renderTabs() string {
	views := []struct {
		label string
		view  github.View
		key   string
	}{
		{"Unread", github.ViewUnread, "1"},
		{"All", github.ViewAll, "2"},
		{"Participating", github.ViewParticipating, "3"},
	}

	var tabs []string
	for _, v := range views {
		style := lipgloss.NewStyle().Padding(0, 2)
		label := fmt.Sprintf("[%s] %s", v.key, v.label)
		if v.view == a.currentView {
			style = style.Foreground(theme.ColorBase).Background(theme.ActiveTab).Bold(true)
		} else {
			style = style.Foreground(theme.InactiveTab)
		}
		tabs = append(tabs, style.Render(label))
	}

	count := len(a.filteredNotifications())
	counter := lipgloss.NewStyle().
		Foreground(theme.ColorOverlay1).
		Render(fmt.Sprintf("  %d items", count))

	row := lipgloss.JoinHorizontal(lipgloss.Top, append(tabs, counter)...)
	return lipgloss.NewStyle().Width(a.width).Render(row)
}

func (a App) renderFilters() string {
	var parts []string
	if a.repoFilter != "" {
		parts = append(parts, fmt.Sprintf("repo:%s", a.repoFilter))
	}
	if a.reasonFilter != "" {
		parts = append(parts, fmt.Sprintf("reason:%s", a.reasonFilter))
	}
	label := strings.Join(parts, "  ")
	return lipgloss.NewStyle().
		Foreground(theme.ColorPeach).
		Italic(true).
		Width(a.width).
		Render("  Filters: " + label)
}

func (a App) renderNotificationList(notifications []github.Notification, height int) string {
	a.clampScroll()
	end := a.offset + height
	if end > len(notifications) {
		end = len(notifications)
	}
	visible := notifications[a.offset:end]

	var rows []string
	for i, n := range visible {
		idx := a.offset + i
		rows = append(rows, a.renderNotificationRow(n, idx == a.cursor))
	}

	// Pad remaining height
	for len(rows) < height {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}

func (a App) renderNotificationRow(n github.Notification, selected bool) string {
	icon := n.Icon()
	reason := n.ReasonLabel()
	repo := n.Repository.FullName
	title := n.Subject.Title
	ago := timeAgo(n.UpdatedAt)

	reasonStyled := lipgloss.NewStyle().Foreground(theme.ReasonColor).Width(8).Render(reason)
	repoStyled := lipgloss.NewStyle().Foreground(theme.RepoColor).Width(30).Render(truncate(repo, 30))
	agoStyled := lipgloss.NewStyle().Foreground(theme.TimeColor).Width(8).Align(lipgloss.Right).Render(ago)

	// Title gets remaining width
	titleWidth := a.width - 8 - 30 - 8 - 6 // reason + repo + ago + icon + padding
	if titleWidth < 10 {
		titleWidth = 10
	}
	titleStyled := truncate(title, titleWidth)

	row := fmt.Sprintf(" %s %s %s %s %s", icon, reasonStyled, repoStyled, titleStyled, agoStyled)

	style := lipgloss.NewStyle().Width(a.width)
	if selected {
		style = style.Background(theme.ColorSurface0).Foreground(theme.ColorText).Bold(true)
	} else if !n.Unread {
		style = style.Foreground(theme.Dimmed)
	} else {
		style = style.Foreground(theme.ColorText)
	}

	return style.Render(row)
}

func (a App) renderStatusBar() string {
	left := ""
	if a.statusText != "" {
		color := theme.StatusOK
		if a.statusError {
			color = theme.StatusError
		}
		left = lipgloss.NewStyle().Foreground(color).Render(a.statusText)
	}

	right := "q:quit  ?:help  r:read  m:mute  Enter:open"
	rightStyled := lipgloss.NewStyle().Foreground(theme.Dimmed).Render(right)

	gap := a.width - lipgloss.Width(left) - lipgloss.Width(rightStyled)
	if gap < 0 {
		gap = 0
	}

	bar := left + strings.Repeat(" ", gap) + rightStyled
	return lipgloss.NewStyle().
		Width(a.width).
		Background(theme.ColorSurface1).
		Render(bar)
}

func (a App) renderCentered(text string, height int) string {
	styled := lipgloss.NewStyle().Foreground(theme.ColorOverlay1).Render(text)
	return lipgloss.Place(a.width, height, lipgloss.Center, lipgloss.Center, styled)
}

// --- State helpers ---

func (a App) filteredNotifications() []github.Notification {
	if a.repoFilter == "" && a.reasonFilter == "" {
		return a.notifications
	}
	var result []github.Notification
	for _, n := range a.notifications {
		if a.repoFilter != "" && !strings.Contains(
			strings.ToLower(n.Repository.FullName),
			strings.ToLower(a.repoFilter),
		) {
			continue
		}
		if a.reasonFilter != "" && n.Reason != a.reasonFilter {
			continue
		}
		result = append(result, n)
	}
	return result
}

func (a App) selectedNotification() *github.Notification {
	filtered := a.filteredNotifications()
	if a.cursor < 0 || a.cursor >= len(filtered) {
		return nil
	}
	n := filtered[a.cursor]
	return &n
}

func (a *App) removeNotification(threadID string) {
	var kept []github.Notification
	for _, n := range a.notifications {
		if n.ID != threadID {
			kept = append(kept, n)
		}
	}
	a.notifications = kept
	a.clampCursor()
}

func (a *App) clampCursor() {
	filtered := a.filteredNotifications()
	max := len(filtered) - 1
	if max < 0 {
		max = 0
	}
	if a.cursor > max {
		a.cursor = max
	}
	if a.cursor < 0 {
		a.cursor = 0
	}
	a.clampScroll()
}

func (a *App) clampScroll() {
	height := a.contentHeight()
	if height <= 0 {
		return
	}
	// Ensure cursor is visible
	if a.cursor < a.offset {
		a.offset = a.cursor
	}
	if a.cursor >= a.offset+height {
		a.offset = a.cursor - height + 1
	}
}

func (a App) contentHeight() int {
	// height minus tabs (1) + status bar (1) + possible filter line
	used := 2
	if a.repoFilter != "" || a.reasonFilter != "" {
		used++
	}
	h := a.height - used
	if h < 1 {
		h = 1
	}
	return h
}

// --- Utilities ---

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
