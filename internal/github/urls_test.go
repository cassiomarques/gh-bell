package github

import "testing"

func TestAPIURLToWebURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "issue",
			in:   "https://api.github.com/repos/cli/cli/issues/123",
			want: "https://github.com/cli/cli/issues/123",
		},
		{
			name: "pull request",
			in:   "https://api.github.com/repos/owner/repo/pulls/42",
			want: "https://github.com/owner/repo/pull/42",
		},
		{
			name: "release",
			in:   "https://api.github.com/repos/owner/repo/releases/99",
			want: "https://github.com/owner/repo/releases/99",
		},
		{
			name: "commit",
			in:   "https://api.github.com/repos/owner/repo/commits/abc123",
			want: "https://github.com/owner/repo/commits/abc123",
		},
		{
			name: "non-api URL passthrough",
			in:   "https://github.com/owner/repo/issues/1",
			want: "https://github.com/owner/repo/issues/1",
		},
		{
			name: "unrecognized URL passthrough",
			in:   "https://example.com/foo",
			want: "https://example.com/foo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := apiURLToWebURL(tc.in)
			if got != tc.want {
				t.Errorf("apiURLToWebURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNotificationWebURL(t *testing.T) {
	n := Notification{
		Subject: Subject{
			URL: "https://api.github.com/repos/cli/cli/pulls/100",
		},
	}
	want := "https://github.com/cli/cli/pull/100"
	if got := n.WebURL(); got != want {
		t.Errorf("WebURL() = %q, want %q", got, want)
	}
}

func TestNotificationIcon(t *testing.T) {
	tests := []struct {
		subjectType string
		want        string
	}{
		{"Issue", "I"},
		{"PullRequest", "P"},
		{"Release", "R"},
		{"Discussion", "D"},
		{"Unknown", "*"},
	}
	for _, tc := range tests {
		n := Notification{Subject: Subject{Type: tc.subjectType}}
		if got := n.Icon(); got != tc.want {
			t.Errorf("Icon() for %q = %q, want %q", tc.subjectType, got, tc.want)
		}
	}
}

func TestNotificationReasonLabel(t *testing.T) {
	tests := []struct {
		reason string
		want   string
	}{
		{"review_requested", "review"},
		{"state_change", "state"},
		{"ci_activity", "ci"},
		{"mention", "mention"},
		{"comment", "comment"},
	}
	for _, tc := range tests {
		n := Notification{Reason: tc.reason}
		if got := n.ReasonLabel(); got != tc.want {
			t.Errorf("ReasonLabel() for %q = %q, want %q", tc.reason, got, tc.want)
		}
	}
}
