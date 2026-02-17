// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

// CreateGitSnapshot inserts a new git snapshot into the database.
func (r *Repository) CreateGitSnapshot(ctx context.Context, snapshot *models.GitSnapshot) error {
	if snapshot.ID == "" {
		snapshot.ID = uuid.New().String()
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}

	filesJSON := "{}"
	if snapshot.Files != nil {
		filesBytes, err := json.Marshal(snapshot.Files)
		if err != nil {
			return fmt.Errorf("failed to serialize git snapshot files: %w", err)
		}
		filesJSON = string(filesBytes)
	}

	metadataJSON := "{}"
	if snapshot.Metadata != nil {
		metadataBytes, err := json.Marshal(snapshot.Metadata)
		if err != nil {
			return fmt.Errorf("failed to serialize git snapshot metadata: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_git_snapshots (
			id, session_id, snapshot_type, branch, remote_branch, head_commit, base_commit,
			ahead, behind, files, triggered_by, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), snapshot.ID, snapshot.SessionID, string(snapshot.SnapshotType), snapshot.Branch,
		snapshot.RemoteBranch, snapshot.HeadCommit, snapshot.BaseCommit, snapshot.Ahead,
		snapshot.Behind, filesJSON, snapshot.TriggeredBy, metadataJSON, snapshot.CreatedAt)

	return err
}

// GetLatestGitSnapshot retrieves the most recent git snapshot for a session.
// Returns sql.ErrNoRows if no snapshot is found.
func (r *Repository) GetLatestGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error) {
	snapshot := &models.GitSnapshot{}
	var snapshotType string
	var filesJSON string
	var metadataJSON string

	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id, session_id, snapshot_type, branch, remote_branch, head_commit, base_commit,
		       ahead, behind, files, triggered_by, metadata, created_at
		FROM task_session_git_snapshots
		WHERE session_id = ?
		ORDER BY created_at DESC LIMIT 1
	`), sessionID).Scan(
		&snapshot.ID, &snapshot.SessionID, &snapshotType, &snapshot.Branch,
		&snapshot.RemoteBranch, &snapshot.HeadCommit, &snapshot.BaseCommit,
		&snapshot.Ahead, &snapshot.Behind, &filesJSON, &snapshot.TriggeredBy,
		&metadataJSON, &snapshot.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	snapshot.SnapshotType = models.SnapshotType(snapshotType)
	if filesJSON != "" && filesJSON != "{}" {
		if err := json.Unmarshal([]byte(filesJSON), &snapshot.Files); err != nil {
			return nil, fmt.Errorf("failed to deserialize git snapshot files: %w", err)
		}
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &snapshot.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize git snapshot metadata: %w", err)
		}
	}

	return snapshot, nil
}

// GetFirstGitSnapshot retrieves the oldest git snapshot for a session (first one created).
// Returns sql.ErrNoRows if no snapshot is found.
func (r *Repository) GetFirstGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error) {
	snapshot := &models.GitSnapshot{}
	var snapshotType string
	var filesJSON string
	var metadataJSON string

	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id, session_id, snapshot_type, branch, remote_branch, head_commit, base_commit,
		       ahead, behind, files, triggered_by, metadata, created_at
		FROM task_session_git_snapshots
		WHERE session_id = ?
		ORDER BY created_at ASC LIMIT 1
	`), sessionID).Scan(
		&snapshot.ID, &snapshot.SessionID, &snapshotType, &snapshot.Branch,
		&snapshot.RemoteBranch, &snapshot.HeadCommit, &snapshot.BaseCommit,
		&snapshot.Ahead, &snapshot.Behind, &filesJSON, &snapshot.TriggeredBy,
		&metadataJSON, &snapshot.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	snapshot.SnapshotType = models.SnapshotType(snapshotType)
	if filesJSON != "" && filesJSON != "{}" {
		if err := json.Unmarshal([]byte(filesJSON), &snapshot.Files); err != nil {
			return nil, fmt.Errorf("failed to deserialize git snapshot files: %w", err)
		}
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &snapshot.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize git snapshot metadata: %w", err)
		}
	}

	return snapshot, nil
}

// GetGitSnapshotsBySession retrieves all git snapshots for a session, ordered by created_at descending.
// If limit > 0, only that many snapshots are returned.
// Returns an empty slice if no snapshots are found.
func (r *Repository) GetGitSnapshotsBySession(ctx context.Context, sessionID string, limit int) ([]*models.GitSnapshot, error) {
	query := `
		SELECT id, session_id, snapshot_type, branch, remote_branch, head_commit, base_commit,
		       ahead, behind, files, triggered_by, metadata, created_at
		FROM task_session_git_snapshots
		WHERE session_id = ?
		ORDER BY created_at DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.db.QueryContext(ctx, r.db.Rebind(query), sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.GitSnapshot
	for rows.Next() {
		snapshot := &models.GitSnapshot{}
		var snapshotType string
		var filesJSON string
		var metadataJSON string

		err := rows.Scan(
			&snapshot.ID, &snapshot.SessionID, &snapshotType, &snapshot.Branch,
			&snapshot.RemoteBranch, &snapshot.HeadCommit, &snapshot.BaseCommit,
			&snapshot.Ahead, &snapshot.Behind, &filesJSON, &snapshot.TriggeredBy,
			&metadataJSON, &snapshot.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		snapshot.SnapshotType = models.SnapshotType(snapshotType)
		if filesJSON != "" && filesJSON != "{}" {
			if err := json.Unmarshal([]byte(filesJSON), &snapshot.Files); err != nil {
				return nil, fmt.Errorf("failed to deserialize git snapshot files: %w", err)
			}
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &snapshot.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize git snapshot metadata: %w", err)
			}
		}

		result = append(result, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// CreateSessionCommit inserts a new commit record into the database.
func (r *Repository) CreateSessionCommit(ctx context.Context, commit *models.SessionCommit) error {
	if commit.ID == "" {
		commit.ID = uuid.New().String()
	}
	if commit.CreatedAt.IsZero() {
		commit.CreatedAt = time.Now().UTC()
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_commits (
			id, session_id, commit_sha, parent_sha, author_name, author_email,
			commit_message, committed_at, pre_commit_snapshot_id, post_commit_snapshot_id,
			files_changed, insertions, deletions, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), commit.ID, commit.SessionID, commit.CommitSHA, commit.ParentSHA,
		commit.AuthorName, commit.AuthorEmail, commit.CommitMessage, commit.CommittedAt,
		commit.PreCommitSnapshotID, commit.PostCommitSnapshotID, commit.FilesChanged,
		commit.Insertions, commit.Deletions, commit.CreatedAt)

	return err
}

// GetSessionCommits retrieves all commits for a session, ordered by committed_at descending.
// Returns an empty slice if no commits are found.
func (r *Repository) GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error) {
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`
		SELECT id, session_id, commit_sha, parent_sha, author_name, author_email,
		       commit_message, committed_at, pre_commit_snapshot_id, post_commit_snapshot_id,
		       files_changed, insertions, deletions, created_at
		FROM task_session_commits
		WHERE session_id = ?
		ORDER BY committed_at DESC
	`), sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.SessionCommit
	for rows.Next() {
		commit := &models.SessionCommit{}
		err := rows.Scan(
			&commit.ID, &commit.SessionID, &commit.CommitSHA, &commit.ParentSHA,
			&commit.AuthorName, &commit.AuthorEmail, &commit.CommitMessage,
			&commit.CommittedAt, &commit.PreCommitSnapshotID, &commit.PostCommitSnapshotID,
			&commit.FilesChanged, &commit.Insertions, &commit.Deletions, &commit.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, commit)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// GetLatestSessionCommit retrieves the most recent commit for a session.
// Returns sql.ErrNoRows if no commit is found.
func (r *Repository) GetLatestSessionCommit(ctx context.Context, sessionID string) (*models.SessionCommit, error) {
	commit := &models.SessionCommit{}

	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id, session_id, commit_sha, parent_sha, author_name, author_email,
		       commit_message, committed_at, pre_commit_snapshot_id, post_commit_snapshot_id,
		       files_changed, insertions, deletions, created_at
		FROM task_session_commits
		WHERE session_id = ?
		ORDER BY committed_at DESC LIMIT 1
	`), sessionID).Scan(
		&commit.ID, &commit.SessionID, &commit.CommitSHA, &commit.ParentSHA,
		&commit.AuthorName, &commit.AuthorEmail, &commit.CommitMessage,
		&commit.CommittedAt, &commit.PreCommitSnapshotID, &commit.PostCommitSnapshotID,
		&commit.FilesChanged, &commit.Insertions, &commit.Deletions, &commit.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return commit, nil
}

// DeleteSessionCommit removes a commit record from the database.
func (r *Repository) DeleteSessionCommit(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_session_commits WHERE id = ?`), id)
	return err
}
