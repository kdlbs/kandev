package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// Participant is the office-flavour view of a task participant row. After
// ADR 0005 Wave C the data lives in workflow_step_participants — we keep
// this struct shape so existing office callers (dashboard, scheduler)
// don't need to learn about workflow_step rows. CreatedAt is best-effort:
// workflow_step_participants doesn't store it, so we emit zero time.
type Participant struct {
	TaskID           string    `db:"task_id" json:"task_id"`
	AgentProfileID   string    `db:"agent_profile_id" json:"agent_profile_id"`
	Role             string    `db:"role" json:"role"`
	DecisionRequired bool      `db:"decision_required" json:"decision_required"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
}

// stepIDForTask resolves the current workflow_step_id for the given task.
// Returns "" with no error when the task has no step yet — that case must
// short-circuit calls (you cannot create a participant without a step).
func (r *Repository) stepIDForTask(ctx context.Context, taskID string) (string, error) {
	var stepID sql.NullString
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT workflow_step_id FROM tasks WHERE id = ?`,
	), taskID).Scan(&stepID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !stepID.Valid {
		return "", nil
	}
	return stepID.String, nil
}

// GetTaskWorkflowStepID returns the current workflow_step_id for the given
// task. Returns "" with no error when the task has no step bound. Exposed
// so the dashboard service can resolve a task's step before recording a
// workflow_step_decisions row (ADR 0005 Wave E).
func (r *Repository) GetTaskWorkflowStepID(ctx context.Context, taskID string) (string, error) {
	return r.stepIDForTask(ctx, taskID)
}

// GetWorkflowStepStageType returns the persisted stage type for a workflow
// step. It returns an empty string when the step does not exist so callers
// can apply compatibility fallbacks for older run payloads.
func (r *Repository) GetWorkflowStepStageType(ctx context.Context, stepID string) (string, error) {
	if stepID == "" {
		return "", nil
	}
	var stageType sql.NullString
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT stage_type FROM workflow_steps WHERE id = ?`,
	), stepID).Scan(&stageType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !stageType.Valid {
		return "", nil
	}
	return stageType.String, nil
}

// ParticipantWriteOutcome identifies what AddTaskParticipant did to the
// role slate.
type ParticipantWriteOutcome string

const (
	// ParticipantWriteOutcomeClaimed means an existing unclaimed "auto"
	// seat was reassigned to the named agent profile and marked "manual".
	ParticipantWriteOutcomeClaimed ParticipantWriteOutcome = "claimed"
	// ParticipantWriteOutcomeInserted means a new "manual" seat was written.
	ParticipantWriteOutcomeInserted ParticipantWriteOutcome = "inserted"
	// ParticipantWriteOutcomeUnchanged means the store was not written:
	// either the task has no step (or does not exist), or the named agent
	// already held a seat at that identity — including a promotion from
	// "auto" to "manual" provenance in place, which changes a column but
	// produces no service-layer effect and so is reported the same as any
	// other unchanged write.
	ParticipantWriteOutcomeUnchanged ParticipantWriteOutcome = "unchanged"
)

// ParticipantWriteResult reports what AddTaskParticipant did so the caller
// can drive the outcome's side effects (activity entry, displaced session
// termination, displaced run cancellation) from values captured inside the
// transaction, instead of re-reading the task's step after commit — a
// re-read could observe a step the task has since left.
type ParticipantWriteResult struct {
	Outcome ParticipantWriteOutcome
	// StepID is the step the write landed at. Empty when Outcome is
	// Unchanged.
	StepID string
	// DisplacedAgentProfileID is the agent profile a claim reassigned the
	// seat away from. Empty on every outcome except Claimed.
	DisplacedAgentProfileID string
}

// AddTaskParticipant registers agentID in role for taskID, claiming an
// unclaimed automatic seat in place when one exists rather than always
// inserting a second seat into the role's slate. Returns
// ParticipantWriteOutcomeUnchanged, with no error, when the task has no
// workflow_step_id yet or does not exist — the caller surface gives no way
// to reject creation, and a later step-set will not retroactively add the
// participant.
//
// Both this writer and automatic casting's EnsureRoleSeat
// (internal/workflow/repository) serialize on the same advisory lock,
// ParticipantRoleSeatLockKey(taskID, role), acquired as the transaction's
// first statement on the server dialect. The task's current step is then
// resolved on the transaction handle, inside that lock — never through the
// read-only pool, which would escape the exclusion as surely as a
// mismatched lock key would on the server dialect.
func (r *Repository) AddTaskParticipant(ctx context.Context, taskID, agentID, role string) (ParticipantWriteResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return ParticipantWriteResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.lockParticipantRoleSeat(ctx, tx, taskID, role); err != nil {
		return ParticipantWriteResult{}, err
	}

	stepID, err := r.stepIDForTaskTx(ctx, tx, taskID)
	if err != nil {
		return ParticipantWriteResult{}, fmt.Errorf("resolve task workflow_step_id: %w", err)
	}
	if stepID == "" {
		// Task has no step, or does not exist at all — commit having
		// written nothing.
		return commitParticipantWrite(tx, ParticipantWriteResult{Outcome: ParticipantWriteOutcomeUnchanged})
	}

	unchanged, err := r.probeExistingIdentity(ctx, tx, stepID, taskID, role, agentID)
	if err != nil {
		return ParticipantWriteResult{}, err
	}
	if unchanged {
		return commitParticipantWrite(tx, ParticipantWriteResult{Outcome: ParticipantWriteOutcomeUnchanged})
	}

	claimed, err := r.attemptClaim(ctx, tx, stepID, taskID, role, agentID)
	if err != nil {
		return ParticipantWriteResult{}, err
	}
	if claimed != nil {
		return commitParticipantWrite(tx, *claimed)
	}
	// No claim landed — either no claimable auto seat existed, or the
	// selected one was removed, reprovenanced, or decided since
	// findClaimableAutoSeat's read (claimAutoSeat's guard is a defensive
	// backstop: recordStepDecisionTx now shares this transaction's
	// ParticipantRoleSeatLockKey exclusion, so it cannot actually interleave
	// here). Either way, fall through to inserting a fresh seat rather than
	// completing having written nothing.

	inserted, err := r.insertManualParticipant(ctx, tx, stepID, taskID, role, agentID)
	if err != nil {
		return ParticipantWriteResult{}, err
	}
	if !inserted {
		// The natural-key backstop fired: a row already existed at this
		// exact (step, task, role, agent) identity despite the exclusion —
		// report the same outcome probeExistingIdentity would have (design
		// doc "Contention").
		return commitParticipantWrite(tx, ParticipantWriteResult{Outcome: ParticipantWriteOutcomeUnchanged})
	}
	return commitParticipantWrite(tx, ParticipantWriteResult{Outcome: ParticipantWriteOutcomeInserted, StepID: stepID})
}

// lockParticipantRoleSeat acquires the Postgres-only advisory lock both
// automatic casting and manual registration take before mutating a role's
// seat slate for a task. No-op on the embedded dialect, which serializes
// writers for free.
func (r *Repository) lockParticipantRoleSeat(ctx context.Context, tx *sqlx.Tx, taskID, role string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	lockKey := workflowrepo.ParticipantRoleSeatLockKey(taskID, role)
	if _, err := tx.ExecContext(ctx, r.db.Rebind(
		"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))"), lockKey); err != nil {
		return fmt.Errorf("lock role seat identity: %w", err)
	}
	return nil
}

// attemptClaim tries to reassign an unclaimed auto seat to agentID. Returns
// a non-nil result when the claim committed successfully; a nil result
// (with a nil error) means nothing was claimed and the caller should fall
// through to inserting a fresh seat — either no eligible seat existed, or
// claimAutoSeat's conditional UPDATE affected zero rows.
func (r *Repository) attemptClaim(
	ctx context.Context, tx *sqlx.Tx, stepID, taskID, role, agentID string,
) (*ParticipantWriteResult, error) {
	// An unknown agent profile can never displace a cast seat — skip the
	// claim search entirely. Whether it still gets its own seat via the
	// insert fallback is unchanged by this criterion.
	agentExists, err := r.agentProfileExistsTx(ctx, tx, agentID)
	if err != nil {
		return nil, fmt.Errorf("check agent profile exists: %w", err)
	}
	if !agentExists {
		return nil, nil
	}

	claim, err := r.findClaimableAutoSeat(ctx, tx, stepID, taskID, role)
	if err != nil {
		return nil, err
	}
	if claim == nil {
		return nil, nil
	}

	claimed, err := r.claimAutoSeat(ctx, tx, claim.id, agentID)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, nil
	}
	return &ParticipantWriteResult{
		Outcome:                 ParticipantWriteOutcomeClaimed,
		StepID:                  stepID,
		DisplacedAgentProfileID: claim.agentProfileID,
	}, nil
}

// insertManualParticipant writes a fresh "manual" seat for agentID.
// Returns false, with no error, when the insert hit
// participantsNaturalKeyIndexName instead of landing — a row already
// existed at this exact (step, task, role, agent) identity. Every known
// caller reaches this insert already holding ParticipantRoleSeatLockKey, so
// that should never happen; the index remains a defensive backstop against
// a writer somehow outside the exclusion, and firing it is treated as no
// change rather than retried (design doc "Contention").
func (r *Repository) insertManualParticipant(ctx context.Context, tx *sqlx.Tx, stepID, taskID, role, agentID string) (bool, error) {
	id := uuid.New().String()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES (?, ?, ?, ?, ?, 1, 0, ?)
	`), id, stepID, taskID, role, agentID, string(models.ParticipantProvenanceManual)); err != nil {
		if workflowrepo.IsParticipantsNaturalKeyViolation(err) {
			return false, nil
		}
		return false, fmt.Errorf("insert participant: %w", err)
	}
	return true, nil
}

// commitParticipantWrite commits tx and returns result, or a wrapped error
// if the commit itself fails. Centralizes AddTaskParticipant's repeated
// commit-then-return pattern across its four outcome branches.
func commitParticipantWrite(tx *sqlx.Tx, result ParticipantWriteResult) (ParticipantWriteResult, error) {
	if err := tx.Commit(); err != nil {
		return ParticipantWriteResult{}, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

// stepIDForTaskTx is stepIDForTask on the transaction handle, so the read
// happens inside AddTaskParticipant's exclusion instead of racing it
// through the read-only pool.
func (r *Repository) stepIDForTaskTx(ctx context.Context, tx *sqlx.Tx, taskID string) (string, error) {
	var stepID sql.NullString
	err := tx.QueryRowContext(ctx, tx.Rebind(
		`SELECT workflow_step_id FROM tasks WHERE id = ?`,
	), taskID).Scan(&stepID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !stepID.Valid {
		return "", nil
	}
	return stepID.String, nil
}

// probeExistingIdentity checks for a seat already at the exact identity
// (step, task, role, agent profile). When found, the registration writes
// no new seat: reports true (unchanged) unconditionally, promoting the
// seat's provenance to "manual" in place first when it is "auto" and is
// the slate's sole undecided auto seat — the promotion is not a claim, so
// it earns no fourth outcome. When no seat exists at that identity,
// reports false so the caller proceeds to the claim search.
func (r *Repository) probeExistingIdentity(
	ctx context.Context, tx *sqlx.Tx, stepID, taskID, role, agentID string,
) (bool, error) {
	var existingID, existingProvenance string
	err := tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT id, provenance FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = ? AND agent_profile_id = ?
	`), stepID, taskID, role, agentID).Scan(&existingID, &existingProvenance)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("probe existing participant: %w", err)
	}
	if existingProvenance == string(models.ParticipantProvenanceAuto) {
		if err := r.promoteIfSoleUndecided(ctx, tx, stepID, taskID, role, existingID); err != nil {
			return false, err
		}
	}
	return true, nil
}

// promoteIfSoleUndecided flips an auto seat's provenance to "manual" in
// place when it is the slate's sole undecided auto seat. Reuses
// findClaimableAutoSeat's "sole undecided auto seat in this slate" answer:
// if that seat's id matches seatID, this is the one being promoted; any
// other answer (including none) means nothing changes. Guards the write
// itself with the same no-recorded-decision condition claimAutoSeat uses,
// so a decision that commits between the check and this write is not
// silently overridden.
func (r *Repository) promoteIfSoleUndecided(
	ctx context.Context, tx *sqlx.Tx, stepID, taskID, role, seatID string,
) error {
	sole, err := r.findClaimableAutoSeat(ctx, tx, stepID, taskID, role)
	if err != nil {
		return err
	}
	if sole == nil || sole.id != seatID {
		return nil
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE workflow_step_participants
		SET provenance = ?
		WHERE id = ? AND provenance = ?
		  AND NOT EXISTS (SELECT 1 FROM workflow_step_decisions WHERE participant_id = ?)
	`), string(models.ParticipantProvenanceManual), seatID, string(models.ParticipantProvenanceAuto), seatID); err != nil {
		return fmt.Errorf("promote auto-cast participant: %w", err)
	}
	return nil
}

// agentProfileExistsTx reports whether agentID names a live agent_profiles
// row, on the transaction handle so the check reads consistently with the
// rest of AddTaskParticipant's write. Mirrors GetAgentInstance's visibility
// filter.
func (r *Repository) agentProfileExistsTx(ctx context.Context, tx *sqlx.Tx, agentID string) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, tx.Rebind(
		`SELECT 1 FROM agent_profiles WHERE id = ? AND `+agentInstanceFilter,
	), agentID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// claimableAutoSeat is a candidate seat findClaimableAutoSeat may return.
type claimableAutoSeat struct {
	id             string
	agentProfileID string
}

// findClaimableAutoSeat returns the sole "auto"-provenance seat at
// (stepID, taskID, role) that has zero recorded decisions, or nil when no
// such single, undecided auto seat exists. Runs inside AddTaskParticipant's
// transaction so the check is atomic against a concurrent EnsureRoleSeat or
// second manual registration serializing on the same lock.
func (r *Repository) findClaimableAutoSeat(
	ctx context.Context, tx *sqlx.Tx, stepID, taskID, role string,
) (*claimableAutoSeat, error) {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`
		SELECT id, agent_profile_id FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = ? AND provenance = ?
	`), stepID, taskID, role, string(models.ParticipantProvenanceAuto))
	if err != nil {
		return nil, fmt.Errorf("find claimable auto seat: %w", err)
	}
	var candidates []claimableAutoSeat
	for rows.Next() {
		var c claimableAutoSeat
		if err := rows.Scan(&c.id, &c.agentProfileID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan claimable auto seat: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	if len(candidates) != 1 {
		return nil, nil
	}

	var decisionCount int
	if err := tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT COUNT(*) FROM workflow_step_decisions WHERE participant_id = ?
	`), candidates[0].id).Scan(&decisionCount); err != nil {
		return nil, fmt.Errorf("check auto seat decisions: %w", err)
	}
	if decisionCount != 0 {
		return nil, nil
	}
	return &candidates[0], nil
}

// claimAutoSeat reassigns the seat identified by seatID to agentID and
// marks it "manual", conditional on the seat still carrying provenance
// "auto" and no decision having been recorded against it. Returns false
// when the conditional UPDATE affected no row: the seat was removed, its
// provenance changed, or a decision was recorded, since
// findClaimableAutoSeat's read. recordStepDecisionTx now serializes on the
// same ParticipantRoleSeatLockKey exclusion as this transaction, so that
// race window is already closed by the lock for every known caller; both
// conditions remain as a defensive backstop against a writer that somehow
// reaches this table without holding it. The caller then falls through to
// inserting a fresh seat.
func (r *Repository) claimAutoSeat(ctx context.Context, tx *sqlx.Tx, seatID, agentID string) (bool, error) {
	res, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE workflow_step_participants
		SET agent_profile_id = ?, provenance = ?
		WHERE id = ?
		  AND provenance = ?
		  AND NOT EXISTS (SELECT 1 FROM workflow_step_decisions WHERE participant_id = ?)
	`), agentID, string(models.ParticipantProvenanceManual), seatID, string(models.ParticipantProvenanceAuto), seatID)
	if err != nil {
		return false, fmt.Errorf("claim auto-cast participant: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim auto-cast participant rows affected: %w", err)
	}
	return rows > 0, nil
}

// RemoveTaskParticipant deletes a (task, agent, role) row from
// workflow_step_participants. A delete of a non-existent row is not an
// error. Removes per-task rows only — template-level rows (task_id = ”)
// are untouched.
func (r *Repository) RemoveTaskParticipant(ctx context.Context, taskID, agentID, role string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM workflow_step_participants
		WHERE task_id = ? AND agent_profile_id = ? AND role = ?
	`), taskID, agentID, role)
	return err
}

// ListTaskParticipants returns all participants for a task filtered by role.
// Reads from workflow_step_participants under the task's current workflow
// step, merging template-level and per-task rows (per-task wins on
// (role, agent_profile_id) conflicts).
func (r *Repository) ListTaskParticipants(ctx context.Context, taskID, role string) ([]Participant, error) {
	stepID, err := r.stepIDForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if stepID == "" {
		return []Participant{}, nil
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT task_id, agent_profile_id, role, decision_required
		FROM workflow_step_participants
		WHERE step_id = ? AND role = ?
		  AND (task_id = '' OR task_id = ?)
		ORDER BY position ASC, agent_profile_id ASC, id ASC
	`), stepID, role, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Participant{}
	for rows.Next() {
		var p Participant
		var rowTask sql.NullString
		var decisionRequired int
		if err := rows.Scan(
			&rowTask,
			&p.AgentProfileID,
			&p.Role,
			&decisionRequired,
		); err != nil {
			return nil, err
		}
		// Project the canonical task_id into the row (template-level rows
		// have task_id = ''; we surface them as participants of the input task).
		if rowTask.Valid {
			p.TaskID = rowTask.String
		}
		p.DecisionRequired = decisionRequired != 0
		out = append(out, p)
	}
	return projectOfficeParticipants(taskID, mergeOfficeParticipants(out)), rows.Err()
}

// ListAllTaskParticipants returns every participant for a task across both
// roles. Used by the task DTO so reviewers and approvers can be surfaced
// without two separate round trips.
func (r *Repository) ListAllTaskParticipants(ctx context.Context, taskID string) ([]Participant, error) {
	stepID, err := r.stepIDForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if stepID == "" {
		return []Participant{}, nil
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT task_id, agent_profile_id, role, decision_required
		FROM workflow_step_participants
		WHERE step_id = ?
		  AND (task_id = '' OR task_id = ?)
		ORDER BY role ASC, position ASC, agent_profile_id ASC, id ASC
	`), stepID, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Participant{}
	for rows.Next() {
		var p Participant
		var rowTask sql.NullString
		var decisionRequired int
		if err := rows.Scan(
			&rowTask,
			&p.AgentProfileID,
			&p.Role,
			&decisionRequired,
		); err != nil {
			return nil, err
		}
		if rowTask.Valid {
			p.TaskID = rowTask.String
		}
		p.DecisionRequired = decisionRequired != 0
		out = append(out, p)
	}
	return projectOfficeParticipants(taskID, mergeOfficeParticipants(out)), rows.Err()
}

// mergeOfficeParticipants enforces per-task precedence: when a
// template-level row and a per-task row share (role, agent_profile_id),
// the per-task row wins. workflow_step_participants stores both kinds; we
// post-filter here because the SQL query returns the union.
func mergeOfficeParticipants(rows []Participant) []Participant {
	if len(rows) <= 1 {
		return rows
	}
	type key struct{ role, agent string }
	chosen := make(map[key]int, len(rows))
	out := make([]Participant, 0, len(rows))
	for _, p := range rows {
		k := key{p.Role, p.AgentProfileID}
		if idx, ok := chosen[k]; ok {
			if out[idx].TaskID == "" && p.TaskID != "" {
				out[idx] = p
			}
			continue
		}
		chosen[k] = len(out)
		out = append(out, p)
	}
	return out
}

func projectOfficeParticipants(taskID string, rows []Participant) []Participant {
	for i := range rows {
		rows[i].TaskID = taskID
	}
	return rows
}

// fanOutRunReason is the reason config/workflows/office-default.yml's
// queue_run_for_each_participant action stamps on every run it queues.
// Matched literally so CancelDisplacedParticipantRun only ever touches a
// run that step-entry fan-out itself created, not some other queued run
// that happens to share the same agent, task and step.
const fanOutRunReason = "task_assigned"

// displacedRunCancelReason is the cancel_reason stamped on a run
// CancelDisplacedParticipantRun cancels.
const displacedRunCancelReason = "participant_seat_claimed"

// CancelDisplacedParticipantRun cancels the queued or claimed run(s) the
// step-entry fan-out queued for agentProfileID at (taskID, stepID). A claim
// reassigns the seat after that fan-out already read the pre-claim slate,
// so the run addressed to the displaced agent is not redirected by the
// reassignment. Best-effort: the terminal write itself lives in
// CancelRunsWhere, whose queued/claimed guard leaves an already-finished
// run untouched; callers log and continue on error rather than fail the
// registration that already committed.
//
// task_id and workflow_step_id live in the run's payload JSON, not in
// columns, so the selector extracts them — json_extract is
// SQLite-flavoured and the server dialect needs dialect.JSONExtract, which
// is why this selector lives here rather than in the shared runs writer
// (mirrors CancelRunsForTasks in tree_holds.go).
func (r *Repository) CancelDisplacedParticipantRun(ctx context.Context, taskID, stepID, agentProfileID string) (int64, error) {
	if taskID == "" || stepID == "" || agentProfileID == "" {
		return 0, nil
	}
	driver := r.db.DriverName()
	selector := fmt.Sprintf(
		`agent_profile_id = ? AND reason = ? AND %s = ? AND %s = ?`,
		dialect.JSONExtract(driver, "payload", "task_id"),
		dialect.JSONExtract(driver, "payload", "workflow_step_id"),
	)
	return r.CancelRunsWhere(ctx, displacedRunCancelReason, selector,
		agentProfileID, fanOutRunReason, taskID, stepID)
}
