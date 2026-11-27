package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ClassChange is one row of coordinator_class_changes: a change of one
// class's setting. ChangedBy is empty for a system change (LowerClass).
type ClassChange struct {
	ID            string
	CoordinatorID string
	Class         Action
	FromValue     Setting
	ToValue       Setting
	ChangedBy     string
	Reason        string
	ChangedAt     time.Time
}

// ClassReview is one row of coordinator_class_reviews.
type ClassReview struct {
	ID            string
	CoordinatorID string
	Class         Action
	ReviewedBy    string
	ReviewedAt    time.Time
	WindowStart   time.Time
	WindowEnd     time.Time
	RowCount      int
}

// InsertClassChangeTx appends one class change through exec.
func (s *Store) InsertClassChangeTx(ctx context.Context, exec coordinatorExec, c ClassChange) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if c.ChangedAt.IsZero() {
		c.ChangedAt = s.now()
	}
	_, err := exec.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_class_changes
		(id, coordinator_id, class, from_value, to_value, changed_by, reason, changed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		c.ID, c.CoordinatorID, string(c.Class), string(c.FromValue), string(c.ToValue),
		nonEmptyPtr(c.ChangedBy), nonEmptyPtr(c.Reason), c.ChangedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert class change: %w", err)
	}
	return nil
}

// NewestClassChangeTx returns the newest change of class whose to_value is
// toValue, or nil when there is none.
func (s *Store) NewestClassChangeTx(ctx context.Context, exec coordinatorExec, coordinatorID string, class Action, toValue Setting) (*ClassChange, error) {
	var (
		c         ClassChange
		changedBy sql.NullString
		reason    sql.NullString
		classStr  string
		from, to  string
	)
	err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT id, coordinator_id, class, from_value, to_value, changed_by, reason, changed_at
		FROM coordinator_class_changes WHERE coordinator_id = ? AND class = ? AND to_value = ?
		ORDER BY changed_at DESC, id DESC LIMIT 1`), coordinatorID, string(class), string(toValue)).
		Scan(&c.ID, &c.CoordinatorID, &classStr, &from, &to, &changedBy, &reason, &c.ChangedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read class change: %w", err)
	}
	c.Class, c.FromValue, c.ToValue = Action(classStr), Setting(from), Setting(to)
	c.ChangedBy, c.Reason = changedBy.String, reason.String
	return &c, nil
}

// CountAutomaticDecidedTx counts the coordinator's proposals the automatic
// path claimed at or after since, whatever their status now.
func (s *Store) CountAutomaticDecidedTx(ctx context.Context, exec coordinatorExec, coordinatorID string, since time.Time) (int, error) {
	var n int
	err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT COUNT(*) FROM coordinator_proposals
		WHERE coordinator_id = ? AND decided_automatically = 1 AND automatic_at >= ?`), coordinatorID, since.UTC()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count automatic approvals: %w", err)
	}
	return n, nil
}

// decidedRowFilter selects the create_task rows a manager decided: outcome
// approved, rejected or returned, and never an automatic authorization.
const decidedRowFilter = `coordinator_id = ? AND action_class = 'create_task' AND outcome IN ('approved', 'rejected', 'returned') AND "authorization" <> 'automatic'`

// EarliestDecisionAt is the creation time of the coordinator's oldest decided
// create_task row, or nil when there is none.
func (s *Store) EarliestDecisionAt(ctx context.Context, coordinatorID string) (*time.Time, error) {
	var at time.Time
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT created_at FROM coordinator_activity WHERE `+decidedRowFilter+` ORDER BY created_at ASC LIMIT 1`), coordinatorID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read earliest decision: %w", err)
	}
	at = at.UTC()
	return &at, nil
}

// DecisionRow is one decided create_task row.
type DecisionRow struct {
	ProposalID string
	Outcome    string
	Edited     bool
	DecidedAt  time.Time
}

// DecidedRows returns the decided create_task rows created in [since, before).
func (s *Store) DecidedRows(ctx context.Context, coordinatorID string, since, before time.Time) ([]DecisionRow, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT proposal_id, outcome, edited, created_at FROM coordinator_activity
		WHERE `+decidedRowFilter+` AND created_at >= ? AND created_at < ? ORDER BY created_at, id`),
		coordinatorID, since.UTC(), before.UTC())
	if err != nil {
		return nil, fmt.Errorf("read decided rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DecisionRow
	for rows.Next() {
		var (
			r        DecisionRow
			proposal sql.NullString
		)
		if err := rows.Scan(&proposal, &r.Outcome, &r.Edited, &r.DecidedAt); err != nil {
			return nil, fmt.Errorf("scan decided row: %w", err)
		}
		r.ProposalID = proposal.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// UndoneCreateTaskIDs returns the target task of every approved create_task
// row of the coordinator, whatever its authorization, undone in [since, before).
func (s *Store) UndoneCreateTaskIDs(ctx context.Context, coordinatorID string, since, before time.Time) ([]string, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT target_task_id FROM coordinator_activity
		WHERE coordinator_id = ? AND action_class = 'create_task' AND outcome = 'approved' AND target_task_id IS NOT NULL
		AND undone_at IS NOT NULL AND undone_at >= ? AND undone_at < ? ORDER BY undone_at, id`),
		coordinatorID, since.UTC(), before.UTC())
	if err != nil {
		return nil, fmt.Errorf("read undone tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan undone task: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AutomaticClaimTaskIDs returns the task ids of the coordinator's proposals
// whose current claim is the automatic path's.
func (s *Store) AutomaticClaimTaskIDs(ctx context.Context, coordinatorID string) (map[string]struct{}, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT task_id FROM coordinator_proposals
		WHERE coordinator_id = ? AND claimed_automatically = 1 AND task_id IS NOT NULL`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("read automatic tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan automatic task: %w", err)
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// InsertClassReview appends one class review.
func (s *Store) InsertClassReview(ctx context.Context, r ClassReview) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_class_reviews
		(id, coordinator_id, class, reviewed_by, reviewed_at, window_start, window_end, row_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		r.ID, r.CoordinatorID, string(r.Class), r.ReviewedBy, r.ReviewedAt.UTC(), r.WindowStart.UTC(), r.WindowEnd.UTC(), r.RowCount)
	if err != nil {
		return fmt.Errorf("insert class review: %w", err)
	}
	return nil
}

// NewestClassReview returns the coordinator's newest review of class, or nil.
func (s *Store) NewestClassReview(ctx context.Context, coordinatorID string, class Action) (*ClassReview, error) {
	var r ClassReview
	var classStr string
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT id, coordinator_id, class, reviewed_by, reviewed_at, window_start, window_end, row_count
		FROM coordinator_class_reviews WHERE coordinator_id = ? AND class = ? ORDER BY reviewed_at DESC, id DESC LIMIT 1`),
		coordinatorID, string(class)).
		Scan(&r.ID, &r.CoordinatorID, &classStr, &r.ReviewedBy, &r.ReviewedAt, &r.WindowStart, &r.WindowEnd, &r.RowCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read class review: %w", err)
	}
	r.Class = Action(classStr)
	return &r, nil
}
