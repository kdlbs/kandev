package delivery_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/delivery"
)

func seedTaskSession(t *testing.T, db *sqlx.DB, id, taskID, repositoryID string) {
	t.Helper()
	now := time.Now().UTC()
	environmentID := "env-" + id
	var existingEnvironmentID string
	err := db.Get(&existingEnvironmentID, db.Rebind(`
		SELECT id FROM task_environments WHERE task_id = ?
	`), taskID)
	switch err {
	case nil:
		environmentID = existingEnvironmentID
	case sql.ErrNoRows:
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_environments (id, task_id, status, workspace_path, created_at, updated_at)
			VALUES (?, ?, 'ready', ?, ?, ?)
		`), environmentID, taskID, "/workspace/"+id, now, now); err != nil {
			t.Fatalf("seed task_environment %s: %v", environmentID, err)
		}
	default:
		t.Fatalf("find task_environment for task %s: %v", taskID, err)
	}
	if repositoryID != "" {
		var existingRepoID string
		err := db.Get(&existingRepoID, db.Rebind(`
			SELECT id FROM task_environment_repos
			WHERE task_environment_id = ? AND repository_id = ?
		`), environmentID, repositoryID)
		if err == sql.ErrNoRows {
			if _, err := db.Exec(db.Rebind(`
				INSERT INTO task_environment_repos (id, task_environment_id, repository_id, status, created_at, updated_at)
				VALUES (?, ?, ?, 'active', ?, ?)
			`), "env-repo-"+id, environmentID, repositoryID, now, now); err != nil {
				t.Fatalf("seed task_environment_repo %s: %v", id, err)
			}
		} else if err != nil {
			t.Fatalf("find task_environment_repo for %s: %v", repositoryID, err)
		}
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, repository_id, task_environment_id, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), id, taskID, repositoryID, environmentID, now, now); err != nil {
		t.Fatalf("seed task_session %s: %v", id, err)
	}
}

func seedTaskRepository(t *testing.T, db *sqlx.DB, id, taskID, repositoryID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), id, taskID, repositoryID, now, now); err != nil {
		t.Fatalf("seed task_repository %s: %v", id, err)
	}
}

func seedGitSnapshot(t *testing.T, db *sqlx.DB, id, sessionID, branch, headCommit string, ahead int, createdAt time.Time) {
	t.Helper()
	var environmentID string
	if err := db.Get(&environmentID, db.Rebind(`SELECT task_environment_id FROM task_sessions WHERE id = ?`), sessionID); err != nil {
		t.Fatalf("resolve snapshot environment for %s: %v", sessionID, err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_git_snapshots (id, task_environment_id, session_id, snapshot_type, branch, head_commit, ahead, created_at)
		VALUES (?, ?, ?, 'test', ?, ?, ?, ?)
	`), id, environmentID, sessionID, branch, headCommit, ahead, createdAt); err != nil {
		t.Fatalf("seed snapshot %s: %v", id, err)
	}
}

func TestCandidates_ExcludesMissingRepositoryAndTask(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-real", "ws-1")
	seedTask(t, db, "task-real", "ws-1")

	// A valid pair.
	seedTaskRepository(t, db, "tr-1", "task-real", "repo-real")
	// A pair whose repository row does not exist (dangling repository_id).
	seedTaskSession(t, db, "sess-missing-repo", "task-real", "repo-ghost")
	// A pair whose task row does not exist (orphaned provider row via a
	// session with no matching task — approximated here via task_sessions
	// since seeding an orphaned github_task_prs row needs the github
	// schema; task_sessions has no FK to tasks either).

	ctx := context.Background()
	result, err := repo.Candidates(ctx)
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	if len(result.Pairs) != 1 || result.Pairs[0].TaskID != "task-real" || result.Pairs[0].RepositoryID != "repo-real" {
		t.Fatalf("pairs = %+v, want exactly the valid pair", result.Pairs)
	}
	if result.MissingRepository != 1 {
		t.Fatalf("MissingRepository = %d, want 1", result.MissingRepository)
	}
}

// TestCandidates_ExcludesMissingTask covers spec "Candidate pairs", "Why
// condition 3 exists": a github_task_prs row whose task_id matches no tasks
// row — a hard-deleted task the GitHub task-deleted subscriber did not
// prune the provider row for — is excluded from candidacy and counted via
// MissingTask, never attempted, exactly parallel to the MissingRepository
// case above but reachable only through condition 3 (task_sessions has no
// FK to tasks either, but the spec's own example is a provider row).
func TestCandidates_ExcludesMissingTask(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-real", "ws-1")
	seedGitHubStore(t, db)
	now := time.Now().UTC()
	seedGitHubPR(t, db, "pr-orphan", "task-ghost", "repo-real", "acme", "widgets",
		1, "https://gh/1", "main", &now, nil)

	ctx := context.Background()
	result, err := repo.Candidates(ctx)
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	for _, p := range result.Pairs {
		if p.TaskID == "task-ghost" {
			t.Fatalf("orphaned-task pair must not be a candidate, got %+v", p)
		}
	}
	if result.MissingTask != 1 {
		t.Fatalf("MissingTask = %d, want 1", result.MissingTask)
	}
}

func TestCandidates_TaskWithNoRepositoryProducesNoRow(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedTask(t, db, "task-bare", "ws-1")
	// No task_repositories, task_sessions, or provider row references
	// task-bare at all.

	ctx := context.Background()
	result, err := repo.Candidates(ctx)
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	for _, p := range result.Pairs {
		if p.TaskID == "task-bare" {
			t.Fatalf("task with no repository must produce no candidate pair, got %+v", p)
		}
	}
}

func TestSnapshotsForPair_JoinsThroughSessions(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedTaskSession(t, db, "sess-1", "task-1", "repo-1")
	seedTaskSession(t, db, "sess-other-repo", "task-1", "repo-other")

	at0 := time.Now().UTC().Add(-time.Hour)
	seedGitSnapshot(t, db, "snap-1", "sess-1", "feature", "aaa", 3, at0)
	seedGitSnapshot(t, db, "snap-2", "sess-other-repo", "feature", "zzz", 9, at0)

	ctx := context.Background()
	snaps, err := repo.SnapshotsForPair(ctx, "task-1", "repo-1")
	if err != nil {
		t.Fatalf("SnapshotsForPair: %v", err)
	}
	if len(snaps) != 1 || snaps[0].HeadCommit != "aaa" {
		t.Fatalf("snaps = %+v, want exactly snap-1", snaps)
	}
}

// TestSnapshotsForPair_NullHeadCommitNormalizesToEmpty covers Review round
// 1, finding #6: task_session_git_snapshots.head_commit defaults to an
// empty string but is not NOT NULL (base_schema.go), and spec
// "Classification"'s normalization rule explicitly anticipates a real SQL
// NULL there ("A head_commit that is NULL, empty, or whitespace-only is
// not a distinct head value"). Scanning the column into a bare Go string
// instead of sql.NullString used to fail the whole query on a literal
// NULL row, aborting the pair's evaluation rather than normalizing to
// empty. seedGitSnapshot cannot express a literal NULL (a Go empty string
// binds as a SQL empty string, not NULL), so this test inserts one
// directly.
func TestSnapshotsForPair_NullHeadCommitNormalizesToEmpty(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedTaskSession(t, db, "sess-1", "task-1", "repo-1")

	at0 := time.Now().UTC().Add(-time.Hour)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_git_snapshots (id, task_environment_id, session_id, snapshot_type, branch, head_commit, ahead, created_at)
		VALUES (?, (SELECT task_environment_id FROM task_sessions WHERE id = ?), ?, 'test', ?, NULL, ?, ?)
	`), "snap-null", "sess-1", "sess-1", "feature", 2, at0); err != nil {
		t.Fatalf("seed null-head snapshot: %v", err)
	}

	ctx := context.Background()
	snaps, err := repo.SnapshotsForPair(ctx, "task-1", "repo-1")
	if err != nil {
		t.Fatalf("SnapshotsForPair must tolerate a literal NULL head_commit, got: %v", err)
	}
	if len(snaps) != 1 || snaps[0].HeadCommit != "" {
		t.Fatalf("snaps = %+v, want exactly one snapshot with HeadCommit normalized to empty", snaps)
	}
}

func TestSnapshotsForPairSurviveCaptureSessionDeletion(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environments (id, task_id, status, workspace_path, created_at, updated_at)
		VALUES ('env-shared', 'task-1', 'ready', '/workspace/shared', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed shared environment: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environment_repos (id, task_environment_id, repository_id, status, created_at, updated_at)
		VALUES ('env-repo-shared', 'env-shared', 'repo-1', 'active', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed shared environment repository: %v", err)
	}
	for _, sessionID := range []string{"sess-binding", "sess-capture"} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_sessions (id, task_id, repository_id, task_environment_id, started_at, updated_at)
			VALUES (?, 'task-1', 'repo-1', 'env-shared', ?, ?)
		`), sessionID, now, now); err != nil {
			t.Fatalf("seed session %s: %v", sessionID, err)
		}
	}
	seedGitSnapshot(t, db, "snap-capture", "sess-capture", "feature", "abcd", 2, now)
	if _, err := db.Exec(db.Rebind(`DELETE FROM task_sessions WHERE id = ?`), "sess-capture"); err != nil {
		t.Fatalf("delete capture session: %v", err)
	}

	snapshots, err := repo.SnapshotsForPair(context.Background(), "task-1", "repo-1")
	if err != nil {
		t.Fatalf("SnapshotsForPair: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].HeadCommit != "abcd" || snapshots[0].SessionID != "" {
		t.Fatalf("snapshots = %+v, want environment-owned snapshot with cleared provenance", snapshots)
	}
}

func TestSnapshotsForPairSurviveWithoutCaptureSessions(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environments (id, task_id, status, workspace_path, created_at, updated_at)
		VALUES ('env-without-sessions', 'task-1', 'ready', '/workspace/without-sessions', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed task environment: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environment_repos (id, task_environment_id, repository_id, status, created_at, updated_at)
		VALUES ('env-repo-without-sessions', 'env-without-sessions', 'repo-1', 'active', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed environment repository: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_git_snapshots (
			id, task_environment_id, session_id, snapshot_type, branch, head_commit, metadata, created_at
		) VALUES ('snap-without-session', 'env-without-sessions', NULL, 'test', 'feature', 'abcd', ?, ?)
	`), `{"repository_name":"repo-1"}`, now); err != nil {
		t.Fatalf("seed environment snapshot: %v", err)
	}

	snapshots, err := repo.SnapshotsForPair(context.Background(), "task-1", "repo-1")
	if err != nil {
		t.Fatalf("SnapshotsForPair: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].HeadCommit != "abcd" || snapshots[0].SessionID != "" {
		t.Fatalf("snapshots = %+v, want the environment snapshot without session provenance", snapshots)
	}
}

func TestSnapshotsForPairDoesNotDuplicateAcrossEnvironmentBranches(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedTaskSession(t, db, "sess-1", "task-1", "repo-1")
	now := time.Now().UTC()
	var environmentID string
	if err := db.Get(&environmentID, db.Rebind(`SELECT task_environment_id FROM task_sessions WHERE id = ?`), "sess-1"); err != nil {
		t.Fatalf("resolve task environment: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug, status, created_at, updated_at
		) VALUES ('env-repo-branch', ?, 'repo-1', 'branch-2', 'active', ?, ?)
	`), environmentID, now, now); err != nil {
		t.Fatalf("seed second environment branch: %v", err)
	}
	seedGitSnapshot(t, db, "snap-1", "sess-1", "feature", "abcd", 2, now)

	snapshots, err := repo.SnapshotsForPair(context.Background(), "task-1", "repo-1")
	if err != nil {
		t.Fatalf("SnapshotsForPair: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].HeadCommit != "abcd" {
		t.Fatalf("snapshots = %+v, want one snapshot despite two environment branches", snapshots)
	}
}

func TestProvidersForPair_MissingTablesTolerated(t *testing.T) {
	// A database with only the task schema (no github/gitlab/azuredevops
	// store initialized) must not error; it simply contributes no rows.
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	_ = db

	ctx := context.Background()
	prs, err := repo.ProvidersForPair(ctx, "task-1", "repo-1")
	if err != nil {
		t.Fatalf("ProvidersForPair must tolerate missing provider tables, got: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("prs = %+v, want none", prs)
	}
}

func TestGitHubPRs_MergedAndDetachedFromColumns(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedGitHubStore(t, db)

	now := time.Now().UTC()
	seedGitHubPR(t, db, "pr-1", "task-1", "repo-1", "acme", "widgets", 42, "https://gh/1", "main", &now, nil)
	seedGitHubPR(t, db, "pr-2", "task-1", "repo-1", "acme", "widgets", 43, "https://gh/2", "main", &now, &now)

	ctx := context.Background()
	prs, err := repo.ProvidersForPair(ctx, "task-1", "repo-1")
	if err != nil {
		t.Fatalf("ProvidersForPair: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("prs = %+v, want 2", prs)
	}
	byURL := map[string]delivery.ProviderRequest{}
	for _, p := range prs {
		byURL[p.URL] = p
	}
	if !byURL["https://gh/1"].Merged || byURL["https://gh/1"].Detached {
		t.Fatalf("pr-1 = %+v, want merged and not detached", byURL["https://gh/1"])
	}
	if !byURL["https://gh/2"].Merged || !byURL["https://gh/2"].Detached {
		t.Fatalf("pr-2 = %+v, want merged and detached", byURL["https://gh/2"])
	}
	if byURL["https://gh/1"].ScopeValue != "acme/widgets" {
		t.Fatalf("scope value = %q, want acme/widgets", byURL["https://gh/1"].ScopeValue)
	}
}

func TestGitLabMRs_MergedFromColumnAndScopeIsProjectPath(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedGitLabStore(t, db)

	now := time.Now().UTC()
	seedGitLabMR(t, db, "mr-1", "task-1", "repo-1", "group/project", 7, "https://gitlab/mr/7", "main", &now)
	seedGitLabMR(t, db, "mr-2", "task-1", "repo-1", "group/project", 8, "https://gitlab/mr/8", "main", nil)

	ctx := context.Background()
	prs, err := repo.ProvidersForPair(ctx, "task-1", "repo-1")
	if err != nil {
		t.Fatalf("ProvidersForPair: %v", err)
	}
	byURL := map[string]delivery.ProviderRequest{}
	for _, p := range prs {
		byURL[p.URL] = p
	}
	merged, ok := byURL["https://gitlab/mr/7"]
	if !ok || !merged.Merged || merged.Detached {
		t.Fatalf("mr-1 = %+v, want merged and never detached (GitLab has no detached_at column)", merged)
	}
	if merged.ScopeValue != "group/project" {
		t.Fatalf("scope value = %q, want group/project", merged.ScopeValue)
	}
	if unmerged := byURL["https://gitlab/mr/8"]; unmerged.Merged {
		t.Fatalf("mr-2 = %+v, want not merged (NULL merged_at)", unmerged)
	}
}

func TestAzureDevOpsPRs_StatusCaseInsensitiveAndUnrecognisedIsNotMerged(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedAzureDevOpsStore(t, db)

	seedAzurePR(t, db, "az-1", "task-1", "repo-1", "azrepo-1", 1, "https://az/1", "main", "Completed")
	seedAzurePR(t, db, "az-2", "task-1", "repo-1", "azrepo-1", 2, "https://az/2", "main", "abandoned")
	seedAzurePR(t, db, "az-3", "task-1", "repo-1", "azrepo-1", 3, "https://az/3", "main", "some-future-status")

	ctx := context.Background()
	prs, err := repo.ProvidersForPair(ctx, "task-1", "repo-1")
	if err != nil {
		t.Fatalf("ProvidersForPair: %v", err)
	}
	byURL := map[string]delivery.ProviderRequest{}
	for _, p := range prs {
		byURL[p.URL] = p
	}
	if !byURL["https://az/1"].Merged {
		t.Fatal("Completed (case-insensitive) must be merged")
	}
	if byURL["https://az/2"].Merged {
		t.Fatal("abandoned must not be merged")
	}
	if byURL["https://az/3"].Merged {
		t.Fatal("an unrecognised status must not be merged and must not error")
	}
}
