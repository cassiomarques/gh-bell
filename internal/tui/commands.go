package tui

import (
	"log"
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

// fetchNotificationsCmd returns a Cmd that fetches notifications from the API.
// Retries up to maxRetries times on transient errors (5xx) with backoff.
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
		}
		log.Printf("fetch notifications: all %d attempts failed: %v", maxRetries, lastErr)
		return errorMsg{err: lastErr}
	}
}

// markReadCmd returns a Cmd that marks a single thread as read.
func markReadCmd(client *github.Client, threadID string) tea.Cmd {
	return func() tea.Msg {
		if err := client.MarkThreadRead(threadID); err != nil {
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
			return errorMsg{err: err}
		}
		return allMarkedReadMsg{}
	}
}

// muteThreadCmd returns a Cmd that mutes a notification thread.
func muteThreadCmd(client *github.Client, threadID string) tea.Cmd {
	return func() tea.Msg {
		if err := client.MuteThread(threadID); err != nil {
			return errorMsg{err: err}
		}
		return threadMutedMsg{threadID: threadID}
	}
}

// unsubscribeCmd returns a Cmd that unsubscribes from a notification thread.
func unsubscribeCmd(client *github.Client, threadID string) tea.Cmd {
	return func() tea.Msg {
		if err := client.UnsubscribeThread(threadID); err != nil {
			return errorMsg{err: err}
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
