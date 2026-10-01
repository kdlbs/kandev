package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// ErrNotFound is returned when a coordinator, proposal or stall row does not
// exist, or exists but is scoped to a different workspace or coordinator.
var ErrNotFound = errors.New("coordinator: not found")

// Store provides SQLite/PostgreSQL persistence for coordinators, their
// proposals and stall records. See docs/specs/coordinator/system-design/
// {coordinators,proposals,needs-you}.md.
type Store struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader

	// now is the injectable clock. Overridden only by tests (decision 7's
	// deterministic-order test needs a strictly increasing fake clock).
	now func() time.Time

	// phase3 reports whether phase 3 is effective; improvement proposals are
	// fenced out of every fenced read while it is not. Set by the Service that
	// owns the store, nil (phase 3 off) on a bare store.
	phase3 func() bool

	// afterLock is a test-only hook invoked once inside PatchCoordinator,
	// immediately after the per-coordinator write lock is acquired (right
	// after SQLite's BEGIN IMMEDIATE, right after PostgreSQL's SELECT ...
	// FOR UPDATE) and before the merge/validate/update steps. nil in
	// production; only tests in this package set it.
	afterLock func(ctx context.Context)

	// afterUnattendedLookup is a test-only hook run between RecordUnattendedDenial's
	// turn lookup and its write transaction. nil in production.
	afterUnattendedLookup func()

	// beforeCoordinatorRowDelete is a test-only hook invoked by
	// DeleteCoordinator and DeleteWorkspaceState after the coordinator row
	// lock is held and before the coordinator rows are deleted. nil in
	// production.
	beforeCoordinatorRowDelete func()
}

// NewStore creates the coordinator store and initializes its schema.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{
		db:  writer,
		ro:  reader,
		now: func() time.Time { return time.Now().UTC() },
	}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("coordinator schema init: %w", err)
	}
	return s, nil
}

// createTablesSQL defines all three tables up front: no later work order adds
// a migration (docs/plans/workspace-coordinator/task-01-shared-interface.md).
const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS coordinators (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		agent_profile_id TEXT NOT NULL,
		executor_profile_id TEXT NOT NULL,
		context TEXT NOT NULL DEFAULT '',
		conversation_task_id TEXT,
		config_revision INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_coordinators_workspace_created ON coordinators(workspace_id, created_at, id);

	CREATE TABLE IF NOT EXISTS coordinator_proposals (
		id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		status TEXT NOT NULL,
		spec_json TEXT NOT NULL,
		final_spec_json TEXT,
		claimed_at DATETIME,
		claim_token TEXT,
		task_id TEXT,
		error TEXT,
		reject_reason TEXT,
		decided_by TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_coordinator_proposals_coordinator ON coordinator_proposals(coordinator_id, status, created_at, id);

	CREATE TABLE IF NOT EXISTS coordinator_stalls (
		task_id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		stalled_for_ms INTEGER NOT NULL,
		last_event_at DATETIME NOT NULL,
		detected_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_coordinator_stalls_workspace ON coordinator_stalls(workspace_id);
`

func (s *Store) initSchema() error {
	rendered := dialect.MustRenderSchema(s.db.DriverName(), createTablesSQL)
	if _, err := s.db.Exec(rendered); err != nil {
		return err
	}
	migrate := db.NewRequiredMigrateLogger(s.db, nil)
	if err := migrate.Apply("coordinators.config_revision",
		`ALTER TABLE coordinators ADD COLUMN config_revision INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("required coordinator migration: %w", err)
	}
	if err := s.migratePhase2(migrate); err != nil {
		return err
	}
	if err := s.migrateTaskPair(migrate); err != nil {
		return err
	}
	if err := migrate.Err(); err != nil {
		return fmt.Errorf("required coordinator migration: %w", err)
	}
	return nil
}

// coordinatorRow is the DB scan target for coordinators.
type coordinatorRow struct {
	ID                  string         `db:"id"`
	WorkspaceID         string         `db:"workspace_id"`
	Name                string         `db:"name"`
	AgentProfileID      string         `db:"agent_profile_id"`
	ExecutorProfileID   string         `db:"executor_profile_id"`
	Context             string         `db:"context"`
	ConversationTaskID  sql.NullString `db:"conversation_task_id"`
	ConfigRevision      int64          `db:"config_revision"`
	CreatedAt           time.Time      `db:"created_at"`
	UpdatedAt           time.Time      `db:"updated_at"`
	PolicyJSON          sql.NullString `db:"policy_json"`
	PolicyRevision      int            `db:"policy_revision"`
	WatchScope          string         `db:"watch_scope"`
	AutonomyEnabled     bool           `db:"autonomy_enabled"`
	CostCeilingSubcents sql.NullInt64  `db:"cost_ceiling_subcents"`
	PausedAt            sql.NullTime   `db:"paused_at"`
	PausedBy            sql.NullString `db:"paused_by"`

	TaskAgentProfileID    string `db:"task_agent_profile_id"`
	TaskExecutorProfileID string `db:"task_executor_profile_id"`
}

func (r *coordinatorRow) toCoordinator() *Coordinator {
	c := &Coordinator{
		ID:                r.ID,
		WorkspaceID:       r.WorkspaceID,
		Name:              r.Name,
		AgentProfileID:    r.AgentProfileID,
		ExecutorProfileID: r.ExecutorProfileID,
		Context:           r.Context,
		ConfigRevision:    r.ConfigRevision,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		PolicyRevision:    r.PolicyRevision,
		WatchScope:        r.WatchScope,
		AutonomyEnabled:   r.AutonomyEnabled,

		TaskAgentProfileID:    r.TaskAgentProfileID,
		TaskExecutorProfileID: r.TaskExecutorProfileID,
	}
	if r.CostCeilingSubcents.Valid {
		v := r.CostCeilingSubcents.Int64
		c.CostCeilingSubcents = &v
	}
	if r.PausedAt.Valid {
		at := r.PausedAt.Time.UTC()
		c.PausedAt = &at
		c.PausedBy = r.PausedBy.String
	}
	if r.PolicyJSON.Valid {
		raw := r.PolicyJSON.String
		c.PolicyJSON = &raw
	}
	if r.ConversationTaskID.Valid {
		id := r.ConversationTaskID.String
		c.ConversationTaskID = &id
	}
	return c
}

const coordinatorColumns = `id, workspace_id, name, agent_profile_id, executor_profile_id, context, conversation_task_id, config_revision, created_at, updated_at, policy_json, policy_revision, watch_scope, autonomy_enabled, cost_ceiling_subcents, paused_at, paused_by, task_agent_profile_id, task_executor_profile_id`

// insertCoordinatorColumns lists the columns CreateCoordinator writes; the policy columns
// keep their defaults.
const insertCoordinatorColumns = `id, workspace_id, name, agent_profile_id, executor_profile_id, context, conversation_task_id, config_revision, created_at, updated_at, task_agent_profile_id, task_executor_profile_id`

// CreateCoordinator inserts a new coordinator, assigning an id and timestamps
// when unset. config_revision always starts at 0.
func (s *Store) CreateCoordinator(ctx context.Context, c *Coordinator) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	now := s.now()
	c.CreatedAt = now
	c.UpdatedAt = now
	c.ConfigRevision = 0
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO coordinators (`+insertCoordinatorColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		c.ID, c.WorkspaceID, c.Name, c.AgentProfileID, c.ExecutorProfileID, c.Context,
		nullableString(c.ConversationTaskID), c.ConfigRevision, c.CreatedAt, c.UpdatedAt,
		c.TaskAgentProfileID, c.TaskExecutorProfileID)
	if err != nil {
		return fmt.Errorf("insert coordinator: %w", err)
	}
	return nil
}

// GetCoordinator returns the coordinator with the given id in the given
// workspace. A coordinator whose workspace_id differs from workspaceID is
// treated as absent, per coordinators.md#routes.
func (s *Store) GetCoordinator(ctx context.Context, workspaceID, id string) (*Coordinator, error) {
	var row coordinatorRow
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`
		SELECT `+coordinatorColumns+` FROM coordinators WHERE id = ? AND workspace_id = ?`),
		id, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get coordinator: %w", err)
	}
	return row.toCoordinator(), nil
}

// GetCoordinatorByID returns the coordinator with the given id, with no
// workspace scope check. Used only where the caller already trusts the id
// (the mcp/scope resolver and the executor's fail-closed checks resolve the
// id from CoordinatorForConversationTask first).
func (s *Store) GetCoordinatorByID(ctx context.Context, id string) (*Coordinator, error) {
	var row coordinatorRow
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`
		SELECT `+coordinatorColumns+` FROM coordinators WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get coordinator by id: %w", err)
	}
	return row.toCoordinator(), nil
}

// CoordinatorForConversationTask returns the id of the coordinator whose
// current conversation_task_id equals taskID
// (docs/specs/coordinator/system-design/copilot.md#principal-and-mode). ok is
// false when no coordinator matches: an orphaned or archived conversation
// task, or a task id that never belonged to any coordinator's conversation
// task. Needs no task row of its own.
func (s *Store) CoordinatorForConversationTask(ctx context.Context, taskID string) (string, bool, error) {
	var id string
	err := s.ro.GetContext(ctx, &id, s.ro.Rebind(`
		SELECT id FROM coordinators WHERE conversation_task_id = ?`), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("coordinator for conversation task: %w", err)
	}
	return id, true, nil
}

// SetConversationTaskID implements copilot.md#conversation-task step 4's
// commit: a plain compare-and-swap UPDATE, atomic on its own, needing no
// separate lock. It sets conversation_task_id to newTaskID only when the
// column currently holds staleTaskID (the value the caller read in step 2)
// and config_revision still equals expectedConfigRevision (the value read in
// step 1), including the case where both task ids are "" and the column is
// NULL. ok is true when exactly one row was updated; false means a
// concurrent write already changed the column, the revision, or both, and
// the caller must resolve the race.
func (s *Store) SetConversationTaskID(ctx context.Context, coordinatorID, newTaskID, staleTaskID string, expectedConfigRevision int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinators SET conversation_task_id = ?
		WHERE id = ? AND config_revision = ? AND ((? = '' AND conversation_task_id IS NULL) OR conversation_task_id = ?)`),
		newTaskID, coordinatorID, expectedConfigRevision, staleTaskID, staleTaskID)
	if err != nil {
		return false, fmt.Errorf("set conversation task id: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("set conversation task id rows affected: %w", err)
	}
	return rows == 1, nil
}

// ListCoordinators returns every coordinator of a workspace ordered by
// created_at then id, never nil.
func (s *Store) ListCoordinators(ctx context.Context, workspaceID string) ([]*Coordinator, error) {
	var rows []coordinatorRow
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`
		SELECT `+coordinatorColumns+` FROM coordinators WHERE workspace_id = ? ORDER BY created_at, id`),
		workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list coordinators: %w", err)
	}
	result := make([]*Coordinator, len(rows))
	for i := range rows {
		result[i] = rows[i].toCoordinator()
	}
	return result, nil
}

// coordinatorOwnedTables lists the per-coordinator tables in deletion order.
var coordinatorOwnedTables = []string{
	"coordinator_watches", "coordinator_activity", "coordinator_standing_orders",
	"coordinator_goals", "coordinator_proposals",
}

// DeleteCoordinator deletes the coordinator's owned rows and the coordinator
// row in one transaction (coordinators.md#routes). Conversation-task deletion
// is added by a later work order. ErrNotFound when no row matched.
func (s *Store) DeleteCoordinator(ctx context.Context, workspaceID, id string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete coordinator: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := lockedCoordinatorRow(
		ctx, tx, tx.Rebind, workspaceID, id, dialect.IsPostgres(s.db.DriverName()),
	); err != nil {
		return err
	}
	if err := deleteCoordinatorPhase3Rows(ctx, tx, "= ?", id); err != nil {
		return err
	}
	for _, table := range coordinatorOwnedTables {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM `+table+` WHERE coordinator_id = ? AND workspace_id = ?`),
			id, workspaceID); err != nil {
			return fmt.Errorf("delete coordinator %s: %w", table, err)
		}
	}
	if s.beforeCoordinatorRowDelete != nil {
		s.beforeCoordinatorRowDelete()
	}
	res, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM coordinators WHERE id = ? AND workspace_id = ?`), id, workspaceID)
	if err != nil {
		return fmt.Errorf("delete coordinator: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete coordinator: %w", err)
	}
	return nil
}

// coordinatorExec is the minimal transaction-bound executor used by the PATCH
// body: both *sql.Conn (SQLite BEGIN IMMEDIATE path) and *sqlx.Tx (PostgreSQL
// SELECT...FOR UPDATE path) satisfy it, so every PATCH statement runs on the
// same transaction and never on the store's reader handle.
type coordinatorExec interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// CoordinatorPatch carries the fields a PATCH may change. A nil field is left
// unchanged (docs/specs/coordinator/system-design/coordinators.md#routes,
// Build decision 7).
type CoordinatorPatch struct {
	Name              *string
	AgentProfileID    *string
	ExecutorProfileID *string
	Context           *string

	// TaskAgentProfileID and TaskExecutorProfileID are the Agent for created
	// tasks pair; changing them never touches the conversation.
	TaskAgentProfileID    *string
	TaskExecutorProfileID *string

	// AutonomyEnabled is the phase 3 autonomy switch; nil leaves it unchanged.
	AutonomyEnabled *bool
	// CeilingSet marks CostCeilingSubcents as sent: a nil value with CeilingSet
	// clears the ceiling, and CeilingSet false leaves it unchanged.
	CeilingSet          bool
	CostCeilingSubcents *int64
}

// PatchResult is the committed outcome of a coordinator PATCH.
type PatchResult struct {
	Coordinator *Coordinator
	// ClearedConversationTaskID is the previous conversation task id when the
	// PATCH cleared it, nil otherwise.
	ClearedConversationTaskID *string
	// AutonomyChanged is true when autonomy_enabled or cost_ceiling_subcents
	// differs from the row the PATCH read.
	AutonomyChanged bool
	// TaskPairChanged is true when either task pair value differs from the
	// row the PATCH read.
	TaskPairChanged bool
}

// PatchValidator is invoked once inside the PATCH transaction with the
// coordinator merged from the sent fields, before it is persisted (decision
// 7). It may perform its own reads, through other stores' reader pools, but
// never through the coordinator store's writer pool the lock holds. A
// returned error aborts the PATCH without writing.
type PatchValidator func(ctx context.Context, merged, stored *Coordinator) error

// PatchCoordinator applies patch to the coordinator with the given id, scoped
// to workspaceID, under the per-coordinator write lock (decision 7): SQLite
// BEGIN IMMEDIATE (the single writer lock) or PostgreSQL SELECT ... FOR
// UPDATE. Returns ErrNotFound if no row matches. If validate is non-nil it
// runs against the merged row before the write; a returned error aborts the
// PATCH and is returned unwrapped. If the merged context or either profile id
// differs from the row read under the lock, conversation_task_id is cleared;
// its previous value is returned only when it was non-nil.
func (s *Store) PatchCoordinator(ctx context.Context, workspaceID, id string, patch CoordinatorPatch, validate PatchValidator) (*Coordinator, *string, error) {
	res, err := s.PatchCoordinatorResult(ctx, workspaceID, id, patch, validate)
	if err != nil {
		return nil, nil, err
	}
	return res.Coordinator, res.ClearedConversationTaskID, nil
}

// patchGuard adds steps to the PATCH transaction: pre runs after the row is
// read under the lock and before validation (and may complete the patch from
// what it read), pinContext (when set) makes the
// write conditional on the stored context, and post runs in the same
// transaction after the write.
type patchGuard struct {
	pre           func(ctx context.Context, exec coordinatorExec, row *coordinatorRow, patch *CoordinatorPatch) error
	pinContext    *string
	onPinMismatch error
	post          func(ctx context.Context, exec coordinatorExec) error
}

// PatchCoordinatorResult is PatchCoordinator returning the full PatchResult,
// including whether the autonomy settings changed.
func (s *Store) PatchCoordinatorResult(ctx context.Context, workspaceID, id string, patch CoordinatorPatch, validate PatchValidator) (*PatchResult, error) {
	return s.patchCoordinatorGuarded(ctx, workspaceID, id, patch, validate, nil)
}

func (s *Store) patchCoordinatorGuarded(ctx context.Context, workspaceID, id string, patch CoordinatorPatch, validate PatchValidator, guard *patchGuard) (*PatchResult, error) {
	if dialect.IsPostgres(s.db.DriverName()) {
		return s.patchCoordinatorPostgres(ctx, workspaceID, id, patch, validate, guard)
	}
	return s.patchCoordinatorSQLite(ctx, workspaceID, id, patch, validate, guard)
}

// patchCoordinatorSQLite takes the single writer lock up front with BEGIN
// IMMEDIATE, before any read, so a competing PATCH serializes rather than
// racing the read.
func (s *Store) patchCoordinatorSQLite(ctx context.Context, workspaceID, id string, patch CoordinatorPatch, validate PatchValidator, guard *patchGuard) (*PatchResult, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire writer connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return nil, fmt.Errorf("begin immediate: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			// The rollback must reach SQLite even when the caller canceled
			// ctx: a canceled ROLLBACK would leave the BEGIN IMMEDIATE
			// transaction and its write lock open on the pooled connection.
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()

	if s.afterLock != nil {
		s.afterLock(ctx)
	}

	result, err := s.patchCoordinatorBody(ctx, conn, func(q string) string { return q }, workspaceID, id, patch, validate, false, nil, guard)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return nil, fmt.Errorf("commit patch coordinator: %w", err)
	}
	committed = true
	return result, nil
}

// patchCoordinatorPostgres acquires the lock via SELECT ... FOR UPDATE inside
// patchCoordinatorBody: on PostgreSQL that statement is what acquires the
// row lock, so afterLock fires right after it returns (see the passed hook).
func (s *Store) patchCoordinatorPostgres(ctx context.Context, workspaceID, id string, patch CoordinatorPatch, validate PatchValidator, guard *patchGuard) (*PatchResult, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin patch coordinator: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	hook := func() {
		if s.afterLock != nil {
			s.afterLock(ctx)
		}
	}
	result, err := s.patchCoordinatorBody(ctx, tx, s.db.Rebind, workspaceID, id, patch, validate, true, hook, guard)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit patch coordinator: %w", err)
	}
	return result, nil
}

// patchCoordinatorBody reads the row under lock, merges the patch, validates,
// stamps updated_at with the store's clock, and writes the update. hookAfterRead
// is called right after the row read succeeds (used only by the PostgreSQL
// path, where that read is what acquires the lock); pass nil otherwise.
func (s *Store) patchCoordinatorBody(ctx context.Context, exec coordinatorExec, rebind func(string) string, workspaceID, id string, patch CoordinatorPatch, validate PatchValidator, forUpdate bool, hookAfterRead func(), guard *patchGuard) (*PatchResult, error) {
	turningAutonomyOff := patch.AutonomyEnabled != nil && !*patch.AutonomyEnabled
	if turningAutonomyOff {
		if err := s.takeWakeLock(ctx, exec, id); err != nil {
			return nil, err
		}
	}
	row, err := lockedCoordinatorRow(ctx, exec, rebind, workspaceID, id, forUpdate)
	if err != nil {
		return nil, err
	}
	if hookAfterRead != nil {
		hookAfterRead()
	}
	if err := guard.runPre(ctx, exec, row, &patch); err != nil {
		return nil, err
	}

	merged := mergeCoordinatorPatch(row, patch)
	if validate != nil {
		if err := validate(ctx, merged, row.toCoordinator()); err != nil {
			return nil, err
		}
	}
	clearedConversationTaskID, newConversationTaskID, newConfigRevision := configChangeEffects(row, merged)

	now := s.now()
	if err := s.writePatchedCoordinator(ctx, exec, rebind, row, merged, newConversationTaskID, newConfigRevision, now, guard); err != nil {
		return nil, err
	}
	if err := guard.runPost(ctx, exec); err != nil {
		return nil, err
	}
	if turningAutonomyOff {
		if _, err := exec.ExecContext(ctx, rebind(`
			UPDATE coordinator_wakes SET status = 'superseded', updated_at = ?
			WHERE coordinator_id = ? AND status = 'pending'`), now, row.ID); err != nil {
			return nil, fmt.Errorf("supersede pending wakes: %w", err)
		}
	}

	merged.ConversationTaskID = newConversationTaskID
	merged.ConfigRevision = newConfigRevision
	merged.CreatedAt = row.CreatedAt
	merged.UpdatedAt = now
	return &PatchResult{
		Coordinator:               merged,
		ClearedConversationTaskID: clearedConversationTaskID,
		AutonomyChanged:           autonomySettingsChanged(row, merged),
		TaskPairChanged: merged.TaskAgentProfileID != row.TaskAgentProfileID ||
			merged.TaskExecutorProfileID != row.TaskExecutorProfileID,
	}, nil
}

func (g *patchGuard) runPre(ctx context.Context, exec coordinatorExec, row *coordinatorRow, patch *CoordinatorPatch) error {
	if g == nil || g.pre == nil {
		return nil
	}
	return g.pre(ctx, exec, row, patch)
}

func (g *patchGuard) runPost(ctx context.Context, exec coordinatorExec) error {
	if g == nil || g.post == nil {
		return nil
	}
	return g.post(ctx, exec)
}

// configChangeEffects derives what a merged patch does to the conversation and
// the config revision: a changed context or profile drops the conversation and
// bumps the revision.
func configChangeEffects(row *coordinatorRow, merged *Coordinator) (cleared, conversationTaskID *string, revision int64) {
	conversationTaskID, revision = merged.ConversationTaskID, row.ConfigRevision
	if merged.Context == row.Context && merged.AgentProfileID == row.AgentProfileID && merged.ExecutorProfileID == row.ExecutorProfileID {
		return nil, conversationTaskID, revision
	}
	if row.ConversationTaskID.Valid {
		old := row.ConversationTaskID.String
		cleared = &old
	}
	return cleared, nil, row.ConfigRevision + 1
}

// writePatchedCoordinator writes the merged row. A guard that pins the stored
// context adds it to the WHERE clause, so a write that lost the context it was
// built against changes no row and returns the guard's mismatch error.
func (s *Store) writePatchedCoordinator(ctx context.Context, exec coordinatorExec, rebind func(string) string, row *coordinatorRow, merged *Coordinator, conversationTaskID *string, configRevision int64, now time.Time, guard *patchGuard) error {
	query := `
		UPDATE coordinators SET name = ?, agent_profile_id = ?, executor_profile_id = ?, context = ?, conversation_task_id = ?, config_revision = ?, autonomy_enabled = ?, cost_ceiling_subcents = ?, task_agent_profile_id = ?, task_executor_profile_id = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?`
	args := []any{merged.Name, merged.AgentProfileID, merged.ExecutorProfileID, merged.Context,
		nullableString(conversationTaskID), configRevision,
		autonomyColumn(merged.AutonomyEnabled), nullableInt64(merged.CostCeilingSubcents),
		merged.TaskAgentProfileID, merged.TaskExecutorProfileID, now, row.ID, row.WorkspaceID}
	pinned := guard != nil && guard.pinContext != nil
	if pinned {
		query += ` AND context = ?`
		args = append(args, *guard.pinContext)
	}
	res, err := exec.ExecContext(ctx, rebind(query), args...)
	if err != nil {
		return fmt.Errorf("update coordinator: %w", err)
	}
	if !pinned {
		return nil
	}
	matched, err := matchedRow(res)
	if err != nil {
		return err
	}
	if !matched {
		return guard.onPinMismatch
	}
	return nil
}

// autonomySettingsChanged reports whether the merged autonomy switch or
// ceiling differs from the row the PATCH read.
func autonomySettingsChanged(row *coordinatorRow, merged *Coordinator) bool {
	if row.AutonomyEnabled != merged.AutonomyEnabled {
		return true
	}
	if row.CostCeilingSubcents.Valid != (merged.CostCeilingSubcents != nil) {
		return true
	}
	return merged.CostCeilingSubcents != nil && *merged.CostCeilingSubcents != row.CostCeilingSubcents.Int64
}

// autonomyColumn renders the autonomy switch as its 0/1 column value.
func autonomyColumn(on bool) int {
	if on {
		return 1
	}
	return 0
}

func nullableInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

// lockedCoordinatorRow reads a coordinator row by (id, workspace_id) on the
// transaction-bound executor, optionally appending FOR UPDATE.
func lockedCoordinatorRow(ctx context.Context, exec coordinatorExec, rebind func(string) string, workspaceID, id string, forUpdate bool) (*coordinatorRow, error) {
	query := `SELECT ` + coordinatorColumns + ` FROM coordinators WHERE id = ? AND workspace_id = ?`
	if forUpdate {
		query += forUpdateClause
	}
	var row coordinatorRow
	err := exec.QueryRowContext(ctx, rebind(query), id, workspaceID).Scan(
		&row.ID, &row.WorkspaceID, &row.Name, &row.AgentProfileID, &row.ExecutorProfileID,
		&row.Context, &row.ConversationTaskID, &row.ConfigRevision, &row.CreatedAt, &row.UpdatedAt,
		&row.PolicyJSON, &row.PolicyRevision, &row.WatchScope,
		&row.AutonomyEnabled, &row.CostCeilingSubcents, &row.PausedAt, &row.PausedBy,
		&row.TaskAgentProfileID, &row.TaskExecutorProfileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock coordinator: %w", err)
	}
	return &row, nil
}

// mergeCoordinatorPatch returns a Coordinator built from row with patch's
// non-nil fields applied on top.
func mergeCoordinatorPatch(row *coordinatorRow, patch CoordinatorPatch) *Coordinator {
	merged := row.toCoordinator()
	if patch.Name != nil {
		merged.Name = *patch.Name
	}
	if patch.AgentProfileID != nil {
		merged.AgentProfileID = *patch.AgentProfileID
	}
	if patch.ExecutorProfileID != nil {
		merged.ExecutorProfileID = *patch.ExecutorProfileID
	}
	if patch.Context != nil {
		merged.Context = *patch.Context
	}
	if patch.TaskAgentProfileID != nil {
		merged.TaskAgentProfileID = *patch.TaskAgentProfileID
	}
	if patch.TaskExecutorProfileID != nil {
		merged.TaskExecutorProfileID = *patch.TaskExecutorProfileID
	}
	if patch.AutonomyEnabled != nil {
		merged.AutonomyEnabled = *patch.AutonomyEnabled
	}
	if patch.CeilingSet {
		merged.CostCeilingSubcents = patch.CostCeilingSubcents
	}
	return merged
}

func nullableString(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}

// resetConversation clears the coordinator's conversation task through exec,
// increments config_revision, and returns the previous task id, or "" when
// none was set. The caller archives that task after commit with
// Service.archiveConversation.
func (s *Store) resetConversation(ctx context.Context, exec coordinatorExec, coordinatorID string) (string, error) {
	var prev sql.NullString
	err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT conversation_task_id FROM coordinators WHERE id = ?`), coordinatorID).Scan(&prev)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read conversation task: %w", err)
	}
	if _, err := exec.ExecContext(ctx, s.db.Rebind(`UPDATE coordinators SET conversation_task_id = NULL, config_revision = config_revision + 1, updated_at = ? WHERE id = ?`),
		s.now(), coordinatorID); err != nil {
		return "", fmt.Errorf("reset conversation: %w", err)
	}
	return prev.String, nil
}
