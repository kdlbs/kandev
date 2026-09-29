package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"expvar"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidActivity is returned when an activity row or refusal argument
// fails validation. Nothing is written.
var ErrInvalidActivity = errors.New("coordinator: invalid activity")

// ActivityOutcome is the result recorded on an activity row.
type ActivityOutcome string

// Activity outcomes.
const (
	ActivityProposed ActivityOutcome = "proposed"
	ActivityApproved ActivityOutcome = "approved"
	ActivityRejected ActivityOutcome = "rejected"
	ActivityFailed   ActivityOutcome = "failed"
	ActivityRefused  ActivityOutcome = "refused"
	ActivityUndone   ActivityOutcome = "undone"
)

// ActivityAuthorization records why a row was or was not allowed to proceed.
type ActivityAuthorization string

// Activity authorizations.
const (
	AuthRequiresApproval ActivityAuthorization = "requires_approval"
	AuthDenied           ActivityAuthorization = "denied"
)

const (
	activityDetailMaxRunes = 1000
	refusalCoalesceWindow  = 60 * time.Second
	activityColumns        = `id, coordinator_id, workspace_id, action_class, outcome, "authorization", target_task_id, proposal_id, actor_user_id, reason_code, detail, edited, refusal_count, undone_at, undone_by, undo_of_id, created_at, updated_at`
)

var activityRowsTotal = expvar.NewMap("coordinator_activity_rows_total")

// activityRowsCounter reads the per-outcome activity counter.
func activityRowsCounter(outcome ActivityOutcome) int64 {
	if v, ok := activityRowsTotal.Get(string(outcome)).(*expvar.Int); ok {
		return v.Value()
	}
	return 0
}

// ActivityRow is one row of the coordinator activity log.
type ActivityRow struct {
	ID            string                `db:"id" json:"id"`
	CoordinatorID string                `db:"coordinator_id" json:"coordinator_id"`
	WorkspaceID   string                `db:"workspace_id" json:"workspace_id"`
	ActionClass   Action                `db:"action_class" json:"action_class"`
	Outcome       ActivityOutcome       `db:"outcome" json:"outcome"`
	Authorization ActivityAuthorization `db:"authorization" json:"authorization"`
	TargetTaskID  *string               `db:"target_task_id" json:"target_task_id"`
	ProposalID    *string               `db:"proposal_id" json:"proposal_id"`
	ActorUserID   *string               `db:"actor_user_id" json:"actor_user_id"`
	ReasonCode    *string               `db:"reason_code" json:"reason_code"`
	Detail        string                `db:"detail" json:"detail"`
	Edited        bool                  `db:"edited" json:"edited"`
	RefusalCount  int                   `db:"refusal_count" json:"refusal_count"`
	UndoneAt      *time.Time            `db:"undone_at" json:"undone_at"`
	UndoneBy      *string               `db:"undone_by" json:"undone_by"`
	UndoOfID      *string               `db:"undo_of_id" json:"undo_of_id"`
	CreatedAt     time.Time             `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time             `db:"updated_at" json:"updated_at"`
}

// RefusalRecorder records a refused coordinator action, coalescing repeats.
type RefusalRecorder interface {
	RecordRefusal(ctx context.Context, coordinatorID, workspaceID string, class Action, reasonCode string) error
}

func validActivityClass(a Action) bool {
	return a == ActionUnknown || isPolicyAction(a)
}

func (r ActivityRow) validate() error {
	switch {
	case r.CoordinatorID == "" || r.WorkspaceID == "":
		return fmt.Errorf("%w: coordinator and workspace are required", ErrInvalidActivity)
	case !validActivityClass(r.ActionClass):
		return fmt.Errorf("%w: action class %q", ErrInvalidActivity, r.ActionClass)
	}
	switch r.Outcome {
	case ActivityProposed, ActivityApproved, ActivityRejected, ActivityFailed, ActivityRefused, ActivityUndone:
	default:
		return fmt.Errorf("%w: outcome %q", ErrInvalidActivity, r.Outcome)
	}
	switch r.Authorization {
	case AuthRequiresApproval, AuthDenied:
	default:
		return fmt.Errorf("%w: authorization %q", ErrInvalidActivity, r.Authorization)
	}
	return nil
}

// InsertActivity validates and appends one activity row through exec. The
// caller holds the coordinator lock when the row must not outlive its
// coordinator.
func (s *Store) InsertActivity(ctx context.Context, exec coordinatorExec, row ActivityRow) error {
	if err := row.validate(); err != nil {
		return err
	}
	now := s.now().UTC()
	if row.ID == "" {
		row.ID = uuid.NewString()
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = now
	}
	if row.RefusalCount < 1 {
		row.RefusalCount = 1
	}
	// The row is bound to the coordinator's real workspace, whatever the
	// caller passed.
	var workspaceID string
	if err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT workspace_id FROM coordinators WHERE id = ?`), row.CoordinatorID).Scan(&workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("resolve activity workspace: %w", err)
	}
	row.WorkspaceID = workspaceID
	row.Detail = truncateRunes(row.Detail, activityDetailMaxRunes)
	_, err := exec.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_activity (`+activityColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		row.ID, row.CoordinatorID, row.WorkspaceID, string(row.ActionClass), string(row.Outcome),
		string(row.Authorization), row.TargetTaskID, row.ProposalID, row.ActorUserID, row.ReasonCode,
		row.Detail, row.Edited, row.RefusalCount, row.UndoneAt, row.UndoneBy, row.UndoOfID,
		row.CreatedAt, row.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert coordinator activity: %w", err)
	}
	activityRowsTotal.Add(string(row.Outcome), 1)
	return nil
}

// MarkUndone stamps an activity row as undone. It reports whether a row was
// changed; an already-undone or absent row changes nothing.
func (s *Store) MarkUndone(ctx context.Context, exec coordinatorExec, rowID, undoneBy string, at time.Time) (bool, error) {
	at = at.UTC()
	res, err := exec.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_activity SET undone_at = ?, undone_by = ?, updated_at = ? WHERE id = ? AND undone_at IS NULL`),
		at, undoneBy, at, rowID)
	if err != nil {
		return false, fmt.Errorf("mark activity undone: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("mark activity undone: %w", err)
	}
	return n > 0, nil
}

// Record appends an activity row through exec, which must be the handle of
// withCoordinatorLock. It is a no-op while phase 2 is off.
func (s *Service) Record(ctx context.Context, exec coordinatorExec, row ActivityRow) error {
	if !s.phase2 {
		return nil
	}
	return s.store.InsertActivity(ctx, exec, row)
}

// RecordRefusal records a refused action. A refusal with the same class and
// reason inside the coalescing window increments the existing row instead of
// adding one. A missing coordinator writes nothing.
func (s *Service) RecordRefusal(ctx context.Context, coordinatorID, workspaceID string, class Action, reasonCode string) error {
	if reasonCode == "" || !validActivityClass(class) {
		return fmt.Errorf("%w: refusal needs a reason and a known class", ErrInvalidActivity)
	}
	if !s.phase2 {
		return nil
	}
	err := s.store.withCoordinatorLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		return s.store.recordRefusalTx(ctx, tx, coordinatorID, workspaceID, class, reasonCode)
	})
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (s *Store) recordRefusalTx(ctx context.Context, tx coordinatorExec, coordinatorID, workspaceID string, class Action, reasonCode string) error {
	now := s.now().UTC()
	var id string
	err := tx.QueryRowContext(ctx, s.db.Rebind(`SELECT id FROM coordinator_activity
		WHERE coordinator_id = ? AND action_class = ? AND reason_code = ? AND outcome = ? AND created_at >= ?
		ORDER BY created_at DESC, id DESC LIMIT 1`),
		coordinatorID, string(class), reasonCode, string(ActivityRefused), now.Add(-refusalCoalesceWindow)).Scan(&id)
	switch {
	case err == nil:
		_, err = tx.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_activity SET refusal_count = refusal_count + 1, updated_at = ? WHERE id = ?`), now, id)
		if err != nil {
			return fmt.Errorf("coalesce refusal: %w", err)
		}
		return nil
	case errors.Is(err, sql.ErrNoRows):
		reason := reasonCode
		return s.InsertActivity(ctx, tx, ActivityRow{
			CoordinatorID: coordinatorID,
			WorkspaceID:   workspaceID,
			ActionClass:   class,
			Outcome:       ActivityRefused,
			Authorization: AuthDenied,
			ReasonCode:    &reason,
		})
	default:
		return fmt.Errorf("find refusal row: %w", err)
	}
}
