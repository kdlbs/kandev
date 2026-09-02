package configsync

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seedTestConfig creates the config row Reconcile's RecordSyncStatus writes
// through, mirroring what the (not-yet-written) HTTP handlers do before
// ever calling Reconcile.
func seedTestConfig(t *testing.T, store *Store, workspaceID string, path string) {
	t.Helper()
	_, err := store.UpsertConfigForWorkspace(context.Background(), workspaceID, &SetConfigRequest{
		Provider:  ProviderGitHub,
		RepoOwner: "acme",
		RepoName:  "kandev-config",
		Branch:    "main",
		Path:      &path,
	})
	require.NoError(t, err)
}

func newRunnerTestConfig(workspaceID, path string) *Config {
	return &Config{
		WorkspaceID: workspaceID,
		Provider:    ProviderGitHub,
		RepoOwner:   "acme",
		RepoName:    "kandev-config",
		Branch:      "main",
		Path:        path,
	}
}

func TestReconcile_HappyPathCreatesEveryKindAndRecordsSuccess(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{fileEntry("cfg/kandev.yml")}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{fileEntry("cfg/agents/ceo.yml")}
	fg.files["cfg/agents/ceo.yml"] = []byte("name: ceo\nrole: manager\n")
	fg.dirs["cfg/projects"] = []github.RepoContentEntry{fileEntry("cfg/projects/website.yml")}
	fg.files["cfg/projects/website.yml"] = []byte("name: website\ncolor: blue\n")
	fg.dirs["cfg/routines"] = []github.RepoContentEntry{fileEntry("cfg/routines/nightly.yml")}
	fg.files["cfg/routines/nightly.yml"] = []byte("name: nightly\ntask_template: run tests\n")
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/reviewer")}
	fg.dirs["cfg/skills/reviewer"] = []github.RepoContentEntry{fileEntry("cfg/skills/reviewer/SKILL.md")}
	fg.files["cfg/skills/reviewer/SKILL.md"] = []byte("---\nname: reviewer\n---\nBody.\n")

	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()
	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.ElementsMatch(t, []string{"ceo", "website", "nightly", "reviewer"}, result.Created)
	assert.False(t, result.Unchanged)

	cfg, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.LastOk)
	assert.Empty(t, cfg.LastError)
	assert.NotEmpty(t, cfg.LastHash)

	agents, err := repo.ListAgentInstances(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "ceo", agents[0].Name)

	skills, err := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "reviewer", skills[0].Slug)
}

func TestReconcile_RerunWithNoUpstreamChangesIsUnchanged(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{fileEntry("cfg/agents/ceo.yml")}
	fg.files["cfg/agents/ceo.yml"] = []byte("name: ceo\nrole: manager\n")

	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()
	_, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)

	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.Empty(t, result.Created)
	assert.Empty(t, result.Updated)
	assert.Empty(t, result.Deleted)
	assert.True(t, result.Unchanged)
}

func TestReconcile_RemovingUpstreamFileDeletesEntity(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{fileEntry("cfg/agents/ceo.yml")}
	fg.files["cfg/agents/ceo.yml"] = []byte("name: ceo\nrole: manager\n")

	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()
	_, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)

	fg.dirs["cfg/agents"] = []github.RepoContentEntry{}
	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.Equal(t, []string{"ceo"}, result.Deleted)

	agents, err := repo.ListAgentInstances(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, agents)
}

func TestReconcile_WalkFailureRecordsFailureAndReturnsError(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub() // no "cfg" dir registered: root listing 404s.
	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()

	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.Error(t, err)
	assert.Nil(t, result)

	cfg, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.False(t, cfg.LastOk)
	assert.NotEmpty(t, cfg.LastError)
}

func TestReconcile_FailureIsRecordedEvenWhenCallerContextAlreadyExpired(t *testing.T) {
	// AC-OFFICE-CONFIG-SYNC-004.4a: a run abandoned at its deadline must
	// still be recorded as a failure. The status write cannot reuse the
	// run's own (expired) context, or the write itself fails silently.
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub() // no "cfg" dir registered: root listing 404s.
	runner := NewRunner(fg, nil, repo, store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // expired/canceled before Reconcile does any work

	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.Error(t, err)
	assert.Nil(t, result)

	cfg, getErr := store.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, getErr)
	require.NotNil(t, cfg)
	assert.False(t, cfg.LastOk)
	assert.NotEmpty(t, cfg.LastError, "the failure must be recorded even though the caller's context was already done")
}

func TestReconcile_ApplyFailureRecordsWarningsAccumulatedBeforeIt(t *testing.T) {
	// AC-OFFICE-CONFIG-SYNC-004.5b: when a later kind's write fails mid-apply,
	// warnings already produced by prior parse/fetch phases must still be
	// recorded with the failure, not discarded.
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, _, err = settingsstore.Provide(db, db, nil)
	require.NoError(t, err)
	repo, err := sqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	store, err := NewStore(db, db)
	require.NoError(t, err)

	// Simulates a write failure for one specific agent name, so the run
	// fails partway through the agent kind after another agent file has
	// already produced a parse-phase warning during buildKindsFetch.
	_, err = db.Exec(`
		CREATE TRIGGER fail_agent_boom BEFORE INSERT ON agent_profiles
		WHEN NEW.name = 'boom'
		BEGIN
			SELECT RAISE(FAIL, 'forced failure for test');
		END;
	`)
	require.NoError(t, err)

	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{
		fileEntry("cfg/agents/other.yml"),
		fileEntry("cfg/agents/boom.yml"),
	}
	fg.files["cfg/agents/other.yml"] = []byte("name: Mismatch\nrole: manager\n")
	fg.files["cfg/agents/boom.yml"] = []byte("name: boom\nrole: contributor\n")

	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()

	result, reconcileErr := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.Error(t, reconcileErr)
	assert.Nil(t, result)

	cfg, getErr := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, getErr)
	require.NotNil(t, cfg)
	assert.False(t, cfg.LastOk)
	require.NotEmpty(t, cfg.LastWarnings, "the parse-phase warning produced before the failing kind must survive")

	found := false
	for _, w := range cfg.LastWarnings {
		if strings.Contains(w, "other.yml") {
			found = true
		}
	}
	assert.True(t, found, "expected the stem-mismatch warning for other.yml among recorded warnings, got %v", cfg.LastWarnings)
}

func TestReconcile_UnreadableAgentFileExemptsOnlyThatEntityFromDeletion(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{
		fileEntry("cfg/agents/ceo.yml"),
		fileEntry("cfg/agents/dev.yml"),
	}
	fg.files["cfg/agents/ceo.yml"] = []byte("name: ceo\nrole: manager\n")
	fg.files["cfg/agents/dev.yml"] = []byte("name: dev\nrole: contributor\n")

	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()
	_, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)

	// dev.yml is still listed (so its manifest entry can be path-matched)
	// but becomes unreadable-content on fetch, which must exempt only
	// "dev" from this run's deletion sweep, not every agent.
	delete(fg.files, "cfg/agents/dev.yml")

	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.Empty(t, result.Deleted)
	assert.NotEmpty(t, result.Warnings)

	agents, err := repo.ListAgentInstances(ctx, "ws-1")
	require.NoError(t, err)
	assert.Len(t, agents, 2, "both agents must survive: dev is exempt, ceo was never removed")
}

// TestApplyKindSplit_CreatesOnlyThenDeletesOnlyMatchesApplyKind guards the
// AC-OFFICE-CONFIG-SYNC-003.9 refactor: applyKindCreatesOnly followed by
// applyKindDeletesOnly (what the orchestrator calls, separated so every
// kind's creates run before any kind's deletes) must behave identically to
// the combined applyKind a single kind's own tests already exercise.
func TestApplyKindSplit_CreatesOnlyThenDeletesOnlyMatchesApplyKind(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[routineProjection]{{
		Key: "Nightly", SourcePath: "routines/nightly.yml",
		Projection: routineProjection{TaskTemplate: "run tests"},
	}}
	createRes, err := applyKindCreatesOnly(ctx, repo.Writer(), store, routineOps(repo), "ws-1", fetched, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"Nightly"}, createRes.Created)

	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	// Nothing fetched this pass: the manifest-only key must be deleted, same
	// as applyKind's combined behavior (TestRoutineOps_RemovedUpstreamDeletesEntity).
	require.NoError(t, applyKindDeletesOnly(ctx, repo.Writer(), store, routineOps(repo), "ws-1", nil, manifest, nil, false, createRes))
	assert.Equal(t, []string{"Nightly"}, createRes.Deleted)

	routines, err := repo.ListRoutines(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, routines)
}

func TestCapWarnings_TruncatesAtLimitWithCountEntry(t *testing.T) {
	warnings := make([]string, maxRecordedWarnings+10)
	for i := range warnings {
		warnings[i] = "w"
	}
	capped := capWarnings(warnings)
	assert.Len(t, capped, maxRecordedWarnings)
	assert.Contains(t, capped[len(capped)-1], "11 further warning")
}
