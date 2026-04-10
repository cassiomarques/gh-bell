package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cassiomarques/gh-bell/internal/github"

	_ "modernc.org/sqlite"
)

// Store provides persistent storage for notifications, thread details,
// mutes, and preferences using SQLite.
type Store struct {
	db *sql.DB
}

// MutedThread represents a persistently muted notification thread.
type MutedThread struct {
	ThreadID     string
	RepoFullName string
	SubjectTitle string
	MutedAt      time.Time
}

// Open creates or opens a SQLite database at the given path.
// It enables WAL mode and foreign keys, then runs schema migrations.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrent read/write performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS notifications (
			id              TEXT PRIMARY KEY,
			unread          BOOLEAN NOT NULL,
			reason          TEXT NOT NULL,
			updated_at      DATETIME NOT NULL,
			subject_title   TEXT NOT NULL,
			subject_type    TEXT NOT NULL,
			subject_url     TEXT,
			comment_url     TEXT,
			repo_full_name  TEXT NOT NULL,
			repo_html_url   TEXT,
			repo_private    BOOLEAN DEFAULT 0,
			repo_owner      TEXT,
			first_seen_at   DATETIME NOT NULL,
			last_fetched_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_notifications_repo ON notifications(repo_full_name);
		CREATE INDEX IF NOT EXISTS idx_notifications_updated ON notifications(updated_at);

		CREATE TABLE IF NOT EXISTS thread_details (
			thread_id       TEXT PRIMARY KEY,
			state           TEXT,
			body            TEXT,
			author          TEXT,
			labels_json     TEXT,
			draft           BOOLEAN DEFAULT 0,
			merged          BOOLEAN DEFAULT 0,
			merged_by       TEXT,
			additions       INTEGER DEFAULT 0,
			deletions       INTEGER DEFAULT 0,
			tag_name        TEXT,
			comment_body    TEXT,
			comment_author  TEXT,
			comment_at      DATETIME,
			fetched_at      DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS muted_threads (
			thread_id       TEXT PRIMARY KEY,
			repo_full_name  TEXT NOT NULL,
			subject_title   TEXT,
			muted_at        DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS preferences (
			key   TEXT PRIMARY KEY,
			value TEXT
		);
	`)
	if err != nil {
		return err
	}

	// Add columns introduced after initial schema. SQLite ignores
	// "duplicate column name" errors, so we attempt each and tolerate failure.
	newCols := []string{
		"ALTER TABLE thread_details ADD COLUMN html_url TEXT DEFAULT ''",
		"ALTER TABLE thread_details ADD COLUMN requested_reviewers_json TEXT DEFAULT ''",
		"ALTER TABLE thread_details ADD COLUMN requested_teams_json TEXT DEFAULT ''",
		"ALTER TABLE thread_details ADD COLUMN milestone TEXT DEFAULT ''",
		"ALTER TABLE thread_details ADD COLUMN review_decision TEXT DEFAULT ''",
		"ALTER TABLE thread_details ADD COLUMN ci_status TEXT DEFAULT ''",
		"ALTER TABLE thread_details ADD COLUMN mergeable TEXT DEFAULT ''",
		"ALTER TABLE thread_details ADD COLUMN latest_commit_at DATETIME",
		"ALTER TABLE thread_details ADD COLUMN latest_review_at DATETIME",
	}
	for _, ddl := range newCols {
		s.db.Exec(ddl) // ignore "duplicate column" errors
	}
	return nil
}

// --- Notifications ---

// UpsertNotification inserts or updates a notification in the cache.
// On insert, first_seen_at is set to now. On update, it's preserved.
func (s *Store) UpsertNotification(n github.Notification) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO notifications (id, unread, reason, updated_at, subject_title,
			subject_type, subject_url, comment_url, repo_full_name, repo_html_url,
			repo_private, repo_owner, first_seen_at, last_fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			unread = excluded.unread,
			reason = excluded.reason,
			updated_at = excluded.updated_at,
			subject_title = excluded.subject_title,
			subject_type = excluded.subject_type,
			subject_url = excluded.subject_url,
			comment_url = excluded.comment_url,
			repo_full_name = excluded.repo_full_name,
			repo_html_url = excluded.repo_html_url,
			repo_private = excluded.repo_private,
			repo_owner = excluded.repo_owner,
			last_fetched_at = excluded.last_fetched_at`,
		n.ID, n.Unread, n.Reason, n.UpdatedAt.UTC(), n.Subject.Title,
		n.Subject.Type, n.Subject.URL, n.Subject.LatestCommentURL,
		n.Repository.FullName, n.Repository.HTMLURL, n.Repository.Private,
		n.Repository.Owner.Login, now, now,
	)
	return err
}

// UpsertNotifications upserts a batch of notifications in a single transaction.
func (s *Store) UpsertNotifications(notifications []github.Notification) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	stmt, err := tx.Prepare(`
		INSERT INTO notifications (id, unread, reason, updated_at, subject_title,
			subject_type, subject_url, comment_url, repo_full_name, repo_html_url,
			repo_private, repo_owner, first_seen_at, last_fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			unread = excluded.unread,
			reason = excluded.reason,
			updated_at = excluded.updated_at,
			subject_title = excluded.subject_title,
			subject_type = excluded.subject_type,
			subject_url = excluded.subject_url,
			comment_url = excluded.comment_url,
			repo_full_name = excluded.repo_full_name,
			repo_html_url = excluded.repo_html_url,
			repo_private = excluded.repo_private,
			repo_owner = excluded.repo_owner,
			last_fetched_at = excluded.last_fetched_at`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, n := range notifications {
		_, err := stmt.Exec(
			n.ID, n.Unread, n.Reason, n.UpdatedAt.UTC(), n.Subject.Title,
			n.Subject.Type, n.Subject.URL, n.Subject.LatestCommentURL,
			n.Repository.FullName, n.Repository.HTMLURL, n.Repository.Private,
			n.Repository.Owner.Login, now, now,
		)
		if err != nil {
			return fmt.Errorf("upsert notification %s: %w", n.ID, err)
		}
	}

	return tx.Commit()
}

// ListNotifications returns cached notifications, optionally filtered by unread status.
// Results are ordered by updated_at DESC.
func (s *Store) ListNotifications(unreadOnly bool) ([]github.Notification, error) {
	query := `
		SELECT id, unread, reason, updated_at, subject_title, subject_type,
			subject_url, comment_url, repo_full_name, repo_html_url,
			repo_private, repo_owner
		FROM notifications`
	if unreadOnly {
		query += " WHERE unread = 1"
	}
	query += " ORDER BY updated_at DESC"

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []github.Notification
	for rows.Next() {
		var n github.Notification
		var subjectURL, commentURL, repoHTML, repoOwner sql.NullString
		err := rows.Scan(
			&n.ID, &n.Unread, &n.Reason, &n.UpdatedAt,
			&n.Subject.Title, &n.Subject.Type,
			&subjectURL, &commentURL,
			&n.Repository.FullName, &repoHTML,
			&n.Repository.Private, &repoOwner,
		)
		if err != nil {
			return nil, err
		}
		if subjectURL.Valid {
			n.Subject.URL = subjectURL.String
		}
		if commentURL.Valid {
			n.Subject.LatestCommentURL = commentURL.String
		}
		if repoHTML.Valid {
			n.Repository.HTMLURL = repoHTML.String
		}
		if repoOwner.Valid {
			n.Repository.Owner.Login = repoOwner.String
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// DeleteNotification removes a notification from the cache.
func (s *Store) DeleteNotification(id string) error {
	_, err := s.db.Exec("DELETE FROM notifications WHERE id = ?", id)
	return err
}

// MarkNotificationRead sets the unread flag to false for a notification.
func (s *Store) MarkNotificationRead(id string) error {
	_, err := s.db.Exec("UPDATE notifications SET unread = 0 WHERE id = ?", id)
	return err
}

// MarkAllRead sets all notifications as read.
func (s *Store) MarkAllRead() error {
	_, err := s.db.Exec("UPDATE notifications SET unread = 0")
	return err
}

// PurgeOldNotifications deletes read notifications older than the given cutoff.
// Returns the number of rows deleted.
func (s *Store) PurgeOldNotifications(olderThan time.Time) (int, error) {
	res, err := s.db.Exec(
		"DELETE FROM notifications WHERE unread = 0 AND updated_at < ?",
		olderThan.UTC(),
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PurgeOldDetails deletes thread details that have no corresponding notification.
// Returns the number of rows deleted.
func (s *Store) PurgeOldDetails() (int, error) {
	res, err := s.db.Exec(`
		DELETE FROM thread_details
		WHERE thread_id NOT IN (SELECT id FROM notifications)
	`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// --- Thread Details ---

// LatestUpdatedAt returns the most recent updated_at timestamp from cached
// notifications, or nil if the table is empty.
func (s *Store) LatestUpdatedAt() (*time.Time, error) {
	var raw *string
	err := s.db.QueryRow("SELECT MAX(updated_at) FROM notifications").Scan(&raw)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	// modernc.org/sqlite stores time.Time as "2006-01-02 15:04:05 +0000 UTC"
	formats := []string{
		"2006-01-02 15:04:05 +0000 UTC",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05+00:00",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, *raw); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("parse MAX(updated_at) %q: unknown format", *raw)
}

// NotificationCount returns the number of notifications in the cache.
func (s *Store) NotificationCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM notifications").Scan(&count)
	return count, err
}

// UpsertDetail stores or updates cached thread detail data.
func (s *Store) UpsertDetail(threadID string, d *github.ThreadDetail) error {
	labelsJSON, err := json.Marshal(d.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}

	reviewersJSON, err := json.Marshal(d.RequestedReviewers)
	if err != nil {
		return fmt.Errorf("marshal requested_reviewers: %w", err)
	}

	teamsJSON, err := json.Marshal(d.RequestedTeams)
	if err != nil {
		return fmt.Errorf("marshal requested_teams: %w", err)
	}

	var milestone string
	if d.Milestone != nil {
		milestone = d.Milestone.Title
	}

	var mergedBy sql.NullString
	if d.MergedBy != nil {
		mergedBy = sql.NullString{String: d.MergedBy.Login, Valid: true}
	}

	var commentBody, commentAuthor sql.NullString
	var commentAt sql.NullTime
	if d.LatestComment != nil {
		commentBody = sql.NullString{String: d.LatestComment.Body, Valid: true}
		commentAuthor = sql.NullString{String: d.LatestComment.User.Login, Valid: true}
		commentAt = sql.NullTime{Time: d.LatestComment.CreatedAt.UTC(), Valid: true}
	}

	var latestCommitAt, latestReviewAt sql.NullTime
	if d.LatestCommitAt != nil {
		latestCommitAt = sql.NullTime{Time: d.LatestCommitAt.UTC(), Valid: true}
	}
	if d.LatestReviewAt != nil {
		latestReviewAt = sql.NullTime{Time: d.LatestReviewAt.UTC(), Valid: true}
	}

	now := time.Now().UTC()
	_, err = s.db.Exec(`
		INSERT INTO thread_details (thread_id, state, body, author, labels_json,
			draft, merged, merged_by, additions, deletions, tag_name,
			comment_body, comment_author, comment_at, fetched_at,
			html_url, requested_reviewers_json, requested_teams_json, milestone,
			review_decision, ci_status, mergeable, latest_commit_at, latest_review_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(thread_id) DO UPDATE SET
			state = excluded.state,
			body = excluded.body,
			author = excluded.author,
			labels_json = excluded.labels_json,
			draft = excluded.draft,
			merged = excluded.merged,
			merged_by = excluded.merged_by,
			additions = excluded.additions,
			deletions = excluded.deletions,
			tag_name = excluded.tag_name,
			comment_body = excluded.comment_body,
			comment_author = excluded.comment_author,
			comment_at = excluded.comment_at,
			fetched_at = excluded.fetched_at,
			html_url = excluded.html_url,
			requested_reviewers_json = excluded.requested_reviewers_json,
			requested_teams_json = excluded.requested_teams_json,
			milestone = excluded.milestone,
			review_decision = excluded.review_decision,
			ci_status = excluded.ci_status,
			mergeable = excluded.mergeable,
			latest_commit_at = excluded.latest_commit_at,
			latest_review_at = excluded.latest_review_at`,
		threadID, d.State, d.Body, d.User.Login, string(labelsJSON),
		d.Draft, d.Merged, mergedBy, d.Additions, d.Deletions, d.TagName,
		commentBody, commentAuthor, commentAt, now,
		d.HTMLURL, string(reviewersJSON), string(teamsJSON), milestone,
		d.ReviewDecision, d.CIStatus, d.Mergeable, latestCommitAt, latestReviewAt,
	)
	return err
}

// GetDetail retrieves a cached thread detail. Returns nil if not found.
func (s *Store) GetDetail(threadID string) (*github.ThreadDetail, time.Time, error) {
	var d github.ThreadDetail
	var labelsJSON string
	var reviewersJSON, teamsJSON, milestone string
	var mergedBy sql.NullString
	var commentBody, commentAuthor sql.NullString
	var commentAt sql.NullTime
	var fetchedAt time.Time
	var latestCommitAt, latestReviewAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT state, body, author, labels_json, draft, merged, merged_by,
			additions, deletions, tag_name, comment_body, comment_author,
			comment_at, fetched_at,
			html_url, requested_reviewers_json, requested_teams_json, milestone,
			review_decision, ci_status, mergeable, latest_commit_at, latest_review_at
		FROM thread_details WHERE thread_id = ?`, threadID,
	).Scan(
		&d.State, &d.Body, &d.User.Login, &labelsJSON,
		&d.Draft, &d.Merged, &mergedBy,
		&d.Additions, &d.Deletions, &d.TagName,
		&commentBody, &commentAuthor, &commentAt, &fetchedAt,
		&d.HTMLURL, &reviewersJSON, &teamsJSON, &milestone,
		&d.ReviewDecision, &d.CIStatus, &d.Mergeable, &latestCommitAt, &latestReviewAt,
	)
	if err == sql.ErrNoRows {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, err
	}

	if labelsJSON != "" {
		if err := json.Unmarshal([]byte(labelsJSON), &d.Labels); err != nil {
			return nil, time.Time{}, fmt.Errorf("unmarshal labels: %w", err)
		}
	}

	if reviewersJSON != "" {
		if err := json.Unmarshal([]byte(reviewersJSON), &d.RequestedReviewers); err != nil {
			return nil, time.Time{}, fmt.Errorf("unmarshal requested_reviewers: %w", err)
		}
	}

	if teamsJSON != "" {
		if err := json.Unmarshal([]byte(teamsJSON), &d.RequestedTeams); err != nil {
			return nil, time.Time{}, fmt.Errorf("unmarshal requested_teams: %w", err)
		}
	}

	if milestone != "" {
		d.Milestone = &github.Milestone{Title: milestone}
	}

	if mergedBy.Valid {
		d.MergedBy = &github.User{Login: mergedBy.String}
	}

	if commentBody.Valid {
		d.LatestComment = &github.Comment{
			Body: commentBody.String,
			User: github.User{Login: commentAuthor.String},
		}
		if commentAt.Valid {
			d.LatestComment.CreatedAt = commentAt.Time
		}
	}

	if latestCommitAt.Valid {
		t := latestCommitAt.Time
		d.LatestCommitAt = &t
	}
	if latestReviewAt.Valid {
		t := latestReviewAt.Time
		d.LatestReviewAt = &t
	}

	return &d, fetchedAt, nil
}

// UpdateDetailEnrichment updates only the GraphQL enrichment fields for an
// existing thread detail row. Does nothing if the thread has no detail cached.
func (s *Store) UpdateDetailEnrichment(threadID string, e *github.PREnrichment) error {
	var latestCommitAt, latestReviewAt sql.NullTime
	if e.LatestCommitAt != nil {
		latestCommitAt = sql.NullTime{Time: e.LatestCommitAt.UTC(), Valid: true}
	}
	if e.LatestReviewAt != nil {
		latestReviewAt = sql.NullTime{Time: e.LatestReviewAt.UTC(), Valid: true}
	}
	_, err := s.db.Exec(`
		UPDATE thread_details SET
			review_decision = ?,
			ci_status = ?,
			mergeable = ?,
			latest_commit_at = ?,
			latest_review_at = ?
		WHERE thread_id = ?`,
		e.ReviewDecision, e.CIStatus, e.Mergeable,
		latestCommitAt, latestReviewAt,
		threadID,
	)
	return err
}

// DeleteDetail removes cached thread detail.
func (s *Store) DeleteDetail(threadID string) error {
	_, err := s.db.Exec("DELETE FROM thread_details WHERE thread_id = ?", threadID)
	return err
}

// --- Muted Threads ---

// MuteThread adds a thread to the persistent mute list.
func (s *Store) MuteThread(threadID, repoFullName, subjectTitle string) error {
	_, err := s.db.Exec(`
		INSERT INTO muted_threads (thread_id, repo_full_name, subject_title, muted_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(thread_id) DO UPDATE SET
			repo_full_name = excluded.repo_full_name,
			subject_title = excluded.subject_title,
			muted_at = excluded.muted_at`,
		threadID, repoFullName, subjectTitle, time.Now().UTC(),
	)
	return err
}

// UnmuteThread removes a thread from the mute list.
func (s *Store) UnmuteThread(threadID string) error {
	_, err := s.db.Exec("DELETE FROM muted_threads WHERE thread_id = ?", threadID)
	return err
}

// IsMuted checks whether a thread is in the mute list.
func (s *Store) IsMuted(threadID string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM muted_threads WHERE thread_id = ?", threadID).Scan(&count)
	return count > 0, err
}

// ListMuted returns all muted threads.
func (s *Store) ListMuted() ([]MutedThread, error) {
	rows, err := s.db.Query(`
		SELECT thread_id, repo_full_name, subject_title, muted_at
		FROM muted_threads ORDER BY muted_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MutedThread
	for rows.Next() {
		var m MutedThread
		var title sql.NullString
		if err := rows.Scan(&m.ThreadID, &m.RepoFullName, &title, &m.MutedAt); err != nil {
			return nil, err
		}
		if title.Valid {
			m.SubjectTitle = title.String
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// --- Preferences ---

// GetPref retrieves a stored preference value. Returns empty string if not found.
func (s *Store) GetPref(key string) (string, error) {
	var value sql.NullString
	err := s.db.QueryRow("SELECT value FROM preferences WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if value.Valid {
		return value.String, nil
	}
	return "", nil
}

// SetPref stores a preference key-value pair.
func (s *Store) SetPref(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO preferences (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// DeletePref removes a preference.
func (s *Store) DeletePref(key string) error {
	_, err := s.db.Exec("DELETE FROM preferences WHERE key = ?", key)
	return err
}

// DataDir returns the default data directory path (~/.gh-bell/).
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".gh-bell"), nil
}

// DefaultDBPath returns the default database file path (~/.gh-bell/meta.db).
func DefaultDBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "meta.db"), nil
}
