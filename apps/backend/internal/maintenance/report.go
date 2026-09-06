package maintenance

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// CandidateGroup summarizes one retention category's policy-eligible rows.
// Only counts and byte estimates are carried - never row content, IDs, task
// identifiers, or any other field that could leak conversation or PR data
// into a report that might be logged or printed.
type CandidateGroup struct {
	RowCount int
	EstBytes int64
}

// Report is the dry-run (and pre-execution) summary of retention
// candidates and current storage size. Safe to print or log verbatim.
type Report struct {
	GeneratedAt             time.Time
	DatabaseSizeBytes       int64
	DuplicateGitSnapshots   CandidateGroup
	ObsoletePlanRevisions   CandidateGroup
	OrphanedMessagePayloads CandidateGroup
}

// TotalCandidateRows sums the row counts across every retention category.
func (r Report) TotalCandidateRows() int {
	return r.DuplicateGitSnapshots.RowCount + r.ObsoletePlanRevisions.RowCount + r.OrphanedMessagePayloads.RowCount
}

// TotalEstBytes sums the estimated reclaimable bytes across every retention
// category. This is an estimate of row content size, not the eventual disk
// space reclaimed by a VACUUM (which depends on page layout); the report
// clarifies that distinction to callers.
func (r Report) TotalEstBytes() int64 {
	return r.DuplicateGitSnapshots.EstBytes + r.ObsoletePlanRevisions.EstBytes + r.OrphanedMessagePayloads.EstBytes
}

// candidateSet holds the concrete row identifiers behind a Report so a
// later Execute call deletes exactly what Analyze reported, without
// re-querying and risking a report/execution race within the same run.
type candidateSet struct {
	gitSnapshotIDs  []string
	planRevisionIDs []string
	payloadDigests  []string
}

// AnalyzeOptions configures how Analyze selects retention candidates.
type AnalyzeOptions struct {
	// KeepPlanRevisions additionally protects this many of the most recent
	// non-HEAD plan revisions per task, on top of the HEAD/ancestry
	// protections ListObsoletePlanRevisionCandidates always applies. Zero
	// means "no extra recency window" (only HEAD/ancestry are protected).
	KeepPlanRevisions int
	// CandidateLimit caps how many rows each category query returns. Zero
	// means unlimited. Intended for very large databases where a bounded
	// dry-run report is preferable to scanning every candidate row.
	CandidateLimit int
}

// analyze computes the current Report and the concrete candidate ID sets
// behind it, using repo's read-only candidate queries plus a handful of
// direct byte-estimate aggregate queries against the same connection.
// Purely read-only: no row is ever written or deleted.
func analyze(ctx context.Context, repo *sqlite.Repository, opts AnalyzeOptions) (Report, candidateSet, error) {
	report := Report{GeneratedAt: time.Now().UTC()}
	var set candidateSet

	dupSnapshots, err := repo.ListDuplicateGitSnapshotCandidates(ctx, opts.CandidateLimit)
	if err != nil {
		return Report{}, candidateSet{}, fmt.Errorf("list duplicate git snapshot candidates: %w", err)
	}
	for _, c := range dupSnapshots {
		set.gitSnapshotIDs = append(set.gitSnapshotIDs, c.ID)
	}
	gitBytes, err := sumGitSnapshotBytes(ctx, repo.DB(), set.gitSnapshotIDs)
	if err != nil {
		return Report{}, candidateSet{}, fmt.Errorf("estimate duplicate git snapshot bytes: %w", err)
	}
	report.DuplicateGitSnapshots = CandidateGroup{RowCount: len(dupSnapshots), EstBytes: gitBytes}

	taskIDs, err := distinctPlanRevisionTaskIDs(ctx, repo.DB())
	if err != nil {
		return Report{}, candidateSet{}, fmt.Errorf("list plan revision task ids: %w", err)
	}
	var planRevisionBytes int64
	remaining := opts.CandidateLimit
	for _, taskID := range taskIDs {
		perTaskLimit := 0
		if opts.CandidateLimit > 0 {
			if remaining <= 0 {
				break
			}
			perTaskLimit = remaining
		}
		revisions, err := repo.ListObsoletePlanRevisionCandidates(ctx, taskID, opts.KeepPlanRevisions)
		if err != nil {
			return Report{}, candidateSet{}, fmt.Errorf("list obsolete plan revision candidates for task %s: %w", taskID, err)
		}
		if perTaskLimit > 0 && len(revisions) > perTaskLimit {
			revisions = revisions[:perTaskLimit]
		}
		for _, rev := range revisions {
			set.planRevisionIDs = append(set.planRevisionIDs, rev.ID)
			planRevisionBytes += rev.ContentBytes
		}
		if opts.CandidateLimit > 0 {
			remaining -= len(revisions)
		}
	}
	report.ObsoletePlanRevisions = CandidateGroup{RowCount: len(set.planRevisionIDs), EstBytes: planRevisionBytes}

	orphanedPayloads, err := repo.ListOrphanedMessagePayloadCandidates(ctx, opts.CandidateLimit)
	if err != nil {
		return Report{}, candidateSet{}, fmt.Errorf("list orphaned message payload candidates: %w", err)
	}
	var payloadBytes int64
	for _, c := range orphanedPayloads {
		set.payloadDigests = append(set.payloadDigests, c.Digest)
		payloadBytes += c.CompressedSize
	}
	report.OrphanedMessagePayloads = CandidateGroup{RowCount: len(orphanedPayloads), EstBytes: payloadBytes}

	return report, set, nil
}

// distinctPlanRevisionTaskIDs enumerates every task_id present in
// task_plan_revisions, so the maintenance run can call the existing
// per-task ListObsoletePlanRevisionCandidates once per task. Read-only.
func distinctPlanRevisionTaskIDs(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT task_id FROM task_plan_revisions ORDER BY task_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query distinct plan revision task ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var taskID string
		if err := rows.Scan(&taskID); err != nil {
			return nil, fmt.Errorf("scan plan revision task id: %w", err)
		}
		out = append(out, taskID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plan revision task ids: %w", err)
	}
	return out, nil
}

// sumGitSnapshotBytesChunkSize bounds each IN clause below SQLite's default
// SQLITE_MAX_VARIABLE_NUMBER (999 in most builds), leaving headroom.
const sumGitSnapshotBytesChunkSize = 400

// sumGitSnapshotBytes estimates the on-disk content size of the given git
// snapshot rows by summing the length of their text columns. This is an
// estimate (it ignores SQLite's own row/page overhead), not an exact
// reclaimable-byte count; DuplicateGitSnapshotCandidate carries no
// pre-computed size column today; adding one would touch Task 05's already
// tested struct shape, so this query - scoped to exactly the candidate IDs
// Analyze already selected - is a cheaper approach.
func sumGitSnapshotBytes(ctx context.Context, db *sql.DB, ids []string) (int64, error) {
	var total int64
	for start := 0; start < len(ids); start += sumGitSnapshotBytesChunkSize {
		end := start + sumGitSnapshotBytesChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf(`
			SELECT COALESCE(SUM(
				LENGTH(files) + LENGTH(metadata) + LENGTH(branch) + LENGTH(remote_branch) +
				LENGTH(head_commit) + LENGTH(base_commit) + LENGTH(triggered_by) + 96
			), 0)
			FROM task_session_git_snapshots WHERE id IN (%s)`, placeholders)
		args := make([]interface{}, len(batch))
		for i, id := range batch {
			args[i] = id
		}
		var sum int64
		if err := db.QueryRowContext(ctx, query, args...).Scan(&sum); err != nil {
			return 0, fmt.Errorf("sum git snapshot bytes: %w", err)
		}
		total += sum
	}
	return total, nil
}
