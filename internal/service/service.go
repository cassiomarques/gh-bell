package service

import (
	"fmt"
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
	client    github.NotificationAPI
	store     *storage.Store
	search    *search.SearchIndex
	detailTTL time.Duration
}

// New creates a NotificationService.
func New(client github.NotificationAPI, store *storage.Store) *NotificationService {
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

const (
	maxPerPage     = 100 // GitHub API maximum
	fullSyncPref   = "full_sync_done"
)

// LoadCached returns notifications from the local SQLite cache for instant
// startup. If unreadOnly is true, only unread notifications are returned.
func (s *NotificationService) LoadCached(unreadOnly bool) ([]github.Notification, error) {
	return s.store.ListNotifications(unreadOnly)
}

// Refresh fetches notifications from the GitHub API, stores them in SQLite,
// and returns the merged result. This is the primary "refresh from API" path.
// It still works as a single-page fetch for backward compatibility with tests.
func (s *NotificationService) Refresh(opts github.ListOptions) ([]github.Notification, error) {
	result, err := s.client.ListNotifications(opts)
	if err != nil {
		return nil, err
	}

	if len(result.Notifications) > 0 {
		if storeErr := s.store.UpsertNotifications(result.Notifications); storeErr != nil {
			log.Printf("warning: failed to cache notifications: %v", storeErr)
		}
		// Index notification titles in Bleve for full-text search
		s.indexNotifications(result.Notifications)
	}

	return result.Notifications, nil
}

// SmartRefresh decides between a full paginated sync (first time) and an
// incremental sync (using the `since` parameter) for subsequent refreshes.
// After syncing with the API, it returns all notifications from the local cache.
func (s *NotificationService) SmartRefresh(view github.View) ([]github.Notification, error) {
	if s.IsFullSyncDone() {
		return s.refreshIncremental(view)
	}
	return s.refreshFull(view)
}

// ForceFullSync clears the full_sync_done flag and performs a full paginated
// fetch from scratch. Used when the user wants to reload everything.
func (s *NotificationService) ForceFullSync(view github.View) ([]github.Notification, error) {
	s.store.DeletePref(fullSyncPref)
	return s.refreshFull(view)
}

// IsFullSyncDone returns whether a full sync has been completed previously.
func (s *NotificationService) IsFullSyncDone() bool {
	v, _ := s.store.GetPref(fullSyncPref)
	return v == "1"
}

// refreshFull paginates through all API pages to build a complete local cache.
// Uses the Link response header (rel="next") to detect more pages — the GitHub
// notifications API can return fewer items than per_page even when more exist.
func (s *NotificationService) refreshFull(view github.View) ([]github.Notification, error) {
	log.Println("sync: starting full paginated fetch")
	var total int
	page := 1
	for {
		opts := github.ListOptions{
			View:    view,
			PerPage: maxPerPage,
			Page:    page,
		}
		result, err := s.client.ListNotifications(opts)
		if err != nil {
			if total > 0 {
				// Partial success — return what we have from cache
				log.Printf("sync: error on page %d after fetching %d: %v", page, total, err)
				break
			}
			return nil, err
		}

		if len(result.Notifications) > 0 {
			if storeErr := s.store.UpsertNotifications(result.Notifications); storeErr != nil {
				log.Printf("warning: failed to cache page %d: %v", page, storeErr)
			}
			s.indexNotifications(result.Notifications)
			total += len(result.Notifications)
			log.Printf("sync: page %d fetched %d notifications (total: %d)", page, len(result.Notifications), total)
		}

		if !result.HasNextPage {
			break
		}
		page++
	}

	// Mark full sync as complete
	if err := s.store.SetPref(fullSyncPref, "1"); err != nil {
		log.Printf("warning: could not save full_sync_done pref: %v", err)
	}
	log.Printf("sync: full sync complete — %d notifications across %d pages", total, page)

	// Return everything from the local cache (merged across all pages)
	unreadOnly := view == github.ViewUnread
	return s.store.ListNotifications(unreadOnly)
}

// refreshIncremental fetches only notifications updated since the latest
// cached timestamp. Much faster than a full sync for periodic refreshes.
func (s *NotificationService) refreshIncremental(view github.View) ([]github.Notification, error) {
	since, err := s.store.LatestUpdatedAt()
	if err != nil {
		log.Printf("sync: could not get latest timestamp, falling back to full: %v", err)
		return s.refreshFull(view)
	}
	if since == nil {
		// No cached data — do a full sync
		return s.refreshFull(view)
	}

	log.Printf("sync: incremental fetch since %s", since.Format("2006-01-02T15:04:05Z"))
	var total int
	page := 1
	for {
		opts := github.ListOptions{
			View:    view,
			PerPage: maxPerPage,
			Page:    page,
			Since:   since,
		}
		result, err := s.client.ListNotifications(opts)
		if err != nil {
			if total > 0 {
				log.Printf("sync: incremental error on page %d after %d: %v", page, total, err)
				break
			}
			return nil, err
		}

		if len(result.Notifications) > 0 {
			if storeErr := s.store.UpsertNotifications(result.Notifications); storeErr != nil {
				log.Printf("warning: failed to cache incremental page %d: %v", page, storeErr)
			}
			s.indexNotifications(result.Notifications)
			total += len(result.Notifications)
		}

		if !result.HasNextPage {
			break
		}
		page++
	}

	if total > 0 {
		log.Printf("sync: incremental fetch got %d updated notifications", total)
	}

	// Return everything from the local cache
	unreadOnly := view == github.ViewUnread
	return s.store.ListNotifications(unreadOnly)
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

// EnrichPRs calls the GraphQL API to enrich PR notifications with review decision,
// CI status, mergeable state, and timestamps. Results are merged into the detail
// cache and persisted in SQLite.
func (s *NotificationService) EnrichPRs(notifications []github.Notification) (map[string]*github.PREnrichment, error) {
	var refs []github.PRRef
	for _, n := range notifications {
		if n.Subject.Type != "PullRequest" {
			continue
		}
		owner, repo, number, ok := github.ParseSubjectURL(n.Subject.URL)
		if !ok {
			continue
		}
		refs = append(refs, github.PRRef{
			Owner:    owner,
			Repo:     repo,
			Number:   number,
			ThreadID: n.ID,
		})
	}
	if len(refs) == 0 {
		return nil, nil
	}

	log.Printf("enrichment: enriching %d PRs via GraphQL", len(refs))
	enriched, err := s.client.EnrichPRsBatch(refs)
	if err != nil {
		return nil, fmt.Errorf("graphql enrichment: %w", err)
	}

	// Persist enrichment data to SQLite
	for threadID, e := range enriched {
		if storeErr := s.store.UpdateDetailEnrichment(threadID, e); storeErr != nil {
			log.Printf("warning: failed to persist enrichment for %s: %v", threadID, storeErr)
		}
	}

	log.Printf("enrichment: enriched %d/%d PRs", len(enriched), len(refs))
	return enriched, nil
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

// MarkThreadDone dismisses a notification via the API and removes it from the local cache.
func (s *NotificationService) MarkThreadDone(threadID string) error {
	if err := s.client.MarkThreadDone(threadID); err != nil {
		return err
	}
	if storeErr := s.store.DeleteNotification(threadID); storeErr != nil {
		log.Printf("warning: failed to delete cached notification for done %s: %v", threadID, storeErr)
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

// Cleanup purges read notifications older than the given number of days,
// along with their orphaned thread details. Returns total records purged.
func (s *NotificationService) Cleanup(days int) (int, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	purgedNotifs, err := s.store.PurgeOldNotifications(cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge notifications: %w", err)
	}
	purgedDetails, err := s.store.PurgeOldDetails()
	if err != nil {
		return purgedNotifs, fmt.Errorf("purge details: %w", err)
	}
	return purgedNotifs + purgedDetails, nil
}

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
