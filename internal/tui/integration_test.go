package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cassiomarques/gh-bell/internal/github"
	"github.com/cassiomarques/gh-bell/internal/search"
	"github.com/cassiomarques/gh-bell/internal/service"
	"github.com/cassiomarques/gh-bell/internal/storage"
)

// --- Integration test helpers ---

// integrationStack wires a FakeClient + real in-memory SQLite + real
// in-memory Bleve + real NotificationService for end-to-end testing.
type integrationStack struct {
	fake    *github.FakeClient
	store   *storage.Store
	idx     *search.SearchIndex
	service *service.NotificationService
}

func newIntegrationStack(t *testing.T) *integrationStack {
	t.Helper()

	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	idx, err := search.OpenInMemory()
	if err != nil {
		t.Fatalf("open search index: %v", err)
	}
	t.Cleanup(func() { idx.Close() })

	fake := &github.FakeClient{
		CurrentUser: "testuser",
	}

	svc := service.New(fake, store)
	svc.SetSearch(idx)
	t.Cleanup(func() { svc.Close() })

	return &integrationStack{
		fake:    fake,
		store:   store,
		idx:     idx,
		service: svc,
	}
}

func sampleIntegrationNotifications() []github.Notification {
	return []github.Notification{
		{
			ID: "100", Unread: true, Reason: "review_requested",
			UpdatedAt: time.Now().Add(-5 * time.Minute),
			Subject: github.Subject{
				Title: "Fix authentication bug in login flow",
				Type:  "PullRequest",
				URL:   "https://api.github.com/repos/acme/web/pulls/42",
			},
			Repository: github.Repository{FullName: "acme/web", HTMLURL: "https://github.com/acme/web"},
		},
		{
			ID: "101", Unread: true, Reason: "mention",
			UpdatedAt: time.Now().Add(-2 * time.Hour),
			Subject: github.Subject{
				Title: "Add caching layer for API responses",
				Type:  "Issue",
				URL:   "https://api.github.com/repos/acme/lib/issues/10",
			},
			Repository: github.Repository{FullName: "acme/lib", HTMLURL: "https://github.com/acme/lib"},
		},
		{
			ID: "102", Unread: false, Reason: "assign",
			UpdatedAt: time.Now().Add(-48 * time.Hour),
			Subject: github.Subject{
				Title: "Upgrade database migration tooling",
				Type:  "Issue",
				URL:   "https://api.github.com/repos/other/tools/issues/5",
			},
			Repository: github.Repository{FullName: "other/tools", HTMLURL: "https://github.com/other/tools"},
		},
	}
}

// --- Integration tests ---

func TestIntegration_FullStartupFlow(t *testing.T) {
	stack := newIntegrationStack(t)
	stack.fake.Notifications = sampleIntegrationNotifications()

	app := NewApp(stack.fake, WithService(stack.service))
	app.width = 120
	app.height = 40

	// Init returns batched Cmds — extract and run the fetch command.
	// In Bubble Tea, Init() returns a Cmd that the runtime executes.
	initCmd := app.Init()
	if initCmd == nil {
		t.Fatal("Init should return commands")
	}

	// Simulate the Cmd execution: run fetch via service directly
	notifications, err := stack.service.Refresh(github.ListOptions{View: github.ViewUnread, PerPage: 50})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Feed the result as a message (simulating what the Cmd would produce)
	updated, _ := app.Update(notificationsLoadedMsg{notifications: notifications})
	app = updated.(App)

	if len(app.notifications) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(app.notifications))
	}

	// Verify notifications were persisted in SQLite
	cached, err := stack.service.LoadCached(false)
	if err != nil {
		t.Fatalf("LoadCached: %v", err)
	}
	if len(cached) != 3 {
		t.Fatalf("expected 3 cached notifications, got %d", len(cached))
	}
}

func TestIntegration_TwoPhaseStartup(t *testing.T) {
	stack := newIntegrationStack(t)
	stack.fake.Notifications = sampleIntegrationNotifications()

	// Phase 1: populate cache from a prior "session"
	_, err := stack.service.Refresh(github.ListOptions{View: github.ViewUnread, PerPage: 50})
	if err != nil {
		t.Fatalf("initial refresh: %v", err)
	}

	// Phase 2: simulate a new app launch — should load cached first
	app := NewApp(stack.fake, WithService(stack.service))
	app.width = 120
	app.height = 40

	// Simulate cachedNotificationsLoadedMsg (what loadCachedNotificationsCmd produces)
	cached, _ := stack.service.LoadCached(true)
	updated, _ := app.Update(cachedNotificationsLoadedMsg{notifications: cached})
	app = updated.(App)

	// App should show cached data immediately, still in loading state
	if len(app.notifications) == 0 {
		t.Fatal("expected cached notifications to be displayed immediately")
	}
	if !app.loading {
		t.Fatal("should still be loading (API refresh pending)")
	}

	// Then API refresh arrives
	fresh, _ := stack.service.Refresh(github.ListOptions{View: github.ViewUnread, PerPage: 50})
	updated, _ = app.Update(notificationsLoadedMsg{notifications: fresh})
	app = updated.(App)

	if app.loading {
		t.Fatal("should not be loading after API refresh")
	}
	if len(app.notifications) != 3 {
		t.Fatalf("expected 3 notifications after refresh, got %d", len(app.notifications))
	}
}

func TestIntegration_MarkReadPersists(t *testing.T) {
	stack := newIntegrationStack(t)
	stack.fake.Notifications = sampleIntegrationNotifications()

	// Load notifications
	app := NewApp(stack.fake, WithService(stack.service))
	app.width = 120
	app.height = 40
	notifications, _ := stack.service.Refresh(github.ListOptions{View: github.ViewUnread, PerPage: 50})
	updated, _ := app.Update(notificationsLoadedMsg{notifications: notifications})
	app = updated.(App)

	// Mark first notification as read
	threadID := app.notifications[0].ID
	err := stack.service.MarkThreadRead(threadID)
	if err != nil {
		t.Fatalf("MarkThreadRead: %v", err)
	}
	updated, _ = app.Update(threadMarkedReadMsg{threadID: threadID})
	app = updated.(App)

	// Verify it was removed from the app's list
	for _, n := range app.notifications {
		if n.ID == threadID {
			t.Fatal("marked-read notification should be removed from list")
		}
	}

	// Verify the API was called
	if len(stack.fake.MarkedRead) != 1 || stack.fake.MarkedRead[0] != threadID {
		t.Fatalf("expected API call to mark %q read, got %v", threadID, stack.fake.MarkedRead)
	}
}

func TestIntegration_MutePersistsAcrossSessions(t *testing.T) {
	stack := newIntegrationStack(t)
	stack.fake.Notifications = sampleIntegrationNotifications()

	// Session 1: load and mute
	app := NewApp(stack.fake, WithService(stack.service))
	app.width = 120
	app.height = 40
	notifications, _ := stack.service.Refresh(github.ListOptions{View: github.ViewUnread, PerPage: 50})
	updated, _ := app.Update(notificationsLoadedMsg{notifications: notifications})
	app = updated.(App)

	mutedID := "101"
	err := stack.service.MuteThread(mutedID, "acme/lib", "Add caching layer for API responses")
	if err != nil {
		t.Fatalf("MuteThread: %v", err)
	}
	// Feed muted msg to remove from TUI
	updated, _ = app.Update(threadMutedMsg{threadID: mutedID})
	app = updated.(App)

	for _, n := range app.notifications {
		if n.ID == mutedID {
			t.Fatal("muted notification should be removed from TUI list")
		}
	}

	// Verify it's in the persistent mute list
	if !stack.service.IsMuted(mutedID) {
		t.Fatal("thread should be in persistent mute list")
	}

	// Session 2: verify persistence survives — mute is still recorded
	muted, err := stack.service.ListMuted()
	if err != nil {
		t.Fatalf("ListMuted: %v", err)
	}
	found := false
	for _, m := range muted {
		if m.ThreadID == mutedID {
			found = true
		}
	}
	if !found {
		t.Fatal("muted thread should persist across sessions")
	}
}

func TestIntegration_FullTextSearchRoundTrip(t *testing.T) {
	stack := newIntegrationStack(t)
	stack.fake.Notifications = sampleIntegrationNotifications()
	stack.fake.Details = map[string]*github.ThreadDetail{
		"https://api.github.com/repos/acme/web/pulls/42": {
			Body:      "This PR fixes the authentication bug where users could not log in with SSO.",
			State:     "open",
			Assignees: []github.User{{Login: "testuser"}},
		},
		"https://api.github.com/repos/acme/lib/issues/10": {
			Body:  "We need a caching layer using Redis for our API responses.",
			State: "open",
		},
		"https://api.github.com/repos/other/tools/issues/5": {
			Body:  "Current migration tool is outdated. Evaluate golang-migrate.",
			State: "open",
		},
	}

	// Load notifications (indexes titles)
	notifications, _ := stack.service.Refresh(github.ListOptions{View: github.ViewUnread, PerPage: 50})

	// Fetch details for each (indexes bodies)
	for _, n := range notifications {
		_, err := stack.service.FetchAndStoreDetail(n.ID, n.Subject.URL, n.Subject.LatestCommentURL, &n)
		if err != nil {
			t.Fatalf("FetchAndStoreDetail(%s): %v", n.ID, err)
		}
	}

	// Search for "authentication" — should find notification 100
	results, err := stack.service.Search("authentication", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results for 'authentication'")
	}

	found := false
	for _, r := range results {
		if r.ThreadID == "100" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected thread 100 in search results for 'authentication'")
	}

	// Search for "Redis" — should find notification 101
	results2, err := stack.service.Search("Redis", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found = false
	for _, r := range results2 {
		if r.ThreadID == "101" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected thread 101 in search results for 'Redis'")
	}

	// Wire into TUI search filter
	app := NewApp(stack.fake, WithService(stack.service))
	app.width = 120
	app.height = 40
	updated, _ := app.Update(notificationsLoadedMsg{notifications: notifications})
	app = updated.(App)

	// Simulate searchResultsMsg
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ThreadID
	}
	updated, _ = app.Update(searchResultsMsg{query: "authentication", threadIDs: ids})
	app = updated.(App)

	filtered := app.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "100" {
		t.Fatalf("expected 1 filtered notification (100), got %d", len(filtered))
	}
}

func TestIntegration_ViewPreferencePersists(t *testing.T) {
	stack := newIntegrationStack(t)
	stack.fake.Notifications = sampleIntegrationNotifications()

	// Session 1: switch to ViewAll
	app := NewApp(stack.fake, WithService(stack.service))
	app.width = 120
	app.height = 40
	a, _ := app.switchView(github.ViewAll)
	if a.currentView != github.ViewAll {
		t.Fatalf("expected ViewAll, got %d", a.currentView)
	}

	// Session 2: new App should restore ViewAll
	app2 := NewApp(stack.fake, WithService(stack.service))
	if app2.currentView != github.ViewAll {
		t.Fatalf("expected ViewAll restored, got %d", app2.currentView)
	}

	// Switch back to Unread
	a2, _ := app2.switchView(github.ViewUnread)
	_ = a2

	// Session 3: should restore Unread
	app3 := NewApp(stack.fake, WithService(stack.service))
	if app3.currentView != github.ViewUnread {
		t.Fatalf("expected ViewUnread restored, got %d", app3.currentView)
	}
}

func TestIntegration_AssignedFilterWithRealService(t *testing.T) {
	stack := newIntegrationStack(t)
	stack.fake.Notifications = sampleIntegrationNotifications()
	stack.fake.Details = map[string]*github.ThreadDetail{
		"https://api.github.com/repos/acme/web/pulls/42": {
			State:     "open",
			Assignees: []github.User{{Login: "testuser"}},
		},
		"https://api.github.com/repos/acme/lib/issues/10": {
			State:     "open",
			Assignees: []github.User{{Login: "other-dev"}},
		},
		"https://api.github.com/repos/other/tools/issues/5": {
			State:     "open",
			Assignees: []github.User{{Login: "testuser"}, {Login: "other-dev"}},
		},
	}

	// Load notifications
	app := NewApp(stack.fake, WithService(stack.service))
	app.width = 120
	app.height = 40

	notifications, _ := stack.service.Refresh(github.ListOptions{View: github.ViewUnread, PerPage: 50})
	updated, _ := app.Update(notificationsLoadedMsg{notifications: notifications})
	app = updated.(App)

	// Set current user (normally fetched at startup)
	updated, _ = app.Update(currentUserMsg{login: "testuser"})
	app = updated.(App)

	// Fetch details into the detail cache
	for _, n := range notifications {
		detail, _ := stack.service.FetchAndStoreDetail(n.ID, n.Subject.URL, "", &n)
		app.detailCache[n.ID] = detail
	}

	// Enable assigned filter
	app.assignedFilter = true

	filtered := app.filteredNotifications()
	// Should include 100 (assigned to testuser) and 102 (reason=assign fallback + assigned)
	// but NOT 101 (assigned to other-dev)
	for _, n := range filtered {
		if n.ID == "101" {
			t.Fatal("notification 101 (assigned to other-dev) should be excluded")
		}
	}

	foundIDs := make(map[string]bool)
	for _, n := range filtered {
		foundIDs[n.ID] = true
	}
	if !foundIDs["100"] {
		t.Fatal("expected notification 100 (testuser is assignee)")
	}
	if !foundIDs["102"] {
		t.Fatal("expected notification 102 (reason=assign)")
	}
}

func TestIntegration_DetailCacheTTL(t *testing.T) {
	stack := newIntegrationStack(t)
	stack.fake.Notifications = sampleIntegrationNotifications()

	initialDetail := &github.ThreadDetail{
		Body:  "Initial body",
		State: "open",
	}
	stack.fake.Details = map[string]*github.ThreadDetail{
		"https://api.github.com/repos/acme/web/pulls/42": initialDetail,
	}

	// Fetch detail — should cache it in SQLite
	n := sampleIntegrationNotifications()[0]
	detail1, err := stack.service.FetchAndStoreDetail(n.ID, n.Subject.URL, "", &n)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if detail1.Body != "Initial body" {
		t.Fatalf("expected initial body, got %q", detail1.Body)
	}

	// GetCachedDetail should return the cached version (within TTL)
	cached, ok := stack.service.GetCachedDetail(n.ID, n.UpdatedAt)
	if !ok {
		t.Fatal("expected cached detail to be available")
	}
	if cached.Body != "Initial body" {
		t.Fatalf("expected cached 'Initial body', got %q", cached.Body)
	}

	// Expire the TTL — cached detail should be stale
	stack.service.SetDetailTTL(0)
	_, ok = stack.service.GetCachedDetail(n.ID, n.UpdatedAt)
	if ok {
		t.Fatal("expected cache miss after TTL expiry")
	}

	// Fetch again from API with updated content
	updatedDetail := &github.ThreadDetail{
		Body:  "Updated body with more info",
		State: "closed",
	}
	stack.fake.Details["https://api.github.com/repos/acme/web/pulls/42"] = updatedDetail

	detail2, err := stack.service.FetchAndStoreDetail(n.ID, n.Subject.URL, "", &n)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if detail2.Body != "Updated body with more info" {
		t.Fatalf("expected updated body, got %q", detail2.Body)
	}
}

func TestIntegration_ErrorFromAPI(t *testing.T) {
	stack := newIntegrationStack(t)
	stack.fake.ListErr = fmt.Errorf("HTTP 502: Bad Gateway")

	app := NewApp(stack.fake, WithService(stack.service))
	app.width = 120
	app.height = 40

	// Attempt to refresh — should get an error
	_, err := stack.service.Refresh(github.ListOptions{View: github.ViewUnread, PerPage: 50})
	if err == nil {
		t.Fatal("expected error from refresh with broken API")
	}

	// Feed error into TUI
	updated, _ := app.Update(errorMsg{err: err})
	app = updated.(App)

	if !app.statusError {
		t.Fatal("expected statusError set")
	}
	if !strings.Contains(app.statusText, "Error") {
		t.Fatalf("expected error in statusText, got %q", app.statusText)
	}
	if app.loading {
		t.Fatal("loading should be false after error")
	}
}

func TestIntegration_FiltersCombineWithSearch(t *testing.T) {
	stack := newIntegrationStack(t)
	stack.fake.Notifications = sampleIntegrationNotifications()

	app := NewApp(stack.fake, WithService(stack.service))
	app.width = 120
	app.height = 40

	notifications, _ := stack.service.Refresh(github.ListOptions{View: github.ViewUnread, PerPage: 50})
	updated, _ := app.Update(notificationsLoadedMsg{notifications: notifications})
	app = updated.(App)

	// Apply repo filter for "acme"
	app.repoFilter = "acme"
	filtered := app.filteredNotifications()
	if len(filtered) != 2 {
		t.Fatalf("repo filter 'acme' should match 2 notifications, got %d", len(filtered))
	}

	// Add search results that only include notification 100
	app.searchResultIDs = map[string]bool{"100": true}
	app.searchQuery = "auth"
	filtered = app.filteredNotifications()
	if len(filtered) != 1 || filtered[0].ID != "100" {
		t.Fatalf("combined repo + search should yield 1 result, got %d", len(filtered))
	}

	// Adding reason filter that doesn't match should yield 0
	app.reasonFilter = "mention"
	filtered = app.filteredNotifications()
	if len(filtered) != 0 {
		t.Fatalf("adding non-matching reason filter should yield 0, got %d", len(filtered))
	}
}
