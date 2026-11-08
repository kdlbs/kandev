package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
)

const mergeProposalsSQL = `
	CREATE TABLE IF NOT EXISTS coordinator_merge_proposals (
		proposal_id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		coordinator_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		change_key TEXT NOT NULL,
		head_sha TEXT NOT NULL,
		method TEXT NOT NULL,
		automatic_revision INTEGER,
		manager_only INTEGER NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT '',
		last_reason TEXT NOT NULL DEFAULT '',
		open_key TEXT,
		created_at {{timestamp}} NOT NULL
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_merge_open ON coordinator_merge_proposals(workspace_id, open_key);
	CREATE INDEX IF NOT EXISTS idx_coordinator_merge_coordinator ON coordinator_merge_proposals(coordinator_id);
	CREATE TABLE IF NOT EXISTS coordinator_merge_dispatches (
		proposal_id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		change_key TEXT NOT NULL,
		head_sha TEXT NOT NULL,
		claim_token TEXT NOT NULL,
		automatic INTEGER NOT NULL DEFAULT 0,
		automatic_since_revision INTEGER NOT NULL DEFAULT 0,
		queued INTEGER NOT NULL DEFAULT 0,
		outcome TEXT NOT NULL DEFAULT '',
		merge_commit TEXT NOT NULL DEFAULT '',
		lowering TEXT NOT NULL DEFAULT '',
		dispatched_at {{timestamp}} NOT NULL,
		settled_at {{timestamp}}
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_merge_dispatch_unsettled
		ON coordinator_merge_dispatches(workspace_id, change_key, head_sha) WHERE settled_at IS NULL;
	DROP INDEX IF EXISTS coordinator_proposals_open_target;
	CREATE UNIQUE INDEX IF NOT EXISTS coordinator_proposals_open_target_v2 ON coordinator_proposals(coordinator_id, kind, target_task_id)
		WHERE kind NOT IN ('create_task', 'merge') AND status IN ('pending','approving','failed');
`

func (s *Store) migrateMergeProposals() error {
	if _, err := s.db.Exec(dialect.MustRenderSchema(s.db.DriverName(), mergeProposalsSQL)); err != nil {
		return fmt.Errorf("coordinator merge proposals schema: %w", err)
	}
	return nil
}

const mergeOpenStatuses = `'pending','approving','failed','uncertain'`

const mergeProposalColumns = `proposal_id, workspace_id, coordinator_id, repository_id, change_key, head_sha, method,
	automatic_revision, manager_only, note, last_reason, created_at`

func scanMergeProposal(row interface{ Scan(...any) error }) (*MergeProposal, error) {
	var m MergeProposal
	var rev sql.NullInt64
	var managerOnly int
	if err := row.Scan(&m.ProposalID, &m.WorkspaceID, &m.CoordinatorID, &m.RepositoryID, &m.ChangeKey, &m.HeadSHA, &m.Method,
		&rev, &managerOnly, &m.Note, &m.LastReason, &m.CreatedAt); err != nil {
		return nil, err
	}
	if rev.Valid {
		n := int(rev.Int64)
		m.AutomaticRevision = &n
	}
	m.ManagerOnly = managerOnly != 0
	return &m, nil
}

// insertMergeRowTx stores the merge detail of a just-inserted proposal. It
// frees the change key of any proposal that has since settled, then claims it;
// the unique index refuses a second open proposal for one change in a
// workspace.
func (s *Store) insertMergeRowTx(ctx context.Context, tx coordinatorExec, m MergeProposal) error {
	if dialect.IsPostgres(s.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "coordinator_merge_open|"+m.WorkspaceID+"|"+m.ChangeKey); err != nil {
			return fmt.Errorf("lock merge change: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_merge_proposals SET open_key = NULL
		WHERE workspace_id = ? AND open_key = ? AND NOT EXISTS (
			SELECT 1 FROM coordinator_proposals p WHERE p.id = coordinator_merge_proposals.proposal_id AND p.status IN (`+mergeOpenStatuses+`))`),
		m.WorkspaceID, m.ChangeKey); err != nil {
		return fmt.Errorf("free merge change key: %w", err)
	}
	var rev any
	if m.AutomaticRevision != nil {
		rev = *m.AutomaticRevision
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_merge_proposals
		(proposal_id, workspace_id, coordinator_id, repository_id, change_key, head_sha, method, automatic_revision,
		 manager_only, note, last_reason, open_key, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, '', '', ?, ?)`),
		m.ProposalID, m.WorkspaceID, m.CoordinatorID, m.RepositoryID, m.ChangeKey, m.HeadSHA, m.Method, rev,
		m.ChangeKey, s.now().UTC()); err != nil {
		return fmt.Errorf("insert merge proposal: %w", err)
	}
	return nil
}

// getMergeProposal reads the merge detail of a proposal.
func (s *Store) getMergeProposal(ctx context.Context, q coordinatorExec, proposalID string) (*MergeProposal, error) {
	m, err := scanMergeProposal(q.QueryRowContext(ctx, s.db.Rebind(
		`SELECT `+mergeProposalColumns+` FROM coordinator_merge_proposals WHERE proposal_id = ?`), proposalID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// openMergeForChange returns the merge detail of the open proposal for a change
// in the workspace, or nil. An uncertain proposal is open.
func (s *Store) openMergeForChange(ctx context.Context, q coordinatorExec, workspaceID, changeKey string) (*MergeProposal, error) {
	m, err := scanMergeProposal(q.QueryRowContext(ctx, s.db.Rebind(`SELECT `+prefixed("m.", mergeProposalColumns)+`
		FROM coordinator_merge_proposals m JOIN coordinator_proposals p ON p.id = m.proposal_id
		WHERE m.workspace_id = ? AND m.change_key = ? AND p.status IN (`+mergeOpenStatuses+`)`), workspaceID, changeKey))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

// listOpenMerges lists the open merge proposals of one coordinator, oldest first.
func (s *Store) listOpenMerges(ctx context.Context, q coordinatorExec, coordinatorID string) ([]*MergeProposal, error) {
	return s.queryMerges(ctx, q, `SELECT `+prefixed("m.", mergeProposalColumns)+`
		FROM coordinator_merge_proposals m JOIN coordinator_proposals p ON p.id = m.proposal_id
		WHERE m.coordinator_id = ? AND p.status IN (`+mergeOpenStatuses+`) ORDER BY m.created_at, m.proposal_id`, coordinatorID)
}

func (s *Store) queryMerges(ctx context.Context, q coordinatorExec, query string, args ...any) ([]*MergeProposal, error) {
	rows, err := q.QueryContext(ctx, s.db.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list merge proposals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*MergeProposal
	for rows.Next() {
		m, err := scanMergeProposal(rows)
		if err != nil {
			return nil, fmt.Errorf("scan merge proposal: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// prefixed qualifies each column of a comma-separated list with prefix.
func prefixed(prefix, columns string) string {
	parts := strings.Split(columns, ",")
	for i, c := range parts {
		parts[i] = prefix + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}

// setMergeStateTx records the manager-only mark and the last waiting reason.
func (s *Store) setMergeStateTx(ctx context.Context, tx coordinatorExec, proposalID string, managerOnly bool, note, lastReason string) error {
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_merge_proposals SET manager_only = ?, note = ?, last_reason = ?
		WHERE proposal_id = ?`), autonomyColumn(managerOnly), note, lastReason, proposalID); err != nil {
		return fmt.Errorf("set merge state: %w", err)
	}
	return nil
}

// insertDispatchTx writes the dispatch record. It fails when the proposal
// already has one or another unsettled dispatch holds the change and head.
func (s *Store) insertDispatchTx(ctx context.Context, tx coordinatorExec, d MergeDispatch) error {
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_merge_dispatches
		(proposal_id, workspace_id, change_key, head_sha, claim_token, automatic, automatic_since_revision, dispatched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		d.ProposalID, d.WorkspaceID, d.ChangeKey, d.HeadSHA, d.ClaimToken, autonomyColumn(d.Automatic),
		d.AutomaticSinceRevision, s.now().UTC()); err != nil {
		return fmt.Errorf("insert merge dispatch: %w", err)
	}
	return nil
}

const dispatchColumns = `proposal_id, workspace_id, change_key, head_sha, claim_token, automatic, automatic_since_revision,
	queued, outcome, merge_commit, lowering, dispatched_at, settled_at`

func scanDispatch(row interface{ Scan(...any) error }) (*MergeDispatch, error) {
	var d MergeDispatch
	var automatic, queued int
	var settled sql.NullTime
	if err := row.Scan(&d.ProposalID, &d.WorkspaceID, &d.ChangeKey, &d.HeadSHA, &d.ClaimToken, &automatic,
		&d.AutomaticSinceRevision, &queued, &d.Outcome, &d.MergeCommit, &d.Lowering, &d.DispatchedAt, &settled); err != nil {
		return nil, err
	}
	d.Automatic, d.Queued = automatic != 0, queued != 0
	if settled.Valid {
		t := settled.Time
		d.SettledAt = &t
	}
	return &d, nil
}

// getDispatch reads the dispatch record of a proposal, or nil.
func (s *Store) getDispatch(ctx context.Context, q coordinatorExec, proposalID string) (*MergeDispatch, error) {
	d, err := scanDispatch(q.QueryRowContext(ctx, s.db.Rebind(
		`SELECT `+dispatchColumns+` FROM coordinator_merge_dispatches WHERE proposal_id = ?`), proposalID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return d, err
}

// settleDispatchTx settles an unsettled dispatch with its outcome and, for a
// merge, the merge commit. It reports whether a row was unsettled.
func (s *Store) settleDispatchTx(ctx context.Context, tx coordinatorExec, proposalID, outcome, mergeCommit string, now time.Time) (bool, error) {
	res, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_merge_dispatches SET outcome = ?, merge_commit = ?, settled_at = ?
		WHERE proposal_id = ? AND settled_at IS NULL`), outcome, mergeCommit, now.UTC(), proposalID)
	if err != nil {
		return false, fmt.Errorf("settle merge dispatch: %w", err)
	}
	return matchedRow(res)
}

// markDispatchQueued records that the provider answered queued or in progress.
func (s *Store) markDispatchQueued(ctx context.Context, tx coordinatorExec, proposalID string) error {
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_merge_dispatches SET queued = 1
		WHERE proposal_id = ? AND settled_at IS NULL`), proposalID); err != nil {
		return fmt.Errorf("mark dispatch queued: %w", err)
	}
	return nil
}

// setLoweringTx moves the lowering state of a dispatch from one value to
// another and reports whether it moved.
func (s *Store) setLoweringTx(ctx context.Context, tx coordinatorExec, proposalID, from, to string) (bool, error) {
	res, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_merge_dispatches SET lowering = ?
		WHERE proposal_id = ? AND lowering = ?`), to, proposalID, from)
	if err != nil {
		return false, fmt.Errorf("set dispatch lowering: %w", err)
	}
	return matchedRow(res)
}

// proposalByIDTx reads one proposal of any kind by id.
func (s *Store) proposalByIDTx(ctx context.Context, q coordinatorExec, id string) (*Proposal, error) {
	rows, err := q.QueryContext(ctx, s.db.Rebind(`SELECT `+proposalColumns+` FROM coordinator_proposals WHERE id = ?`), id)
	if err != nil {
		return nil, fmt.Errorf("read proposal: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, ErrNotFound
	}
	var r proposalRow
	if err := scanProposalRow(rows, &r); err != nil {
		return nil, err
	}
	return r.toProposal()
}

// mergeTransition is one status change of a merge proposal.
type mergeTransition struct {
	ID string
	// From lists the statuses the change may start from. Token, when set,
	// fences the approving status to that claim; an uncertain row is matched
	// whatever the token.
	From  []ProposalStatus
	Token string
	// ClaimedBefore, when set, matches an approving row claimed before it.
	ClaimedBefore *time.Time
	To            ProposalStatus
	Code          string
	OutcomeJSON   string
}

// transitionMergeTx applies the change and reports whether a row matched.
func (s *Store) transitionMergeTx(ctx context.Context, tx coordinatorExec, t mergeTransition, now time.Time) (bool, error) {
	clauses := make([]string, 0, len(t.From))
	args := []any{string(t.To), t.Code, t.OutcomeJSON, t.OutcomeJSON, now.UTC(), t.ID}
	for _, from := range t.From {
		clause := "status = ?"
		args = append(args, string(from))
		if from == ProposalStatusApproving && t.Token != "" {
			clause = "(status = ? AND claim_token = ?)"
			args = append(args, t.Token)
		}
		if from == ProposalStatusApproving && t.ClaimedBefore != nil {
			clause = "(status = ? AND claimed_at < ?)"
			args = append(args, t.ClaimedBefore.UTC())
		}
		clauses = append(clauses, clause)
	}
	clearClaim := ""
	if t.To == ProposalStatusPending {
		clearClaim = ", claimed_at = NULL"
	}
	res, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_proposals
		SET status = ?, error = NULLIF(?, ''), outcome_json = CASE WHEN ? = '' THEN outcome_json ELSE ? END,
		    claim_token = NULL, updated_at = ?`+clearClaim+`
		WHERE id = ? AND (`+strings.Join(clauses, " OR ")+`)`), args...)
	if err != nil {
		return false, fmt.Errorf("transition merge proposal: %w", err)
	}
	return matchedRow(res)
}
