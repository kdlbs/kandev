package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/runs/commentkeys"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

// CreateRunTx creates a new run queue entry using a transaction the caller
// owns. Callers that need to combine the insert with other transactional
// writes (for example the parent-wake reconciler's receipt upsert) use
// this directly; CreateRun wraps it with a private transaction for the
// common single-statement case.
//
// ContinuationScope is decided here, once, before the row exists to be
// coalesced into — never at read or write time downstream. A routine
// wakeup that later coalesces into this run (MarkWakeupRequestCoalesced)
// only ever patches context_snapshot, so every later reader/writer of
// this run's continuation summary reads the value persisted here instead
// of re-deriving it against a snapshot that may have drifted.
func (r *Repository) CreateRunTx(ctx context.Context, tx *sqlx.Tx, req *runmodels.Run) error {
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	ensureRunDefaults(req)
	req.RequestedAt = time.Now().UTC()
	req.ContinuationScope = models.ContinuationScopeForRun(req, req.AgentProfileID)

	_, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			idempotency_key, context_snapshot, capabilities, input_snapshot,
			output_summary, failure_reason, session_id, retry_count, scheduled_retry_at,
			requested_at, error_message, cancel_reason, continuation_scope,
			chain_causation_id, parent_run_id, causation_depth, priority_class,
			human_rooted, routine_id, actor_kind, actor_id, workspace_id,
			wake_wave_key, wake_wave_string, causation_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?)
	`), req.ID, req.AgentProfileID, req.Reason, req.Payload, req.Status,
		req.CoalescedCount, req.IdempotencyKey, req.ContextSnapshot,
		req.Capabilities, req.InputSnapshot, req.OutputSummary, req.FailureReason,
		req.SessionID, req.RetryCount, req.ScheduledRetryAt, req.RequestedAt,
		req.ErrorMessage, req.CancelReason, req.ContinuationScope,
		req.ChainCausationID, req.ParentRunID, req.CausationDepth, req.PriorityClass,
		dialect.BoolToInt(req.HumanRooted), req.RoutineID, string(req.ActorKind), req.ActorID, req.WorkspaceID,
		req.WakeWaveKey, req.WakeWaveString, req.CausationID)
	return err
}

// CreateRun creates a new run queue entry in a transaction owned by this
// method. See CreateRunTx for the field-defaulting / continuation-scope
// derivation this delegates to.
func (r *Repository) CreateRun(ctx context.Context, req *runmodels.Run) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.CreateRunTx(ctx, tx, req); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureRunDefaults(req *runmodels.Run) {
	if req.Payload == "" {
		req.Payload = "{}"
	}
	if req.ContextSnapshot == "" {
		req.ContextSnapshot = "{}"
	}
	if req.Capabilities == "" {
		req.Capabilities = "{}"
	}
	if req.InputSnapshot == "" {
		req.InputSnapshot = "{}"
	}
	if req.ResultJSON == "" {
		req.ResultJSON = "{}"
	}
	if req.Status == "" {
		req.Status = "queued"
	}
}

// UpdateRunRuntimeSnapshot stores the runtime context captured before launch.
func (r *Repository) UpdateRunRuntimeSnapshot(
	ctx context.Context,
	id string,
	capabilities string,
	inputSnapshot string,
	sessionID string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET capabilities = ?, input_snapshot = ?, session_id = ?
		WHERE id = ?
	`), capabilities, inputSnapshot, sessionID, id)
	return err
}

// SetRunSessionID persists the session id a launch produced. A no-op
// when sessionID is empty — the caller counts that as a without-session
// launch rather than clobbering whatever the column already held.
// Guarded to status = 'claimed' so a launch that loses a race against a
// concurrent cancel or terminal write cannot mutate an already-terminal
// row's session id after the fact: without the guard, a terminal-shape
// classification already recorded off the empty session id would drift
// from what the row shows on a later read. Returns whether the row was
// still claimed so the caller can distinguish a real write from a
// stale run id or a lost race.
func (r *Repository) SetRunSessionID(
	ctx context.Context, runID, sessionID string,
) (bool, error) {
	if sessionID == "" {
		return false, nil
	}
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs SET session_id = ? WHERE id = ? AND status = 'claimed'
	`), sessionID, runID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// UpdateRunRuntimeSnapshotCAS is UpdateRunRuntimeSnapshot's compare-and-swap
// sibling: the write only takes effect while the run's current
// capabilities still equal prevCapabilities. Used to decide first-write-
// wins when two processors build runtime context for the same run
// concurrently (docs/specs/office/system-design/
// taskless-coordinator-authority-01.md#first-write-wins-and-how). The
// comparison is a value compare rather than a SQL JSON extraction so it
// stays dialect-neutral across SQLite and Postgres. The bool reports
// whether this call's write took effect.
func (r *Repository) UpdateRunRuntimeSnapshotCAS(
	ctx context.Context,
	id string,
	prevCapabilities string,
	capabilities string,
	inputSnapshot string,
	sessionID string,
) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET capabilities = ?, input_snapshot = ?, session_id = ?
		WHERE id = ? AND COALESCE(capabilities, '') = ?
	`), capabilities, inputSnapshot, sessionID, id, prevCapabilities)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateRunPromptArtifacts persists the assembled prompt the agent
// received and the continuation-summary content prepended at dispatch.
// Called from the scheduler-integration after BuildAgentPrompt completes
// so the run-detail UI can render exactly what the agent saw. Either
// argument may be empty — the columns default to ” and tolerate it.
func (r *Repository) UpdateRunPromptArtifacts(
	ctx context.Context, id, assembledPrompt, summaryInjected string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET assembled_prompt = ?, summary_injected = ?
		WHERE id = ?
	`), assembledPrompt, summaryInjected, id)
	return err
}

// UpdateRunResultJSON stores the structured adapter output captured
// at run completion. Used by the continuation-summary builder as the
// primary input for "Recent actions" / "Recent decisions". Falls back
// to '{}' when raw is empty so the column default invariant holds.
func (r *Repository) UpdateRunResultJSON(ctx context.Context, id, raw string) error {
	if raw == "" {
		raw = "{}"
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET result_json = ?
		WHERE id = ?
	`), raw, id)
	return err
}

// UpdateRunOutputSummary stores the final runtime summary for a run.
func (r *Repository) UpdateRunOutputSummary(ctx context.Context, id, outputSummary, failureReason string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET output_summary = ?, failure_reason = ?
		WHERE id = ?
	`), outputSummary, failureReason, id)
	return err
}

// ListRuns returns run requests filtered by workspace (via agent profile join), ordered by time.
func (r *Repository) ListRuns(ctx context.Context, workspaceID string) ([]*runmodels.Run, error) {
	var reqs []*runmodels.Run
	err := r.ro.SelectContext(ctx, &reqs, r.ro.Rebind(`
		SELECT w.* FROM runs w
		JOIN agent_profiles a ON a.id = w.agent_profile_id
		WHERE a.workspace_id = ?
		ORDER BY w.requested_at DESC
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if reqs == nil {
		reqs = []*runmodels.Run{}
	}
	return reqs, nil
}

// ClaimRun atomically claims the oldest queued run for an agent.
func (r *Repository) ClaimRun(ctx context.Context, agentInstanceID string) (*runmodels.Run, error) {
	now := time.Now().UTC()
	var req runmodels.Run
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'claimed', claimed_at = ?
		WHERE id = (
			SELECT id FROM runs
			WHERE agent_profile_id = ? AND status = 'queued'
			ORDER BY requested_at ASC
			LIMIT 1
		)
		RETURNING *
	`), now, agentInstanceID).StructScan(&req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// FinishRun marks a run as terminal (status) and records its outcome in the
// same statement, so the row can never hold a terminal status with a stale
// outcome from a different transition. outcome is nil for the failed path
// and for callers with no established semantic label (docs/specs/
// task-delivery-ledger/spec.md, "Office run outcome").
// FinishRun writes the terminal status/outcome and returns the row as it
// stands immediately after that write, via the same statement (RETURNING),
// so a caller classifying the transition (office_loop_terminal_total) never
// depends on a separate read succeeding independently of the write that
// persisted it. Guarded to status = 'claimed', the same status every caller
// reaches this from (ClaimNextEligibleRun then processRun/an event
// subscriber): without the guard, a cancel that commits between the
// caller's read and this write would have its 'cancelled' status and
// finished_at overwritten by this transition. Returns (nil, nil) for an
// unknown id or a run no longer claimed (already terminal by another
// writer): zero rows changed, so there is nothing to classify.
func (r *Repository) FinishRun(ctx context.Context, id, status string, outcome *string) (*runmodels.Run, error) {
	now := time.Now().UTC()
	var run runmodels.Run
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		UPDATE runs SET status = ?, outcome = ?, finished_at = ?
		WHERE id = ? AND status = 'claimed'
		RETURNING *
	`), status, outcome, now, id).StructScan(&run)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// GetRunByID returns the run row for a given ID. Returns sql.ErrNoRows when unknown.
func (r *Repository) GetRunByID(ctx context.Context, id string) (*runmodels.Run, error) {
	return getRunByID(ctx, r.ro, id)
}

// GetRunByIDTx is GetRunByID run against a caller-owned transaction, so a
// causing-run lookup participates in the single enqueue transaction
// AC-OFFICE-LAUNCH-SAFETY-003.8 requires instead of racing it on a
// separate reader connection.
func (r *Repository) GetRunByIDTx(ctx context.Context, tx *sqlx.Tx, id string) (*runmodels.Run, error) {
	return getRunByID(ctx, tx, id)
}

func getRunByID(ctx context.Context, exec sqlExecutor, id string) (*runmodels.Run, error) {
	var run runmodels.Run
	err := exec.QueryRowxContext(ctx, exec.Rebind(`
		SELECT * FROM runs WHERE id = ?
	`), id).StructScan(&run)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// GetRunWorkspaceID resolves a run's owning workspace via its agent
// profile, the same join idiom ListRuns uses. A raw join, not
// GetAgentInstance: GetAgentInstance filters deleted_at IS NULL, which
// would make a run under a soft-deleted agent profile permanently
// unresolvable (and so permanently deniable) to its own owner. Returns
// sql.ErrNoRows for an unknown run.
func (r *Repository) GetRunWorkspaceID(ctx context.Context, runID string) (string, error) {
	var workspaceID string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT a.workspace_id FROM runs r
		JOIN agent_profiles a ON a.id = r.agent_profile_id
		WHERE r.id = ?
	`), runID).Scan(&workspaceID)
	if err != nil {
		return "", err
	}
	return workspaceID, nil
}

// GetClaimedRunByID returns a run only while it is still claimed. Lifecycle
// events carry this immutable run identity so a delayed predecessor event
// cannot finish a newer claimed run for the same task and agent.
func (r *Repository) GetClaimedRunByID(ctx context.Context, id string) (*runmodels.Run, error) {
	var run runmodels.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM runs WHERE id = ? AND status = 'claimed'
	`), id).StructScan(&run)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// CommentRunStatus is the slim per-comment run snapshot returned by
// GetRunsByCommentIDs. The backend maps these onto CommentDTO so the
// frontend can render a Queued / Working / Failed badge on the user
// comment that triggered each run.
type CommentRunStatus struct {
	RunID        string
	Status       string
	ErrorMessage string
}

// GetRunsByCommentIDs returns the latest run associated with each comment id.
// The canonical same-task wake joins on idempotency_key =
// "task_comment:<comment_id>"; salted fan-out keys join through the persisted
// payload.comment_id so cross-task comment wakes still surface status.
// Comments without a matching run are simply absent from the map.
// When a comment has multiple matching rows (rare — would require the
// idempotency window to lapse and the comment to re-trigger) the most
// recently requested row wins.
func (r *Repository) GetRunsByCommentIDs(
	ctx context.Context, commentIDs []string,
) (map[string]CommentRunStatus, error) {
	out := map[string]CommentRunStatus{}
	if len(commentIDs) == 0 {
		return out, nil
	}
	args := make([]interface{}, 0)
	keyPlaceholders := make([]string, len(commentIDs))
	commentPlaceholders := make([]string, len(commentIDs))
	wanted := make(map[string]struct{}, len(commentIDs))
	for i, id := range commentIDs {
		args = append(args, commentkeys.TaskComment(id))
		keyPlaceholders[i] = "?"
		wanted[id] = struct{}{}
	}
	for _, id := range commentIDs {
		args = append(args, commentkeys.TaskComment(id))
	}
	args = append(args, commentkeys.TaskCommentPrefix+"%", commentkeys.TaskCommentReason)
	for i, id := range commentIDs {
		args = append(args, id)
		commentPlaceholders[i] = "?"
	}
	commentIDExpr := dialect.JSONExtract(r.ro.DriverName(), "payload", "comment_id")
	query := fmt.Sprintf(`
		SELECT id, idempotency_key, status, error_message, requested_at, payload
		FROM runs
		WHERE idempotency_key IN (%s)
		UNION ALL
		SELECT id, idempotency_key, status, error_message, requested_at, payload
		FROM runs
		WHERE (idempotency_key IS NULL OR idempotency_key NOT IN (%s))
		  AND (idempotency_key LIKE ? OR reason = ?)
		  AND %s IN (%s)
		ORDER BY requested_at DESC
	`,
		strings.Join(keyPlaceholders, ","),
		strings.Join(keyPlaceholders, ","),
		commentIDExpr,
		strings.Join(commentPlaceholders, ","),
	)
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			id, status, errMsg, payload string
			idemKey                     sql.NullString
			requestedAt                 time.Time
		)
		if err := rows.Scan(&id, &idemKey, &status, &errMsg, &requestedAt, &payload); err != nil {
			return nil, err
		}
		commentID := commentIDFromRun(idemKey.String, payload, wanted)
		if commentID == "" {
			continue
		}
		// First write wins — rows are ordered DESC so the first row per
		// comment id is the most recent.
		if _, ok := out[commentID]; ok {
			continue
		}
		out[commentID] = CommentRunStatus{
			RunID:        id,
			Status:       status,
			ErrorMessage: errMsg,
		}
	}
	return out, rows.Err()
}

func commentIDFromRun(idempotencyKey, payload string, wanted map[string]struct{}) string {
	payloadID := commentIDFromPayload(payload, wanted)
	if commentkeys.IsSaltedTaskCommentKey(idempotencyKey) && payloadID != "" {
		return payloadID
	}
	if id := commentkeys.CommentIDFromKey(idempotencyKey); id != "" {
		if _, found := wanted[id]; found {
			return id
		}
	}
	return payloadID
}

func commentIDFromPayload(payload string, wanted map[string]struct{}) string {
	if payload == "" {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return ""
	}
	id, ok := raw["comment_id"].(string)
	if !ok {
		return ""
	}
	if _, found := wanted[id]; !found {
		return ""
	}
	return id
}

// GetClaimedTasklessRunForAgent returns the most recently claimed
// taskless run (payload.task_id is empty / missing) for the given
// agent. Used by the AgentCompleted event subscriber to attribute a
// fresh-session heartbeat run when no task_id is on the event. Returns
// sql.ErrNoRows when no such run exists — the caller should treat
// that as "not a heartbeat completion event".
func (r *Repository) GetClaimedTasklessRunForAgent(
	ctx context.Context, agentProfileID string,
) (*runmodels.Run, error) {
	taskIDExpr := dialect.JSONExtract(r.ro.DriverName(), "payload", "task_id")
	var req runmodels.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(fmt.Sprintf(`
		SELECT * FROM runs
		WHERE agent_profile_id = ?
		  AND status = 'claimed'
		  AND COALESCE(%s, '') = ''
		ORDER BY claimed_at DESC
		LIMIT 1
	`, taskIDExpr)), agentProfileID).StructScan(&req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// GetClaimedRunForAgent returns the agent's sole claimed run, across every
// task (or none). Used to resolve the live causing run for a wake whose
// actor is an agent profile but which carries no explicit CausingRunID or
// task-boundary carrier — a reactivity-pipeline wake, not a workflow-engine
// action inside a known task boundary. Returns sql.ErrNoRows both when the
// agent has no claimed run and when it has more than one (an agent whose
// max_concurrent_sessions ceiling is above 1 can hold several at once):
// with no way to tell which claim actually caused this wake, ordering by
// claimed_at and guessing the newest would misattribute causation, so
// multiple claims fail closed exactly like no claim — the caller then
// resolves the wake as its own root cause, exactly as if this lookup had
// never run.
func (r *Repository) GetClaimedRunForAgent(ctx context.Context, agentProfileID string) (*runmodels.Run, error) {
	var runs []runmodels.Run
	if err := r.ro.SelectContext(ctx, &runs, r.ro.Rebind(`
		SELECT * FROM runs
		WHERE agent_profile_id = ?
		  AND status = 'claimed'
		ORDER BY claimed_at DESC
		LIMIT 2
	`), agentProfileID); err != nil {
		return nil, err
	}
	if len(runs) != 1 {
		return nil, sql.ErrNoRows
	}
	return &runs[0], nil
}

// GetClaimedRunForCausationAttribution returns the agent's claimed run to
// attribute as the cause of a reactivity wake — resolveCausingRunID's sole
// caller. It shares GetClaimedRunForAgent's agent-scoped, cross-task query
// but not its ambiguity behavior: GetClaimedRunForAgent fails closed to
// sql.ErrNoRows on more than one claim, which is correct when the caller
// then treats "no claim" as "no claim". Here, the caller instead treats a
// missing claim as "wake resolves as its own root cause" (CausationDepth
// resets to 0), so failing closed the same way on ambiguity would let a
// wake from an agent already deep in a causation chain launder itself back
// to root and bypass the depth ceiling. With no signal for which of the
// agent's several claims actually caused this wake, this resolves to
// whichever carries the greatest CausationDepth: whichever claim it really
// was, the computed child depth is never lower than it should be. Returns
// sql.ErrNoRows only when the agent holds no claimed run at all.
func (r *Repository) GetClaimedRunForCausationAttribution(ctx context.Context, agentProfileID string) (*runmodels.Run, error) {
	var runs []runmodels.Run
	if err := r.ro.SelectContext(ctx, &runs, r.ro.Rebind(`
		SELECT * FROM runs
		WHERE agent_profile_id = ?
		  AND status = 'claimed'
	`), agentProfileID); err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, sql.ErrNoRows
	}
	deepest := runs[0]
	for _, run := range runs[1:] {
		if run.CausationDepth > deepest.CausationDepth {
			deepest = run
		}
	}
	return &deepest, nil
}

// GetClaimedRunByTaskID returns the claimed run associated with a task payload.
func (r *Repository) GetClaimedRunByTaskID(ctx context.Context, taskID string) (*runmodels.Run, error) {
	taskIDExpr := dialect.JSONExtract(r.ro.DriverName(), "payload", "task_id")
	var req runmodels.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(fmt.Sprintf(`
		SELECT * FROM runs
		WHERE status = 'claimed'
		  AND %s = ?
		ORDER BY claimed_at DESC
		LIMIT 1
	`, taskIDExpr)), taskID).StructScan(&req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// GetClaimedRunByTaskAndAgent returns the claimed run for a task that
// belongs to a specific agent. Unlike GetClaimedRunByTaskID, this is scoped
// to the agent as well as the task: ClaimNextEligibleRun's busy-lock is
// per-agent, not per-task, so two different agents can each have a claimed
// run on the same task at once. Callers that already know which agent's
// lifecycle event they are handling (AgentCompleted/AgentFailed) must use
// this instead of the unscoped lookup, which prefers whichever row was
// claimed most recently regardless of which agent actually sent the event
// (Review round 4, BLOCKING FINDING 2).
func (r *Repository) GetClaimedRunByTaskAndAgent(
	ctx context.Context, taskID, agentProfileID string,
) (*runmodels.Run, error) {
	taskIDExpr := dialect.JSONExtract(r.ro.DriverName(), "payload", "task_id")
	var req runmodels.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(fmt.Sprintf(`
		SELECT * FROM runs
		WHERE status = 'claimed'
		  AND agent_profile_id = ?
		  AND %s = ?
		ORDER BY claimed_at DESC
		LIMIT 1
	`, taskIDExpr)), agentProfileID, taskID).StructScan(&req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// CheckIdempotencyKey returns true if the key already exists within the window.
func (r *Repository) CheckIdempotencyKey(ctx context.Context, key string, windowHours int) (bool, error) {
	return checkIdempotencyKey(ctx, r.ro, key, windowHours)
}

// CheckIdempotencyKeyTx is CheckIdempotencyKey run against a caller-owned
// transaction, so it participates in the single enqueue transaction
// AC-OFFICE-LAUNCH-SAFETY-003.8 requires instead of racing it on a
// separate reader connection.
func (r *Repository) CheckIdempotencyKeyTx(ctx context.Context, tx *sqlx.Tx, key string, windowHours int) (bool, error) {
	return checkIdempotencyKey(ctx, tx, key, windowHours)
}

func checkIdempotencyKey(ctx context.Context, exec sqlExecutor, key string, windowHours int) (bool, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	var count int
	err := exec.QueryRowxContext(ctx, exec.Rebind(`
		SELECT COUNT(*) FROM runs
		WHERE idempotency_key = ? AND requested_at > ?
	`), key, cutoff).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CoalesceRun tries to merge with an existing queued run for the same
// agent and reason within the given window. Returns true if coalesced.
func (r *Repository) CoalesceRun(
	ctx context.Context, agentInstanceID, reason string, windowSecs int, payload string,
) (bool, error) {
	return coalesceRun(ctx, r.db, r.db.DriverName(), agentInstanceID, reason, windowSecs, payload)
}

// CoalesceRunTx is CoalesceRun run against a caller-owned transaction, so
// a request that coalesces participates in the same enqueue transaction
// as the idempotency check and causation resolution rather than racing
// them on a separate statement.
func (r *Repository) CoalesceRunTx(
	ctx context.Context, tx *sqlx.Tx, agentInstanceID, reason string, windowSecs int, payload string,
) (bool, error) {
	return coalesceRun(ctx, tx, tx.DriverName(), agentInstanceID, reason, windowSecs, payload)
}

func coalesceRun(
	ctx context.Context, exec interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
		Rebind(query string) string
	}, driverName, agentInstanceID, reason string, windowSecs int, payload string,
) (bool, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(windowSecs) * time.Second)
	taskID, invalidTaskID := taskIDFromPayload(payload)
	args := []interface{}{payload, agentInstanceID, reason, cutoff, commentkeys.TaskCommentPrefix + "%"}
	// A payload's task_id identifies which launch it belongs to: merging
	// across two different task_ids (present or absent) would replace one
	// launch's payload with an unrelated one and silently drop it. The
	// check is symmetric so both directions are covered. json_extract (and
	// its Postgres ->> equivalent) yields NULL for both an absent key and
	// an explicit JSON null, and '' for a present-but-empty string, so the
	// taskless branch coalesces all three shapes together via COALESCE.
	jsonExtract := dialect.JSONExtract(driverName, "payload", "task_id")
	var taskPredicate string
	switch {
	case invalidTaskID:
		// task_id is present but not a string (e.g. a number): it names a
		// task we can't compare textually, so it must not be treated as
		// taskless and must not match any queued row at all.
		taskPredicate = " AND 1 = 0"
	case taskID != "":
		// The stored value must itself be a JSON string, not merely equal
		// as text: Postgres's ->> converts a stored JSON number (or
		// object) to text before the comparison, so an untyped payload
		// with e.g. {"task_id":42} could otherwise textually match an
		// incoming {"task_id":"42"} and get overwritten.
		taskPredicate = fmt.Sprintf(" AND %s AND %s = ?",
			dialect.JSONTypeIsString(driverName, "payload", "task_id"), jsonExtract)
		args = append(args, taskID)
	default:
		taskPredicate = fmt.Sprintf(" AND COALESCE(%s, '') = ''", jsonExtract)
	}
	query := fmt.Sprintf(`
		UPDATE runs
		SET coalesced_count = coalesced_count + 1, payload = ?
		WHERE id = (
			SELECT id FROM runs
			WHERE agent_profile_id = ? AND reason = ? AND status = 'queued'
			  AND requested_at > ?
			  AND (idempotency_key IS NULL OR idempotency_key NOT LIKE ?)
			  AND wake_wave_key = ''
			%s
			ORDER BY requested_at DESC
			LIMIT 1
		)
	`, taskPredicate)
	res, err := exec.ExecContext(ctx, exec.Rebind(query), args...)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// taskIDFromPayload extracts payload.task_id for CoalesceRun's task-scoping
// predicate. invalidTaskID is true only when the key is present with a
// non-string value: that shape names some task_id, just not one comparable
// as a string, so it must be kept out of the taskless bucket (an absent key,
// a JSON null, or a present empty string all return "", invalidTaskID=false).
func taskIDFromPayload(payload string) (taskID string, invalidTaskID bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return "", false
	}
	v, present := raw["task_id"]
	if !present || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", true
	}
	return s, false
}

// CountAgentInitiatedAssignmentWakes counts runs for taskID with the given
// reason whose stored payload carries actor_type "agent" and whose
// requested_at falls in the half-open interval (windowStart,
// evaluationInstant], exclusive of its old edge and inclusive of its new
// one. The inclusive upper bound keeps a future-dated row (clock skew
// across writers under Postgres) from counting indefinitely instead of
// aging out with the window it actually belongs to. Deliberately carries
// no status filter: "admitted" is defined as "a runs row was inserted",
// so a run that has since completed still holds its allowance slot until
// the window passes. taskID must already be known non-empty; callers
// with an unattributable task must not reach this method.
func (r *Repository) CountAgentInitiatedAssignmentWakes(
	ctx context.Context, taskID, reason string, windowStart, evaluationInstant time.Time,
) (int, error) {
	driver := r.ro.DriverName()
	taskExtract := dialect.JSONExtract(driver, "payload", "task_id")
	actorExtract := dialect.JSONExtract(driver, "payload", "actor_type")
	// Guard the stored task_id's JSON type the same way CoalesceRun does:
	// Postgres's ->> converts a stored JSON number to text before
	// comparison, so an untyped payload with e.g. {"task_id":42} could
	// otherwise textually match taskID and join another task's allowance.
	query := fmt.Sprintf(`
		SELECT COUNT(*) FROM runs
		WHERE reason = ?
		  AND %s
		  AND %s = ?
		  AND %s = ?
		  AND requested_at > ?
		  AND requested_at <= ?
	`, dialect.JSONTypeIsString(driver, "payload", "task_id"), taskExtract, actorExtract)
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query),
		reason, taskID, "agent", windowStart, evaluationInstant).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ScheduleRetry resets a run to queued with an incremented retry count
// and a scheduled retry time. Re-stamps priority_class to recovery
// unless it is already human (AC-OFFICE-BACKPRESSURE-001.7), so a
// retried run is not stuck behind the class of work it kept losing to.
// session_id is cleared: every caller either
// runs pre-launch (the run never had one) or post-start (the session it
// had belongs to the failed attempt), and a relaunch must mint its
// runtime credentials against the session the new attempt actually gets,
// not a stale one from a previous attempt. error_message is cleared for
// the same reason: a requeued run is not yet failed, so it must not carry
// the previous attempt's error into a later successful finish.
func (r *Repository) ScheduleRetry(ctx context.Context, runID string, retryAt time.Time, retryCount int) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'queued', retry_count = ?, scheduled_retry_at = ?,
		    claimed_at = NULL, finished_at = NULL, session_id = '', error_message = '',
		    priority_class = CASE WHEN priority_class = ? THEN priority_class ELSE ? END
		WHERE id = ?
	`), retryCount, retryAt, runmodels.PriorityClassHuman, runmodels.PriorityClassRecovery, runID)
	return err
}

// ScheduleRetryIfClaimed behaves like ScheduleRetry but only when the run
// is still status='claimed', mirroring the same guard MarkRunFailed uses.
// A caller that wants to requeue a run it has not itself moved off
// 'claimed' must not resurrect a row a concurrent writer already
// terminalized (a task-tree cancel, workspace pause, or participant
// eviction) out from under it — every one of those writers targets
// exactly the 'claimed' status this guard checks. wrote=false means a
// concurrent writer already changed the row's status; the caller must
// not retry again or treat the run as requeued.
func (r *Repository) ScheduleRetryIfClaimed(ctx context.Context, runID string, retryAt time.Time, retryCount int) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'queued', retry_count = ?, scheduled_retry_at = ?,
		    claimed_at = NULL, finished_at = NULL, session_id = '', error_message = '',
		    priority_class = CASE WHEN priority_class = ? THEN priority_class ELSE ? END
		WHERE id = ? AND status = 'claimed'
	`), retryCount, retryAt, runmodels.PriorityClassHuman, runmodels.PriorityClassRecovery, runID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CleanExpired deletes finished/failed runs older than the given time.
func (r *Repository) CleanExpired(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM runs
		WHERE status IN ('finished', 'failed') AND finished_at < ?
	`), olderThan)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RecoverStale resets claimed runs older than the given time back to
// queued. Re-stamps priority_class to recovery unless it is already
// human (AC-OFFICE-BACKPRESSURE-001.7), the same rule ScheduleRetry
// applies, so a run stuck long enough to be swept is not also stuck at
// its original, possibly lower, claim priority.
func (r *Repository) RecoverStale(ctx context.Context, claimedOlderThan time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'queued', claimed_at = NULL,
		    priority_class = CASE WHEN priority_class = ? THEN priority_class ELSE ? END
		WHERE status = 'claimed' AND claimed_at < ?
	`), runmodels.PriorityClassHuman, runmodels.PriorityClassRecovery, claimedOlderThan)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListPendingRunsForTask returns queued runs in retry state for the given task.
func (r *Repository) ListPendingRunsForTask(ctx context.Context, taskID string) ([]*runmodels.Run, error) {
	var reqs []*runmodels.Run
	err := r.ro.SelectContext(ctx, &reqs, r.ro.Rebind(`
		SELECT * FROM runs
		WHERE status = 'queued'
		  AND scheduled_retry_at IS NOT NULL
		  AND json_extract(payload, '$.task_id') = ?
	`), taskID)
	if err != nil {
		return nil, err
	}
	if reqs == nil {
		reqs = []*runmodels.Run{}
	}
	return reqs, nil
}

// FindInflightRunForAgent returns the most-recent in-flight run for
// the given agent. "In-flight" means status='queued' (with or without a
// scheduled retry) or status='claimed'. The dispatcher uses this for
// claim-time coalescing — when a wakeup-request lands and an in-flight
// run already exists for the agent, the new request is merged into the
// existing run rather than creating a fresh one.
//
// Returns sql.ErrNoRows when no such run exists.
func (r *Repository) FindInflightRunForAgent(
	ctx context.Context, agentProfileID string,
) (*runmodels.Run, error) {
	var run runmodels.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM runs
		WHERE agent_profile_id = ?
		  AND status IN ('queued', 'claimed')
		ORDER BY requested_at DESC
		LIMIT 1
	`), agentProfileID).StructScan(&run)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// ListRunsForAgentPaged returns runs for an agent ordered by
// (requested_at DESC, id DESC), starting strictly after the given
// cursor. Pass cursor.IsZero() == true to fetch the first page.
// Returns at most `limit` rows; tie-break on id keeps adjacent rows
// with identical requested_at strictly ordered. The handler is
// expected to derive next_cursor from the last row's requested_at.
func (r *Repository) ListRunsForAgentPaged(
	ctx context.Context, agentInstanceID string, cursor time.Time, cursorID string, limit int,
) ([]*runmodels.Run, error) {
	if limit <= 0 {
		limit = 25
	}
	var reqs []*runmodels.Run
	if cursor.IsZero() {
		err := r.ro.SelectContext(ctx, &reqs, r.ro.Rebind(`
			SELECT * FROM runs
			WHERE agent_profile_id = ?
			ORDER BY requested_at DESC, id DESC
			LIMIT ?
		`), agentInstanceID, limit)
		if err != nil {
			return nil, err
		}
	} else {
		// Strictly after the cursor (requested_at, id) DESC: a row is
		// "after" when its requested_at < cursor, OR requested_at ==
		// cursor AND id < cursorID.
		err := r.ro.SelectContext(ctx, &reqs, r.ro.Rebind(`
			SELECT * FROM runs
			WHERE agent_profile_id = ?
			  AND (requested_at < ? OR (requested_at = ? AND id < ?))
			ORDER BY requested_at DESC, id DESC
			LIMIT ?
		`), agentInstanceID, cursor, cursor, cursorID, limit)
		if err != nil {
			return nil, err
		}
	}
	if reqs == nil {
		reqs = []*runmodels.Run{}
	}
	return reqs, nil
}

// RunCostRollup carries the per-run aggregated token + cost numbers
// returned by GetRunWithCosts. Cost rows are joined to the run via
// the run payload's task_id; if the run has no cost rows yet the
// fields are zero. CostSubcents stores hundredths of a cent (UI divides
// by 10000).
type RunCostRollup struct {
	InputTokens  int64 `db:"input_tokens" json:"input_tokens"`
	OutputTokens int64 `db:"output_tokens" json:"output_tokens"`
	CachedTokens int64 `db:"cached_tokens" json:"cached_tokens"`
	CostSubcents int64 `db:"cost_subcents" json:"cost_subcents"`
}

// GetRunWithCosts returns the run row plus a token + cost rollup
// computed by joining office_cost_events on the task_id stored in
// the run payload (json_extract). Returns sql.ErrNoRows when the run
// id is unknown. The rollup may be all-zero when the run hasn't
// produced cost events yet.
func (r *Repository) GetRunWithCosts(
	ctx context.Context, runID string,
) (*runmodels.Run, *RunCostRollup, error) {
	var run runmodels.Run
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM runs WHERE id = ?
	`), runID).StructScan(&run)
	if err != nil {
		return nil, nil, err
	}
	var rollup RunCostRollup
	err = r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT
			COALESCE(SUM(tokens_in), 0)        AS input_tokens,
			COALESCE(SUM(tokens_out), 0)       AS output_tokens,
			COALESCE(SUM(tokens_cached_in), 0) AS cached_tokens,
			COALESCE(SUM(cost_subcents), 0)    AS cost_subcents
		FROM office_cost_events
		WHERE (task_id != '' AND task_id = COALESCE(json_extract(?, '$.task_id'), ''))
          OR (task_id = '' AND session_id IN (SELECT id FROM office_run_sessions WHERE run_id = ?))
	`), run.Payload, run.ID).StructScan(&rollup)
	if err != nil {
		return &run, &RunCostRollup{}, nil
	}
	return &run, &rollup, nil
}

// SetRunRequestedAtForTest backfills requested_at for a seeded run.
// Test-only: bypasses the normal CreateRun stamping so E2E specs can
// build deterministic ordered pages without sleeping between writes.
func (r *Repository) SetRunRequestedAtForTest(
	ctx context.Context, runID string, ts time.Time,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs SET requested_at = ? WHERE id = ?
	`), ts, runID)
	return err
}

// SetRunStatusForTest forces the status + timing fields for a seeded
// run. Test-only: lets the E2E harness land non-queued rows
// (claimed/finished/failed/cancelled) without going through the
// production state machine. A "finished" status defaults outcome to
// "processed", mirroring the ordinary production call site
// (event_subscribers.go's agent-completed path) that "finished" almost
// always represents — the same default already applied by
// office/dashboard's seedSummaryRun test helper (docs/specs/
// task-delivery-ledger/spec.md, "Office run outcome"). Every other
// status writes a NULL outcome, matching FailRun and the dormant
// SchedulerService paths. Existing callers that only cared about
// status keep reading as succeeded under the outcome-aware bucketing
// with no call-site changes.
func (r *Repository) SetRunStatusForTest(
	ctx context.Context, runID, status string,
	claimedAt, finishedAt *time.Time,
) error {
	var outcome interface{}
	if status == "finished" {
		outcome = "processed"
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = ?, outcome = ?, claimed_at = ?, finished_at = ?
		WHERE id = ?
	`), status, outcome, claimedAt, finishedAt, runID)
	return err
}

// SetRunErrorMessageForTest forces the error_message field on a run row.
// Test-only: lets the harness simulate a failed run carrying the same
// error string a real agent_failed event would have produced.
func (r *Repository) SetRunErrorMessageForTest(
	ctx context.Context, runID, errMsg string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs SET error_message = ? WHERE id = ?
	`), errMsg, runID)
	return err
}
