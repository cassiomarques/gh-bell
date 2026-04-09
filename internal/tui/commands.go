package tui

import (
	"fmt"
	"log"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cassiomarques/gh-bell/internal/github"
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
	if os.Getenv("GH_BELL_TOKEN") != "" {
		return fmt.Errorf("token expired or invalid — regenerate your classic PAT at "+
			"https://github.com/settings/tokens and update GH_BELL_TOKEN")
	}
	return fmt.Errorf("authentication failed — try: gh auth login, "+
		"or set GH_BELL_TOKEN to a classic PAT")
}

// fetchNotificationsCmd returns a Cmd that fetches notifications from the API.
// Retries up to maxRetries times on transient server errors with backoff.
// Auth errors (expired/invalid token) are returned immediately without retry.
func fetchNotificationsCmd(client *github.Client, view github.View) tea.Cmd {
	return func() tea.Msg {
		var lastErr error
		for attempt := range maxRetries {
			if attempt > 0 {
				log.Printf("fetch notifications: retry %d/%d after error: %v", attempt+1, maxRetries, lastErr)
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
			}
			log.Printf("fetch notifications: attempt %d, view=%d", attempt+1, view)
			notifications, err := client.ListNotifications(github.ListOptions{
				View:    view,
				PerPage: 50,
			})
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
					"Try: GH_BELL_TOKEN=ghp_your_classic_pat gh bell", lastErr)}
		}
		return errorMsg{err: lastErr}
	}
}

// markReadCmd returns a Cmd that marks a single thread as read.
func markReadCmd(client *github.Client, threadID string) tea.Cmd {
	return func() tea.Msg {
		if err := client.MarkThreadRead(threadID); err != nil {
			if github.IsAuthError(err) {
				return errorMsg{err: authErrorMessage(err)}
			}
			return errorMsg{err: err}
		}
		return threadMarkedReadMsg{threadID: threadID}
	}
}

// markAllReadCmd returns a Cmd that marks all notifications as read.
func markAllReadCmd(client *github.Client) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		if err := client.MarkAllRead(&now); err != nil {
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
func muteThreadCmd(client *github.Client, threadID string) tea.Cmd {
	return func() tea.Msg {
		if err := client.MuteThread(threadID); err != nil {
			if github.IsAuthError(err) {
				return errorMsg{err: authErrorMessage(err)}
			}
			return errorMsg{err: err}
		}
		// Mark as read so it doesn't reappear on refresh
		if err := client.MarkThreadRead(threadID); err != nil {
			log.Printf("mute: thread muted but failed to mark read: %v", err)
		}
		return threadMutedMsg{threadID: threadID}
	}
}

// unsubscribeCmd returns a Cmd that unsubscribes from a notification thread.
// Also marks the thread as read so it disappears from the unread list.
func unsubscribeCmd(client *github.Client, threadID string) tea.Cmd {
	return func() tea.Msg {
		if err := client.UnsubscribeThread(threadID); err != nil {
			if github.IsAuthError(err) {
				return errorMsg{err: authErrorMessage(err)}
			}
			return errorMsg{err: err}
		}
		if err := client.MarkThreadRead(threadID); err != nil {
			log.Printf("unsubscribe: unsubscribed but failed to mark read: %v", err)
		}
		return threadUnsubscribedMsg{threadID: threadID}
	}
}

// refreshTickCmd returns a Cmd that sends a refreshTickMsg after an interval.
func refreshTickCmd() tea.Cmd {
	return tea.Tick(defaultRefreshInterval, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

// clearStatusCmd returns a Cmd that clears the status bar after a delay.
func clearStatusCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}
