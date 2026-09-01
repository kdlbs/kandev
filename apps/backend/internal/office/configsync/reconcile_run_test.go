package configsync

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		Path:      path,
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

func TestCapWarnings_TruncatesAtLimitWithCountEntry(t *testing.T) {
	warnings := make([]string, maxRecordedWarnings+10)
	for i := range warnings {
		warnings[i] = "w"
	}
	capped := capWarnings(warnings)
	assert.Len(t, capped, maxRecordedWarnings)
	assert.Contains(t, capped[len(capped)-1], "11 further warning")
}
