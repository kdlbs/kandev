package sqlite

//revive:disable:file-length-limit // Git snapshot/commit persistence coverage is intentionally scenario-heavy.

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedSessionForGit creates the task, environment, and session rows that the
// git snapshot and commit tables reference.
func seedSessionForGit(t *testing.T, repo *Repository, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
	environmentID := "env-" + taskID
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment(%s): %v", environmentID, err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
}

// countRows is a raw row counter used to assert deletes actually removed rows
// rather than trusting the repository read path to agree with itself.
func countRows(t *testing.T, repo *Repository, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := repo.db.QueryRowContext(context.Background(), repo.db.Rebind(query), args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

// assertTimeEqual compares two timestamps at microsecond resolution. SQLite
// round-trips full nanoseconds but Postgres TIMESTAMP tops out at microseconds,
// so the shared assertion helpers — which both dialects' tests call — must
// compare at the coarser of the two. A dropped or zeroed column still fails.
func assertTimeEqual(t *testing.T, label string, got, want time.Time) {
	t.Helper()
	if !got.Truncate(time.Microsecond).Equal(want.Truncate(time.Microsecond)) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func fullGitSnapshot(sessionID string, createdAt time.Time) *models.GitSnapshot {
	return &models.GitSnapshot{
		ID:           "snapshot-full",
		SessionID:    sessionID,
		SnapshotType: models.SnapshotTypeArchive,
		Branch:       "feature/full",
		RemoteBranch: "origin/feature/full",
		HeadCommit:   "head-sha-1234",
		BaseCommit:   "base-sha-9876",
		Ahead:        3,
		Behind:       7,
		Files: map[string]interface{}{
			"apps/backend/main.go": map[string]interface{}{
				"status":    "modified",
				"additions": float64(12),
			},
		},
		// Production archive rows (captureArchiveDiff) carry no trigger; the
		// trigger field must stay empty so ordering tests exercise the
		// conditional `archive AND task archived` rank, not a
		// triggered_by=agent_completed rank.
		TriggeredBy: "",
		Metadata: map[string]interface{}{
			"reason":  "task archived",
			"attempt": float64(2),
		},
		CreatedAt: createdAt,
	}
}

func assertGitSnapshotEqual(t *testing.T, got, want *models.GitSnapshot) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.TaskEnvironmentID != want.TaskEnvironmentID {
		t.Errorf("TaskEnvironmentID = %q, want %q", got.TaskEnvironmentID, want.TaskEnvironmentID)
	}
	if got.SessionID != want.SessionID {
		t.Errorf("SessionID = %q, want %q", got.SessionID, want.SessionID)
	}
	if got.SnapshotType != want.SnapshotType {
		t.Errorf("SnapshotType = %q, want %q", got.SnapshotType, want.SnapshotType)
	}
	if got.Branch != want.Branch {
		t.Errorf("Branch = %q, want %q", got.Branch, want.Branch)
	}
	if got.RemoteBranch != want.RemoteBranch {
		t.Errorf("RemoteBranch = %q, want %q", got.RemoteBranch, want.RemoteBranch)
	}
	if got.HeadCommit != want.HeadCommit {
		t.Errorf("HeadCommit = %q, want %q", got.HeadCommit, want.HeadCommit)
	}
	if got.BaseCommit != want.BaseCommit {
		t.Errorf("BaseCommit = %q, want %q", got.BaseCommit, want.BaseCommit)
	}
	if got.Ahead != want.Ahead {
		t.Errorf("Ahead = %d, want %d", got.Ahead, want.Ahead)
	}
	if got.Behind != want.Behind {
		t.Errorf("Behind = %d, want %d", got.Behind, want.Behind)
	}
	if got.TriggeredBy != want.TriggeredBy {
		t.Errorf("TriggeredBy = %q, want %q", got.TriggeredBy, want.TriggeredBy)
	}
	assertTimeEqual(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
	assertJSONMapEqual(t, "Files", got.Files, want.Files)
	assertJSONMapEqual(t, "Metadata", got.Metadata, want.Metadata)
}

// assertJSONMapEqual compares two JSON-decoded maps structurally. It is written
// by hand (instead of reflect.DeepEqual on the whole snapshot) so a dropped
// nested key names the field it came from.
func assertJSONMapEqual(t *testing.T, label string, got, want map[string]interface{}) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s has %d keys (%v), want %d (%v)", label, len(got), got, len(want), want)
		return
	}
	for key, wantValue := range want {
		gotValue, ok := got[key]
		if !ok {
			t.Errorf("%s missing key %q", label, key)
			continue
		}
		wantNested, wantIsMap := wantValue.(map[string]interface{})
		gotNested, gotIsMap := gotValue.(map[string]interface{})
		if wantIsMap != gotIsMap {
			t.Errorf("%s[%q] = %#v, want %#v", label, key, gotValue, wantValue)
			continue
		}
		if wantIsMap {
			assertJSONMapEqual(t, label+"["+key+"]", gotNested, wantNested)
			continue
		}
		if gotValue != wantValue {
			t.Errorf("%s[%q] = %#v, want %#v", label, key, gotValue, wantValue)
		}
	}
}

func TestCreateGitSnapshotRoundTripsEveryField(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-full", "session-snap-full")

	createdAt := time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)
	want := fullGitSnapshot("session-snap-full", createdAt)
	if err := repo.CreateGitSnapshot(ctx, want); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}

	// Archive the task so GetLatestGitSnapshot selects the archive row via
	// its rank-0 branch (snapshot_type='archive' AND task archived) rather
	// than merely being the only row — the fixture's intent is the
	// authoritative-archive read, and the row is only the sole row here by
	// construction. (Claude review note on PR #2851.)
	if err := repo.ArchiveTask(ctx, "task-snap-full"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	got, err := repo.GetLatestGitSnapshot(ctx, "session-snap-full")
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	assertGitSnapshotEqual(t, got, want)

	// The same row must survive the other three read paths identically.
	first, err := repo.GetFirstGitSnapshot(ctx, "session-snap-full")
	if err != nil {
		t.Fatalf("GetFirstGitSnapshot: %v", err)
	}
	assertGitSnapshotEqual(t, first, want)

	listed, err := repo.GetGitSnapshotsBySession(ctx, "session-snap-full", 0)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("GetGitSnapshotsBySession returned %d rows, want 1", len(listed))
	}
	assertGitSnapshotEqual(t, listed[0], want)

	batch, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, []string{"session-snap-full"})
	if err != nil {
		t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("batch returned %d entries, want 1", len(batch))
	}
	assertGitSnapshotEqual(t, batch["session-snap-full"], want)
}

func TestCreateGitSnapshotDefaultsIDAndCreatedAt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-defaults", "session-snap-defaults")

	before := time.Now().UTC().Add(-time.Second)
	snapshot := &models.GitSnapshot{
		SessionID:    "session-snap-defaults",
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch:       "main",
	}
	if err := repo.CreateGitSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}
	if snapshot.ID == "" {
		t.Fatal("CreateGitSnapshot left ID empty; a UUID should have been generated")
	}
	if snapshot.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt = %v, want a freshly stamped time after %v", snapshot.CreatedAt, before)
	}

	got, err := repo.GetLatestGitSnapshot(ctx, "session-snap-defaults")
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	if got.ID != snapshot.ID {
		t.Errorf("persisted ID = %q, want the generated %q", got.ID, snapshot.ID)
	}
	// Files/Metadata were nil on the way in; they serialize to "{}" and must
	// come back nil rather than as an empty-but-non-nil map.
	if got.Files != nil {
		t.Errorf("Files = %#v, want nil for a snapshot written without files", got.Files)
	}
	if got.Metadata != nil {
		t.Errorf("Metadata = %#v, want nil for a snapshot written without metadata", got.Metadata)
	}
}

func TestCreateGitSnapshotEmptyMapsReadBackAsNil(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-empty", "session-snap-empty")

	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snapshot-empty", SessionID: "session-snap-empty",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
		Files:    map[string]interface{}{},
		Metadata: map[string]interface{}{},
	}); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}

	got, err := repo.GetLatestGitSnapshot(ctx, "session-snap-empty")
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	if got.Files != nil {
		t.Errorf("Files = %#v, want nil (an empty map serializes to the {} sentinel)", got.Files)
	}
	if got.Metadata != nil {
		t.Errorf("Metadata = %#v, want nil (an empty map serializes to the {} sentinel)", got.Metadata)
	}
}

func TestCreateGitSnapshotRejectsUnserializableFiles(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-bad", "session-snap-bad")

	err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snapshot-bad", SessionID: "session-snap-bad",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
		Files: map[string]interface{}{"chan": make(chan int)},
	})
	if err == nil {
		t.Fatal("CreateGitSnapshot accepted an unserializable Files map")
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_session_git_snapshots WHERE session_id = ?`, "session-snap-bad"); got != 0 {
		t.Errorf("rows after a failed serialize = %d, want 0", got)
	}
}

func TestGetLatestGitSnapshotPrefersAgentCompletedOverNewerLiveMonitor(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-prefer", "session-snap-prefer")

	base := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snap-completed", SessionID: "session-snap-prefer",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
		TriggeredBy: "agent_completed", HeadCommit: "authoritative", CreatedAt: base,
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(agent_completed): %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snap-live", SessionID: "session-snap-prefer",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
		TriggeredBy: TriggeredByLiveMonitor, HeadCommit: "stale-poll", CreatedAt: base.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(live_monitor): %v", err)
	}

	got, err := repo.GetLatestGitSnapshot(ctx, "session-snap-prefer")
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	if got.ID != "snap-completed" {
		t.Errorf("GetLatestGitSnapshot returned %q (head %q), want the agent_completed row", got.ID, got.HeadCommit)
	}

	batch, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, []string{"session-snap-prefer"})
	if err != nil {
		t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
	}
	if batch["session-snap-prefer"].ID != "snap-completed" {
		t.Errorf("batch returned %q, want the agent_completed row (must mirror GetLatestGitSnapshot)", batch["session-snap-prefer"].ID)
	}
}

func TestGetLatestGitSnapshotFallsBackToNewestWithoutAgentCompleted(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-newest", "session-snap-newest")

	base := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"snap-old", "snap-mid", "snap-new"} {
		if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
			ID: id, SessionID: "session-snap-newest",
			SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
			TriggeredBy: TriggeredByLiveMonitor, CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("CreateGitSnapshot(%s): %v", id, err)
		}
	}

	got, err := repo.GetLatestGitSnapshot(ctx, "session-snap-newest")
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	if got.ID != "snap-new" {
		t.Errorf("GetLatestGitSnapshot = %q, want snap-new (most recent by created_at)", got.ID)
	}

	first, err := repo.GetFirstGitSnapshot(ctx, "session-snap-newest")
	if err != nil {
		t.Fatalf("GetFirstGitSnapshot: %v", err)
	}
	if first.ID != "snap-old" {
		t.Errorf("GetFirstGitSnapshot = %q, want snap-old (oldest by created_at)", first.ID)
	}
}

func TestGitSnapshotReadsReturnErrNoRowsForUnknownSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	if _, err := repo.GetLatestGitSnapshot(ctx, "session-missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetLatestGitSnapshot error = %v, want sql.ErrNoRows", err)
	}
	if _, err := repo.GetFirstGitSnapshot(ctx, "session-missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetFirstGitSnapshot error = %v, want sql.ErrNoRows", err)
	}
	if _, err := repo.GetLatestSessionCommit(ctx, "session-missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetLatestSessionCommit error = %v, want sql.ErrNoRows", err)
	}

	snapshots, err := repo.GetGitSnapshotsBySession(ctx, "session-missing", 0)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(snapshots) != 0 {
		t.Errorf("GetGitSnapshotsBySession = %v, want empty", snapshots)
	}
	commits, err := repo.GetSessionCommits(ctx, "session-missing")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("GetSessionCommits = %v, want empty", commits)
	}
}

func TestGetGitSnapshotsBySessionOrdersDescendingAndHonoursLimit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-list", "session-snap-list")
	seedSessionForGit(t, repo, "task-snap-other", "session-snap-other")

	base := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	for i, id := range []string{"list-1", "list-2", "list-3"} {
		if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
			ID: id, SessionID: "session-snap-list",
			SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("CreateGitSnapshot(%s): %v", id, err)
		}
	}
	// A snapshot on a different session must never leak into the results.
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "other-1", SessionID: "session-snap-other",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main", CreatedAt: base.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(other): %v", err)
	}

	all, err := repo.GetGitSnapshotsBySession(ctx, "session-snap-list", 0)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession(limit=0): %v", err)
	}
	gotIDs := make([]string, 0, len(all))
	for _, snapshot := range all {
		gotIDs = append(gotIDs, snapshot.ID)
	}
	wantIDs := []string{"list-3", "list-2", "list-1"}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("limit=0 returned %v, want %v", gotIDs, wantIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("limit=0 returned %v, want %v (newest first)", gotIDs, wantIDs)
		}
	}

	limited, err := repo.GetGitSnapshotsBySession(ctx, "session-snap-list", 2)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession(limit=2): %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("limit=2 returned %d rows, want 2", len(limited))
	}
	if limited[0].ID != "list-3" || limited[1].ID != "list-2" {
		t.Errorf("limit=2 returned %q,%q, want list-3,list-2", limited[0].ID, limited[1].ID)
	}

	negative, err := repo.GetGitSnapshotsBySession(ctx, "session-snap-list", -1)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession(limit=-1): %v", err)
	}
	if len(negative) != 3 {
		t.Errorf("limit=-1 returned %d rows, want all 3", len(negative))
	}
}

func TestUpsertLatestLiveGitSnapshotKeepsOneLiveRowPerSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-upsert", "session-snap-upsert")

	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	// An archive snapshot that the live upsert must not disturb.
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "archive-row", SessionID: "session-snap-upsert",
		SnapshotType: models.SnapshotTypeArchive, Branch: "main",
		TriggeredBy: "task_archived", CreatedAt: base,
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(archive): %v", err)
	}

	first := &models.GitSnapshot{
		ID: "live-1", SessionID: "session-snap-upsert", Branch: "main",
		HeadCommit: "first-head", CreatedAt: base.Add(time.Minute),
		// Deliberately wrong: the method must force these two fields.
		SnapshotType: models.SnapshotTypeArchive, TriggeredBy: "someone-else",
	}
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, first); err != nil {
		t.Fatalf("UpsertLatestLiveGitSnapshot(first): %v", err)
	}
	if first.SnapshotType != models.SnapshotTypeStatusUpdate {
		t.Errorf("SnapshotType = %q, want it forced to %q", first.SnapshotType, models.SnapshotTypeStatusUpdate)
	}
	if first.TriggeredBy != TriggeredByLiveMonitor {
		t.Errorf("TriggeredBy = %q, want it forced to %q", first.TriggeredBy, TriggeredByLiveMonitor)
	}

	second := &models.GitSnapshot{
		ID: "live-2", SessionID: "session-snap-upsert", Branch: "main",
		HeadCommit: "second-head", Ahead: 4, CreatedAt: base.Add(2 * time.Minute),
		Files: map[string]interface{}{"a.go": "modified"},
	}
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, second); err != nil {
		t.Fatalf("UpsertLatestLiveGitSnapshot(second): %v", err)
	}

	liveRows := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE session_id = ? AND triggered_by = ?`,
		"session-snap-upsert", TriggeredByLiveMonitor)
	if liveRows != 1 {
		t.Errorf("live_monitor rows = %d, want exactly 1 after two upserts", liveRows)
	}
	archiveRows := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE session_id = ? AND snapshot_type = ?`,
		"session-snap-upsert", string(models.SnapshotTypeArchive))
	if archiveRows != 1 {
		t.Errorf("archive rows = %d, want the pre-existing archive snapshot untouched", archiveRows)
	}

	all, err := repo.GetGitSnapshotsBySession(ctx, "session-snap-upsert", 0)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("session has %d snapshots, want 2 (archive + one live)", len(all))
	}
	if all[0].ID != "live-2" || all[0].HeadCommit != "second-head" || all[0].Ahead != 4 {
		t.Errorf("newest snapshot = %+v, want the second live upsert", all[0])
	}
	assertJSONMapEqual(t, "Files", all[0].Files, map[string]interface{}{"a.go": "modified"})
}

func TestUpsertLatestLiveGitSnapshotKeepsOneLiveRowPerRepository(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-multi-repo", "session-snap-multi-repo")
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	for _, snapshot := range []*models.GitSnapshot{
		{
			ID: "live-root-1", SessionID: "session-snap-multi-repo", HeadCommit: "root-1",
			Metadata: map[string]interface{}{"repository_name": "", "timestamp": "2026-06-01T09:01:00Z"}, CreatedAt: base.Add(time.Minute),
		},
		{
			ID: "live-frontend-1", SessionID: "session-snap-multi-repo", HeadCommit: "frontend-1",
			Metadata: map[string]interface{}{"repository_name": "frontend", "timestamp": "2026-06-01T09:01:00Z"}, CreatedAt: base.Add(time.Minute),
		},
		{
			ID: "live-root-2", SessionID: "session-snap-multi-repo", HeadCommit: "root-2",
			Metadata: map[string]interface{}{"repository_name": "", "timestamp": "2026-06-01T09:02:00Z"}, CreatedAt: base.Add(2 * time.Minute),
		},
	} {
		if err := repo.UpsertLatestLiveGitSnapshot(ctx, snapshot); err != nil {
			t.Fatalf("UpsertLatestLiveGitSnapshot(%s): %v", snapshot.ID, err)
		}
	}

	liveRows := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE session_id = ? AND triggered_by = ?`,
		"session-snap-multi-repo", TriggeredByLiveMonitor)
	if liveRows != 2 {
		t.Fatalf("live_monitor rows = %d, want one row per repository", liveRows)
	}

	all, err := repo.GetGitSnapshotsBySession(ctx, "session-snap-multi-repo", 0)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	byRepository := make(map[string]string, len(all))
	for _, snapshot := range all {
		repositoryName, _ := snapshot.Metadata["repository_name"].(string)
		byRepository[repositoryName] = snapshot.HeadCommit
	}
	if got := byRepository[""]; got != "root-2" {
		t.Errorf("root live snapshot = %q, want root-2", got)
	}
	if got := byRepository["frontend"]; got != "frontend-1" {
		t.Errorf("frontend live snapshot = %q, want frontend-1", got)
	}
}

func TestUpsertLatestLiveGitSnapshotGeneratesIDAndRejectsNil(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-nil", "session-snap-nil")

	if err := repo.UpsertLatestLiveGitSnapshot(ctx, nil); err == nil {
		t.Fatal("UpsertLatestLiveGitSnapshot(nil) returned nil error")
	}

	snapshot := &models.GitSnapshot{SessionID: "session-snap-nil", Branch: "main"}
	before := time.Now().UTC().Add(-time.Second)
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("UpsertLatestLiveGitSnapshot: %v", err)
	}
	if snapshot.ID == "" {
		t.Error("ID was left empty; a UUID should have been generated")
	}
	if snapshot.CreatedAt.Before(before) {
		t.Errorf("CreatedAt = %v, want a freshly stamped time after %v", snapshot.CreatedAt, before)
	}
	got, err := repo.GetLatestGitSnapshot(ctx, "session-snap-nil")
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	if got.ID != snapshot.ID {
		t.Errorf("persisted ID = %q, want %q", got.ID, snapshot.ID)
	}
}

func TestDeleteLiveMonitorSnapshotsRemovesOnlyLiveRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-snap-del", "session-snap-del")
	seedSessionForGit(t, repo, "task-snap-keep", "session-snap-keep")

	base := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	rows := []*models.GitSnapshot{
		{ID: "del-live", SessionID: "session-snap-del", SnapshotType: models.SnapshotTypeStatusUpdate,
			Branch: "main", TriggeredBy: TriggeredByLiveMonitor, CreatedAt: base},
		{ID: "del-completed", SessionID: "session-snap-del", SnapshotType: models.SnapshotTypeStatusUpdate,
			Branch: "main", TriggeredBy: "agent_completed", CreatedAt: base.Add(time.Minute)},
		{ID: "keep-live", SessionID: "session-snap-keep", SnapshotType: models.SnapshotTypeStatusUpdate,
			Branch: "main", TriggeredBy: TriggeredByLiveMonitor, CreatedAt: base},
	}
	for _, row := range rows {
		if err := repo.CreateGitSnapshot(ctx, row); err != nil {
			t.Fatalf("CreateGitSnapshot(%s): %v", row.ID, err)
		}
	}

	if err := repo.DeleteLiveMonitorSnapshots(ctx, "session-snap-del"); err != nil {
		t.Fatalf("DeleteLiveMonitorSnapshots: %v", err)
	}

	remaining, err := repo.GetGitSnapshotsBySession(ctx, "session-snap-del", 0)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != "del-completed" {
		t.Errorf("remaining snapshots = %+v, want only del-completed", remaining)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE session_id = ?`, "session-snap-keep"); got != 1 {
		t.Errorf("other session rows = %d, want 1 (delete must be session-scoped)", got)
	}

	// Deleting again is a no-op, not an error.
	if err := repo.DeleteLiveMonitorSnapshots(ctx, "session-snap-del"); err != nil {
		t.Errorf("second DeleteLiveMonitorSnapshots: %v", err)
	}
}

func TestGetLatestGitSnapshotsBySessionIDsBatchesAndSkipsUnknown(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC)
	for i, suffix := range []string{"a", "b"} {
		seedSessionForGit(t, repo, "task-batch-"+suffix, "session-batch-"+suffix)
		if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
			ID: "batch-old-" + suffix, SessionID: "session-batch-" + suffix,
			SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
			TriggeredBy: TriggeredByLiveMonitor, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("CreateGitSnapshot(old %s): %v", suffix, err)
		}
		if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
			ID: "batch-new-" + suffix, SessionID: "session-batch-" + suffix,
			SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
			TriggeredBy: TriggeredByLiveMonitor, HeadCommit: "newest-" + suffix,
			CreatedAt: base.Add(time.Hour + time.Duration(i)*time.Minute),
		}); err != nil {
			t.Fatalf("CreateGitSnapshot(new %s): %v", suffix, err)
		}
	}

	got, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx,
		[]string{"session-batch-a", "session-batch-b", "session-batch-missing"})
	if err != nil {
		t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("map has %d entries (%v), want 2 — unknown sessions must be absent", len(got), got)
	}
	for _, suffix := range []string{"a", "b"} {
		entry, ok := got["session-batch-"+suffix]
		if !ok {
			t.Fatalf("missing entry for session-batch-%s", suffix)
		}
		if entry.ID != "batch-new-"+suffix {
			t.Errorf("session-batch-%s = %q, want batch-new-%s (one row per session, newest)", suffix, entry.ID, suffix)
		}
		if entry.HeadCommit != "newest-"+suffix {
			t.Errorf("session-batch-%s head = %q, want newest-%s", suffix, entry.HeadCommit, suffix)
		}
	}

	empty, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, nil)
	if err != nil {
		t.Fatalf("GetLatestGitSnapshotsBySessionIDs(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("nil input returned %v, want an empty map", empty)
	}
}

func TestGetLatestGitSnapshotsBySessionIDsChunksBeyondHostParamLimit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	// Two sessions carry snapshots; the ID list is padded past
	// sqliteMaxHostParams so the chunking loop runs more than once.
	seedSessionForGit(t, repo, "task-chunk-real", "session-chunk-real")
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "chunk-real", SessionID: "session-chunk-real",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
		CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}

	ids := make([]string, 0, sqliteMaxHostParams+2)
	for i := 0; i < sqliteMaxHostParams+1; i++ {
		ids = append(ids, "session-chunk-pad-"+time.Duration(i).String())
	}
	ids = append(ids, "session-chunk-real")

	got, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, ids)
	if err != nil {
		t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
	}
	if len(got) != 1 || got["session-chunk-real"] == nil {
		t.Fatalf("map = %v, want exactly the one real session", got)
	}
	if got["session-chunk-real"].ID != "chunk-real" {
		t.Errorf("entry = %q, want chunk-real", got["session-chunk-real"].ID)
	}
}

func TestGitSnapshotsAndCommitsCascadeOnSessionDelete(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-cascade", "session-cascade")

	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "cascade-snap", SessionID: "session-cascade",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}
	if _, err := repo.CreateSessionCommit(ctx, &models.SessionCommit{
		ID: "cascade-commit", SessionID: "session-cascade", CommitSHA: "abc123",
		CommittedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateSessionCommit: %v", err)
	}

	if err := repo.DeleteTaskSession(ctx, "session-cascade"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}

	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE task_environment_id = ?`, "env-task-cascade"); got != 1 {
		t.Errorf("git snapshot rows after session delete = %d, want 1 (environment ownership)", got)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE session_id IS NULL`); got != 1 {
		t.Errorf("session provenance rows after session delete = %d, want 1", got)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ?`, "session-cascade"); got != 0 {
		t.Errorf("commit rows after session delete = %d, want 0 (FK cascade)", got)
	}
}

func TestCreateSessionCommitRoundTripsEveryField(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-commit-full", "session-commit-full")

	want := &models.SessionCommit{
		ID:                   "commit-full",
		SessionID:            "session-commit-full",
		CommitSHA:            "0123456789abcdef",
		ParentSHA:            "fedcba9876543210",
		AuthorName:           "Kandev Agent",
		AuthorEmail:          "agent@kandev.local",
		CommitMessage:        "feat: round-trip every column",
		CommittedAt:          time.Date(2026, 4, 2, 3, 4, 5, 678000000, time.UTC),
		PreCommitSnapshotID:  "pre-snap",
		PostCommitSnapshotID: "post-snap",
		FilesChanged:         9,
		Insertions:           123,
		Deletions:            45,
		CreatedAt:            time.Date(2026, 4, 2, 3, 4, 6, 0, time.UTC),
	}
	if _, err := repo.CreateSessionCommit(ctx, want); err != nil {
		t.Fatalf("CreateSessionCommit: %v", err)
	}

	got, err := repo.GetLatestSessionCommit(ctx, "session-commit-full")
	if err != nil {
		t.Fatalf("GetLatestSessionCommit: %v", err)
	}
	assertSessionCommitEqual(t, got, want)

	listed, err := repo.GetSessionCommits(ctx, "session-commit-full")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("GetSessionCommits returned %d rows, want 1", len(listed))
	}
	assertSessionCommitEqual(t, listed[0], want)
}

func assertSessionCommitEqual(t *testing.T, got, want *models.SessionCommit) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.SessionID != want.SessionID {
		t.Errorf("SessionID = %q, want %q", got.SessionID, want.SessionID)
	}
	if got.CommitSHA != want.CommitSHA {
		t.Errorf("CommitSHA = %q, want %q", got.CommitSHA, want.CommitSHA)
	}
	if got.ParentSHA != want.ParentSHA {
		t.Errorf("ParentSHA = %q, want %q", got.ParentSHA, want.ParentSHA)
	}
	if got.AuthorName != want.AuthorName {
		t.Errorf("AuthorName = %q, want %q", got.AuthorName, want.AuthorName)
	}
	if got.AuthorEmail != want.AuthorEmail {
		t.Errorf("AuthorEmail = %q, want %q", got.AuthorEmail, want.AuthorEmail)
	}
	if got.CommitMessage != want.CommitMessage {
		t.Errorf("CommitMessage = %q, want %q", got.CommitMessage, want.CommitMessage)
	}
	assertTimeEqual(t, "CommittedAt", got.CommittedAt, want.CommittedAt)
	if got.PreCommitSnapshotID != want.PreCommitSnapshotID {
		t.Errorf("PreCommitSnapshotID = %q, want %q", got.PreCommitSnapshotID, want.PreCommitSnapshotID)
	}
	if got.PostCommitSnapshotID != want.PostCommitSnapshotID {
		t.Errorf("PostCommitSnapshotID = %q, want %q", got.PostCommitSnapshotID, want.PostCommitSnapshotID)
	}
	if got.FilesChanged != want.FilesChanged {
		t.Errorf("FilesChanged = %d, want %d", got.FilesChanged, want.FilesChanged)
	}
	if got.Insertions != want.Insertions {
		t.Errorf("Insertions = %d, want %d", got.Insertions, want.Insertions)
	}
	if got.Deletions != want.Deletions {
		t.Errorf("Deletions = %d, want %d", got.Deletions, want.Deletions)
	}
	assertTimeEqual(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
}

func TestCreateSessionCommitDefaultsIDAndCreatedAt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-commit-defaults", "session-commit-defaults")

	before := time.Now().UTC().Add(-time.Second)
	commit := &models.SessionCommit{
		SessionID: "session-commit-defaults", CommitSHA: "deadbeef",
		CommittedAt: time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC),
	}
	if _, err := repo.CreateSessionCommit(ctx, commit); err != nil {
		t.Fatalf("CreateSessionCommit: %v", err)
	}
	if commit.ID == "" {
		t.Fatal("ID was left empty; a UUID should have been generated")
	}
	if commit.CreatedAt.Before(before) {
		t.Errorf("CreatedAt = %v, want a freshly stamped time after %v", commit.CreatedAt, before)
	}
	got, err := repo.GetLatestSessionCommit(ctx, "session-commit-defaults")
	if err != nil {
		t.Fatalf("GetLatestSessionCommit: %v", err)
	}
	if got.ID != commit.ID {
		t.Errorf("persisted ID = %q, want %q", got.ID, commit.ID)
	}
}

// Regression: the same underlying git commit can now be observed by more
// than one trigger (live event, per-turn sweep, archive capture). A second
// CreateSessionCommit for the same (session_id, commit_sha) must be a no-op
// - not a duplicate row, and not an error - and must preserve the original
// observation's created_at rather than overwriting it with the replay time.
func TestCreateSessionCommitIsIdempotentOnSessionAndSHA(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-commit-idempotent", "session-commit-idempotent")

	first := &models.SessionCommit{
		ID: "commit-first", SessionID: "session-commit-idempotent", CommitSHA: "shared-sha",
		CommitMessage: "feat: first observation",
		CommittedAt:   time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC),
	}
	inserted, err := repo.CreateSessionCommit(ctx, first)
	if err != nil {
		t.Fatalf("CreateSessionCommit(first): %v", err)
	}
	if !inserted {
		t.Error("CreateSessionCommit(first) inserted = false, want true")
	}

	second := &models.SessionCommit{
		ID: "commit-second", SessionID: "session-commit-idempotent", CommitSHA: "shared-sha",
		CommitMessage: "feat: re-observed by a different trigger",
		CommittedAt:   time.Date(2026, 4, 2, 4, 0, 0, 0, time.UTC),
	}
	inserted, err = repo.CreateSessionCommit(ctx, second)
	if err != nil {
		t.Fatalf("CreateSessionCommit(second): %v", err)
	}
	if inserted {
		t.Error("CreateSessionCommit(second) inserted = true, want false (ON CONFLICT DO NOTHING)")
	}

	listed, err := repo.GetSessionCommits(ctx, "session-commit-idempotent")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("commit rows = %d, want 1", len(listed))
	}
	if listed[0].ID != "commit-first" {
		t.Errorf("surviving row = %q, want commit-first (the original observation)", listed[0].ID)
	}
	if listed[0].CommitMessage != first.CommitMessage {
		t.Errorf("CommitMessage = %q, want the first observation's message unchanged", listed[0].CommitMessage)
	}
}

func TestGetSessionCommitsOrdersByCommittedAtDescending(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-commit-order", "session-commit-order")
	seedSessionForGit(t, repo, "task-commit-noise", "session-commit-noise")

	base := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"c1", "c2", "c3"} {
		if _, err := repo.CreateSessionCommit(ctx, &models.SessionCommit{
			ID: id, SessionID: "session-commit-order", CommitSHA: id,
			CommittedAt: base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("CreateSessionCommit(%s): %v", id, err)
		}
	}
	if _, err := repo.CreateSessionCommit(ctx, &models.SessionCommit{
		ID: "noise", SessionID: "session-commit-noise", CommitSHA: "noise",
		CommittedAt: base.Add(10 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateSessionCommit(noise): %v", err)
	}

	got, err := repo.GetSessionCommits(ctx, "session-commit-order")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	want := []string{"c3", "c2", "c1"}
	if len(got) != len(want) {
		t.Fatalf("GetSessionCommits returned %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("GetSessionCommits order = %q at index %d, want %q", got[i].ID, i, want[i])
		}
	}

	latest, err := repo.GetLatestSessionCommit(ctx, "session-commit-order")
	if err != nil {
		t.Fatalf("GetLatestSessionCommit: %v", err)
	}
	if latest.ID != "c3" {
		t.Errorf("GetLatestSessionCommit = %q, want c3", latest.ID)
	}
}

func TestDeleteSessionCommitRemovesOnlyTheTargetRow(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-commit-del", "session-commit-del")

	base := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"keep-1", "drop-me", "keep-2"} {
		if _, err := repo.CreateSessionCommit(ctx, &models.SessionCommit{
			ID: id, SessionID: "session-commit-del", CommitSHA: id,
			CommittedAt: base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("CreateSessionCommit(%s): %v", id, err)
		}
	}

	if err := repo.DeleteSessionCommit(ctx, "drop-me"); err != nil {
		t.Fatalf("DeleteSessionCommit: %v", err)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ?`, "session-commit-del"); got != 2 {
		t.Errorf("rows after delete = %d, want 2", got)
	}
	remaining, err := repo.GetSessionCommits(ctx, "session-commit-del")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	for _, commit := range remaining {
		if commit.ID == "drop-me" {
			t.Fatalf("deleted commit still present: %+v", commit)
		}
	}

	// Deleting an unknown id is a tolerated no-op (no sentinel).
	if err := repo.DeleteSessionCommit(ctx, "never-existed"); err != nil {
		t.Errorf("DeleteSessionCommit(unknown): %v", err)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ?`, "session-commit-del"); got != 2 {
		t.Errorf("rows after no-op delete = %d, want 2", got)
	}
}

func TestGitSnapshotMethodsPropagateBackendErrors(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-git-broken", "session-git-broken")
	closeRepoDB(t, repo)

	snapshot := &models.GitSnapshot{
		ID: "snap-broken", SessionID: "session-git-broken",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := repo.CreateGitSnapshot(ctx, snapshot); err == nil {
		t.Error("CreateGitSnapshot on a closed database returned nil")
	}
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, snapshot); err == nil {
		t.Error("UpsertLatestLiveGitSnapshot on a closed database returned nil")
	}
	if err := repo.DeleteLiveMonitorSnapshots(ctx, "session-git-broken"); err == nil {
		t.Error("DeleteLiveMonitorSnapshots on a closed database returned nil")
	}
	if _, err := repo.GetLatestGitSnapshot(ctx, "session-git-broken"); err == nil {
		t.Error("GetLatestGitSnapshot on a closed database returned nil")
	}
	if _, err := repo.GetFirstGitSnapshot(ctx, "session-git-broken"); err == nil {
		t.Error("GetFirstGitSnapshot on a closed database returned nil")
	}
	if _, err := repo.GetGitSnapshotsBySession(ctx, "session-git-broken", 0); err == nil {
		t.Error("GetGitSnapshotsBySession on a closed database returned nil")
	}
	if _, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, []string{"session-git-broken"}); err == nil {
		t.Error("GetLatestGitSnapshotsBySessionIDs on a closed database returned nil")
	}
	if _, err := repo.CreateSessionCommit(ctx, &models.SessionCommit{
		ID: "commit-broken", SessionID: "session-git-broken", CommitSHA: "x",
		CommittedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err == nil {
		t.Error("CreateSessionCommit on a closed database returned nil")
	}
	if _, err := repo.GetSessionCommits(ctx, "session-git-broken"); err == nil {
		t.Error("GetSessionCommits on a closed database returned nil")
	}
	if _, err := repo.GetLatestSessionCommit(ctx, "session-git-broken"); err == nil {
		t.Error("GetLatestSessionCommit on a closed database returned nil")
	}
	if err := repo.DeleteSessionCommit(ctx, "commit-broken"); err == nil {
		t.Error("DeleteSessionCommit on a closed database returned nil")
	}
}
