package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seedFailureAgent inserts an agent_profiles row with the failure-handling
// columns set explicitly, so the inbox queries have something to join to.
func seedFailureAgent(
	t *testing.T, db *sqlx.DB,
	id, workspaceID, name, pauseReason string, consecutiveFailures, threshold int, deleted bool,
) {
	t.Helper()
	now := time.Now().UTC()
	var deletedAt interface{}
	if deleted {
		deletedAt = now
	}
	if _, err := db.Exec(`INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, workspace_id, role,
		 pause_reason, consecutive_failures, failure_threshold, deleted_at,
		 created_at, updated_at)
		VALUES (?, 'agent', ?, ?, ?, '', ?, ?, ?, ?, ?, ?)`,
		id, name, name, workspaceID, pauseReason, consecutiveFailures, threshold,
		deletedAt, now, now); err != nil {
		t.Fatalf("seed agent_profile %s: %v", id, err)
	}
}

func TestIncrementAndResetAgentConsecutiveFailures(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedFailureAgent(t, db, "agent-a", "ws-1", "A", "", 0, 0, false)
	seedFailureAgent(t, db, "agent-b", "ws-1", "B", "", 0, 0, false)

	for want := 1; want <= 3; want++ {
		got, err := repo.IncrementAgentConsecutiveFailures(ctx, "agent-a")
		if err != nil {
			t.Fatalf("increment #%d: %v", want, err)
		}
		if got != want {
			t.Fatalf("increment #%d returned %d, want the post-increment value %d", want, got, want)
		}
	}

	// The other agent's counter must not move.
	other, err := repo.GetEffectiveFailureThreshold(ctx, "agent-b")
	if err != nil {
		t.Fatalf("threshold for agent-b: %v", err)
	}
	if other != sqlite.DefaultAgentFailureThreshold {
		t.Errorf("agent-b threshold = %d, want default", other)
	}
	var bFailures int
	if err := db.Get(&bFailures,
		`SELECT consecutive_failures FROM agent_profiles WHERE id = 'agent-b'`); err != nil {
		t.Fatalf("read agent-b failures: %v", err)
	}
	if bFailures != 0 {
		t.Errorf("agent-b consecutive_failures = %d, want 0", bFailures)
	}

	if err := repo.ResetAgentConsecutiveFailures(ctx, "agent-a"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	var aFailures int
	if err := db.Get(&aFailures,
		`SELECT consecutive_failures FROM agent_profiles WHERE id = 'agent-a'`); err != nil {
		t.Fatalf("read agent-a failures: %v", err)
	}
	if aFailures != 0 {
		t.Errorf("consecutive_failures after reset = %d, want 0", aFailures)
	}

	// A reset then increment starts the count over from 1.
	n, err := repo.IncrementAgentConsecutiveFailures(ctx, "agent-a")
	if err != nil {
		t.Fatalf("increment after reset: %v", err)
	}
	if n != 1 {
		t.Errorf("increment after reset = %d, want 1", n)
	}
}

func TestIncrementAgentConsecutiveFailures_MissingAgent(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)

	// The UPDATE matches nothing, so the follow-up SELECT has no row.
	_, err := repo.IncrementAgentConsecutiveFailures(context.Background(), "ghost")
	if err == nil {
		t.Fatal("increment on a missing agent = nil error, want sql.ErrNoRows")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("error = %v, want sql.ErrNoRows", err)
	}
}

func TestGetEffectiveFailureThreshold_PrecedenceChain(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedFailureAgent(t, db, "agent-override", "ws-1", "Override", "", 0, 9, false)
	seedFailureAgent(t, db, "agent-inherit", "ws-1", "Inherit", "", 0, 0, false)
	seedFailureAgent(t, db, "agent-no-ws", "", "NoWorkspace", "", 0, 0, false)

	// No workspace row yet → the global default.
	got, err := repo.GetEffectiveFailureThreshold(ctx, "agent-inherit")
	if err != nil {
		t.Fatalf("threshold (no workspace row): %v", err)
	}
	if got != sqlite.DefaultAgentFailureThreshold {
		t.Errorf("got %d, want the global default %d", got, sqlite.DefaultAgentFailureThreshold)
	}

	if err := repo.SetWorkspaceAgentFailureThreshold(ctx, "ws-1", 5); err != nil {
		t.Fatalf("set workspace threshold: %v", err)
	}

	// Workspace override now applies to the agent with no per-agent value.
	got, err = repo.GetEffectiveFailureThreshold(ctx, "agent-inherit")
	if err != nil {
		t.Fatalf("threshold (workspace override): %v", err)
	}
	if got != 5 {
		t.Errorf("got %d, want the workspace override 5", got)
	}

	// The per-agent override wins over the workspace value.
	got, err = repo.GetEffectiveFailureThreshold(ctx, "agent-override")
	if err != nil {
		t.Fatalf("threshold (agent override): %v", err)
	}
	if got != 9 {
		t.Errorf("got %d, want the per-agent override 9", got)
	}

	// A workspace-less agent falls through to the '' workspace, which has
	// no settings row, so the global default applies.
	got, err = repo.GetEffectiveFailureThreshold(ctx, "agent-no-ws")
	if err != nil {
		t.Fatalf("threshold (no workspace): %v", err)
	}
	if got != sqlite.DefaultAgentFailureThreshold {
		t.Errorf("got %d, want the global default", got)
	}

	if _, err := repo.GetEffectiveFailureThreshold(ctx, "ghost"); err == nil {
		t.Error("threshold for a missing agent = nil error, want sql.ErrNoRows")
	} else if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("error = %v, want sql.ErrNoRows", err)
	}
}

func TestWorkspaceAgentFailureThreshold_UpsertAndClamp(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	got, err := repo.GetWorkspaceAgentFailureThreshold(ctx, "ws-none")
	if err != nil {
		t.Fatalf("get (missing row): %v", err)
	}
	if got != sqlite.DefaultAgentFailureThreshold {
		t.Errorf("missing row = %d, want the default", got)
	}

	if err := repo.SetWorkspaceAgentFailureThreshold(ctx, "ws-1", 7); err != nil {
		t.Fatalf("set 7: %v", err)
	}
	if got, err = repo.GetWorkspaceAgentFailureThreshold(ctx, "ws-1"); err != nil || got != 7 {
		t.Fatalf("get after set = %d, %v; want 7, nil", got, err)
	}

	// Second write on the same workspace updates in place (ON CONFLICT).
	if err := repo.SetWorkspaceAgentFailureThreshold(ctx, "ws-1", 11); err != nil {
		t.Fatalf("set 11: %v", err)
	}
	if got, err = repo.GetWorkspaceAgentFailureThreshold(ctx, "ws-1"); err != nil || got != 11 {
		t.Fatalf("get after upsert = %d, %v; want 11, nil", got, err)
	}
	var rowCount int
	if err := repo.ReaderDB().Get(&rowCount,
		`SELECT COUNT(*) FROM office_workspace_settings WHERE workspace_id = 'ws-1'`); err != nil {
		t.Fatalf("count settings rows: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("settings rows = %d, want 1 (upsert, not insert)", rowCount)
	}

	// A non-positive threshold is normalised to the default on write…
	if err := repo.SetWorkspaceAgentFailureThreshold(ctx, "ws-1", 0); err != nil {
		t.Fatalf("set 0: %v", err)
	}
	if got, err = repo.GetWorkspaceAgentFailureThreshold(ctx, "ws-1"); err != nil ||
		got != sqlite.DefaultAgentFailureThreshold {
		t.Fatalf("get after set 0 = %d, %v; want the default", got, err)
	}

	// …and a non-positive value already in the column is normalised on read.
	if _, err := repo.ExecRaw(ctx,
		`UPDATE office_workspace_settings SET agent_failure_threshold = -4 WHERE workspace_id = 'ws-1'`,
	); err != nil {
		t.Fatalf("force negative threshold: %v", err)
	}
	if got, err = repo.GetWorkspaceAgentFailureThreshold(ctx, "ws-1"); err != nil ||
		got != sqlite.DefaultAgentFailureThreshold {
		t.Fatalf("get with negative column = %d, %v; want the default", got, err)
	}
}

func TestMarkRunFailed_StampsMessageAndPreservesFinishedAt(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedSummaryRun(t, repo, "mr-open", "agent-a", "claimed", "2026-01-01 09:00:00", "2026-01-01 10:00:00", "", "{}")
	seedSummaryRun(t, repo, "mr-done", "agent-a", "finished",
		"2026-01-02 09:00:00", "2026-01-02 10:00:00", "2026-01-02 11:00:00", "{}")
	seedSummaryRun(t, repo, "mr-other", "agent-a", "queued", "2026-01-03 09:00:00", "", "", "{}")

	if err := repo.MarkRunFailed(ctx, "mr-open", "provider exploded"); err != nil {
		t.Fatalf("MarkRunFailed: %v", err)
	}
	run, err := repo.GetRun(ctx, "mr-open")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != "failed" {
		t.Errorf("Status = %q, want failed", run.Status)
	}
	if run.ErrorMessage != "provider exploded" {
		t.Errorf("ErrorMessage = %q, want the verbatim message", run.ErrorMessage)
	}
	if run.FinishedAt == nil {
		t.Error("FinishedAt is nil, want it stamped")
	}

	// Re-marking is idempotent and must not move an existing finished_at.
	if err := repo.MarkRunFailed(ctx, "mr-done", "later failure"); err != nil {
		t.Fatalf("MarkRunFailed (already finished): %v", err)
	}
	run, err = repo.GetRun(ctx, "mr-done")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.FinishedAt == nil || run.FinishedAt.Format("2006-01-02 15:04:05") != "2026-01-02 11:00:00" {
		t.Errorf("FinishedAt = %v, want the original 2026-01-02 11:00:00", run.FinishedAt)
	}
	if run.ErrorMessage != "later failure" {
		t.Errorf("ErrorMessage = %q, want it overwritten", run.ErrorMessage)
	}

	// An untouched run keeps its status.
	if run, err = repo.GetRun(ctx, "mr-other"); err != nil {
		t.Fatalf("GetRun(mr-other): %v", err)
	}
	if run.Status != "queued" {
		t.Errorf("mr-other status = %q, want queued", run.Status)
	}

	// Marking a missing run is a silent no-op, not an error.
	if err := repo.MarkRunFailed(ctx, "no-such-run", "x"); err != nil {
		t.Errorf("MarkRunFailed(missing) = %v, want nil", err)
	}
}

func TestInboxDismissals_UpsertQueryAndList(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	dismissed, err := repo.IsInboxItemDismissed(ctx, "user-1", "agent_run_failed", "run-1")
	if err != nil {
		t.Fatalf("IsInboxItemDismissed: %v", err)
	}
	if dismissed {
		t.Fatal("item reported dismissed before any dismissal was recorded")
	}

	if err := repo.DismissInboxItem(ctx, "user-1", "agent_run_failed", "run-1"); err != nil {
		t.Fatalf("DismissInboxItem: %v", err)
	}
	if dismissed, err = repo.IsInboxItemDismissed(ctx, "user-1", "agent_run_failed", "run-1"); err != nil ||
		!dismissed {
		t.Fatalf("IsInboxItemDismissed = %v, %v; want true, nil", dismissed, err)
	}

	// Each part of the natural key is significant.
	for _, tc := range []struct{ user, kind, item string }{
		{"user-2", "agent_run_failed", "run-1"},
		{"user-1", "agent_paused_after_failures", "run-1"},
		{"user-1", "agent_run_failed", "run-2"},
	} {
		got, err := repo.IsInboxItemDismissed(ctx, tc.user, tc.kind, tc.item)
		if err != nil {
			t.Fatalf("IsInboxItemDismissed(%v): %v", tc, err)
		}
		if got {
			t.Errorf("IsInboxItemDismissed(%s, %s, %s) = true, want false", tc.user, tc.kind, tc.item)
		}
	}

	// Second dismiss of the same triple is accepted and stays one row.
	if err := repo.DismissInboxItem(ctx, "user-1", "agent_run_failed", "run-1"); err != nil {
		t.Fatalf("second DismissInboxItem: %v", err)
	}
	var count int
	if err := repo.ReaderDB().Get(&count, `SELECT COUNT(*) FROM office_inbox_dismissals`); err != nil {
		t.Fatalf("count dismissals: %v", err)
	}
	if count != 1 {
		t.Errorf("dismissal rows = %d, want 1 (idempotent upsert)", count)
	}

	if err := repo.DismissInboxItem(ctx, "user-1", "agent_run_failed", "run-3"); err != nil {
		t.Fatalf("DismissInboxItem run-3: %v", err)
	}
	if err := repo.DismissInboxItem(ctx, "user-1", "agent_paused_after_failures", "agent-x"); err != nil {
		t.Fatalf("DismissInboxItem paused: %v", err)
	}
	if err := repo.DismissInboxItem(ctx, "user-2", "agent_run_failed", "run-9"); err != nil {
		t.Fatalf("DismissInboxItem other user: %v", err)
	}

	ids, err := repo.ListDismissedInboxItemIDs(ctx, "user-1", "agent_run_failed")
	if err != nil {
		t.Fatalf("ListDismissedInboxItemIDs: %v", err)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if len(ids) != 2 || !got["run-1"] || !got["run-3"] {
		t.Errorf("ids = %v, want exactly run-1 and run-3 (kind- and user-scoped)", ids)
	}

	empty, err := repo.ListDismissedInboxItemIDs(ctx, "user-1", "no_such_kind")
	if err != nil {
		t.Fatalf("ListDismissedInboxItemIDs(unknown kind): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ids = %v, want none", empty)
	}
}

func TestListFailedRunsForInbox(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedFailureAgent(t, db, "agent-live", "ws-1", "Live Agent", "", 2, 0, false)
	seedFailureAgent(t, db, "agent-paused", "ws-1", "Paused Agent", "Auto-paused: too many failures", 3, 0, false)
	seedFailureAgent(t, db, "agent-manual", "ws-1", "Manual Pause", "user paused me", 0, 0, false)
	seedFailureAgent(t, db, "agent-elsewhere", "ws-2", "Other WS", "", 0, 0, false)

	seedSummaryRun(t, repo, "f-old", "agent-live", "failed",
		"2026-01-01 09:00:00", "", "2026-01-01 10:00:00", `{"task_id":"t-1"}`)
	seedSummaryRun(t, repo, "f-new", "agent-live", "failed",
		"2026-01-02 09:00:00", "", "2026-01-02 10:00:00", `{"task_id":"t-2"}`)
	// No payload task_id → COALESCE to "".
	seedSummaryRun(t, repo, "f-notask", "agent-manual", "failed",
		"2026-01-03 09:00:00", "", "2026-01-03 10:00:00", `{}`)
	// Excluded: not failed / auto-paused agent / other workspace.
	seedSummaryRun(t, repo, "f-ok", "agent-live", "finished",
		"2026-01-04 09:00:00", "", "2026-01-04 10:00:00", `{}`)
	seedSummaryRun(t, repo, "f-autopaused", "agent-paused", "failed",
		"2026-01-05 09:00:00", "", "2026-01-05 10:00:00", `{}`)
	seedSummaryRun(t, repo, "f-otherws", "agent-elsewhere", "failed",
		"2026-01-06 09:00:00", "", "2026-01-06 10:00:00", `{}`)
	if _, err := repo.ExecRaw(ctx,
		`UPDATE runs SET error_message = 'kaboom' WHERE id = 'f-new'`); err != nil {
		t.Fatalf("set error message: %v", err)
	}

	rows, err := repo.ListFailedRunsForInbox(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatalf("ListFailedRunsForInbox: %v", err)
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RunID)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %v, want f-notask, f-new, f-old", ids)
	}
	// ORDER BY failed_at DESC.
	if ids[0] != "f-notask" || ids[1] != "f-new" || ids[2] != "f-old" {
		t.Fatalf("order = %v, want f-notask, f-new, f-old", ids)
	}
	newest := rows[1]
	if newest.AgentProfileID != "agent-live" {
		t.Errorf("AgentProfileID = %q, want agent-live", newest.AgentProfileID)
	}
	if newest.AgentName != "Live Agent" {
		t.Errorf("AgentName = %q, want 'Live Agent' from the join", newest.AgentName)
	}
	if newest.WorkspaceID != "ws-1" {
		t.Errorf("WorkspaceID = %q, want ws-1", newest.WorkspaceID)
	}
	if newest.TaskID != "t-2" {
		t.Errorf("TaskID = %q, want t-2 extracted from the payload", newest.TaskID)
	}
	if newest.ErrorMessage != "kaboom" {
		t.Errorf("ErrorMessage = %q, want kaboom", newest.ErrorMessage)
	}
	if newest.FailedAtRaw != "2026-01-02 10:00:00" {
		t.Errorf("FailedAtRaw = %q, want the stored finished_at", newest.FailedAtRaw)
	}
	if newest.FailedAt.Format(time.RFC3339) != "2026-01-02T10:00:00Z" {
		t.Errorf("FailedAt = %v, want the parsed 2026-01-02T10:00:00Z", newest.FailedAt)
	}
	if rows[0].TaskID != "" {
		t.Errorf("payload without task_id → TaskID = %q, want empty", rows[0].TaskID)
	}

	// A user dismissal hides the row for that user only.
	if err := repo.DismissInboxItem(ctx, "user-1", "agent_run_failed", "f-new"); err != nil {
		t.Fatalf("dismiss f-new: %v", err)
	}
	rows, err = repo.ListFailedRunsForInbox(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatalf("ListFailedRunsForInbox after dismiss: %v", err)
	}
	for _, row := range rows {
		if row.RunID == "f-new" {
			t.Fatal("dismissed run still returned for the dismissing user")
		}
	}
	rows, err = repo.ListFailedRunsForInbox(ctx, "ws-1", "user-2")
	if err != nil {
		t.Fatalf("ListFailedRunsForInbox other user: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("other user rows = %d, want 3 — a dismissal is per-user", len(rows))
	}

	// The '_auto' sentinel hides the row from everyone.
	if err := repo.DismissInboxItem(ctx, "_auto", "agent_run_failed", "f-old"); err != nil {
		t.Fatalf("auto-dismiss f-old: %v", err)
	}
	rows, err = repo.ListFailedRunsForInbox(ctx, "ws-1", "user-2")
	if err != nil {
		t.Fatalf("ListFailedRunsForInbox after auto-dismiss: %v", err)
	}
	for _, row := range rows {
		if row.RunID == "f-old" {
			t.Fatal("_auto dismissal did not hide the run from other users")
		}
	}
}

func TestListFailedRunsForInbox_FallsBackToRequestedAt(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedFailureAgent(t, db, "agent-live", "ws-1", "Live", "", 0, 0, false)
	seedSummaryRun(t, repo, "f-nofinish", "agent-live", "failed", "2026-03-04 05:06:07", "", "", "{}")

	rows, err := repo.ListFailedRunsForInbox(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatalf("ListFailedRunsForInbox: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].FailedAtRaw != "2026-03-04 05:06:07" {
		t.Errorf("FailedAtRaw = %q, want the requested_at fallback", rows[0].FailedAtRaw)
	}
	if rows[0].FailedAt.Format(time.RFC3339) != "2026-03-04T05:06:07Z" {
		t.Errorf("FailedAt = %v, want the parsed fallback", rows[0].FailedAt)
	}
}

// An unparseable timestamp must degrade to the zero time rather than
// failing the whole inbox query.
func TestListFailedRunsForInbox_UnparseableTimestampIsZeroTime(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedFailureAgent(t, db, "agent-live", "ws-1", "Live", "", 0, 0, false)
	seedSummaryRun(t, repo, "f-bad", "agent-live", "failed", "2026-01-01 09:00:00", "", "", "{}")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE runs SET finished_at = 'not-a-timestamp' WHERE id = 'f-bad'`); err != nil {
		t.Fatalf("corrupt finished_at: %v", err)
	}

	rows, err := repo.ListFailedRunsForInbox(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatalf("ListFailedRunsForInbox: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].FailedAtRaw != "not-a-timestamp" {
		t.Errorf("FailedAtRaw = %q, want the raw value preserved", rows[0].FailedAtRaw)
	}
	if !rows[0].FailedAt.IsZero() {
		t.Errorf("FailedAt = %v, want the zero time", rows[0].FailedAt)
	}
}

func TestListAutoPausedAgentsForInbox(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	seedFailureAgent(t, db, "ap-1", "ws-1", "Auto One", "Auto-paused: 3 consecutive failures", 3, 0, false)
	seedFailureAgent(t, db, "ap-2", "ws-1", "Auto Two", "Auto-paused: provider down", 5, 0, false)
	seedFailureAgent(t, db, "manual", "ws-1", "Manual", "user paused", 1, 0, false)
	seedFailureAgent(t, db, "healthy", "ws-1", "Healthy", "", 0, 0, false)
	seedFailureAgent(t, db, "deleted", "ws-1", "Deleted", "Auto-paused: gone", 9, 0, true)
	seedFailureAgent(t, db, "otherws", "ws-2", "Other", "Auto-paused: elsewhere", 4, 0, false)
	seedFailureAgent(t, db, "nows", "", "No Workspace", "Auto-paused: unscoped", 4, 0, false)

	// Deterministic ordering: updated_at DESC.
	if _, err := repo.ExecRaw(ctx,
		`UPDATE agent_profiles SET updated_at = '2026-01-01 10:00:00' WHERE id = 'ap-1'`); err != nil {
		t.Fatalf("stamp ap-1: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`UPDATE agent_profiles SET updated_at = '2026-01-02 10:00:00' WHERE id = 'ap-2'`); err != nil {
		t.Fatalf("stamp ap-2: %v", err)
	}

	rows, err := repo.ListAutoPausedAgentsForInbox(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatalf("ListAutoPausedAgentsForInbox: %v", err)
	}
	if len(rows) != 2 {
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.AgentID)
		}
		t.Fatalf("rows = %v, want ap-2 and ap-1 only", ids)
	}
	if rows[0].AgentID != "ap-2" || rows[1].AgentID != "ap-1" {
		t.Fatalf("order = %q,%q, want ap-2,ap-1 (updated_at DESC)", rows[0].AgentID, rows[1].AgentID)
	}
	if rows[0].AgentName != "Auto Two" {
		t.Errorf("AgentName = %q, want 'Auto Two'", rows[0].AgentName)
	}
	if rows[0].WorkspaceID != "ws-1" {
		t.Errorf("WorkspaceID = %q, want ws-1", rows[0].WorkspaceID)
	}
	if rows[0].PauseReason != "Auto-paused: provider down" {
		t.Errorf("PauseReason = %q, want the stored reason", rows[0].PauseReason)
	}
	if rows[0].ConsecutiveFailures != 5 {
		t.Errorf("ConsecutiveFailures = %d, want 5", rows[0].ConsecutiveFailures)
	}
	if rows[0].UpdatedAt.Format("2006-01-02 15:04:05") != "2026-01-02 10:00:00" {
		t.Errorf("UpdatedAt = %v, want 2026-01-02 10:00:00", rows[0].UpdatedAt)
	}

	if err := repo.DismissInboxItem(ctx, "_auto", "agent_paused_after_failures", "ap-2"); err != nil {
		t.Fatalf("dismiss ap-2: %v", err)
	}
	rows, err = repo.ListAutoPausedAgentsForInbox(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatalf("ListAutoPausedAgentsForInbox after dismiss: %v", err)
	}
	if len(rows) != 1 || rows[0].AgentID != "ap-1" {
		t.Errorf("rows = %+v, want only ap-1", rows)
	}
}

func TestListFailedRunsForAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedSummaryRun(t, repo, "fa-1", "agent-a", "failed",
		"2026-01-01 09:00:00", "", "2026-01-01 10:00:00", "{}")
	seedSummaryRun(t, repo, "fa-2", "agent-a", "failed",
		"2026-01-02 09:00:00", "", "2026-01-02 10:00:00", "{}")
	seedSummaryRun(t, repo, "fa-ok", "agent-a", "finished",
		"2026-01-03 09:00:00", "", "2026-01-03 10:00:00", "{}")
	seedSummaryRun(t, repo, "fa-other", "agent-b", "failed",
		"2026-01-04 09:00:00", "", "2026-01-04 10:00:00", "{}")

	ids, err := repo.ListFailedRunsForAgent(ctx, "agent-a")
	if err != nil {
		t.Fatalf("ListFailedRunsForAgent: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want the two failed runs for agent-a", ids)
	}
	if ids[0] != "fa-2" || ids[1] != "fa-1" {
		t.Errorf("order = %v, want fa-2,fa-1 (finished_at DESC)", ids)
	}

	none, err := repo.ListFailedRunsForAgent(ctx, "agent-clean")
	if err != nil {
		t.Fatalf("ListFailedRunsForAgent(clean): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ids = %v, want none", none)
	}
}

func TestHasPriorTasklessFailedRun(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// A taskless failed run for agent-a — this is the "first" one.
	seedSummaryRun(t, repo, "tl-1", "agent-a", "failed",
		"2026-01-01 09:00:00", "", "2026-01-01 10:00:00", "{}")

	// Before any prior failure exists (excluding itself), the answer is false.
	has, err := repo.HasPriorTasklessFailedRun(ctx, "agent-a", "agent:agent-a", "tl-1")
	if err != nil {
		t.Fatalf("HasPriorTasklessFailedRun: %v", err)
	}
	if has {
		t.Error("has = true, want false — tl-1 is its own only taskless failure")
	}

	// A second taskless failure now sees tl-1 as a prior one.
	seedSummaryRun(t, repo, "tl-2", "agent-a", "failed",
		"2026-01-02 09:00:00", "", "2026-01-02 10:00:00", "{}")
	has, err = repo.HasPriorTasklessFailedRun(ctx, "agent-a", "agent:agent-a", "tl-2")
	if err != nil {
		t.Fatalf("HasPriorTasklessFailedRun (second): %v", err)
	}
	if !has {
		t.Error("has = false, want true — tl-1 is a prior taskless failure")
	}

	// A failed run that does carry a task_id doesn't count as taskless.
	seedSummaryRun(t, repo, "tl-task", "agent-b", "failed",
		"2026-01-01 09:00:00", "", "2026-01-01 10:00:00", `{"task_id":"t-1"}`)
	seedSummaryRun(t, repo, "tl-b", "agent-b", "failed",
		"2026-01-02 09:00:00", "", "2026-01-02 10:00:00", "{}")
	has, err = repo.HasPriorTasklessFailedRun(ctx, "agent-b", "agent:agent-b", "tl-b")
	if err != nil {
		t.Fatalf("HasPriorTasklessFailedRun (task-scoped prior excluded): %v", err)
	}
	if has {
		t.Error("has = true, want false — the only other failed run for agent-b carries a task_id")
	}

	// A non-failed taskless run doesn't count.
	seedSummaryRun(t, repo, "tl-c1", "agent-c", "finished",
		"2026-01-01 09:00:00", "", "2026-01-01 10:00:00", "{}")
	seedSummaryRun(t, repo, "tl-c2", "agent-c", "failed",
		"2026-01-02 09:00:00", "", "2026-01-02 10:00:00", "{}")
	has, err = repo.HasPriorTasklessFailedRun(ctx, "agent-c", "agent:agent-c", "tl-c2")
	if err != nil {
		t.Fatalf("HasPriorTasklessFailedRun (non-failed prior excluded): %v", err)
	}
	if has {
		t.Error("has = true, want false — the only other run for agent-c isn't failed")
	}
}

func TestHasPriorTasklessFailedRun_IsolatedByContinuationScope(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedSummaryRunWithScope(t, repo, "scope-a-1", "agent-a", "failed",
		"2026-01-01 09:00:00", "", "2026-01-01 10:00:00", "{}", "routine:routine-a")

	// A failure from routine-a must not hide the first failure from routine-b.
	has, err := repo.HasPriorTasklessFailedRun(ctx, "agent-a", "routine:routine-b", "scope-b-1")
	if err != nil {
		t.Fatalf("HasPriorTasklessFailedRun (other routine): %v", err)
	}
	if has {
		t.Error("has = true, want false — a different routine scope must not count")
	}

	seedSummaryRunWithScope(t, repo, "scope-b-1", "agent-a", "failed",
		"2026-01-02 09:00:00", "", "2026-01-02 10:00:00", "{}", "routine:routine-b")
	has, err = repo.HasPriorTasklessFailedRun(ctx, "agent-a", "routine:routine-b", "scope-b-1")
	if err != nil {
		t.Fatalf("HasPriorTasklessFailedRun (same routine): %v", err)
	}
	if has {
		t.Error("has = true, want false — scope-b-1 is its own only failure")
	}

	seedSummaryRunWithScope(t, repo, "scope-b-2", "agent-a", "failed",
		"2026-01-03 09:00:00", "", "2026-01-03 10:00:00", "{}", "routine:routine-b")
	has, err = repo.HasPriorTasklessFailedRun(ctx, "agent-a", "routine:routine-b", "scope-b-2")
	if err != nil {
		t.Fatalf("HasPriorTasklessFailedRun (repeat): %v", err)
	}
	if !has {
		t.Error("has = false, want true — scope-b-1 is a prior failure in routine-b")
	}
}

func seedSummaryRunWithScope(
	t *testing.T, repo *sqlite.Repository,
	id, agentID, status, requestedAt, claimedAt, finishedAt, payload, scope string,
) {
	t.Helper()
	seedSummaryRun(t, repo, id, agentID, status, requestedAt, claimedAt, finishedAt, payload)
	if _, err := repo.ExecRaw(context.Background(),
		`UPDATE runs SET continuation_scope = ? WHERE id = ?`, scope, id); err != nil {
		t.Fatalf("set continuation scope for %s: %v", id, err)
	}
}

func TestGetRun_MissingRowReturnsNoRows(t *testing.T) {
	repo := newTestRepo(t)

	got, err := repo.GetRun(context.Background(), "no-such-run")
	if err == nil {
		t.Fatal("GetRun(missing) = nil error, want sql.ErrNoRows")
	}
	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("error = %v, want sql.ErrNoRows", err)
	}
}
