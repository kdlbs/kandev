package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func seedGitSnapshotSession(t *testing.T, repo *Repository, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-" + taskID, Name: "Workspace " + taskID}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + taskID, WorkspaceID: "ws-" + taskID, Name: "Workflow " + taskID}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-" + taskID, WorkflowID: "wf-" + taskID,
		WorkflowStepID: "step-" + taskID, Title: taskID, Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}

// TestGetGitSnapshotsBySession_OrdersByRecencyRegardlessOfTriggeredBy
// replaces a prior version of this test that asserted the opposite ordering
// (agent_completed always sorts first, regardless of actual recency) as
// correct. That ordering IS the session-delete pre-delete-warning defect
// documented in docs/specs/session-delete-resource-cleanup REVIEW-ROUND: 2:
// an agent turn completes (authoritative "agent_completed" snapshot with
// full file-diff data), then the user hand-edits a file outside the agent
// (a later, genuinely fresher "live_monitor" tick, which never populates
// Files by design — see UpsertLatestLiveGitSnapshot). A consumer that reads
// "the newest snapshot" — the session-delete warning's session.git.snapshots
// consumer — must see the newer row, not have a stale-but-"authoritative"
// row shadow it and silently undercount real uncommitted work to zero.
// Callers that need the newer row's file count when its Files column is
// empty fall back to its Metadata's modified/added/deleted/untracked/renamed
// lists (see apps/web/hooks/use-session-delete-warning.ts), which
// saveGitStatusSnapshot and UpsertLatestLiveGitSnapshot both always persist
// regardless of triggered_by — so recency-first ordering no longer trades
// away file-count accuracy the way it would have before that fallback
// existed.
func TestGetGitSnapshotsBySession_OrdersByRecencyRegardlessOfTriggeredBy(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	const taskID = "task-snapshot-recency"
	const sessionID = "session-snapshot-recency"
	seedGitSnapshotSession(t, repo, taskID, sessionID)

	authoritativeAt := time.Now().UTC()
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch: "feature/recency", Ahead: 1,
		Files:       map[string]interface{}{"diff_update_test.txt": map[string]interface{}{"status": "modified"}},
		TriggeredBy: "agent_completed",
		CreatedAt:   authoritativeAt,
	}); err != nil {
		t.Fatalf("create authoritative snapshot: %v", err)
	}

	// A live_monitor row lands strictly AFTER the authoritative one — e.g. the
	// user hand-edited a file via the terminal after the turn completed. It
	// carries no Files data (UpsertLatestLiveGitSnapshot never populates it)
	// but is the genuinely newer state and must sort first.
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, Branch: "feature/recency", Ahead: 1,
		CreatedAt: authoritativeAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create newer live_monitor snapshot: %v", err)
	}

	snapshots, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 5)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(snapshots))
	}

	newest := snapshots[0]
	if newest.TriggeredBy != TriggeredByLiveMonitor {
		t.Fatalf("newest snapshot triggered_by = %q, want %q (the genuinely newer row must sort first)",
			newest.TriggeredBy, TriggeredByLiveMonitor)
	}
	oldest := snapshots[1]
	if oldest.TriggeredBy != "agent_completed" {
		t.Fatalf("oldest snapshot triggered_by = %q, want agent_completed", oldest.TriggeredBy)
	}
}

// TestGetGitSnapshotsBySession_OrdersByRecencyRegardlessOfTriggeredBy_InverseArrangement
// is the sibling of the test above, seeding the opposite arrangement: a
// genuinely NEWER agent_completed row against an OLDER live_monitor row.
// Without this inverse case, "sorts by recency" is indistinguishable from
// "always prefers live_monitor" or "no ordering at all" — both wrong
// implementations pass the single-arrangement test above (mutation-proven in
// docs/specs/session-delete-resource-cleanup REVIEW-ROUND: 3: dropping the
// ORDER BY clause entirely, or replacing it with `ORDER BY triggered_by
// DESC`, both left the sibling test green). This test only passes when the
// query is genuinely ordering by created_at, in either direction.
func TestGetGitSnapshotsBySession_OrdersByRecencyRegardlessOfTriggeredBy_InverseArrangement(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	const taskID = "task-snapshot-recency-inverse"
	const sessionID = "session-snapshot-recency-inverse"
	seedGitSnapshotSession(t, repo, taskID, sessionID)

	staleAt := time.Now().UTC()
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, Branch: "feature/recency-inverse", Ahead: 1,
		CreatedAt: staleAt,
	}); err != nil {
		t.Fatalf("create older live_monitor snapshot: %v", err)
	}

	// A fresh agent turn completes strictly AFTER the live_monitor tick above
	// (e.g. the agent kept working after the periodic poll fired) and must
	// sort first — the newer row wins regardless of which triggered_by it
	// carries.
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch: "feature/recency-inverse", Ahead: 2,
		Files:       map[string]interface{}{"newer.ts": map[string]interface{}{"status": "modified"}},
		TriggeredBy: "agent_completed",
		CreatedAt:   staleAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create newer agent_completed snapshot: %v", err)
	}

	snapshots, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 5)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(snapshots))
	}

	newest := snapshots[0]
	if newest.TriggeredBy != "agent_completed" {
		t.Fatalf("newest snapshot triggered_by = %q, want agent_completed (the genuinely newer row must sort first)",
			newest.TriggeredBy)
	}
	oldest := snapshots[1]
	if oldest.TriggeredBy != TriggeredByLiveMonitor {
		t.Fatalf("oldest snapshot triggered_by = %q, want %q", oldest.TriggeredBy, TriggeredByLiveMonitor)
	}
}

// TestGetGitSnapshotsBySession_TiebreaksIdenticalTimestampsStably closes the
// round-4 test-rigor gap: the two recency tests above deliberately separate
// their rows by a full minute, so neither exercises the `, id DESC`
// secondary key added by REVIEW-ROUND: 3's fix — mutation-proven in
// docs/specs/session-delete-resource-cleanup REVIEW-ROUND: 4 (dropping `, id
// DESC`, or flipping it to `id ASC`, both left the whole suite green).
//
// `id` is a random UUIDv4 (see CreateGitSnapshot), not a monotonic column, so
// this test does NOT assert that the tiebreak picks the "newer" row on an
// exact tie — that guarantee is not available without a dialect-specific
// rowid/sequence column this repository's SQLite+Postgres portability rule
// rules out (the same tradeoff already made, and documented, for
// UpdateTaskSessionLastReadMessageID's id-based tiebreak in session.go).
// What the query DOES guarantee, and what this test pins, is that a tie
// resolves deterministically: the same pair of rows returns in the same
// order on every call, and that order matches `id DESC` exactly (proving the
// tiebreak clause is genuinely wired in, not merely present but inert).
func TestGetGitSnapshotsBySession_TiebreaksIdenticalTimestampsStably(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	const taskID = "task-snapshot-tie"
	const sessionID = "session-snapshot-tie"
	seedGitSnapshotSession(t, repo, taskID, sessionID)

	tieAt := time.Now().UTC()
	// Explicit IDs, chosen so lexicographic order is unambiguous and known
	// in advance: "id-b" > "id-a", so `id DESC` must return id-b first.
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "id-a-tiebreak", SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch: "feature/tie", Ahead: 1, TriggeredBy: "agent_completed",
		CreatedAt: tieAt,
	}); err != nil {
		t.Fatalf("create first tied snapshot: %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "id-b-tiebreak", SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch: "feature/tie", Ahead: 2, TriggeredBy: TriggeredByLiveMonitor,
		CreatedAt: tieAt,
	}); err != nil {
		t.Fatalf("create second tied snapshot: %v", err)
	}

	var firstOrder []string
	for call := 0; call < 5; call++ {
		snapshots, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 5)
		if err != nil {
			t.Fatalf("GetGitSnapshotsBySession (call %d): %v", call, err)
		}
		if len(snapshots) != 2 {
			t.Fatalf("snapshot count (call %d) = %d, want 2", call, len(snapshots))
		}
		order := []string{snapshots[0].ID, snapshots[1].ID}
		if call == 0 {
			firstOrder = order
			if order[0] != "id-b-tiebreak" || order[1] != "id-a-tiebreak" {
				t.Fatalf("tie order = %v, want [id-b-tiebreak, id-a-tiebreak] (id DESC)", order)
			}
			continue
		}
		if order[0] != firstOrder[0] || order[1] != firstOrder[1] {
			t.Fatalf("tie order changed across calls: call 0 = %v, call %d = %v — tiebreak is not stable", firstOrder, call, order)
		}
	}
}

// TestGetGitSnapshotsBySession_ExcludesArchiveTypeRows guards the
// session-delete pre-delete-warning defect documented in
// docs/specs/session-delete-resource-cleanup REVIEW-ROUND: 3: this method's
// only production consumer is the pre-delete warning
// (apps/web/hooks/use-session-delete-warning.ts), which dedupes snapshots by
// branch. captureArchiveDiff (apps/backend/internal/orchestrator/task_operations.go)
// writes an "archive" snapshot for a session that was archived before later
// being deleted, and that row never sets Branch — it always persists as "".
// Without a snapshot_type filter, that archive row's (possibly stale) Files
// count gets bucketed under the "" branch key and added on top of whatever
// the session's real branch reports, inflating (or, if the real branch is
// ever also empty, potentially shadowing) the warning. Only "status_update"
// rows carry live pre-delete-warning-relevant state.
func TestGetGitSnapshotsBySession_ExcludesArchiveTypeRows(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	const taskID = "task-snapshot-archive"
	const sessionID = "session-snapshot-archive"
	seedGitSnapshotSession(t, repo, taskID, sessionID)

	now := time.Now().UTC()
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch: "feature/real", Ahead: 1,
		Files:       map[string]interface{}{"real.ts": map[string]interface{}{"status": "modified"}},
		TriggeredBy: "agent_completed",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("create status_update snapshot: %v", err)
	}

	// captureArchiveDiff never sets Branch or Ahead, so this row always
	// persists with Branch == "" — reproduced here exactly as production does.
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, SnapshotType: models.SnapshotTypeArchive,
		Files:     map[string]interface{}{"archived-stale.ts": map[string]interface{}{"status": "modified"}},
		CreatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create archive snapshot: %v", err)
	}

	snapshots, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 5)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshot count = %d, want 1 (archive-type row must be excluded), got %#v", len(snapshots), snapshots)
	}
	if snapshots[0].SnapshotType != models.SnapshotTypeStatusUpdate {
		t.Fatalf("returned snapshot type = %q, want %q", snapshots[0].SnapshotType, models.SnapshotTypeStatusUpdate)
	}
	if snapshots[0].Branch != "feature/real" {
		t.Fatalf("returned snapshot branch = %q, want feature/real", snapshots[0].Branch)
	}
}

// TestGetGitSnapshotsBySession_FilesColumnDecodesToEmptyMapNotNil guards the
// serialization half of the same defect: a genuinely-empty files column
// (persisted as the literal "{}") must decode to a non-nil empty map, not Go
// nil — a nil map marshals to JSON `null`, and a WS consumer that expects
// "files is always an object" (e.g. Object.keys(files) on the frontend)
// would otherwise be surprised by null for a snapshot that legitimately has
// zero uncommitted files.
func TestGetGitSnapshotsBySession_FilesColumnDecodesToEmptyMapNotNil(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	const taskID = "task-snapshot-empty-files"
	const sessionID = "session-snapshot-empty-files"
	seedGitSnapshotSession(t, repo, taskID, sessionID)

	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch: "main", Ahead: 0, Files: nil, TriggeredBy: "agent_completed",
	}); err != nil {
		t.Fatalf("create clean snapshot: %v", err)
	}

	snapshots, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 1)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshot count = %d, want 1", len(snapshots))
	}
	if snapshots[0].Files == nil {
		t.Fatal("Files decoded as Go nil (marshals to JSON null); want a non-nil empty map")
	}
	if len(snapshots[0].Files) != 0 {
		t.Fatalf("Files = %#v, want empty", snapshots[0].Files)
	}
}
