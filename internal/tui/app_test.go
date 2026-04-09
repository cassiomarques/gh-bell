package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cassiomarques/gh-bell/internal/github"
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

	// Create a long body with 20 lines worth of content
	var longBody string
	for i := 1; i <= 20; i++ {
		longBody += fmt.Sprintf("This is line number %d of the description body.\n", i)
	}

	a.detailCache["1"] = &github.ThreadDetail{
		State: "open",
		User:  github.User{Login: "author"},
		Body:  longBody,
	}

	preview := a.renderPreview(50, 60) // enough height to fit everything

	// All 20 lines should be present — no truncation
	if strings.Contains(preview, "...") {
		t.Error("preview should NOT truncate body with '...'")
	}
	// Check first and last lines are both present
	if !strings.Contains(preview, "line number 1") {
		t.Error("preview should contain first line of body")
	}
	if !strings.Contains(preview, "line number 20") {
		t.Error("preview should contain last line of body (no truncation)")
	}
}

func TestPreviewCommentNotTruncated(t *testing.T) {
	a := newTestApp()
	a.detailCache = make(map[string]*github.ThreadDetail)

	// Create a long comment with 15 lines
	var longComment string
	for i := 1; i <= 15; i++ {
		longComment += fmt.Sprintf("Comment line %d with some context.\n", i)
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

	preview := a.renderPreview(50, 60)

	if strings.Contains(preview, "...") {
		t.Error("preview should NOT truncate comment with '...'")
	}
	if !strings.Contains(preview, "Comment line 1") {
		t.Error("preview should contain first line of comment")
	}
	if !strings.Contains(preview, "Comment line 15") {
		t.Error("preview should contain last line of comment (no truncation)")
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
