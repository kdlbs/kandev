package sqlite

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func sp(s string) *string       { return &s }
func i64p(n int64) *int64       { return &n }
func tp(t time.Time) *time.Time { return &t }

func newSubagentContextTestRepo(t *testing.T, taskID, sessionID, turnID string) *Repository {
	t.Helper()
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	return repo
}

func mustGetSubagentContext(t *testing.T, repo *Repository, sessionID, toolCallID string) *models.SubagentContext {
	t.Helper()
	rows, err := repo.ListSubagentContextsBySession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListSubagentContextsBySession: %v", err)
	}
	for _, row := range rows {
		if row.ToolCallID == toolCallID {
			return row
		}
	}
	t.Fatalf("no subagent context row for (%s, %s) among %d rows", sessionID, toolCallID, len(rows))
	return nil
}

// TestUpsertSubagentContextInsertsRow covers AC-1: a first observation
// creates a row.
func TestUpsertSubagentContextInsertsRow(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-1", "session-1", "turn-1")
	ctx := context.Background()
	observedAt := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-1",
		TaskID:        "task-1",
		TurnID:        sp("turn-1"),
		ToolCallID:    "tc-1",
		Source:        "live",
		ObservedAt:    observedAt,
		UpdatedAt:     observedAt,
	})
	if err != nil {
		t.Fatalf("UpsertSubagentContext: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-1", "tc-1")
	if row.Source != "live" {
		t.Errorf("source = %q, want live", row.Source)
	}
	if !row.ObservedAt.Equal(observedAt) {
		t.Errorf("observed_at = %v, want %v", row.ObservedAt, observedAt)
	}
	if row.TurnID == nil || *row.TurnID != "turn-1" {
		t.Errorf("turn_id = %v, want turn-1", row.TurnID)
	}
}

// TestUpsertSubagentContextSecondCallUpdatesInPlace covers AC-3: a later
// frame for an already-recorded key updates the same row, never a second one.
func TestUpsertSubagentContextSecondCallUpdatesInPlace(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-2", "session-2", "turn-2")
	ctx := context.Background()
	first := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)

	base := models.SubagentContext{
		TaskSessionID: "session-2", TaskID: "task-2", TurnID: sp("turn-2"),
		ToolCallID: "tc-2", Source: "live", ObservedAt: first, UpdatedAt: first,
	}
	if err := repo.UpsertSubagentContext(ctx, &base); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second_ := base
	second_.Model = sp("claude-opus-5")
	second_.UpdatedAt = second
	if err := repo.UpsertSubagentContext(ctx, &second_); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	rows, err := repo.ListSubagentContextsBySession(ctx, "session-2")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1", len(rows))
	}
	if rows[0].Model == nil || *rows[0].Model != "claude-opus-5" {
		t.Errorf("model = %v, want claude-opus-5", rows[0].Model)
	}
}

// TestUpsertSubagentContextFillForward covers AC-4: a frame with an
// empty/absent field never blanks a value already learned.
func TestUpsertSubagentContextFillForward(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-3", "session-3", "turn-3")
	ctx := context.Background()
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-3", TaskID: "task-3", TurnID: sp("turn-3"),
		ToolCallID: "tc-3", Source: "live", ObservedAt: ts, UpdatedAt: ts,
		SubagentType: sp("security-reviewer"), Description: sp("review the diff"),
		Model: sp("claude-opus-5"), TotalTokens: i64p(500),
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// A later frame reports nothing new — every already-learned field must
	// survive untouched.
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-3", TaskID: "task-3",
		ToolCallID: "tc-3", Source: "live", ObservedAt: ts, UpdatedAt: ts.Add(time.Second),
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-3", "tc-3")
	if row.SubagentType == nil || *row.SubagentType != "security-reviewer" {
		t.Errorf("subagent_type = %v, want preserved", row.SubagentType)
	}
	if row.Description == nil || *row.Description != "review the diff" {
		t.Errorf("description = %v, want preserved", row.Description)
	}
	if row.Model == nil || *row.Model != "claude-opus-5" {
		t.Errorf("model = %v, want preserved", row.Model)
	}
	if row.TotalTokens == nil || *row.TotalTokens != 500 {
		t.Errorf("total_tokens = %v, want preserved", row.TotalTokens)
	}
}

// TestUpsertSubagentContextReplacesWithNonEmptyValue covers AC-5: a
// non-empty reported value replaces the stored value.
func TestUpsertSubagentContextReplacesWithNonEmptyValue(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-4", "session-4", "turn-4")
	ctx := context.Background()
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-4", TaskID: "task-4", ToolCallID: "tc-4",
		Source: "live", ObservedAt: ts, UpdatedAt: ts, TotalTokens: i64p(100),
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-4", TaskID: "task-4", ToolCallID: "tc-4",
		Source: "live", ObservedAt: ts, UpdatedAt: ts.Add(time.Second), TotalTokens: i64p(250),
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-4", "tc-4")
	if row.TotalTokens == nil || *row.TotalTokens != 250 {
		t.Errorf("total_tokens = %v, want 250 (replaced)", row.TotalTokens)
	}
}

// TestUpsertSubagentContextSettledAtWriteOnce covers AC-11: a second terminal
// frame with a later timestamp leaves the first settled_at value.
func TestUpsertSubagentContextSettledAtWriteOnce(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-5", "session-5", "turn-5")
	ctx := context.Background()
	firstSettle := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	laterSettle := firstSettle.Add(time.Hour)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-5", TaskID: "task-5", ToolCallID: "tc-5",
		Source: "live", ObservedAt: firstSettle, UpdatedAt: firstSettle,
		ToolStatus: sp("completed"), SettledAt: tp(firstSettle),
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-5", TaskID: "task-5", ToolCallID: "tc-5",
		Source: "live", ObservedAt: firstSettle, UpdatedAt: laterSettle,
		ToolStatus: sp("completed"), SettledAt: tp(laterSettle),
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-5", "tc-5")
	if row.SettledAt == nil || !row.SettledAt.Equal(firstSettle) {
		t.Errorf("settled_at = %v, want %v (write-once)", row.SettledAt, firstSettle)
	}
}

// TestUpsertSubagentContextPostSettleFillsNullsButFreezesStatus covers AC-12:
// a non-terminal frame after settling leaves settled_at and tool_status
// alone but still fills columns that are currently NULL.
func TestUpsertSubagentContextPostSettleFillsNullsButFreezesStatus(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-6", "session-6", "turn-6")
	ctx := context.Background()
	settleTime := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-6", TaskID: "task-6", ToolCallID: "tc-6",
		Source: "live", ObservedAt: settleTime, UpdatedAt: settleTime,
		ToolStatus: sp("completed"), SettledAt: tp(settleTime),
	}); err != nil {
		t.Fatalf("first (terminal) upsert: %v", err)
	}

	// A stray non-terminal/duplicate frame arrives after settling — it must
	// not resurrect tool_status or settled_at, but new information (model)
	// still fills the NULL column.
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-6", TaskID: "task-6", ToolCallID: "tc-6",
		Source: "live", ObservedAt: settleTime, UpdatedAt: settleTime.Add(time.Second),
		ToolStatus: sp("running"), Model: sp("claude-opus-5"),
	}); err != nil {
		t.Fatalf("second (post-settle) upsert: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-6", "tc-6")
	if row.ToolStatus == nil || *row.ToolStatus != "completed" {
		t.Errorf("tool_status = %v, want frozen at completed", row.ToolStatus)
	}
	if row.SettledAt == nil || !row.SettledAt.Equal(settleTime) {
		t.Errorf("settled_at = %v, want frozen at %v", row.SettledAt, settleTime)
	}
	if row.Model == nil || *row.Model != "claude-opus-5" {
		t.Errorf("model = %v, want filled from post-settle frame", row.Model)
	}
}

// TestUpsertSubagentContextNeverTerminalRowSettledAtNull covers AC-10: a row
// that never reaches terminal keeps settled_at NULL and still exists.
func TestUpsertSubagentContextNeverTerminalRowSettledAtNull(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-7", "session-7", "turn-7")
	ctx := context.Background()
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-7", TaskID: "task-7", ToolCallID: "tc-7",
		Source: "live", ObservedAt: ts, UpdatedAt: ts, ToolStatus: sp("in_progress"),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-7", "tc-7")
	if row.SettledAt != nil {
		t.Errorf("settled_at = %v, want NULL", row.SettledAt)
	}
}

// TestUpsertSubagentContextTurnIDPreservedFromFirstFrame covers AC-2a: the
// original turn_id survives a later frame carrying a different turn.
func TestUpsertSubagentContextTurnIDPreservedFromFirstFrame(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-8", "session-8", "turn-8a")
	ctx := context.Background()
	if err := seedTurn(t, repo, "turn-8b", "session-8", "task-8"); err != nil {
		t.Fatalf("seed second turn: %v", err)
	}
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-8", TaskID: "task-8", TurnID: sp("turn-8a"),
		ToolCallID: "tc-8", Source: "live", ObservedAt: ts, UpdatedAt: ts,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-8", TaskID: "task-8", TurnID: sp("turn-8b"),
		ToolCallID: "tc-8", Source: "live", ObservedAt: ts, UpdatedAt: ts.Add(time.Second),
	}); err != nil {
		t.Fatalf("second upsert under a different turn: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-8", "tc-8")
	if row.TurnID == nil || *row.TurnID != "turn-8a" {
		t.Errorf("turn_id = %v, want turn-8a (original turn preserved)", row.TurnID)
	}
}

// TestUpsertSubagentContextIsAsyncSticky covers "is_async is sticky": once
// set true, a later frame reporting false must not reset it.
func TestUpsertSubagentContextIsAsyncSticky(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-9", "session-9", "turn-9")
	ctx := context.Background()
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-9", TaskID: "task-9", ToolCallID: "tc-9",
		Source: "live", ObservedAt: ts, UpdatedAt: ts, IsAsync: true,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-9", TaskID: "task-9", ToolCallID: "tc-9",
		Source: "live", ObservedAt: ts, UpdatedAt: ts.Add(time.Second), IsAsync: false,
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-9", "tc-9")
	if !row.IsAsync {
		t.Error("is_async = false, want sticky true")
	}
}

// TestUpsertSubagentContextSourceAndObservedAtNotOverwritten covers write-once
// source/observed_at (AC-1a, AC-22): neither is present in the conflict SET
// list, so a later upsert (even one that claims a different source or a
// different observed_at) must not change them.
func TestUpsertSubagentContextSourceAndObservedAtNotOverwritten(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-10", "session-10", "turn-10")
	ctx := context.Background()
	firstObserved := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	laterObserved := firstObserved.Add(time.Hour)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-10", TaskID: "task-10", ToolCallID: "tc-10",
		Source: "backfill", ObservedAt: firstObserved, UpdatedAt: firstObserved,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-10", TaskID: "task-10", ToolCallID: "tc-10",
		Source: "live", ObservedAt: laterObserved, UpdatedAt: laterObserved,
	}); err != nil {
		t.Fatalf("second (live) upsert: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-10", "tc-10")
	if row.Source != "backfill" {
		t.Errorf("source = %q, want backfill preserved (a live write must not relabel a backfilled row)", row.Source)
	}
	if !row.ObservedAt.Equal(firstObserved) {
		t.Errorf("observed_at = %v, want %v (first observation wins)", row.ObservedAt, firstObserved)
	}
}

// TestUpsertSubagentContextNestedParentToolCallID covers AC-6: a nested
// subagent is stored, not suppressed.
func TestUpsertSubagentContextNestedParentToolCallID(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-11", "session-11", "turn-11")
	ctx := context.Background()
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-11", TaskID: "task-11", ToolCallID: "tc-11-child",
		ParentToolCallID: sp("tc-11-parent"), Source: "live", ObservedAt: ts, UpdatedAt: ts,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-11", "tc-11-child")
	if row.ParentToolCallID == nil || *row.ParentToolCallID != "tc-11-parent" {
		t.Errorf("parent_tool_call_id = %v, want tc-11-parent", row.ParentToolCallID)
	}
}

// TestUpsertSubagentContextDefaultsExecutionIDToUnknown covers AC-31: an
// empty ExecutionID is not stored verbatim as "", it stores the reserved
// 'unknown' sentinel so the column is never NULL and never blank.
func TestUpsertSubagentContextDefaultsExecutionIDToUnknown(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-exec-default", "session-exec-default", "turn-exec-default")
	ctx := context.Background()
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-exec-default", TaskID: "task-exec-default", ToolCallID: "tc-exec-default",
		Source: "live", ObservedAt: ts, UpdatedAt: ts,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-exec-default", "tc-exec-default")
	if row.AgentExecutionID != "unknown" {
		t.Errorf("agent_execution_id = %q, want unknown", row.AgentExecutionID)
	}
}

// TestUpsertSubagentContextStoresExecutionIDVerbatim covers AC-30: a
// non-empty ExecutionID is stored exactly as given, not normalized or
// truncated.
func TestUpsertSubagentContextStoresExecutionIDVerbatim(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-exec-verbatim", "session-exec-verbatim", "turn-exec-verbatim")
	ctx := context.Background()
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-exec-verbatim", TaskID: "task-exec-verbatim", ToolCallID: "tc-exec-verbatim",
		AgentExecutionID: "exec-abc-123", Source: "live", ObservedAt: ts, UpdatedAt: ts,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	row := mustGetSubagentContext(t, repo, "session-exec-verbatim", "tc-exec-verbatim")
	if row.AgentExecutionID != "exec-abc-123" {
		t.Errorf("agent_execution_id = %q, want exec-abc-123", row.AgentExecutionID)
	}
}

// TestUpsertSubagentContextCrossExecutionToolCallIDDoesNotClobber covers
// AC-32, the maintainer-found defect Amendment 1 fixes: two different agent
// process executions can legitimately reuse the same tool_call_id (a fresh
// ACP session numbering its tool calls from 1 again). Before
// agent_execution_id joined the upsert/unique key, the second execution's
// frame would silently overwrite the first execution's row instead of
// creating its own.
func TestUpsertSubagentContextCrossExecutionToolCallIDDoesNotClobber(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-cross-exec", "session-cross-exec", "turn-cross-exec")
	ctx := context.Background()
	firstObserved := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	secondObserved := firstObserved.Add(time.Hour)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-cross-exec", TaskID: "task-cross-exec", ToolCallID: "tc-shared",
		AgentExecutionID: "exec-first", Model: sp("model-first"),
		Source: "live", ObservedAt: firstObserved, UpdatedAt: firstObserved,
	}); err != nil {
		t.Fatalf("first execution upsert: %v", err)
	}
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-cross-exec", TaskID: "task-cross-exec", ToolCallID: "tc-shared",
		AgentExecutionID: "exec-second", Model: sp("model-second"),
		Source: "live", ObservedAt: secondObserved, UpdatedAt: secondObserved,
	}); err != nil {
		t.Fatalf("second execution upsert: %v", err)
	}

	rows, err := repo.ListSubagentContextsBySession(ctx, "session-cross-exec")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("row count = %d, want 2 (one per execution, same tool_call_id must not clobber)", len(rows))
	}

	byExecution := map[string]*models.SubagentContext{}
	for _, row := range rows {
		byExecution[row.AgentExecutionID] = row
	}
	first, ok := byExecution["exec-first"]
	if !ok || first.Model == nil || *first.Model != "model-first" {
		t.Errorf("exec-first row = %+v, want model-first preserved", first)
	}
	second, ok := byExecution["exec-second"]
	if !ok || second.Model == nil || *second.Model != "model-second" {
		t.Errorf("exec-second row = %+v, want model-second preserved", second)
	}
}

// TestUpsertSubagentContextCascadeDeletesWithSession covers AC-16: deleting
// the parent session removes its subagent context rows.
func TestUpsertSubagentContextCascadeDeletesWithSession(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-12", "session-12", "turn-12")
	ctx := context.Background()
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-12", TaskID: "task-12", ToolCallID: "tc-12",
		Source: "live", ObservedAt: ts, UpdatedAt: ts,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, "session-12"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}

	rows, err := repo.ListSubagentContextsBySession(ctx, "session-12")
	if err != nil {
		t.Fatalf("list after cascade: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("row count after session delete = %d, want 0 (FK cascade)", len(rows))
	}
}

// TestListSubagentContextsOrderingUsesToolCallIDTiebreak covers AC-13: rows
// sharing an observed_at order by tool_call_id, never by the generated id.
func TestListSubagentContextsOrderingUsesToolCallIDTiebreak(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-13", "session-13", "turn-13")
	ctx := context.Background()
	sharedObservedAt := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	// Insert in an order that would produce the wrong result if id (insertion
	// order / generated UUID) were used as the tiebreak instead of tool_call_id.
	for _, toolCallID := range []string{"tc-z", "tc-a", "tc-m"} {
		if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
			TaskSessionID: "session-13", TaskID: "task-13", ToolCallID: toolCallID,
			Source: "live", ObservedAt: sharedObservedAt, UpdatedAt: sharedObservedAt,
		}); err != nil {
			t.Fatalf("upsert %s: %v", toolCallID, err)
		}
	}

	rows, err := repo.ListSubagentContextsBySession(ctx, "session-13")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("row count = %d, want 3", len(rows))
	}
	got := []string{rows[0].ToolCallID, rows[1].ToolCallID, rows[2].ToolCallID}
	want := []string{"tc-a", "tc-m", "tc-z"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v (tool_call_id tiebreak)", got, want)
		}
	}
}

// TestListSubagentContextsByTurn covers the turn-scoped read seam.
func TestListSubagentContextsByTurn(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-14", "session-14", "turn-14a")
	ctx := context.Background()
	if err := seedTurn(t, repo, "turn-14b", "session-14", "task-14"); err != nil {
		t.Fatalf("seed second turn: %v", err)
	}
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-14", TaskID: "task-14", TurnID: sp("turn-14a"),
		ToolCallID: "tc-14a", Source: "live", ObservedAt: ts, UpdatedAt: ts,
	}); err != nil {
		t.Fatalf("upsert turn-14a: %v", err)
	}
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-14", TaskID: "task-14", TurnID: sp("turn-14b"),
		ToolCallID: "tc-14b", Source: "live", ObservedAt: ts.Add(time.Second), UpdatedAt: ts.Add(time.Second),
	}); err != nil {
		t.Fatalf("upsert turn-14b: %v", err)
	}

	rows, err := repo.ListSubagentContextsByTurn(ctx, "turn-14a")
	if err != nil {
		t.Fatalf("ListSubagentContextsByTurn: %v", err)
	}
	if len(rows) != 1 || rows[0].ToolCallID != "tc-14a" {
		t.Fatalf("turn-14a rows = %+v, want exactly tc-14a", rows)
	}
}

// TestUpsertSubagentContextConcurrentSameKey covers AC-14: concurrent frames
// for the same key produce exactly one row via the atomic upsert statement,
// with no read-then-write race. Run with -race.
func TestUpsertSubagentContextConcurrentSameKey(t *testing.T) {
	repo := newSubagentContextTestRepo(t, "task-15", "session-15", "turn-15")
	ctx := context.Background()
	ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	const concurrency = 8
	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = repo.UpsertSubagentContext(ctx, &models.SubagentContext{
				TaskSessionID: "session-15", TaskID: "task-15", ToolCallID: "tc-15",
				Source: "live", ObservedAt: ts, UpdatedAt: ts.Add(time.Duration(i) * time.Millisecond),
				TotalTokens: i64p(int64(i)),
			})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent upsert %d: %v", i, err)
		}
	}

	rows, err := repo.ListSubagentContextsBySession(ctx, "session-15")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want exactly 1 for concurrent same-key upserts", len(rows))
	}
}

// TestUpsertSubagentContextConcurrentDifferentKeys covers AC-15: concurrent
// frames for different tool_call_ids in one turn (the observed 3-, 4-, 5-,
// and 8-way fan-outs) all land, and neither blocks the other.
func TestUpsertSubagentContextConcurrentDifferentKeys(t *testing.T) {
	for _, fanOut := range []int{3, 4, 5, 8} {
		t.Run(fmt.Sprintf("fanout-%d", fanOut), func(t *testing.T) {
			repo := newSubagentContextTestRepo(t, "task-16", "session-16", "turn-16")
			ctx := context.Background()
			ts := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

			var wg sync.WaitGroup
			errs := make([]error, fanOut)
			for i := 0; i < fanOut; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					errs[i] = repo.UpsertSubagentContext(ctx, &models.SubagentContext{
						TaskSessionID: "session-16", TaskID: "task-16", TurnID: sp("turn-16"),
						ToolCallID: sprintfToolCallID(i), Source: "live", ObservedAt: ts, UpdatedAt: ts,
					})
				}(i)
			}
			wg.Wait()
			for i, err := range errs {
				if err != nil {
					t.Fatalf("concurrent upsert %d: %v", i, err)
				}
			}

			rows, err := repo.ListSubagentContextsBySession(ctx, "session-16")
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(rows) != fanOut {
				t.Fatalf("row count = %d, want %d for a %d-way fan-out", len(rows), fanOut, fanOut)
			}
		})
	}
}

func sprintfToolCallID(i int) string {
	return fmt.Sprintf("tc-fanout-%d", i)
}

// seedTurn inserts an additional turn row for a session already seeded by
// seedForMsgTest (which creates exactly one turn).
func seedTurn(t *testing.T, repo *Repository, turnID, sessionID, taskID string) error {
	t.Helper()
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), turnID, sessionID, taskID, now, now, now)
	return err
}
