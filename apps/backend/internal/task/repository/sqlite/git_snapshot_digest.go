// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// gitSnapshotDigestPayload is the canonical (field-order-stable, since
// encoding/json marshals struct fields in declaration order) shape hashed by
// computeGitSnapshotDigest. It deliberately excludes session_id,
// snapshot_type, triggered_by and created_at: those describe *why* and *for
// whom* a snapshot was captured, not the repository state it describes, so
// two rows with an identical state but different lifecycle context are
// still identified as content-equivalent by this digest.
type gitSnapshotDigestPayload struct {
	Branch       string `json:"branch"`
	RemoteBranch string `json:"remote_branch"`
	HeadCommit   string `json:"head_commit"`
	BaseCommit   string `json:"base_commit"`
	Ahead        int    `json:"ahead"`
	Behind       int    `json:"behind"`
	Files        string `json:"files"`
}

// computeGitSnapshotDigest returns the SHA-256 hex digest identifying a git
// snapshot's repository-state content, independent of session/lifecycle
// metadata. filesJSON is the already-serialized Files column value (see
// serializeSnapshotJSON) so callers never need to re-marshal it.
func computeGitSnapshotDigest(branch, remoteBranch, headCommit, baseCommit string, ahead, behind int, filesJSON string) string {
	// json.Marshal on a fixed struct never errors (no channels/funcs/cycles),
	// so the error is deliberately ignored rather than threaded through every
	// caller of what is otherwise a pure function.
	payload, _ := json.Marshal(gitSnapshotDigestPayload{
		Branch: branch, RemoteBranch: remoteBranch, HeadCommit: headCommit,
		BaseCommit: baseCommit, Ahead: ahead, Behind: behind, Files: filesJSON,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// migrateGitSnapshotContentDigest adds task_session_git_snapshots.
// content_digest and backfills it for every existing row, so historical
// content-duplicate rows become identifiable via
// ListDuplicateGitSnapshotCandidates for a later, explicit maintenance pass
// to prune.
func (r *Repository) migrateGitSnapshotContentDigest() error {
	r.migrate.Apply("task_session_git_snapshots.content_digest", `ALTER TABLE task_session_git_snapshots ADD COLUMN content_digest TEXT NOT NULL DEFAULT ''`)
	r.migrate.Apply("idx_git_snapshots_session_digest", `CREATE INDEX IF NOT EXISTS idx_git_snapshots_session_digest ON task_session_git_snapshots(session_id, content_digest)`)
	return r.backfillGitSnapshotContentDigests()
}

// backfillGitSnapshotContentDigests computes content_digest for every row
// that doesn't have one yet (fresh installs never do; upgraded installs
// have historical rows to backfill once). SQLite/Postgres have no portable
// SHA-256 SQL function, so this iterates in Go rather than a single UPDATE.
func (r *Repository) backfillGitSnapshotContentDigests() error {
	rows, err := r.db.Queryx(`
		SELECT id, branch, remote_branch, head_commit, base_commit, ahead, behind, files
		FROM task_session_git_snapshots WHERE content_digest = ''`)
	if err != nil {
		return fmt.Errorf("query git snapshots for content_digest backfill: %w", err)
	}
	type pending struct {
		id     string
		digest string
	}
	var updates []pending
	for rows.Next() {
		var id, branch, remoteBranch, headCommit, baseCommit, filesJSON string
		var ahead, behind int
		if err := rows.Scan(&id, &branch, &remoteBranch, &headCommit, &baseCommit, &ahead, &behind, &filesJSON); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan git snapshot for content_digest backfill: %w", err)
		}
		updates = append(updates, pending{id: id, digest: computeGitSnapshotDigest(branch, remoteBranch, headCommit, baseCommit, ahead, behind, filesJSON)})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate git snapshots for content_digest backfill: %w", err)
	}
	_ = rows.Close()

	for _, u := range updates {
		if _, err := r.db.Exec(r.db.Rebind(`
			UPDATE task_session_git_snapshots SET content_digest = ? WHERE id = ?
		`), u.digest, u.id); err != nil {
			return fmt.Errorf("backfill content_digest for git snapshot %s: %w", u.id, err)
		}
	}
	return nil
}

// DuplicateGitSnapshotCandidate identifies a non-authoritative git snapshot
// row that shares its (session_id, content_digest) pair with a newer row for
// the same session. Archive and agent-completed rows are never candidates:
// their lifecycle purpose can make them authoritative even when a newer live
// observation has identical Git content.
type DuplicateGitSnapshotCandidate struct {
	ID            string
	SessionID     string
	ContentDigest string
}

// ListDuplicateGitSnapshotCandidates returns content-duplicate git snapshot
// rows across all sessions: for every (session_id, content_digest) group
// with more than one row, superseded live/status rows are reported while
// archive and agent-completed rows remain protected. Read-only and
// non-destructive - CreateGitSnapshot always records every poll (per the
// plan's "destructive maintenance is explicit, dry-run-first" constraint),
// so this is the sole place duplicates are identified for explicit
// maintenance to act on.
func (r *Repository) ListDuplicateGitSnapshotCandidates(ctx context.Context, limit int) ([]DuplicateGitSnapshotCandidate, error) {
	query := `
		SELECT s.id, s.session_id, s.content_digest
		FROM task_session_git_snapshots s
		WHERE s.content_digest <> ''
		  AND s.snapshot_type <> 'archive'
		  AND s.triggered_by <> 'agent_completed'
		  AND EXISTS (
			  SELECT 1 FROM task_session_git_snapshots newer
			  WHERE newer.session_id = s.session_id
				AND newer.content_digest = s.content_digest
				AND (
					newer.created_at > s.created_at
					OR (newer.created_at = s.created_at AND newer.id > s.id)
				)
		  )
		ORDER BY s.session_id, s.created_at ASC`
	return listLimitedCandidates(ctx, r.ro, query, limit, "duplicate git snapshot candidates",
		func(rows *sql.Rows) (DuplicateGitSnapshotCandidate, error) {
			var c DuplicateGitSnapshotCandidate
			err := rows.Scan(&c.ID, &c.SessionID, &c.ContentDigest)
			return c, err
		})
}
