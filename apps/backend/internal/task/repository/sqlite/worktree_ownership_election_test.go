package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	dbutil "github.com/kandev/kandev/internal/db"
)

// TestCutover_ElectsNewestWorktreeForContestedSlot proves that two live
// sessions of the same task holding different worktrees for one repository
// slot no longer abort the upgrade: the newest worktree owns the slot and the
// older one is demoted to history.
func TestCutover_ElectsNewestWorktreeForContestedSlot(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	older := now.Add(-2 * time.Hour)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-conflict', 'ws-1', 'legacy', ?, ?)`), older, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	for _, s := range []string{"sess-c1", "sess-c2"} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
			VALUES (?, 'task-conflict', 'RUNNING', ?, ?)`), s, older, now); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	seedLegacySessionWorktree(t, db, "sess-c1", "wt-c1", "repo-c", "", "/t/c1", "feature/c1", "active", older)
	seedLegacySessionWorktree(t, db, "sess-c2", "wt-c2", "repo-c", "", "/t/c2", "feature/c2", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironmentByTaskID(context.Background(), "task-conflict")
	if err != nil {
		t.Fatalf("GetTaskEnvironmentByTaskID: %v", err)
	}
	if env == nil {
		t.Fatal("contested task must keep an environment")
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-c2" {
		t.Fatalf("elected repository rows = %+v, want one wt-c2 row", env.Repos)
	}
	if legacyTableExists(t, db, "task_session_worktrees") {
		t.Fatal("legacy table must be dropped once the cutover completes")
	}
}

// TestCutover_FlatEnvironmentOutranksLiveSessionWorktree reproduces the
// upgrade failure from issue #2505: a database with no canonical repository
// row where a still-live session holds an older worktree for the slot the
// surviving flat environment owns. The flat environment keeps the slot.
func TestCutover_FlatEnvironmentOutranksLiveSessionWorktree(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-live", taskID: "task-live", repoID: "repo-live", sessionID: "sess-live"}
	seedLegacyTask(t, db, seed, now.Add(-3*time.Hour))
	if _, err := db.Exec(db.Rebind(`UPDATE task_sessions SET state = 'IDLE' WHERE id = ?`), seed.sessionID); err != nil {
		t.Fatalf("set session state: %v", err)
	}
	seedLegacyFlatEnv(t, db, seed, "wt-current", "/tasks/live/repo", "feature/current", now)
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-old", "repo-live", "", "/tasks/live/old", "feature/old", "active", now.Add(-3*time.Hour))

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-current" {
		t.Fatalf("repository rows = %+v, want one wt-current row", env.Repos)
	}
	session, err := repo.GetTaskSession(context.Background(), seed.sessionID)
	if err != nil {
		t.Fatalf("get task session: %v", err)
	}
	if session.TaskEnvironmentID != seed.envID {
		t.Fatalf("session environment = %q, want %q", session.TaskEnvironmentID, seed.envID)
	}
}

// TestCutover_LiveSessionOutranksTerminalSessionForSlot proves the election
// prefers a live session's worktree over a terminal session's newer one when
// no canonical owner exists for the slot.
func TestCutover_LiveSessionOutranksTerminalSessionForSlot(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-mixed', 'ws-1', 'legacy', ?, ?)`), now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	for _, session := range []struct{ id, state string }{{"sess-live", "WAITING_FOR_INPUT"}, {"sess-done", "COMPLETED"}} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
			VALUES (?, 'task-mixed', ?, ?, ?)`), session.id, session.state, now, now); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	seedLegacySessionWorktree(t, db, "sess-live", "wt-live", "repo-mixed", "", "/t/live", "feature/live", "active", now.Add(-time.Hour))
	seedLegacySessionWorktree(t, db, "sess-done", "wt-done", "repo-mixed", "", "/t/done", "feature/done", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironmentByTaskID(context.Background(), "task-mixed")
	if err != nil {
		t.Fatalf("GetTaskEnvironmentByTaskID: %v", err)
	}
	if env == nil || len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-live" {
		t.Fatalf("elected repository rows = %+v, want one wt-live row", env)
	}
}

// TestCutover_IrreconcilableMetadataFailsClosed proves the cutover still
// refuses to guess when two live sessions disagree about the path of the same
// physical worktree and no canonical owner can settle it.
func TestCutover_IrreconcilableMetadataFailsClosed(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-metadata', 'ws-1', 'legacy', ?, ?)`), now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	for _, s := range []string{"sess-m1", "sess-m2"} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
			VALUES (?, 'task-metadata', 'RUNNING', ?, ?)`), s, now, now); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	seedLegacySessionWorktree(t, db, "sess-m1", "wt-m", "repo-m", "", "/t/one", "feature/m", "active", now)
	seedLegacySessionWorktree(t, db, "sess-m2", "wt-m", "repo-m", "", "/t/two", "feature/m", "active", now)

	if _, err := NewWithDB(db, db, nil); err == nil {
		t.Fatal("expected cutover to fail on irreconcilable worktree metadata")
	} else if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("error must describe the conflict, got: %v", err)
	}
	// Pre-upgrade state must be intact.
	if !legacyTableExists(t, db, "task_session_worktrees") {
		t.Fatal("legacy table must survive a failed cutover")
	}
	envCols := tableColumnSet(t, db, "task_environments")
	if !envCols["worktree_id"] || !envCols["repository_id"] {
		t.Fatal("legacy flat columns must survive a failed cutover")
	}
}

// TestCutoverElection_RecordsDemotionDiagnostics proves every demoted legacy
// worktree is recorded with the directory and branch support needs to find it
// on disk, since the migration itself never touches the filesystem.
func TestCutoverElection_RecordsDemotionDiagnostics(t *testing.T) {
	now := time.Now().UTC()
	cut := &worktreeCutover{
		envs:                      map[string]*legacyEnv{},
		sessions:                  map[string]*legacySession{},
		sessionTasks:              map[string]string{},
		sessionEnvIDs:             map[string]string{},
		sessionWorktreeSuperseded: map[string]bool{},
		demotedWorktrees:          map[string]bool{},
		authoritativeWorktreeIDs:  map[string]bool{},
		tasks:                     map[string]*taskWorktreeTargets{},
		taskEnvIDs:                map[string]string{},
		loserEnvIDs:               map[string]bool{},
		executorTypes:             map[string]string{},
	}
	for _, id := range []string{"sess-old", "sess-new"} {
		cut.sessions[id] = &legacySession{id: id, taskID: "task-1", state: "RUNNING"}
		cut.sessionTasks[id] = "task-1"
	}
	cut.sessionWts = []legacySessionWorktree{
		{sessionID: "sess-old", worktreeID: "wt-old", repositoryID: "repo-1",
			worktreePath: "/tasks/old", worktreeBranch: "feature/old",
			createdAt: now.Add(-time.Hour), updatedAt: now.Add(-time.Hour), status: "active"},
		{sessionID: "sess-new", worktreeID: "wt-new", repositoryID: "repo-1",
			worktreePath: "/tasks/new", worktreeBranch: "feature/new",
			createdAt: now, updatedAt: now, status: "active"},
	}

	cut.electSlotWorktreeOwners()

	if !cut.demotedWorktrees[sessionWorktreeKey("sess-old", "wt-old")] {
		t.Fatal("older worktree must be demoted")
	}
	if cut.demotedWorktrees[sessionWorktreeKey("sess-new", "wt-new")] {
		t.Fatal("elected worktree must not be demoted")
	}
	if len(cut.demotions) != 1 {
		t.Fatalf("demotions = %v, want exactly one entry", cut.demotions)
	}
	for _, want := range []string{"sess-old", "wt-old", "/tasks/old", "feature/old", "wt-new"} {
		if !strings.Contains(cut.demotions[0], want) {
			t.Fatalf("demotion %q must mention %q", cut.demotions[0], want)
		}
	}
}

// TestCutoverElection_DoesNotLogRolledBackDemotions proves operators only see
// demotion diagnostics after the ownership transaction commits successfully.
func TestCutoverElection_DoesNotLogRolledBackDemotions(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-log', 'ws-1', 'legacy', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	for _, sessionID := range []string{"sess-log-old", "sess-log-new"} {
		if _, err := db.Exec(`
			INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
			VALUES (?, 'task-log', 'RUNNING', ?, ?)`, sessionID, now, now); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	seedLegacySessionWorktree(t, db, "sess-log-old", "wt-log-old", "repo-log", "", "/tasks/old", "feature/old", "active", now.Add(-time.Hour))
	seedLegacySessionWorktree(t, db, "sess-log-new", "wt-log-new", "repo-log", "", "/tasks/new", "feature/new", "active", now)

	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	repo := &Repository{
		db: db, ro: db, log: log, migrate: dbutil.NewMigrateLogger(db, log),
		failCutoverAfter: "pre_swap",
	}
	if err := repo.initSchema(); err == nil {
		t.Fatal("expected injected cutover failure")
	}
	message := "cutover: duplicate legacy worktrees demoted to history"
	if entries := logs.FilterMessage(message).All(); len(entries) != 0 {
		t.Fatalf("rolled-back demotion logs = %d, want none", len(entries))
	}
}
