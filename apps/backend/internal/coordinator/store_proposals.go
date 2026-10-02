package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// maxOpenProposals is the per-coordinator cap on proposals whose status is
// pending, approving or failed (docs/specs/coordinator/system-design/
// proposals.md#propose, Build decision 9's "open" definition).
const maxOpenProposals = 25

// ErrCoordinatorProposalCapReached is returned by InsertProposal when the
// coordinator already holds maxOpenProposals open proposals.
var ErrCoordinatorProposalCapReached = errors.New("coordinator: open proposal cap reached")

// openProposalStatuses are the statuses counted against maxOpenProposals and
// reported as a coordinator's open_proposals count (decision 9).
var openProposalStatuses = []ProposalStatus{ProposalStatusPending, ProposalStatusApproving, ProposalStatusFailed}

const proposalColumns = `id, coordinator_id, workspace_id, status, spec_json, final_spec_json, claimed_at, claim_token, task_id, error, reject_reason, decided_by, created_at, updated_at, kind, target_task_id, standing_order_ids, starts_agent, outcome_json`

// proposalRow is the DB scan target for coordinator_proposals.
type proposalRow struct {
	ID            string         `db:"id"`
	CoordinatorID string         `db:"coordinator_id"`
	WorkspaceID   string         `db:"workspace_id"`
	Status        string         `db:"status"`
	SpecJSON      string         `db:"spec_json"`
	FinalSpecJSON sql.NullString `db:"final_spec_json"`
	ClaimedAt     sql.NullTime   `db:"claimed_at"`
	ClaimToken    sql.NullString `db:"claim_token"`
	TaskID        sql.NullString `db:"task_id"`
	Error         sql.NullString `db:"error"`
	RejectReason  sql.NullString `db:"reject_reason"`
	DecidedBy     sql.NullString `db:"decided_by"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
	Kind          string         `db:"kind"`
	TargetTaskID  sql.NullString `db:"target_task_id"`
	StandingIDs   string         `db:"standing_order_ids"`
	StartsAgent   bool           `db:"starts_agent"`
	OutcomeJSON   sql.NullString `db:"outcome_json"`
}

func (r *proposalRow) toProposal() (*Proposal, error) {
	var spec ProposalSpec
	rawSpec := ""
	if r.Kind == "" || r.Kind == ProposalKindCreateTask {
		if err := json.Unmarshal([]byte(r.SpecJSON), &spec); err != nil {
			return nil, fmt.Errorf("unmarshal proposal spec: %w", err)
		}
	} else {
		rawSpec = r.SpecJSON
	}
	p := &Proposal{
		RawSpec:       rawSpec,
		ID:            r.ID,
		CoordinatorID: r.CoordinatorID,
		WorkspaceID:   r.WorkspaceID,
		Status:        ProposalStatus(r.Status),
		Spec:          spec,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
		Kind:          r.Kind,
		StartsAgent:   r.StartsAgent,
	}
	p.StandingOrderIDs = []string{}
	if r.StandingIDs != "" {
		if err := json.Unmarshal([]byte(r.StandingIDs), &p.StandingOrderIDs); err != nil {
			return nil, fmt.Errorf("unmarshal proposal standing order ids: %w", err)
		}
	}
	if r.TargetTaskID.Valid {
		p.TargetTaskID = &r.TargetTaskID.String
	}
	if r.OutcomeJSON.Valid {
		p.OutcomeJSON = &r.OutcomeJSON.String
	}
	if r.FinalSpecJSON.Valid && rawSpec != "" {
		p.RawFinalSpec = r.FinalSpecJSON.String
	} else if r.FinalSpecJSON.Valid {
		var final ProposalSpec
		if err := json.Unmarshal([]byte(r.FinalSpecJSON.String), &final); err != nil {
			return nil, fmt.Errorf("unmarshal proposal final spec: %w", err)
		}
		p.FinalSpec = &final
	}
	r.copyNullables(p)
	return p, nil
}

// copyNullables copies the nullable columns that map to optional Proposal fields.
func (r *proposalRow) copyNullables(p *Proposal) {
	if r.ClaimedAt.Valid {
		p.ClaimedAt = &r.ClaimedAt.Time
	}
	if r.ClaimToken.Valid {
		p.ClaimToken = &r.ClaimToken.String
	}
	if r.TaskID.Valid {
		p.TaskID = &r.TaskID.String
	}
	if r.Error.Valid {
		p.Error = &r.Error.String
	}
	if r.RejectReason.Valid {
		p.RejectReason = &r.RejectReason.String
	}
	if r.DecidedBy.Valid {
		p.DecidedBy = &r.DecidedBy.String
	}
}

// ProposalPre runs inside the coordinator lock before the cap check. A
// non-nil proposal it returns is the outcome of the call: nothing is
// inserted and the value is copied into the caller's proposal.
type ProposalPre func(ctx context.Context, tx coordinatorExec) (*Proposal, error)

// ProposalInTx runs inside the coordinator lock after the insert, with the
// inserted proposal.
type ProposalInTx func(ctx context.Context, tx coordinatorExec, p *Proposal) error

// InsertProposal locks the coordinator's row (the same per-coordinator lock
// PatchCoordinator uses), refuses at maxOpenProposals open proposals, and
// otherwise inserts p as pending, assigning its id and timestamps.
// ErrNotFound if the coordinator does not exist in p.WorkspaceID.
func (s *Store) InsertProposal(ctx context.Context, p *Proposal, phase2 bool) error {
	return s.InsertProposalWith(ctx, p, phase2, nil, nil)
}

// InsertProposalWith is InsertProposal with hooks that run in the same locked
// transaction: pre before the cap check, inTx after the insert. Both follow
// the withCoordinatorLock rule for what they may touch.
func (s *Store) InsertProposalWith(ctx context.Context, p *Proposal, phase2 bool, pre ProposalPre, inTx ProposalInTx) error {
	return s.withCoordinatorLock(ctx, p.CoordinatorID, func(tx coordinatorExec) error {
		return s.insertProposalBody(ctx, tx, p, phase2, pre, inTx)
	})
}

// insertProposalBody checks the workspace scope (404 if absent), runs pre,
// counts open proposals, refuses at the cap, else inserts p as pending and
// runs inTx.
func (s *Store) insertProposalBody(ctx context.Context, exec coordinatorExec, p *Proposal, phase2 bool, pre ProposalPre, inTx ProposalInTx) error {
	if _, err := lockedCoordinatorRow(ctx, exec, s.db.Rebind, p.WorkspaceID, p.CoordinatorID, false); err != nil {
		return err
	}
	if pre != nil {
		done, err := pre(ctx, exec)
		if err != nil {
			return err
		}
		if done != nil {
			*p = *done
			return nil
		}
	}

	open, err := s.CountOpenProposalsTx(ctx, exec, p.CoordinatorID, phase2)
	if err != nil {
		return err
	}
	if open >= maxOpenProposals {
		return ErrCoordinatorProposalCapReached
	}

	specJSON := []byte(p.RawSpec)
	if p.RawSpec == "" {
		var err error
		if specJSON, err = json.Marshal(p.Spec); err != nil {
			return fmt.Errorf("marshal proposal spec: %w", err)
		}
	}
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	if p.Kind == "" {
		p.Kind = ProposalKindCreateTask
	}
	if p.StandingOrderIDs == nil {
		p.StandingOrderIDs = []string{}
	}
	idsJSON, err := json.Marshal(p.StandingOrderIDs)
	if err != nil {
		return fmt.Errorf("marshal standing order ids: %w", err)
	}
	now := s.now()
	p.Status = ProposalStatusPending
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err = exec.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO coordinator_proposals (`+proposalColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		p.ID, p.CoordinatorID, p.WorkspaceID, string(p.Status), string(specJSON),
		nil, nil, nil, nil, nil, nil, nil, p.CreatedAt, p.UpdatedAt,
		p.Kind, nullableString(p.TargetTaskID), string(idsJSON), p.StartsAgent, nullableString(p.OutcomeJSON))
	if err != nil {
		return fmt.Errorf("insert proposal: %w", err)
	}
	if inTx != nil {
		return inTx(ctx, exec, p)
	}
	return nil
}

// countOpenProposals counts pending+approving+failed proposals of a
// coordinator, on any executor that supports parameterized queries (a
// transaction-bound coordinatorExec during InsertProposal, or the store's
// reader pool via CountOpenProposals).
func countOpenProposals(ctx context.Context, exec queryRowExec, rebind func(string) string, coordinatorID string, phase2 bool) (int, error) {
	placeholders := make([]string, len(openProposalStatuses))
	args := make([]any, 0, len(openProposalStatuses)+1)
	args = append(args, coordinatorID)
	for i, status := range openProposalStatuses {
		placeholders[i] = "?"
		args = append(args, string(status))
	}
	query := rebind(`SELECT COUNT(*) FROM coordinator_proposals WHERE coordinator_id = ? AND status IN (` + strings.Join(placeholders, ",") + `)` + kindFilter(phase2))
	var count int
	if err := exec.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count open proposals: %w", err)
	}
	return count, nil
}

// queryRowExec is the minimal read surface countOpenProposals needs; both
// coordinatorExec and *sqlx.DB satisfy it.
type queryRowExec interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// CountOpenProposals returns the number of pending+approving+failed proposals
// for a coordinator (decision 9's open_proposals DTO field), read from the
// reader pool.
func (s *Store) CountOpenProposals(ctx context.Context, coordinatorID string, phase2 bool) (int, error) {
	return countOpenProposals(ctx, s.ro, s.ro.Rebind, coordinatorID, phase2)
}

// CountOpenProposalsTx is CountOpenProposals on a transaction-bound handle.
func (s *Store) CountOpenProposalsTx(ctx context.Context, exec coordinatorExec, coordinatorID string, phase2 bool) (int, error) {
	return countOpenProposals(ctx, exec, s.db.Rebind, coordinatorID, phase2)
}

// kindFilter hides every proposal kind other than create_task while phase 2
// is off.
func kindFilter(phase2 bool) string {
	if phase2 {
		return ""
	}
	return " AND kind = '" + ProposalKindCreateTask + "'"
}

// CountOpenProposalsByWorkspace is the batch counterpart to
// CountOpenProposals: it returns every coordinator's open (pending+approving+
// failed) proposal count in workspaceID with a single grouped query, so
// ListCoordinators pairs a workspace's coordinators with their counts
// without issuing one query per coordinator. A coordinator with no open
// proposals is absent from the returned map; callers read a missing key as
// its Go zero value, 0.
func (s *Store) CountOpenProposalsByWorkspace(ctx context.Context, workspaceID string, phase2 bool) (map[string]int, error) {
	placeholders := make([]string, len(openProposalStatuses))
	args := make([]any, 0, len(openProposalStatuses)+1)
	args = append(args, workspaceID)
	for i, status := range openProposalStatuses {
		placeholders[i] = "?"
		args = append(args, string(status))
	}
	query := s.ro.Rebind(`
		SELECT coordinator_id, COUNT(*) AS open_count FROM coordinator_proposals
		WHERE workspace_id = ? AND status IN (` + strings.Join(placeholders, ",") + `)` + kindFilter(phase2) + `
		GROUP BY coordinator_id`)

	var rows []struct {
		CoordinatorID string `db:"coordinator_id"`
		OpenCount     int    `db:"open_count"`
	}
	if err := s.ro.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("count open proposals by workspace: %w", err)
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.CoordinatorID] = row.OpenCount
	}
	return counts, nil
}

// GetProposal returns a proposal scoped to both workspaceID and
// coordinatorID; ErrNotFound if absent or scoped elsewhere.
func (s *Store) GetProposal(ctx context.Context, workspaceID, coordinatorID, id string, phase2 bool) (*Proposal, error) {
	var row proposalRow
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`
		SELECT `+proposalColumns+` FROM coordinator_proposals
		WHERE id = ? AND coordinator_id = ? AND workspace_id = ?`+kindFilter(phase2)),
		id, coordinatorID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get proposal: %w", err)
	}
	return row.toProposal()
}

// ListProposalsStatus selects which proposals ListProposals returns.
type ListProposalsStatus int

const (
	// ListProposalsPending returns every open proposal, oldest first,
	// unbounded (docs/specs/coordinator/system-design/proposals.md#routes).
	ListProposalsPending ListProposalsStatus = iota
	// ListProposalsAll returns every proposal, newest first, capped at 50.
	ListProposalsAll
)

// ListProposals returns a coordinator's proposals per status. Never nil.
func (s *Store) ListProposals(ctx context.Context, workspaceID, coordinatorID string, status ListProposalsStatus, phase2 bool) ([]*Proposal, error) {
	query := `SELECT ` + proposalColumns + ` FROM coordinator_proposals WHERE coordinator_id = ? AND workspace_id = ?` + kindFilter(phase2)
	args := []any{coordinatorID, workspaceID}
	switch status {
	case ListProposalsPending:
		placeholders := make([]string, len(openProposalStatuses))
		for i, proposalStatus := range openProposalStatuses {
			placeholders[i] = "?"
			args = append(args, string(proposalStatus))
		}
		query += ` AND status IN (` + strings.Join(placeholders, ",") + `) ORDER BY created_at ASC, id ASC`
	case ListProposalsAll:
		query += ` ORDER BY created_at DESC, id DESC LIMIT 50`
	}

	var rows []proposalRow
	if err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list proposals: %w", err)
	}
	result := make([]*Proposal, len(rows))
	for i := range rows {
		p, err := rows[i].toProposal()
		if err != nil {
			return nil, err
		}
		result[i] = p
	}
	return result, nil
}

// ListApprovingClaimedBefore returns every approving proposal, across every
// workspace, whose claimed_at is strictly before cutoff, ordered by
// claimed_at then id (docs/specs/coordinator/system-design/proposals.md#recovery).
// Unbounded: the startup pass is expected to run against a small number of
// stuck rows, and no new index backs this query. cutoff is normalized to UTC
// before binding: claimed_at is always stored in UTC (see ClaimProposal,
// ReclaimStale), and SQLite compares DATETIME columns lexicographically, so a
// cutoff carrying a different offset would not order correctly against it.
func (s *Store) ListApprovingClaimedBefore(ctx context.Context, cutoff time.Time, phase2 bool) ([]*Proposal, error) {
	var rows []proposalRow
	if err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`
		SELECT `+proposalColumns+` FROM coordinator_proposals
		WHERE status = ? AND claimed_at < ?`+kindFilter(phase2)+`
		ORDER BY claimed_at ASC, id ASC`),
		string(ProposalStatusApproving), cutoff.UTC()); err != nil {
		return nil, fmt.Errorf("list approving proposals claimed before cutoff: %w", err)
	}
	result := make([]*Proposal, len(rows))
	for i := range rows {
		p, err := rows[i].toProposal()
		if err != nil {
			return nil, err
		}
		result[i] = p
	}
	return result, nil
}

// ClaimProposal conditionally moves a pending or failed proposal to
// approving, storing finalSpec, decidedBy and a fresh claim token. Returns
// matched=false (never an error) when no row satisfied the condition. now is
// normalized to UTC before binding: claimed_at is compared lexicographically
// (ListApprovingClaimedBefore, ReclaimStale), which only orders correctly
// when every stored value carries the same offset.
func (s *Store) ClaimProposal(ctx context.Context, id, token string, finalSpec ProposalSpec, decidedBy string, now time.Time) (bool, error) {
	finalJSON, err := json.Marshal(finalSpec)
	if err != nil {
		return false, fmt.Errorf("marshal final spec: %w", err)
	}
	return s.ClaimProposalRaw(ctx, id, token, string(finalJSON), decidedBy, now)
}

// ClaimProposalRaw is ClaimProposal for a frozen spec already encoded as JSON,
// the form every non-create_task kind stores.
func (s *Store) ClaimProposalRaw(ctx context.Context, id, token, finalJSON, decidedBy string, now time.Time) (bool, error) {
	now = now.UTC()
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET status = ?, claimed_at = ?, claim_token = ?, final_spec_json = ?, decided_by = ?, error = NULL, updated_at = ?
		WHERE id = ? AND status IN (?, ?)`),
		string(ProposalStatusApproving), now, token, finalJSON, decidedBy, now,
		id, string(ProposalStatusPending), string(ProposalStatusFailed))
	if err != nil {
		return false, fmt.Errorf("claim proposal: %w", err)
	}
	return matchedRow(res)
}

// ReclaimStale re-issues the claim token and claimed_at of a proposal stuck
// in approving with claimed_at before staleBefore. It never rewrites
// final_spec_json or decided_by (docs/specs/coordinator/system-design/
// proposals.md#stale-re-claim). now and staleBefore are normalized to UTC
// before binding, for the same lexicographic-ordering reason as ClaimProposal
// and ListApprovingClaimedBefore.
func (s *Store) ReclaimStale(ctx context.Context, id, token string, now, staleBefore time.Time, phase2 bool) (bool, error) {
	now = now.UTC()
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET claimed_at = ?, claim_token = ?, updated_at = ?
		WHERE id = ? AND status = ? AND claimed_at < ?`+kindFilter(phase2)),
		now, token, now, id, string(ProposalStatusApproving), staleBefore.UTC())
	if err != nil {
		return false, fmt.Errorf("reclaim stale proposal: %w", err)
	}
	return matchedRow(res)
}

// CompleteProposal marks an approving proposal (fenced by its current claim
// token) approved, recording taskID and clearing the claim token. now is
// normalized to UTC before binding, matching ClaimProposal and ReclaimStale,
// so every stored timestamp in the table carries the same offset.
func (s *Store) CompleteProposal(ctx context.Context, id, token, taskID string, now time.Time) (bool, error) {
	return s.CompleteProposalTx(ctx, s.db, id, token, taskID, now)
}

// CompleteProposalTx is CompleteProposal on a transaction-bound handle.
func (s *Store) CompleteProposalTx(ctx context.Context, exec coordinatorExec, id, token, taskID string, now time.Time) (bool, error) {
	now = now.UTC()
	res, err := exec.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET status = ?, task_id = ?, claim_token = NULL, updated_at = ?
		WHERE id = ? AND status = ? AND claim_token = ?`),
		string(ProposalStatusApproved), taskID, now,
		id, string(ProposalStatusApproving), token)
	if err != nil {
		return false, fmt.Errorf("complete proposal: %w", err)
	}
	return matchedRow(res)
}

// FailProposal marks an approving proposal (fenced by its current claim
// token) failed, recording errMsg truncated to 1000 runes and clearing the
// claim token. now is normalized to UTC before binding, matching
// ClaimProposal and ReclaimStale.
func (s *Store) FailProposal(ctx context.Context, id, token, errMsg string, now time.Time) (bool, error) {
	return s.FailProposalTx(ctx, s.db, id, token, errMsg, now)
}

// FailProposalTx is FailProposal on a transaction-bound handle.
func (s *Store) FailProposalTx(ctx context.Context, exec coordinatorExec, id, token, errMsg string, now time.Time) (bool, error) {
	now = now.UTC()
	res, err := exec.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET status = ?, error = ?, claim_token = NULL, updated_at = ?
		WHERE id = ? AND status = ? AND claim_token = ?`),
		string(ProposalStatusFailed), truncateRunes(errMsg, 1000), now,
		id, string(ProposalStatusApproving), token)
	if err != nil {
		return false, fmt.Errorf("fail proposal: %w", err)
	}
	return matchedRow(res)
}

// RejectProposal conditionally moves a pending or failed proposal to
// rejected, trimming and truncating reason to 500 runes (stored NULL when
// empty after trimming). now is normalized to UTC before binding, matching
// ClaimProposal and ReclaimStale.
func (s *Store) RejectProposal(ctx context.Context, id, reason, decidedBy string, now time.Time) (bool, error) {
	return s.RejectProposalTx(ctx, s.db, id, reason, decidedBy, now)
}

// RejectProposalTx is RejectProposal on a transaction-bound handle.
func (s *Store) RejectProposalTx(ctx context.Context, exec coordinatorExec, id, reason, decidedBy string, now time.Time) (bool, error) {
	now = now.UTC()
	trimmed := strings.TrimSpace(reason)
	res, err := exec.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET status = ?, reject_reason = ?, decided_by = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?)`),
		string(ProposalStatusRejected), nonEmptyPtr(truncateRunes(trimmed, 500)), decidedBy, now,
		id, string(ProposalStatusPending), string(ProposalStatusFailed))
	if err != nil {
		return false, fmt.Errorf("reject proposal: %w", err)
	}
	return matchedRow(res)
}

func matchedRow(res sql.Result) (bool, error) {
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return rows > 0, nil
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func nonEmptyPtr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
