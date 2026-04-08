package tui

import (
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
			Subject:   github.Subject{Title: "Release v2.0", Type: "Release", URL: "https://api.github.com/repos/org/tool/releases/5"},
			Repository: github.Repository{FullName: "org/tool", HTMLURL: "https://github.com/org/tool"},
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
	a.collectReasons()
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
	if a.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped)", a.cursor)
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
	if len(a.notifications) != 2 {
		t.Fatalf("expected 2 notifications after removal, got %d", len(a.notifications))
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

	if len(a.knownReasons) != 3 {
		t.Errorf("expected 3 known reasons, got %d", len(a.knownReasons))
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
