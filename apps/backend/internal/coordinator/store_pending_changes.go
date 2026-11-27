package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Pending change statuses and the one field a change targets.
const (
	ChangeStatusPending   = "pending"
	ChangeStatusApplied   = "applied"
	ChangeStatusDiscarded = "discarded"
	changeFieldContext    = "context"
)

// ChangeReasonContextChanged is the 409 reason of an Apply whose base no
// longer matches the stored context.
const ChangeReasonContextChanged = "context_changed"

// ErrChangeNotFound answers a change that is absent, another coordinator's, or
// whose proposal is not approved; the three are indistinguishable.
var ErrChangeNotFound = errors.New("coordinator: pending change not found")

// ChangeConflictError is a 409: the change was already settled (Reason empty)
// or its base no longer matches the coordinator's context.
type ChangeConflictError struct {
	Change *PendingChange
	Reason string
}

func (e *ChangeConflictError) Error() string {
	if e.Reason != "" {
		return "coordinator: pending change conflict: " + e.Reason
	}
	return "coordinator: pending change already settled"
}

// PendingChange is a stored improvement waiting for a manager to apply or
// discard it.
type PendingChange struct {
	ID            string    `json:"id"`
	CoordinatorID string    `json:"coordinator_id"`
	ProposalID    string    `json:"proposal_id"`
	ProposalTitle string    `json:"proposal_title"`
	Field         string    `json:"field"`
	BaseValue     string    `json:"base_value"`
	NewValue      string    `json:"new_value"`
	Status        string    `json:"status"`
	DecidedBy     *string   `json:"decided_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

const pendingChangeSelect = `SELECT pc.id, pc.coordinator_id, pc.proposal_id, pc.field, pc.base_value, pc.new_value,
	pc.status, pc.decided_by, pc.created_at, pc.updated_at, p.spec_json, p.status
	FROM coordinator_pending_changes pc
	JOIN coordinator_proposals p ON p.id = pc.proposal_id`

type pendingChangeScanner interface {
	Scan(dest ...any) error
}

// scanPendingChange returns the change and the status of its proposal.
func scanPendingChange(row pendingChangeScanner) (*PendingChange, string, error) {
	var (
		c          PendingChange
		decidedBy  sql.NullString
		specJSON   string
		proposalSt string
	)
	if err := row.Scan(&c.ID, &c.CoordinatorID, &c.ProposalID, &c.Field, &c.BaseValue, &c.NewValue,
		&c.Status, &decidedBy, &c.CreatedAt, &c.UpdatedAt, &specJSON, &proposalSt); err != nil {
		return nil, "", err
	}
	if decidedBy.Valid {
		c.DecidedBy = &decidedBy.String
	}
	var spec struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal([]byte(specJSON), &spec)
	c.ProposalTitle = spec.Title
	c.CreatedAt, c.UpdatedAt = c.CreatedAt.UTC(), c.UpdatedAt.UTC()
	return &c, proposalSt, nil
}

// InsertPendingChange stores the change of an approved improvement; a change
// already stored for the proposal is left as it is.
func (s *Store) InsertPendingChange(ctx context.Context, coordinatorID, proposalID, base, next string) error {
	now := s.now()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_pending_changes
		(id, coordinator_id, proposal_id, field, base_value, new_value, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (proposal_id) DO NOTHING`),
		uuid.New().String(), coordinatorID, proposalID, changeFieldContext, base, next, ChangeStatusPending, now, now)
	if err != nil {
		return fmt.Errorf("insert pending change: %w", err)
	}
	return nil
}

// ListPendingChanges returns the coordinator's pending changes whose proposal
// is approved, oldest first, from one query.
func (s *Store) ListPendingChanges(ctx context.Context, coordinatorID string) ([]*PendingChange, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(pendingChangeSelect+`
		WHERE pc.coordinator_id = ? AND pc.status = ? AND p.status = ?
		ORDER BY pc.created_at ASC, pc.id ASC`), coordinatorID, ChangeStatusPending, string(ProposalStatusApproved))
	if err != nil {
		return nil, fmt.Errorf("list pending changes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []*PendingChange{}
	for rows.Next() {
		c, _, err := scanPendingChange(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pending change: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// changeOn reads one change of the coordinator and its proposal's status on
// exec. A miss returns nil.
func (s *Store) changeOn(ctx context.Context, exec coordinatorExec, coordinatorID, changeID string) (*PendingChange, string, error) {
	c, st, err := scanPendingChange(exec.QueryRowContext(ctx, s.db.Rebind(pendingChangeSelect+`
		WHERE pc.id = ? AND pc.coordinator_id = ?`), changeID, coordinatorID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("read pending change: %w", err)
	}
	return c, st, nil
}

// settlePendingChangeTx moves a pending change of an approved proposal to
// status; false when the change was not pending, not the coordinator's, or its
// proposal was not approved.
func (s *Store) settlePendingChangeTx(ctx context.Context, exec coordinatorExec, coordinatorID, changeID, status, decidedBy string) (bool, error) {
	res, err := exec.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_pending_changes
		SET status = ?, decided_by = ?, updated_at = ?
		WHERE id = ? AND coordinator_id = ? AND status = ?
		AND EXISTS (SELECT 1 FROM coordinator_proposals p
			WHERE p.id = coordinator_pending_changes.proposal_id AND p.status = ?)`),
		status, nullableString(optString(decidedBy)), s.now(), changeID, coordinatorID, ChangeStatusPending, string(ProposalStatusApproved))
	if err != nil {
		return false, fmt.Errorf("settle pending change: %w", err)
	}
	return matchedRow(res)
}

// DiscardPendingChange settles a pending change discarded and returns it. A
// missing change (or one whose proposal is not approved) is ErrChangeNotFound;
// a change no longer pending is a *ChangeConflictError carrying it.
func (s *Store) DiscardPendingChange(ctx context.Context, coordinatorID, changeID, decidedBy string) (*PendingChange, error) {
	var out *PendingChange
	err := s.withCoordinatorLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		matched, err := s.settlePendingChangeTx(ctx, tx, coordinatorID, changeID, ChangeStatusDiscarded, decidedBy)
		if err != nil {
			return err
		}
		c, proposalStatus, err := s.changeOn(ctx, tx, coordinatorID, changeID)
		if err != nil {
			return err
		}
		if c == nil || proposalStatus != string(ProposalStatusApproved) {
			return ErrChangeNotFound
		}
		if !matched {
			return &ChangeConflictError{Change: c}
		}
		out = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
