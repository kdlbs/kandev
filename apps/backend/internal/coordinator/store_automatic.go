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
	err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT
		(SELECT COUNT(*) FROM coordinator_proposals
			WHERE coordinator_id = ? AND decided_automatically = 1 AND automatic_at >= ?)
		+ (SELECT COUNT(*) FROM coordinator_merge_dispatches d
			JOIN coordinator_merge_proposals m ON m.proposal_id = d.proposal_id
			WHERE m.coordinator_id = ? AND d.automatic = 1 AND d.dispatched_at >= ?)`),
		coordinatorID, since.UTC(), coordinatorID, since.UTC()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count automatic approvals: %w", err)
	}
	return n, nil
}

// decidedOutcomeFilter selects the rows a manager decided: outcome approved,
// rejected or returned, and never an automatic authorization.
const decidedOutcomeFilter = `a.outcome IN ('approved', 'rejected', 'returned') AND a."authorization" <> 'automatic'`

// decidedFrom is the FROM and WHERE of the decided rows of class, with the
// coordinator id as its first argument. start_agent rows are those of any kind
// class whose proposal starts an agent; the others are bound to action_class.
func decidedFrom(class Action) string {
	if class == ActionStartAgent {
		return `coordinator_activity a JOIN coordinator_proposals p ON p.id = a.proposal_id AND p.starts_agent = TRUE
			WHERE a.coordinator_id = ? AND ` + decidedOutcomeFilter
	}
	return `coordinator_activity a WHERE a.coordinator_id = ? AND a.action_class = '` + string(class) + `' AND ` + decidedOutcomeFilter
}

// EarliestDecisionAt is the creation time of the coordinator's oldest decided
// row of class, or nil when there is none.
func (s *Store) EarliestDecisionAt(ctx context.Context, coordinatorID string, class Action) (*time.Time, error) {
	var at time.Time
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT a.created_at FROM `+decidedFrom(class)+` ORDER BY a.created_at ASC, a.id ASC LIMIT 1`), coordinatorID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read earliest decision: %w", err)
	}
	at = at.UTC()
	return &at, nil
}

// DecisionRow is one decided row.
type DecisionRow struct {
	ProposalID string
	Outcome    string
	Edited     bool
	DecidedAt  time.Time
}

// DecidedRows returns the decided rows of class created in [since, before),
// ordered by created_at then id.
func (s *Store) DecidedRows(ctx context.Context, coordinatorID string, class Action, since, before time.Time) ([]DecisionRow, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT a.proposal_id, a.outcome, a.edited, a.created_at FROM `+decidedFrom(class)+
		` AND a.created_at >= ? AND a.created_at < ? ORDER BY a.created_at, a.id`),
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

// CountDecidedRowsTx counts the decided rows of class created in
// [since, before) through exec, for use inside the coordinator lock.
func (s *Store) CountDecidedRowsTx(ctx context.Context, exec coordinatorExec, coordinatorID string, class Action, since, before time.Time) (int, error) {
	var n int
	err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT COUNT(*) FROM `+decidedFrom(class)+` AND a.created_at >= ? AND a.created_at < ?`),
		coordinatorID, since.UTC(), before.UTC()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count decided rows: %w", err)
	}
	return n, nil
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
	return s.InsertClassReviewTx(ctx, s.db, r)
}

// InsertClassReviewTx appends one class review through exec.
func (s *Store) InsertClassReviewTx(ctx context.Context, exec coordinatorExec, r ClassReview) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	_, err := exec.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_class_reviews
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

// activityClassFrom is the FROM and WHERE of the coordinator's activity rows
// of class with the given outcome and authorization: start_agent rows are those
// of any kind class whose proposal starts an agent. The coordinator id is the
// first argument.
func activityClassFrom(class Action) string {
	if class == ActionStartAgent {
		return `coordinator_activity a JOIN coordinator_proposals p ON p.id = a.proposal_id AND p.starts_agent = TRUE
			WHERE a.coordinator_id = ?`
	}
	return `coordinator_activity a WHERE a.coordinator_id = ? AND a.action_class = '` + string(class) + `'`
}

// FailedOrLoweredCount counts, in [since, before), the failed automatic
// activity rows of class plus the system lowerings of class that were not a
// manager override or a lost raiser. System lowerings always carry a reason;
// a manager's own change does not, with or without an identity.
func (s *Store) FailedOrLoweredCount(ctx context.Context, coordinatorID string, class Action, since, before time.Time) (int, error) {
	var failed int
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT COUNT(*) FROM `+activityClassFrom(class)+
		` AND a.outcome = 'failed' AND a."authorization" = 'automatic' AND a.created_at >= ? AND a.created_at < ?`),
		coordinatorID, since.UTC(), before.UTC()).Scan(&failed)
	if err != nil {
		return 0, fmt.Errorf("count failed automatic rows: %w", err)
	}
	var lowered int
	err = s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT COUNT(*) FROM coordinator_class_changes
		WHERE coordinator_id = ? AND class = ? AND to_value = 'requires_approval' AND changed_by IS NULL
		AND reason IS NOT NULL AND reason NOT IN (?, ?) AND changed_at >= ? AND changed_at < ?`),
		coordinatorID, string(class), overrideLoweredReason, raiserLoweredReason, since.UTC(), before.UTC()).Scan(&lowered)
	if err != nil {
		return 0, fmt.Errorf("count system lowerings: %w", err)
	}
	return failed + lowered, nil
}

// InsertEligibilityChangeTx appends one row to coordinator_eligibility_changes
// through exec. changedBy is null when empty.
func (s *Store) InsertEligibilityChangeTx(ctx context.Context, exec coordinatorExec, coordinatorID, changedBy string, from, to EligibilitySettings, revision int) error {
	fromJSON, err := json.Marshal(from)
	if err != nil {
		return fmt.Errorf("encode eligibility: %w", err)
	}
	toJSON, err := json.Marshal(to)
	if err != nil {
		return fmt.Errorf("encode eligibility: %w", err)
	}
	_, err = exec.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_eligibility_changes
		(id, coordinator_id, changed_by, from_json, to_json, policy_revision, changed_at) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		uuid.NewString(), coordinatorID, nonEmptyPtr(changedBy), string(fromJSON), string(toJSON), revision, s.now().UTC())
	if err != nil {
		return fmt.Errorf("insert eligibility change: %w", err)
	}
	return nil
}
