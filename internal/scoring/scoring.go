// Package scoring computes actionability and priority scores for GitHub
// notifications. The score determines sort order in "smart" mode — higher
// scores float to the top of the list.
//
// The scoring model is inspired by github.com/BlakeWilliams/ghq.
package scoring

import (
	"math"
	"strings"
	"time"

	"github.com/cassiomarques/gh-bell/internal/github"
)

// ActionReason describes why a notification needs (or doesn't need) attention.
type ActionReason string

const (
	// Author actions (I created the PR/issue)
	ActionMergeConflicts   ActionReason = "merge_conflicts"
	ActionCIFailed         ActionReason = "ci_failed"
	ActionReadyToMerge     ActionReason = "ready_to_merge"
	ActionChangesRequested ActionReason = "changes_requested"
	ActionApproved         ActionReason = "approved"
	ActionCIPending        ActionReason = "ci_pending"
	ActionWaitingForReview ActionReason = "waiting_for_review"
	ActionDraft            ActionReason = "draft"

	// Reviewer/participant actions
	ActionReReviewRequested ActionReason = "re_review_requested"
	ActionReviewRequested   ActionReason = "review_requested"
	ActionMentioned         ActionReason = "mentioned"
	ActionAssigned          ActionReason = "assigned"

	// Informational
	ActionMerged         ActionReason = "merged"
	ActionClosed         ActionReason = "closed"
	ActionSecurityAlert  ActionReason = "security_alert"
	ActionSubscribed     ActionReason = "subscribed"
	ActionNone           ActionReason = ""
)

// ActionLabel returns a human-readable short label for display in the TUI.
func (a ActionReason) Label() string {
	switch a {
	case ActionMergeConflicts:
		return "Conflicts"
	case ActionCIFailed:
		return "CI failed"
	case ActionReadyToMerge:
		return "Ready to merge"
	case ActionChangesRequested:
		return "Changes req'd"
	case ActionApproved:
		return "Approved"
	case ActionCIPending:
		return "CI pending"
	case ActionWaitingForReview:
		return "Waiting review"
	case ActionDraft:
		return "Draft"
	case ActionReReviewRequested:
		return "Re-review"
	case ActionReviewRequested:
		return "Review req'd"
	case ActionMentioned:
		return "Mentioned"
	case ActionAssigned:
		return "Assigned"
	case ActionMerged:
		return "Merged"
	case ActionClosed:
		return "Closed"
	case ActionSecurityAlert:
		return "Security"
	case ActionSubscribed:
		return "Subscribed"
	default:
		return string(a)
	}
}

// actionConfig holds the weight and half-life for a given action reason.
type actionConfig struct {
	priority int
	halfLife time.Duration
}

const (
	halfDay   = 12 * time.Hour
	oneDay    = 24 * time.Hour
	twoDays   = 48 * time.Hour
	threeDays = 72 * time.Hour
	fourDays  = 96 * time.Hour
	oneWeek   = 168 * time.Hour
)

// configs maps each action reason to its priority and half-life.
//
// Priority determines the base weight: weight = 100 * 0.8^priority.
// Lower priority number = higher weight = more urgent.
//
// Half-life controls how fast the score decays over time.
// At age = half-life, the score is halved. Shorter half-lives
// cause items to drop off the list faster.
var configs = map[ActionReason]actionConfig{
	ActionSecurityAlert:     {priority: 0, halfLife: oneWeek},
	ActionMergeConflicts:    {priority: 1, halfLife: oneWeek},
	ActionCIFailed:          {priority: 2, halfLife: oneWeek},
	ActionReadyToMerge:      {priority: 3, halfLife: twoDays},
	ActionChangesRequested:  {priority: 4, halfLife: threeDays},
	ActionApproved:          {priority: 5, halfLife: threeDays},
	ActionReReviewRequested: {priority: 6, halfLife: twoDays},
	ActionReviewRequested:   {priority: 7, halfLife: twoDays},
	ActionAssigned:          {priority: 8, halfLife: threeDays},
	ActionCIPending:         {priority: 9, halfLife: oneDay},
	ActionWaitingForReview:  {priority: 10, halfLife: fourDays},
	ActionMentioned:         {priority: 11, halfLife: oneDay},
	ActionDraft:             {priority: 12, halfLife: oneWeek},
	ActionMerged:            {priority: 13, halfLife: halfDay},
	ActionClosed:            {priority: 14, halfLife: halfDay},
	ActionSubscribed:        {priority: 15, halfLife: halfDay},
}

// ComputeAction determines the actionability reason for a notification.
// It uses the notification metadata, enriched thread detail (may be nil),
// and the current user's login.
func ComputeAction(n github.Notification, detail *github.ThreadDetail, currentUser string) ActionReason {
	// Security alerts are always top priority
	if n.Subject.Type == "RepositoryVulnerabilityAlert" {
		return ActionSecurityAlert
	}

	// Without detail, fall back to notification reason
	if detail == nil {
		return actionFromReason(n.Reason)
	}

	isAuthor := strings.EqualFold(detail.User.Login, currentUser)

	if n.Subject.Type == "PullRequest" {
		if isAuthor {
			return computeAuthoredPRAction(detail)
		}
		return computeReviewerPRAction(n, detail, currentUser)
	}

	// Issues and other types: simpler model
	return computeIssueAction(n, detail, currentUser)
}

// computeAuthoredPRAction determines the action for a PR the user authored.
func computeAuthoredPRAction(d *github.ThreadDetail) ActionReason {
	if d.State == "closed" && !d.Merged {
		return ActionClosed
	}
	if d.Merged {
		return ActionMerged
	}

	// GraphQL fields (Phase 2 — these will be empty strings until then)
	if d.ReviewDecision == "CHANGES_REQUESTED" {
		return ActionChangesRequested
	}
	if d.CIStatus == "failure" || d.CIStatus == "error" {
		return ActionCIFailed
	}
	if d.Mergeable == "CONFLICTING" {
		return ActionMergeConflicts
	}
	if d.ReviewDecision == "APPROVED" && d.CIStatus == "success" && d.Mergeable == "MERGEABLE" {
		return ActionReadyToMerge
	}
	if d.ReviewDecision == "APPROVED" {
		if d.CIStatus == "pending" {
			return ActionCIPending
		}
		return ActionApproved
	}
	if d.CIStatus == "pending" {
		return ActionCIPending
	}
	if d.Draft {
		return ActionDraft
	}
	return ActionWaitingForReview
}

// computeReviewerPRAction determines the action for a PR where the user is a reviewer.
func computeReviewerPRAction(n github.Notification, d *github.ThreadDetail, currentUser string) ActionReason {
	if d.Merged {
		return ActionMerged
	}
	if d.State == "closed" {
		return ActionClosed
	}

	// Check if review is explicitly requested from me
	for _, u := range d.RequestedReviewers {
		if strings.EqualFold(u.Login, currentUser) {
			return ActionReviewRequested
		}
	}

	// Fall back to notification reason
	return actionFromReason(n.Reason)
}

// computeIssueAction determines the action for issues and other notification types.
func computeIssueAction(n github.Notification, d *github.ThreadDetail, currentUser string) ActionReason {
	if d.State == "closed" {
		return ActionClosed
	}

	// Check assignees
	for _, u := range d.Assignees {
		if strings.EqualFold(u.Login, currentUser) {
			return ActionAssigned
		}
	}

	return actionFromReason(n.Reason)
}

// actionFromReason maps a raw GitHub notification reason to an ActionReason.
func actionFromReason(reason string) ActionReason {
	switch reason {
	case "review_requested":
		return ActionReviewRequested
	case "assign":
		return ActionAssigned
	case "mention":
		return ActionMentioned
	case "team_mention":
		return ActionMentioned
	case "security_alert":
		return ActionSecurityAlert
	case "ci_activity":
		return ActionCIPending
	case "author":
		return ActionWaitingForReview
	case "subscribed":
		return ActionSubscribed
	default:
		return ActionSubscribed
	}
}

// ComputeScore calculates the priority score with time decay.
// Higher score = higher priority. Pass the current time for testability.
func ComputeScore(action ActionReason, updatedAt time.Time, now time.Time) float64 {
	cfg, ok := configs[action]
	if !ok {
		return 0
	}

	weight := 100.0 * math.Pow(0.8, float64(cfg.priority))
	age := now.Sub(updatedAt)
	if age < 0 {
		age = 0
	}
	ageHours := age.Hours()
	halfLifeHours := cfg.halfLife.Hours()

	return weight * math.Pow(2, -ageHours/halfLifeHours)
}

// ScoredNotification pairs a notification with its computed action and score.
type ScoredNotification struct {
	Notification github.Notification
	Action       ActionReason
	Score        float64
}

// ScoreAll computes action and score for a slice of notifications.
// detail may be nil for any notification (falls back to reason-based scoring).
func ScoreAll(notifications []github.Notification, details map[string]*github.ThreadDetail, currentUser string, now time.Time) []ScoredNotification {
	result := make([]ScoredNotification, len(notifications))
	for i, n := range notifications {
		var detail *github.ThreadDetail
		if details != nil {
			detail = details[n.ID]
		}
		action := ComputeAction(n, detail, currentUser)
		score := ComputeScore(action, n.UpdatedAt, now)
		result[i] = ScoredNotification{
			Notification: n,
			Action:       action,
			Score:        score,
		}
	}
	return result
}
