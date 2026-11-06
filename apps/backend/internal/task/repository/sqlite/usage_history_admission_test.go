package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

func TestUsageHistoryBindingSchema(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	_, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	if err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}

	var bindingTables int
	if err := repo.db.Get(&bindingTables, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name IN ('usage_scopes', 'usage_subjects', 'usage_execution_bindings')
	`); err != nil {
		t.Fatalf("count retained binding tables: %v", err)
	}
	if bindingTables != 3 {
		t.Fatalf("retained binding tables = %d, want 3", bindingTables)
	}
}

func TestUsageHistoryNativeAtomic(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	analyticsRepo, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	if err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}
	repo.SetUsageHistoryRetainer(analyticsRepo)
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-1", Name: "usage history", OrgID: "org-1"}); err != nil {
		t.Fatalf("create trusted usage workspace: %v", err)
	}
	createUsageEventsTestTask(t, repo, "task-retained")
	createUsageEventsTestSession(t, repo, "session-retained", "task-retained")
	turn := &models.Turn{ID: "turn-retained", TaskID: "task-retained", TaskSessionID: "session-retained"}
	turnCtx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "native-actor", Role: authn.RoleMember})
	if err := repo.CreateTurn(turnCtx, turn); err != nil {
		t.Fatalf("create retained actor turn: %v", err)
	}

	event := newTestUsageEvent("evt-retained", "task-retained", "session-retained")
	event.TurnID, event.UsageBindingID = turn.ID, turn.UsageBindingID
	thought, reasoningOutput, reportedCacheWrite, reportedTotal := int64(8), int64(4), int64(15), int64(180)
	event.TokensThought = &thought
	event.ReasoningOutputTokens = &reasoningOutput
	event.ReportedCacheWriteTokens = &reportedCacheWrite
	event.ReportedTotalTokens = &reportedTotal
	event.TokensTotal = 163
	event.CostSource = "provider_reported"
	event.AgentType = "claude"
	event.NativeScope, event.MeasurementSource, event.UsageCompleteness = "direct", "response", "exact"
	event.UsageSchemaVersion, event.ContractVersion = 1, 1
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent: %v", err)
	}
	if err := repo.CreateTaskUsageEvent(context.Background(), event); !errors.Is(err, ErrDuplicateUsageEvent) {
		t.Fatalf("duplicate CreateTaskUsageEvent error = %v, want ErrDuplicateUsageEvent", err)
	}

	var retained struct {
		Count                 int           `db:"count"`
		TaskID                string        `db:"task_id"`
		SessionID             string        `db:"session_id"`
		BindingID             string        `db:"usage_binding_id"`
		ScopeID               string        `db:"scope_id"`
		SubjectID             string        `db:"subject_id"`
		ActorID               string        `db:"actor_id"`
		ActorStatus           string        `db:"actor_status"`
		OrganizationID        string        `db:"organization_id"`
		AgentClient           string        `db:"agent_client"`
		SourceTotal           sql.NullInt64 `db:"source_total_tokens"`
		ReportedCacheWrite    sql.NullInt64 `db:"reported_cache_write_tokens"`
		ReasoningOutput       sql.NullInt64 `db:"reasoning_output_tokens"`
		CostSubcents          sql.NullInt64 `db:"cost_subcents"`
		CostBasis             string        `db:"cost_basis"`
		NativeScope           string        `db:"native_scope"`
		MeasurementSource     string        `db:"measurement_source"`
		Completeness          string        `db:"usage_completeness"`
		SchemaVersion         int           `db:"usage_schema_version"`
		TokenSemanticsVersion int           `db:"token_semantics_version"`
		ReasoningInsideOutput int           `db:"reasoning_inside_output"`
	}
	if err := repo.db.Get(&retained, `
		SELECT COUNT(*) AS count, b.task_id, COALESCE(b.session_id, '') AS session_id,
		       b.usage_binding_id, b.scope_id, b.subject_id, b.actor_id, b.actor_status,
		       s.organization_id, b.agent_client, b.source_total_tokens, b.reported_cache_write_tokens,
		       b.reasoning_output_tokens, b.cost_subcents, b.cost_basis, b.native_scope,
		       b.measurement_source, b.usage_completeness, b.usage_schema_version,
		       b.token_semantics_version, b.reasoning_inside_output
		FROM session_usage_buckets b JOIN usage_scopes s ON s.scope_id = b.scope_id
		WHERE b.source = 'native' AND b.source_record_id = ? AND b.usage_identity = ?
		GROUP BY b.task_id, b.session_id, b.usage_binding_id, b.scope_id, b.subject_id,
		         b.actor_id, b.actor_status, s.organization_id
	`, event.UsageEventID, "native:"+event.UsageEventID); err != nil {
		t.Fatalf("read retained native observation: %v", err)
	}
	if retained.Count != 1 || retained.TaskID != event.TaskID || retained.SessionID != event.SessionID ||
		retained.BindingID != turn.UsageBindingID || retained.ScopeID == "" || retained.SubjectID == "" ||
		retained.ActorID != "native-actor" || retained.ActorStatus != "attributed" || retained.OrganizationID != "org-1" ||
		retained.AgentClient != "claude" || !retained.SourceTotal.Valid || retained.SourceTotal.Int64 != 180 ||
		!retained.ReportedCacheWrite.Valid || retained.ReportedCacheWrite.Int64 != 15 ||
		!retained.ReasoningOutput.Valid || retained.ReasoningOutput.Int64 != 4 ||
		!retained.CostSubcents.Valid || retained.CostSubcents.Int64 != 42 || retained.CostBasis != "reported" ||
		retained.NativeScope != "direct" || retained.MeasurementSource != "response" ||
		retained.Completeness != "exact" || retained.SchemaVersion != 1 ||
		retained.TokenSemanticsVersion != 1 || retained.ReasoningInsideOutput != 1 {
		t.Fatalf("retained native ownership = %+v, want the trusted turn actor and org/workspace scope", retained)
	}
	inTokens, cachedTokens, outTokens, costSubcents := readTaskSessionRollup(t, repo, event.SessionID)
	if inTokens != event.TokensIn || cachedTokens != 25 || outTokens != 30 || costSubcents != event.CostSubcents {
		t.Fatalf("rollup after duplicate event = [%d %d %d %d], want one event [%d 25 30 %d]", inTokens, cachedTokens, outTokens, costSubcents, event.TokensIn, event.CostSubcents)
	}
	var ledgerBindingID string
	if err := repo.db.Get(&ledgerBindingID, `SELECT usage_binding_id FROM task_usage_events WHERE usage_event_id = ?`, event.UsageEventID); err != nil {
		t.Fatalf("read native ledger binding id: %v", err)
	}
	if ledgerBindingID != turn.UsageBindingID {
		t.Fatalf("native ledger binding id = %q, want %q", ledgerBindingID, turn.UsageBindingID)
	}
	if _, err := repo.db.ExecContext(context.Background(), `DELETE FROM tasks WHERE id = ?`, event.TaskID); err != nil {
		t.Fatalf("delete source task: %v", err)
	}
	var afterDelete int
	if err := repo.db.Get(&afterDelete, `SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native' AND source_record_id = ?`, event.UsageEventID); err != nil {
		t.Fatalf("read retained usage after task deletion: %v", err)
	}
	if afterDelete != 1 {
		t.Fatalf("retained native rows after task deletion = %d, want 1", afterDelete)
	}
}

func TestUsageHistoryNativeRollbackAtomic(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	analyticsRepo, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	if err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}
	repo.SetUsageHistoryRetainer(analyticsRepo)
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-1", Name: "usage history", OrgID: "org-1"}); err != nil {
		t.Fatalf("create trusted usage workspace: %v", err)
	}
	createUsageEventsTestTask(t, repo, "task-retain-rollback")
	createUsageEventsTestSession(t, repo, "session-retain-rollback", "task-retain-rollback")
	tables := []string{
		"session_usage_measurements", "session_usage_buckets", "usage_live_links", "usage_dirty_partitions", "usage_execution_bindings", "usage_scopes", "usage_subjects",
		"usage_collection_bindings", "usage_collection_contexts", "usage_collection_current_targets", "usage_collection_grants",
		"usage_collection_source_guards", "usage_collection_guard_workspaces", "usage_collection_guard_erasure",
		"usage_legacy_adoptions", "usage_scope_fences", "usage_history_migration_gaps",
	}
	before := make(map[string][]byte, len(tables))
	for _, table := range tables {
		before[table] = usageAdmissionTableSnapshot(t, repo, table)
	}
	if _, err := repo.db.Exec(`CREATE TRIGGER fail_native_retention BEFORE INSERT ON session_usage_buckets
		WHEN NEW.source = 'native' BEGIN SELECT RAISE(ABORT, 'retained native failure'); END`); err != nil {
		t.Fatalf("create retention failure trigger: %v", err)
	}
	event := newTestUsageEvent("evt-retain-rollback", "task-retain-rollback", "session-retain-rollback")
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err == nil {
		t.Fatal("CreateTaskUsageEvent succeeded despite retained native write failure")
	}
	if got := countTaskUsageEventRows(t, repo); got != 0 {
		t.Errorf("task usage rows after rollback = %d, want 0", got)
	}
	inTokens, cachedTokens, outTokens, costSubcents := readTaskSessionRollup(t, repo, "session-retain-rollback")
	if inTokens != 0 || cachedTokens != 0 || outTokens != 0 || costSubcents != 0 {
		t.Errorf("session rollup after rollback = [%d %d %d %d], want all zero", inTokens, cachedTokens, outTokens, costSubcents)
	}
	for _, table := range tables {
		after := usageAdmissionTableSnapshot(t, repo, table)
		if !bytes.Equal(before[table], after) {
			t.Errorf("%s changed after failed usage transaction: before=%s after=%s", table, before[table], after)
		}
	}
}

func usageAdmissionTableSnapshot(t *testing.T, repo *Repository, table string) []byte {
	t.Helper()
	rows, err := repo.db.Queryx(`SELECT * FROM ` + table + ` ORDER BY 1`)
	if err != nil {
		t.Fatalf("read %s admission baseline: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	state := make([]json.RawMessage, 0)
	for rows.Next() {
		values, err := rows.SliceScan()
		if err != nil {
			t.Fatalf("scan %s admission baseline: %v", table, err)
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			t.Fatalf("encode %s admission baseline: %v", table, err)
		}
		state = append(state, encoded)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read %s admission baseline: %v", table, err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encode %s admission state: %v", table, err)
	}
	return encoded
}

func TestUsageHistoryAdmissionActorBinding(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	analyticsRepo, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	if err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}
	repo.SetUsageHistoryRetainer(analyticsRepo)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "usage history", OrgID: "org-1"}); err != nil {
		t.Fatalf("create trusted usage workspace: %v", err)
	}
	createUsageEventsTestTask(t, repo, "task-actor-binding")
	createUsageEventsTestSession(t, repo, "session-actor-binding", "task-actor-binding")
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID: "executor-actor-binding", Name: "actor binding", Type: "local",
		Status: models.ExecutorStatusActive, OwnerUserID: "executor-owner",
	}); err != nil {
		t.Fatalf("create trusted executor: %v", err)
	}
	executionCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "execution-actor", Role: authn.RoleMember, Synthetic: true})
	if err := repo.AdmitExecutorUse(executionCtx, "executor-actor-binding", "", "executor-owner", "", "session-actor-binding", "mutable-session-actor"); err != nil {
		t.Fatalf("admit executor use: %v", err)
	}
	var executionBindingID string
	if err := repo.db.Get(&executionBindingID, `SELECT usage_execution_binding_id FROM task_sessions WHERE id = ?`, "session-actor-binding"); err != nil {
		t.Fatalf("read execution binding pointer: %v", err)
	}
	var execution struct {
		ActorID     string `db:"actor_id"`
		ActorStatus string `db:"actor_status"`
		ActorKind   string `db:"actor_kind"`
	}
	if err := repo.db.Get(&execution, `SELECT actor_id, actor_status, actor_kind FROM usage_execution_bindings WHERE binding_id = ?`, executionBindingID); err != nil {
		t.Fatalf("read immutable execution binding: %v", err)
	}
	if execution.ActorID != "execution-actor" || execution.ActorStatus != "attributed" || execution.ActorKind != "synthetic" {
		t.Fatalf("execution actor binding = %+v, want the authenticated synthetic actor", execution)
	}

	turnCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "turn-actor", Role: authn.RoleMember})
	explicitTurn := &models.Turn{ID: "turn-explicit-actor", TaskID: "task-actor-binding", TaskSessionID: "session-actor-binding"}
	if err := repo.CreateTurn(turnCtx, explicitTurn); err != nil {
		t.Fatalf("create explicitly attributed turn: %v", err)
	}
	inheritedTurn := &models.Turn{ID: "turn-inherited-actor", TaskID: "task-actor-binding", TaskSessionID: "session-actor-binding"}
	if err := repo.CreateTurn(ctx, inheritedTurn); err != nil {
		t.Fatalf("create inherited turn: %v", err)
	}
	sharedTurn := &models.Turn{ID: "turn-shared-actor", TaskID: "task-actor-binding", TaskSessionID: "session-actor-binding"}
	if err := repo.CreateTurn(models.WithExecutorActor(ctx, ""), sharedTurn); err != nil {
		t.Fatalf("create explicit shared turn: %v", err)
	}
	assertUsageTurnActor(t, repo, explicitTurn.ID, "turn-actor", "attributed", "user")
	assertUsageTurnActor(t, repo, inheritedTurn.ID, "execution-actor", "attributed", "synthetic")
	assertUsageTurnActor(t, repo, sharedTurn.ID, "", "unknown", "unknown")
	var inheritedParent string
	if err := repo.db.Get(&inheritedParent, `SELECT parent_binding_id FROM usage_execution_bindings WHERE turn_id = ?`, inheritedTurn.ID); err != nil {
		t.Fatalf("read inherited turn's execution binding: %v", err)
	}
	if inheritedParent != executionBindingID {
		t.Fatalf("inherited turn parent binding = %q, want original execution binding %q", inheritedParent, executionBindingID)
	}
	if explicitTurn.UsageBindingID == "" || inheritedTurn.UsageBindingID == "" || sharedTurn.UsageBindingID == "" {
		t.Fatal("turn admission did not assign immutable usage binding ids")
	}
	laterExecutionCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "next-execution-actor", Role: authn.RoleMember})
	if err := repo.AdmitExecutorUse(laterExecutionCtx, "executor-actor-binding", "", "executor-owner", "", "session-actor-binding", "next-execution-actor"); err != nil {
		t.Fatalf("admit next executor use: %v", err)
	}
	var currentExecutionBindingID string
	if err := repo.db.Get(&currentExecutionBindingID, `SELECT usage_execution_binding_id FROM task_sessions WHERE id = ?`, "session-actor-binding"); err != nil {
		t.Fatalf("read next execution binding pointer: %v", err)
	}
	if currentExecutionBindingID == executionBindingID {
		t.Fatal("next execution admission reused the previous execution binding id")
	}
	if err := repo.db.Get(&execution, `SELECT actor_id, actor_status, actor_kind FROM usage_execution_bindings WHERE binding_id = ?`, executionBindingID); err != nil {
		t.Fatalf("reread immutable prior execution binding: %v", err)
	}
	if execution.ActorID != "execution-actor" || execution.ActorStatus != "attributed" || execution.ActorKind != "synthetic" {
		t.Fatalf("prior execution binding changed after new admission = %+v", execution)
	}
	assertUsageTurnActor(t, repo, inheritedTurn.ID, "execution-actor", "attributed", "synthetic")
	if err := repo.db.Get(&inheritedParent, `SELECT parent_binding_id FROM usage_execution_bindings WHERE turn_id = ?`, inheritedTurn.ID); err != nil {
		t.Fatalf("reread inherited turn's execution binding: %v", err)
	}
	if inheritedParent != executionBindingID {
		t.Fatalf("inherited turn parent binding changed to %q after next execution admission", inheritedParent)
	}
	if _, err := repo.db.Exec(`UPDATE task_sessions SET executor_actor_user_id = 'later-session-actor' WHERE id = ?`, "session-actor-binding"); err != nil {
		t.Fatalf("mutate current session actor: %v", err)
	}
	assertUsageTurnActor(t, repo, inheritedTurn.ID, "execution-actor", "attributed", "synthetic")
	var scope struct {
		WorkspaceID    string `db:"workspace_id"`
		OrganizationID string `db:"organization_id"`
	}
	if err := repo.db.Get(&scope, `SELECT s.workspace_id, s.organization_id FROM usage_execution_bindings b JOIN usage_scopes s ON s.scope_id = b.scope_id WHERE b.binding_id = ?`, explicitTurn.UsageBindingID); err != nil {
		t.Fatalf("read trusted retained scope: %v", err)
	}
	if scope.WorkspaceID != "ws-1" || scope.OrganizationID != "org-1" {
		t.Fatalf("trusted retained scope = %+v, want workspace ws-1 and org-1", scope)
	}

	sharedEvent := newTestUsageEvent("evt-shared-actor", "task-actor-binding", "session-actor-binding")
	sharedEvent.TurnID = sharedTurn.ID
	if err := repo.CreateTaskUsageEvent(ctx, sharedEvent); err != nil {
		t.Fatalf("retain native usage for shared turn: %v", err)
	}
	var retainedActor struct {
		ActorID     string `db:"actor_id"`
		ActorStatus string `db:"actor_status"`
		Attribution string `db:"attribution_status"`
		BindingID   string `db:"usage_binding_id"`
	}
	if err := repo.db.Get(&retainedActor, `SELECT actor_id, actor_status, attribution_status, usage_binding_id FROM session_usage_buckets WHERE source = 'native' AND source_record_id = ?`, sharedEvent.UsageEventID); err != nil {
		t.Fatalf("read shared-turn retained usage actor: %v", err)
	}
	if retainedActor.ActorID != "" || retainedActor.ActorStatus != "unknown" || retainedActor.Attribution != "attributed" || retainedActor.BindingID == "" {
		t.Fatalf("shared-turn retained actor = %+v, want unknown actor with independent work attribution", retainedActor)
	}
	secondSharedEvent := newTestUsageEvent("evt-shared-actor-2", "task-actor-binding", "session-actor-binding")
	secondSharedEvent.TurnID = sharedTurn.ID
	if err := repo.CreateTaskUsageEvent(ctx, secondSharedEvent); err != nil {
		t.Fatalf("retain second native event for shared turn: %v", err)
	}
	var secondBindingID string
	if err := repo.db.Get(&secondBindingID, `SELECT usage_binding_id FROM session_usage_buckets WHERE source = 'native' AND source_record_id = ?`, secondSharedEvent.UsageEventID); err != nil {
		t.Fatalf("read second shared-turn binding: %v", err)
	}
	if secondBindingID != retainedActor.BindingID {
		t.Fatalf("unknown binding ids for one turn = %q and %q, want stable turn binding", retainedActor.BindingID, secondBindingID)
	}

	fallbackEvent := newTestUsageEvent("evt-bound-fallback", "task-actor-binding", "session-actor-binding")
	fallbackEvent.TurnID = explicitTurn.ID
	if err := repo.CreateTaskUsageEvent(ctx, fallbackEvent); err != nil {
		t.Fatalf("retain native event without wire binding id: %v", err)
	}
	var fallbackActor struct {
		ActorID   string `db:"actor_id"`
		BindingID string `db:"usage_binding_id"`
	}
	if err := repo.db.Get(&fallbackActor, `SELECT actor_id, usage_binding_id FROM session_usage_buckets WHERE source = 'native' AND source_record_id = ?`, fallbackEvent.UsageEventID); err != nil {
		t.Fatalf("read native event resolved from turn binding: %v", err)
	}
	if fallbackActor.ActorID != "turn-actor" || fallbackActor.BindingID != explicitTurn.UsageBindingID {
		t.Fatalf("native fallback binding = %+v, want authenticated turn actor and binding", fallbackActor)
	}
}

func assertUsageTurnActor(t *testing.T, repo *Repository, turnID, actorID, actorStatus, actorKind string) {
	t.Helper()
	var got struct {
		ActorID     string `db:"actor_id"`
		ActorStatus string `db:"actor_status"`
		ActorKind   string `db:"actor_kind"`
	}
	if err := repo.db.Get(&got, `SELECT actor_id, actor_status, actor_kind FROM usage_execution_bindings WHERE turn_id = ?`, turnID); err != nil {
		t.Fatalf("read turn binding %s: %v", turnID, err)
	}
	if got.ActorID != actorID || got.ActorStatus != actorStatus || got.ActorKind != actorKind {
		t.Errorf("turn %s actor binding = %+v, want actor=%q status=%q kind=%q", turnID, got, actorID, actorStatus, actorKind)
	}
}
