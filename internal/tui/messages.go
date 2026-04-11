package tui

import (
	"github.com/cassiomarques/gh-bell/internal/github"
)

// --- Messages ---
// In the Elm Architecture, all state changes are triggered by messages.
// The Update function pattern-matches on these types to decide what to do.
// This keeps state transitions explicit and traceable — you can always
// grep for a message type to find every place it's handled.

// notificationsLoadedMsg is sent when the API returns notifications.
type notificationsLoadedMsg struct {
	notifications []github.Notification
}

// errorMsg is sent when an API call or other operation fails.
type errorMsg struct {
	err error
}

// statusMsg is sent to display a temporary message in the status bar.
type statusMsg struct {
	text    string
	isError bool
}

// clearStatusMsg signals that the status bar should be cleared.
type clearStatusMsg struct{}

// threadMarkedReadMsg is sent after successfully marking a thread as read.
type threadMarkedReadMsg struct {
	threadID string
}

// allMarkedReadMsg is sent after marking all notifications as read.
type allMarkedReadMsg struct{}

// threadMutedMsg is sent after muting a thread.
type threadMutedMsg struct {
	threadID string
}

// threadUnsubscribedMsg is sent after unsubscribing from a thread.
type threadUnsubscribedMsg struct {
	threadID string
}

// threadDoneMsg is sent after dismissing a notification ("Done").
type threadDoneMsg struct {
	threadID string
}

// refreshTickMsg triggers a periodic re-fetch of notifications.
type refreshTickMsg struct{}

// threadDetailLoadedMsg carries enriched detail fetched lazily for a thread.
type threadDetailLoadedMsg struct {
	threadID string
	detail   *github.ThreadDetail
}

// threadDetailErrorMsg is sent when detail fetching fails (non-fatal).
type threadDetailErrorMsg struct {
	threadID string
}

// spinnerTickMsg advances the loading spinner animation frame.
type spinnerTickMsg struct{}

// cachedNotificationsLoadedMsg carries notifications loaded from the local
// SQLite cache for instant startup (before the API refresh completes).
type cachedNotificationsLoadedMsg struct {
	notifications []github.Notification
}

// searchResultsMsg carries Bleve full-text search results.
type searchResultsMsg struct {
	threadIDs []string // matching thread IDs
	query     string   // the query that produced these results
}

// currentUserMsg carries the authenticated user's login, fetched once at startup.
type currentUserMsg struct {
	login string
}

// visibleMarkedReadMsg is sent after batch-marking all visible notifications as read.
type visibleMarkedReadMsg struct {
	count int
	ids   []string
}

// visibleMutedMsg is sent after batch-muting all visible notifications.
type visibleMutedMsg struct {
	count int
	ids   []string
}

// batchDoneMsg is sent after batch-dismissing notifications.
type batchDoneMsg struct {
	count int
	ids   []string
}

// cleanupDoneMsg is sent after auto-cleanup of old notifications completes.
type cleanupDoneMsg struct {
	purged int
}

// logUpdatedMsg carries refreshed log file contents.
type logUpdatedMsg struct {
	lines []string
}

// prEnrichmentMsg carries GraphQL enrichment results for PR notifications.
type prEnrichmentMsg struct {
	enrichments map[string]*github.PREnrichment
}
