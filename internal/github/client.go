package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// IsAuthError returns true if the error indicates an authentication failure
// (HTTP 401), typically caused by an expired or invalid token.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 401") || strings.Contains(s, "Bad credentials")
}

// IsServerError returns true if the error indicates a GitHub server-side issue (5xx).
func IsServerError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 502") || strings.Contains(s, "HTTP 503") ||
		strings.Contains(s, "HTTP 504")
}

// View controls which notifications to fetch.
type View int

const (
	ViewUnread       View = iota // only unread (default)
	ViewAll                      // read + unread
	ViewParticipating            // only where user is directly involved
)

// ListOptions configures the notification list request.
type ListOptions struct {
	View    View
	PerPage int
	Page    int
	Since   *time.Time
}

// Client wraps the GitHub REST API for notification operations.
type Client struct {
	rest *api.RESTClient
}

// NewClient creates a Client. It checks GH_BELL_TOKEN first (for users who
// prefer a classic PAT), then falls back to the default go-gh auth chain
// (GH_TOKEN → GITHUB_TOKEN → gh auth login keyring).
func NewClient() (*Client, error) {
	var rest *api.RESTClient
	var err error

	if token := os.Getenv("GH_BELL_TOKEN"); token != "" {
		rest, err = api.NewRESTClient(api.ClientOptions{AuthToken: token})
		if err != nil {
			return nil, fmt.Errorf("creating client with GH_BELL_TOKEN: %w", err)
		}
	} else {
		rest, err = api.DefaultRESTClient()
		if err != nil {
			return nil, fmt.Errorf("creating GitHub API client (is gh authenticated?): %w", err)
		}
	}
	return &Client{rest: rest}, nil
}

// ListNotifications fetches notifications according to the given options.
func (c *Client) ListNotifications(opts ListOptions) ([]Notification, error) {
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 50
	}
	page := opts.Page
	if page <= 0 {
		page = 1
	}

	endpoint := fmt.Sprintf("notifications?per_page=%d&page=%d", perPage, page)

	switch opts.View {
	case ViewAll:
		endpoint += "&all=true"
	case ViewParticipating:
		endpoint += "&participating=true"
	}

	if opts.Since != nil {
		endpoint += "&since=" + opts.Since.Format(time.RFC3339)
	}

	var notifications []Notification
	err := c.rest.Get(endpoint, &notifications)
	if err != nil {
		return nil, fmt.Errorf("fetching notifications: %w", err)
	}
	return notifications, nil
}

// MarkThreadRead marks a single notification thread as read.
func (c *Client) MarkThreadRead(threadID string) error {
	endpoint := fmt.Sprintf("notifications/threads/%s", threadID)
	err := c.rest.Patch(endpoint, nil, nil)
	if err != nil {
		return fmt.Errorf("marking thread %s as read: %w", threadID, err)
	}
	return nil
}

// MarkAllRead marks all notifications as read up to the given time.
// If upTo is nil, marks everything as read.
func (c *Client) MarkAllRead(upTo *time.Time) error {
	body := map[string]string{}
	if upTo != nil {
		body["last_read_at"] = upTo.Format(time.RFC3339)
	}
	encoded, err := jsonBody(body)
	if err != nil {
		return err
	}
	err = c.rest.Put("notifications", encoded, nil)
	if err != nil {
		return fmt.Errorf("marking all notifications as read: %w", err)
	}
	return nil
}

// MuteThread sets a thread subscription to ignored, silencing future notifications.
func (c *Client) MuteThread(threadID string) error {
	endpoint := fmt.Sprintf("notifications/threads/%s/subscription", threadID)
	body := map[string]bool{"ignored": true}
	encoded, err := jsonBody(body)
	if err != nil {
		return err
	}
	err = c.rest.Put(endpoint, encoded, nil)
	if err != nil {
		return fmt.Errorf("muting thread %s: %w", threadID, err)
	}
	return nil
}

// jsonBody encodes v as JSON and returns a reader suitable for REST calls.
func jsonBody(v any) (*bytes.Reader, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding request body: %w", err)
	}
	return bytes.NewReader(b), nil
}

// UnsubscribeThread removes the user's subscription from a thread.
func (c *Client) UnsubscribeThread(threadID string) error {
	endpoint := fmt.Sprintf("notifications/threads/%s/subscription", threadID)
	err := c.rest.Delete(endpoint, nil)
	if err != nil {
		return fmt.Errorf("unsubscribing from thread %s: %w", threadID, err)
	}
	return nil
}
