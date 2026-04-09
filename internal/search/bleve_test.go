package search

import (
	"testing"
)

func testIndex(t *testing.T) *SearchIndex {
	t.Helper()
	idx, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func seedIndex(t *testing.T, idx *SearchIndex) {
	t.Helper()
	docs := []struct {
		id, title, body, comment, labels, repo, reason, subjectType string
	}{
		{
			id: "1", title: "Fix authentication bug in login",
			body: "The login endpoint returns 401 for valid credentials when rate limited",
			comment: "I can reproduce this on staging", labels: "bug security",
			repo: "acme/webapp", reason: "mention", subjectType: "Issue",
		},
		{
			id: "2", title: "Add dark mode support",
			body: "Users have requested dark mode for the settings panel",
			comment: "This would be a great addition to the UI",
			labels: "feature enhancement", repo: "acme/webapp",
			reason: "subscribed", subjectType: "PullRequest",
		},
		{
			id: "3", title: "Upgrade Go to 1.23",
			body: "We should upgrade to Go 1.23 for the new iterator features",
			comment: "", labels: "infrastructure",
			repo: "acme/backend", reason: "review_requested", subjectType: "PullRequest",
		},
		{
			id: "4", title: "Release v2.0.0",
			body: "Major release with breaking changes to the API",
			comment: "", labels: "",
			repo: "acme/backend", reason: "subscribed", subjectType: "Release",
		},
		{
			id: "5", title: "Security vulnerability in dependency",
			body: "CVE-2025-1234 affects lodash < 4.17.21",
			comment: "We need to patch this ASAP",
			labels: "security critical", repo: "org/frontend",
			reason: "security_alert", subjectType: "RepositoryVulnerabilityAlert",
		},
	}

	for _, d := range docs {
		if err := idx.Index(d.id, d.title, d.body, d.comment, d.labels, d.repo, d.reason, d.subjectType); err != nil {
			t.Fatalf("Index(%s): %v", d.id, err)
		}
	}
}

func TestIndexAndCount(t *testing.T) {
	idx := testIndex(t)
	seedIndex(t, idx)

	count, err := idx.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Errorf("expected 5 docs, got %d", count)
	}
}

func TestSearchExact(t *testing.T) {
	idx := testIndex(t)
	seedIndex(t, idx)

	tests := []struct {
		name    string
		query   string
		wantIDs []string
	}{
		{"by title word", "authentication", []string{"1"}},
		{"by body content", "dark mode", []string{"2"}},
		{"by repo", "repo:acme/backend", []string{"3", "4"}},
		{"by label", "labels:security", []string{"1", "5"}},
		{"by type", "type:Release", []string{"4"}},
		{"by comment", "staging", []string{"1"}},
		{"multi-word", "Go upgrade", []string{"3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := idx.Search(tt.query, 10)
			if err != nil {
				t.Fatalf("Search(%q): %v", tt.query, err)
			}
			gotIDs := make(map[string]bool)
			for _, r := range results {
				gotIDs[r.ThreadID] = true
			}
			for _, wantID := range tt.wantIDs {
				if !gotIDs[wantID] {
					t.Errorf("expected result ID=%s in results for query %q, got %v",
						wantID, tt.query, resultIDs(results))
				}
			}
		})
	}
}

func TestSearchFuzzy(t *testing.T) {
	idx := testIndex(t)
	seedIndex(t, idx)

	// Fuzzy search should find "login" with a close typo "lgin" (1 edit)
	results, err := idx.SearchFuzzy("lgin", 10)
	if err != nil {
		t.Fatalf("SearchFuzzy: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected fuzzy match for 'lgin', got 0 results")
	}
}

func TestSearchEmpty(t *testing.T) {
	idx := testIndex(t)
	seedIndex(t, idx)

	results, err := idx.Search("nonexistent_xyzzy_term", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchFuzzyEmpty(t *testing.T) {
	idx := testIndex(t)

	results, err := idx.SearchFuzzy("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestDelete(t *testing.T) {
	idx := testIndex(t)
	seedIndex(t, idx)

	if err := idx.Delete("1"); err != nil {
		t.Fatal(err)
	}

	count, err := idx.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("expected 4 after delete, got %d", count)
	}

	results, err := idx.Search("authentication", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Error("deleted doc should not appear in results")
	}
}

func TestSearchResultHasFragments(t *testing.T) {
	idx := testIndex(t)
	seedIndex(t, idx)

	results, err := idx.Search("authentication", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	// Fragments should have highlighted content
	if len(results[0].Fragments) == 0 {
		t.Error("expected non-empty fragments for highlighted search")
	}
}

func TestOpenOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.bleve"

	// Create
	idx1, err := Open(path)
	if err != nil {
		t.Fatalf("Open (create): %v", err)
	}
	if err := idx1.Index("1", "Test title", "body", "", "", "repo/test", "mention", "Issue"); err != nil {
		t.Fatal(err)
	}
	idx1.Close()

	// Re-open
	idx2, err := Open(path)
	if err != nil {
		t.Fatalf("Open (reopen): %v", err)
	}
	defer idx2.Close()

	count, err := idx2.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 after reopen, got %d", count)
	}
}

func resultIDs(results []SearchResult) []string {
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ThreadID
	}
	return ids
}
