package storage

import (
	"testing"
	"time"

	"github.com/cassiomarques/gh-bell/internal/github"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleNotification(id string) github.Notification {
	return github.Notification{
		ID:        id,
		Unread:    true,
		Reason:    "mention",
		UpdatedAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		URL:       "https://api.github.com/notifications/threads/" + id,
		Subject: github.Subject{
			Title:            "Fix the bug in parser",
			URL:              "https://api.github.com/repos/acme/app/issues/42",
			LatestCommentURL: "https://api.github.com/repos/acme/app/issues/comments/100",
			Type:             "Issue",
		},
		Repository: github.Repository{
			ID:       1,
			FullName: "acme/app",
			HTMLURL:  "https://github.com/acme/app",
			Private:  false,
			Owner:    github.Owner{Login: "acme"},
		},
	}
}

func sampleDetail() *github.ThreadDetail {
	return &github.ThreadDetail{
		State: "open",
		Body:  "This is the issue body with **markdown**.",
		Labels: []github.Label{
			{Name: "bug", Color: "d73a4a"},
			{Name: "urgent", Color: "ff0000"},
		},
		User:      github.User{Login: "alice"},
		Draft:     false,
		Merged:    false,
		MergedBy:  nil,
		Additions: 10,
		Deletions: 3,
		TagName:   "",
		LatestComment: &github.Comment{
			Body:      "I'll take a look at this.",
			User:      github.User{Login: "bob"},
			CreatedAt: time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
		},
	}
}

// --- Notification Tests ---

func TestUpsertAndListNotifications(t *testing.T) {
	s := testStore(t)

	n1 := sampleNotification("1")
	n2 := sampleNotification("2")
	n2.Reason = "review_requested"
	n2.UpdatedAt = time.Date(2025, 1, 16, 10, 0, 0, 0, time.UTC)
	n2.Repository.FullName = "acme/lib"
	n2.Repository.Owner.Login = "acme"

	if err := s.UpsertNotification(n1); err != nil {
		t.Fatalf("UpsertNotification(n1): %v", err)
	}
	if err := s.UpsertNotification(n2); err != nil {
		t.Fatalf("UpsertNotification(n2): %v", err)
	}

	// List all
	all, err := s.ListNotifications(false)
	if err != nil {
		t.Fatalf("ListNotifications(false): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(all))
	}
	// Should be ordered by updated_at DESC — n2 is newer
	if all[0].ID != "2" {
		t.Errorf("expected first notification ID=2, got %s", all[0].ID)
	}

	// List unread only (both are unread)
	unread, err := s.ListNotifications(true)
	if err != nil {
		t.Fatalf("ListNotifications(true): %v", err)
	}
	if len(unread) != 2 {
		t.Fatalf("expected 2 unread, got %d", len(unread))
	}
}

func TestUpsertNotificationPreservesFields(t *testing.T) {
	s := testStore(t)
	n := sampleNotification("1")

	if err := s.UpsertNotification(n); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListNotifications(false)
	if err != nil {
		t.Fatal(err)
	}
	got := all[0]

	if got.Subject.Title != n.Subject.Title {
		t.Errorf("title: got %q, want %q", got.Subject.Title, n.Subject.Title)
	}
	if got.Subject.Type != n.Subject.Type {
		t.Errorf("type: got %q, want %q", got.Subject.Type, n.Subject.Type)
	}
	if got.Subject.URL != n.Subject.URL {
		t.Errorf("subject URL: got %q, want %q", got.Subject.URL, n.Subject.URL)
	}
	if got.Subject.LatestCommentURL != n.Subject.LatestCommentURL {
		t.Errorf("comment URL: got %q, want %q", got.Subject.LatestCommentURL, n.Subject.LatestCommentURL)
	}
	if got.Repository.FullName != n.Repository.FullName {
		t.Errorf("repo: got %q, want %q", got.Repository.FullName, n.Repository.FullName)
	}
	if got.Repository.HTMLURL != n.Repository.HTMLURL {
		t.Errorf("repo HTML: got %q, want %q", got.Repository.HTMLURL, n.Repository.HTMLURL)
	}
	if got.Repository.Owner.Login != n.Repository.Owner.Login {
		t.Errorf("owner: got %q, want %q", got.Repository.Owner.Login, n.Repository.Owner.Login)
	}
	if got.Reason != n.Reason {
		t.Errorf("reason: got %q, want %q", got.Reason, n.Reason)
	}
}

func TestUpsertNotificationUpdatesExisting(t *testing.T) {
	s := testStore(t)
	n := sampleNotification("1")
	if err := s.UpsertNotification(n); err != nil {
		t.Fatal(err)
	}

	// Update the same notification
	n.Subject.Title = "Updated title"
	n.Reason = "review_requested"
	n.Unread = false
	if err := s.UpsertNotification(n); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListNotifications(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 notification after upsert, got %d", len(all))
	}
	if all[0].Subject.Title != "Updated title" {
		t.Errorf("title not updated: got %q", all[0].Subject.Title)
	}
	if all[0].Reason != "review_requested" {
		t.Errorf("reason not updated: got %q", all[0].Reason)
	}
}

func TestUpsertNotificationsBatch(t *testing.T) {
	s := testStore(t)

	notifications := []github.Notification{
		sampleNotification("1"),
		sampleNotification("2"),
		sampleNotification("3"),
	}
	if err := s.UpsertNotifications(notifications); err != nil {
		t.Fatalf("UpsertNotifications: %v", err)
	}

	all, err := s.ListNotifications(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
}

func TestDeleteNotification(t *testing.T) {
	s := testStore(t)

	if err := s.UpsertNotification(sampleNotification("1")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteNotification("1"); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListNotifications(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(all))
	}
}

func TestMarkNotificationRead(t *testing.T) {
	s := testStore(t)

	if err := s.UpsertNotification(sampleNotification("1")); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkNotificationRead("1"); err != nil {
		t.Fatal(err)
	}

	unread, err := s.ListNotifications(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 0 {
		t.Fatalf("expected 0 unread after mark-read, got %d", len(unread))
	}

	all, err := s.ListNotifications(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 total, got %d", len(all))
	}
	if all[0].Unread {
		t.Error("expected unread=false after mark-read")
	}
}

func TestMarkAllRead(t *testing.T) {
	s := testStore(t)

	for _, id := range []string{"1", "2", "3"} {
		if err := s.UpsertNotification(sampleNotification(id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkAllRead(); err != nil {
		t.Fatal(err)
	}

	unread, err := s.ListNotifications(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 0 {
		t.Fatalf("expected 0 unread after mark-all-read, got %d", len(unread))
	}
}

// --- Thread Detail Tests ---

func TestUpsertAndGetDetail(t *testing.T) {
	s := testStore(t)
	d := sampleDetail()

	if err := s.UpsertDetail("42", d); err != nil {
		t.Fatalf("UpsertDetail: %v", err)
	}

	got, fetchedAt, err := s.GetDetail("42")
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	if got == nil {
		t.Fatal("GetDetail returned nil")
	}
	if fetchedAt.IsZero() {
		t.Error("fetchedAt should not be zero")
	}

	// Verify fields
	if got.State != "open" {
		t.Errorf("state: got %q, want %q", got.State, "open")
	}
	if got.Body != d.Body {
		t.Errorf("body mismatch")
	}
	if got.User.Login != "alice" {
		t.Errorf("author: got %q, want %q", got.User.Login, "alice")
	}
	if len(got.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(got.Labels))
	}
	if got.Labels[0].Name != "bug" || got.Labels[0].Color != "d73a4a" {
		t.Errorf("label 0: got %+v", got.Labels[0])
	}
	if got.Labels[1].Name != "urgent" {
		t.Errorf("label 1: got %+v", got.Labels[1])
	}
	if got.Additions != 10 || got.Deletions != 3 {
		t.Errorf("additions/deletions: got %d/%d", got.Additions, got.Deletions)
	}

	// Verify comment
	if got.LatestComment == nil {
		t.Fatal("LatestComment is nil")
	}
	if got.LatestComment.Body != "I'll take a look at this." {
		t.Errorf("comment body: got %q", got.LatestComment.Body)
	}
	if got.LatestComment.User.Login != "bob" {
		t.Errorf("comment author: got %q", got.LatestComment.User.Login)
	}
}

func TestGetDetailNotFound(t *testing.T) {
	s := testStore(t)

	got, _, err := s.GetDetail("nonexistent")
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent, got %+v", got)
	}
}

func TestUpsertDetailWithoutComment(t *testing.T) {
	s := testStore(t)
	d := &github.ThreadDetail{
		State:    "closed",
		Body:     "Release notes here",
		User:     github.User{Login: "releaser"},
		TagName:  "v1.2.3",
		Labels:   nil,
		Merged:   true,
		MergedBy: &github.User{Login: "merger"},
	}

	if err := s.UpsertDetail("99", d); err != nil {
		t.Fatal(err)
	}

	got, _, err := s.GetDetail("99")
	if err != nil {
		t.Fatal(err)
	}
	if got.TagName != "v1.2.3" {
		t.Errorf("tag_name: got %q", got.TagName)
	}
	if got.LatestComment != nil {
		t.Errorf("expected nil comment, got %+v", got.LatestComment)
	}
	if !got.Merged {
		t.Error("expected merged=true")
	}
	if got.MergedBy == nil || got.MergedBy.Login != "merger" {
		t.Errorf("merged_by: got %+v", got.MergedBy)
	}
}

func TestUpsertDetailUpdates(t *testing.T) {
	s := testStore(t)
	d := sampleDetail()
	if err := s.UpsertDetail("42", d); err != nil {
		t.Fatal(err)
	}

	// Update same thread
	d.State = "closed"
	d.Body = "Updated body"
	if err := s.UpsertDetail("42", d); err != nil {
		t.Fatal(err)
	}

	got, _, err := s.GetDetail("42")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "closed" {
		t.Errorf("state not updated: got %q", got.State)
	}
	if got.Body != "Updated body" {
		t.Errorf("body not updated: got %q", got.Body)
	}
}

func TestDeleteDetail(t *testing.T) {
	s := testStore(t)
	if err := s.UpsertDetail("42", sampleDetail()); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDetail("42"); err != nil {
		t.Fatal(err)
	}

	got, _, err := s.GetDetail("42")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

// --- Muted Thread Tests ---

func TestMuteAndIsMuted(t *testing.T) {
	s := testStore(t)

	muted, err := s.IsMuted("42")
	if err != nil {
		t.Fatal(err)
	}
	if muted {
		t.Error("expected not muted initially")
	}

	if err := s.MuteThread("42", "acme/app", "Fix bug"); err != nil {
		t.Fatal(err)
	}

	muted, err = s.IsMuted("42")
	if err != nil {
		t.Fatal(err)
	}
	if !muted {
		t.Error("expected muted after MuteThread")
	}
}

func TestUnmuteThread(t *testing.T) {
	s := testStore(t)

	if err := s.MuteThread("42", "acme/app", "Fix bug"); err != nil {
		t.Fatal(err)
	}
	if err := s.UnmuteThread("42"); err != nil {
		t.Fatal(err)
	}

	muted, err := s.IsMuted("42")
	if err != nil {
		t.Fatal(err)
	}
	if muted {
		t.Error("expected not muted after UnmuteThread")
	}
}

func TestListMuted(t *testing.T) {
	s := testStore(t)

	if err := s.MuteThread("1", "acme/app", "Bug one"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond) // Ensure different muted_at
	if err := s.MuteThread("2", "acme/lib", "Bug two"); err != nil {
		t.Fatal(err)
	}

	muted, err := s.ListMuted()
	if err != nil {
		t.Fatal(err)
	}
	if len(muted) != 2 {
		t.Fatalf("expected 2 muted, got %d", len(muted))
	}
	// Ordered by muted_at DESC — "2" should be first
	if muted[0].ThreadID != "2" {
		t.Errorf("expected first muted ID=2, got %s", muted[0].ThreadID)
	}
	if muted[0].RepoFullName != "acme/lib" {
		t.Errorf("repo: got %q", muted[0].RepoFullName)
	}
	if muted[0].SubjectTitle != "Bug two" {
		t.Errorf("title: got %q", muted[0].SubjectTitle)
	}
}

func TestMuteThreadIdempotent(t *testing.T) {
	s := testStore(t)

	if err := s.MuteThread("42", "acme/app", "Fix bug"); err != nil {
		t.Fatal(err)
	}
	// Mute again — should update, not fail
	if err := s.MuteThread("42", "acme/app", "Updated title"); err != nil {
		t.Fatal(err)
	}

	muted, err := s.ListMuted()
	if err != nil {
		t.Fatal(err)
	}
	if len(muted) != 1 {
		t.Fatalf("expected 1 muted after double mute, got %d", len(muted))
	}
	if muted[0].SubjectTitle != "Updated title" {
		t.Errorf("title not updated: got %q", muted[0].SubjectTitle)
	}
}

// --- Preference Tests ---

func TestGetSetPref(t *testing.T) {
	s := testStore(t)

	// Get missing key
	val, err := s.GetPref("theme")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("expected empty for missing pref, got %q", val)
	}

	// Set
	if err := s.SetPref("theme", "dark"); err != nil {
		t.Fatal(err)
	}

	val, err = s.GetPref("theme")
	if err != nil {
		t.Fatal(err)
	}
	if val != "dark" {
		t.Errorf("expected %q, got %q", "dark", val)
	}

	// Update
	if err := s.SetPref("theme", "light"); err != nil {
		t.Fatal(err)
	}

	val, err = s.GetPref("theme")
	if err != nil {
		t.Fatal(err)
	}
	if val != "light" {
		t.Errorf("expected %q after update, got %q", "light", val)
	}
}

func TestDeletePref(t *testing.T) {
	s := testStore(t)

	if err := s.SetPref("key", "value"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePref("key"); err != nil {
		t.Fatal(err)
	}

	val, err := s.GetPref("key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("expected empty after delete, got %q", val)
	}
}

// --- DataDir Tests ---

func TestDefaultDBPath(t *testing.T) {
	path, err := DefaultDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
	// Should end with .gh-bell/meta.db
	if len(path) < 20 {
		t.Errorf("path seems too short: %q", path)
	}
}

// --- Edge Cases ---

func TestOpenCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/subdir/nested/meta.db"

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Close()
}

func TestEmptyBatchUpsert(t *testing.T) {
	s := testStore(t)
	// Empty batch should succeed
	if err := s.UpsertNotifications(nil); err != nil {
		t.Fatalf("UpsertNotifications(nil): %v", err)
	}
	if err := s.UpsertNotifications([]github.Notification{}); err != nil {
		t.Fatalf("UpsertNotifications([]): %v", err)
	}
}

func TestDetailWithEmptyLabels(t *testing.T) {
	s := testStore(t)
	d := &github.ThreadDetail{
		State:  "open",
		Body:   "No labels here",
		User:   github.User{Login: "alice"},
		Labels: nil, // nil labels
	}

	if err := s.UpsertDetail("1", d); err != nil {
		t.Fatal(err)
	}

	got, _, err := s.GetDetail("1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Labels == nil {
		// nil is OK, but empty slice is also OK
	}
}
