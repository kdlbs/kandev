package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestGitSnapshotEnvironmentMigrationBackfillsWinnersAndReplays(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID      = "task-git-cutover"
		environment = "env-git-cutover"
		oldSession  = "session-git-cutover-old"
		newSession  = "session-git-cutover-new"
		lostSession = "session-git-cutover-lost"
	)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-git-cutover", Title: "Git snapshot cutover"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environment, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	for _, session := range []*models.TaskSession{
		{ID: oldSession, TaskID: taskID, TaskEnvironmentID: environment},
		{ID: newSession, TaskID: taskID, TaskEnvironmentID: environment},
		{ID: lostSession, TaskID: "task-git-cutover-unresolved"},
	} {
		if session.TaskID != taskID {
			if err := repo.CreateTask(ctx, &models.Task{ID: session.TaskID, WorkspaceID: "ws-" + session.TaskID, Title: session.TaskID}); err != nil {
				t.Fatalf("CreateTask(%s): %v", session.TaskID, err)
			}
		}
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}

	replaceGitSnapshotTableWithLegacy(t, repo)
	base := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	legacyRows := []struct {
		id, sessionID, files, metadata string
		createdAt                      time.Time
	}{
		{"legacy-root-detailed", oldSession, `{"old.go":{"status":"modified"}}`, `{}`, base},
		{"legacy-root-sparse", newSession, `{}`, `{}`, base.Add(time.Hour)},
		{"legacy-named", oldSession, `{"named.go":{"status":"added"}}`, `{"repository_name":"named"}`, base.Add(2 * time.Hour)},
		{"legacy-unresolved", lostSession, `{"lost.go":{"status":"modified"}}`, `{}`, base.Add(3 * time.Hour)},
	}
	for _, row := range legacyRows {
		if _, err := repo.db.Exec(repo.db.Rebind(`
			INSERT INTO task_session_git_snapshots (
				id, session_id, snapshot_type, branch, remote_branch, head_commit, base_commit,
				ahead, behind, files, triggered_by, metadata, created_at
			) VALUES (?, ?, ?, ?, '', '', '', 0, 0, ?, '', ?, ?)
		`), row.id, row.sessionID, string(models.SnapshotTypeStatusUpdate), "main", row.files, row.metadata, row.createdAt); err != nil {
			t.Fatalf("insert legacy row %s: %v", row.id, err)
		}
	}

	if err := repo.initSchema(); err != nil {
		t.Fatalf("run git snapshot cutover: %v", err)
	}
	assertGitSnapshotEnvironmentCutoverRows(t, repo, environment, newSession)

	if err := repo.initSchema(); err != nil {
		t.Fatalf("replay schema after git snapshot cutover: %v", err)
	}
	assertGitSnapshotEnvironmentCutoverRows(t, repo, environment, newSession)
}

func TestGitSnapshotEnvironmentCutoverRollsBackAndCanReplay(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID      = "task-git-cutover-rollback"
		environment = "env-git-cutover-rollback"
		sessionID   = "session-git-cutover-rollback"
	)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-git-cutover-rollback", Title: "Git snapshot rollback"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environment, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environment,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	replaceGitSnapshotTableWithLegacy(t, repo)
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_git_snapshots (
			id, session_id, snapshot_type, branch, files, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`), "legacy-rollback", sessionID, string(models.SnapshotTypeStatusUpdate), "main", `{}`, `{}`, time.Now().UTC()); err != nil {
		t.Fatalf("insert legacy rollback row: %v", err)
	}

	repo.failGitSnapshotCutoverAfter = "pre_swap"
	if err := repo.initSchema(); err == nil {
		t.Fatal("git snapshot cutover unexpectedly succeeded with injected failure")
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_session_git_snapshots WHERE id = ?`, "legacy-rollback"); got != 1 {
		t.Fatalf("legacy row count after rollback = %d, want 1", got)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, gitSnapshotCutoverShadowTable); got != 0 {
		t.Fatalf("shadow table count after rollback = %d, want 0", got)
	}

	repo.failGitSnapshotCutoverAfter = ""
	if err := repo.initSchema(); err != nil {
		t.Fatalf("replay git snapshot cutover after rollback: %v", err)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_session_git_snapshots WHERE task_environment_id = ?`, environment); got != 1 {
		t.Fatalf("environment row count after replay = %d, want 1", got)
	}
}

func replaceGitSnapshotTableWithLegacy(t *testing.T, repo *Repository) {
	t.Helper()
	for _, indexName := range []string{
		"idx_git_snapshots_environment",
		"idx_git_snapshots_environment_type",
		"idx_git_snapshots_session",
		"idx_git_snapshots_type",
	} {
		if _, err := repo.db.Exec("DROP INDEX IF EXISTS " + indexName); err != nil {
			t.Fatalf("drop snapshot index %s: %v", indexName, err)
		}
	}
	if _, err := repo.db.Exec("DROP TABLE task_session_git_snapshots"); err != nil {
		t.Fatalf("drop current snapshot table: %v", err)
	}
	if _, err := repo.db.Exec(`
		CREATE TABLE task_session_git_snapshots (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			snapshot_type TEXT NOT NULL,
			branch TEXT NOT NULL,
			remote_branch TEXT DEFAULT '',
			head_commit TEXT DEFAULT '',
			base_commit TEXT DEFAULT '',
			ahead INTEGER DEFAULT 0,
			behind INTEGER DEFAULT 0,
			files TEXT DEFAULT '{}',
			triggered_by TEXT DEFAULT '',
			metadata TEXT DEFAULT '{}',
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		)
	`); err != nil {
		t.Fatalf("create legacy snapshot table: %v", err)
	}
}

func assertGitSnapshotEnvironmentCutoverRows(t *testing.T, repo *Repository, environment, expectedRootSession string) {
	t.Helper()
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_session_git_snapshots`); got != 2 {
		t.Fatalf("cutover snapshot count = %d, want 2 current repository rows", got)
	}
	current, err := repo.GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(context.Background(), []string{environment})
	if err != nil {
		t.Fatalf("read cutover current rows: %v", err)
	}
	if len(current) != 2 {
		t.Fatalf("cutover current rows = %+v, want two repositories", current)
	}
	byRepository := make(map[string]*models.GitSnapshot, len(current))
	for _, snapshot := range current {
		byRepository[gitSnapshotRepositoryName(snapshot)] = snapshot
	}
	root := byRepository[""]
	if root == nil || root.ID != "legacy-root-sparse" || root.SessionID != expectedRootSession || root.Files != nil {
		t.Fatalf("cutover root = %+v, want sparse winner with session %q", root, expectedRootSession)
	}
	if named := byRepository["named"]; named == nil || named.ID != "legacy-named" {
		t.Fatalf("cutover named row = %+v, want legacy-named", named)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_session_git_snapshots WHERE id = ?`, "legacy-unresolved"); got != 0 {
		t.Fatalf("unresolved legacy rows = %d, want 0", got)
	}
}
