// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// TriggeredByLiveMonitor identifies snapshots written by the orchestrator's live
// git status persistence path. Used to scope the upsert in
// UpsertLatestLiveGitSnapshot so we don't disturb archive/completion snapshots.
const TriggeredByLiveMonitor = "live_monitor"

const triggeredByAgentCompleted = "agent_completed"

// snapshotRankExpr is the ORDER BY ranking shared by GetLatestGitSnapshot and
// GetLatestGitSnapshotsBySessionIDs. Both selectors must pick the same row for
// a session, so the expression lives in one place:
//
//   - rank 0: an archive snapshot that is still authoritative — either its
//     owning task is still archived (terminal cumulative diff), or the task
//     was unarchived but not yet resumed and no non-archive snapshot newer
//     than this archive row exists yet (a reload in that window must not
//     regress to pre-archive state).
//   - rank 1: agent_completed snapshots from the current execution generation.
//   - rank 2: live_monitor snapshots (periodic polls; may race completion).
//   - rank 3: pre-archive completions and archive snapshots that are stale
//     (task unarchived AND a newer non-archive snapshot exists).
//
// Ties within a rank break by created_at DESC, then id DESC so the selection
// is deterministic even when Postgres stores multiple writes at the same
// microsecond timestamp.
const snapshotRankExpr = `
			CASE
				WHEN snapshot_type = 'archive' AND (
					EXISTS (
						SELECT 1 FROM task_sessions ts
						JOIN tasks t ON t.id = ts.task_id
						WHERE ts.id = task_session_git_snapshots.session_id
						  AND t.archived_at IS NOT NULL
					)
					OR NOT EXISTS (
						SELECT 1 FROM task_session_git_snapshots newer
						WHERE newer.session_id = task_session_git_snapshots.session_id
						  AND newer.snapshot_type <> 'archive'
						  AND newer.created_at > task_session_git_snapshots.created_at
					)
				) THEN 0
				WHEN triggered_by = 'agent_completed' AND EXISTS (
					SELECT 1 FROM task_session_git_snapshots archive
					WHERE archive.session_id = task_session_git_snapshots.session_id
					  AND archive.snapshot_type = 'archive'
					  AND archive.created_at > task_session_git_snapshots.created_at
				) THEN 3
				WHEN triggered_by = 'agent_completed' THEN 1
				WHEN snapshot_type = 'archive' THEN 3
				ELSE 2
			END,
			created_at DESC,
			id DESC`

// snapshotRepositoryExpr returns the normalized repository partition key used
// by the multi-repository status selector. Missing and explicit-empty names
// both represent the root repository and therefore share the empty key.
func snapshotRepositoryExpr(driver, tableName string) string {
	return "COALESCE(" + dialect.JSONExtract(driver, tableName+".metadata", "repository_name") + ", '')"
}

// snapshotRankExprForRepository mirrors snapshotRankExpr for the per-repository
// status selector. The legacy session-wide selectors intentionally keep using
// snapshotRankExpr. Every correlated generation check below compares the
// candidate row with the same normalized repository key used by the window
// partition, so another repository cannot change this row's rank.
func snapshotRankExprForRepository(driver string) string {
	outerRepository := snapshotRepositoryExpr(driver, "task_session_git_snapshots")
	newerRepository := snapshotRepositoryExpr(driver, "newer")
	archiveRepository := snapshotRepositoryExpr(driver, "archive")
	return fmt.Sprintf(`
			CASE
				WHEN snapshot_type = 'archive' AND (
					EXISTS (
						SELECT 1 FROM task_sessions ts
						JOIN tasks t ON t.id = ts.task_id
						WHERE ts.id = task_session_git_snapshots.session_id
						  AND t.archived_at IS NOT NULL
					)
					OR NOT EXISTS (
						SELECT 1 FROM task_session_git_snapshots newer
						WHERE newer.session_id = task_session_git_snapshots.session_id
						  AND %s = %s
						  AND newer.snapshot_type <> 'archive'
						  AND newer.created_at > task_session_git_snapshots.created_at
					)
				) THEN 0
				WHEN triggered_by = 'agent_completed' AND EXISTS (
					SELECT 1 FROM task_session_git_snapshots archive
					WHERE archive.session_id = task_session_git_snapshots.session_id
					  AND %s = %s
					  AND archive.snapshot_type = 'archive'
					  AND archive.created_at > task_session_git_snapshots.created_at
				) THEN 3
				WHEN triggered_by = 'agent_completed' THEN 1
				WHEN snapshot_type = 'archive' THEN 3
				ELSE 2
			END,
			created_at DESC,
			id DESC`, newerRepository, outerRepository, archiveRepository, outerRepository)
}

// environmentSnapshotRankExpr orders environment-owned observations by time.
// Snapshot type never outranks a newer observation. When timestamps tie, a row
// with file details wins so equal-time writes remain useful and deterministic.
const environmentSnapshotRankExpr = `
			created_at DESC,
			CASE WHEN COALESCE(TRIM(files), '') NOT IN ('', '{}') THEN 1 ELSE 0 END DESC,
			id DESC`

func environmentSnapshotRankExprForRepository(_ string) string {
	return environmentSnapshotRankExpr
}

// UpsertLatestLiveGitSnapshot keeps at most one cached "live monitor" snapshot
// per environment and repository by deleting the previous row for that repository
// and inserting the new one in a single transaction. This is the cache that
// backs the sidebar diff badge for tasks whose executor isn't currently
// running.
func (r *Repository) UpsertLatestLiveGitSnapshot(ctx context.Context, snapshot *models.GitSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	if err := r.resolveGitSnapshotEnvironment(ctx, snapshot); err != nil {
		return err
	}
	snapshot.SnapshotType = models.SnapshotTypeStatusUpdate
	snapshot.TriggeredBy = TriggeredByLiveMonitor
	if snapshot.ID == "" {
		snapshot.ID = uuid.New().String()
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockGitSnapshotEnvironment(tx, snapshot.TaskEnvironmentID); err != nil {
		return err
	}

	repositoryName := gitSnapshotRepositoryName(snapshot)
	repositoryExpr := "COALESCE(" + dialect.JSONExtract(r.db.DriverName(), "metadata", "repository_name") + ", '')"
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_session_git_snapshots
		WHERE task_environment_id = ? AND snapshot_type = ? AND triggered_by = ? AND `+repositoryExpr+` = ?
	`), snapshot.TaskEnvironmentID, string(models.SnapshotTypeStatusUpdate), TriggeredByLiveMonitor, repositoryName); err != nil {
		return fmt.Errorf("delete previous live snapshot: %w", err)
	}

	filesJSON, metadataJSON, err := serializeSnapshotJSON(snapshot)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_git_snapshots (
			id, task_environment_id, session_id, snapshot_type, branch, remote_branch, head_commit, base_commit,
			ahead, behind, files, triggered_by, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), snapshot.ID, snapshot.TaskEnvironmentID, nullableGitSnapshotSessionID(snapshot.SessionID), string(snapshot.SnapshotType), snapshot.Branch,
		snapshot.RemoteBranch, snapshot.HeadCommit, snapshot.BaseCommit, snapshot.Ahead,
		snapshot.Behind, filesJSON, snapshot.TriggeredBy, metadataJSON, snapshot.CreatedAt); err != nil {
		return fmt.Errorf("insert live snapshot: %w", err)
	}

	return tx.Commit()
}

// serializeSnapshotJSON marshals a snapshot's Files and Metadata maps into
// their JSON-text column forms. Nil maps serialize to "{}" (the sentinel
// that scans back to a nil map), preserving the row shape for both SQLite
// and Postgres.
func serializeSnapshotJSON(snapshot *models.GitSnapshot) (string, string, error) {
	filesJSON := "{}"
	if snapshot.Files != nil {
		b, err := json.Marshal(snapshot.Files)
		if err != nil {
			return "", "", fmt.Errorf("failed to serialize git snapshot files: %w", err)
		}
		filesJSON = string(b)
	}
	metadataJSON := "{}"
	if snapshot.Metadata != nil {
		b, err := json.Marshal(snapshot.Metadata)
		if err != nil {
			return "", "", fmt.Errorf("failed to serialize git snapshot metadata: %w", err)
		}
		metadataJSON = string(b)
	}
	return filesJSON, metadataJSON, nil
}

// DeleteLiveMonitorSnapshots removes all live_monitor-triggered snapshots for a
// session. Called after creating an agent_completed snapshot to prevent stale
// live_monitor data from superseding the authoritative completion snapshot.
func (r *Repository) DeleteLiveMonitorSnapshots(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_session_git_snapshots
		WHERE session_id = ? AND triggered_by = ?
	`), sessionID, TriggeredByLiveMonitor)
	return err
}

// DeleteLiveMonitorSnapshotsByTaskEnvironmentID removes live snapshots for an
// environment, including rows whose session provenance belongs to a sibling
// execution in the same shared workspace.
func (r *Repository) DeleteLiveMonitorSnapshotsByTaskEnvironmentID(ctx context.Context, taskEnvironmentID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_session_git_snapshots
		WHERE task_environment_id = ? AND triggered_by = ?
	`), taskEnvironmentID, TriggeredByLiveMonitor)
	return err
}

// CreateGitSnapshot inserts a new git snapshot into the database.
func (r *Repository) CreateGitSnapshot(ctx context.Context, snapshot *models.GitSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	if err := r.resolveGitSnapshotEnvironment(ctx, snapshot); err != nil {
		return err
	}
	if snapshot.ID == "" {
		snapshot.ID = uuid.New().String()
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}

	filesJSON, metadataJSON, err := serializeSnapshotJSON(snapshot)
	if err != nil {
		return err
	}

	if snapshot.TriggeredBy != triggeredByAgentCompleted {
		_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_git_snapshots (
			id, task_environment_id, session_id, snapshot_type, branch, remote_branch, head_commit, base_commit,
			ahead, behind, files, triggered_by, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), snapshot.ID, snapshot.TaskEnvironmentID, nullableGitSnapshotSessionID(snapshot.SessionID), string(snapshot.SnapshotType), snapshot.Branch,
			snapshot.RemoteBranch, snapshot.HeadCommit, snapshot.BaseCommit, snapshot.Ahead,
			snapshot.Behind, filesJSON, snapshot.TriggeredBy, metadataJSON, snapshot.CreatedAt)
		return err
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin completion snapshot tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockGitSnapshotEnvironment(tx, snapshot.TaskEnvironmentID); err != nil {
		return err
	}
	repositoryName := gitSnapshotRepositoryName(snapshot)
	repositoryExpr := "COALESCE(" + dialect.JSONExtract(r.db.DriverName(), "metadata", "repository_name") + ", '')"
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_session_git_snapshots
		WHERE task_environment_id = ? AND triggered_by IN (?, ?) AND `+repositoryExpr+` = ?
	`), snapshot.TaskEnvironmentID, TriggeredByLiveMonitor, triggeredByAgentCompleted, repositoryName); err != nil {
		return fmt.Errorf("supersede previous completion snapshots: %w", err)
	}
	if _, err := insertGitSnapshot(ctx, tx, r.db, snapshot, filesJSON, metadataJSON); err != nil {
		return err
	}
	return tx.Commit()
}

type gitSnapshotExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func insertGitSnapshot(
	ctx context.Context,
	exec gitSnapshotExecutor,
	db *sqlx.DB,
	snapshot *models.GitSnapshot,
	filesJSON, metadataJSON string,
) (sql.Result, error) {
	return exec.ExecContext(ctx, db.Rebind(`
		INSERT INTO task_session_git_snapshots (
			id, task_environment_id, session_id, snapshot_type, branch, remote_branch, head_commit, base_commit,
			ahead, behind, files, triggered_by, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), snapshot.ID, snapshot.TaskEnvironmentID, nullableGitSnapshotSessionID(snapshot.SessionID), string(snapshot.SnapshotType), snapshot.Branch,
		snapshot.RemoteBranch, snapshot.HeadCommit, snapshot.BaseCommit, snapshot.Ahead,
		snapshot.Behind, filesJSON, snapshot.TriggeredBy, metadataJSON, snapshot.CreatedAt)
}

func (r *Repository) lockGitSnapshotEnvironment(tx *sqlx.Tx, taskEnvironmentID string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext($1))`, taskEnvironmentID); err != nil {
		return fmt.Errorf("lock git snapshot environment %s: %w", taskEnvironmentID, err)
	}

	return nil
}

func (r *Repository) resolveGitSnapshotEnvironment(ctx context.Context, snapshot *models.GitSnapshot) error {
	if snapshot.TaskEnvironmentID != "" {
		return nil
	}
	if snapshot.SessionID == "" {
		return fmt.Errorf("task_environment_id is required for git snapshot")
	}
	var environmentID sql.NullString
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT task_environment_id FROM task_sessions WHERE id = ?
	`), snapshot.SessionID).Scan(&environmentID); err != nil {
		return fmt.Errorf("resolve git snapshot environment for session %s: %w", snapshot.SessionID, err)
	}
	if !environmentID.Valid || environmentID.String == "" {
		return fmt.Errorf("session %s has no task environment for git snapshot", snapshot.SessionID)
	}
	snapshot.TaskEnvironmentID = environmentID.String
	return nil
}

func nullableGitSnapshotSessionID(sessionID string) interface{} {
	if sessionID == "" {
		return nil
	}
	return sessionID
}

func (r *Repository) getGitSnapshotByOrder(ctx context.Context, sessionID, orderDir string) (*models.GitSnapshot, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM task_session_git_snapshots
		WHERE session_id = ?
		ORDER BY created_at %s LIMIT 1
	`, gitSnapshotSelectColumns, orderDir)
	return scanGitSnapshot(r.ro.QueryRowContext(ctx, r.ro.Rebind(query), sessionID))
}

// GetLatestGitSnapshot retrieves the best git snapshot for a session.
// An archive snapshot (terminal cumulative diff captured at archive time) is
// authoritative while its owning task is still archived, and also while the
// task is unarchived but not yet resumed (no newer non-archive snapshot
// exists yet); once a newer agent_completed / live_monitor row describes the
// current execution, it wins over the stale archive row.
// Returns sql.ErrNoRows if no snapshot is found.
func (r *Repository) GetLatestGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error) {
	query := `
		SELECT ` + gitSnapshotSelectColumns + `
		FROM task_session_git_snapshots
		WHERE session_id = ?
		ORDER BY` + snapshotRankExpr + `
		LIMIT 1
	`
	return scanGitSnapshot(r.ro.QueryRowContext(ctx, r.ro.Rebind(query), sessionID))
}

// GetLatestGitSnapshotsBySessionIDs loads one authoritative snapshot per
// session in one query per placeholder-sized chunk. It mirrors
// GetLatestGitSnapshot's ranking (snapshotRankExpr) without introducing an
// N+1 read when task-list summaries repair historical rows.
func (r *Repository) GetLatestGitSnapshotsBySessionIDs(
	ctx context.Context,
	sessionIDs []string,
) (map[string]*models.GitSnapshot, error) {
	result := make(map[string]*models.GitSnapshot, len(sessionIDs))
	snapshots, err := r.queryLatestGitSnapshots(ctx, sessionIDs, "session_id", "session_id", snapshotRankExpr, "session_id")
	if err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		if _, exists := result[snapshot.SessionID]; !exists {
			result[snapshot.SessionID] = snapshot
		}
	}
	return result, nil
}

// GetLatestGitStatusSnapshotsBySessionIDs loads one authoritative snapshot per
// session and repository. Unlike GetLatestGitSnapshotsBySessionIDs, this
// preserves every repository needed to rebuild a multi-repository status
// notification while retaining the same archive/completion/live ranking.
func (r *Repository) GetLatestGitStatusSnapshotsBySessionIDs(
	ctx context.Context,
	sessionIDs []string,
) ([]*models.GitSnapshot, error) {
	repositoryExpr := snapshotRepositoryExpr(r.db.DriverName(), "task_session_git_snapshots")
	rankExpr := snapshotRankExprForRepository(r.db.DriverName())
	return r.queryLatestGitSnapshots(
		ctx,
		sessionIDs,
		"session_id, "+repositoryExpr,
		"session_id",
		rankExpr,
		"session_id, id",
	)
}

// GetLatestGitSnapshotByTaskEnvironmentID retrieves the best current snapshot
// for an environment. The multi-repository status method below is preferred
// when callers need every repository; this method preserves the single-row
// shape used by older summary consumers.
func (r *Repository) GetLatestGitSnapshotByTaskEnvironmentID(ctx context.Context, taskEnvironmentID string) (*models.GitSnapshot, error) {
	query := `
		SELECT ` + gitSnapshotSelectColumns + `
		FROM task_session_git_snapshots
		WHERE task_environment_id = ?
		ORDER BY` + environmentSnapshotRankExpr + `
		LIMIT 1
	`
	return scanGitSnapshot(r.ro.QueryRowContext(ctx, r.ro.Rebind(query), taskEnvironmentID))
}

// GetLatestGitSnapshotsByTaskEnvironmentIDs loads one current snapshot per
// environment. It is the environment-keyed counterpart to the historical
// session batch read.
func (r *Repository) GetLatestGitSnapshotsByTaskEnvironmentIDs(
	ctx context.Context,
	taskEnvironmentIDs []string,
) (map[string]*models.GitSnapshot, error) {
	result := make(map[string]*models.GitSnapshot, len(taskEnvironmentIDs))
	snapshots, err := r.queryLatestGitSnapshots(
		ctx,
		taskEnvironmentIDs,
		"task_environment_id",
		"task_environment_id",
		environmentSnapshotRankExpr,
		"task_environment_id",
	)
	if err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		if _, exists := result[snapshot.TaskEnvironmentID]; !exists {
			result[snapshot.TaskEnvironmentID] = snapshot
		}
	}
	return result, nil
}

// GetLatestGitStatusSnapshotsByTaskEnvironmentIDs loads one authoritative
// current snapshot per environment and normalized repository. A newer sparse
// row is returned as-is; files are never borrowed from an older row.
func (r *Repository) GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(
	ctx context.Context,
	taskEnvironmentIDs []string,
) ([]*models.GitSnapshot, error) {
	repositoryExpr := snapshotRepositoryExpr(r.db.DriverName(), "task_session_git_snapshots")
	rankExpr := environmentSnapshotRankExprForRepository(r.db.DriverName())
	return r.queryLatestGitSnapshots(
		ctx,
		taskEnvironmentIDs,
		"task_environment_id, "+repositoryExpr,
		"task_environment_id",
		rankExpr,
		"task_environment_id, id",
	)
}

func (r *Repository) queryLatestGitSnapshots(
	ctx context.Context,
	ids []string,
	partitionExpr string,
	whereExpr string,
	rankExpr string,
	orderExpr string,
) ([]*models.GitSnapshot, error) {
	result := make([]*models.GitSnapshot, 0)
	if len(ids) == 0 {
		return result, nil
	}
	for _, chunk := range chunkIDs(ids, sqliteMaxHostParams) {
		placeholders, args := buildInPlaceholders(chunk)
		query := `
			SELECT ` + gitSnapshotSelectColumns + `
			FROM (
				SELECT ` + gitSnapshotSelectColumns + `,
				       ROW_NUMBER() OVER (
					       PARTITION BY ` + partitionExpr + `
					       ORDER BY` + rankExpr + `
				       ) AS row_number
				FROM task_session_git_snapshots
				WHERE ` + whereExpr + ` IN (` + placeholders + `)
			) ranked
			WHERE row_number = 1
			ORDER BY ` + orderExpr + `
		`
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			snapshot, scanErr := scanGitSnapshot(rows)
			if scanErr != nil {
				_ = rows.Close()
				return nil, scanErr
			}
			result = append(result, snapshot)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}
	return result, nil
}

func gitSnapshotRepositoryName(snapshot *models.GitSnapshot) string {
	if snapshot != nil && snapshot.Metadata != nil {
		if repositoryName, ok := snapshot.Metadata["repository_name"].(string); ok {
			return repositoryName
		}
	}
	return ""
}

type gitSnapshotScanner interface {
	Scan(dest ...interface{}) error
}

const gitSnapshotSelectColumns = `id, task_environment_id, session_id, snapshot_type, branch, remote_branch, head_commit, base_commit,
	       ahead, behind, files, triggered_by, metadata, created_at`

// scanGitSnapshot scans one task_session_git_snapshots row (all 14 columns)
// into a GitSnapshot, decoding the JSON-text files/metadata columns. The "{}"
// sentinel scans back to nil maps so empty rows round-trip cleanly.
func scanGitSnapshot(scanner gitSnapshotScanner) (*models.GitSnapshot, error) {
	snapshot := &models.GitSnapshot{}
	var snapshotType string
	var sessionID sql.NullString
	var filesJSON sql.NullString
	var metadataJSON sql.NullString
	if err := scanner.Scan(
		&snapshot.ID, &snapshot.TaskEnvironmentID, &sessionID, &snapshotType, &snapshot.Branch,
		&snapshot.RemoteBranch, &snapshot.HeadCommit, &snapshot.BaseCommit,
		&snapshot.Ahead, &snapshot.Behind, &filesJSON, &snapshot.TriggeredBy,
		&metadataJSON, &snapshot.CreatedAt,
	); err != nil {
		return nil, err
	}
	if sessionID.Valid {
		snapshot.SessionID = sessionID.String
	}
	snapshot.SnapshotType = models.SnapshotType(snapshotType)
	if filesJSON.Valid && filesJSON.String != "" && filesJSON.String != "{}" {
		if err := json.Unmarshal([]byte(filesJSON.String), &snapshot.Files); err != nil {
			return nil, fmt.Errorf("failed to deserialize git snapshot files: %w", err)
		}
	}
	if metadataJSON.Valid && metadataJSON.String != "" && metadataJSON.String != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &snapshot.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize git snapshot metadata: %w", err)
		}
	}
	return snapshot, nil
}

// GetFirstGitSnapshot retrieves the oldest git snapshot for a session (first one created).
// Returns sql.ErrNoRows if no snapshot is found.
func (r *Repository) GetFirstGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error) {
	return r.getGitSnapshotByOrder(ctx, sessionID, "ASC")
}

// GetGitSnapshotsBySession retrieves all git snapshots for a session, ordered by created_at descending.
// If limit > 0, only that many snapshots are returned.
// Returns an empty slice if no snapshots are found.
func (r *Repository) GetGitSnapshotsBySession(ctx context.Context, sessionID string, limit int) ([]*models.GitSnapshot, error) {
	query := `
		SELECT ` + gitSnapshotSelectColumns + `
		FROM task_session_git_snapshots
		WHERE session_id = ?
		ORDER BY created_at DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.GitSnapshot
	for rows.Next() {
		snapshot, err := scanGitSnapshot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// CreateSessionCommit inserts a new commit record into the database.
// Idempotent on (session_id, commit_sha): a commit already observed for this
// session is silently skipped rather than duplicated, since it can now be
// reported from more than one trigger (live commit events, the per-turn
// reconcile sweep, and archive capture) for the same underlying git commit.
// The returned bool reports whether a new row was actually inserted, so
// callers can distinguish a fresh observation from a re-observed duplicate
// for writer-health counters.
//
// pre_commit_snapshot_id / post_commit_snapshot_id are always written empty
// (commit.PreCommitSnapshotID / PostCommitSnapshotID are never populated by
// any caller). No writer for either column exists anywhere in this codebase;
// they do not link a commit row to the task_session_git_snapshots chain.
// Treat them as reserved/unpopulated, not as live data.
func (r *Repository) CreateSessionCommit(ctx context.Context, commit *models.SessionCommit) (bool, error) {
	if commit.ID == "" {
		commit.ID = uuid.New().String()
	}
	if commit.CreatedAt.IsZero() {
		commit.CreatedAt = time.Now().UTC()
	}

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_commits (
			id, session_id, commit_sha, parent_sha, author_name, author_email,
			commit_message, committed_at, pre_commit_snapshot_id, post_commit_snapshot_id,
			files_changed, insertions, deletions, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id, commit_sha) DO NOTHING
	`), commit.ID, commit.SessionID, commit.CommitSHA, commit.ParentSHA,
		commit.AuthorName, commit.AuthorEmail, commit.CommitMessage, commit.CommittedAt,
		commit.PreCommitSnapshotID, commit.PostCommitSnapshotID, commit.FilesChanged,
		commit.Insertions, commit.Deletions, commit.CreatedAt)
	if err != nil {
		return false, err
	}

	// RowsAffected returns 0 on a conflict-skipped row in both SQLite and Postgres.
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// GetSessionCommits retrieves all commits for a session, ordered by committed_at descending.
// Returns an empty slice if no commits are found.
func (r *Repository) GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
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

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
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
