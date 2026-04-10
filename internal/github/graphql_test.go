package github

import (
	"strings"
	"testing"
)

func TestParseSubjectURL(t *testing.T) {
	tests := []struct {
		url            string
		wantOwner      string
		wantRepo       string
		wantNumber     int
		wantOK         bool
	}{
		{
			url:        "https://api.github.com/repos/owner/repo/pulls/42",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 42,
			wantOK:     true,
		},
		{
			url:        "https://api.github.com/repos/my-org/my-repo/pulls/123",
			wantOwner:  "my-org",
			wantRepo:   "my-repo",
			wantNumber: 123,
			wantOK:     true,
		},
		{
			url:    "https://api.github.com/repos/owner/repo/issues/42",
			wantOK: false, // issues, not PRs
		},
		{
			url:    "https://github.com/owner/repo/pull/42",
			wantOK: false, // web URL, not API URL
		},
		{
			url:    "",
			wantOK: false,
		},
		{
			url:    "https://api.github.com/repos/owner/repo",
			wantOK: false, // no number
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo, number, ok := ParseSubjectURL(tt.url)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if number != tt.wantNumber {
				t.Errorf("number = %d, want %d", number, tt.wantNumber)
			}
		})
	}
}

func TestBuildBatchQuery(t *testing.T) {
	prs := []PRRef{
		{Owner: "org1", Repo: "repo1", Number: 10, ThreadID: "t1"},
		{Owner: "org2", Repo: "repo2", Number: 20, ThreadID: "t2"},
	}

	query := buildBatchQuery(prs)

	// Should contain aliases for each PR
	if !strings.Contains(query, "pr0: repository") {
		t.Error("query should contain pr0 alias")
	}
	if !strings.Contains(query, "pr1: repository") {
		t.Error("query should contain pr1 alias")
	}

	// Should have owner/repo/number for each
	if !strings.Contains(query, `"org1"`) {
		t.Error("query should contain org1 owner")
	}
	if !strings.Contains(query, `"repo2"`) {
		t.Error("query should contain repo2 repo")
	}
	if !strings.Contains(query, "number: 10") {
		t.Error("query should contain number 10")
	}
	if !strings.Contains(query, "number: 20") {
		t.Error("query should contain number 20")
	}

	// Should request the expected fields
	if !strings.Contains(query, "reviewDecision") {
		t.Error("query should contain reviewDecision")
	}
	if !strings.Contains(query, "mergeable") {
		t.Error("query should contain mergeable")
	}
	if !strings.Contains(query, "statusCheckRollup") {
		t.Error("query should contain statusCheckRollup")
	}
	if !strings.Contains(query, "committedDate") {
		t.Error("query should contain committedDate")
	}
}

func TestBuildBatchQueryEmpty(t *testing.T) {
	query := buildBatchQuery(nil)
	if !strings.Contains(query, "query") {
		t.Error("empty batch should still produce a valid query structure")
	}
}
