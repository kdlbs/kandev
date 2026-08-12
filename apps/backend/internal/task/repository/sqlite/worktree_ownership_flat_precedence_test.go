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
