package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/cassiomarques/gh-bell/internal/github"
	"github.com/cassiomarques/gh-bell/internal/storage"
)

func testStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// --- Cache TTL Tests ---

func TestGetCachedDetail_Fresh(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store) // no API client needed for cache-only tests

	detail := &github.ThreadDetail{
		State: "open",
		Body:  "test body",
		User:  github.User{Login: "alice"},
	}
	if err := store.UpsertDetail("42", detail); err != nil {
		t.Fatal(err)
	}

	got, ok := svc.GetCachedDetail("42", time.Time{})
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.State != "open" {
		t.Errorf("state: got %q, want %q", got.State, "open")
	}
}

func TestGetCachedDetail_Stale_TTL(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store)
	svc.SetDetailTTL(1 * time.Millisecond) // very short TTL

	detail := &github.ThreadDetail{
		State: "open",
		Body:  "test body",
		User:  github.User{Login: "alice"},
	}
	if err := store.UpsertDetail("42", detail); err != nil {
		t.Fatal(err)
	}

	// Wait for TTL to expire
	time.Sleep(5 * time.Millisecond)

	_, ok := svc.GetCachedDetail("42", time.Time{})
	if ok {
		t.Error("expected cache miss (TTL expired)")
	}
}

func TestGetCachedDetail_Stale_UpdatedAt(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store)

	detail := &github.ThreadDetail{
		State: "open",
		Body:  "test body",
		User:  github.User{Login: "alice"},
	}
	if err := store.UpsertDetail("42", detail); err != nil {
		t.Fatal(err)
	}

	// Notification was updated AFTER the detail was cached
	futureUpdate := time.Now().Add(1 * time.Hour)
	_, ok := svc.GetCachedDetail("42", futureUpdate)
	if ok {
		t.Error("expected cache miss (notification updated after fetch)")
	}
}

func TestGetCachedDetail_NotFound(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store)

	_, ok := svc.GetCachedDetail("nonexistent", time.Time{})
	if ok {
		t.Error("expected cache miss for nonexistent")
	}
}

// --- Mute Persistence Tests ---

func TestIsMuted_PersistsThroughService(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store)

	if svc.IsMuted("42") {
		t.Error("expected not muted initially")
	}

	// Directly mute via store (simulating what MuteThread does after API call)
	if err := store.MuteThread("42", "acme/app", "Fix bug"); err != nil {
		t.Fatal(err)
	}

	if !svc.IsMuted("42") {
		t.Error("expected muted after persisting")
	}
}

func TestUnmuteThread_RemovesFromStore(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store)

	if err := store.MuteThread("42", "acme/app", "Fix bug"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UnmuteThread("42"); err != nil {
		t.Fatal(err)
	}
	if svc.IsMuted("42") {
		t.Error("expected not muted after unmute")
	}
}

func TestListMuted(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store)

	if err := store.MuteThread("1", "acme/app", "Bug one"); err != nil {
		t.Fatal(err)
	}
	if err := store.MuteThread("2", "acme/lib", "Bug two"); err != nil {
		t.Fatal(err)
	}

	muted, err := svc.ListMuted()
	if err != nil {
		t.Fatal(err)
	}
	if len(muted) != 2 {
		t.Fatalf("expected 2 muted, got %d", len(muted))
	}
}

// --- LoadCached Tests ---

func TestLoadCached_Empty(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store)

	notifications, err := svc.LoadCached(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 0 {
		t.Fatalf("expected 0, got %d", len(notifications))
	}
}

func TestLoadCached_WithData(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store)

	n := github.Notification{
		ID:        "1",
		Unread:    true,
		Reason:    "mention",
		UpdatedAt: time.Now().UTC(),
		Subject: github.Subject{
			Title: "Test notification",
			Type:  "Issue",
		},
		Repository: github.Repository{
			FullName: "acme/app",
			Owner:    github.Owner{Login: "acme"},
		},
	}
	if err := store.UpsertNotification(n); err != nil {
		t.Fatal(err)
	}

	notifications, err := svc.LoadCached(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 1 {
		t.Fatalf("expected 1, got %d", len(notifications))
	}
	if notifications[0].ID != "1" {
		t.Errorf("ID: got %q", notifications[0].ID)
	}
}

func TestLoadCached_UnreadOnly(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store)

	for _, id := range []string{"1", "2"} {
		n := github.Notification{
			ID:        id,
			Unread:    id == "1", // only "1" is unread
			Reason:    "mention",
			UpdatedAt: time.Now().UTC(),
			Subject:   github.Subject{Title: "Test", Type: "Issue"},
			Repository: github.Repository{
				FullName: "acme/app",
				Owner:    github.Owner{Login: "acme"},
			},
		}
		if err := store.UpsertNotification(n); err != nil {
			t.Fatal(err)
		}
	}

	unread, err := svc.LoadCached(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 1 {
		t.Fatalf("expected 1 unread, got %d", len(unread))
	}
	if unread[0].ID != "1" {
		t.Errorf("expected unread ID=1, got %s", unread[0].ID)
	}
}

// --- Preferences Tests ---

func TestPreferences(t *testing.T) {
	store := testStore(t)
	svc := New(nil, store)

	if got := svc.GetPref("view"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	if err := svc.SetPref("view", "all"); err != nil {
		t.Fatal(err)
	}

	if got := svc.GetPref("view"); got != "all" {
		t.Errorf("expected %q, got %q", "all", got)
	}
}

// --- Close Tests ---

func TestClose_NilStore(t *testing.T) {
	svc := &NotificationService{}
	if err := svc.Close(); err != nil {
		t.Errorf("Close with nil store: %v", err)
	}
}

// --- Pagination / Incremental Sync Tests ---

// paginatingFake implements NotificationAPI and returns different results
// per page, simulating the GitHub API pagination behavior including the
// Link header's HasNextPage signal.
type paginatingFake struct {
	github.FakeClient
	pages   map[int][]github.Notification // page -> notifications
	callLog []github.ListOptions          // recorded calls
}

func (f *paginatingFake) ListNotifications(opts github.ListOptions) (github.ListResult, error) {
	f.callLog = append(f.callLog, opts)
	if f.FakeClient.ListErr != nil {
		return github.ListResult{}, f.FakeClient.ListErr
	}
	page := opts.Page
	if page == 0 {
		page = 1
	}
	notifications := f.pages[page]
	_, nextExists := f.pages[page+1]
	return github.ListResult{
		Notifications: notifications,
		HasNextPage:   nextExists,
	}, nil
}

func makeNotification(id string, updatedAt time.Time) github.Notification {
	return github.Notification{
		ID: id, Unread: true, Reason: "mention",
		UpdatedAt: updatedAt,
		Subject:   github.Subject{Title: "Test " + id, Type: "Issue"},
		Repository: github.Repository{
			FullName: "acme/app", HTMLURL: "https://github.com/acme/app",
			Owner: github.Owner{Login: "acme"},
		},
	}
}

func TestSmartRefresh_FullSync_SinglePage(t *testing.T) {
	store := testStore(t)
	fake := &paginatingFake{
		pages: map[int][]github.Notification{
			1: {makeNotification("1", time.Now())},
		},
	}
	svc := New(fake, store)

	results, err := svc.SmartRefresh(github.ViewUnread)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !svc.IsFullSyncDone() {
		t.Error("expected full_sync_done to be set")
	}
}

func TestSmartRefresh_FullSync_MultiplePages(t *testing.T) {
	store := testStore(t)

	// Create pages: page 1 has 100 items (full page = more to come),
	// page 2 has 50 items (partial = last page).
	page1 := make([]github.Notification, 100)
	for i := range 100 {
		page1[i] = makeNotification(
			fmt.Sprintf("p1-%d", i),
			time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),
		)
	}
	page2 := make([]github.Notification, 50)
	for i := range 50 {
		page2[i] = makeNotification(
			fmt.Sprintf("p2-%d", i),
			time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		)
	}

	fake := &paginatingFake{
		pages: map[int][]github.Notification{1: page1, 2: page2},
	}
	svc := New(fake, store)

	results, err := svc.SmartRefresh(github.ViewUnread)
	if err != nil {
		t.Fatal(err)
	}

	// Should get all 150 from the cache
	if len(results) != 150 {
		t.Fatalf("expected 150 results, got %d", len(results))
	}
	// Should have made 2 API calls
	if len(fake.callLog) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(fake.callLog))
	}
	if fake.callLog[0].Page != 1 {
		t.Errorf("first call page: got %d, want 1", fake.callLog[0].Page)
	}
	if fake.callLog[1].Page != 2 {
		t.Errorf("second call page: got %d, want 2", fake.callLog[1].Page)
	}
	if !svc.IsFullSyncDone() {
		t.Error("expected full_sync_done to be set")
	}
}

func TestSmartRefresh_Incremental(t *testing.T) {
	store := testStore(t)

	// Seed the store with existing data and set full_sync_done
	existing := makeNotification("existing-1", time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC))
	if err := store.UpsertNotifications([]github.Notification{existing}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPref("full_sync_done", "1"); err != nil {
		t.Fatal(err)
	}

	// API returns 1 new notification
	newNotif := makeNotification("new-1", time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC))
	fake := &paginatingFake{
		pages: map[int][]github.Notification{1: {newNotif}},
	}
	svc := New(fake, store)

	results, err := svc.SmartRefresh(github.ViewUnread)
	if err != nil {
		t.Fatal(err)
	}

	// Should return both old + new from cache
	if len(results) != 2 {
		t.Fatalf("expected 2 results (old + new), got %d", len(results))
	}
	// The API call should have used the `since` parameter
	if len(fake.callLog) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(fake.callLog))
	}
	if fake.callLog[0].Since == nil {
		t.Error("expected Since parameter to be set for incremental refresh")
	}
}

func TestForceFullSync_ClearsFlagAndRefetches(t *testing.T) {
	store := testStore(t)

	// Seed store and mark as synced
	if err := store.SetPref("full_sync_done", "1"); err != nil {
		t.Fatal(err)
	}

	fake := &paginatingFake{
		pages: map[int][]github.Notification{
			1: {makeNotification("1", time.Now())},
		},
	}
	svc := New(fake, store)

	results, err := svc.ForceFullSync(github.ViewUnread)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// After force resync, flag should be set again
	if !svc.IsFullSyncDone() {
		t.Error("expected full_sync_done to be set after force resync")
	}
	// The call should NOT have used `since` (it's a full fetch)
	if fake.callLog[0].Since != nil {
		t.Error("force resync should not use since parameter")
	}
}

func TestSmartRefresh_FallsBackToFullWhenNoData(t *testing.T) {
	store := testStore(t)
	// Set full_sync_done but leave the store empty — should fall back to full sync
	if err := store.SetPref("full_sync_done", "1"); err != nil {
		t.Fatal(err)
	}

	fake := &paginatingFake{
		pages: map[int][]github.Notification{
			1: {makeNotification("1", time.Now())},
		},
	}
	svc := New(fake, store)

	results, err := svc.SmartRefresh(github.ViewUnread)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Should not have used since (fell back to full)
	if fake.callLog[0].Since != nil {
		t.Error("expected full sync (no since) when store is empty despite flag")
	}
}
