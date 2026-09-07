package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCutover_CanonicalRepoSupersedesDivergentFlatOwner(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{
		envID:     "env-flat-conflict",
		taskID:    "task-flat-conflict",
		repoID:    "repo-flat-conflict",
		sessionID: "sess-flat-conflict",
	}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "wt-flat", "/tasks/flat", "feature/flat", now)
	seedLegacyEnvRepo(t, db, "env-repo-canonical", seed.envID, seed.repoID,
		"wt-canonical", "/tasks/canonical", "feature/canonical", now)

	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	repo, err := NewWithDB(db, db, log)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-canonical" {
		t.Fatalf("normalized repos = %+v, want canonical wt-canonical", env.Repos)
	}
	entries := logs.FilterMessage("cutover: duplicate legacy worktrees demoted to history").All()
	if len(entries) != 1 {
		t.Fatalf("flat demotion logs = %d, want one", len(entries))
	}
	logged := fmt.Sprint(entries[0].ContextMap()["worktrees"])
	for _, want := range []string{"env-flat-conflict", "repo-flat-conflict", "wt-flat", "/tasks/flat", "feature/flat", "wt-canonical"} {
		if !strings.Contains(logged, want) {
			t.Fatalf("flat demotion log %q must mention %q", logged, want)
		}
	}
}

func TestCutover_PreservesDivergentFlatOutsideCanonicalSlot(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{
		envID:     "env-flat-slot",
		taskID:    "task-flat-slot",
		repoID:    "repo-flat-slot",
		sessionID: "sess-flat-slot",
	}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "wt-flat-slot", "/tasks/flat-slot", "feature/flat", now)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, worktree_path, worktree_branch, position,
			error_message, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 0, '', ?, ?)`),
		"env-repo-slot", seed.envID, seed.repoID, "feature/canonical",
		"wt-canonical-slot", "/tasks/canonical-slot", "feature/canonical", now, now); err != nil {
		t.Fatalf("seed canonical repository row: %v", err)
	}

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	slots := make(map[string]string, len(env.Repos))
	for _, envRepo := range env.Repos {
		slots[envRepo.RepositoryID+"\x00"+envRepo.BranchSlug] = envRepo.WorktreeID
	}
	if len(slots) != 2 || slots[seed.repoID+"\x00"] != "wt-flat-slot" ||
		slots[seed.repoID+"\x00feature/canonical"] != "wt-canonical-slot" {
		t.Fatalf("normalized repository slots = %+v", env.Repos)
	}
}

func TestCutover_DuplicateCanonicalRowsRetainSurvivingEnvironmentPrecedence(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		name := "surviving row first"
		if reverse {
			name = "surviving row last"
		}
		t.Run(name, func(t *testing.T) {
			db := openLegacyDB(t)
			now := time.Now().UTC().Truncate(time.Second)
			older := now.Add(-time.Hour)
			seed := legacySeed{
				envID:     "env-duplicate-survivor",
				taskID:    "task-duplicate-canonical",
				repoID:    "repo-duplicate-canonical",
				sessionID: "sess-duplicate-canonical",
			}
			seedLegacyTask(t, db, seed, now)
			seedLegacyFlatEnv(t, db, seed, "wt-flat-duplicate", "/tasks/flat-duplicate", "feature/flat", now)
			seedLegacyFlatEnv(t, db, legacySeed{
				envID:  "env-duplicate-loser",
				taskID: seed.taskID,
				repoID: seed.repoID,
			}, "", "", "", older)

			rows := []struct {
				id, envID string
			}{
				{id: "env-repo-survivor", envID: seed.envID},
				{id: "env-repo-loser", envID: "env-duplicate-loser"},
			}
			if reverse {
				rows[0], rows[1] = rows[1], rows[0]
			}
			for _, row := range rows {
				seedLegacyEnvRepo(t, db, row.id, row.envID, seed.repoID,
					"wt-canonical-duplicate", "/tasks/canonical-duplicate", "feature/canonical", now)
			}

			repo, err := NewWithDB(db, db, nil)
			if err != nil {
				t.Fatalf("cutover: %v", err)
			}
			env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
			if err != nil {
				t.Fatalf("get task environment: %v", err)
			}
			if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-canonical-duplicate" {
				t.Fatalf("normalized repos = %+v, want canonical duplicate owner", env.Repos)
			}
		})
	}
}

func TestCutover_RehomedCanonicalRowSupersedesSurvivingFlatOwner(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{
		envID:     "env-rehome-survivor",
		taskID:    "task-rehome-canonical",
		repoID:    "repo-rehome-canonical",
		sessionID: "sess-rehome-canonical",
	}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "wt-flat-rehome", "/tasks/flat-rehome", "feature/flat", now)
	seedLegacyFlatEnv(t, db, legacySeed{
		envID:  "env-rehome-loser",
		taskID: seed.taskID,
		repoID: seed.repoID,
	}, "", "", "", now.Add(-time.Hour))
	seedLegacyEnvRepo(t, db, "env-repo-rehomed", "env-rehome-loser", seed.repoID,
		"wt-canonical-rehome", "/tasks/canonical-rehome", "feature/canonical", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-canonical-rehome" {
		t.Fatalf("normalized repos = %+v, want rehomed canonical owner", env.Repos)
	}
}

func TestCutover_DoesNotLogRolledBackFlatDemotion(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{
		envID:     "env-flat-rollback",
		taskID:    "task-flat-rollback",
		repoID:    "repo-flat-rollback",
		sessionID: "sess-flat-rollback",
	}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "wt-flat-rollback", "/tasks/flat-rollback", "feature/flat", now)
	seedLegacyEnvRepo(t, db, "env-repo-rollback", seed.envID, seed.repoID,
		"wt-canonical-rollback", "/tasks/canonical-rollback", "feature/canonical", now)

	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	repo := &Repository{
		db:               db,
		ro:               db,
		log:              log,
		migrate:          dbutil.NewMigrateLogger(db, log),
		failCutoverAfter: "pre_swap",
	}
	if err := repo.initSchema(); err == nil {
		t.Fatal("expected injected cutover failure")
	}
	if entries := logs.FilterMessage("cutover: duplicate legacy worktrees demoted to history").All(); len(entries) != 0 {
		t.Fatalf("rolled-back flat demotion logs = %d, want none", len(entries))
	}
}

// TestCutover_SupersededHistoricalSessionWorktreeKeepsCanonicalSlot covers the
// shape reported in issue #2505: a terminal session keeps an active legacy
// worktree row on the empty branch slot while the surviving flat environment
// and its canonical repository row own the same repository on a named slot.
// The session row is superseded, so it must stay out of the normalized graph
// instead of opening a second slot the worktree inventory check then rejects.
func TestCutover_SupersededHistoricalSessionWorktreeKeepsCanonicalSlot(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{
		envID:     "env-superseded-slot",
		taskID:    "task-superseded-slot",
		repoID:    "repo-superseded-slot",
		sessionID: "sess-superseded-slot",
	}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "wt-canonical-slot", "/tasks/canonical-slot", "feature/canonical", now)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, worktree_path, worktree_branch, position,
			error_message, created_at, updated_at
		) VALUES (?, ?, ?, 'main', ?, ?, ?, 0, '', ?, ?)`),
		"env-repo-superseded", seed.envID, seed.repoID,
		"wt-canonical-slot", "/tasks/canonical-slot", "feature/canonical", now, now); err != nil {
		t.Fatalf("seed canonical repository row: %v", err)
	}
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-superseded", seed.repoID, "",
		"/tasks/superseded", "feature/superseded", "active", now)

	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	repo, err := NewWithDB(db, db, log)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if len(env.Repos) != 1 {
		t.Fatalf("normalized repos = %+v, want only the canonical slot", env.Repos)
	}
	if env.Repos[0].WorktreeID != "wt-canonical-slot" || env.Repos[0].BranchSlug != "main" {
		t.Fatalf("normalized repo = %+v, want wt-canonical-slot on slot main", env.Repos[0])
	}
	entries := logs.FilterMessage("cutover: duplicate legacy worktrees demoted to history").All()
	if len(entries) != 1 {
		t.Fatalf("session demotion logs = %d, want one", len(entries))
	}
	logged := fmt.Sprint(entries[0].ContextMap()["worktrees"])
	for _, want := range []string{"sess-superseded-slot", "wt-superseded", "/tasks/superseded", "feature/superseded", "wt-canonical-slot"} {
		if !strings.Contains(logged, want) {
			t.Fatalf("session demotion log %q must mention %q", logged, want)
		}
	}
}

func TestCutover_PreservesActiveSessionWhenFlatOwnerIsDeletedCanonical(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{
		envID:     "env-deleted-canonical",
		taskID:    "task-deleted-canonical",
		repoID:    "repo-deleted-canonical",
		sessionID: "sess-deleted-canonical",
	}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "wt-deleted-canonical", "/tasks/deleted-canonical", "feature/deleted", now)
	addLegacyRepoLifecycleColumns(t, db)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, worktree_path, worktree_branch, position,
			error_message, status, created_at, updated_at, merged_at, deleted_at
		) VALUES (?, ?, ?, 'main', ?, ?, ?, 0, '', 'deleted', ?, ?, NULL, ?)`),
		"env-repo-deleted-canonical", seed.envID, seed.repoID,
		"wt-deleted-canonical", "/tasks/deleted-canonical", "feature/deleted", now, now, now); err != nil {
		t.Fatalf("seed deleted canonical repository row: %v", err)
	}
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-active-session", seed.repoID, "",
		"/tasks/active-session", "feature/active", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	slots := make(map[string]*models.TaskEnvironmentRepo, len(env.Repos))
	for _, envRepo := range env.Repos {
		slots[envRepo.RepositoryID+"\x00"+envRepo.BranchSlug] = envRepo
	}
	deleted := slots[seed.repoID+"\x00main"]
	active := slots[seed.repoID+"\x00"]
	if deleted == nil || deleted.WorktreeID != "wt-deleted-canonical" || deleted.Status != "deleted" {
		t.Fatalf("deleted canonical repository = %+v", deleted)
	}
	if active == nil || active.WorktreeID != "wt-active-session" || active.Status == "deleted" {
		t.Fatalf("active session repository = %+v, normalized repos = %+v", active, env.Repos)
	}
}

func TestCutover_PreservesDeletedSessionWorktreeHistory(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{
		envID:     "env-deleted-session-history",
		taskID:    "task-deleted-session-history",
		sessionID: "sess-deleted-session-history",
	}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, legacySeed{envID: seed.envID, taskID: seed.taskID}, "", "", "", now)
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-deleted-session-history", "repo-deleted-session-history", "",
		"/tasks/deleted-session-history", "feature/deleted", "deleted", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if len(env.Repos) != 1 {
		t.Fatalf("normalized repos = %+v, want deleted history", env.Repos)
	}
	deleted := env.Repos[0]
	if deleted.WorktreeID != "wt-deleted-session-history" || deleted.Status != "deleted" || deleted.DeletedAt == nil {
		t.Fatalf("deleted session worktree = %+v", deleted)
	}
}
