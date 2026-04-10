package scoring

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/cassiomarques/gh-bell/internal/github"
)

func makeNotification(id, reason, subjectType string, updatedAt time.Time) github.Notification {
	return github.Notification{
		ID:        id,
		Unread:    true,
		Reason:    reason,
		UpdatedAt: updatedAt,
		Subject: github.Subject{
			Title: "Test notification " + id,
			Type:  subjectType,
		},
		Repository: github.Repository{FullName: "acme/app"},
	}
}

func TestComputeAction_SecurityAlert(t *testing.T) {
	n := makeNotification("1", "security_alert", "RepositoryVulnerabilityAlert", time.Now())
	action := ComputeAction(n, nil, "myuser")
	if action != ActionSecurityAlert {
		t.Errorf("expected SecurityAlert, got %s", action)
	}
}

func TestComputeAction_NoDetail_FallbackToReason(t *testing.T) {
	tests := []struct {
		reason string
		want   ActionReason
	}{
		{"review_requested", ActionReviewRequested},
		{"assign", ActionAssigned},
		{"mention", ActionMentioned},
		{"team_mention", ActionMentioned},
		{"ci_activity", ActionCIPending},
		{"author", ActionWaitingForReview},
		{"subscribed", ActionSubscribed},
		{"state_change", ActionSubscribed},
	}
	for _, tt := range tests {
		n := makeNotification("1", tt.reason, "PullRequest", time.Now())
		got := ComputeAction(n, nil, "myuser")
		if got != tt.want {
			t.Errorf("reason=%q: got %s, want %s", tt.reason, got, tt.want)
		}
	}
}

func TestComputeAction_AuthoredPR_Merged(t *testing.T) {
	n := makeNotification("1", "author", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State:  "closed",
		Merged: true,
		User:   github.User{Login: "myuser"},
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionMerged {
		t.Errorf("expected Merged, got %s", action)
	}
}

func TestComputeAction_AuthoredPR_Closed(t *testing.T) {
	n := makeNotification("1", "author", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State: "closed",
		User:  github.User{Login: "myuser"},
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionClosed {
		t.Errorf("expected Closed, got %s", action)
	}
}

func TestComputeAction_AuthoredPR_Draft(t *testing.T) {
	n := makeNotification("1", "author", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State: "open",
		Draft: true,
		User:  github.User{Login: "myuser"},
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionDraft {
		t.Errorf("expected Draft, got %s", action)
	}
}

func TestComputeAction_AuthoredPR_WaitingForReview(t *testing.T) {
	n := makeNotification("1", "author", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State: "open",
		User:  github.User{Login: "myuser"},
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionWaitingForReview {
		t.Errorf("expected WaitingForReview, got %s", action)
	}
}

func TestComputeAction_AuthoredPR_ChangesRequested(t *testing.T) {
	n := makeNotification("1", "author", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State:          "open",
		User:           github.User{Login: "myuser"},
		ReviewDecision: "CHANGES_REQUESTED",
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionChangesRequested {
		t.Errorf("expected ChangesRequested, got %s", action)
	}
}

func TestComputeAction_AuthoredPR_CIFailed(t *testing.T) {
	n := makeNotification("1", "author", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State:    "open",
		User:     github.User{Login: "myuser"},
		CIStatus: "failure",
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionCIFailed {
		t.Errorf("expected CIFailed, got %s", action)
	}
}

func TestComputeAction_AuthoredPR_MergeConflicts(t *testing.T) {
	n := makeNotification("1", "author", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State:     "open",
		User:      github.User{Login: "myuser"},
		Mergeable: "CONFLICTING",
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionMergeConflicts {
		t.Errorf("expected MergeConflicts, got %s", action)
	}
}

func TestComputeAction_AuthoredPR_ReadyToMerge(t *testing.T) {
	n := makeNotification("1", "author", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State:          "open",
		User:           github.User{Login: "myuser"},
		ReviewDecision: "APPROVED",
		CIStatus:       "success",
		Mergeable:      "MERGEABLE",
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionReadyToMerge {
		t.Errorf("expected ReadyToMerge, got %s", action)
	}
}

func TestComputeAction_AuthoredPR_Approved(t *testing.T) {
	n := makeNotification("1", "author", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State:          "open",
		User:           github.User{Login: "myuser"},
		ReviewDecision: "APPROVED",
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionApproved {
		t.Errorf("expected Approved, got %s", action)
	}
}

func TestComputeAction_AuthoredPR_ApprovedCIPending(t *testing.T) {
	n := makeNotification("1", "author", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State:          "open",
		User:           github.User{Login: "myuser"},
		ReviewDecision: "APPROVED",
		CIStatus:       "pending",
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionCIPending {
		t.Errorf("expected CIPending, got %s", action)
	}
}

func TestComputeAction_ReviewerPR_ReviewRequested(t *testing.T) {
	n := makeNotification("1", "review_requested", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State:              "open",
		User:               github.User{Login: "otheruser"},
		RequestedReviewers: []github.User{{Login: "myuser"}},
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionReviewRequested {
		t.Errorf("expected ReviewRequested, got %s", action)
	}
}

func TestComputeAction_ReviewerPR_ReviewRequestedCaseInsensitive(t *testing.T) {
	n := makeNotification("1", "review_requested", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State:              "open",
		User:               github.User{Login: "otheruser"},
		RequestedReviewers: []github.User{{Login: "MyUser"}},
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionReviewRequested {
		t.Errorf("expected ReviewRequested, got %s", action)
	}
}

func TestComputeAction_ReviewerPR_Merged(t *testing.T) {
	n := makeNotification("1", "review_requested", "PullRequest", time.Now())
	detail := &github.ThreadDetail{
		State:  "closed",
		Merged: true,
		User:   github.User{Login: "otheruser"},
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionMerged {
		t.Errorf("expected Merged, got %s", action)
	}
}

func TestComputeAction_Issue_Assigned(t *testing.T) {
	n := makeNotification("1", "assign", "Issue", time.Now())
	detail := &github.ThreadDetail{
		State:     "open",
		User:      github.User{Login: "otheruser"},
		Assignees: []github.User{{Login: "myuser"}},
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionAssigned {
		t.Errorf("expected Assigned, got %s", action)
	}
}

func TestComputeAction_Issue_Closed(t *testing.T) {
	n := makeNotification("1", "mention", "Issue", time.Now())
	detail := &github.ThreadDetail{
		State: "closed",
		User:  github.User{Login: "otheruser"},
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionClosed {
		t.Errorf("expected Closed, got %s", action)
	}
}

func TestComputeAction_Issue_Mentioned(t *testing.T) {
	n := makeNotification("1", "mention", "Issue", time.Now())
	detail := &github.ThreadDetail{
		State: "open",
		User:  github.User{Login: "otheruser"},
	}
	action := ComputeAction(n, detail, "myuser")
	if action != ActionMentioned {
		t.Errorf("expected Mentioned, got %s", action)
	}
}

// --- Score computation tests ---

func TestComputeScore_HigherPriorityGetsHigherScore(t *testing.T) {
	now := time.Now()
	updated := now.Add(-1 * time.Hour)

	secScore := ComputeScore(ActionSecurityAlert, updated, now)
	conflictScore := ComputeScore(ActionMergeConflicts, updated, now)
	reviewScore := ComputeScore(ActionReviewRequested, updated, now)
	mergedScore := ComputeScore(ActionMerged, updated, now)
	subScore := ComputeScore(ActionSubscribed, updated, now)

	if secScore <= conflictScore {
		t.Errorf("security (%f) should score higher than conflicts (%f)", secScore, conflictScore)
	}
	if conflictScore <= reviewScore {
		t.Errorf("conflicts (%f) should score higher than review_requested (%f)", conflictScore, reviewScore)
	}
	if reviewScore <= mergedScore {
		t.Errorf("review_requested (%f) should score higher than merged (%f)", reviewScore, mergedScore)
	}
	if mergedScore <= subScore {
		t.Errorf("merged (%f) should score higher than subscribed (%f)", mergedScore, subScore)
	}
}

func TestComputeScore_DecaysOverTime(t *testing.T) {
	now := time.Now()
	recent := ComputeScore(ActionReviewRequested, now.Add(-1*time.Hour), now)
	old := ComputeScore(ActionReviewRequested, now.Add(-48*time.Hour), now)

	if recent <= old {
		t.Errorf("recent (%f) should score higher than old (%f)", recent, old)
	}
}

func TestComputeScore_HalfLifeHalvesScore(t *testing.T) {
	now := time.Now()
	// ReviewRequested has halfLife = twoDays = 48h
	fresh := ComputeScore(ActionReviewRequested, now, now)
	halfDecayed := ComputeScore(ActionReviewRequested, now.Add(-48*time.Hour), now)

	ratio := halfDecayed / fresh
	if math.Abs(ratio-0.5) > 0.01 {
		t.Errorf("score at half-life should be ~50%% of fresh, got ratio %f", ratio)
	}
}

func TestComputeScore_UnknownActionReturnsZero(t *testing.T) {
	score := ComputeScore(ActionNone, time.Now(), time.Now())
	if score != 0 {
		t.Errorf("unknown action should score 0, got %f", score)
	}
}

func TestComputeScore_FutureUpdatedAtDoesNotGoBelowZero(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	score := ComputeScore(ActionReviewRequested, future, now)
	if score < 0 {
		t.Errorf("score should not be negative for future timestamps, got %f", score)
	}
}

// --- ScoreAll tests ---

func TestScoreAll_SortsCorrectlyByScore(t *testing.T) {
	now := time.Now()
	notifications := []github.Notification{
		makeNotification("1", "subscribed", "PullRequest", now.Add(-1*time.Hour)),
		makeNotification("2", "review_requested", "PullRequest", now.Add(-1*time.Hour)),
		makeNotification("3", "mention", "Issue", now.Add(-1*time.Hour)),
	}

	scored := ScoreAll(notifications, nil, "myuser", now)

	// Sort by score descending (as the TUI would)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// review_requested should be first (highest priority)
	if scored[0].Notification.ID != "2" {
		t.Errorf("expected review_requested (id=2) first, got id=%s (action=%s)", scored[0].Notification.ID, scored[0].Action)
	}
	// mention should be second
	if scored[1].Notification.ID != "3" {
		t.Errorf("expected mention (id=3) second, got id=%s (action=%s)", scored[1].Notification.ID, scored[1].Action)
	}
	// subscribed should be last
	if scored[2].Notification.ID != "1" {
		t.Errorf("expected subscribed (id=1) last, got id=%s (action=%s)", scored[2].Notification.ID, scored[2].Action)
	}
}

func TestScoreAll_WithDetails(t *testing.T) {
	now := time.Now()
	notifications := []github.Notification{
		makeNotification("1", "author", "PullRequest", now.Add(-1*time.Hour)),
		makeNotification("2", "author", "PullRequest", now.Add(-1*time.Hour)),
	}
	details := map[string]*github.ThreadDetail{
		"1": {State: "open", User: github.User{Login: "myuser"}, Draft: true},
		"2": {State: "closed", Merged: true, User: github.User{Login: "myuser"}},
	}

	scored := ScoreAll(notifications, details, "myuser", now)

	// Draft PR should score higher than merged (draft decays slower)
	if scored[0].Action != ActionDraft {
		t.Errorf("notification 1 should be Draft, got %s", scored[0].Action)
	}
	if scored[1].Action != ActionMerged {
		t.Errorf("notification 2 should be Merged, got %s", scored[1].Action)
	}
}

// --- Action label tests ---

func TestActionReasonLabel(t *testing.T) {
	tests := []struct {
		action ActionReason
		want   string
	}{
		{ActionMergeConflicts, "Conflicts"},
		{ActionCIFailed, "CI failed"},
		{ActionReadyToMerge, "Ready to merge"},
		{ActionChangesRequested, "Changes req'd"},
		{ActionApproved, "Approved"},
		{ActionReviewRequested, "Review req'd"},
		{ActionMerged, "Merged"},
		{ActionClosed, "Closed"},
		{ActionSecurityAlert, "Security"},
		{ActionNone, ""},
	}
	for _, tt := range tests {
		got := tt.action.Label()
		if got != tt.want {
			t.Errorf("%s.Label() = %q, want %q", tt.action, got, tt.want)
		}
	}
}
