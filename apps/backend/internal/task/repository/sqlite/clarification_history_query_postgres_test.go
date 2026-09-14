package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// newInboxHistoryPostgresRepo opens an isolated Postgres schema and mirrors
// newRepoForSessionTests, whose SQLite-only newRepoForSessionTests is what
// every other clarification_history_query_test.go case runs against.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set. It also creates the "ws-1"
// workspace every case below scopes its bundles to: unlike SQLite,
// PostgreSQL enforces the tasks.workspace_id foreign key (see
// seedMetadataCASTask's comment on the same gap), so seedInboxHistorySession
// would otherwise fail before exercising any dialect-sensitive SQL at all.
func newInboxHistoryPostgresRepo(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "ws-1"}); err != nil {
		t.Fatalf("create workspace ws-1: %v", err)
	}
	return repo
}

// TestListInboxHistoryBundlesRealSequenceSupersededVsOperationalPostgres is
// the Postgres counterpart of the SQLite test of the same name (minus the
// suffix): AC .8's regression sequence, and AC .5's read path, both depend on
// dialect-sensitive SQL (dialect.JSONExtract/JSONExtractPath,
// dialect.ExcludeTruthyMetadataPredicate, and the permission-ordering window
// function in inboxHistoryQueryBase) that has never run against Postgres
// syntax before this file.
func TestListInboxHistoryBundlesRealSequenceSupersededVsOperationalPostgres(t *testing.T) {
	repo := newInboxHistoryPostgresRepo(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)

	seedInboxHistorySession(t, repo, "task-superseded-pg", "session-superseded-pg", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-superseded-pg", "session-superseded-pg", "turn-old-pg", base, base)
	createClarificationBundleMessage(t, repo, "clar-old-pg", "task-superseded-pg", "session-superseded-pg", "turn-old-pg", "pending-superseded-pg", "q1", base)

	// Start a new turn without answering the question.
	createPendingActionTurn(t, repo, "task-superseded-pg", "session-superseded-pg", "turn-new-pg", base.Add(time.Minute), base.Add(time.Minute))
	createPendingActionMessage(t, repo, "ordinary-new-pg", "task-superseded-pg", "session-superseded-pg", "turn-new-pg",
		models.MessageTypeMessage, "<missing>", base.Add(time.Minute))

	operational, err := repo.ListUnresolvedClarificationBundles(ctx, models.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: "ws-1", Limit: 50,
	})
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if _, found := findBundleByPendingID(operational.Bundles, "pending-superseded-pg"); found {
		t.Fatal("operational listing returned a superseded bundle")
	}

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	bundle, found := findHistoryBundle(page.Bundles, "pending-superseded-pg")
	if !found {
		t.Fatalf("history listing did not return the superseded bundle: %+v", page.Bundles)
	}
	if bundle.Reason != models.ClarificationHistoryReasonSuperseded {
		t.Fatalf("reason = %q, want superseded", bundle.Reason)
	}
	if bundle.AskingTurnID != "turn-old-pg" {
		t.Fatalf("asking turn = %q, want turn-old-pg", bundle.AskingTurnID)
	}
	if bundle.SupersedingTurnID != "turn-new-pg" {
		t.Fatalf("superseding turn = %q, want turn-new-pg", bundle.SupersedingTurnID)
	}
}

// TestListInboxHistoryBundlesSessionEndedOmitsSupersedingTurnPostgres pins
// the session_ended reason and AC .10's omission rule for it against
// Postgres's terminalSessionStatesExpr branch.
func TestListInboxHistoryBundlesSessionEndedOmitsSupersedingTurnPostgres(t *testing.T) {
	repo := newInboxHistoryPostgresRepo(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)

	seedInboxHistorySession(t, repo, "task-ended-pg", "session-ended-pg", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-ended-pg", "session-ended-pg", "turn-ended-pg", base, base)
	createClarificationBundleMessage(t, repo, "clar-ended-pg", "task-ended-pg", "session-ended-pg", "turn-ended-pg", "pending-ended-pg", "q1", base)

	session, err := repo.GetTaskSession(ctx, "session-ended-pg")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.State = models.TaskSessionStateCompleted
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	bundle, found := findHistoryBundle(page.Bundles, "pending-ended-pg")
	if !found {
		t.Fatalf("history listing did not return the session_ended bundle: %+v", page.Bundles)
	}
	if bundle.Reason != models.ClarificationHistoryReasonSessionEnded {
		t.Fatalf("reason = %q, want session_ended", bundle.Reason)
	}
	if bundle.SupersedingTurnID != "" {
		t.Fatalf("superseding turn = %q, want omitted", bundle.SupersedingTurnID)
	}
}

// TestListInboxHistoryBundlesUnreadableClarificationOnlyPostgres pins that
// unreadable applies only to clarification bundles on Postgres's
// dialect.JSONExtract/dialect.ExcludeTruthyMetadataPredicate branches, the
// same case the SQLite test of the same name (minus the suffix) covers.
func TestListInboxHistoryBundlesUnreadableClarificationOnlyPostgres(t *testing.T) {
	repo := newInboxHistoryPostgresRepo(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 11, 0, 0, 0, time.UTC)

	// Unreadable clarification: current turn, missing question_id, so it's
	// otherwise live except for the missing identifier.
	seedInboxHistorySession(t, repo, "task-unreadable-pg", "session-unreadable-pg", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-unreadable-pg", "session-unreadable-pg", "turn-unreadable-pg", base, base)
	createClarificationBundleMessage(t, repo, "clar-unreadable-pg", "task-unreadable-pg", "session-unreadable-pg", "turn-unreadable-pg", "pending-unreadable-pg", "", base)

	// Live, current-turn, non-terminal-session permission request: must
	// NOT be listed at all (it is live -- none of the three reasons hold).
	seedInboxHistorySession(t, repo, "task-live-perm-pg", "session-live-perm-pg", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-live-perm-pg", "session-live-perm-pg", "turn-live-perm-pg", base, base)
	createInteractionMessage(t, repo, "perm-live-pg", "task-live-perm-pg", "session-live-perm-pg", "turn-live-perm-pg",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-live-perm-pg"}, base)

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	bundle, found := findHistoryBundle(page.Bundles, "pending-unreadable-pg")
	if !found {
		t.Fatalf("history listing did not return the unreadable bundle: %+v", page.Bundles)
	}
	if bundle.Reason != models.ClarificationHistoryReasonUnreadable {
		t.Fatalf("reason = %q, want unreadable", bundle.Reason)
	}
	if _, found := findHistoryBundle(page.Bundles, "pending-live-perm-pg"); found {
		t.Fatal("a live, current-turn, non-terminal-session permission request must not be listed")
	}
}

// TestListInboxHistoryBundlesPermissionSameTurnSupersessionPostgres pins
// AC .2's second superseded clause on Postgres: an older permission request
// superseded by a newer one on the SAME turn is superseded with no separate
// superseding turn to name (AC .10), exercising the
// ROW_NUMBER() OVER (PARTITION BY ... ORDER BY ...) window function that
// inboxHistoryQueryBase builds for permission_not_newest.
func TestListInboxHistoryBundlesPermissionSameTurnSupersessionPostgres(t *testing.T) {
	repo := newInboxHistoryPostgresRepo(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 13, 0, 0, 0, time.UTC)

	seedInboxHistorySession(t, repo, "task-multi-perm-pg", "session-multi-perm-pg", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-multi-perm-pg", "session-multi-perm-pg", "turn-multi-perm-pg", base, base)
	createInteractionMessage(t, repo, "perm-older-pg", "task-multi-perm-pg", "session-multi-perm-pg", "turn-multi-perm-pg",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-perm-older-pg"}, base)
	createInteractionMessage(t, repo, "perm-newer-pg", "task-multi-perm-pg", "session-multi-perm-pg", "turn-multi-perm-pg",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-perm-newer-pg"}, base.Add(time.Second))

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	older, found := findHistoryBundle(page.Bundles, "pending-perm-older-pg")
	if !found {
		t.Fatalf("history listing did not return the superseded permission: %+v", page.Bundles)
	}
	if older.Reason != models.ClarificationHistoryReasonSuperseded {
		t.Fatalf("older permission reason = %q, want superseded", older.Reason)
	}
	if older.SupersedingTurnID != "" {
		t.Fatalf("same-turn superseding turn = %q, want omitted", older.SupersedingTurnID)
	}
	if _, found := findHistoryBundle(page.Bundles, "pending-perm-newer-pg"); found {
		t.Fatal("the newest permission on the current turn is live and must not be listed")
	}
}

// TestCountInboxHistoryBundlesIsWorkspaceWideTotalPostgres pins AC .20's
// total on Postgres: independent of page size, same as the SQLite test of
// the same name (minus the suffix).
func TestCountInboxHistoryBundlesIsWorkspaceWideTotalPostgres(t *testing.T) {
	repo := newInboxHistoryPostgresRepo(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 16, 0, 0, 0, time.UTC)

	for i, suffix := range []string{"a", "b", "c"} {
		taskID, sessionID, turnID := "task-count-pg-"+suffix, "session-count-pg-"+suffix, "turn-count-pg-"+suffix
		seedInboxHistorySession(t, repo, taskID, sessionID, "ws-1", models.TaskSessionStateCompleted, false)
		createdAt := base.Add(time.Duration(i) * time.Minute)
		createPendingActionTurn(t, repo, taskID, sessionID, turnID, createdAt, createdAt)
		createClarificationBundleMessage(t, repo, "clar-count-pg-"+suffix, taskID, sessionID, turnID, "pending-count-pg-"+suffix, "q1", createdAt)
	}

	total, err := repo.CountInboxHistoryBundles(ctx, models.ListClarificationHistoryOptions{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("CountInboxHistoryBundles: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 1))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	if len(page.Bundles) != 1 {
		t.Fatalf("page bundles = %d, want 1 (total must stay independent of page size)", len(page.Bundles))
	}
}
