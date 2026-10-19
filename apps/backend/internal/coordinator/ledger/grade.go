package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// repairLinks sets turn_id on the proposals and guarded-call log rows a call
// wrote before the start handler had put the turn in the active map. A row is
// only claimed when no other turn of the coordinator overlaps the window:
// leaving NULL is always safe.
func (l *Ledger) repairLinks(ctx context.Context, row *turnRow, startedAt, finishedAt time.Time) error {
	db := l.deps.DB
	const noOverlap = ` AND NOT EXISTS (SELECT 1 FROM coordinator_turns t2 WHERE t2.coordinator_id = ? AND t2.id <> ?
		AND t2.started_at <= ? AND (t2.finished_at IS NULL OR t2.finished_at >= ?))`
	stmts := []string{
		`UPDATE coordinator_proposals SET turn_id = ? WHERE coordinator_id = ? AND turn_id IS NULL AND created_at >= ? AND created_at <= ?` + noOverlap,
		`UPDATE coordinator_activity SET turn_id = ? WHERE coordinator_id = ? AND turn_id IS NULL AND created_at >= ? AND created_at <= ?
			AND (outcome = 'proposed' OR "authorization" = 'denied') AND undo_of_id IS NULL` + noOverlap,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, db.Rebind(stmt),
			row.ID, row.CoordinatorID, startedAt.UTC(), finishedAt.UTC(), row.CoordinatorID, row.ID, finishedAt.UTC(), startedAt.UTC()); err != nil {
			return fmt.Errorf("repair turn links: %w", err)
		}
	}
	return nil
}

// gradeTurn gathers the stored facts and applies the pure verdict.
func (l *Ledger) gradeTurn(ctx context.Context, row *turnRow, unattendedID, outcome string) (string, error) {
	db := l.deps.DB
	in := VerdictInput{Outcome: outcome, Trigger: row.Trigger, WakeKinds: decodeKinds(row.WakeKinds), RetryPending: l.calls.retryPending(row.SessionID)}
	var counts struct {
		Calls   int `db:"calls"`
		Refused int `db:"refused"`
	}
	if err := db.GetContext(ctx, &counts, db.Rebind(`
		SELECT COUNT(*) AS calls, COALESCE(SUM(CASE WHEN allowed = ? THEN 1 ELSE 0 END), 0) AS refused
		FROM coordinator_turn_calls WHERE turn_id = ?`), false, row.ID); err != nil {
		return "", fmt.Errorf("read call digest: %w", err)
	}
	in.Calls, in.Refused = counts.Calls, counts.Refused
	if err := db.GetContext(ctx, &in.CallsTruncated, db.Rebind(`SELECT calls_truncated FROM coordinator_turns WHERE id = ?`), row.ID); err != nil {
		return "", fmt.Errorf("read truncation: %w", err)
	}
	if err := db.GetContext(ctx, &in.Proposals, db.Rebind(`SELECT COUNT(*) FROM coordinator_proposals WHERE turn_id = ?`), row.ID); err != nil {
		return "", fmt.Errorf("count turn proposals: %w", err)
	}
	if unattendedID != "" {
		var acted int
		if err := db.GetContext(ctx, &acted, db.Rebind(`
			SELECT COUNT(*) FROM coordinator_activity
			WHERE unattended_turn_id = ? AND outcome = 'approved' AND "authorization" = 'automatic'`), unattendedID); err != nil {
			return "", fmt.Errorf("count automatic approvals: %w", err)
		}
		in.Acted = acted > 0
	}
	in.ActionPending = l.actionPending(ctx, row, in)
	return Verdict(in), nil
}

// actionPending reads the pending-interaction signal only when it can decide
// the verdict; a failed read grades as not pending and is logged.
func (l *Ledger) actionPending(ctx context.Context, row *turnRow, in VerdictInput) bool {
	if in.Trigger != TriggerWake || in.Proposals > 0 || in.Acted || l.deps.ActionPending == nil {
		return false
	}
	pending, err := l.deps.ActionPending(ctx, row.SessionID)
	if err != nil {
		l.log.Warn("coordinator ledger: pending interaction read failed", zap.Error(err))
		return false
	}
	return pending
}

type usageModel struct {
	Model     string `db:"model"`
	AgentType string `db:"agent_type"`
}

// fillModel sets the model and harness of a row from the turn's first usage row
// with a non-empty model, only while the model is still empty.
func (l *Ledger) fillModel(ctx context.Context, rowID, sessionTurnID string) error {
	var u []usageModel
	if err := l.deps.DB.SelectContext(ctx, &u, l.deps.DB.Rebind(`
		SELECT COALESCE(model, '') AS model, COALESCE(agent_type, '') AS agent_type FROM task_usage_events
		WHERE turn_id = ? AND TRIM(COALESCE(model, '')) <> '' ORDER BY created_at, id LIMIT 1`), sessionTurnID); err != nil {
		return fmt.Errorf("read usage model: %w", err)
	}
	if len(u) == 0 {
		return nil
	}
	model := strings.ToLower(strings.TrimSpace(u[0].Model))
	harness := ""
	if at := strings.TrimSpace(u[0].AgentType); at != "" {
		harness = at + "@" + l.deps.BuildVersion
	}
	_, err := l.deps.DB.ExecContext(ctx, l.deps.DB.Rebind(`UPDATE coordinator_turns SET model = ?, harness = ? WHERE id = ? AND model = ''`), model, harness, rowID)
	return err
}
