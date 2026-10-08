package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestTurnChangeSetSchemaReplayAndCAS(t *testing.T) {
	repo := newRepoForSessionTests(t)
	runTurnChangeSetSchemaReplayAndCAS(t, repo)
}

func TestPostgresTurnChangeSetSchemaReplayAndCAS(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	runTurnChangeSetSchemaReplayAndCAS(t, repo)
}

func TestTurnChangeSetSchemaUpgradeFromLegacy(t *testing.T) {
	repo := newRepoForSessionTests(t)
	assertTurnChangeSetSchemaUpgrade(t, repo)
}

func TestTurnRepositoryStartAcceptsTypedUnavailableCapture(t *testing.T) {
	err := validateTurnRepositoryChangeStart("change-set", models.TurnRepositoryChangeSet{
		ID: "repository-change", CheckoutID: "checkout", TaskEnvironmentRepoID: "env-repo",
		RepositoryID: "repository", Availability: models.TurnChangeAvailabilityUnavailable,
		Reason: models.TurnChangeReasonCheckoutUnavailable,
	})
	if err != nil {
		t.Fatalf("validate unavailable start: %v", err)
	}
}

func TestTurnRepositoryStartReportsMissingCheckoutIdentity(t *testing.T) {
	err := validateTurnRepositoryChangeStart("change-set", models.TurnRepositoryChangeSet{
		ID: "repository-change", CheckoutID: "checkout", RepositoryID: "repository",
		Availability: models.TurnChangeAvailabilityUnavailable, Reason: models.TurnChangeReasonCheckoutUnavailable,
	})
	if err == nil || !strings.Contains(err.Error(), "task_environment_repo_id") {
		t.Fatalf("validate missing environment repository identity error = %v, want field name", err)
	}
}

func TestFinalizeTurnChangeSetWithoutCapture(t *testing.T) {
	repo := newRepoForSessionTests(t)
	runFinalizeTurnChangeSetWithoutCapture(t, repo)
}

func TestPostgresFinalizeTurnChangeSetWithoutCapture(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	runFinalizeTurnChangeSetWithoutCapture(t, repo)
}

func runFinalizeTurnChangeSetWithoutCapture(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-disabled-turn-changes", Title: "Disabled"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-disabled-turn-changes", TaskID: "task-disabled-turn-changes"}); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-disabled-turn-changes", TaskID: "task-disabled-turn-changes", TaskEnvironmentID: "env-disabled-turn-changes"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	startedAt := time.Now().UTC()
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-disabled-turn-changes", TaskID: "task-disabled-turn-changes", TaskSessionID: "session-disabled-turn-changes", StartedAt: startedAt}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	changeSet := &models.TurnChangeSet{
		ID: "change-set-disabled-turn-changes", TaskID: "task-disabled-turn-changes", TaskSessionID: "session-disabled-turn-changes",
		TurnID: "turn-disabled-turn-changes", TaskEnvironmentID: "env-disabled-turn-changes", Revision: 1,
		CaptureEnabled: false, Availability: models.TurnChangeAvailabilityUnavailable,
		Reason: models.TurnChangeReasonCaptureDisabled,
	}
	if err := repo.CreateTurnChangeSet(ctx, changeSet); err != nil {
		t.Fatalf("create disabled change set: %v", err)
	}
	terminalAt := startedAt.Add(time.Minute)
	accepted, err := repo.FinalizeTurnChangeSet(ctx, changeSet.ID, 1, models.TurnChangeSetFinalization{
		Availability: models.TurnChangeAvailabilityUnavailable, Reason: models.TurnChangeReasonCaptureDisabled,
		TerminalAt: terminalAt, TerminalOutcome: "end_turn",
	})
	if err != nil || !accepted {
		t.Fatalf("finalize disabled capture = %t, %v; want accepted", accepted, err)
	}
	loaded, err := repo.GetTurnChangeSet(ctx, changeSet.TaskID, changeSet.TaskSessionID, changeSet.ID)
	if err != nil || loaded.TerminalAt == nil || loaded.Reason != models.TurnChangeReasonCaptureDisabled {
		t.Fatalf("loaded disabled change set = %+v, %v", loaded, err)
	}
}

func TestPostgresTurnChangeSetSchemaUpgradeFromLegacy(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	assertTurnChangeSetSchemaUpgrade(t, repo)
}

func assertTurnChangeSetSchemaUpgrade(t *testing.T, repo *Repository) {
	t.Helper()
	for _, table := range []string{
		"turn_change_content_leases", "turn_change_content_links", "turn_change_contents", "turn_file_changes",
		"turn_repository_changes", "turn_change_sets",
	} {
		if _, err := repo.db.Exec(`DROP TABLE ` + table); err != nil {
			t.Fatalf("drop %s to simulate a legacy schema: %v", table, err)
		}
	}
	if err := repo.initTurnChangesSchema(); err != nil {
		t.Fatalf("upgrade legacy schema: %v", err)
	}
	for _, table := range []string{
		"turn_change_sets", "turn_repository_changes", "turn_file_changes",
		"turn_change_contents", "turn_change_content_links", "turn_change_content_leases",
	} {
		var count int
		query := `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?`
		if !dialect.IsPostgres(repo.db.DriverName()) {
			query = `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`
		}
		if err := repo.db.Get(&count, repo.db.Rebind(query), table); err != nil || count != 1 {
			t.Fatalf("legacy schema upgrade table %s count = %d, %v; want one", table, count, err)
		}
	}
}

func runTurnChangeSetSchemaReplayAndCAS(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	const (
		taskID    = "task-turn-changes"
		envID     = "env-turn-changes"
		sessionID = "session-turn-changes"
		turnID    = "turn-turn-changes"
	)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Turn changes"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: envID, TaskID: taskID, Status: models.TaskEnvironmentStatusCreating,
		Repos: []*models.TaskEnvironmentRepo{{ID: "env-repo-turn-changes", RepositoryID: "repository-turn-changes"}},
	}); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: envID,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	startedAt := time.Now().UTC()
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskID: taskID, TaskSessionID: sessionID, StartedAt: startedAt,
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}

	changeSet := &models.TurnChangeSet{
		ID: "change-set-turn-changes", TaskID: taskID, TaskSessionID: sessionID,
		TurnID: turnID, TaskEnvironmentID: envID, Revision: 1,
		CaptureEnabled: true, SettingsUserID: "user-turn-changes", SettingsRevision: 7,
		ResolutionKind: "authenticated_user", Availability: models.TurnChangeAvailabilityPending,
	}
	if err := repo.CreateTurnChangeSet(ctx, changeSet); err != nil {
		t.Fatalf("create change set: %v", err)
	}
	if err := repo.CreateTurnChangeSet(ctx, changeSet); !errors.Is(err, ErrTurnChangeSetIdentityConflict) {
		t.Fatalf("duplicate turn change set error = %v, want identity conflict", err)
	}

	start := models.TurnRepositoryChangeSet{
		ID: "repository-change-turn-changes", TurnChangeSetID: changeSet.ID,
		TaskEnvironmentRepoID: "env-repo-turn-changes", CheckoutID: "checkout-turn-changes",
		TaskRepositoryID: "task-repository-turn-changes", RepositoryID: "repository-turn-changes",
		DisplayName: "repo", StartCommitOID: "abc123", StartTreeOID: "tree123", HashAlgorithm: "sha1",
		StartCapturedAt: &startedAt, StartReachabilityRef: "refs/kandev/turn-changes/start",
		Availability: models.TurnChangeAvailabilityPending,
	}
	accepted, err := repo.AcceptTurnChangeSetStart(ctx, changeSet.ID, 1, []models.TurnRepositoryChangeSet{start})
	if err != nil || !accepted {
		t.Fatalf("accept start = %t, %v; want accepted", accepted, err)
	}
	start.StartCommitOID = "different"
	accepted, err = repo.AcceptTurnChangeSetStart(ctx, changeSet.ID, 1, []models.TurnRepositoryChangeSet{start})
	if err != nil || accepted {
		t.Fatalf("repeat start = %t, %v; want stale compare-and-set", accepted, err)
	}
	start.StartCommitOID = "abc123"

	terminalAt := startedAt.Add(time.Minute)
	end := models.TurnRepositoryChangeSet{
		ID: start.ID, TurnChangeSetID: changeSet.ID, CheckoutID: start.CheckoutID,
		EndCommitOID: "def456", EndTreeOID: "tree456", HashAlgorithm: "sha1",
		EndReachabilityRef: "refs/kandev/turn-changes/end", EndCapturedAt: &terminalAt,
	}
	overlap := models.TurnChangeOverlap{
		ChangeSetID: "overlap-turn-changes", CheckoutID: start.CheckoutID, StartedAt: startedAt, EndedAt: &terminalAt,
	}
	finalization := models.TurnChangeSetFinalization{
		Availability: models.TurnChangeAvailabilityReady, Complete: true,
		SummaryComplete: true, ContentComplete: true, TerminalAt: terminalAt,
		FileCount: 1, AddedLines: int64Ptr(2), DeletedLines: int64Ptr(1), RepositoryCount: 1,
		OverlapIntervals: []models.TurnChangeOverlap{overlap},
		Repositories: []models.TurnRepositoryChangeSet{{
			ID: start.ID, TurnChangeSetID: changeSet.ID, CheckoutID: start.CheckoutID,
			EndCommitOID: end.EndCommitOID, EndTreeOID: end.EndTreeOID, EndReachabilityRef: end.EndReachabilityRef,
			EndCapturedAt: end.EndCapturedAt, Availability: models.TurnChangeAvailabilityReady,
			EnumerationComplete: true, ComparisonComplete: true, ContentComplete: true,
		}},
	}
	accepted, err = repo.FinalizeTurnChangeSet(ctx, changeSet.ID, 2, finalization)
	if accepted || !errors.Is(err, ErrTurnChangeRelationship) {
		t.Fatalf("finalize unaccepted end endpoint = %t, %v; want immutable relationship error", accepted, err)
	}
	accepted, err = repo.AcceptTurnRepositoryEnd(ctx, changeSet.ID, start.ID, start.StartCommitOID, start.StartTreeOID, end)
	if err != nil || !accepted {
		t.Fatalf("accept end endpoint = %t, %v; want accepted", accepted, err)
	}
	mismatchedEnd := end
	mismatchedEnd.EndCommitOID = "other-commit"
	accepted, err = repo.AcceptTurnRepositoryEnd(ctx, changeSet.ID, start.ID, start.StartCommitOID, start.StartTreeOID, mismatchedEnd)
	if err != nil || accepted {
		t.Fatalf("replace accepted end endpoint = %t, %v; want immutable endpoint", accepted, err)
	}
	accepted, err = repo.FinalizeTurnChangeSet(ctx, changeSet.ID, 2, finalization)
	if err != nil || !accepted {
		t.Fatalf("finalize = %t, %v; want accepted", accepted, err)
	}
	accepted, err = repo.FinalizeTurnChangeSet(ctx, changeSet.ID, 2, models.TurnChangeSetFinalization{
		Availability: models.TurnChangeAvailabilityFailed, TerminalAt: terminalAt,
	})
	if err != nil || accepted {
		t.Fatalf("repeat finalization = %t, %v; want stale compare-and-set", accepted, err)
	}

	loaded, err := repo.GetTurnChangeSet(ctx, taskID, sessionID, changeSet.ID)
	if err != nil {
		t.Fatalf("get change set: %v", err)
	}
	if loaded.Revision != 3 || loaded.Availability != models.TurnChangeAvailabilityReady || loaded.Partial() {
		t.Fatalf("loaded change set = %+v, want revision 3 and complete ready summary", loaded)
	}
	if loaded.AddedLines == nil || *loaded.AddedLines != 2 || loaded.DeletedLines == nil || *loaded.DeletedLines != 1 {
		t.Fatalf("loaded line counts = %v/%v, want 2/1", loaded.AddedLines, loaded.DeletedLines)
	}
	if len(loaded.OverlapIntervals) != 1 || loaded.OverlapIntervals[0].ChangeSetID != overlap.ChangeSetID {
		t.Fatalf("loaded overlap intervals = %+v, want %+v", loaded.OverlapIntervals, overlap)
	}
	repositoryChanges, err := repo.ListTurnRepositoryChanges(ctx, changeSet.ID)
	if err != nil || len(repositoryChanges) != 1 {
		t.Fatalf("repository changes = %d, %v; want one", len(repositoryChanges), err)
	}
	if repositoryChanges[0].StartCommitOID != "abc123" || repositoryChanges[0].EndCommitOID != "def456" {
		t.Fatalf("stored endpoints = %q..%q, want immutable abc123..def456", repositoryChanges[0].StartCommitOID, repositoryChanges[0].EndCommitOID)
	}

	if err := repo.initTurnChangesSchema(); err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	loaded, err = repo.GetTurnChangeSet(ctx, taskID, sessionID, changeSet.ID)
	if err != nil || loaded.Revision != 3 {
		t.Fatalf("change set after schema replay = %+v, %v", loaded, err)
	}

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO turn_file_changes (id, repository_change_id, checkout_id, path, path_bytes, kind, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), "file-turn-changes", start.ID, start.CheckoutID, "src/file.go", []byte("src/file.go"), "modified", terminalAt); err != nil {
		t.Fatalf("insert file metadata: %v", err)
	}
	for _, path := range []string{"literal\\backslash.txt", "literal/backslash.txt"} {
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
			INSERT INTO turn_file_changes (id, repository_change_id, checkout_id, path, path_bytes, kind, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`), "file-"+path, start.ID, start.CheckoutID, path, []byte(path), "modified", terminalAt); err != nil {
			t.Fatalf("insert literal Git path %q: %v", path, err)
		}
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO turn_file_changes (id, repository_change_id, checkout_id, path, path_bytes, kind, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), "file-duplicate-path", start.ID, start.CheckoutID, "literal/backslash.txt", []byte("literal/backslash.txt"), "modified", terminalAt); err == nil {
		t.Fatal("duplicate checkout/path identity was accepted")
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO turn_change_contents (id, digest, codec, uncompressed_bytes, payload_bytes, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), "content-turn-changes", "sha256-turn-changes", "zstd", 3, []byte{1, 2, 3}, terminalAt); err != nil {
		t.Fatalf("insert content: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO turn_change_content_links (file_change_id, variant, content_id, created_at)
		VALUES (?, ?, ?, ?)
	`), "file-turn-changes", "canonical_patch", "content-turn-changes", terminalAt); err != nil {
		t.Fatalf("link content: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM task_environments WHERE id = ?`), envID); err != nil {
		t.Fatalf("delete ephemeral environment: %v", err)
	}
	if _, err := repo.GetTurnChangeSet(ctx, taskID, sessionID, changeSet.ID); err != nil {
		t.Fatalf("environment cleanup removed durable history: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM tasks WHERE id = ?`), taskID); err != nil {
		t.Fatalf("delete owning task: %v", err)
	}
	for _, table := range []string{"turn_change_sets", "turn_repository_changes", "turn_file_changes", "turn_change_content_links"} {
		var count int
		if err := repo.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM "+table); err != nil || count != 0 {
			t.Fatalf("%s rows after owner delete = %d, %v; want 0", table, count, err)
		}
	}
	var contentCount int
	if err := repo.db.GetContext(ctx, &contentCount, `SELECT COUNT(*) FROM turn_change_contents`); err != nil || contentCount != 1 {
		t.Fatalf("shared content rows after owner delete = %d, %v; want retained payload", contentCount, err)
	}
}

func TestTurnChangeSetRejectsMismatchedTaskSessionTurnAndEnvironment(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "turn-changes-owner", "turn-changes-session", "turn-changes-turn")
	seedForMsgTest(t, repo, "turn-changes-other-task", "turn-changes-other-session", "turn-changes-other-turn")
	for _, environment := range []struct {
		id      string
		taskID  string
		session string
	}{
		{id: "turn-changes-owner-env", taskID: "turn-changes-owner", session: "turn-changes-session"},
		{id: "turn-changes-other-env", taskID: "turn-changes-other-task", session: "turn-changes-other-session"},
	} {
		if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
			ID: environment.id, TaskID: environment.taskID, Status: models.TaskEnvironmentStatusCreating,
		}); err != nil {
			t.Fatalf("create environment %s: %v", environment.id, err)
		}
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`), environment.id, environment.session); err != nil {
			t.Fatalf("bind environment %s to session %s: %v", environment.id, environment.session, err)
		}
	}
	for _, test := range []struct {
		name          string
		taskID        string
		sessionID     string
		turnID        string
		environmentID string
	}{
		{name: "mismatched task", taskID: "turn-changes-other-task", sessionID: "turn-changes-session", turnID: "turn-changes-turn", environmentID: "turn-changes-owner-env"},
		{name: "mismatched turn session", taskID: "turn-changes-owner", sessionID: "turn-changes-session", turnID: "turn-changes-other-turn", environmentID: "turn-changes-owner-env"},
		{name: "mismatched environment task", taskID: "turn-changes-owner", sessionID: "turn-changes-session", turnID: "turn-changes-turn", environmentID: "turn-changes-other-env"},
	} {
		t.Run(test.name, func(t *testing.T) {
			changeSet := &models.TurnChangeSet{
				ID: "invalid-" + test.name, TaskID: test.taskID, TaskSessionID: test.sessionID,
				TurnID: test.turnID, TaskEnvironmentID: test.environmentID, Revision: 1,
				Availability: models.TurnChangeAvailabilityPending,
			}
			if err := repo.CreateTurnChangeSet(ctx, changeSet); !errors.Is(err, ErrTurnChangeRelationship) {
				t.Fatalf("create invalid change set error = %v, want relationship error", err)
			}
		})
	}
}

func int64Ptr(value int64) *int64 { return &value }
