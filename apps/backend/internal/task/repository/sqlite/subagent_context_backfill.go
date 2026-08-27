// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

const (
	subagentContextCaptureSinceKey       = "subagent_context_capture_since"
	subagentContextBackfillThroughKey    = "subagent_context_backfill_through"
	subagentContextBackfillMigrationName = "task_session_subagents.backfill"
)

// warnSubagentContextMigration logs a failure in the exact migration warning
// shape used by the repository's MigrateLogger. The backfill is telemetry-only:
// failures are visible at WARN and never abort backend startup.
func (r *Repository) warnSubagentContextMigration(name string, err error) {
	if r.log == nil {
		return
	}
	r.log.Warn("migration failed", zap.String("name", name), zap.Error(err))
}

// migrateSubagentContextBackfill recovers task_session_subagents history from
// existing task_session_messages rows whose metadata.normalized.kind is
// "subagent_task" (AC-21), and publishes the two write-once kandev_meta
// activation keys that let a consumer distinguish "zero fan-out" from
// "unmeasured, before capture began" (AC-24, AC-25). See
// docs/specs/agents/requirements/subagent-context-persistence.md § Backfill.
//
// runMigrations() (and therefore this function) runs on every boot, not just
// the first. AC-24d (revision 4) requires the guard to be "both backfill keys
// present", never `capture_since` alone: a pre-amendment build wrote only
// `capture_since` in an unguarded third statement, so guarding on it alone
// makes a `backfill_through`-missing installation permanently stuck. Checking
// both, and having each key's own INSERT use ON CONFLICT DO NOTHING, is what
// lets a partial pre-existing pair repair itself on the next boot without a
// second mechanism.
func (r *Repository) migrateSubagentContextBackfill() {
	capturePresent, err := r.subagentContextActivationKeyPresent(subagentContextCaptureSinceKey)
	if err != nil {
		r.warnSubagentContextMigration(subagentContextBackfillMigrationName+".guard", err)
		return
	}
	throughPresent, err := r.subagentContextActivationKeyPresent(subagentContextBackfillThroughKey)
	if err != nil {
		r.warnSubagentContextMigration(subagentContextBackfillMigrationName+".guard", err)
		return
	}
	if capturePresent && throughPresent {
		return
	}

	if err := r.migrateSubagentContextBackfillUnsafe(); err != nil {
		r.warnSubagentContextMigration(subagentContextBackfillMigrationName, err)
	}
}

// migrateSubagentContextBackfillUnsafe runs the AC-21 INSERT ... SELECT and
// both AC-24 key writes in one transaction (AC-24d): either everything here
// commits, or nothing does, so a failure at any point leaves neither key
// written and the backfill retries whole on the next boot (AC-24a).
//
// Ordering inside the transaction is deliberate:
//  1. The `backfill_through` high-water mark is computed from AC-21's SOURCE
//     PREDICATE, over a snapshot no later than the INSERT's own (AC-24d
//     revision 3) — computing it BEFORE the INSERT satisfies that on both
//     SQLite and PostgreSQL's READ COMMITTED, since it can only under-claim,
//     never over-claim, the window actually covered.
//  2. The INSERT ... SELECT runs, deduplicating rows that share
//     (task_session_id, tool_call_id) by newest created_at then greatest id
//     (AC-21b), and skipping rows missing any of the three required
//     identities (AC-21a). ON CONFLICT DO NOTHING (AC-22) makes a retry a
//     no-op against rows already backfilled or already captured live under
//     the 'unknown' sentinel.
//  3. `capture_since` is sampled by Go, in-transaction, at or after step 2
//     completes and before the key writes (AC-24 point 4) — never at
//     transaction start, which would over-claim the whole scan's duration.
//  4. Both keys are written with their own ON CONFLICT DO NOTHING, so a key
//     already present — from this same transaction's retry guard failing to
//     trigger, or from a pre-amendment partial pair — is preserved verbatim
//     (AC-24 point 5, AC-24d revision 4).
func (r *Repository) migrateSubagentContextBackfillUnsafe() error {
	postgres := dialect.IsPostgres(r.db.DriverName())
	predicate, selectCols := subagentContextBackfillPredicateAndColumns(postgres)

	tx, err := r.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin subagent context backfill transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	backfillThrough, err := subagentContextBackfillHighWaterMark(tx, postgres, predicate)
	if err != nil {
		return err
	}

	insertSQL := `
		WITH ranked AS (
			SELECT
				m.*,
				` + selectCols.toolCallID + ` AS derived_tool_call_id,
				ROW_NUMBER() OVER (
					PARTITION BY m.task_session_id, ` + selectCols.toolCallID + `
					ORDER BY m.created_at DESC, m.id DESC
				) AS rn
			FROM task_session_messages m
			WHERE ` + predicate + `
		)
		INSERT INTO task_session_subagents (
			id, task_session_id, task_id, turn_id, tool_call_id, agent_execution_id, parent_tool_call_id,
			subagent_type, description, agent_id, child_session_id, model,
			agent_status, tool_status, is_async, total_tokens, tool_use_count,
			duration_ms, source, observed_at, settled_at, updated_at
		)
		SELECT
			m.id,
			m.task_session_id,
			m.task_id,
			NULLIF(m.turn_id, ''),
			m.derived_tool_call_id,
			'` + subagentContextUnknownExecutionID + `',
			` + selectCols.parentToolCallID + `,
			` + selectCols.subagentType + `,
			` + selectCols.description + `,
			` + selectCols.agentID + `,
			` + selectCols.childSessionID + `,
			` + selectCols.model + `,
			` + selectCols.agentStatus + `,
			` + selectCols.toolStatus + `,
			` + selectCols.isAsync + `,
			` + selectCols.totalTokens + `,
			` + selectCols.toolUseCount + `,
			` + selectCols.durationMs + `,
			'backfill',
			m.created_at,
			(CASE WHEN ` + selectCols.terminal + ` THEN m.updated_at ELSE NULL END),
			m.updated_at
		FROM ranked m
		WHERE m.rn = 1
		ON CONFLICT (task_session_id, agent_execution_id, tool_call_id) DO NOTHING
	`
	if _, err := tx.Exec(insertSQL); err != nil {
		return fmt.Errorf("subagent context backfill insert: %w", err)
	}

	captureSince := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(subagentContextActivationKeyInsertSQL(subagentContextCaptureSinceKey, captureSince)); err != nil {
		return fmt.Errorf("write subagent_context_capture_since: %w", err)
	}
	if _, err := tx.Exec(subagentContextActivationKeyInsertSQL(subagentContextBackfillThroughKey, backfillThrough)); err != nil {
		return fmt.Errorf("write subagent_context_backfill_through: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subagent context backfill transaction: %w", err)
	}
	return nil
}

// subagentContextBackfillHighWaterMark evaluates AC-21's source predicate's
// MAX(created_at) inside the caller's transaction, before the INSERT that
// shares the same predicate (AC-24d revision 3). AC-24f: the empty string is
// the sentinel for "no message matched", not a timestamp, and is exempt from
// the RFC3339Nano requirement the non-empty case observes.
//
// SQLite's driver only auto-converts a column value to time.Time when it can
// see the declared column type; wrapping it in MAX() loses that, so the scan
// target must be a string on that dialect (see parseLegacyTimestamp's own
// comment for the same fact). PostgreSQL's driver has no such gap.
func subagentContextBackfillHighWaterMark(tx *sqlx.Tx, postgres bool, predicate string) (string, error) {
	query := `SELECT MAX(m.created_at) FROM task_session_messages m WHERE ` + predicate
	if postgres {
		var maxCreatedAt sql.NullTime
		if err := tx.QueryRow(query).Scan(&maxCreatedAt); err != nil {
			return "", fmt.Errorf("compute subagent context backfill high-water mark: %w", err)
		}
		if !maxCreatedAt.Valid {
			return "", nil
		}
		return maxCreatedAt.Time.UTC().Format(time.RFC3339Nano), nil
	}
	var maxCreatedAt sql.NullString
	if err := tx.QueryRow(query).Scan(&maxCreatedAt); err != nil {
		return "", fmt.Errorf("compute subagent context backfill high-water mark: %w", err)
	}
	if !maxCreatedAt.Valid || maxCreatedAt.String == "" {
		return "", nil
	}
	parsed := parseLegacyTimestamp(maxCreatedAt.String)
	if parsed.IsZero() {
		return "", fmt.Errorf("compute subagent context backfill high-water mark: unparsable timestamp %q", maxCreatedAt.String)
	}
	return parsed.Format(time.RFC3339Nano), nil
}

// subagentContextBackfillColumns names every dialect-aware source expression
// the backfill INSERT's SELECT list needs, built alongside the predicate so
// the two stay in sync (both are derived from the same "meta" helper).
type subagentContextBackfillColumns struct {
	toolCallID       string
	parentToolCallID string
	subagentType     string
	description      string
	agentID          string
	childSessionID   string
	model            string
	agentStatus      string
	toolStatus       string
	isAsync          string
	totalTokens      string
	toolUseCount     string
	durationMs       string
	terminal         string
}

// subagentContextBackfillPredicateAndColumns builds AC-21's source predicate
// (which qualifying messages) and the dialect-aware column expressions the
// INSERT's SELECT list projects from each one. AC-21a: the predicate itself
// enforces the three required identities (task_id, tool_call_id, and the
// "subagent_task" kind marker), so a message missing any of them contributes
// no row rather than a row with a blank identity column.
func subagentContextBackfillPredicateAndColumns(postgres bool) (string, subagentContextBackfillColumns) {
	meta := func(path string) string { return jsonKey(postgres, "m.metadata", path) }
	nullable := func(expr string) string { return "NULLIF(" + expr + ", '')" }
	subagentText := func(field string) string { return nullable(meta("normalized.subagent_task." + field)) }
	subagentInt := func(field string) string { return jsonInt(postgres, "m.metadata", "normalized.subagent_task", field) }

	toolCallID := meta("tool_call_id")
	toolStatusRaw := meta("status")
	terminal := "(" + toolStatusRaw + " IN ('complete','completed','success','error','failed','cancelled'))"
	predicate := `
			` + meta("normalized.kind") + ` = 'subagent_task'
			AND m.task_id IS NOT NULL AND m.task_id != ''
			AND ` + toolCallID + ` IS NOT NULL AND ` + toolCallID + ` != ''`

	cols := subagentContextBackfillColumns{
		toolCallID:       toolCallID,
		parentToolCallID: nullable(meta("parent_tool_call_id")),
		subagentType:     subagentText("subagent_type"),
		description:      subagentText("description"),
		agentID:          subagentText("agent_id"),
		childSessionID:   subagentText("child_session_id"),
		model:            subagentText("model"),
		agentStatus:      subagentText("status"),
		toolStatus:       nullable(toolStatusRaw),
		isAsync:          jsonBoolToInt(postgres, "m.metadata", "normalized.subagent_task", "is_async"),
		totalTokens:      subagentInt("total_tokens"),
		toolUseCount:     subagentInt("tool_use_count"),
		durationMs:       subagentInt("duration_ms"),
		terminal:         terminal,
	}
	return predicate, cols
}
