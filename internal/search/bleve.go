package search

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/highlight/highlighter/ansi"
)

// SearchResult represents a single search hit.
type SearchResult struct {
	ThreadID  string
	Score     float64
	Fragments map[string][]string // field → highlighted snippets
}

// document is the indexed representation of a notification thread.
type document struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	Comment string `json:"comment"`
	Labels  string `json:"labels"`
	Repo    string `json:"repo"`
	Reason  string `json:"reason"`
	Type    string `json:"type"`
}

// SearchIndex wraps a Bleve index for full-text search of notifications.
type SearchIndex struct {
	index bleve.Index
}

// Open opens or creates a Bleve index at the given path.
func Open(indexPath string) (*SearchIndex, error) {
	idx, err := bleve.Open(indexPath)
	if err == bleve.ErrorIndexPathDoesNotExist {
		idx, err = bleve.New(indexPath, buildMapping())
		if err != nil {
			return nil, fmt.Errorf("create search index: %w", err)
		}
	} else if err != nil {
		// Index may be corrupted — try to recreate
		log.Printf("search: index open failed, recreating: %v", err)
		if removeErr := os.RemoveAll(indexPath); removeErr != nil {
			return nil, fmt.Errorf("remove corrupted index: %w", removeErr)
		}
		idx, err = bleve.New(indexPath, buildMapping())
		if err != nil {
			return nil, fmt.Errorf("recreate search index: %w", err)
		}
	}
	return &SearchIndex{index: idx}, nil
}

// OpenInMemory creates an in-memory Bleve index (for tests).
func OpenInMemory() (*SearchIndex, error) {
	idx, err := bleve.NewMemOnly(buildMapping())
	if err != nil {
		return nil, fmt.Errorf("create in-memory index: %w", err)
	}
	return &SearchIndex{index: idx}, nil
}

// Close closes the index.
func (s *SearchIndex) Close() error {
	if s.index != nil {
		return s.index.Close()
	}
	return nil
}

func buildMapping() mapping.IndexMapping {
	m := bleve.NewIndexMapping()
	m.DefaultAnalyzer = "en"

	docMapping := bleve.NewDocumentMapping()

	textField := func() *mapping.FieldMapping {
		f := bleve.NewTextFieldMapping()
		f.Analyzer = "en"
		f.Store = true
		f.IncludeTermVectors = true
		return f
	}

	docMapping.AddFieldMappingsAt("title", textField())
	docMapping.AddFieldMappingsAt("body", textField())
	docMapping.AddFieldMappingsAt("comment", textField())
	docMapping.AddFieldMappingsAt("labels", textField())
	docMapping.AddFieldMappingsAt("repo", textField())
	docMapping.AddFieldMappingsAt("reason", textField())
	docMapping.AddFieldMappingsAt("type", textField())

	m.DefaultMapping = docMapping
	return m
}

// Index adds or updates a document in the search index.
func (s *SearchIndex) Index(threadID, title, body, comment, labels, repo, reason, subjectType string) error {
	doc := document{
		Title:   title,
		Body:    body,
		Comment: comment,
		Labels:  labels,
		Repo:    repo,
		Reason:  reason,
		Type:    subjectType,
	}
	return s.index.Index(threadID, doc)
}

// Delete removes a document from the index.
func (s *SearchIndex) Delete(threadID string) error {
	return s.index.Delete(threadID)
}

// Search performs an exact full-text search.
func (s *SearchIndex) Search(queryStr string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	q := bleve.NewQueryStringQuery(queryStr)
	req := bleve.NewSearchRequestOptions(q, limit, 0, false)
	req.Highlight = bleve.NewHighlightWithStyle(ansi.Name)
	req.Fields = []string{"title", "repo"}

	res, err := s.index.Search(req)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return mapResults(res), nil
}

// SearchFuzzy performs a fuzzy/typo-tolerant search using the same analyzer
// as the index, so stemming is applied to both query and indexed terms.
func (s *SearchIndex) SearchFuzzy(queryStr string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	queryStr = strings.TrimSpace(queryStr)
	if queryStr == "" {
		return nil, nil
	}

	mq := bleve.NewMatchQuery(queryStr)
	mq.SetFuzziness(1)
	req := bleve.NewSearchRequestOptions(mq, limit, 0, false)
	req.Highlight = bleve.NewHighlightWithStyle(ansi.Name)
	req.Fields = []string{"title", "repo"}

	res, err := s.index.Search(req)
	if err != nil {
		return nil, fmt.Errorf("fuzzy search: %w", err)
	}
	return mapResults(res), nil
}

// Reindex clears the index and re-indexes all provided documents.
func (s *SearchIndex) Reindex(docs map[string]document) error {
	batch := s.index.NewBatch()
	for id, doc := range docs {
		if err := batch.Index(id, doc); err != nil {
			return fmt.Errorf("batch index %s: %w", id, err)
		}
	}
	return s.index.Batch(batch)
}

// Count returns the number of documents in the index.
func (s *SearchIndex) Count() (uint64, error) {
	return s.index.DocCount()
}

func mapResults(res *bleve.SearchResult) []SearchResult {
	results := make([]SearchResult, 0, len(res.Hits))
	for _, hit := range res.Hits {
		results = append(results, SearchResult{
			ThreadID:  hit.ID,
			Score:     hit.Score,
			Fragments: hit.Fragments,
		})
	}
	return results
}
