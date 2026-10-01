package recorder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator/outcomes"
	"github.com/kandev/kandev/internal/db/dialect"
)

// Grader computes and stores the outcome row of a decided proposal.
type Grader struct {
	db  *sqlx.DB
	log *zap.Logger
}

// NewGrader builds a grader writing through db.
func NewGrader(db *sqlx.DB, log *zap.Logger) *Grader {
	if log == nil {
		log = zap.NewNop()
	}
	return &Grader{db: db, log: log}
}

const createTaskKind = "create_task"

type proposalFacts struct {
	ID                   string         `db:"id"`
	CoordinatorID        string         `db:"coordinator_id"`
	Status               string         `db:"status"`
	Kind                 string         `db:"kind"`
	SpecJSON             string         `db:"spec_json"`
	FinalSpecJSON        sql.NullString `db:"final_spec_json"`
	RejectReason         sql.NullString `db:"reject_reason"`
	TaskID               sql.NullString `db:"task_id"`
	TurnID               sql.NullString `db:"turn_id"`
	ClaimedAutomatically bool           `db:"claimed_automatically"`
	UpdatedAt            time.Time      `db:"updated_at"`
}

type storedOutcome struct {
	TurnID      sql.NullString `db:"turn_id"`
	TaskResult  sql.NullString `db:"task_result"`
	Cost        sql.NullInt64  `db:"cost_subcents"`
	MergedAt    sql.NullTime   `db:"merged_at"`
	LastStepID  string         `db:"last_step_id"`
	ReopenCount int            `db:"reopen_count"`
	Final       bool           `db:"final"`
}

// outcomeValues is a row ready to upsert.
type outcomeValues struct {
	proposalID, coordinatorID, kind, decision string
	turnID                                    sql.NullString
	automatic                                 bool
	decidedAt                                 time.Time
	editedFields, reasonCode, lastStepID      string
	taskID, taskResult                        sql.NullString
	cost                                      sql.NullInt64
	approvedAt, mergedAt                      sql.NullTime
	final                                     bool
	gradedAt                                  time.Time
}

// Grade recomputes the outcome row of proposalID from stored facts and upserts
// it. decidedAt is the hook's decision time, zero for every other caller. A
// proposal without a decision gets no row. A read error keeps the earlier row,
// is counted and returned.
func (g *Grader) Grade(ctx context.Context, proposalID string, decidedAt time.Time) error {
	tx, err := g.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin grade: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockOutcome(ctx, tx, g.db.DriverName(), proposalID); err != nil {
		return err
	}
	p, err := readProposal(ctx, tx, proposalID)
	if err != nil {
		return failRead(GradeProposalRead, err)
	}
	if p == nil || !decided(p.Status) {
		return nil
	}
	stored, err := readStored(ctx, tx, proposalID)
	if err != nil {
		return failRead(GradeProposalRead, err)
	}
	v, err := g.derive(ctx, tx, p, stored, decidedAt)
	if err != nil {
		return err
	}
	if stored != nil && !stored.Final && !v.final && outcomes.Reopened(stored.TaskResult.String, v.taskResult.String) {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE coordinator_outcomes SET reopen_count = reopen_count + 1
			WHERE proposal_id = ? AND task_result = ? AND final = ?`), proposalID, outcomes.ResultDone, false); err != nil {
			return fmt.Errorf("count reopening: %w", err)
		}
	}
	if err := upsertOutcome(ctx, tx, v); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit grade: %w", err)
	}
	return nil
}

func failRead(reason string, err error) error {
	bump(gradeFailedTotal, reason)
	return fmt.Errorf("grade %s: %w", reason, err)
}

func decided(status string) bool {
	return status == "approved" || status == "rejected" || status == "returned"
}

// lockOutcome serialises graders of one proposal. SQLite has one writer
// connection, so the transaction itself serialises them.
func lockOutcome(ctx context.Context, tx *sqlx.Tx, driver, proposalID string) error {
	if !dialect.IsPostgres(driver) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`SELECT pg_advisory_xact_lock(hashtextextended('coordinator_outcome:' || ?, 0))`), proposalID); err != nil {
		return fmt.Errorf("lock outcome: %w", err)
	}
	return nil
}

func readProposal(ctx context.Context, tx *sqlx.Tx, id string) (*proposalFacts, error) {
	var p proposalFacts
	err := tx.GetContext(ctx, &p, tx.Rebind(`
		SELECT id, coordinator_id, status, COALESCE(NULLIF(kind, ''), 'create_task') AS kind, spec_json, final_spec_json,
		       reject_reason, task_id, turn_id, claimed_automatically, updated_at
		FROM coordinator_proposals WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func readStored(ctx context.Context, tx *sqlx.Tx, id string) (*storedOutcome, error) {
	var s storedOutcome
	err := tx.GetContext(ctx, &s, tx.Rebind(`SELECT turn_id, task_result, cost_subcents, merged_at, last_step_id, reopen_count, final FROM coordinator_outcomes WHERE proposal_id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// derive computes every column except decided_at's first value and the
// reopen count from the stored facts.
func (g *Grader) derive(ctx context.Context, tx *sqlx.Tx, p *proposalFacts, stored *storedOutcome, decidedAt time.Time) (outcomeValues, error) {
	approvedAt, undone, err := readApprovedAction(ctx, tx, p.ID)
	if err != nil {
		return outcomeValues{}, failRead(GradeProposalRead, err)
	}
	v := outcomeValues{
		proposalID: p.ID, coordinatorID: p.CoordinatorID, kind: p.Kind, turnID: p.TurnID,
		automatic: p.ClaimedAutomatically && p.Status == "approved",
		decidedAt: decidedAt.UTC(), reasonCode: outcomes.ReasonNone, gradedAt: time.Now().UTC(),
		approvedAt: approvedAt,
	}
	if v.decidedAt.IsZero() {
		v.decidedAt = p.UpdatedAt.UTC()
	}
	edited := outcomes.EditedFields([]byte(p.SpecJSON), []byte(p.FinalSpecJSON.String))
	encoded, _ := json.Marshal(edited)
	v.editedFields = string(encoded)
	switch {
	case p.Status == "rejected":
		v.decision, v.reasonCode = outcomes.DecisionRejected, outcomes.Code("", p.RejectReason.String)
	case p.Status == "returned":
		v.decision = outcomes.DecisionReturned
	case undone:
		v.decision = outcomes.DecisionUndone
	case len(edited) > 0:
		v.decision = outcomes.DecisionEdited
	default:
		v.decision = outcomes.DecisionApproved
	}
	v.final = v.decision == outcomes.DecisionRejected || v.decision == outcomes.DecisionReturned || v.decision == outcomes.DecisionUndone
	if p.Kind != createTaskKind || !p.TaskID.Valid {
		v.final = true
		return v, nil
	}
	if err := g.gradeTask(ctx, tx, p.TaskID.String, stored, &v); err != nil {
		return outcomeValues{}, err
	}
	return v, nil
}

// readApprovedAction reads the approved created or moved activity row of the
// proposal: its time, and whether it was undone.
func readApprovedAction(ctx context.Context, tx *sqlx.Tx, proposalID string) (sql.NullTime, bool, error) {
	var row struct {
		CreatedAt time.Time    `db:"created_at"`
		UndoneAt  sql.NullTime `db:"undone_at"`
	}
	err := tx.GetContext(ctx, &row, tx.Rebind(`
		SELECT created_at, undone_at FROM coordinator_activity
		WHERE proposal_id = ? AND outcome = 'approved' AND action_class IN ('create_task', 'move')
		ORDER BY created_at, id LIMIT 1`), proposalID)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullTime{}, false, nil
	}
	if err != nil {
		return sql.NullTime{}, false, err
	}
	return sql.NullTime{Time: row.CreatedAt.UTC(), Valid: true}, row.UndoneAt.Valid, nil
}

// gradeTask fills the task columns. A task found deleted keeps the stored
// result and ends grading.
func (g *Grader) gradeTask(ctx context.Context, tx *sqlx.Tx, taskID string, stored *storedOutcome, v *outcomeValues) error {
	v.taskID = sql.NullString{String: taskID, Valid: true}
	facts, stepID, merged, found, err := readTaskFacts(ctx, tx, taskID)
	if err != nil {
		return failRead(GradeTaskRead, err)
	}
	if !found {
		v.final = true
		if stored != nil {
			v.taskResult, v.cost, v.mergedAt, v.lastStepID = stored.TaskResult, stored.Cost, stored.MergedAt, stored.LastStepID
		}
		return nil
	}
	v.lastStepID = stepID
	v.mergedAt = merged
	result := outcomes.TaskResult(facts)
	v.taskResult = sql.NullString{String: result, Valid: true}
	if outcomes.IsFinalResult(result) {
		v.final = true
	}
	cost, err := readTaskCost(ctx, tx, taskID)
	if err != nil {
		return failRead(GradeUsageRead, err)
	}
	v.cost = cost
	return nil
}

func readTaskFacts(ctx context.Context, tx *sqlx.Tx, taskID string) (facts outcomes.TaskFacts, stepID string, mergedAt sql.NullTime, found bool, err error) {
	var task struct {
		StepID     sql.NullString `db:"workflow_step_id"`
		ArchivedAt sql.NullTime   `db:"archived_at"`
	}
	if err = tx.GetContext(ctx, &task, tx.Rebind(`SELECT workflow_step_id, archived_at FROM tasks WHERE id = ?`), taskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return facts, "", mergedAt, false, nil
		}
		return facts, "", mergedAt, false, err
	}
	stepID, facts.Archived = task.StepID.String, task.ArchivedAt.Valid
	var completes sql.NullBool
	if err = tx.GetContext(ctx, &completes, tx.Rebind(`SELECT complete_task_on_enter FROM workflow_steps WHERE id = ?`), stepID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return facts, "", mergedAt, false, err
	}
	facts.StepCompletes = completes.Valid && completes.Bool
	var mergedTimes []sql.NullTime
	if err = tx.SelectContext(ctx, &mergedTimes, tx.Rebind(`SELECT merged_at FROM github_task_prs WHERE task_id = ? AND state = 'merged' AND detached_at IS NULL ORDER BY merged_at`), taskID); err != nil {
		return facts, "", mergedAt, false, err
	}
	if len(mergedTimes) > 0 {
		facts.PRMerged = true
		for _, t := range mergedTimes {
			if t.Valid {
				mergedAt = sql.NullTime{Time: t.Time.UTC(), Valid: true}
				break
			}
		}
	}
	var state sql.NullString
	err = tx.GetContext(ctx, &state, tx.Rebind(`SELECT state FROM task_sessions WHERE task_id = ? ORDER BY started_at DESC, id DESC LIMIT 1`), taskID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return facts, "", mergedAt, false, err
	}
	facts.LatestSessionFailed = state.String == "FAILED"
	return facts, stepID, mergedAt, true, nil
}

// readTaskCost sums priced usage rows of the task; it is null, never zero,
// when there are none or any row is unpriced.
func readTaskCost(ctx context.Context, tx *sqlx.Tx, taskID string) (sql.NullInt64, error) {
	var u struct {
		Rows     int64 `db:"n"`
		Unpriced int64 `db:"unpriced"`
		Cost     int64 `db:"cost"`
	}
	if err := tx.GetContext(ctx, &u, tx.Rebind(`
		SELECT COUNT(*) AS n, COALESCE(SUM(CASE WHEN cost_source = 'unpriced' THEN 1 ELSE 0 END), 0) AS unpriced,
		       COALESCE(SUM(cost_subcents), 0) AS cost
		FROM task_usage_events WHERE task_id = ?`), taskID); err != nil {
		return sql.NullInt64{}, err
	}
	if u.Rows == 0 || u.Unpriced > 0 {
		return sql.NullInt64{}, nil
	}
	return sql.NullInt64{Int64: u.Cost, Valid: true}, nil
}

// settledSet is the per-column SET clause of the guarded upsert: a final row
// keeps what it stored.
func settledSet(col string) string {
	return col + ` = CASE WHEN coordinator_outcomes.final THEN coordinator_outcomes.` + col + ` ELSE excluded.` + col + ` END`
}

const upsertHead = `
	INSERT INTO coordinator_outcomes (proposal_id, coordinator_id, turn_id, kind, decision, automatic, decided_at, edited_fields,
		reason_code, task_id, task_result, cost_subcents, reopen_count, approved_at, merged_at, last_step_id, final, graded_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?)
	ON CONFLICT (proposal_id) DO UPDATE SET
		turn_id = COALESCE(coordinator_outcomes.turn_id, excluded.turn_id),
		decision = CASE WHEN coordinator_outcomes.final AND excluded.decision <> 'undone' THEN coordinator_outcomes.decision ELSE excluded.decision END,
		`

func upsertSQL() string {
	sets := ""
	for i, col := range []string{"edited_fields", "reason_code", "task_result", "cost_subcents", "approved_at", "merged_at", "last_step_id", "final", "graded_at"} {
		if i > 0 {
			sets += ",\n\t\t"
		}
		sets += settledSet(col)
	}
	return upsertHead + sets + `
	WHERE coordinator_outcomes.final = ? OR excluded.decision = 'undone'
		OR (coordinator_outcomes.turn_id IS NULL AND excluded.turn_id IS NOT NULL)`
}

func upsertOutcome(ctx context.Context, tx *sqlx.Tx, v outcomeValues) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(upsertSQL()),
		v.proposalID, v.coordinatorID, v.turnID, v.kind, v.decision, v.automatic, v.decidedAt, v.editedFields,
		v.reasonCode, v.taskID, v.taskResult, v.cost, v.approvedAt, v.mergedAt, v.lastStepID, v.final, v.gradedAt, false); err != nil {
		return fmt.Errorf("upsert outcome: %w", err)
	}
	return nil
}
