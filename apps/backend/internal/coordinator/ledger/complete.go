package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/events/bus"
)

const outcomeUnknown = "unknown"

// outcomeReadRetries bounds how often completion waits for the unattended
// row's own turn-end settle before reading the session state instead.
const outcomeReadRetries = 3

// OnTurnCompleted settles the ledger row of a finished coordinator turn.
func (l *Ledger) OnTurnCompleted(ctx context.Context, event *bus.Event) {
	te, ok := parseTurnEvent(event)
	if !ok {
		return
	}
	l.safe(StageComplete, func() error {
		coordinatorID, isCoordinator, err := l.deps.CoordinatorForTask(ctx, te.taskID)
		if err != nil {
			return fmt.Errorf("coordinator for task: %w", err)
		}
		if !isCoordinator {
			return nil
		}
		return l.complete(ctx, coordinatorID, te, 0)
	})
}

func (l *Ledger) complete(ctx context.Context, coordinatorID string, te turnEvent, attempt int) error {
	row, err := l.ensureRow(ctx, coordinatorID, te)
	if err != nil {
		return err
	}
	if row.FinishedAt.Valid {
		l.dropActive(te.sessionID, "", te.sessionTurnID)
		return nil
	}
	row, unattendedID, err := l.reevaluateTrigger(ctx, row, te)
	if err != nil {
		return err
	}
	outcome, retry, err := l.turnOutcome(ctx, te, attempt)
	if err != nil {
		return err
	}
	if retry {
		l.scheduleCompletion(coordinatorID, te, attempt+1)
		return nil
	}
	finishedAt := te.completedAt
	if finishedAt.IsZero() {
		finishedAt = l.now()
	}
	startedAt := row.StartedAt
	l.safe(StageLink, func() error { return l.repairLinks(ctx, row, startedAt, finishedAt) })
	l.drainCalls(ctx)
	l.applyTruncations()
	verdict, err := l.gradeTurn(ctx, row, unattendedID, outcome)
	if err != nil {
		return err
	}
	res, err := l.deps.DB.ExecContext(ctx, l.deps.DB.Rebind(
		`UPDATE coordinator_turns SET finished_at = ?, outcome = ?, verdict = ? WHERE id = ? AND finished_at IS NULL`),
		finishedAt.UTC(), outcome, verdict, row.ID)
	if err != nil {
		return fmt.Errorf("complete ledger turn: %w", err)
	}
	l.dropActive(te.sessionID, "", te.sessionTurnID)
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	l.safe(StageModel, func() error { return l.fillModel(ctx, row.ID, te.sessionTurnID) })
	return nil
}

// scheduleCompletion re-runs completion after the retry interval, so a turn
// whose bound row is not yet settled is read again.
func (l *Ledger) scheduleCompletion(coordinatorID string, te turnEvent, attempt int) {
	l.lifeMu.Lock()
	if l.stopped {
		l.lifeMu.Unlock()
		return
	}
	l.wg.Add(1)
	l.lifeMu.Unlock()
	go func() {
		defer l.wg.Done()
		timer := time.NewTimer(l.retryEvery)
		defer timer.Stop()
		select {
		case <-l.stopCh:
			return
		case <-timer.C:
		}
		l.safe(StageComplete, func() error { return l.complete(context.Background(), coordinatorID, te, attempt) })
	}()
}

// ensureRow returns the turn's row, inserting a late row when start never
// recorded one. A late row has no snapshot.
func (l *Ledger) ensureRow(ctx context.Context, coordinatorID string, te turnEvent) (*turnRow, error) {
	if row, err := l.turnBySessionTurn(ctx, te.sessionID, te.sessionTurnID); err != nil || row != nil {
		return row, err
	}
	late := te
	if started, err := l.sessionTurnStart(ctx, te.sessionTurnID); err != nil {
		return nil, err
	} else if !started.IsZero() {
		late.startedAt = started
	}
	if late.startedAt.IsZero() {
		late.startedAt = l.now()
	}
	nr, _, err := l.buildRow(ctx, coordinatorID, late, uuid.NewString(), false)
	if err != nil {
		return nil, err
	}
	created, err := l.insertRow(ctx, nr, nil)
	if err != nil {
		return nil, err
	}
	if created {
		l.linkUnattended(ctx, nr.id, nr.unattendedID)
	}
	row, err := l.turnBySessionTurn(ctx, te.sessionID, te.sessionTurnID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("late ledger row missing after insert")
	}
	return row, nil
}

func (l *Ledger) sessionTurnStart(ctx context.Context, sessionTurnID string) (time.Time, error) {
	var t time.Time
	err := l.deps.DB.GetContext(ctx, &t, l.deps.DB.Rebind(`SELECT started_at FROM task_session_turns WHERE id = ?`), sessionTurnID)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("read session turn start: %w", err)
	}
	return t.UTC(), nil
}

// reevaluateTrigger turns a turn recorded as a message into a wake when its
// unattended row now matches. It returns the id of the unattended row bound to
// the turn ("" when none).
func (l *Ledger) reevaluateTrigger(ctx context.Context, row *turnRow, te turnEvent) (*turnRow, string, error) {
	m, err := l.findUnattended(ctx, te.sessionID, te.sessionTurnID)
	if err != nil || m == nil {
		return row, "", err
	}
	if row.Trigger != TriggerMessage {
		return row, m.ID, nil
	}
	kinds, err := l.wakeKinds(ctx, m.ID)
	if err != nil {
		return row, m.ID, err
	}
	res, err := l.deps.DB.ExecContext(ctx, l.deps.DB.Rebind(
		`UPDATE coordinator_turns SET "trigger" = ?, wake_kinds = ? WHERE id = ? AND "trigger" = ?`),
		TriggerWake, encodeKinds(kinds), row.ID, TriggerMessage)
	if err != nil {
		return row, m.ID, fmt.Errorf("correct ledger trigger: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		row.Trigger, row.WakeKinds = TriggerWake, encodeKinds(kinds)
		l.linkUnattended(ctx, row.ID, m.ID)
	}
	return row, m.ID, nil
}

// turnOutcome reads the outcome from the bound unattended row, else from the
// session state. retry is true when the bound row has not settled yet and the
// retry budget remains.
func (l *Ledger) turnOutcome(ctx context.Context, te turnEvent, attempt int) (outcome string, retry bool, err error) {
	var bound sql.NullString
	var found bool
	var rows []sql.NullString
	if err := l.deps.DB.SelectContext(ctx, &rows, l.deps.DB.Rebind(
		`SELECT outcome FROM coordinator_unattended_turns WHERE session_id = ? AND session_turn_id = ? ORDER BY started_at DESC, id DESC LIMIT 1`),
		te.sessionID, te.sessionTurnID); err != nil {
		return "", false, fmt.Errorf("read bound outcome: %w", err)
	}
	if len(rows) == 1 {
		found, bound = true, rows[0]
	}
	if found && bound.Valid && bound.String != "" {
		return bound.String, false, nil
	}
	if found && attempt < outcomeReadRetries {
		return "", true, nil
	}
	return l.sessionStateOutcome(ctx, te.sessionID), false, nil
}

func (l *Ledger) sessionStateOutcome(ctx context.Context, sessionID string) string {
	var state string
	if err := l.deps.DB.GetContext(ctx, &state, l.deps.DB.Rebind(`SELECT state FROM task_sessions WHERE id = ?`), sessionID); err != nil {
		return outcomeUnknown
	}
	switch strings.ToUpper(state) {
	case "FAILED":
		return "failed"
	case "CANCELLED":
		return "cancelled"
	case "COMPLETED", "IDLE", "WAITING_FOR_INPUT":
		return "completed"
	}
	return outcomeUnknown
}
