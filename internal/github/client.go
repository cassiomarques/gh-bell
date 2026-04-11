package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// NotificationAPI defines the interface for GitHub notification operations.
// The concrete Client implements this; tests can substitute a FakeClient.
type NotificationAPI interface {
	ListNotifications(opts ListOptions) (ListResult, error)
	MarkThreadRead(threadID string) error
	MarkThreadDone(threadID string) error
	MarkAllRead(upTo *time.Time) error
	MuteThread(threadID string) error
	UnsubscribeThread(threadID string) error
	FetchThreadDetail(subjectURL, commentURL string) (*ThreadDetail, error)
	GetCurrentUser() (string, error)
	EnrichPRsBatch(prs []PRRef) (map[string]*PREnrichment, error)
}

// ListResult holds the notifications returned by a single API page along with
// pagination metadata parsed from the Link response header.
type ListResult struct {
	Notifications []Notification
	HasNextPage   bool
}

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
	rest  *api.RESTClient
	token string
}

// NewClient creates a Client using the provided token. If token is empty,
// it falls back to the default go-gh auth chain
// (GH_TOKEN → GITHUB_TOKEN → gh auth login keyring).
func NewClient(token string) (*Client, error) {
	var rest *api.RESTClient
	var err error

	if token != "" {
		rest, err = api.NewRESTClient(api.ClientOptions{AuthToken: token})
		if err != nil {
			return nil, fmt.Errorf("creating client with token: %w", err)
		}
	} else {
		rest, err = api.DefaultRESTClient()
		if err != nil {
			return nil, fmt.Errorf("creating GitHub API client (is gh authenticated?): %w", err)
		}
	}
	return &Client{rest: rest, token: token}, nil
}

// ListNotifications fetches notifications according to the given options.
// It uses Request() instead of Get() to access the Link response header for
// reliable pagination — the GitHub API can return fewer items than per_page
// even when more pages exist.
func (c *Client) ListNotifications(opts ListOptions) (ListResult, error) {
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

	resp, err := c.rest.Request(http.MethodGet, endpoint, nil)
	if err != nil {
		return ListResult{}, fmt.Errorf("fetching notifications: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ListResult{}, fmt.Errorf("reading notification response: %w", err)
	}

	var notifications []Notification
	if err := json.Unmarshal(body, &notifications); err != nil {
		return ListResult{}, fmt.Errorf("decoding notifications: %w", err)
	}

	linkHeader := resp.Header.Get("Link")
	hasNext := hasNextPage(linkHeader)

	return ListResult{
		Notifications: notifications,
		HasNextPage:   hasNext,
	}, nil
}

// hasNextPage parses the Link header to check for a rel="next" link.
// Example: <https://api.github.com/...?page=2>; rel="next", <...>; rel="last"
func hasNextPage(linkHeader string) bool {
	if linkHeader == "" {
		return false
	}
	for _, part := range strings.Split(linkHeader, ",") {
		if strings.Contains(part, `rel="next"`) {
			return true
		}
	}
	return false
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

// MarkThreadDone dismisses a notification thread ("Done" in GitHub's UI).
// Unlike MarkThreadRead which only dims the notification, this removes it
// from the notification list entirely. A new notification is created if
// there is new activity on the thread later.
func (c *Client) MarkThreadDone(threadID string) error {
	endpoint := fmt.Sprintf("notifications/threads/%s", threadID)
	err := c.rest.Delete(endpoint, nil)
	if err != nil {
		return fmt.Errorf("marking thread %s as done: %w", threadID, err)
	}
	return nil
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

// FetchThreadDetail fetches enriched details for a notification by calling
// the subject URL (issue/PR/release) and optionally the latest comment URL.
// The subject URL is an API URL like "https://api.github.com/repos/owner/repo/issues/42"
// which we strip to a relative path for the REST client.
func (c *Client) FetchThreadDetail(subjectURL, commentURL string) (*ThreadDetail, error) {
	detail := &ThreadDetail{}

	if subjectURL != "" {
		endpoint := stripAPIHost(subjectURL)
		if endpoint != "" {
			if err := c.rest.Get(endpoint, detail); err != nil {
				return nil, fmt.Errorf("fetching subject detail: %w", err)
			}
		}
	}

	if commentURL != "" {
		endpoint := stripAPIHost(commentURL)
		if endpoint != "" {
			var comment Comment
			if err := c.rest.Get(endpoint, &comment); err == nil {
				detail.LatestComment = &comment
			}
			// Non-fatal: we still have subject detail even if comment fetch fails
		}
	}

	return detail, nil
}

// stripAPIHost removes the "https://api.github.com/" prefix to get a
// relative endpoint path for the REST client.
func stripAPIHost(url string) string {
	const prefix = "https://api.github.com/"
	if strings.HasPrefix(url, prefix) {
		return url[len(prefix):]
	}
	return ""
}

// GetCurrentUser fetches the authenticated user's login name.
func (c *Client) GetCurrentUser() (string, error) {
	var user User
	if err := c.rest.Get("user", &user); err != nil {
		return "", fmt.Errorf("fetching current user: %w", err)
	}
	return user.Login, nil
}
