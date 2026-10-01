package recorder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator/outcomes"
)

const (
	moveBackWindow = 30 * 24 * time.Hour
	seenJudged     = "judged"
	seenRetry      = "retry"
	actionCreate   = "create_task"
	actionMove     = "move"
)

type approvedAction struct {
	CoordinatorID string       `db:"coordinator_id"`
	WorkspaceID   string       `db:"workspace_id"`
	ProposalID    string       `db:"proposal_id"`
	ActionClass   string       `db:"action_class"`
	CreatedAt     time.Time    `db:"created_at"`
	UndoneAt      sql.NullTime `db:"undone_at"`
}

type historyRow struct {
	ID        int64          `db:"id"`
	ToStepID  string         `db:"to_step_id"`
	ActorID   sql.NullString `db:"actor_id"`
	CreatedAt time.Time      `db:"created_at"`
}

type stepInfo struct {
	workflowID string
	position   int
}

// scanState is what one scan of one task reads before judging.
type scanState struct {
	taskID     string
	workflowID string
	actions    []approvedAction
	rows       []historyRow
	steps      map[string]stepInfo
	seen       map[seenKey]string
}

type seenKey struct {
	row         int64
	coordinator string
}

// ScanTask judges every unseen backward move of taskID against each
// coordinator that acted on it. It returns the creation times of every history
// row it read, so a retry chain can tell whether a row has arrived; it returns
// none when the task has no coordinator action to compare against.
func (c *Capture) ScanTask(ctx context.Context, taskID string) ([]time.Time, error) {
	st, err := c.loadScan(ctx, taskID)
	if err != nil || st == nil {
		return nil, err
	}
	times := make([]time.Time, len(st.rows))
	for i, r := range st.rows {
		times[i] = r.CreatedAt
	}
	for _, r := range st.rows {
		for _, coordID := range st.coordinators() {
			if s := st.seen[seenKey{r.ID, coordID}]; s == seenJudged {
				continue
			}
			c.judgeRow(ctx, st, coordID, r)
		}
	}
	return times, nil
}

func (st *scanState) coordinators() []string {
	set := map[string]bool{}
	var ids []string
	for _, a := range st.actions {
		if !set[a.CoordinatorID] {
			set[a.CoordinatorID] = true
			ids = append(ids, a.CoordinatorID)
		}
	}
	sort.Strings(ids)
	return ids
}

// loadScan reads the task's actions, history rows, step order and seen rows. A
// nil state with a nil error means there is nothing to judge.
func (c *Capture) loadScan(ctx context.Context, taskID string) (*scanState, error) {
	st := &scanState{taskID: taskID, steps: map[string]stepInfo{}, seen: map[seenKey]string{}}
	if err := c.db.SelectContext(ctx, &st.actions, c.db.Rebind(`
		SELECT coordinator_id, workspace_id, proposal_id, action_class, created_at, undone_at FROM coordinator_activity
		WHERE target_task_id = ? AND outcome = 'approved' AND action_class IN ('create_task', 'move') AND proposal_id IS NOT NULL
		ORDER BY created_at, proposal_id`), taskID); err != nil {
		bump(readFailedTotal, ReadFailedReferenceAction)
		return nil, fmt.Errorf("read approved actions: %w", err)
	}
	if len(st.actions) == 0 {
		return nil, nil
	}
	earliest := st.actions[0].CreatedAt
	floor := c.now().Add(-moveBackWindow)
	if floor.After(earliest) {
		earliest = floor
	}
	if err := c.db.SelectContext(ctx, &st.rows, c.db.Rebind(`
		SELECT h.id, h.to_step_id, h.actor_id, h.created_at FROM session_step_history h
		JOIN task_sessions s ON s.id = h.session_id
		WHERE s.task_id = ? AND h.created_at > ? ORDER BY h.created_at, h.id`), taskID, earliest.UTC()); err != nil {
		bump(readFailedTotal, ReadFailedHistoryRows)
		return nil, fmt.Errorf("read history rows: %w", err)
	}
	if len(st.rows) == 0 {
		return st, nil
	}
	if err := c.loadSteps(ctx, st); err != nil {
		return nil, err
	}
	return st, c.loadSeen(ctx, st)
}

func (c *Capture) loadSteps(ctx context.Context, st *scanState) error {
	var wf sql.NullString
	if err := c.db.GetContext(ctx, &wf, c.db.Rebind(`SELECT workflow_id FROM tasks WHERE id = ?`), st.taskID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		bump(readFailedTotal, ReadFailedStepOrder)
		return fmt.Errorf("read task workflow: %w", err)
	}
	st.workflowID = wf.String
	var steps []struct {
		ID         string `db:"id"`
		WorkflowID string `db:"workflow_id"`
		Position   int    `db:"position"`
	}
	if err := c.db.SelectContext(ctx, &steps, `SELECT id, workflow_id, position FROM workflow_steps`); err != nil {
		bump(readFailedTotal, ReadFailedStepOrder)
		return fmt.Errorf("read step order: %w", err)
	}
	for _, s := range steps {
		st.steps[s.ID] = stepInfo{workflowID: s.WorkflowID, position: s.Position}
	}
	return nil
}

func (c *Capture) loadSeen(ctx context.Context, st *scanState) error {
	var seen []struct {
		Row   int64  `db:"history_row_id"`
		Coord string `db:"coordinator_id"`
		State string `db:"state"`
	}
	if err := c.db.SelectContext(ctx, &seen, c.db.Rebind(`
		SELECT m.history_row_id, m.coordinator_id, m.state FROM coordinator_moveback_seen m
		WHERE m.history_row_id IN (SELECT h.id FROM session_step_history h JOIN task_sessions s ON s.id = h.session_id WHERE s.task_id = ?)`), st.taskID); err != nil {
		bump(readFailedTotal, ReadFailedHistoryRows)
		return fmt.Errorf("read seen rows: %w", err)
	}
	for _, s := range seen {
		st.seen[seenKey{s.Row, s.Coord}] = s.State
	}
	return nil
}

// reference picks the approved action of coordID that was current when the
// row was written: not undone as of the row, greatest creation time earlier
// than the row, ties by proposal id descending.
func (st *scanState) reference(coordID string, row historyRow) *approvedAction {
	var best *approvedAction
	for i := range st.actions {
		a := &st.actions[i]
		if a.CoordinatorID != coordID || !a.CreatedAt.Before(row.CreatedAt) {
			continue
		}
		if a.UndoneAt.Valid && !a.UndoneAt.Time.After(row.CreatedAt) {
			continue
		}
		if best == nil || a.CreatedAt.After(best.CreatedAt) || a.CreatedAt.Equal(best.CreatedAt) && a.ProposalID > best.ProposalID {
			best = a
		}
	}
	return best
}

// judgeRow judges one candidate once, in the order position, actor, manager.
func (c *Capture) judgeRow(ctx context.Context, st *scanState, coordID string, row historyRow) {
	ref := st.reference(coordID, row)
	if ref == nil {
		c.markSeen(ctx, coordID, row, seenJudged, nil)
		return
	}
	refStep, ok, err := c.referenceStep(ctx, st, ref)
	if err != nil {
		return
	}
	dest, inFlow := st.steps[row.ToStepID]
	if !ok || !inFlow || dest.workflowID != st.workflowID || dest.position >= st.steps[refStep].position {
		c.markSeen(ctx, coordID, row, seenJudged, nil)
		return
	}
	if !row.ActorID.Valid || row.ActorID.String == "" {
		c.markSeen(ctx, coordID, row, seenJudged, func() { bump(ignoredTotal, IgnoredActorUnknown) })
		return
	}
	c.judgeManager(ctx, coordID, ref, row)
}

func (c *Capture) judgeManager(ctx context.Context, coordID string, ref *approvedAction, row historyRow) {
	verdict, err := c.checker.Check(ctx, ref.WorkspaceID, row.ActorID.String)
	if err != nil {
		c.commitJudgement(ctx, coordID, row, seenRetry, nil, func() { bump(ignoredTotal, IgnoredAuthzError) })
		return
	}
	if reason := verdictReason(verdict); reason != "" {
		c.commitJudgement(ctx, coordID, row, seenJudged, nil, func() { bump(ignoredTotal, reason) })
		return
	}
	var p struct {
		TurnID sql.NullString `db:"turn_id"`
		Kind   string         `db:"kind"`
	}
	if err := c.db.GetContext(ctx, &p, c.db.Rebind(`SELECT turn_id, COALESCE(NULLIF(kind, ''), 'create_task') AS kind FROM coordinator_proposals WHERE id = ?`), ref.ProposalID); err != nil {
		bump(readFailedTotal, ReadFailedReferenceSpec)
		return
	}
	c.commitJudgement(ctx, coordID, row, seenJudged, &feedbackRow{
		coordinatorID: coordID, kind: FeedbackMovedBack, proposalID: ref.ProposalID, turnID: p.TurnID, userID: row.ActorID.String,
		reasonCode: outcomes.ReasonNone, proposalKind: p.Kind, toStepID: row.ToStepID, key: strconv.FormatInt(row.ID, 10), createdAt: row.CreatedAt,
	}, nil)
}

// referenceStep resolves the step the reference action left the card in. ok is
// false when the proposal names none that can be compared.
func (c *Capture) referenceStep(ctx context.Context, st *scanState, ref *approvedAction) (string, bool, error) {
	var p struct {
		Spec  string         `db:"spec_json"`
		Final sql.NullString `db:"final_spec_json"`
	}
	if err := c.db.GetContext(ctx, &p, c.db.Rebind(`SELECT spec_json, final_spec_json FROM coordinator_proposals WHERE id = ?`), ref.ProposalID); err != nil {
		bump(readFailedTotal, ReadFailedReferenceSpec)
		return "", false, err
	}
	raw := p.Spec
	if p.Final.Valid && p.Final.String != "" {
		raw = p.Final.String
	}
	var spec struct {
		StepID     string `json:"step_id"`
		ToStepID   string `json:"to_step_id"`
		WorkflowID string `json:"workflow_id"`
	}
	_ = json.Unmarshal([]byte(raw), &spec)
	step := spec.StepID
	if ref.ActionClass == actionMove {
		step = spec.ToStepID
	}
	if step == "" && ref.ActionClass == actionCreate {
		return c.startStep(ctx, st, spec.WorkflowID)
	}
	_, known := st.steps[step]
	return step, known, nil
}

func (c *Capture) startStep(ctx context.Context, st *scanState, workflowID string) (string, bool, error) {
	if workflowID == "" {
		workflowID = st.workflowID
	}
	var id string
	err := c.db.GetContext(ctx, &id, c.db.Rebind(`SELECT id FROM workflow_steps WHERE workflow_id = ? AND is_start_step = ? ORDER BY position LIMIT 1`), workflowID, true)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		bump(readFailedTotal, ReadFailedReferenceSpec)
		return "", false, err
	}
	return id, true, nil
}

// markSeen records a candidate that ends without an observation.
func (c *Capture) markSeen(ctx context.Context, coordID string, row historyRow, state string, after func()) {
	c.commitJudgement(ctx, coordID, row, state, nil, after)
}

// commitJudgement marks the pair seen and stores its observation in one
// transaction. The observation and after (the counter increment) run only when
// this call produced the seen row, so two scans of one row yield one effect.
func (c *Capture) commitJudgement(ctx context.Context, coordID string, row historyRow, state string, fb *feedbackRow, after func()) {
	tx, err := c.db.BeginTxx(ctx, nil)
	if err != nil {
		c.log.Warn("coordinator override: begin failed", zap.Error(err))
		return
	}
	defer func() { _ = tx.Rollback() }()
	inserted, err := c.insertSeen(ctx, tx, coordID, row.ID, state)
	if err != nil || !inserted {
		if err != nil {
			c.log.Warn("coordinator override: seen insert failed", zap.Error(err))
		}
		return
	}
	if fb != nil {
		if _, err := c.insertFeedback(ctx, tx, *fb); err != nil {
			c.log.Warn("coordinator override: feedback insert failed", zap.Error(err))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		c.log.Warn("coordinator override: commit failed", zap.Error(err))
		return
	}
	if after != nil {
		after()
	}
}

// insertSeen reports whether the call inserted the pair or promoted a retry
// to judged. A retry never replaces an existing row.
func (c *Capture) insertSeen(ctx context.Context, tx execer, coordID string, rowID int64, state string) (bool, error) {
	conflict := `DO NOTHING`
	if state == seenJudged {
		conflict = `DO UPDATE SET state = excluded.state, seen_at = excluded.seen_at WHERE coordinator_moveback_seen.state = 'retry'`
	}
	res, err := tx.ExecContext(ctx, c.db.Rebind(`INSERT INTO coordinator_moveback_seen (history_row_id, coordinator_id, seen_at, state)
		VALUES (?, ?, ?, ?) ON CONFLICT (history_row_id, coordinator_id) `+conflict), rowID, coordID, c.now().UTC(), state)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
