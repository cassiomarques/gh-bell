package github

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// PRRef identifies a pull request for batch GraphQL enrichment.
type PRRef struct {
	Owner    string
	Repo     string
	Number   int
	ThreadID string // notification thread ID for mapping results back
}

// PREnrichment holds fields fetched via GraphQL that aren't available from REST.
type PREnrichment struct {
	ReviewDecision string     // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	CIStatus       string     // SUCCESS, FAILURE, PENDING, ERROR, ""
	Mergeable      string     // MERGEABLE, CONFLICTING, UNKNOWN, ""
	LatestCommitAt *time.Time // most recent commit timestamp
	LatestReviewAt *time.Time // most recent review timestamp
}

// maxBatchSize is the maximum number of PRs to query in a single GraphQL request.
const maxBatchSize = 50

// EnrichPRsBatch fetches review decision, CI status, mergeable state, and
// latest commit/review timestamps for a batch of PRs using a single GraphQL
// query with aliases. Returns a map keyed by thread ID.
func (c *Client) EnrichPRsBatch(prs []PRRef) (map[string]*PREnrichment, error) {
	if len(prs) == 0 {
		return nil, nil
	}

	gql, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: c.token})
	if err != nil {
		return nil, fmt.Errorf("creating GraphQL client: %w", err)
	}

	result := make(map[string]*PREnrichment, len(prs))

	// Process in batches of maxBatchSize
	for i := 0; i < len(prs); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(prs) {
			end = len(prs)
		}
		batch := prs[i:end]

		enriched, err := c.enrichBatch(gql, batch)
		if err != nil {
			log.Printf("graphql: batch enrichment error (batch %d-%d): %v", i, end, err)
			continue // partial failure is OK
		}
		for k, v := range enriched {
			result[k] = v
		}
	}

	return result, nil
}

// enrichBatch builds and executes a single GraphQL query for a batch of PRs.
func (c *Client) enrichBatch(gql *api.GraphQLClient, prs []PRRef) (map[string]*PREnrichment, error) {
	query := buildBatchQuery(prs)

	var response map[string]prNode
	err := gql.DoWithContext(context.Background(), query, nil, &response)
	if err != nil {
		return nil, fmt.Errorf("graphql query: %w", err)
	}

	result := make(map[string]*PREnrichment, len(prs))
	for idx, pr := range prs {
		alias := fmt.Sprintf("pr%d", idx)
		repoNode, ok := response[alias]
		if !ok {
			continue
		}
		if repoNode.PullRequest == nil {
			continue
		}
		prData := repoNode.PullRequest
		enrichment := &PREnrichment{
			ReviewDecision: prData.ReviewDecision,
			Mergeable:      prData.Mergeable,
		}

		// Extract CI status from the last commit's status check rollup
		if len(prData.Commits.Nodes) > 0 {
			commit := prData.Commits.Nodes[0].Commit
			if commit.CommittedDate != "" {
				if t, err := time.Parse(time.RFC3339, commit.CommittedDate); err == nil {
					enrichment.LatestCommitAt = &t
				}
			}
			if commit.StatusCheckRollup != nil {
				enrichment.CIStatus = commit.StatusCheckRollup.State
			}
		}

		// Extract latest review timestamp
		if len(prData.Reviews.Nodes) > 0 && prData.Reviews.Nodes[0].SubmittedAt != "" {
			if t, err := time.Parse(time.RFC3339, prData.Reviews.Nodes[0].SubmittedAt); err == nil {
				enrichment.LatestReviewAt = &t
			}
		}

		result[pr.ThreadID] = enrichment
	}

	return result, nil
}

// prNode mirrors the GraphQL response shape for a repository alias.
type prNode struct {
	PullRequest *prFields `json:"pullRequest"`
}

type prFields struct {
	ReviewDecision string        `json:"reviewDecision"`
	Mergeable      string        `json:"mergeable"`
	Commits        commitConnection `json:"commits"`
	Reviews        reviewConnection `json:"reviews"`
}

type commitConnection struct {
	Nodes []commitNode `json:"nodes"`
}

type commitNode struct {
	Commit commitData `json:"commit"`
}

type commitData struct {
	StatusCheckRollup *statusRollup `json:"statusCheckRollup"`
	CommittedDate     string        `json:"committedDate"`
}

type statusRollup struct {
	State string `json:"state"`
}

type reviewConnection struct {
	Nodes []reviewNode `json:"nodes"`
}

type reviewNode struct {
	SubmittedAt string `json:"submittedAt"`
}

// buildBatchQuery constructs a GraphQL query with aliases for each PR.
func buildBatchQuery(prs []PRRef) string {
	var b strings.Builder
	b.WriteString("query {\n")
	for i, pr := range prs {
		fmt.Fprintf(&b, "  pr%d: repository(owner: %q, name: %q) {\n", i, pr.Owner, pr.Repo)
		fmt.Fprintf(&b, "    pullRequest(number: %d) {\n", pr.Number)
		b.WriteString("      reviewDecision\n")
		b.WriteString("      mergeable\n")
		b.WriteString("      commits(last: 1) { nodes { commit { statusCheckRollup { state } committedDate } } }\n")
		b.WriteString("      reviews(last: 1) { nodes { submittedAt } }\n")
		b.WriteString("    }\n")
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// ParseSubjectURL extracts owner, repo, and number from a GitHub API subject URL.
// e.g. "https://api.github.com/repos/owner/repo/pulls/42" → ("owner", "repo", 42, true)
func ParseSubjectURL(url string) (owner, repo string, number int, ok bool) {
	const prefix = "https://api.github.com/repos/"
	if !strings.HasPrefix(url, prefix) {
		return
	}
	rest := url[len(prefix):]
	// rest = "owner/repo/pulls/42" or "owner/repo/issues/42"
	parts := strings.Split(rest, "/")
	if len(parts) < 4 {
		return
	}
	if parts[2] != "pulls" {
		return // only enrich PRs
	}
	var num int
	_, err := fmt.Sscanf(parts[3], "%d", &num)
	if err != nil {
		return
	}
	return parts[0], parts[1], num, true
}
