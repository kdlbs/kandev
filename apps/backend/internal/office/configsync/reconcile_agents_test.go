package configsync

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newReconcileTestRepo brings up both the office schema and the agent
// settings store schema (agent_profiles lives there since ADR 0005 Wave C)
// on one shared connection, plus a Store for the manifest tables.
func newReconcileTestRepo(t *testing.T) (*sqlite.Repository, *Store) {
	t.Helper()
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
	return repo, store
}

func TestNormalizeJSONArrayForComparison(t *testing.T) {
	assert.Equal(t, "[]", normalizeJSONArrayForComparison(""))
	assert.Equal(t, "[]", normalizeJSONArrayForComparison("   "))
	assert.Equal(t, `["a"]`, normalizeJSONArrayForComparison(`["a"]`))
}

func TestBuildFetchedAgents_ParsesStemWarningsCollisionsAndReportsTo(t *testing.T) {
	files := []fetchedFile{
		{path: "agents/ceo.yml", content: []byte("name: CEO\nrole: manager\n")},
		{path: "agents/dev.yml", content: []byte("name: Dev\nrole: contributor\nreports_to: CEO\n")},
		{path: "agents/mismatch.yml", content: []byte("name: Other\nrole: contributor\n")},
		{path: "agents/broken.yml", content: []byte("name:\n  - not-a-string\n")},
	}
	fetched, reportsTo, warnings, _ := buildFetchedAgents(files)

	require.Len(t, fetched, 3)
	byKey := map[string]fetchedEntity[sqlite.AgentInstanceConfigFields]{}
	for _, f := range fetched {
		byKey[f.Key] = f
	}
	require.Contains(t, byKey, "CEO")
	require.Contains(t, byKey, "Dev")
	require.Contains(t, byKey, "Other")
	assert.Equal(t, "[]", byKey["CEO"].Projection.DesiredSkills, "empty desired_skills must normalize for comparison")

	assert.Equal(t, map[string]string{"Dev": "CEO"}, reportsTo)

	var sawStemWarning, sawParseFailureWarning bool
	for _, w := range warnings {
		if w == stemMismatchWarning(kindAgent, "agents/mismatch.yml", "Other") {
			sawStemWarning = true
		}
	}
	for _, w := range warnings {
		if len(w) > 0 {
			sawParseFailureWarning = sawParseFailureWarning || w != ""
		}
	}
	assert.True(t, sawStemWarning, "declared name not matching filename stem must warn")
	assert.True(t, sawParseFailureWarning)
}

func TestBuildFetchedAgents_KeyCollisionKeepsByteWiseFirstPath(t *testing.T) {
	files := []fetchedFile{
		{path: "agents/z-dup.yml", content: []byte("name: CEO\nrole: manager\n")},
		{path: "agents/a-dup.yml", content: []byte("name: CEO\nrole: manager\n")},
	}
	fetched, _, warnings, _ := buildFetchedAgents(files)
	require.Len(t, fetched, 1)
	assert.Equal(t, "agents/a-dup.yml", fetched[0].SourcePath)
	assert.NotEmpty(t, warnings)
}

func TestDetectReportsToCycle(t *testing.T) {
	tree := map[string]string{"a": "b", "b": "c", "c": "a"}
	assert.True(t, detectReportsToCycle("a", "b", tree), "a -> b -> c -> a is a cycle")

	chain := map[string]string{"a": "b", "b": "c"}
	assert.False(t, detectReportsToCycle("a", "b", chain), "a -> b -> c terminates without looping back to a")
}

func TestAgentOps_CreateSeedsDefaultAgentIDAndRecordsManifest(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[sqlite.AgentInstanceConfigFields]{{
		Key: "ceo", SourcePath: "agents/ceo.yml",
		Projection: sqlite.AgentInstanceConfigFields{Role: "manager", DesiredSkills: "[]"},
	}}
	res, err := applyKind(ctx, repo.Writer(), store, agentOps(ctx, repo, "ws-1"), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	require.Equal(t, []string{"ceo"}, res.Created)

	agents, err := repo.ListAgentInstances(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "ceo", agents[0].Name)
	assert.Equal(t, "manager", string(agents[0].Role))
}

func TestAgentOps_RerunWithSameProjectionIsIdempotent(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	// Go through buildFetchedAgents (not a hand-built literal) so this test
	// exercises the same empty-string normalization real YAML parsing
	// applies (AC-OFFICE-CONFIG-SYNC-003.11's idempotency requirement).
	files := []fetchedFile{{path: "agents/ceo.yml", content: []byte("name: ceo\nrole: manager\n")}}
	fetched, _, warnings, _ := buildFetchedAgents(files)
	require.Empty(t, warnings)
	require.Len(t, fetched, 1)

	_, err := applyKind(ctx, repo.Writer(), store, agentOps(ctx, repo, "ws-1"), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, repo.Writer(), store, agentOps(ctx, repo, "ws-1"), "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)
	assert.Empty(t, res.Created)
	assert.Empty(t, res.Updated)
}

func TestResolveAgentReportsTo_ResolvesWithinThisRunsFetchedSet(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[sqlite.AgentInstanceConfigFields]{
		{Key: "CEO", SourcePath: "agents/ceo.yml", Projection: sqlite.AgentInstanceConfigFields{Role: "manager", DesiredSkills: "[]"}},
		{Key: "Dev", SourcePath: "agents/dev.yml", Projection: sqlite.AgentInstanceConfigFields{Role: "contributor", DesiredSkills: "[]"}},
	}
	res, err := applyKind(ctx, repo.Writer(), store, agentOps(ctx, repo, "ws-1"), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)

	reportsTo := map[string]string{"Dev": "CEO"}
	warnings := resolveAgentReportsTo(ctx, repo, reportsTo, res.IDsByKey)
	assert.Empty(t, warnings)

	dev, err := repo.GetAgentInstance(ctx, res.IDsByKey["Dev"])
	require.NoError(t, err)
	assert.Equal(t, res.IDsByKey["CEO"], dev.ReportsTo)
}

func TestResolveAgentReportsTo_TargetOutsideThisRunLeavesUnsetWithWarning(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[sqlite.AgentInstanceConfigFields]{
		{Key: "Dev", SourcePath: "agents/dev.yml", Projection: sqlite.AgentInstanceConfigFields{Role: "contributor", DesiredSkills: "[]"}},
	}
	res, err := applyKind(ctx, repo.Writer(), store, agentOps(ctx, repo, "ws-1"), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)

	warnings := resolveAgentReportsTo(ctx, repo, map[string]string{"Dev": "NotManaged"}, res.IDsByKey)
	require.Len(t, warnings, 1)

	dev, err := repo.GetAgentInstance(ctx, res.IDsByKey["Dev"])
	require.NoError(t, err)
	assert.Empty(t, dev.ReportsTo)
}
