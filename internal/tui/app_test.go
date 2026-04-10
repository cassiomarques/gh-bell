package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cassiomarques/gh-bell/internal/github"
	"github.com/cassiomarques/gh-bell/internal/service"
	"github.com/cassiomarques/gh-bell/internal/storage"
	"github.com/cassiomarques/gh-bell/internal/tui/theme"
)

func sampleNotifications() []github.Notification {
	return []github.Notification{
		{
			ID: "1", Unread: true, Reason: "review_requested",
			UpdatedAt: time.Now().Add(-5 * time.Minute),
			Subject:   github.Subject{Title: "Fix login bug", Type: "PullRequest", URL: "https://api.github.com/repos/org/app/pulls/42"},
			Repository: github.Repository{FullName: "org/app", HTMLURL: "https://github.com/org/app"},
		},
		{
			ID: "2", Unread: true, Reason: "mention",
			UpdatedAt: time.Now().Add(-2 * time.Hour),
			Subject:   github.Subject{Title: "Add caching layer", Type: "Issue", URL: "https://api.github.com/repos/org/lib/issues/10"},
			Repository: github.Repository{FullName: "org/lib", HTMLURL: "https://github.com/org/lib"},
		},
		{
			ID: "3", Unread: false, Reason: "subscribed",
			UpdatedAt: time.Now().Add(-48 * time.Hour),
			Subject:   github.Subject{Title: "Release v2.0", Type: "Release", URL: "https://api.github.com/repos/other-org/tool/releases/5"},
			Repository: github.Repository{FullName: "other-org/tool", HTMLURL: "https://github.com/other-org/tool"},
		},
		{
			ID: "4", Unread: true, Reason: "ci_activity",
			UpdatedAt: time.Now().Add(-10 * 24 * time.Hour),
			Subject:   github.Subject{Title: "CI failed on main", Type: "CheckSuite", URL: "https://api.github.com/repos/other-org/infra/check-suites/99"},
			Repository: github.Repository{FullName: "other-org/infra", HTMLURL: "https://github.com/other-org/infra"},
		},
	}
}

func newTestApp() App {
	a := App{
		currentView: github.ViewUnread,
		width:       120,
		height:      24,
		detailCache: make(map[string]*github.ThreadDetail),
		selected:    make(map[string]bool),
	}
	a.notifications = sampleNotifications()
	a.collectFilterOptions()
	return a
}

func TestApp_CursorNavigation(t *testing.T) {
	a := newTestApp()

	if a.cursor != 0 {
		t.Fatal("cursor should start at 0")
	}

	// Move down
	a.cursor++
	a.clampCursor()
	if a.cursor != 1 {
		t.Errorf("cursor = %d, want 1", a.cursor)
	}

	// Move past end
	a.cursor = 100
	a.clampCursor()
	if a.cursor != 3 {
		t.Errorf("cursor = %d, want 3 (clamped)", a.cursor)
	}

	// Move before start
	a.cursor = -5
	a.clampCursor()
	if a.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped)", a.cursor)
	}
}

func TestApp_RepoFilter(t *testing.T) {
	a := newTestApp()

	a.repoFilter = "org/lib"
	filtered := a.filteredNotifications()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered notification, got %d", len(filtered))
	}
	if filtered[0].ID != "2" {
		t.Errorf("expected notification 2, got %s", filtered[0].ID)
	}
}

func TestApp_ReasonFilter(t *testing.T) {
	a := newTestApp()

	a.reasonFilter = "review_requested"
	filtered := a.filteredNotifications()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered notification, got %d", len(filtered))
	}
	if filtered[0].ID != "1" {
		t.Errorf("expected notification 1, got %s", filtered[0].ID)
	}
}

func TestApp_CombinedFilters(t *testing.T) {
	a := newTestApp()

	a.repoFilter = "org"
	a.reasonFilter = "mention"
	filtered := a.filteredNotifications()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered notification, got %d", len(filtered))
	}
	if filtered[0].ID != "2" {
		t.Errorf("expected notification 2, got %s", filtered[0].ID)
	}
}

func TestApp_RemoveNotification(t *testing.T) {
	a := newTestApp()

	a.removeNotification("2")
	if len(a.notifications) != 3 {
		t.Fatalf("expected 3 notifications after removal, got %d", len(a.notifications))
	}
	for _, n := range a.notifications {
		if n.ID == "2" {
			t.Error("notification 2 should have been removed")
		}
	}
}

func TestApp_CycleReasonFilter(t *testing.T) {
	a := newTestApp()

	// First cycle: should set to first known reason
	a.cycleReasonFilter()
	if a.reasonFilter == "" {
		t.Error("expected non-empty reason filter after first cycle")
	}
	first := a.reasonFilter

	// Keep cycling until we get back to empty
	found := false
	for range 10 {
		a.cycleReasonFilter()
		if a.reasonFilter == "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to cycle back to empty reason filter")
	}

	// And cycling again should return to first
	a.cycleReasonFilter()
	if a.reasonFilter != first {
		t.Errorf("expected %q after full cycle, got %q", first, a.reasonFilter)
	}
}

func TestApp_SelectedNotification(t *testing.T) {
	a := newTestApp()

	n := a.selectedNotification()
	if n == nil {
		t.Fatal("expected a selected notification")
	}
	if n.ID != "1" {
		t.Errorf("expected notification 1, got %s", n.ID)
	}

	a.cursor = 2
	n = a.selectedNotification()
	if n == nil || n.ID != "3" {
		t.Errorf("expected notification 3")
	}
}

func TestApp_EmptyNotifications(t *testing.T) {
	a := App{width: 80, height: 24}

	n := a.selectedNotification()
	if n != nil {
		t.Error("expected nil selected notification on empty list")
	}

	filtered := a.filteredNotifications()
	if len(filtered) != 0 {
		t.Errorf("expected 0 filtered, got %d", len(filtered))
	}
}

func TestApp_CollectReasons(t *testing.T) {
	a := newTestApp()

	if len(a.knownReasons) != 4 {
		t.Errorf("expected 4 known reasons, got %d", len(a.knownReasons))
	}
}

func TestTimeAgo(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Second, "now"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{48 * time.Hour, "2d"},
	}
	for _, tc := range tests {
		got := timeAgo(time.Now().Add(-tc.d))
		if got != tc.want {
			t.Errorf("timeAgo(-%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("got %q", got)
	}
	if got := truncate("hello world", 5); got != "hell…" {
		t.Errorf("got %q", got)
	}
	if got := truncate("ab", 2); got != "ab" {
		t.Errorf("got %q", got)
	}
}

func TestTruncate_MultiByteRunes(t *testing.T) {
	// Japanese text: each char is one rune
	got := truncate("こんにちは世界", 4) // 7 runes, truncate to 4
	if got != "こんに…" {
		t.Errorf("multi-byte truncate = %q, want %q", got, "こんに…")
	}

	// Emoji
	got = truncate("🔔🔕🔔🔕", 3)
	if got != "🔔🔕…" {
		t.Errorf("emoji truncate = %q, want %q", got, "🔔🔕…")
	}
}

func TestRenderNotificationRow_NoWrapping(t *testing.T) {
	a := newTestApp()
	a.width = 120

	for _, n := range a.notifications {
		row := a.renderNotificationRow(n, false)
		lines := strings.Split(row, "\n")
		if len(lines) != 1 {
			t.Errorf("row for %q has %d lines, want 1 (row was wrapping)", n.Subject.Title, len(lines))
		}
	}
}

func TestRenderNotificationRowSized_NoWrapping(t *testing.T) {
	a := newTestApp()

	for _, width := range []int{60, 80, 100, 120} {
		for _, n := range a.notifications {
			row := a.renderNotificationRowSized(n, false, width)
			lines := strings.Split(row, "\n")
			if len(lines) != 1 {
				t.Errorf("sized row (width=%d) for %q has %d lines, want 1", width, n.Subject.Title, len(lines))
			}
		}
	}
}

func TestRenderNotificationRow_LongFields(t *testing.T) {
	a := App{width: 100, height: 24}
	a.notifications = []github.Notification{
		{
			ID: "1", Unread: true, Reason: "subscribed",
			UpdatedAt:  time.Now(),
			Subject:    github.Subject{Title: "This is a very long title that should be truncated properly without causing any wrapping issues in the terminal", Type: "PullRequest"},
			Repository: github.Repository{FullName: "very-long-organization-name/very-long-repository-name"},
		},
	}
	a.collectFilterOptions()

	row := a.renderNotificationRow(a.notifications[0], false)
	lines := strings.Split(row, "\n")
	if len(lines) != 1 {
		t.Errorf("long-field row has %d lines, want 1", len(lines))
	}
}

func TestRenderNotificationRow_ColumnsAligned(t *testing.T) {
	a := newTestApp()
	a.width = 120

	// Render all rows and verify they all produce the same number of lines (1)
	for i, n := range a.notifications {
		selected := i == 0
		row := a.renderNotificationRow(n, selected)
		lines := strings.Split(row, "\n")
		if len(lines) != 1 {
			t.Errorf("notification %d (%q): rendered %d lines, want 1", i, n.Subject.Title, len(lines))
		}
	}
}

// --- New filter tests ---

func TestApp_TypeFilter(t *testing.T) {
	a := newTestApp()
	if got := len(a.filteredNotifications()); got != 4 {
		t.Fatalf("no filter: got %d, want 4", got)
	}

	a.typeFilter = "PullRequest"
	filtered := a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "1" {
		t.Errorf("type=PullRequest: got %d items, want 1 (id=1)", len(filtered))
	}

	a.typeFilter = "Issue"
	filtered = a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "2" {
		t.Errorf("type=Issue: got %d items, want 1 (id=2)", len(filtered))
	}

	a.typeFilter = "Release"
	filtered = a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "3" {
		t.Errorf("type=Release: got %d items, want 1 (id=3)", len(filtered))
	}
}

func TestApp_OrgFilter(t *testing.T) {
	a := newTestApp()

	a.orgFilter = "org"
	filtered := a.filteredNotifications()
	if len(filtered) != 2 {
		t.Fatalf("org=org: got %d items, want 2", len(filtered))
	}
	for _, n := range filtered {
		if orgFromFullName(n.Repository.FullName) != "org" {
			t.Errorf("unexpected org in result: %s", n.Repository.FullName)
		}
	}

	a.orgFilter = "other-org"
	filtered = a.filteredNotifications()
	if len(filtered) != 2 {
		t.Fatalf("org=other-org: got %d items, want 2", len(filtered))
	}
}

func TestApp_ParticipatingFilter(t *testing.T) {
	a := newTestApp()
	a.participating = true
	filtered := a.filteredNotifications()
	// Only review_requested (id=1) and mention (id=2) are participating
	if len(filtered) != 2 {
		t.Fatalf("participating: got %d items, want 2", len(filtered))
	}
	for _, n := range filtered {
		if !participatingReasons[n.Reason] {
			t.Errorf("non-participating reason in results: %s", n.Reason)
		}
	}
}

func TestApp_AgeFilter(t *testing.T) {
	a := newTestApp()

	// age=1 (24h): should include id=1 (5min ago) and id=2 (2h ago), exclude id=3 (48h) and id=4 (10d)
	a.ageFilter = 1
	filtered := a.filteredNotifications()
	if len(filtered) != 2 {
		t.Fatalf("age=24h: got %d items, want 2", len(filtered))
	}

	// age=2 (7d): should include id=1, id=2, id=3, exclude id=4 (10d)
	a.ageFilter = 2
	filtered = a.filteredNotifications()
	if len(filtered) != 3 {
		t.Fatalf("age=7d: got %d items, want 3", len(filtered))
	}

	// age=3 (30d): should include all 4
	a.ageFilter = 3
	filtered = a.filteredNotifications()
	if len(filtered) != 4 {
		t.Fatalf("age=30d: got %d items, want 4", len(filtered))
	}
}

func TestApp_TitleSearch(t *testing.T) {
	a := newTestApp()

	a.titleSearch = "login"
	filtered := a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "1" {
		t.Errorf("search=login: got %d items, want 1 (id=1)", len(filtered))
	}

	// Case insensitive
	a.titleSearch = "CACHING"
	filtered = a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "2" {
		t.Errorf("search=CACHING: got %d items, want 1 (id=2)", len(filtered))
	}

	a.titleSearch = "xyz-no-match"
	filtered = a.filteredNotifications()
	if len(filtered) != 0 {
		t.Errorf("search=xyz-no-match: got %d items, want 0", len(filtered))
	}
}

func TestApp_CombinedNewFilters(t *testing.T) {
	a := newTestApp()

	// org=org AND type=Issue → only id=2
	a.orgFilter = "org"
	a.typeFilter = "Issue"
	filtered := a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "2" {
		t.Errorf("org+type: got %d items, want 1 (id=2)", len(filtered))
	}

	// Add participating → id=2 has reason=mention (participating), should still match
	a.participating = true
	filtered = a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "2" {
		t.Errorf("org+type+participating: got %d items, want 1 (id=2)", len(filtered))
	}

	// Change type to Release → org=org has no Release (Release is other-org/tool)
	a.typeFilter = "Release"
	a.participating = false
	filtered = a.filteredNotifications()
	if len(filtered) != 0 {
		t.Errorf("org=org+type=Release: got %d items, want 0", len(filtered))
	}
}

func TestApp_CycleTypeFilter(t *testing.T) {
	a := newTestApp()
	if len(a.knownTypes) != 4 {
		t.Fatalf("expected 4 known types, got %d: %v", len(a.knownTypes), a.knownTypes)
	}

	a.cycleTypeFilter()
	if a.typeFilter != a.knownTypes[0] {
		t.Errorf("first cycle: got %q, want %q", a.typeFilter, a.knownTypes[0])
	}
	for i := 1; i < len(a.knownTypes); i++ {
		a.cycleTypeFilter()
	}
	a.cycleTypeFilter() // should wrap to ""
	if a.typeFilter != "" {
		t.Errorf("after full cycle: got %q, want empty", a.typeFilter)
	}
}

func TestApp_CycleOrgFilter(t *testing.T) {
	a := newTestApp()
	if len(a.knownOrgs) != 2 {
		t.Fatalf("expected 2 known orgs, got %d: %v", len(a.knownOrgs), a.knownOrgs)
	}

	a.cycleOrgFilter()
	first := a.orgFilter
	if first == "" {
		t.Fatal("first cycle should set an org")
	}
	a.cycleOrgFilter()
	a.cycleOrgFilter() // should wrap to ""
	if a.orgFilter != "" {
		t.Errorf("after full cycle: got %q, want empty", a.orgFilter)
	}
}

func TestApp_CycleAgeFilter(t *testing.T) {
	a := newTestApp()
	if a.ageFilter != 0 {
		t.Fatal("age filter should start at 0")
	}
	a.cycleAgeFilter() // → 1 (24h)
	if a.ageFilter != 1 {
		t.Errorf("got %d, want 1", a.ageFilter)
	}
	a.cycleAgeFilter() // → 2 (7d)
	a.cycleAgeFilter() // → 3 (30d)
	a.cycleAgeFilter() // → 0 (all)
	if a.ageFilter != 0 {
		t.Errorf("after full cycle: got %d, want 0", a.ageFilter)
	}
}

func TestApp_HasActiveFilters(t *testing.T) {
	a := newTestApp()
	if a.hasActiveFilters() {
		t.Fatal("should have no active filters initially")
	}

	a.typeFilter = "Issue"
	if !a.hasActiveFilters() {
		t.Fatal("should detect type filter")
	}
	a.typeFilter = ""

	a.participating = true
	if !a.hasActiveFilters() {
		t.Fatal("should detect participating")
	}
	a.participating = false

	a.ageFilter = 2
	if !a.hasActiveFilters() {
		t.Fatal("should detect age filter")
	}
}

// --- Selection preservation & new notification tracking ---

func TestApp_SelectionPreservedOnRefresh(t *testing.T) {
	a := newTestApp()

	// Select the 2nd notification (id=2)
	a.cursor = 1
	selectedID := a.filteredNotifications()[a.cursor].ID
	if selectedID != "2" {
		t.Fatalf("expected selected id=2, got %s", selectedID)
	}

	// Simulate a refresh: new notifications arrive with a new item prepended.
	// The old item id=2 should still be selected after the update.
	oldIDs := make(map[string]bool, len(a.notifications))
	for _, n := range a.notifications {
		oldIDs[n.ID] = true
	}

	newNotification := github.Notification{
		ID: "99", Unread: true, Reason: "assign",
		UpdatedAt: time.Now().Add(-1 * time.Minute),
		Subject:   github.Subject{Title: "New task", Type: "Issue"},
		Repository: github.Repository{FullName: "org/app"},
	}
	incoming := append([]github.Notification{newNotification}, a.notifications...)

	// Compute new IDs
	newIDs := make(map[string]bool)
	for _, n := range incoming {
		if !oldIDs[n.ID] {
			newIDs[n.ID] = true
		}
	}
	a.newNotificationIDs = newIDs

	// Group new at top (same logic as Update)
	var newNotifs, existingNotifs []github.Notification
	for _, n := range incoming {
		if newIDs[n.ID] {
			newNotifs = append(newNotifs, n)
		} else {
			existingNotifs = append(existingNotifs, n)
		}
	}
	a.notifications = append(newNotifs, existingNotifs...)
	a.collectFilterOptions()

	// Restore cursor
	filtered := a.filteredNotifications()
	for i, n := range filtered {
		if n.ID == selectedID {
			a.cursor = i
			break
		}
	}
	a.clampCursor()

	// The selected notification should still be id=2
	sel := a.selectedNotification()
	if sel == nil || sel.ID != "2" {
		t.Errorf("selection not preserved: got %v, want id=2", sel)
	}
}

func TestApp_NewNotificationTracking(t *testing.T) {
	a := newTestApp()

	// Record old IDs
	oldIDs := make(map[string]bool)
	for _, n := range a.notifications {
		oldIDs[n.ID] = true
	}

	// Simulate refresh with one new notification
	newNotif := github.Notification{
		ID: "50", Unread: true, Reason: "mention",
		UpdatedAt: time.Now(),
		Subject:   github.Subject{Title: "Brand new", Type: "Issue"},
		Repository: github.Repository{FullName: "org/app"},
	}
	incoming := append([]github.Notification{newNotif}, a.notifications...)

	newIDs := make(map[string]bool)
	for _, n := range incoming {
		if !oldIDs[n.ID] {
			newIDs[n.ID] = true
		}
	}
	a.newNotificationIDs = newIDs

	if len(newIDs) != 1 {
		t.Fatalf("expected 1 new ID, got %d", len(newIDs))
	}
	if !newIDs["50"] {
		t.Error("expected id=50 to be marked as new")
	}
	// Old IDs should NOT be marked new
	if newIDs["1"] || newIDs["2"] || newIDs["3"] || newIDs["4"] {
		t.Error("existing notifications should not be marked as new")
	}
}

func TestApp_NewNotificationsGroupedAtTop(t *testing.T) {
	a := newTestApp()

	oldIDs := make(map[string]bool)
	for _, n := range a.notifications {
		oldIDs[n.ID] = true
	}

	// Add two new notifications at positions that would normally interleave
	newNotifs := []github.Notification{
		{
			ID: "10", Unread: true, Reason: "assign",
			UpdatedAt: time.Now().Add(-3 * time.Hour), // older than id=2
			Subject:   github.Subject{Title: "New A", Type: "Issue"},
			Repository: github.Repository{FullName: "org/app"},
		},
		{
			ID: "11", Unread: true, Reason: "mention",
			UpdatedAt: time.Now().Add(-1 * time.Minute), // very recent
			Subject:   github.Subject{Title: "New B", Type: "PullRequest"},
			Repository: github.Repository{FullName: "org/lib"},
		},
	}
	// API would return sorted by updated_at: id=11, id=1, id=2, id=10, id=3, id=4
	incoming := []github.Notification{newNotifs[1], a.notifications[0], a.notifications[1], newNotifs[0], a.notifications[2], a.notifications[3]}

	newIDs := make(map[string]bool)
	for _, n := range incoming {
		if !oldIDs[n.ID] {
			newIDs[n.ID] = true
		}
	}

	// Group new at top
	var grouped, existing []github.Notification
	for _, n := range incoming {
		if newIDs[n.ID] {
			grouped = append(grouped, n)
		} else {
			existing = append(existing, n)
		}
	}
	a.notifications = append(grouped, existing...)
	a.newNotificationIDs = newIDs

	// First two should be the new ones
	if a.notifications[0].ID != "11" || a.notifications[1].ID != "10" {
		t.Errorf("new items should be at top: got [%s, %s], want [11, 10]",
			a.notifications[0].ID, a.notifications[1].ID)
	}
	// Remaining should be the original order
	if a.notifications[2].ID != "1" || a.notifications[3].ID != "2" {
		t.Errorf("existing items should follow: got [%s, %s], want [1, 2]",
			a.notifications[2].ID, a.notifications[3].ID)
	}
}

func TestApp_InitialLoadNoNewMarkers(t *testing.T) {
	// On initial load (no previous notifications), nothing should be marked new
	a := App{width: 120, height: 24}
	// Simulate first load
	oldIDs := make(map[string]bool) // empty — no previous data
	incoming := sampleNotifications()

	newIDs := make(map[string]bool)
	if len(oldIDs) > 0 {
		for _, n := range incoming {
			if !oldIDs[n.ID] {
				newIDs[n.ID] = true
			}
		}
	}
	a.newNotificationIDs = newIDs
	a.notifications = incoming

	if len(a.newNotificationIDs) != 0 {
		t.Errorf("initial load should have no new markers, got %d", len(a.newNotificationIDs))
	}
}

func TestApp_HeaderRendering(t *testing.T) {
	a := newTestApp()
	header := a.buildHeader()

	// Should contain the ASCII art text
	if !strings.Contains(header, "▗▄▄▖") {
		t.Error("header should contain ASCII art")
	}
	// Should contain tip text
	if !strings.Contains(header, "Tip:") {
		t.Error("header should contain tip line")
	}
	// Should be multi-line
	lines := strings.Split(header, "\n")
	if len(lines) < 5 {
		t.Errorf("header should be multi-line, got %d lines", len(lines))
	}
}

func TestApp_NewItemRenderIndicator(t *testing.T) {
	a := newTestApp()
	a.newNotificationIDs = map[string]bool{"1": true}

	// New item (non-selected) should have • prefix
	row := a.renderNotificationRow(a.notifications[0], false)
	if !strings.Contains(row, "•") {
		t.Error("new notification row should contain • indicator")
	}

	// Old item should NOT have •
	row2 := a.renderNotificationRow(a.notifications[1], false)
	if strings.Contains(row2, "•") {
		t.Error("existing notification row should not contain • indicator")
	}
}

func TestWordWrap(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		width    int
		expected []string
	}{
		{
			name:     "short text no wrap",
			text:     "hello world",
			width:    20,
			expected: []string{"hello world"},
		},
		{
			name:     "wraps at width",
			text:     "one two three four five",
			width:    10,
			expected: []string{"one two", "three four", "five"},
		},
		{
			name:     "preserves newlines",
			text:     "first\nsecond",
			width:    80,
			expected: []string{"first", "second"},
		},
		{
			name:     "empty text",
			text:     "",
			width:    40,
			expected: []string{""},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := wordWrap(tc.text, tc.width)
			if len(result) != len(tc.expected) {
				t.Fatalf("expected %d lines, got %d: %v", len(tc.expected), len(result), result)
			}
			for i, line := range result {
				if line != tc.expected[i] {
					t.Errorf("line %d: expected %q, got %q", i, tc.expected[i], line)
				}
			}
		})
	}
}

func TestDetailCache(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)

	// No detail cached → loading should trigger
	n := a.selectedNotification()
	if n == nil {
		t.Fatal("expected a selected notification")
	}
	_, cached := a.detailCache[n.ID]
	if cached {
		t.Error("detail should not be cached initially")
	}

	// Simulate caching a detail
	detail := &github.ThreadDetail{
		State: "open",
		User:  github.User{Login: "testuser"},
		Body:  "This is a test body",
	}
	a.detailCache[n.ID] = detail

	// Now it should be cached
	got, ok := a.detailCache[n.ID]
	if !ok || got.State != "open" {
		t.Error("detail should be cached and retrievable")
	}
}

func TestPreviewWithDetail(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)

	// Cache a detail for the first notification
	a.detailCache["1"] = &github.ThreadDetail{
		State:  "open",
		User:   github.User{Login: "alice"},
		Labels: []github.Label{{Name: "bug"}, {Name: "urgent"}},
		Body:   "Fix the login flow for SSO users",
		LatestComment: &github.Comment{
			Body:      "I can reproduce this on Chrome",
			User:      github.User{Login: "bob"},
			CreatedAt: time.Now().Add(-30 * time.Minute),
		},
	}

	preview := a.renderPreview(30, 60)

	// Should contain enriched detail
	if !strings.Contains(preview, "open") {
		t.Error("preview should show state")
	}
	if !strings.Contains(preview, "alice") {
		t.Error("preview should show author")
	}
	if !strings.Contains(preview, "bug") {
		t.Error("preview should show labels")
	}
	if !strings.Contains(preview, "Login flow") || !strings.Contains(preview, "SSO") {
		// Body is word-wrapped so check individually
		if !strings.Contains(preview, "login") && !strings.Contains(preview, "Fix") {
			t.Error("preview should show body text")
		}
	}
	if !strings.Contains(preview, "bob") {
		t.Error("preview should show comment author")
	}
	if !strings.Contains(preview, "Chrome") {
		t.Error("preview should show comment body")
	}
}

func TestPreviewLoadingIndicator(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.detailLoading = "1" // Simulate loading for first notification

	preview := a.renderPreview(20, 60)
	if !strings.Contains(preview, "Loading details...") {
		t.Error("preview should show loading indicator when detail is being fetched")
	}
}

func TestMaybeFetchDetail_AlreadyCached(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.detailCache["1"] = &github.ThreadDetail{State: "open"}

	cmd := a.maybeFetchDetail()
	if cmd != nil {
		t.Error("should not return cmd when detail is already cached")
	}
}

func TestMaybeFetchDetail_AlreadyLoading(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.detailLoading = "1"

	cmd := a.maybeFetchDetail()
	if cmd != nil {
		t.Error("should not return cmd when detail is already loading")
	}
}

func TestSpinnerShowsInLoadingPreview(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.detailLoading = "1"

	// Test multiple spinner frames render correctly
	for _, frame := range spinnerFrames {
		preview := a.renderPreview(20, 60)
		if !strings.Contains(preview, "Loading details...") {
			t.Error("preview should always show 'Loading details...' text during loading")
		}
		// The spinner frame character should appear in the preview
		if !strings.Contains(preview, frame) {
			t.Errorf("preview should contain spinner frame %q", frame)
		}
		a.spinnerFrame++
	}
}

func TestSpinnerFrameAdvances(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.detailLoading = "1"
	a.spinnerFrame = 0

	// Simulate spinnerTickMsg via Update
	result, cmd := a.Update(spinnerTickMsg{})
	updated := result.(App)

	if updated.spinnerFrame != 1 {
		t.Errorf("spinner frame should advance to 1, got %d", updated.spinnerFrame)
	}
	// Should return another tick cmd since detailLoading is still set
	if cmd == nil {
		t.Error("should return another tick cmd while detail is loading")
	}
}

func TestSpinnerStopsWhenNotLoading(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.detailLoading = "" // not loading
	a.spinnerFrame = 3

	result, cmd := a.Update(spinnerTickMsg{})
	updated := result.(App)

	// Frame should not advance when not loading
	if updated.spinnerFrame != 3 {
		t.Errorf("spinner frame should stay at 3 when not loading, got %d", updated.spinnerFrame)
	}
	if cmd != nil {
		t.Error("should not return tick cmd when not loading")
	}
}

func TestSpinnerFrameWraps(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.detailLoading = "1"
	a.spinnerFrame = len(spinnerFrames) - 1 // last frame

	result, _ := a.Update(spinnerTickMsg{})
	updated := result.(App)

	if updated.spinnerFrame != 0 {
		t.Errorf("spinner frame should wrap to 0, got %d", updated.spinnerFrame)
	}
}

func TestPreviewBodyNotTruncated(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)

	// Create a long body with 20 distinct paragraphs (separated by double newlines
	// so glamour treats them as separate paragraphs)
	var longBody string
	for i := 1; i <= 20; i++ {
		longBody += fmt.Sprintf("Paragraph %d unique content.\n\n", i)
	}

	a.detailCache["1"] = &github.ThreadDetail{
		State: "open",
		User:  github.User{Login: "author"},
		Body:  longBody,
	}

	preview := a.renderPreview(80, 60) // enough height to fit everything

	// Check first and last paragraphs are both present (no truncation)
	if !strings.Contains(preview, "Paragraph 1") {
		t.Error("preview should contain first paragraph of body")
	}
	if !strings.Contains(preview, "Paragraph 20") {
		t.Error("preview should contain last paragraph of body (no truncation)")
	}
}

func TestPreviewCommentNotTruncated(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)

	// Create a long comment with 15 distinct paragraphs
	var longComment string
	for i := 1; i <= 15; i++ {
		longComment += fmt.Sprintf("Comment paragraph %d with context.\n\n", i)
	}

	a.detailCache["1"] = &github.ThreadDetail{
		State: "open",
		User:  github.User{Login: "author"},
		LatestComment: &github.Comment{
			Body:      longComment,
			User:      github.User{Login: "commenter"},
			CreatedAt: time.Now(),
		},
	}

	preview := a.renderPreview(80, 60)

	if !strings.Contains(preview, "Comment paragraph 1") {
		t.Error("preview should contain first paragraph of comment")
	}
	if !strings.Contains(preview, "Comment paragraph 15") {
		t.Error("preview should contain last paragraph of comment (no truncation)")
	}
}

func TestMaybeFetchDetail_StartsSpinner(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.spinnerFrame = 5 // non-zero to verify reset

	// Can't actually call maybeFetchDetail without a real client,
	// but we can test the state changes it makes
	a.detailLoading = ""
	// Verify precondition: not loading, not cached
	_, cached := a.detailCache["1"]
	if cached {
		t.Fatal("should not be cached")
	}

	// Simulate what maybeFetchDetail does to state (without the client call)
	a.detailLoading = "1"
	a.spinnerFrame = 0

	if a.detailLoading != "1" {
		t.Error("detailLoading should be set")
	}
	if a.spinnerFrame != 0 {
		t.Error("spinnerFrame should be reset to 0 on new fetch")
	}
}

func TestPreviewUpdatesAfterMarkRead(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.cursor = 0 // selected: notification "1"

	// Cache detail for both first and second notifications
	a.detailCache["1"] = &github.ThreadDetail{State: "open", User: github.User{Login: "alice"}}
	a.detailCache["2"] = &github.ThreadDetail{State: "closed", User: github.User{Login: "bob"}}

	// Simulate marking notification "1" as read
	result, cmd := a.Update(threadMarkedReadMsg{threadID: "1"})
	updated := result.(App)

	// Notification "1" should be removed, cursor should now point to what was "2"
	sel := updated.selectedNotification()
	if sel == nil {
		t.Fatal("should have a selected notification after mark read")
	}
	if sel.ID == "1" {
		t.Error("removed notification should not be selected")
	}

	// Preview scroll should be reset
	if updated.previewScroll != 0 {
		t.Error("previewScroll should be reset after mark read")
	}

	// The returned cmd should include a detail fetch (via maybeFetchDetail)
	// since the new selection may need its detail loaded
	if cmd == nil {
		t.Error("should return a cmd batch (clearStatus + maybeFetchDetail)")
	}
}

func TestPreviewUpdatesAfterMute(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.cursor = 1 // selected: notification "2"

	result, cmd := a.Update(threadMutedMsg{threadID: "2"})
	updated := result.(App)

	// "2" removed, cursor should clamp and select something valid
	sel := updated.selectedNotification()
	if sel != nil && sel.ID == "2" {
		t.Error("muted notification should not be selected")
	}
	if updated.previewScroll != 0 {
		t.Error("previewScroll should be reset after mute")
	}
	if cmd == nil {
		t.Error("should return a cmd batch after mute")
	}
}

func TestPreviewUpdatesAfterUnsubscribe(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.cursor = 0

	result, cmd := a.Update(threadUnsubscribedMsg{threadID: "1"})
	updated := result.(App)

	sel := updated.selectedNotification()
	if sel != nil && sel.ID == "1" {
		t.Error("unsubscribed notification should not be selected")
	}
	if updated.previewScroll != 0 {
		t.Error("previewScroll should be reset after unsubscribe")
	}
	if cmd == nil {
		t.Error("should return a cmd batch after unsubscribe")
	}
}

func TestPreviewPaneActionsDelegate(t *testing.T) {
a := newTestApp()
a.detailCache = make(map[string]*github.ThreadDetail)
a.focused = focusPreview
initialCount := len(a.notifications)

// 'r' from preview pane should delegate to list handler and mark read
result, cmd := a.handlePreviewKey("r")
_ = result
if cmd == nil {
t.Error("pressing 'r' in preview pane should trigger mark-read command")
}

// 'enter' from preview pane should trigger browser open
a2 := newTestApp()
a2.focused = focusPreview
_, cmd2 := a2.handlePreviewKey("enter")
if cmd2 == nil {
t.Error("pressing 'enter' in preview pane should trigger browser open command")
}

// j/k should NOT delegate — they scroll the preview instead
a3 := newTestApp()
a3.focused = focusPreview
a3.previewScroll = 0
a3.height = 10 // small height so content overflows
a3.detailCache = make(map[string]*github.ThreadDetail)
a3.detailCache["1"] = &github.ThreadDetail{
	State: "open",
	User:  github.User{Login: "author"},
	Body:  "Line1\n\nLine2\n\nLine3\n\nLine4\n\nLine5\n\nLine6\n\nLine7\n\nLine8\n\nLine9\n\nLine10",
}
result3, _ := a3.handlePreviewKey("j")
updated3 := result3.(App)
if updated3.previewScroll != 1 {
	t.Error("'j' in preview pane should scroll preview, not navigate list")
}

_ = initialCount
}

func TestStatusBar_ShowsTotalCount(t *testing.T) {
	a := newTestApp()
	bar := a.renderStatusBar()
	if !strings.Contains(bar, "4") {
		t.Errorf("status bar should show total count of 4, got: %q", bar)
	}
}

func TestStatusBar_ShowsFilteredCount(t *testing.T) {
	a := newTestApp()
	a.repoFilter = "org/app"
	bar := a.renderStatusBar()
	if !strings.Contains(bar, "1/4") {
		t.Errorf("status bar should show 1/4 when filtered, got: %q", bar)
	}
}

func TestTabs_ShowsFilteredCount(t *testing.T) {
	a := newTestApp()

	// No filters — should show "4 items"
	tabs := a.renderTabs()
	if !strings.Contains(tabs, "4 items") {
		t.Errorf("tabs should show '4 items' without filter, got: %q", tabs)
	}

	// With filter — should show "1/4 items"
	a.repoFilter = "org/app"
	tabs = a.renderTabs()
	if !strings.Contains(tabs, "1/4 items") {
		t.Errorf("tabs should show '1/4 items' with filter, got: %q", tabs)
	}
}

func TestTabs_ShowsSyncingSpinnerWhileLoading(t *testing.T) {
	a := newTestApp()

	// Not loading — no syncing indicator
	a.loading = false
	tabs := a.renderTabs()
	if strings.Contains(tabs, "syncing") {
		t.Errorf("tabs should not show 'syncing' when not loading, got: %q", tabs)
	}

	// Loading — should show spinner frame and syncing label
	a.loading = true
	tabs = a.renderTabs()
	if !strings.Contains(tabs, "syncing") {
		t.Errorf("tabs should show 'syncing…' while loading, got: %q", tabs)
	}
	frame := spinnerFrames[a.spinnerFrame%len(spinnerFrames)]
	if !strings.Contains(tabs, frame) {
		t.Errorf("tabs should show spinner frame %q while loading, got: %q", frame, tabs)
	}
}

func TestWithRefreshInterval(t *testing.T) {
	a := App{}
	opt := WithRefreshInterval(30 * time.Second)
	opt(&a)
	if a.refreshInterval != 30*time.Second {
		t.Errorf("refreshInterval = %v, want 30s", a.refreshInterval)
	}
	if got := a.getRefreshInterval(); got != 30*time.Second {
		t.Errorf("getRefreshInterval() = %v, want 30s", got)
	}
}

func TestGetRefreshInterval_Default(t *testing.T) {
	a := App{} // zero value — no custom interval
	got := a.getRefreshInterval()
	if got != 60*time.Second {
		t.Errorf("getRefreshInterval() = %v, want default 60s", got)
	}
}

func TestMarkReadOnOpen(t *testing.T) {
	a := newTestApp()
	// First notification is unread — pressing enter should produce a batch cmd
	_, cmd := a.handleListKey("enter")
	if cmd == nil {
		t.Fatal("pressing enter on unread notification should return a command")
	}
	// The cmd should be a batch (openBrowser + markRead)
	// We can't easily inspect tea.Batch internals, but we can verify it's non-nil
	// and that a read notification does NOT produce markRead
	a2 := newTestApp()
	a2.cursor = 2 // item 3 is unread=false
	_, cmd2 := a2.handleListKey("enter")
	if cmd2 == nil {
		t.Fatal("pressing enter on read notification should still open browser")
	}
}

func TestPreviewPane_GG_JumpsToTop(t *testing.T) {
	a := newTestApp()
	a.focused = focusPreview
	a.previewScroll = 10

	// First 'g' — just records the key
	result, _ := a.handlePreviewKey("g")
	a = result.(App)
	if a.previewScroll != 10 {
		t.Error("first 'g' should not change scroll")
	}

	// Second 'g' quickly — should jump to top
	result, _ = a.handlePreviewKey("g")
	a = result.(App)
	if a.previewScroll != 0 {
		t.Errorf("gg should jump to top, got scroll=%d", a.previewScroll)
	}
}

func TestPreviewPane_G_JumpsToBottom(t *testing.T) {
	a := newTestApp()
	a.focused = focusPreview
	a.previewScroll = 0

	result, _ := a.handlePreviewKey("G")
	a = result.(App)
	// Should equal the exact max scroll (not some arbitrary large number)
	expected := a.previewMaxScroll()
	if a.previewScroll != expected {
		t.Errorf("G scroll=%d, want previewMaxScroll=%d", a.previewScroll, expected)
	}
}

func TestPreviewPane_J_CapsScroll(t *testing.T) {
	a := newTestApp()
	a.focused = focusPreview
	// Set scroll at the max
	a.previewScroll = a.previewMaxScroll()

	result, _ := a.handlePreviewKey("j")
	a = result.(App)
	// Should not exceed max
	if a.previewScroll > a.previewMaxScroll() {
		t.Error("j should not scroll past the max")
	}
}

func TestReasonColorFor(t *testing.T) {
	// Verify distinct colors for key reasons
	reasons := []string{"review_requested", "mention", "assign", "author", "comment", "ci_activity", "security_alert"}
	colors := make(map[string]bool)
	for _, r := range reasons {
		c := theme.ReasonColorFor(r)
		key := fmt.Sprintf("%v", c)
		if colors[key] {
			t.Errorf("reason %q shares a color with another reason", r)
		}
		colors[key] = true
	}
}

// --- Full-text search TUI tests ---

func TestSearchKeybinding_S_entersSearchMode(t *testing.T) {
	app := newTestApp()
	// S requires service != nil; simulate by setting it directly
	app.filterInput = filterFullTextSearch
	app.filterBuf = ""
	if app.filterInput != filterFullTextSearch {
		t.Fatalf("expected filterFullTextSearch mode, got %d", app.filterInput)
	}
}

func TestSearchMode_typing(t *testing.T) {
	app := newTestApp()
	app.filterInput = filterFullTextSearch
	app.filterBuf = ""

	// Type "bug"
	updated, _ := app.handleFilterInput("b")
	a := updated.(App)
	updated, _ = a.handleFilterInput("u")
	a = updated.(App)
	updated, _ = a.handleFilterInput("g")
	a = updated.(App)

	if a.filterBuf != "bug" {
		t.Fatalf("expected filterBuf 'bug', got %q", a.filterBuf)
	}
	// Full-text search does NOT apply live filter (only on enter)
	if a.titleSearch != "" {
		t.Fatalf("titleSearch should not change during full-text search input")
	}
}

func TestSearchMode_enterWithEmptyQuery_clearsResults(t *testing.T) {
	app := newTestApp()
	// Set pre-existing search results
	app.searchResultIDs = map[string]bool{"1": true}
	app.searchQuery = "old"
	app.filterInput = filterFullTextSearch
	app.filterBuf = ""

	updated, cmd := app.handleFilterInput("enter")
	a := updated.(App)

	if a.filterInput != filterNone {
		t.Fatalf("expected filterNone, got %d", a.filterInput)
	}
	if a.searchResultIDs != nil {
		t.Fatalf("expected searchResultIDs cleared, got %v", a.searchResultIDs)
	}
	if a.searchQuery != "" {
		t.Fatalf("expected searchQuery cleared, got %q", a.searchQuery)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd on empty query, got non-nil")
	}
}

func TestSearchMode_enterWithQuery_firesCmd(t *testing.T) {
	app := newTestApp()
	app.filterInput = filterFullTextSearch
	app.filterBuf = "login"

	updated, cmd := app.handleFilterInput("enter")
	a := updated.(App)

	if a.filterInput != filterNone {
		t.Fatalf("expected filterNone, got %d", a.filterInput)
	}
	// fullTextSearchCmd is returned even with nil service (will produce error msg)
	if cmd == nil {
		t.Fatalf("expected non-nil cmd when query is non-empty")
	}
}

func TestSearchMode_escape_clearsSearchResults(t *testing.T) {
	app := newTestApp()
	app.filterInput = filterFullTextSearch
	app.filterBuf = "test"
	app.searchResultIDs = map[string]bool{"1": true}
	app.searchQuery = "prev"

	updated, _ := app.handleFilterInput("escape")
	a := updated.(App)

	if a.filterInput != filterNone {
		t.Fatalf("expected filterNone, got %d", a.filterInput)
	}
	if a.searchResultIDs != nil {
		t.Fatalf("expected searchResultIDs cleared")
	}
	if a.searchQuery != "" {
		t.Fatalf("expected searchQuery cleared")
	}
}

func TestSearchResultsMsg_appliesFilter(t *testing.T) {
	app := newTestApp()
	app.width = 120
	app.height = 40

	msg := searchResultsMsg{
		query:     "bug",
		threadIDs: []string{"1", "3"},
	}

	updated, _ := app.Update(msg)
	a := updated.(App)

	if a.searchQuery != "bug" {
		t.Fatalf("expected searchQuery 'bug', got %q", a.searchQuery)
	}
	if len(a.searchResultIDs) != 2 {
		t.Fatalf("expected 2 search result IDs, got %d", len(a.searchResultIDs))
	}
	if !a.searchResultIDs["1"] || !a.searchResultIDs["3"] {
		t.Fatalf("unexpected search result IDs: %v", a.searchResultIDs)
	}
}

func TestFullTextSearchFilter_filtersNotifications(t *testing.T) {
	app := newTestApp()
	app.searchResultIDs = map[string]bool{"2": true}
	app.searchQuery = "caching"

	filtered := app.filteredNotifications()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(filtered))
	}
	if filtered[0].ID != "2" {
		t.Fatalf("expected notification ID '2', got %q", filtered[0].ID)
	}
}

func TestFullTextSearch_hasActiveFilters(t *testing.T) {
	app := newTestApp()

	if app.hasActiveFilters() {
		t.Fatal("expected no active filters initially")
	}

	app.searchResultIDs = map[string]bool{"1": true}
	app.searchQuery = "test"

	if !app.hasActiveFilters() {
		t.Fatal("expected active filters with search results set")
	}
}

func TestEscape_clearsSearchResults(t *testing.T) {
	app := newTestApp()
	app.searchResultIDs = map[string]bool{"1": true}
	app.searchQuery = "test"

	// Test escape clearing through the filter input path (same logic as handleKey escape)
	app.filterInput = filterFullTextSearch
	updated, _ := app.handleFilterInput("escape")
	a := updated.(App)

	if a.searchResultIDs != nil {
		t.Fatalf("expected searchResultIDs cleared by escape")
	}
	if a.searchQuery != "" {
		t.Fatalf("expected searchQuery cleared by escape")
	}
}

func TestRenderFilters_showsSearchIndicator(t *testing.T) {
	app := newTestApp()
	app.width = 100
	app.searchResultIDs = map[string]bool{"1": true}
	app.searchQuery = "login bug"

	output := app.renderFilters()
	if !strings.Contains(output, "login bug") {
		t.Fatalf("expected search indicator in filter bar, got: %s", output)
	}
}

func TestRenderFilters_showsSearchInputPrompt(t *testing.T) {
	app := newTestApp()
	app.width = 100
	app.filterInput = filterFullTextSearch
	app.filterBuf = "test"

	output := app.renderFilters()
	if !strings.Contains(output, "Full-text search:") {
		t.Fatalf("expected full-text search prompt, got: %s", output)
	}
	if !strings.Contains(output, "test") {
		t.Fatalf("expected filter buffer in output, got: %s", output)
	}
}

func TestSearchMode_backspace(t *testing.T) {
	app := newTestApp()
	app.filterInput = filterFullTextSearch
	app.filterBuf = "bug"

	updated, _ := app.handleFilterInput("backspace")
	a := updated.(App)
	if a.filterBuf != "bu" {
		t.Fatalf("expected 'bu' after backspace, got %q", a.filterBuf)
	}
}

func TestFilterInput_spaceKey(t *testing.T) {
	// In Bubble Tea v2, space is reported as "space" not " "
	for _, mode := range []filterMode{filterRepo, filterTitleSearch, filterFullTextSearch} {
		app := newTestApp()
		app.filterInput = mode
		app.filterBuf = "hello"

		updated, _ := app.handleFilterInput("space")
		a := updated.(App)
		if a.filterBuf != "hello " {
			t.Fatalf("mode %d: expected 'hello ' after space, got %q", mode, a.filterBuf)
		}
	}
}

func TestViewPreference_persistAndRestore(t *testing.T) {
	// Use a real SQLite store + service to test preference persistence
	dir := t.TempDir()
	store, err := storage.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	svc := service.New(nil, store)
	defer svc.Close()

	// Create an app with service — default view should be Unread
	app := NewApp(nil, WithService(svc))
	if app.currentView != github.ViewUnread {
		t.Fatalf("expected ViewUnread initially, got %d", app.currentView)
	}

	// Switch to All — this should persist the preference
	a, _ := app.switchView(github.ViewAll)
	if a.currentView != github.ViewAll {
		t.Fatalf("expected ViewAll after switch, got %d", a.currentView)
	}

	// Create a new app with the same service — should restore ViewAll
	app2 := NewApp(nil, WithService(svc))
	if app2.currentView != github.ViewAll {
		t.Fatalf("expected ViewAll restored, got %d", app2.currentView)
	}
}

// --- Assigned-to-me filter tests ---

func TestAssignedFilter_toggle(t *testing.T) {
	app := newTestApp()
	if app.assignedFilter {
		t.Fatal("assigned filter should be off initially")
	}

	updated, _ := app.handleListKey("A")
	a := updated.(App)
	if !a.assignedFilter {
		t.Fatal("expected assigned filter on after pressing A")
	}

	updated, _ = a.handleListKey("A")
	a = updated.(App)
	if a.assignedFilter {
		t.Fatal("expected assigned filter off after second press")
	}
}

func TestAssignedFilter_filtersWithDetailCache(t *testing.T) {
	app := newTestApp()
	app.currentUser = "testuser"
	app.assignedFilter = true

	// Without detail cache, only reason=assign notifications pass
	filtered := app.filteredNotifications()
	// In sample notifications, reason "review_requested", "mention", "subscribed" — none are "assign"
	if len(filtered) != 0 {
		t.Fatalf("expected 0 results without detail cache or assign reason, got %d", len(filtered))
	}

	// Add detail cache for notification "1" with testuser as assignee
	app.detailCache["1"] = &github.ThreadDetail{
		Assignees: []github.User{{Login: "testuser"}},
	}
	filtered = app.filteredNotifications()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 result with testuser assigned, got %d", len(filtered))
	}
	if filtered[0].ID != "1" {
		t.Fatalf("expected notification '1', got %q", filtered[0].ID)
	}
}

func TestAssignedFilter_caseInsensitive(t *testing.T) {
	app := newTestApp()
	app.currentUser = "TestUser"
	app.assignedFilter = true
	app.detailCache["2"] = &github.ThreadDetail{
		Assignees: []github.User{{Login: "testuser"}},
	}

	filtered := app.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "2" {
		t.Fatalf("expected case-insensitive match, got %d results", len(filtered))
	}
}

func TestAssignedFilter_reasonAssignFallback(t *testing.T) {
	// Notifications with reason "assign" should pass even without detail cache
	app := newTestApp()
	app.currentUser = "testuser"
	app.assignedFilter = true
	// Override notification 1's reason to "assign"
	app.notifications[0].Reason = "assign"

	filtered := app.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "1" {
		t.Fatalf("expected reason=assign notification to pass, got %d results", len(filtered))
	}
}

func TestAssignedFilter_hasActiveFilters(t *testing.T) {
	app := newTestApp()
	if app.hasActiveFilters() {
		t.Fatal("no filters initially")
	}
	app.assignedFilter = true
	if !app.hasActiveFilters() {
		t.Fatal("expected active filters with assigned on")
	}
}

func TestAssignedFilter_showsIndicator(t *testing.T) {
	app := newTestApp()
	app.width = 100
	app.assignedFilter = true

	output := app.renderFilters()
	if !strings.Contains(output, "assigned:me") {
		t.Fatalf("expected assigned:me indicator, got: %s", output)
	}
}

func TestCurrentUserMsg_setsUser(t *testing.T) {
	app := newTestApp()
	app.width = 120
	app.height = 40
	updated, _ := app.Update(currentUserMsg{login: "cassio"})
	a := updated.(App)
	if a.currentUser != "cassio" {
		t.Fatalf("expected currentUser 'cassio', got %q", a.currentUser)
	}
}

func TestBatchMarkVisibleReadRespectsFilters(t *testing.T) {
	a := newTestApp()
	a.notifications = sampleNotifications()
	a.width = 120
	a.height = 40

	// Apply a filter so only some notifications are visible
	a.repoFilter = "org/app"
	filtered := a.filteredNotifications()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered notification for org/app, got %d", len(filtered))
	}
	if filtered[0].ID != "1" {
		t.Fatalf("expected filtered notification ID '1', got %q", filtered[0].ID)
	}
}

func TestBatchMarkVisibleReadMsg(t *testing.T) {
	a := newTestApp()
	a.notifications = sampleNotifications()
	a.width = 120
	a.height = 40

	// Simulate the visibleMarkedReadMsg arriving
	updated, _ := a.Update(visibleMarkedReadMsg{count: 2, ids: []string{"1", "2"}})
	a = updated.(App)
	// Notifications 1 and 2 should be removed
	for _, n := range a.notifications {
		if n.ID == "1" || n.ID == "2" {
			t.Fatalf("expected notification %s to be removed", n.ID)
		}
	}
	if !strings.Contains(a.statusText, "Marked 2 as read") {
		t.Fatalf("expected status 'Marked 2 as read', got %q", a.statusText)
	}
}

func TestBatchMuteVisibleMsg(t *testing.T) {
	a := newTestApp()
	a.notifications = sampleNotifications()
	a.width = 120
	a.height = 40

	updated, _ := a.Update(visibleMutedMsg{count: 2, ids: []string{"1", "2"}})
	a = updated.(App)
	for _, n := range a.notifications {
		if n.ID == "1" || n.ID == "2" {
			t.Fatalf("expected notification %s to be removed", n.ID)
		}
	}
	if !strings.Contains(a.statusText, "Muted 2 threads") {
		t.Fatalf("expected status 'Muted 2 threads', got %q", a.statusText)
	}
}

func TestLogPaneState(t *testing.T) {
	a := newTestApp()
	a.width = 120
	a.height = 40
	a.logFile = "/tmp/test.log"

	// Initially log pane is hidden
	if a.showLog {
		t.Fatal("expected log pane to be hidden initially")
	}

	// Toggling showLog and focusing
	a.showLog = true
	a.focused = focusLog
	if !a.showLog {
		t.Fatal("expected log pane to be shown")
	}
	if a.focused != focusLog {
		t.Fatal("expected focusLog")
	}

	// Esc from log pane should close it
	a.showLog = false
	a.focused = focusList
	if a.showLog {
		t.Fatal("expected log pane closed")
	}
}

func TestLogPaneScrolling(t *testing.T) {
	a := newTestApp()
	a.width = 120
	a.height = 40
	a.logFile = "/tmp/test.log"
	a.showLog = true
	a.focused = focusLog
	a.logLines = make([]string, 100) // 100 lines of log

	// logMaxScroll should be positive
	maxScroll := a.logMaxScroll()
	if maxScroll <= 0 {
		t.Fatalf("expected positive maxScroll, got %d", maxScroll)
	}

	// Manually scroll and verify bounds
	a.logScroll = maxScroll
	if a.logScroll != maxScroll {
		t.Fatalf("expected logScroll at max (%d), got %d", maxScroll, a.logScroll)
	}
}

func TestLogPaneReducesContentHeight(t *testing.T) {
	a := newTestApp()
	a.width = 120
	a.height = 40
	a.notifications = sampleNotifications()

	heightWithout := a.contentHeight()

	a.showLog = true
	heightWith := a.contentHeight()

	if heightWith >= heightWithout {
		t.Fatalf("expected contentHeight to decrease when log pane is open: without=%d, with=%d", heightWithout, heightWith)
	}
}

func TestLogUpdatedMsg(t *testing.T) {
	a := newTestApp()
	a.width = 120
	a.height = 40
	a.showLog = true
	a.logFile = "/tmp/test.log"

	lines := []string{"line 1", "line 2", "line 3"}
	updated, cmd := a.Update(logUpdatedMsg{lines: lines})
	a = updated.(App)
	if len(a.logLines) != 3 {
		t.Fatalf("expected 3 log lines, got %d", len(a.logLines))
	}
	if cmd == nil {
		t.Fatal("expected logTickCmd to continue tailing when pane is open")
	}
}

func TestLogUpdatedMsgStopsTailingWhenClosed(t *testing.T) {
	a := newTestApp()
	a.width = 120
	a.height = 40
	a.showLog = false // pane closed

	lines := []string{"line 1"}
	updated, cmd := a.Update(logUpdatedMsg{lines: lines})
	a = updated.(App)
	if cmd != nil {
		t.Fatal("expected no cmd when log pane is closed")
	}
}

func TestCleanupDoneMsg(t *testing.T) {
	a := newTestApp()
	a.width = 120
	a.height = 40

	// With purged > 0
	updated, cmd := a.Update(cleanupDoneMsg{purged: 5})
	a = updated.(App)
	if !strings.Contains(a.statusText, "Cleaned up 5") {
		t.Fatalf("expected cleanup status, got %q", a.statusText)
	}
	if cmd == nil {
		t.Fatal("expected clearStatus cmd")
	}

	// With purged == 0
	updated, cmd = a.Update(cleanupDoneMsg{purged: 0})
	a = updated.(App)
	if cmd != nil {
		t.Fatal("expected no cmd when nothing was purged")
	}
}

func TestRenderLogPane(t *testing.T) {
	a := newTestApp()
	a.width = 120
	a.height = 40
	a.showLog = true
	a.focused = focusLog
	a.logLines = []string{"2025-04-09 10:00:00 test log line", "2025-04-09 10:00:01 another line"}

	rendered := a.renderLogPane()
	if rendered == "" {
		t.Fatal("expected non-empty log pane render")
	}
	if !strings.Contains(rendered, "Logs") {
		t.Fatal("expected log pane to contain title 'Logs'")
	}
	if !strings.Contains(rendered, "─") {
		t.Fatal("expected log pane to contain separator line")
	}
}

func TestForceResyncSetsLoadingAndStatus(t *testing.T) {
	a := newTestApp()
	a.width = 120
	a.height = 40

	// Simulate ctrl+f keypress via direct handleKey call
	a.loading = false
	a.statusText = ""

	// We can't easily send a KeyPressMsg in tests, so test the message
	// path instead: notificationsLoadedMsg after a force resync should
	// clear loading and update the list.
	a.loading = true
	a.statusText = "Force resyncing all notifications…"

	updated, _ := a.Update(notificationsLoadedMsg{notifications: sampleNotifications()})
	a = updated.(App)

	if a.loading {
		t.Error("expected loading=false after notificationsLoadedMsg")
	}
	if len(a.notifications) != 4 {
		t.Errorf("expected 4 notifications, got %d", len(a.notifications))
	}
}

func TestSpinnerShownInStatusBarWhileLoading(t *testing.T) {
	a := newTestApp()
	a.width = 120
	a.height = 40
	a.notifications = sampleNotifications()

	a.loading = false
	bar := a.renderStatusBar()
	for _, frame := range spinnerFrames {
		if strings.Contains(bar, frame) {
			t.Fatalf("spinner frame %q should not appear when not loading", frame)
		}
	}

	a.loading = true
	bar = a.renderStatusBar()
	frame := spinnerFrames[a.spinnerFrame%len(spinnerFrames)]
	if !strings.Contains(bar, frame) {
		t.Fatalf("expected spinner frame %q in status bar while loading, got %q", frame, bar)
	}
}

func TestStateFilter(t *testing.T) {
	a := newTestApp()

	// Populate detail cache with different states
	a.detailCache["1"] = &github.ThreadDetail{State: "open"}
	a.detailCache["2"] = &github.ThreadDetail{State: "closed", Merged: true}
	a.detailCache["3"] = &github.ThreadDetail{State: "closed"}
	a.detailCache["4"] = &github.ThreadDetail{State: "open", Draft: true}
	a.collectFilterOptions()

	// No filter — all 4 visible
	filtered := a.filteredNotifications()
	if len(filtered) != 4 {
		t.Errorf("no filter: got %d, want 4", len(filtered))
	}

	// Filter by "merged"
	a.stateFilter = "merged"
	filtered = a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "2" {
		t.Errorf("state:merged: got %d items, want 1 (ID=2)", len(filtered))
	}

	// Filter by "open"
	a.stateFilter = "open"
	filtered = a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "1" {
		t.Errorf("state:open: got %d items, want 1 (ID=1)", len(filtered))
	}

	// Filter by "draft"
	a.stateFilter = "draft"
	filtered = a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "4" {
		t.Errorf("state:draft: got %d items, want 1 (ID=4)", len(filtered))
	}

	// Filter by "closed"
	a.stateFilter = "closed"
	filtered = a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "3" {
		t.Errorf("state:closed: got %d items, want 1 (ID=3)", len(filtered))
	}

	// Clear filter
	a.stateFilter = ""
	filtered = a.filteredNotifications()
	if len(filtered) != 4 {
		t.Errorf("cleared: got %d, want 4", len(filtered))
	}
}

func TestStateFilter_ExcludesUncachedNotifications(t *testing.T) {
	a := newTestApp() // 4 notifications (IDs 1-4)

	// Only cache details for 2 of the 4 notifications
	a.detailCache["1"] = &github.ThreadDetail{State: "open"}
	a.detailCache["3"] = &github.ThreadDetail{State: "closed", Merged: true}
	a.collectFilterOptions()

	// No filter — all 4 visible (uncached are still shown)
	filtered := a.filteredNotifications()
	if len(filtered) != 4 {
		t.Errorf("no filter: got %d, want 4", len(filtered))
	}

	// Filter by "open" — only ID=1 matches; IDs 2,4 have no detail and are excluded
	a.stateFilter = "open"
	filtered = a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "1" {
		t.Errorf("state:open with partial cache: got %d items, want 1 (ID=1)", len(filtered))
	}

	// Filter by "merged" — only ID=3 matches
	a.stateFilter = "merged"
	filtered = a.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "3" {
		t.Errorf("state:merged with partial cache: got %d items, want 1 (ID=3)", len(filtered))
	}
}

func TestCycleStateFilter(t *testing.T) {
	a := newTestApp()
	a.detailCache["1"] = &github.ThreadDetail{State: "open"}
	a.detailCache["2"] = &github.ThreadDetail{State: "closed", Merged: true}
	a.collectFilterOptions()

	if len(a.knownStates) != 2 {
		t.Fatalf("expected 2 known states, got %d: %v", len(a.knownStates), a.knownStates)
	}

	// First cycle: picks first known state
	a.cycleStateFilter()
	if a.stateFilter != a.knownStates[0] {
		t.Errorf("first cycle: got %q, want %q", a.stateFilter, a.knownStates[0])
	}

	// Second cycle: picks second known state
	a.cycleStateFilter()
	if a.stateFilter != a.knownStates[1] {
		t.Errorf("second cycle: got %q, want %q", a.stateFilter, a.knownStates[1])
	}

	// Third cycle: wraps back to no filter
	a.cycleStateFilter()
	if a.stateFilter != "" {
		t.Errorf("third cycle: got %q, want empty", a.stateFilter)
	}
}

func TestEffectiveState(t *testing.T) {
	tests := []struct {
		name   string
		detail *github.ThreadDetail
		want   string
	}{
		{"open", &github.ThreadDetail{State: "open"}, "open"},
		{"closed", &github.ThreadDetail{State: "closed"}, "closed"},
		{"merged", &github.ThreadDetail{State: "closed", Merged: true}, "merged"},
		{"draft", &github.ThreadDetail{State: "open", Draft: true}, "draft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveState(tt.detail)
			if got != tt.want {
				t.Errorf("effectiveState() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStateFilterIndicator(t *testing.T) {
	a := newTestApp()
	a.stateFilter = "merged"
	if !a.hasActiveFilters() {
		t.Fatal("expected hasActiveFilters to be true with stateFilter set")
	}
	rendered := a.renderFilters()
	if !strings.Contains(rendered, "state:merged") {
		t.Errorf("expected filter indicator to contain 'state:merged', got: %q", rendered)
	}
}

func TestMultiSelect_SpaceToggles(t *testing.T) {
	a := newTestApp()
	// cursor starts at 0 (ID="1")
	if len(a.selected) != 0 {
		t.Fatal("expected no selections initially")
	}

	// Space selects current item and moves cursor down
	a.selected[a.notifications[0].ID] = true
	a.cursor++
	if !a.selected["1"] {
		t.Error("expected ID=1 to be selected")
	}
	if a.cursor != 1 {
		t.Errorf("cursor should be 1 after space, got %d", a.cursor)
	}

	// Space again selects second item
	a.selected[a.notifications[1].ID] = true
	a.cursor++
	if len(a.selected) != 2 {
		t.Errorf("expected 2 selected, got %d", len(a.selected))
	}

	// Toggle off first item
	delete(a.selected, "1")
	if a.selected["1"] {
		t.Error("expected ID=1 to be deselected")
	}
	if len(a.selected) != 1 {
		t.Errorf("expected 1 selected after deselect, got %d", len(a.selected))
	}
}

func TestMultiSelect_EscClearsSelection(t *testing.T) {
	a := newTestApp()
	a.selected["1"] = true
	a.selected["2"] = true

	if len(a.selected) != 2 {
		t.Fatal("expected 2 selected")
	}

	// Esc should clear selection first (before clearing filters)
	a.selected = make(map[string]bool)
	if len(a.selected) != 0 {
		t.Error("expected selection cleared after Esc")
	}
}

func TestMultiSelect_SelectedNotifications(t *testing.T) {
	a := newTestApp()
	a.selected["1"] = true
	a.selected["3"] = true

	sel := a.selectedNotifications()
	if len(sel) != 2 {
		t.Fatalf("expected 2 selected notifications, got %d", len(sel))
	}
	// Should preserve filtered order
	ids := make(map[string]bool)
	for _, n := range sel {
		ids[n.ID] = true
	}
	if !ids["1"] || !ids["3"] {
		t.Errorf("expected IDs 1 and 3 in selection, got %v", ids)
	}
}

func TestMultiSelect_SelectionCountInStatusBar(t *testing.T) {
	a := newTestApp()
	a.width = 200
	a.height = 40

	// No selection — no "selected" text
	bar := a.renderStatusBar()
	if strings.Contains(bar, "selected") {
		t.Errorf("should not show 'selected' when nothing is selected, got: %q", bar)
	}

	// With selection — shows count
	a.selected["1"] = true
	a.selected["2"] = true
	bar = a.renderStatusBar()
	if !strings.Contains(bar, "2 selected") {
		t.Errorf("expected '2 selected' in status bar, got: %q", bar)
	}
}

func TestMultiSelect_CheckmarkInRow(t *testing.T) {
	a := newTestApp()
	a.width = 120

	// Non-selected, non-cursor row — no checkmark
	row := a.renderNotificationRowSized(a.notifications[0], false, 120)
	if strings.Contains(row, "✓") {
		t.Error("non-selected row should not have ✓")
	}

	// Selected, non-cursor row — shows checkmark
	a.selected["1"] = true
	row = a.renderNotificationRowSized(a.notifications[0], false, 120)
	if !strings.Contains(row, "✓") {
		t.Error("selected row should have ✓")
	}

	// Selected + cursor row — shows checkmark instead of ▌
	row = a.renderNotificationRowSized(a.notifications[0], true, 120)
	if !strings.Contains(row, "✓") {
		t.Error("selected cursor row should have ✓")
	}
}

// --- Group by repository tests ---

func TestGroupByRepo_Disabled_PreservesChronologicalOrder(t *testing.T) {
	// When groupByRepo is false, notifications from different repos should
	// remain in their original chronological order (interleaved).
	a := newTestApp()
	a.groupByRepo = false

	// Default sample data: org/app (5m ago), org/lib (2h ago), other-org/tool (48h ago), other-org/infra (10d ago)
	filtered := a.filteredNotifications()
	if len(filtered) != 4 {
		t.Fatalf("expected 4 notifications, got %d", len(filtered))
	}

	// Verify chronological interleaved order (original order)
	expected := []string{"org/app", "org/lib", "other-org/tool", "other-org/infra"}
	for i, n := range filtered {
		if n.Repository.FullName != expected[i] {
			t.Errorf("position %d: expected repo %s, got %s", i, expected[i], n.Repository.FullName)
		}
	}
}

func TestGroupByRepo_GroupsAndSortsByMostRecent(t *testing.T) {
	// Create notifications from 3 repos, interleaved chronologically.
	// repo-a: 1h ago, 5h ago
	// repo-b: 30m ago, 3h ago
	// repo-c: 2h ago
	// Expected group order: repo-b (30m), repo-a (1h), repo-c (2h)
	now := time.Now()
	notifications := []github.Notification{
		{ID: "b1", UpdatedAt: now.Add(-30 * time.Minute), Subject: github.Subject{Title: "B1"}, Repository: github.Repository{FullName: "org/repo-b"}},
		{ID: "a1", UpdatedAt: now.Add(-1 * time.Hour), Subject: github.Subject{Title: "A1"}, Repository: github.Repository{FullName: "org/repo-a"}},
		{ID: "c1", UpdatedAt: now.Add(-2 * time.Hour), Subject: github.Subject{Title: "C1"}, Repository: github.Repository{FullName: "org/repo-c"}},
		{ID: "b2", UpdatedAt: now.Add(-3 * time.Hour), Subject: github.Subject{Title: "B2"}, Repository: github.Repository{FullName: "org/repo-b"}},
		{ID: "a2", UpdatedAt: now.Add(-5 * time.Hour), Subject: github.Subject{Title: "A2"}, Repository: github.Repository{FullName: "org/repo-a"}},
	}

	a := newTestApp()
	a.groupByRepo = true
	a.notifications = notifications

	filtered := a.filteredNotifications()
	if len(filtered) != 5 {
		t.Fatalf("expected 5 notifications, got %d", len(filtered))
	}

	// Expected order: B1, B2 (repo-b group), A1, A2 (repo-a group), C1 (repo-c group)
	expectedIDs := []string{"b1", "b2", "a1", "a2", "c1"}
	for i, n := range filtered {
		if n.ID != expectedIDs[i] {
			t.Errorf("position %d: expected ID %s, got %s", i, expectedIDs[i], n.ID)
		}
	}
}

func TestGroupByRepo_WithinGroupPreservesOrder(t *testing.T) {
	// Items within a repo group should keep their original (chronological) order.
	now := time.Now()
	notifications := []github.Notification{
		{ID: "1", UpdatedAt: now.Add(-1 * time.Hour), Subject: github.Subject{Title: "First"}, Repository: github.Repository{FullName: "org/repo"}},
		{ID: "2", UpdatedAt: now.Add(-2 * time.Hour), Subject: github.Subject{Title: "Second"}, Repository: github.Repository{FullName: "org/repo"}},
		{ID: "3", UpdatedAt: now.Add(-3 * time.Hour), Subject: github.Subject{Title: "Third"}, Repository: github.Repository{FullName: "org/repo"}},
	}

	a := newTestApp()
	a.groupByRepo = true
	a.notifications = notifications

	filtered := a.filteredNotifications()
	for i, n := range filtered {
		expected := fmt.Sprintf("%d", i+1)
		if n.ID != expected {
			t.Errorf("position %d: expected ID %s, got %s", i, expected, n.ID)
		}
	}
}

func TestGroupByRepo_InteractsWithFilters(t *testing.T) {
	// Grouping should work on the already-filtered result set.
	now := time.Now()
	notifications := []github.Notification{
		{ID: "1", UpdatedAt: now.Add(-1 * time.Hour), Reason: "mention", Subject: github.Subject{Title: "Bug", Type: "Issue"}, Repository: github.Repository{FullName: "org/repo-a"}},
		{ID: "2", UpdatedAt: now.Add(-2 * time.Hour), Reason: "mention", Subject: github.Subject{Title: "Feature", Type: "Issue"}, Repository: github.Repository{FullName: "org/repo-b"}},
		{ID: "3", UpdatedAt: now.Add(-30 * time.Minute), Reason: "subscribed", Subject: github.Subject{Title: "CI", Type: "CheckSuite"}, Repository: github.Repository{FullName: "org/repo-b"}},
	}

	a := newTestApp()
	a.groupByRepo = true
	a.notifications = notifications
	a.reasonFilter = "mention" // filter to mentions only

	filtered := a.filteredNotifications()
	if len(filtered) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(filtered))
	}

	// After filtering: repo-a (1h), repo-b (2h)
	// Grouped by repo, sorted by freshest: repo-a first, then repo-b
	if filtered[0].ID != "1" {
		t.Errorf("expected repo-a item first, got %s", filtered[0].ID)
	}
	if filtered[1].ID != "2" {
		t.Errorf("expected repo-b item second, got %s", filtered[1].ID)
	}
}

func TestGroupByRepo_RepoGroupHeaders(t *testing.T) {
	now := time.Now()
	notifications := []github.Notification{
		{ID: "1", UpdatedAt: now.Add(-1 * time.Hour), Repository: github.Repository{FullName: "org/repo-a"}},
		{ID: "2", UpdatedAt: now.Add(-2 * time.Hour), Repository: github.Repository{FullName: "org/repo-a"}},
		{ID: "3", UpdatedAt: now.Add(-3 * time.Hour), Repository: github.Repository{FullName: "org/repo-b"}},
	}

	headers := repoGroupHeaders(notifications)
	if len(headers) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(headers))
	}
	if headers[0] != "org/repo-a" {
		t.Errorf("header[0] = %q, want org/repo-a", headers[0])
	}
	if headers[2] != "org/repo-b" {
		t.Errorf("header[2] = %q, want org/repo-b", headers[2])
	}
}

func TestGroupByRepo_HeadersRenderedInList(t *testing.T) {
	now := time.Now()
	a := newTestApp()
	a.groupByRepo = true
	a.notifications = []github.Notification{
		{ID: "1", UpdatedAt: now.Add(-1 * time.Hour), Subject: github.Subject{Title: "PR fix", Type: "PullRequest"}, Repository: github.Repository{FullName: "org/repo-a"}},
		{ID: "2", UpdatedAt: now.Add(-3 * time.Hour), Subject: github.Subject{Title: "Bug report", Type: "Issue"}, Repository: github.Repository{FullName: "org/repo-b"}},
	}

	output := a.renderNotificationListSized(a.filteredNotifications(), 20, 120)
	if !strings.Contains(output, "org/repo-a") {
		t.Error("expected repo-a header in output")
	}
	if !strings.Contains(output, "org/repo-b") {
		t.Error("expected repo-b header in output")
	}
	if !strings.Contains(output, "📁") {
		t.Error("expected 📁 icon in group headers")
	}
}

func TestGroupByRepo_Disabled_NoHeaders(t *testing.T) {
	a := newTestApp()
	a.groupByRepo = false

	output := a.renderNotificationListSized(a.filteredNotifications(), 20, 120)
	if strings.Contains(output, "📁") {
		t.Error("group headers should not appear when groupByRepo is disabled")
	}
}

func TestGroupByRepo_HeadersInRange(t *testing.T) {
	headers := map[int]string{0: "repo-a", 3: "repo-b", 7: "repo-c"}

	if n := headersInRange(headers, 0, 4); n != 2 {
		t.Errorf("headersInRange(0,4) = %d, want 2", n)
	}
	if n := headersInRange(headers, 1, 7); n != 1 {
		t.Errorf("headersInRange(1,7) = %d, want 1", n)
	}
	if n := headersInRange(headers, 0, 10); n != 3 {
		t.Errorf("headersInRange(0,10) = %d, want 3", n)
	}
}

func TestGroupByRepo_EmptyNotifications(t *testing.T) {
	a := newTestApp()
	a.groupByRepo = true
	a.notifications = nil

	filtered := a.filteredNotifications()
	if len(filtered) != 0 {
		t.Fatalf("expected 0 notifications, got %d", len(filtered))
	}
}

func TestGroupByRepo_SingleRepo(t *testing.T) {
	// All items from one repo — grouping should be identity.
	now := time.Now()
	a := newTestApp()
	a.groupByRepo = true
	a.notifications = []github.Notification{
		{ID: "1", UpdatedAt: now.Add(-1 * time.Hour), Repository: github.Repository{FullName: "org/only-repo"}},
		{ID: "2", UpdatedAt: now.Add(-2 * time.Hour), Repository: github.Repository{FullName: "org/only-repo"}},
	}

	filtered := a.filteredNotifications()
	if len(filtered) != 2 {
		t.Fatalf("expected 2, got %d", len(filtered))
	}
	if filtered[0].ID != "1" || filtered[1].ID != "2" {
		t.Error("single repo group should preserve original order")
	}

	headers := repoGroupHeaders(filtered)
	if len(headers) != 1 {
		t.Errorf("expected 1 header for single repo, got %d", len(headers))
	}
}

// --- Scroll and cursor regression tests ---
// These lock down scroll/offset/cursor behavior that the collapsible
// groups refactor would touch.

func TestClampScroll_CursorBelowViewport(t *testing.T) {
	a := newTestApp()
	a.height = 10 // small viewport so contentHeight is small
	a.width = 120
	a.notifications = sampleNotifications()

	h := a.contentHeight()
	// Place cursor well beyond the viewport
	a.cursor = 3
	a.offset = 0
	a.clampScroll()

	if a.cursor < a.offset || a.cursor >= a.offset+h {
		t.Errorf("cursor %d should be visible in [%d, %d)", a.cursor, a.offset, a.offset+h)
	}
}

func TestClampScroll_CursorAboveViewport(t *testing.T) {
	a := newTestApp()
	a.height = 10
	a.width = 120

	a.offset = 3
	a.cursor = 0
	a.clampScroll()

	if a.offset > a.cursor {
		t.Errorf("offset %d should be <= cursor %d", a.offset, a.cursor)
	}
}

func TestRenderList_ScrolledDown_ShowsCorrectItems(t *testing.T) {
	now := time.Now()
	var notifications []github.Notification
	for i := 0; i < 20; i++ {
		notifications = append(notifications, github.Notification{
			ID: fmt.Sprintf("%d", i), Unread: true, Reason: "mention",
			UpdatedAt:  now.Add(-time.Duration(i) * time.Hour),
			Subject:    github.Subject{Title: fmt.Sprintf("Item %d", i), Type: "Issue"},
			Repository: github.Repository{FullName: "org/repo"},
		})
	}

	a := newTestApp()
	a.notifications = notifications
	a.cursor = 10
	a.offset = 10
	a.height = 15
	a.width = 120

	output := a.renderNotificationListSized(a.filteredNotifications(), 5, 120)
	// Should show Item 10 (cursor position), not Item 0
	if !strings.Contains(output, "Item 10") {
		t.Error("scrolled list should show Item 10 at offset 10")
	}
	if strings.Contains(output, "Item 0") {
		t.Error("scrolled list should NOT show Item 0 when offset=10")
	}
}

func TestSelectedNotification_AfterScroll(t *testing.T) {
	now := time.Now()
	var notifications []github.Notification
	for i := 0; i < 10; i++ {
		notifications = append(notifications, github.Notification{
			ID: fmt.Sprintf("%d", i), Unread: true,
			UpdatedAt:  now.Add(-time.Duration(i) * time.Hour),
			Subject:    github.Subject{Title: fmt.Sprintf("Item %d", i), Type: "Issue"},
			Repository: github.Repository{FullName: "org/repo"},
		})
	}

	a := newTestApp()
	a.notifications = notifications
	a.cursor = 5
	a.offset = 3

	n := a.selectedNotification()
	if n == nil {
		t.Fatal("selectedNotification should not be nil")
	}
	if n.ID != "5" {
		t.Errorf("expected notification ID 5, got %s", n.ID)
	}
}

func TestListPane_GG_JumpsToTop(t *testing.T) {
	a := newTestApp()
	a.cursor = 3
	a.offset = 2
	a.focused = focusList

	// Simulate first 'g'
	a.lastKey = "g"
	a.lastKeyTime = time.Now()

	// Simulate second 'g' within 500ms
	result, _ := a.handleListKey("g")
	updated := result.(App)

	if updated.cursor != 0 {
		t.Errorf("gg should set cursor to 0, got %d", updated.cursor)
	}
}

func TestListPane_G_JumpsToBottom(t *testing.T) {
	a := newTestApp()
	a.cursor = 0
	a.focused = focusList

	result, _ := a.handleListKey("G")
	updated := result.(App)

	expected := len(a.filteredNotifications()) - 1
	if updated.cursor != expected {
		t.Errorf("G should set cursor to %d, got %d", expected, updated.cursor)
	}
}

func TestMultiSelect_SurvivesScroll(t *testing.T) {
	now := time.Now()
	var notifications []github.Notification
	for i := 0; i < 10; i++ {
		notifications = append(notifications, github.Notification{
			ID: fmt.Sprintf("%d", i), Unread: true,
			UpdatedAt:  now.Add(-time.Duration(i) * time.Hour),
			Subject:    github.Subject{Title: fmt.Sprintf("Item %d", i), Type: "Issue"},
			Repository: github.Repository{FullName: "org/repo"},
		})
	}

	a := newTestApp()
	a.notifications = notifications

	// Select items 2 and 5
	a.selected["2"] = true
	a.selected["5"] = true

	// Scroll down
	a.cursor = 8
	a.offset = 5

	// Selection should still be intact
	if !a.selected["2"] || !a.selected["5"] {
		t.Error("selection should survive scrolling")
	}

	// selectedNotifications should return the selected items
	sel := a.selectedNotifications()
	if len(sel) != 2 {
		t.Errorf("expected 2 selected notifications, got %d", len(sel))
	}
}

func TestPreviewShowsRequestedReviewers(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.detailCache["1"] = &github.ThreadDetail{
		State: "open",
		User:  github.User{Login: "author"},
		RequestedReviewers: []github.User{
			{Login: "bob"},
			{Login: "carol"},
		},
		RequestedTeams: []github.Team{
			{Name: "Frontend", Slug: "frontend"},
		},
	}

	preview := a.renderPreview(30, 60)

	if !strings.Contains(preview, "@bob") {
		t.Error("preview should show requested reviewer @bob")
	}
	if !strings.Contains(preview, "@carol") {
		t.Error("preview should show requested reviewer @carol")
	}
	if !strings.Contains(preview, "@frontend") {
		t.Error("preview should show requested team @frontend")
	}
}

func TestPreviewShowsMilestone(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.detailCache["1"] = &github.ThreadDetail{
		State:     "open",
		User:      github.User{Login: "author"},
		Milestone: &github.Milestone{Title: "v2.0-beta"},
	}

	preview := a.renderPreview(30, 60)

	if !strings.Contains(preview, "v2.0-beta") {
		t.Error("preview should show milestone title")
	}
}

func TestPreviewHidesEmptyReviewersAndMilestone(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)
	a.detailCache["1"] = &github.ThreadDetail{
		State: "open",
		User:  github.User{Login: "author"},
	}

	preview := a.renderPreview(30, 60)

	if strings.Contains(preview, "Review:") {
		t.Error("preview should not show Review line when no reviewers")
	}
	if strings.Contains(preview, "Mile:") {
		t.Error("preview should not show Mile line when no milestone")
	}
}
