package service

import (
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
