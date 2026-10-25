package configsync

import (
	"context"
	"fmt"
	"strings"
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
	fetched, reportsTo, warnings, unparsed := buildFetchedAgents(files)

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
		if strings.Contains(w, "agents/broken.yml") {
			sawParseFailureWarning = true
		}
	}
	assert.True(t, sawStemWarning, "declared name not matching filename stem must warn")
	assert.True(t, sawParseFailureWarning, "a file that fails to parse must warn naming it")
	assert.Equal(t, []string{"agents/broken.yml"}, unparsed, "a file that fails to parse must be reported as unparsed")
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

func TestDetectReportsToCycle_DownstreamCycleNotInvolvingKeyIsNotACycleForKey(t *testing.T) {
	// a -> b -> c -> b: b and c cycle with each other, but a is never
	// revisited. Resolving a's own reports_to must not be flagged; b's own
	// resolution (starting at "b") independently catches the real cycle.
	tree := map[string]string{"a": "b", "b": "c", "c": "b"}
	assert.False(t, detectReportsToCycle("a", "b", tree),
		"a's chain wanders into b/c's own cycle without ever returning to a")
	assert.True(t, detectReportsToCycle("b", "c", tree), "b -> c -> b does loop back to b")
}

func TestDetectReportsToCycle_HopCapExhaustionIsNotACycle(t *testing.T) {
	// A genuinely acyclic chain deeper than maxReportsToHops must not be
	// misreported as cyclic; the cap is defensive, not a promise every
	// legitimate chain fits under it.
	tree := make(map[string]string, maxReportsToHops+5)
	for i := range maxReportsToHops + 4 {
		tree[fmt.Sprintf("n%d", i)] = fmt.Sprintf("n%d", i+1)
	}
	assert.False(t, detectReportsToCycle("n0", "n1", tree), "a long acyclic chain must not be reported as a cycle")
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
	warnings, err := resolveAgentReportsTo(ctx, repo, reportsTo, res.IDsByKey, nil)
	require.NoError(t, err)
	assert.Empty(t, warnings)

	dev, err := repo.GetAgentInstance(ctx, res.IDsByKey["Dev"])
	require.NoError(t, err)
	assert.Equal(t, res.IDsByKey["CEO"], dev.ReportsTo)
}

func TestResolveAgentReportsTo_ClearsWhenDeclarationRemoved(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[sqlite.AgentInstanceConfigFields]{
		{Key: "CEO", SourcePath: "agents/ceo.yml", Projection: sqlite.AgentInstanceConfigFields{Role: "manager", DesiredSkills: "[]"}},
		{Key: "Dev", SourcePath: "agents/dev.yml", Projection: sqlite.AgentInstanceConfigFields{Role: "contributor", DesiredSkills: "[]"}},
	}
	res, err := applyKind(ctx, repo.Writer(), store, agentOps(ctx, repo, "ws-1"), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)

	// First run: Dev declares reports_to: CEO.
	warnings, err := resolveAgentReportsTo(ctx, repo, map[string]string{"Dev": "CEO"}, res.IDsByKey, nil)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	dev, err := repo.GetAgentInstance(ctx, res.IDsByKey["Dev"])
	require.NoError(t, err)
	require.Equal(t, res.IDsByKey["CEO"], dev.ReportsTo)

	// Second run: dev.yml no longer declares reports_to at all.
	warnings, err = resolveAgentReportsTo(ctx, repo, map[string]string{}, res.IDsByKey, nil)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	dev, err = repo.GetAgentInstance(ctx, res.IDsByKey["Dev"])
	require.NoError(t, err)
	assert.Empty(t, dev.ReportsTo, "removing reports_to from the file must clear the DB field")
}

func TestResolveAgentReportsTo_ClearsOnNewlyUnresolvableReference(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[sqlite.AgentInstanceConfigFields]{
		{Key: "CEO", SourcePath: "agents/ceo.yml", Projection: sqlite.AgentInstanceConfigFields{Role: "manager", DesiredSkills: "[]"}},
		{Key: "Dev", SourcePath: "agents/dev.yml", Projection: sqlite.AgentInstanceConfigFields{Role: "contributor", DesiredSkills: "[]"}},
	}
	res, err := applyKind(ctx, repo.Writer(), store, agentOps(ctx, repo, "ws-1"), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)

	warnings, err := resolveAgentReportsTo(ctx, repo, map[string]string{"Dev": "CEO"}, res.IDsByKey, nil)
	require.NoError(t, err)
	assert.Empty(t, warnings)

	// Second run: dev.yml now declares a self-reference instead.
	warnings, err = resolveAgentReportsTo(ctx, repo, map[string]string{"Dev": "Dev"}, res.IDsByKey, nil)
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	dev, err := repo.GetAgentInstance(ctx, res.IDsByKey["Dev"])
	require.NoError(t, err)
	assert.Empty(t, dev.ReportsTo, "a self-reference on a later run must clear a previously-resolved value")
}

func TestResolveAgentReportsTo_TargetOutsideThisRunLeavesUnsetWithWarning(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[sqlite.AgentInstanceConfigFields]{
		{Key: "Dev", SourcePath: "agents/dev.yml", Projection: sqlite.AgentInstanceConfigFields{Role: "contributor", DesiredSkills: "[]"}},
	}
	res, err := applyKind(ctx, repo.Writer(), store, agentOps(ctx, repo, "ws-1"), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)

	warnings, err := resolveAgentReportsTo(ctx, repo, map[string]string{"Dev": "NotManaged"}, res.IDsByKey, nil)
	require.NoError(t, err)
	require.Len(t, warnings, 1)

	dev, err := repo.GetAgentInstance(ctx, res.IDsByKey["Dev"])
	require.NoError(t, err)
	assert.Empty(t, dev.ReportsTo)
}

// TestResolveOneAgentReportsTo_DistinguishesFetchedButNotAppliedFromAppearsNowhere
// covers AC-OFFICE-CONFIG-SYNC-003.10b: a reports_to target present in the
// fetched set but not applied this run (its file was unreadable or failed to
// parse) must warn distinctly from a target naming no agent anywhere.
func TestResolveOneAgentReportsTo_DistinguishesFetchedButNotAppliedFromAppearsNowhere(t *testing.T) {
	idsByKey := map[string]string{}
	reportsTo := map[string]string{"dev": "ghost"}

	_, appearsNowhere := resolveOneAgentReportsTo("dev", reportsTo, idsByKey, nil)
	assert.Contains(t, appearsNowhere, "not managed by this sync")

	_, fetchedNotApplied := resolveOneAgentReportsTo("dev", reportsTo, idsByKey, map[string]bool{"ghost": true})
	assert.Contains(t, fetchedNotApplied, "fetched but not applied")
	assert.NotEqual(t, appearsNowhere, fetchedNotApplied, "the two unresolved reasons must produce distinguishable warnings")
}

func TestResolveAgentReportsTo_WriteDatabaseFailureIsReturnedNotWarned(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[sqlite.AgentInstanceConfigFields]{
		{Key: "CEO", SourcePath: "agents/ceo.yml", Projection: sqlite.AgentInstanceConfigFields{Role: "manager", DesiredSkills: "[]"}},
		{Key: "Dev", SourcePath: "agents/dev.yml", Projection: sqlite.AgentInstanceConfigFields{Role: "contributor", DesiredSkills: "[]"}},
	}
	res, err := applyKind(ctx, repo.Writer(), store, agentOps(ctx, repo, "ws-1"), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)

	_, err = repo.ExecRaw(ctx, `
		CREATE TRIGGER fail_reports_to_write BEFORE UPDATE OF reports_to ON agent_profiles
		WHEN NEW.id = '`+res.IDsByKey["Dev"]+`'
		BEGIN
			SELECT RAISE(FAIL, 'forced reports_to write failure for test');
		END;
	`)
	require.NoError(t, err)

	_, err = resolveAgentReportsTo(ctx, repo, map[string]string{"Dev": "CEO"}, res.IDsByKey, nil)
	require.Error(t, err, "a real DB failure writing reports_to must be a run failure, not a swallowed warning")

	dev, err := repo.GetAgentInstance(ctx, res.IDsByKey["Dev"])
	require.NoError(t, err)
	assert.Empty(t, dev.ReportsTo, "the write never committed, so the field must remain unset")
}
