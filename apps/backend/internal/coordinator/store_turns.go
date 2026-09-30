package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Turn outcomes written by delivery, recovery and turn end.
const (
	outcomeCompleted   = "completed"
	outcomeFailed      = "failed"
	outcomeCancelled   = "cancelled"
	outcomeStopped     = "stopped_at_ceiling"
	outcomeSendFailed  = "send_failed"
	outcomeInterrupted = "interrupted"

	wakeStatusDelivered  = "delivered"
	wakeStatusSuperseded = "superseded"
)

// deliveryWakeLimit is the most wakes one unattended turn carries.
const deliveryWakeLimit = 20

var errTurnNotStarted = errors.New("coordinator: unattended turn not started")

type pendingWake struct {
	ID         string
	TaskID     string
	Kind       WakeKind
	EpisodeKey string
}

// listPendingWakes returns every pending wake, oldest first.
func (s *Store) listPendingWakes(ctx context.Context, coordinatorID string) ([]pendingWake, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(`
		SELECT id, task_id, kind, episode_key FROM coordinator_wakes
		WHERE coordinator_id = ? AND status = ? ORDER BY created_at, id`), coordinatorID, wakeStatusPending)
	if err != nil {
		return nil, fmt.Errorf("list pending wakes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []pendingWake
	for rows.Next() {
		var w pendingWake
		var kind string
		if err := rows.Scan(&w.ID, &w.TaskID, &kind, &w.EpisodeKey); err != nil {
			return nil, fmt.Errorf("scan pending wake: %w", err)
		}
		w.Kind = WakeKind(kind)
		out = append(out, w)
	}
	return out, rows.Err()
}

// supersedeWake supersedes one still-pending wake and reports whether it
// changed the row.
func (s *Store) supersedeWake(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_wakes SET status = ?, updated_at = ? WHERE id = ? AND status = ?`),
		wakeStatusSuperseded, s.now().UTC(), id, wakeStatusPending)
	return rowChanged(res, err, "supersede wake")
}

// deliveredWake is one wake of a started turn.
type deliveredWake struct {
	ID     string
	TaskID string
	Kind   WakeKind
}

// turnStart names the turn row to insert and the wakes it takes.
type turnStart struct {
	CoordinatorID string
	ConvTaskID    string
	SessionID     string
	WakeIDs       []string
}

// startedTurn is a committed turn start.
type startedTurn struct {
	ID          string
	WorkspaceID string
	Wakes       []deliveredWake
}

// startUnattendedTurn runs delivery step 3 in one transaction under the wake
// lock. It returns (nil, nil) when the start rolled back without an error:
// autonomy is off, the conversation changed, another delivery holds the open
// turn, or no wake was still pending.
func (s *Store) startUnattendedTurn(ctx context.Context, in turnStart) (*startedTurn, error) {
	var out *startedTurn
	err := s.WithWakeLock(ctx, in.CoordinatorID, func(tx coordinatorExec) error {
		var err error
		out, err = s.startTurnTx(ctx, tx, in)
		return err
	})
	if errors.Is(err, errTurnNotStarted) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) startTurnTx(ctx context.Context, tx coordinatorExec, in turnStart) (*startedTurn, error) {
	workspaceID, ceiling, err := s.readTurnStartGuard(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	turnID := uuid.NewString()
	res, err := tx.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO coordinator_unattended_turns
			(id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`),
		turnID, in.CoordinatorID, in.ConvTaskID, in.SessionID, len(in.WakeIDs), ceiling, s.now().UTC())
	if err != nil {
		return nil, fmt.Errorf("insert unattended turn: %w", err)
	}
	if inserted, err := matchedRow(res); err != nil || !inserted {
		return nil, notStarted(err)
	}
	wakes, err := s.claimWakes(ctx, tx, in, turnID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_unattended_turns SET wake_count = ? WHERE id = ?`), len(wakes), turnID); err != nil {
		return nil, fmt.Errorf("set turn wake count: %w", err)
	}
	return &startedTurn{ID: turnID, WorkspaceID: workspaceID, Wakes: wakes}, nil
}

// readTurnStartGuard re-reads the coordinator under the wake lock: autonomy
// must be on, the ceiling set and the conversation unchanged.
func (s *Store) readTurnStartGuard(ctx context.Context, tx coordinatorExec, in turnStart) (workspaceID string, ceiling int64, err error) {
	var autonomy int
	var convTask sql.NullString
	var ceil sql.NullInt64
	err = tx.QueryRowContext(ctx, s.db.Rebind(`
		SELECT workspace_id, autonomy_enabled, conversation_task_id, cost_ceiling_subcents FROM coordinators WHERE id = ?`),
		in.CoordinatorID).Scan(&workspaceID, &autonomy, &convTask, &ceil)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, errTurnNotStarted
	}
	if err != nil {
		return "", 0, fmt.Errorf("re-read coordinator for turn start: %w", err)
	}
	if autonomy != 1 || !ceil.Valid || !convTask.Valid || convTask.String != in.ConvTaskID {
		return "", 0, errTurnNotStarted
	}
	return workspaceID, ceil.Int64, nil
}

// claimWakes marks the chosen pending wakes delivered to the turn and reads
// back exactly the rows it now holds. No row claimed or read back rolls back.
func (s *Store) claimWakes(ctx context.Context, tx coordinatorExec, in turnStart, turnID string) ([]deliveredWake, error) {
	marks := strings.TrimSuffix(strings.Repeat("?,", len(in.WakeIDs)), ",")
	args := []any{wakeStatusDelivered, turnID, in.CoordinatorID, wakeStatusPending}
	for _, id := range in.WakeIDs {
		args = append(args, id)
	}
	res, err := tx.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_wakes SET status = ?, turn_id = ?
		WHERE coordinator_id = ? AND status = ? AND id IN (`+marks+`)`), args...)
	if err != nil {
		return nil, fmt.Errorf("deliver wakes: %w", err)
	}
	if changed, err := matchedRow(res); err != nil || !changed {
		return nil, notStarted(err)
	}
	wakes, err := readTurnWakes(ctx, tx, s.db.Rebind, turnID)
	if err != nil || len(wakes) == 0 {
		return nil, notStarted(err)
	}
	return wakes, nil
}

func notStarted(err error) error {
	if err != nil {
		return err
	}
	return errTurnNotStarted
}

func readTurnWakes(ctx context.Context, q coordinatorExec, rebind func(string) string, turnID string) ([]deliveredWake, error) {
	rows, err := q.QueryContext(ctx, rebind(`
		SELECT id, task_id, kind FROM coordinator_wakes WHERE turn_id = ? ORDER BY created_at, id`), turnID)
	if err != nil {
		return nil, fmt.Errorf("read turn wakes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []deliveredWake
	for rows.Next() {
		var w deliveredWake
		var kind string
		if err := rows.Scan(&w.ID, &w.TaskID, &kind); err != nil {
			return nil, fmt.Errorf("scan turn wake: %w", err)
		}
		w.Kind = WakeKind(kind)
		out = append(out, w)
	}
	return out, rows.Err()
}

// bindReservedTurn records the reserved session turn id on the open, unreserved row.
func (s *Store) bindReservedTurn(ctx context.Context, id, reservedTurnID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_unattended_turns SET reserved_turn_id = ?
		WHERE id = ? AND outcome IS NULL AND reserved_turn_id IS NULL`), reservedTurnID, id)
	return rowChanged(res, err, "bind reserved turn")
}

// bindAcceptedTurn records the accepted session turn id on the open, unbound row.
func (s *Store) bindAcceptedTurn(ctx context.Context, id, sessionTurnID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_unattended_turns SET session_turn_id = ?
		WHERE id = ? AND outcome IS NULL AND session_turn_id IS NULL`), sessionTurnID, id)
	return rowChanged(res, err, "bind accepted turn")
}

// setTurnMessage records the stored message id on the open row that has none.
func (s *Store) setTurnMessage(ctx context.Context, id, messageID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_unattended_turns SET message_id = ?
		WHERE id = ? AND outcome IS NULL AND message_id IS NULL`), messageID, id)
	return rowChanged(res, err, "set turn message")
}

// unattendedTurn is a full turn row.
type unattendedTurn struct {
	ID              string
	CoordinatorID   string
	ConvTaskID      string
	SessionID       string
	MessageID       string
	SessionTurnID   string
	ReservedTurnID  string
	StopRequestedAt *time.Time
	Outcome         string
	StartedAt       time.Time
	FinishedAt      *time.Time
}

const unattendedTurnColumns = `id, coordinator_id, conversation_task_id, session_id, message_id, session_turn_id,
	reserved_turn_id, stop_requested_at, outcome, started_at, finished_at`

func scanUnattendedTurn(sc interface{ Scan(...any) error }) (*unattendedTurn, error) {
	var t unattendedTurn
	var msg, st, rt, outcome sql.NullString
	var stop, fin sql.NullTime
	if err := sc.Scan(&t.ID, &t.CoordinatorID, &t.ConvTaskID, &t.SessionID, &msg, &st, &rt, &stop, &outcome, &t.StartedAt, &fin); err != nil {
		return nil, err
	}
	t.MessageID, t.SessionTurnID, t.ReservedTurnID, t.Outcome = msg.String, st.String, rt.String, outcome.String
	if stop.Valid {
		v := stop.Time.UTC()
		t.StopRequestedAt = &v
	}
	if fin.Valid {
		v := fin.Time.UTC()
		t.FinishedAt = &v
	}
	t.StartedAt = t.StartedAt.UTC()
	return &t, nil
}

// getUnattendedTurn reads one turn row, nil when it is gone.
func (s *Store) getUnattendedTurn(ctx context.Context, id string) (*unattendedTurn, error) {
	t, err := scanUnattendedTurn(s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT `+unattendedTurnColumns+` FROM coordinator_unattended_turns WHERE id = ?`), id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read unattended turn: %w", err)
	}
	return t, nil
}

// settleUnsentTurn settles an unbound open row and returns its wakes to
// pending in one transaction. It reports whether the row changed.
func (s *Store) settleUnsentTurn(ctx context.Context, id, outcome string) (bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin unsent settle: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := s.now().UTC()
	res, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE coordinator_unattended_turns SET outcome = ?, finished_at = ?
		WHERE id = ? AND outcome IS NULL AND session_turn_id IS NULL`), outcome, now, id)
	changed, err := rowChanged(res, err, "settle unsent turn")
	if err != nil || !changed {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE coordinator_wakes SET status = ?, turn_id = NULL, updated_at = ? WHERE turn_id = ?`),
		wakeStatusPending, now, id); err != nil {
		return false, fmt.Errorf("return wakes to pending: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit unsent settle: %w", err)
	}
	return true, nil
}

// openTurnBySessionTurn returns the open row bound to a session turn, nil when none.
func (s *Store) openTurnBySessionTurn(ctx context.Context, sessionTurnID string) (*unattendedTurn, error) {
	t, err := scanUnattendedTurn(s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT `+unattendedTurnColumns+` FROM coordinator_unattended_turns
		WHERE session_turn_id = ? AND outcome IS NULL`), sessionTurnID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read bound unattended turn: %w", err)
	}
	return t, nil
}

// settleOpenTurn settles an open row and reports whether this call changed it.
func (s *Store) settleOpenTurn(ctx context.Context, id, outcome string, at time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_unattended_turns SET outcome = ?, finished_at = ?
		WHERE id = ? AND outcome IS NULL`), outcome, at.UTC(), id)
	return rowChanged(res, err, "settle unattended turn")
}

// recentSettledTurns returns the bound rows settled at or after since.
func (s *Store) recentSettledTurns(ctx context.Context, coordinatorID string, since time.Time) ([]*unattendedTurn, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(`SELECT `+unattendedTurnColumns+` FROM coordinator_unattended_turns
		WHERE coordinator_id = ? AND outcome IS NOT NULL AND session_turn_id IS NOT NULL AND finished_at >= ?
		ORDER BY finished_at, id`), coordinatorID, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("list recently settled turns: %w", err)
	}
	return scanTurnRows(rows, "recently settled")
}

// openUnattendedTurns returns the open turn rows of one coordinator, or of
// every coordinator when coordinatorID is empty, oldest first.
func (s *Store) openUnattendedTurns(ctx context.Context, coordinatorID string) ([]*unattendedTurn, error) {
	query := `SELECT ` + unattendedTurnColumns + ` FROM coordinator_unattended_turns WHERE outcome IS NULL`
	var args []any
	if coordinatorID != "" {
		query += ` AND coordinator_id = ?`
		args = append(args, coordinatorID)
	}
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(query+` ORDER BY started_at, id`), args...)
	if err != nil {
		return nil, fmt.Errorf("list open unattended turns: %w", err)
	}
	return scanTurnRows(rows, "open")
}

func scanTurnRows(rows *sql.Rows, what string) ([]*unattendedTurn, error) {
	defer func() { _ = rows.Close() }()
	var out []*unattendedTurn
	for rows.Next() {
		t, err := scanUnattendedTurn(rows)
		if err != nil {
			return nil, fmt.Errorf("scan %s unattended turn: %w", what, err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// recordFoundMessage sets message_id on an open row and reports a change.
func (s *Store) recordFoundMessage(ctx context.Context, id, messageID string) (bool, error) {
	return s.setTurnMessage(ctx, id, messageID)
}
