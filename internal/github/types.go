package github

import "time"

// Notification represents a GitHub notification from the REST API.
type Notification struct {
	ID         string     `json:"id"`
	Unread     bool       `json:"unread"`
	Reason     string     `json:"reason"`
	UpdatedAt  time.Time  `json:"updated_at"`
	URL        string     `json:"url"`
	Subject    Subject    `json:"subject"`
	Repository Repository `json:"repository"`
}

// Subject is the item (issue, PR, release, etc.) that triggered the notification.
type Subject struct {
	Title            string `json:"title"`
	URL              string `json:"url"`
	LatestCommentURL string `json:"latest_comment_url"`
	Type             string `json:"type"`
}

// Repository identifies which repo the notification belongs to.
type Repository struct {
	ID       int    `json:"id"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
	Private  bool   `json:"private"`
	Owner    Owner  `json:"owner"`
}

// Owner is the repository owner.
type Owner struct {
	Login string `json:"login"`
}

// ThreadSubscription represents the user's subscription to a notification thread.
type ThreadSubscription struct {
	Subscribed bool      `json:"subscribed"`
	Ignored    bool      `json:"ignored"`
	CreatedAt  time.Time `json:"created_at"`
}

// ThreadDetail holds enriched data fetched lazily for the preview pane.
type ThreadDetail struct {
	// From subject URL (issue/PR/release)
	State     string   `json:"state"`
	Body      string   `json:"body"`
	Labels    []Label  `json:"labels"`
	User      User     `json:"user"`
	Assignees []User   `json:"assignees"`
	Draft     bool     `json:"draft"`
	Merged    bool     `json:"merged"`
	MergedBy  *User    `json:"merged_by"`
	HTMLURL   string   `json:"html_url"`
	// PR-specific
	Additions          int         `json:"additions"`
	Deletions          int         `json:"deletions"`
	RequestedReviewers []User      `json:"requested_reviewers"`
	RequestedTeams     []Team      `json:"requested_teams"`
	Milestone          *Milestone  `json:"milestone"`
	// Release-specific
	TagName string `json:"tag_name"`

	// From latest_comment_url (fetched separately)
	LatestComment *Comment
}

// Label represents a GitHub label.
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// User represents a GitHub user.
type User struct {
	Login string `json:"login"`
}

// Team represents a GitHub team.
type Team struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Milestone represents a GitHub milestone.
type Milestone struct {
	Title string `json:"title"`
}

// Comment represents a GitHub issue/PR comment.
type Comment struct {
	Body      string    `json:"body"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

// WebURL converts a GitHub API URL to a browser-friendly web URL.
// e.g. "https://api.github.com/repos/owner/repo/issues/42"
//    → "https://github.com/owner/repo/issues/42"
func (n Notification) WebURL() string {
	return apiURLToWebURL(n.Subject.URL)
}

// Icon returns a display character for the notification subject type.
func (n Notification) Icon() string {
	switch n.Subject.Type {
	case "Issue":
		return "I"
	case "PullRequest":
		return "P"
	case "Release":
		return "R"
	case "Discussion":
		return "D"
	case "CheckSuite":
		return "C"
	case "RepositoryVulnerabilityAlert":
		return "!"
	default:
		return "*"
	}
}

// ReasonLabel returns a human-readable short label for the notification reason.
func (n Notification) ReasonLabel() string {
	switch n.Reason {
	case "review_requested":
		return "review"
	case "state_change":
		return "state"
	case "ci_activity":
		return "ci"
	case "team_mention":
		return "team"
	case "security_alert":
		return "security"
	default:
		return n.Reason
	}
}
