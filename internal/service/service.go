package service

import (
	"log"
	"strings"
	"time"

	"github.com/cassiomarques/gh-bell/internal/github"
	"github.com/cassiomarques/gh-bell/internal/search"
	"github.com/cassiomarques/gh-bell/internal/storage"
)

// DefaultDetailTTL is how long cached thread details are considered fresh.
const DefaultDetailTTL = 1 * time.Hour

// NotificationService orchestrates the GitHub API client and local SQLite
// store. The TUI talks only to this service — never to the API or DB directly.
type NotificationService struct {
	client    *github.Client
	store     *storage.Store
	search    *search.SearchIndex
	detailTTL time.Duration
}

// New creates a NotificationService.
func New(client *github.Client, store *storage.Store) *NotificationService {
	return &NotificationService{
		client:    client,
		store:     store,
		detailTTL: DefaultDetailTTL,
	}
}

// SetSearch sets the Bleve search index.
func (s *NotificationService) SetSearch(idx *search.SearchIndex) {
	s.search = idx
}

// SetDetailTTL overrides the default detail cache TTL (useful in tests).
func (s *NotificationService) SetDetailTTL(d time.Duration) {
	s.detailTTL = d
}

// Close closes the underlying store and search index.
func (s *NotificationService) Close() error {
	var firstErr error
	if s.search != nil {
		if err := s.search.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.store != nil {
		if err := s.store.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Store returns the underlying Store (for direct access when needed, e.g. preferences).
func (s *NotificationService) Store() *storage.Store {
	return s.store
}

// --- Notification List ---

// LoadCached returns notifications from the local SQLite cache for instant
// startup. If unreadOnly is true, only unread notifications are returned.
func (s *NotificationService) LoadCached(unreadOnly bool) ([]github.Notification, error) {
	return s.store.ListNotifications(unreadOnly)
}

// Refresh fetches notifications from the GitHub API, stores them in SQLite,
// and returns the merged result. This is the primary "refresh from API" path.
func (s *NotificationService) Refresh(opts github.ListOptions) ([]github.Notification, error) {
	notifications, err := s.client.ListNotifications(opts)
	if err != nil {
		return nil, err
	}

	if len(notifications) > 0 {
		if storeErr := s.store.UpsertNotifications(notifications); storeErr != nil {
			log.Printf("warning: failed to cache notifications: %v", storeErr)
		}
		// Index notification titles in Bleve for full-text search
		s.indexNotifications(notifications)
	}

	return notifications, nil
}

// --- Thread Details ---

// GetCachedDetail returns a cached thread detail if it exists and is fresh
// (within the TTL). Returns (nil, false) if not cached or stale.
// The notificationUpdatedAt parameter allows forced invalidation when the
// notification has been updated since the detail was fetched.
func (s *NotificationService) GetCachedDetail(threadID string, notificationUpdatedAt time.Time) (*github.ThreadDetail, bool) {
	detail, fetchedAt, err := s.store.GetDetail(threadID)
	if err != nil {
		log.Printf("warning: failed to read cached detail for %s: %v", threadID, err)
		return nil, false
	}
	if detail == nil {
		return nil, false
	}

	// Stale if fetched before the notification was updated
	if !notificationUpdatedAt.IsZero() && fetchedAt.Before(notificationUpdatedAt) {
		return nil, false
	}

	// Stale if older than TTL
	if time.Since(fetchedAt) > s.detailTTL {
		return nil, false
	}

	return detail, true
}

// FetchAndStoreDetail fetches thread detail from the API and stores it in
// the local cache. Also updates the Bleve search index with enriched content.
// The notification metadata (title, repo, etc.) is needed to build the full
// search document.
func (s *NotificationService) FetchAndStoreDetail(threadID, subjectURL, commentURL string, n *github.Notification) (*github.ThreadDetail, error) {
	detail, err := s.client.FetchThreadDetail(subjectURL, commentURL)
	if err != nil {
		return nil, err
	}

	if storeErr := s.store.UpsertDetail(threadID, detail); storeErr != nil {
		log.Printf("warning: failed to cache detail for %s: %v", threadID, storeErr)
	}

	// Update search index with enriched content (body + comment)
	if s.search != nil && n != nil {
		var commentText string
		if detail.LatestComment != nil {
			commentText = detail.LatestComment.Body
		}
		var labelNames []string
		for _, l := range detail.Labels {
			labelNames = append(labelNames, l.Name)
		}
		if idxErr := s.search.Index(threadID, n.Subject.Title, detail.Body,
			commentText, strings.Join(labelNames, " "),
			n.Repository.FullName, n.Reason, n.Subject.Type); idxErr != nil {
			log.Printf("warning: failed to index detail for %s: %v", threadID, idxErr)
		}
	}

	return detail, nil
}

// --- Actions ---

// MarkThreadRead marks a thread as read via the API and updates the local cache.
func (s *NotificationService) MarkThreadRead(threadID string) error {
	if err := s.client.MarkThreadRead(threadID); err != nil {
		return err
	}
	if storeErr := s.store.MarkNotificationRead(threadID); storeErr != nil {
		log.Printf("warning: failed to update cache for mark-read %s: %v", threadID, storeErr)
	}
	return nil
}

// MarkAllRead marks all notifications as read via the API and updates the cache.
func (s *NotificationService) MarkAllRead(upTo *time.Time) error {
	if err := s.client.MarkAllRead(upTo); err != nil {
		return err
	}
	if storeErr := s.store.MarkAllRead(); storeErr != nil {
		log.Printf("warning: failed to update cache for mark-all-read: %v", storeErr)
	}
	return nil
}

// MuteThread mutes a thread via the API, marks it read, and persists the
// mute in the local store so it survives across sessions.
func (s *NotificationService) MuteThread(threadID, repoFullName, subjectTitle string) error {
	if err := s.client.MuteThread(threadID); err != nil {
		return err
	}
	// Also mark as read (muting only prevents future notifications)
	_ = s.client.MarkThreadRead(threadID)

	if storeErr := s.store.MuteThread(threadID, repoFullName, subjectTitle); storeErr != nil {
		log.Printf("warning: failed to persist mute for %s: %v", threadID, storeErr)
	}
	if storeErr := s.store.MarkNotificationRead(threadID); storeErr != nil {
		log.Printf("warning: failed to update cache for mute %s: %v", threadID, storeErr)
	}
	return nil
}

// UnsubscribeThread unsubscribes from a thread via the API and marks it read.
func (s *NotificationService) UnsubscribeThread(threadID string) error {
	if err := s.client.UnsubscribeThread(threadID); err != nil {
		return err
	}
	_ = s.client.MarkThreadRead(threadID)

	if storeErr := s.store.MarkNotificationRead(threadID); storeErr != nil {
		log.Printf("warning: failed to update cache for unsubscribe %s: %v", threadID, storeErr)
	}
	return nil
}

// --- Mute Persistence ---

// IsMuted checks if a thread is in the persistent mute list.
func (s *NotificationService) IsMuted(threadID string) bool {
	muted, err := s.store.IsMuted(threadID)
	if err != nil {
		log.Printf("warning: failed to check mute status for %s: %v", threadID, err)
		return false
	}
	return muted
}

// ListMuted returns all persistently muted threads.
func (s *NotificationService) ListMuted() ([]storage.MutedThread, error) {
	return s.store.ListMuted()
}

// UnmuteThread removes a thread from the persistent mute list.
func (s *NotificationService) UnmuteThread(threadID string) error {
	return s.store.UnmuteThread(threadID)
}

// --- Preferences ---

// GetPref retrieves a stored preference.
func (s *NotificationService) GetPref(key string) string {
	val, err := s.store.GetPref(key)
	if err != nil {
		log.Printf("warning: failed to read preference %q: %v", key, err)
		return ""
	}
	return val
}

// SetPref stores a preference.
func (s *NotificationService) SetPref(key, value string) error {
	return s.store.SetPref(key, value)
}

// --- Full-Text Search ---

// Search performs an exact full-text search across notification content.
func (s *NotificationService) Search(query string, limit int) ([]search.SearchResult, error) {
	if s.search == nil {
		return nil, nil
	}
	return s.search.Search(query, limit)
}

// SearchFuzzy performs a fuzzy/typo-tolerant search.
func (s *NotificationService) SearchFuzzy(query string, limit int) ([]search.SearchResult, error) {
	if s.search == nil {
		return nil, nil
	}
	return s.search.SearchFuzzy(query, limit)
}

// --- Internal helpers ---

// indexNotifications indexes notification titles in Bleve for full-text search.
// Body and comment text are added later when details are lazily fetched.
func (s *NotificationService) indexNotifications(notifications []github.Notification) {
	if s.search == nil {
		return
	}
	for _, n := range notifications {
		if err := s.search.Index(n.ID, n.Subject.Title, "", "",
			"", n.Repository.FullName, n.Reason, n.Subject.Type); err != nil {
			log.Printf("warning: failed to index notification %s: %v", n.ID, err)
		}
	}
}
