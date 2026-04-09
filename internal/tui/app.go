package tui

import (
	"fmt"
	"log"
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

// filterMode tracks whether the user is typing a filter query.
type filterMode int

const (
	filterNone filterMode = iota
	filterRepo
	filterTitleSearch
)

// participatingReasons are the GitHub notification reasons that indicate
// direct involvement (as opposed to watching/subscribed).
var participatingReasons = map[string]bool{
	"assign":             true,
	"author":             true,
	"comment":            true,
	"mention":            true,
	"review_requested":   true,
	"team_mention":       true,
	"manual":             true,
	"approval_requested": true,
}

// spinnerFrames are the animation frames for the loading spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

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
	showHelp    bool

	// Filters
	repoFilter    string
	reasonFilter  string
	typeFilter    string
	orgFilter     string
	ageFilter     int // 0=all, 1=24h, 2=7d, 3=30d
	titleSearch   string
	participating bool
	filterInput   filterMode
	filterBuf     string

	// Preview
	previewScroll  int
	detailCache    map[string]*github.ThreadDetail // cached enriched details by thread ID
	detailLoading  string                          // thread ID currently being fetched
	spinnerFrame   int                             // animation frame for loading spinner

	// Double-key tracking (for gg)
	lastKey     string
	lastKeyTime time.Time

	// Known values for cycling filters (collected from loaded notifications)
	knownReasons []string
	knownTypes   []string
	knownOrgs    []string

	// New notification tracking (set on each refresh)
	newNotificationIDs map[string]bool

	// Header cache (rebuilt on resize)
	headerCache string
}

// NewApp creates an App wired to the given GitHub API client.
func NewApp(client *github.Client) App {
	return App{
		client:      client,
		currentView: github.ViewUnread,
		loading:     true,
		detailCache: make(map[string]*github.ThreadDetail),
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
		log.Printf("update: WindowSizeMsg %dx%d", msg.Width, msg.Height)
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case tea.KeyPressMsg:
		return a.handleKey(msg)

	case notificationsLoadedMsg:
		log.Printf("update: notificationsLoadedMsg with %d notifications", len(msg.notifications))

		// Preserve selection: remember which notification was selected
		// before replacing the list, then restore cursor position after.
		var selectedID string
		if sel := a.selectedNotification(); sel != nil {
			selectedID = sel.ID
		}

		// Track new items: compare old IDs with incoming IDs.
		// On initial load (no previous data), don't mark anything as new.
		oldIDs := make(map[string]bool, len(a.notifications))
		for _, n := range a.notifications {
			oldIDs[n.ID] = true
		}
		newIDs := make(map[string]bool)
		if len(oldIDs) > 0 {
			for _, n := range msg.notifications {
				if !oldIDs[n.ID] {
					newIDs[n.ID] = true
				}
			}
		}
		a.newNotificationIDs = newIDs

		// Group new notifications at the top (preserving chronological order
		// within each group) so the • indicators aren't scattered.
		if len(newIDs) > 0 {
			var newNotifs, existingNotifs []github.Notification
			for _, n := range msg.notifications {
				if newIDs[n.ID] {
					newNotifs = append(newNotifs, n)
				} else {
					existingNotifs = append(existingNotifs, n)
				}
			}
			a.notifications = append(newNotifs, existingNotifs...)
		} else {
			a.notifications = msg.notifications
		}
		a.loading = false
		a.statusText = ""
		a.statusError = false
		a.collectFilterOptions()

		// Restore cursor to the previously selected notification
		if selectedID != "" {
			filtered := a.filteredNotifications()
			found := false
			for i, n := range filtered {
				if n.ID == selectedID {
					a.cursor = i
					found = true
					break
				}
			}
			if !found {
				a.cursor = 0
			}
		}
		a.clampCursor()

		// Fetch enriched detail for the currently selected notification
		detailCmd := a.maybeFetchDetail()

		if len(newIDs) > 0 {
			a.statusText = fmt.Sprintf("%d new", len(newIDs))
			a.statusError = false
			return a, tea.Batch(clearStatusCmd(), detailCmd)
		}
		return a, detailCmd

	case errorMsg:
		log.Printf("update: errorMsg: %v", msg.err)
		a.loading = false
		a.statusText = fmt.Sprintf("Error: %v  (press ctrl+r to retry)", msg.err)
		a.statusError = true
		return a, nil

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

	case threadDetailLoadedMsg:
		a.detailCache[msg.threadID] = msg.detail
		if a.detailLoading == msg.threadID {
			a.detailLoading = ""
		}
		return a, nil

	case threadDetailErrorMsg:
		if a.detailLoading == msg.threadID {
			a.detailLoading = ""
		}
		return a, nil

	case spinnerTickMsg:
		// Only keep ticking while we're actively loading a detail
		if a.detailLoading != "" {
			a.spinnerFrame = (a.spinnerFrame + 1) % len(spinnerFrames)
			return a, spinnerTickCmd()
		}
		return a, nil
	}

	return a, nil
}

// handleKey routes keypresses to the appropriate handler based on focus.
func (a App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// If we're in filter input mode, route everything there
	if a.filterInput != filterNone {
		return a.handleFilterInput(key)
	}

	// Help overlay intercepts most keys
	if a.showHelp {
		if key == "?" || key == "escape" || key == "esc" || key == "q" {
			a.showHelp = false
		}
		return a, nil
	}

	// Global keys (work regardless of focus)
	switch key {
	case "q", "ctrl+c":
		return a, tea.Quit
	case "?":
		a.showHelp = !a.showHelp
		return a, nil
	case "ctrl+r":
		a.loading = true
		a.statusText = ""
		a.statusError = false
		return a, fetchNotificationsCmd(a.client, a.currentView)
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
	case "/":
		a.filterInput = filterRepo
		a.filterBuf = a.repoFilter
		return a, nil
	case "s":
		a.filterInput = filterTitleSearch
		a.filterBuf = a.titleSearch
		return a, nil
	case "f":
		if a.focused == focusList {
			a.cycleReasonFilter()
			a.cursor = 0
			a.offset = 0
			return a, nil
		}
	case "escape", "esc":
		// Clear all filters
		if a.hasActiveFilters() {
			a.repoFilter = ""
			a.reasonFilter = ""
			a.typeFilter = ""
			a.orgFilter = ""
			a.ageFilter = 0
			a.titleSearch = ""
			a.participating = false
			a.cursor = 0
			a.offset = 0
			return a, nil
		}
	}

	if a.focused == focusList {
		return a.handleListKey(key)
	}
	if a.focused == focusPreview {
		return a.handlePreviewKey(key)
	}

	return a, nil
}

func (a App) handleFilterInput(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		// Confirm filter and exit filter mode
		a.filterInput = filterNone
		return a, nil
	case "escape", "esc":
		// Cancel: clear the active filter and exit
		switch a.filterInput {
		case filterRepo:
			a.repoFilter = ""
		case filterTitleSearch:
			a.titleSearch = ""
		}
		a.filterBuf = ""
		a.filterInput = filterNone
		a.cursor = 0
		a.offset = 0
		return a, nil
	case "backspace":
		if len(a.filterBuf) > 0 {
			a.filterBuf = a.filterBuf[:len(a.filterBuf)-1]
		}
	default:
		if len(key) == 1 {
			a.filterBuf += key
		}
	}
	// Live filter: apply as user types
	switch a.filterInput {
	case filterRepo:
		a.repoFilter = a.filterBuf
	case filterTitleSearch:
		a.titleSearch = a.filterBuf
	}
	a.cursor = 0
	a.offset = 0
	return a, nil
}

func (a App) handlePreviewKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		a.previewScroll++
		return a, nil
	case "k", "up":
		if a.previewScroll > 0 {
			a.previewScroll--
		}
		return a, nil
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
		a.previewScroll = 0
		return a, a.maybeFetchDetail()
	case "k", "up":
		a.cursor--
		a.clampCursor()
		a.previewScroll = 0
		return a, a.maybeFetchDetail()
	case "g":
		// Double-tap 'g' for jump to top
		now := time.Now()
		if a.lastKey == "g" && now.Sub(a.lastKeyTime) < 500*time.Millisecond {
			a.cursor = 0
			a.clampScroll()
			a.lastKey = ""
			a.previewScroll = 0
			return a, a.maybeFetchDetail()
		}
		a.lastKey = "g"
		a.lastKeyTime = now
		return a, nil
	case "G":
		if len(filtered) > 0 {
			a.cursor = len(filtered) - 1
		}
		a.clampScroll()
		a.previewScroll = 0
		return a, a.maybeFetchDetail()

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

	// Filters (cycling)
	case "t":
		a.cycleTypeFilter()
		a.cursor = 0
		a.offset = 0
		return a, nil
	case "p":
		a.participating = !a.participating
		a.cursor = 0
		a.offset = 0
		return a, nil
	case "o":
		a.cycleOrgFilter()
		a.cursor = 0
		a.offset = 0
		return a, nil
	case "a":
		a.cycleAgeFilter()
		a.cursor = 0
		a.offset = 0
		return a, nil

	case "enter":
		if n := a.selectedNotification(); n != nil {
			webURL := n.WebURL()
			if webURL != "" {
				return a, openBrowserCmd(webURL)
			}
		}
	}

	// Clear double-key state for non-g keys
	if key != "g" {
		a.lastKey = ""
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

	if a.showHelp {
		v := tea.NewView(a.renderHelpOverlay())
		v.AltScreen = true
		return v
	}

	var b strings.Builder

	// Header with ASCII art and tips
	header := a.buildHeader()
	b.WriteString(header)
	b.WriteString("\n")

	// View tabs
	b.WriteString(a.renderTabs())
	b.WriteString("\n")

	// Filter indicators
	if a.hasActiveFilters() || a.filterInput != filterNone {
		b.WriteString(a.renderFilters())
		b.WriteString("\n")
	}

	// Main content area
	contentHeight := a.contentHeight()
	if a.loading && len(a.notifications) == 0 {
		b.WriteString(a.renderCentered("⟳ Loading notifications...", contentHeight))
	} else {
		filtered := a.filteredNotifications()
		if len(filtered) == 0 {
			if a.statusError {
				b.WriteString(a.renderCentered("⚠ Failed to load notifications — press Ctrl+R to retry", contentHeight))
			} else {
				b.WriteString(a.renderCentered("🔔 No notifications!", contentHeight))
			}
		} else {
			b.WriteString(a.renderMainContent(filtered, contentHeight))
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
	if a.filterInput == filterRepo {
		prompt := lipgloss.NewStyle().Foreground(theme.ColorMauve).Bold(true).Render("  Filter repo: ")
		input := lipgloss.NewStyle().Foreground(theme.ColorText).Render(a.filterBuf + "▏")
		return lipgloss.NewStyle().Width(a.width).Render(prompt + input)
	}
	if a.filterInput == filterTitleSearch {
		prompt := lipgloss.NewStyle().Foreground(theme.ColorMauve).Bold(true).Render("  Search: ")
		input := lipgloss.NewStyle().Foreground(theme.ColorText).Render(a.filterBuf + "▏")
		return lipgloss.NewStyle().Width(a.width).Render(prompt + input)
	}

	var parts []string
	if a.repoFilter != "" {
		parts = append(parts, fmt.Sprintf("repo:%s", a.repoFilter))
	}
	if a.reasonFilter != "" {
		parts = append(parts, fmt.Sprintf("reason:%s", a.reasonFilter))
	}
	if a.typeFilter != "" {
		parts = append(parts, fmt.Sprintf("type:%s", a.typeFilter))
	}
	if a.orgFilter != "" {
		parts = append(parts, fmt.Sprintf("org:%s", a.orgFilter))
	}
	if a.participating {
		parts = append(parts, "participating")
	}
	if a.ageFilter != 0 {
		parts = append(parts, fmt.Sprintf("age:≤%s", ageFilterLabel(a.ageFilter)))
	}
	if a.titleSearch != "" {
		parts = append(parts, fmt.Sprintf("search:%s", a.titleSearch))
	}
	label := strings.Join(parts, "  ")
	hint := lipgloss.NewStyle().Foreground(theme.Dimmed).Render("  (Esc to clear)")
	return lipgloss.NewStyle().
		Foreground(theme.ColorPeach).
		Italic(true).
		Width(a.width).
		Render("  Filters: " + label + hint)
}

func (a App) renderMainContent(notifications []github.Notification, height int) string {
	// Split: 60% list, 40% preview (when wide enough)
	if a.width >= 100 {
		listWidth := a.width * 6 / 10
		previewWidth := a.width - listWidth - 1 // 1 for border

		listContent := a.renderNotificationListSized(notifications, height, listWidth)
		previewContent := a.renderPreview(height, previewWidth)

		border := lipgloss.NewStyle().
			Foreground(theme.Border).
			Render(strings.Repeat("│\n", height))

		return lipgloss.JoinHorizontal(lipgloss.Top, listContent, border, previewContent)
	}
	// Narrow terminal: list only
	return a.renderNotificationList(notifications, height)
}

func (a App) renderPreview(height, width int) string {
	n := a.selectedNotification()
	if n == nil {
		msg := lipgloss.NewStyle().Foreground(theme.Dimmed).Render("No notification selected")
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, msg)
	}

	var lines []string

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(theme.ColorText).Bold(true).Width(width)
	lines = append(lines, titleStyle.Render(n.Subject.Title))
	lines = append(lines, "")

	// Metadata
	dim := lipgloss.NewStyle().Foreground(theme.Dimmed)
	val := lipgloss.NewStyle().Foreground(theme.ColorSubtext1)

	lines = append(lines, dim.Render("  Type:    ")+val.Render(n.Subject.Type))
	lines = append(lines, dim.Render("  Repo:    ")+lipgloss.NewStyle().Foreground(theme.RepoColor).Render(n.Repository.FullName))
	lines = append(lines, dim.Render("  Reason:  ")+lipgloss.NewStyle().Foreground(theme.ReasonColor).Render(n.ReasonLabel()))
	lines = append(lines, dim.Render("  Updated: ")+val.Render(n.UpdatedAt.Local().Format("Jan 02, 15:04")))

	status := "Read"
	statusColor := theme.Dimmed
	if n.Unread {
		status = "Unread"
		statusColor = theme.ColorGreen
	}
	lines = append(lines, dim.Render("  Status:  ")+lipgloss.NewStyle().Foreground(statusColor).Render(status))

	if n.Repository.Private {
		lines = append(lines, dim.Render("  Scope:   ")+lipgloss.NewStyle().Foreground(theme.ColorYellow).Render("Private"))
	}

	// Enriched detail from lazy fetch
	detail, cached := a.detailCache[n.ID]
	if cached && detail != nil {
		lines = append(lines, "")
		lines = a.renderEnrichedDetail(lines, detail, n.Subject.Type, width)
	} else if a.detailLoading == n.ID {
		lines = append(lines, "")
		frame := spinnerFrames[a.spinnerFrame%len(spinnerFrames)]
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.ColorLavender).Render("  "+frame+" Loading details..."))
	}

	lines = append(lines, "")

	// Web URL
	webURL := n.WebURL()
	if webURL != "" {
		lines = append(lines, dim.Render("  URL: ")+lipgloss.NewStyle().Foreground(theme.ColorSapphire).Underline(true).Render(webURL))
	}

	lines = append(lines, "")

	// Focus indicator
	if a.focused == focusPreview {
		indicator := lipgloss.NewStyle().Foreground(theme.ActiveTab).Render("  ▸ Preview focused (j/k to scroll, Tab to switch)")
		lines = append(lines, indicator)
	} else {
		hint := lipgloss.NewStyle().Foreground(theme.Dimmed).Render("  Tab to focus preview")
		lines = append(lines, hint)
	}

	// Apply scroll
	if a.previewScroll > 0 && a.previewScroll < len(lines) {
		lines = lines[a.previewScroll:]
	}

	// Pad or trim to height
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

// renderEnrichedDetail appends enriched information from the lazy-fetched ThreadDetail.
func (a App) renderEnrichedDetail(lines []string, detail *github.ThreadDetail, subjectType string, width int) []string {
	dim := lipgloss.NewStyle().Foreground(theme.Dimmed)
	val := lipgloss.NewStyle().Foreground(theme.ColorSubtext1)
	bodyWidth := width - 4 // 2 indent + 2 margin

	// State with color coding
	if detail.State != "" {
		stateColor := theme.ColorGreen
		stateLabel := detail.State
		if detail.Merged {
			stateLabel = "merged"
			stateColor = theme.ColorMauve
		} else if detail.State == "closed" {
			stateColor = theme.ColorRed
		}
		if detail.Draft {
			stateLabel += " (draft)"
		}
		lines = append(lines, dim.Render("  State:   ")+lipgloss.NewStyle().Foreground(stateColor).Render(stateLabel))
	}

	// Author
	if detail.User.Login != "" {
		lines = append(lines, dim.Render("  Author:  ")+val.Render("@"+detail.User.Login))
	}

	// Labels
	if len(detail.Labels) > 0 {
		var labelNames []string
		for _, l := range detail.Labels {
			labelNames = append(labelNames, l.Name)
		}
		labelStr := strings.Join(labelNames, ", ")
		lines = append(lines, dim.Render("  Labels:  ")+lipgloss.NewStyle().Foreground(theme.ColorYellow).Render(labelStr))
	}

	// PR-specific stats
	if subjectType == "PullRequest" && (detail.Additions > 0 || detail.Deletions > 0) {
		stats := fmt.Sprintf("+%d / -%d", detail.Additions, detail.Deletions)
		lines = append(lines, dim.Render("  Changes: ")+val.Render(stats))
	}

	// Release tag
	if subjectType == "Release" && detail.TagName != "" {
		lines = append(lines, dim.Render("  Tag:     ")+val.Render(detail.TagName))
	}

	// Body (word-wrapped, full content — preview is scrollable)
	if detail.Body != "" {
		lines = append(lines, "")
		lines = append(lines, dim.Render("  ─── Description ───"))
		bodyLines := wordWrap(detail.Body, bodyWidth)
		for _, bl := range bodyLines {
			lines = append(lines, "  "+val.Render(bl))
		}
	}

	// Latest comment
	if detail.LatestComment != nil && detail.LatestComment.Body != "" {
		c := detail.LatestComment
		lines = append(lines, "")
		commentHeader := fmt.Sprintf("  ─── Comment by @%s (%s) ───",
			c.User.Login, c.CreatedAt.Local().Format("Jan 02, 15:04"))
		lines = append(lines, dim.Render(commentHeader))
		commentLines := wordWrap(c.Body, bodyWidth)
		for _, cl := range commentLines {
			lines = append(lines, "  "+val.Render(cl))
		}
	}

	return lines
}

// wordWrap breaks text into lines of at most maxWidth characters, splitting on
// whitespace. It also splits on existing newlines in the text.
func wordWrap(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(paragraph)
		line := ""
		for _, w := range words {
			if line == "" {
				line = w
			} else if len(line)+1+len(w) <= maxWidth {
				line += " " + w
			} else {
				result = append(result, line)
				line = w
			}
		}
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func (a App) renderNotificationListSized(notifications []github.Notification, height, width int) string {
	a.clampScroll()
	end := a.offset + height
	if end > len(notifications) {
		end = len(notifications)
	}
	visible := notifications[a.offset:end]

	var rows []string
	for i, n := range visible {
		idx := a.offset + i
		rows = append(rows, a.renderNotificationRowSized(n, idx == a.cursor, width))
	}

	for len(rows) < height {
		rows = append(rows, strings.Repeat(" ", width))
	}

	return strings.Join(rows, "\n")
}

func (a App) renderNotificationRowSized(n github.Notification, selected bool, width int) string {
	icon := n.Icon()
	reason := truncate(n.ReasonLabel(), 10)
	repo := truncate(n.Repository.FullName, 24)
	ago := timeAgo(n.UpdatedAt)
	isNew := a.newNotificationIDs[n.ID]

	// Fixed column widths: icon(1) + gap(1) + reason(10) + gap(1) + repo(24) + gap(1) + title(flex) + gap(1) + ago(5)
	const iconW, reasonW, repoW, agoW, padding = 1, 10, 24, 5, 6
	titleWidth := width - iconW - reasonW - repoW - agoW - padding
	if titleWidth < 10 {
		titleWidth = 10
	}
	title := truncate(n.Subject.Title, titleWidth)

	if selected {
		// For selected rows, build plain padded text and let the row style
		// apply background uniformly — no per-column styling that creates gaps.
		row := fmt.Sprintf("▌%s %s %s %s %s",
			icon,
			padRight(reason, reasonW),
			padRight(repo, repoW),
			padRight(title, titleWidth),
			padLeft(ago, agoW),
		)
		return lipgloss.NewStyle().
			Width(width).MaxWidth(width).
			Background(theme.ColorSurface2).
			Foreground(theme.ColorText).
			Bold(true).
			Render(row)
	}

	// Non-selected: show • for new notifications, space for old
	prefix := " "
	if isNew {
		prefix = "•"
	}

	// Non-selected rows use per-column colors
	reasonStyle := lipgloss.NewStyle().Foreground(theme.ReasonColor).Width(reasonW).MaxWidth(reasonW)
	repoStyle := lipgloss.NewStyle().Foreground(theme.RepoColor).Width(repoW).MaxWidth(repoW)
	titleStyle := lipgloss.NewStyle().Foreground(theme.ColorText).Width(titleWidth).MaxWidth(titleWidth)
	agoStyle := lipgloss.NewStyle().Foreground(theme.TimeColor).Width(agoW).MaxWidth(agoW).Align(lipgloss.Right)

	if !n.Unread {
		reasonStyle = reasonStyle.Foreground(theme.Dimmed)
		repoStyle = repoStyle.Foreground(theme.Dimmed)
		titleStyle = titleStyle.Foreground(theme.Dimmed)
		agoStyle = agoStyle.Foreground(theme.Dimmed)
	}

	row := fmt.Sprintf("%s%s %s %s %s %s",
		prefix,
		icon,
		reasonStyle.Render(reason),
		repoStyle.Render(repo),
		titleStyle.Render(title),
		agoStyle.Render(ago),
	)

	style := lipgloss.NewStyle().Width(width).MaxWidth(width)
	if isNew {
		style = style.Foreground(theme.ColorGreen)
	} else if !n.Unread {
		style = style.Foreground(theme.Dimmed)
	} else {
		style = style.Foreground(theme.ColorText)
	}

	return style.Render(row)
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
	reason := truncate(n.ReasonLabel(), 10)
	repo := truncate(n.Repository.FullName, 28)
	ago := timeAgo(n.UpdatedAt)
	isNew := a.newNotificationIDs[n.ID]

	// Fixed column widths: icon(1) + gap(1) + reason(10) + gap(1) + repo(28) + gap(1) + title(flex) + gap(1) + ago(5)
	const iconW, reasonW, repoW, agoW, padding = 1, 10, 28, 5, 6
	titleWidth := a.width - iconW - reasonW - repoW - agoW - padding
	if titleWidth < 10 {
		titleWidth = 10
	}
	title := truncate(n.Subject.Title, titleWidth)

	if selected {
		row := fmt.Sprintf("▌%s %s %s %s %s",
			icon,
			padRight(reason, reasonW),
			padRight(repo, repoW),
			padRight(title, titleWidth),
			padLeft(ago, agoW),
		)
		return lipgloss.NewStyle().
			Width(a.width).MaxWidth(a.width).
			Background(theme.ColorSurface2).
			Foreground(theme.ColorText).
			Bold(true).
			Render(row)
	}

	prefix := " "
	if isNew {
		prefix = "•"
	}

	reasonStyle := lipgloss.NewStyle().Foreground(theme.ReasonColor).Width(reasonW).MaxWidth(reasonW)
	repoStyle := lipgloss.NewStyle().Foreground(theme.RepoColor).Width(repoW).MaxWidth(repoW)
	titleStyle := lipgloss.NewStyle().Foreground(theme.ColorText).Width(titleWidth).MaxWidth(titleWidth)
	agoStyle := lipgloss.NewStyle().Foreground(theme.TimeColor).Width(agoW).MaxWidth(agoW).Align(lipgloss.Right)

	if !n.Unread {
		reasonStyle = reasonStyle.Foreground(theme.Dimmed)
		repoStyle = repoStyle.Foreground(theme.Dimmed)
		titleStyle = titleStyle.Foreground(theme.Dimmed)
		agoStyle = agoStyle.Foreground(theme.Dimmed)
	}

	row := fmt.Sprintf("%s%s %s %s %s %s",
		prefix,
		icon,
		reasonStyle.Render(reason),
		repoStyle.Render(repo),
		titleStyle.Render(title),
		agoStyle.Render(ago),
	)

	style := lipgloss.NewStyle().Width(a.width).MaxWidth(a.width)
	if isNew {
		style = style.Foreground(theme.ColorGreen)
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

	right := "q:quit  ?:help  r:read  m:mute  /:repo  s:search  t:type  p:part  a:age"
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
	if !a.hasActiveFilters() {
		return a.notifications
	}
	var ageCutoff time.Time
	if a.ageFilter > 0 {
		ageCutoff = time.Now().Add(-ageFilterDuration(a.ageFilter))
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
		if a.typeFilter != "" && n.Subject.Type != a.typeFilter {
			continue
		}
		if a.orgFilter != "" && orgFromFullName(n.Repository.FullName) != a.orgFilter {
			continue
		}
		if a.participating && !participatingReasons[n.Reason] {
			continue
		}
		if a.ageFilter > 0 && n.UpdatedAt.Before(ageCutoff) {
			continue
		}
		if a.titleSearch != "" && !strings.Contains(
			strings.ToLower(n.Subject.Title),
			strings.ToLower(a.titleSearch),
		) {
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

func (a *App) cycleReasonFilter() {
	if len(a.knownReasons) == 0 {
		return
	}
	if a.reasonFilter == "" {
		a.reasonFilter = a.knownReasons[0]
		return
	}
	for i, r := range a.knownReasons {
		if r == a.reasonFilter {
			next := (i + 1) % (len(a.knownReasons) + 1) // +1 for "no filter"
			if next == len(a.knownReasons) {
				a.reasonFilter = ""
			} else {
				a.reasonFilter = a.knownReasons[next]
			}
			return
		}
	}
	a.reasonFilter = ""
}

func (a *App) cycleTypeFilter() {
	if len(a.knownTypes) == 0 {
		return
	}
	if a.typeFilter == "" {
		a.typeFilter = a.knownTypes[0]
		return
	}
	for i, t := range a.knownTypes {
		if t == a.typeFilter {
			next := (i + 1) % (len(a.knownTypes) + 1)
			if next == len(a.knownTypes) {
				a.typeFilter = ""
			} else {
				a.typeFilter = a.knownTypes[next]
			}
			return
		}
	}
	a.typeFilter = ""
}

func (a *App) cycleOrgFilter() {
	if len(a.knownOrgs) == 0 {
		return
	}
	if a.orgFilter == "" {
		a.orgFilter = a.knownOrgs[0]
		return
	}
	for i, o := range a.knownOrgs {
		if o == a.orgFilter {
			next := (i + 1) % (len(a.knownOrgs) + 1)
			if next == len(a.knownOrgs) {
				a.orgFilter = ""
			} else {
				a.orgFilter = a.knownOrgs[next]
			}
			return
		}
	}
	a.orgFilter = ""
}

func (a *App) cycleAgeFilter() {
	a.ageFilter = (a.ageFilter + 1) % 4 // 0=all, 1=24h, 2=7d, 3=30d
}

// ageFilterDuration returns the time.Duration for the given age filter value.
func ageFilterDuration(age int) time.Duration {
	switch age {
	case 1:
		return 24 * time.Hour
	case 2:
		return 7 * 24 * time.Hour
	case 3:
		return 30 * 24 * time.Hour
	default:
		return 0
	}
}

// ageFilterLabel returns a human-readable label for the age filter.
func ageFilterLabel(age int) string {
	switch age {
	case 1:
		return "24h"
	case 2:
		return "7d"
	case 3:
		return "30d"
	default:
		return ""
	}
}

func (a *App) collectFilterOptions() {
	seenReasons := make(map[string]bool)
	seenTypes := make(map[string]bool)
	seenOrgs := make(map[string]bool)
	var reasons, types, orgs []string
	for _, n := range a.notifications {
		if !seenReasons[n.Reason] {
			seenReasons[n.Reason] = true
			reasons = append(reasons, n.Reason)
		}
		if !seenTypes[n.Subject.Type] {
			seenTypes[n.Subject.Type] = true
			types = append(types, n.Subject.Type)
		}
		org := orgFromFullName(n.Repository.FullName)
		if org != "" && !seenOrgs[org] {
			seenOrgs[org] = true
			orgs = append(orgs, org)
		}
	}
	a.knownReasons = reasons
	a.knownTypes = types
	a.knownOrgs = orgs
}

// maybeFetchDetail returns a Cmd to lazily fetch enriched detail for the
// currently selected notification, or nil if already cached/loading.
func (a *App) maybeFetchDetail() tea.Cmd {
	n := a.selectedNotification()
	if n == nil || a.client == nil {
		return nil
	}
	if _, ok := a.detailCache[n.ID]; ok {
		return nil // already cached
	}
	if a.detailLoading == n.ID {
		return nil // already fetching
	}
	a.detailLoading = n.ID
	a.spinnerFrame = 0
	// Batch both the fetch command and the spinner tick so the spinner
	// starts animating immediately while we wait for the API response.
	return tea.Batch(
		fetchThreadDetailCmd(a.client, n.ID, n.Subject.URL, n.Subject.LatestCommentURL),
		spinnerTickCmd(),
	)
}

// orgFromFullName extracts the owner/org from "owner/repo".
func orgFromFullName(fullName string) string {
	if i := strings.IndexByte(fullName, '/'); i > 0 {
		return fullName[:i]
	}
	return fullName
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

func (a App) hasActiveFilters() bool {
	return a.repoFilter != "" || a.reasonFilter != "" || a.typeFilter != "" ||
		a.orgFilter != "" || a.ageFilter != 0 || a.titleSearch != "" || a.participating
}

func (a App) contentHeight() int {
	// height minus header + tabs (1) + status bar (1) + possible filter line
	headerH := lipgloss.Height(a.buildHeader()) + 1 // +1 for the newline after header
	used := headerH + 2                              // tabs(1) + status bar(1)
	if a.hasActiveFilters() || a.filterInput != filterNone {
		used++
	}
	h := a.height - used
	if h < 1 {
		h = 1
	}
	return h
}

// --- Header ---

// bellASCII is the ASCII art title for gh-bell.
var bellASCII = []string{
	"  ▄▄ ▗▖ ▗▖     ▗▄▄▖      ▗▄▖  ▗▄▖  ",
	" █▀▀▌▐▌ ▐▌     ▐▛▀▜▌     ▝▜▌  ▝▜▌  ",
	"▐▌   ▐▌ ▐▌     ▐▌ ▐▌ ▟█▙  ▐▌   ▐▌  ",
	"▐▌▗▄▖▐███▌     ▐███ ▐▙▄▟▌ ▐▌   ▐▌  ",
	"▐▌▝▜▌▐▌ ▐▌     ▐▌ ▐▌▐▛▀▀▘ ▐▌   ▐▌  ",
	" █▄▟▌▐▌ ▐▌     ▐▙▄▟▌▝█▄▄▌ ▐▙▄  ▐▙▄ ",
	"  ▀▀ ▝▘ ▝▘     ▝▀▀▀  ▝▀▀   ▀▀   ▀▀",
}

func colorizeASCII(lines []string) string {
	style := lipgloss.NewStyle().Foreground(theme.ColorLavender)
	var result strings.Builder
	for i, line := range lines {
		result.WriteString(style.Render(line))
		if i < len(lines)-1 {
			result.WriteRune('\n')
		}
	}
	return result.String()
}

func (a App) buildHeader() string {
	art := colorizeASCII(bellASCII)

	tipKey := lipgloss.NewStyle().Foreground(theme.ColorMauve).Bold(true)
	tipText := lipgloss.NewStyle().Foreground(theme.ColorOverlay1)
	tip := tipText.Render("  Tip: ") +
		tipKey.Render("?") + tipText.Render(" help · ") +
		tipKey.Render("/") + tipText.Render(" repo · ") +
		tipKey.Render("s") + tipText.Render(" search · ") +
		tipKey.Render("1/2/3") + tipText.Render(" views")

	inner := art + "\n\n" + tip

	return lipgloss.NewStyle().
		Width(a.width).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.ColorSurface2).
		BorderBottom(true).
		BorderLeft(false).
		BorderRight(false).
		BorderTop(false).
		Render(inner)
}

// --- Utilities ---

func (a App) renderHelpOverlay() string {
	heading := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.ColorMauve)

	keyStyle := lipgloss.NewStyle().Foreground(theme.ColorText).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(theme.ColorSubtext1)

	line := func(k, d string) string {
		return "  " + keyStyle.Render(padRight(k, 12)) + " " + descStyle.Render(d)
	}

	var b strings.Builder

	b.WriteString(heading.Render("Navigation"))
	b.WriteByte('\n')
	b.WriteString(line("j/k", "Move down/up"))
	b.WriteByte('\n')
	b.WriteString(line("gg/G", "Jump to top/bottom"))
	b.WriteByte('\n')
	b.WriteString(line("Tab", "Switch focus (list/preview)"))
	b.WriteByte('\n')
	b.WriteByte('\n')

	b.WriteString(heading.Render("Actions"))
	b.WriteByte('\n')
	b.WriteString(line("Enter", "Open in browser"))
	b.WriteByte('\n')
	b.WriteString(line("r", "Mark as read"))
	b.WriteByte('\n')
	b.WriteString(line("R", "Mark all as read"))
	b.WriteByte('\n')
	b.WriteString(line("m", "Mute thread"))
	b.WriteByte('\n')
	b.WriteString(line("u", "Unsubscribe"))
	b.WriteByte('\n')
	b.WriteByte('\n')

	b.WriteString(heading.Render("Filters & Views"))
	b.WriteByte('\n')
	b.WriteString(line("1/2/3", "Unread/All/Participating"))
	b.WriteByte('\n')
	b.WriteString(line("/", "Filter by repo"))
	b.WriteByte('\n')
	b.WriteString(line("s", "Search titles"))
	b.WriteByte('\n')
	b.WriteString(line("f", "Cycle reason filter"))
	b.WriteByte('\n')
	b.WriteString(line("t", "Cycle type filter"))
	b.WriteByte('\n')
	b.WriteString(line("o", "Cycle org filter"))
	b.WriteByte('\n')
	b.WriteString(line("a", "Cycle age filter"))
	b.WriteByte('\n')
	b.WriteString(line("p", "Toggle participating"))
	b.WriteByte('\n')
	b.WriteString(line("Esc", "Clear filters"))
	b.WriteByte('\n')
	b.WriteByte('\n')

	b.WriteString(heading.Render("General"))
	b.WriteByte('\n')
	b.WriteString(line("?", "Toggle this help"))
	b.WriteByte('\n')
	b.WriteString(line("Ctrl+R", "Refresh notifications"))
	b.WriteByte('\n')
	b.WriteString(line("q", "Quit"))

	content := b.String()

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.ColorMauve).
		Padding(0, 1).
		Render("gh-bell  Keybindings")

	boxWidth := 48
	if boxWidth > a.width-4 {
		boxWidth = a.width - 4
	}
	if boxWidth < 20 {
		boxWidth = 20
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ColorMauve).
		Foreground(theme.ColorText).
		Padding(1, 2).
		Width(boxWidth)

	inner := lipgloss.JoinVertical(lipgloss.Left, title, "", content)
	rendered := box.Render(inner)

	// Center the box in the available space
	boxH := lipgloss.Height(rendered)
	boxW := lipgloss.Width(rendered)

	padLeft := (a.width - boxW) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	padTop := (a.height - boxH) / 2
	if padTop < 0 {
		padTop = 0
	}

	helpLines := strings.Split(rendered, "\n")
	leftPad := strings.Repeat(" ", padLeft)
	for i, l := range helpLines {
		helpLines[i] = leftPad + l
	}

	topPad := strings.Repeat("\n", padTop)
	return topPad + strings.Join(helpLines, "\n")
}

// padRight pads s with spaces to the given width.
func padRight(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(r))
}

// padLeft pads s with leading spaces to the given width.
func padLeft(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return s
	}
	return strings.Repeat(" ", w-len(r)) + s
}

func truncate(s string, max int) string {
	// Use rune-aware truncation for proper display width
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
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
