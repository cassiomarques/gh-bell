package tui

import (
	"fmt"
	"log"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cassiomarques/gh-bell/internal/github"
	"github.com/cassiomarques/gh-bell/internal/service"
)

// --- Commands (deferred effects) ---
//
// In the Elm Architecture, Update never performs side effects directly.
// Instead it returns Cmd values — functions that Bubble Tea will execute
// asynchronously. When the work completes, the Cmd sends a message back
// into Update. This keeps Update pure and testable: given a model + message
// it always returns the same new model + commands.
//
// Example flow:
//   1. User presses 'r' (mark read)
//   2. Update sees KeyPressMsg('r'), returns model + markReadCmd(id)
//   3. Bubble Tea runs markReadCmd in the background (API call)
//   4. On success, markReadCmd sends threadMarkedReadMsg{id}
//   5. Update receives threadMarkedReadMsg, removes the item from the list

const defaultRefreshInterval = 60 * time.Second
const maxRetries = 3

// authErrorMessage wraps an auth error with a clear, actionable message
// telling the user to regenerate their token.
func authErrorMessage(err error) error {
	return fmt.Errorf("token expired or invalid — regenerate your classic PAT at "+
		"https://github.com/settings/tokens and update 'token' in ~/.gh-bell/config.yaml")
}

// fetchNotificationsCmd returns a Cmd that fetches notifications from the API.
// When a service is provided, it also stores the results in SQLite.
// Retries up to maxRetries times on transient server errors with backoff.
// Auth errors (expired/invalid token) are returned immediately without retry.
func fetchNotificationsCmd(client github.NotificationAPI, svc *service.NotificationService, view github.View) tea.Cmd {
	return func() tea.Msg {
		var lastErr error
		for attempt := range maxRetries {
			if attempt > 0 {
				log.Printf("fetch notifications: retry %d/%d after error: %v", attempt+1, maxRetries, lastErr)
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
			}
			log.Printf("fetch notifications: attempt %d, view=%d", attempt+1, view)

			var notifications []github.Notification
			var err error
			if svc != nil {
				notifications, err = svc.Refresh(github.ListOptions{
					View:    view,
					PerPage: 50,
				})
			} else if client != nil {
				notifications, err = client.ListNotifications(github.ListOptions{
					View:    view,
					PerPage: 50,
				})
			} else {
				return errorMsg{err: fmt.Errorf("no client or service configured")}
			}

			if err == nil {
				log.Printf("fetch notifications: got %d results", len(notifications))
				return notificationsLoadedMsg{notifications: notifications}
			}
			lastErr = err

			// Auth errors won't resolve with retries — bail immediately
			if github.IsAuthError(err) {
				log.Printf("fetch notifications: auth error (expired/invalid token): %v", err)
				return errorMsg{err: authErrorMessage(err)}
			}
		}
		log.Printf("fetch notifications: all %d attempts failed: %v", maxRetries, lastErr)
		if github.IsServerError(lastErr) {
			return errorMsg{err: fmt.Errorf(
				"%w — GitHub's notification API may not work with OAuth tokens. "+
					"Set 'token' to a classic PAT in ~/.gh-bell/config.yaml", lastErr)}
		}
		return errorMsg{err: lastErr}
	}
}

// markReadCmd returns a Cmd that marks a single thread as read.
func markReadCmd(client github.NotificationAPI, svc *service.NotificationService, threadID string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if svc != nil {
			err = svc.MarkThreadRead(threadID)
		} else if client != nil {
			err = client.MarkThreadRead(threadID)
		}
		if err != nil {
			if github.IsAuthError(err) {
				return errorMsg{err: authErrorMessage(err)}
			}
			return errorMsg{err: err}
		}
		return threadMarkedReadMsg{threadID: threadID}
	}
}

// markAllReadCmd returns a Cmd that marks all notifications as read.
func markAllReadCmd(client github.NotificationAPI, svc *service.NotificationService) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		var err error
		if svc != nil {
			err = svc.MarkAllRead(&now)
		} else if client != nil {
			err = client.MarkAllRead(&now)
		}
		if err != nil {
			if github.IsAuthError(err) {
				return errorMsg{err: authErrorMessage(err)}
			}
			return errorMsg{err: err}
		}
		return allMarkedReadMsg{}
	}
}

// muteThreadCmd returns a Cmd that mutes a notification thread.
// Also marks the thread as read so it disappears from the unread list —
// GitHub's mute only prevents future notifications, it doesn't remove existing ones.
// When a service is available, the mute is persisted in SQLite.
func muteThreadCmd(client github.NotificationAPI, svc *service.NotificationService, threadID, repoFullName, subjectTitle string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if svc != nil {
			err = svc.MuteThread(threadID, repoFullName, subjectTitle)
		} else if client != nil {
			err = client.MuteThread(threadID)
			if err == nil {
				if readErr := client.MarkThreadRead(threadID); readErr != nil {
					log.Printf("mute: thread muted but failed to mark read: %v", readErr)
				}
			}
		}
		if err != nil {
			if github.IsAuthError(err) {
				return errorMsg{err: authErrorMessage(err)}
			}
			return errorMsg{err: err}
		}
		return threadMutedMsg{threadID: threadID}
	}
}

// unsubscribeCmd returns a Cmd that unsubscribes from a notification thread.
// Also marks the thread as read so it disappears from the unread list.
func unsubscribeCmd(client github.NotificationAPI, svc *service.NotificationService, threadID string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if svc != nil {
			err = svc.UnsubscribeThread(threadID)
		} else if client != nil {
			err = client.UnsubscribeThread(threadID)
			if err == nil {
				if readErr := client.MarkThreadRead(threadID); readErr != nil {
					log.Printf("unsubscribe: unsubscribed but failed to mark read: %v", readErr)
				}
			}
		}
		if err != nil {
			if github.IsAuthError(err) {
				return errorMsg{err: authErrorMessage(err)}
			}
			return errorMsg{err: err}
		}
		return threadUnsubscribedMsg{threadID: threadID}
	}
}

// refreshTickCmd returns a Cmd that sends a refreshTickMsg after an interval.
func refreshTickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

// clearStatusCmd returns a Cmd that clears the status bar after a delay.
func clearStatusCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

// fetchThreadDetailCmd lazily fetches enriched details for a notification thread.
// When a service is available, the result is also stored in SQLite and indexed.
func fetchThreadDetailCmd(client github.NotificationAPI, svc *service.NotificationService, threadID, subjectURL, commentURL string, n *github.Notification) tea.Cmd {
	return func() tea.Msg {
		log.Printf("fetch detail: thread=%s subject=%s comment=%s", threadID, subjectURL, commentURL)
		var detail *github.ThreadDetail
		var err error
		if svc != nil {
			detail, err = svc.FetchAndStoreDetail(threadID, subjectURL, commentURL, n)
		} else if client != nil {
			detail, err = client.FetchThreadDetail(subjectURL, commentURL)
		}
		if err != nil {
			log.Printf("fetch detail: error for thread %s: %v", threadID, err)
			return threadDetailErrorMsg{threadID: threadID}
		}
		return threadDetailLoadedMsg{threadID: threadID, detail: detail}
	}
}

// loadCachedNotificationsCmd loads notifications from the local SQLite cache
// for instant startup display.
func loadCachedNotificationsCmd(svc *service.NotificationService, view github.View) tea.Cmd {
	return func() tea.Msg {
		unreadOnly := view == github.ViewUnread
		notifications, err := svc.LoadCached(unreadOnly)
		if err != nil {
			log.Printf("load cached: error: %v", err)
			return nil // non-fatal — API refresh will follow
		}
		if len(notifications) == 0 {
			return nil // nothing cached yet
		}
		log.Printf("load cached: loaded %d notifications from cache", len(notifications))
		return cachedNotificationsLoadedMsg{notifications: notifications}
	}
}

// spinnerTickCmd returns a Cmd that sends a spinnerTickMsg after a short delay
// to animate the loading spinner in the preview pane.
func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// fullTextSearchCmd performs a Bleve full-text search in the background.
func fullTextSearchCmd(svc *service.NotificationService, query string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return searchResultsMsg{query: query}
		}
		results, err := svc.Search(query, 100)
		if err != nil {
			log.Printf("full-text search error: %v", err)
			return searchResultsMsg{query: query}
		}
		ids := make([]string, len(results))
		for i, r := range results {
			ids[i] = r.ThreadID
		}
		log.Printf("full-text search for %q: %d results", query, len(ids))
		return searchResultsMsg{threadIDs: ids, query: query}
	}
}

// fetchCurrentUserCmd fetches the authenticated user's login once at startup.
func fetchCurrentUserCmd(client github.NotificationAPI) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		login, err := client.GetCurrentUser()
		if err != nil {
			log.Printf("fetch current user: %v", err)
			return nil
		}
		log.Printf("current user: %s", login)
		return currentUserMsg{login: login}
	}
}
