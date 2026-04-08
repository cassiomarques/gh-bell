package github

import (
	"fmt"
	"strings"
)

// apiURLToWebURL converts a GitHub REST API URL to a web URL.
//
//	"https://api.github.com/repos/cli/cli/issues/123"
//	→ "https://github.com/cli/cli/issues/123"
//
// Handles pulls, issues, releases, discussions, and commits.
// Returns the original URL unchanged if it doesn't match the expected pattern.
func apiURLToWebURL(apiURL string) string {
	if apiURL == "" {
		return ""
	}

	const apiPrefix = "https://api.github.com/repos/"
	if !strings.HasPrefix(apiURL, apiPrefix) {
		return apiURL
	}

	path := strings.TrimPrefix(apiURL, apiPrefix)

	// The API uses "pulls" while the web uses "pull"
	path = strings.Replace(path, "/pulls/", "/pull/", 1)

	return fmt.Sprintf("https://github.com/%s", path)
}
