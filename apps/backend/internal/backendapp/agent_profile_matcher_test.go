package backendapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

// newMatcherTestRepos opens an isolated SQLite-backed agent settings store for
// buildAgentProfileMatcher tests, along with the raw *sqlx.DB handle so a test
// can force specific CreatedAt values that the store API always overwrites.
func newMatcherTestRepos(t *testing.T) (*Repositories, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "matcher.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	repo, cleanup, err := settingsstore.Provide(db, db, log)
	if err != nil {
		t.Fatalf("settings store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	return &Repositories{AgentSettings: repo}, db
}

func createMatcherTestAgent(t *testing.T, repos *Repositories) string {
	t.Helper()
	agent := &agentsettingsmodels.Agent{Name: "claude-" + uuid.New().String()}
	if err := repos.AgentSettings.CreateAgent(context.Background(), agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	return agent.ID
}

// createMatcherTestProfile creates a profile with the given (agent display
// name, model, mode) triple, applying any overrides after creation.
func createMatcherTestProfile(t *testing.T, repos *Repositories, agentID, displayName, model, mode string) *agentsettingsmodels.AgentProfile {
	t.Helper()
	p := &agentsettingsmodels.AgentProfile{
		AgentID:          agentID,
		Name:             "Profile " + uuid.New().String(),
		AgentDisplayName: displayName,
		Model:            model,
		Mode:             mode,
	}
	if err := repos.AgentSettings.CreateAgentProfile(context.Background(), p); err != nil {
		t.Fatalf("CreateAgentProfile: %v", err)
	}
	return p
}

// TestBuildAgentProfileMatcher_DuplicateDoesNotStealBinding is the reported
// regression: duplicating a profile produces a byte-identical
// (agent_display_name, model, mode) triple, and the copy's CreatedAt is
// always later than its source. The matcher must keep resolving to the
// original (oldest) profile, not the newer copy.
func TestBuildAgentProfileMatcher_DuplicateDoesNotStealBinding(t *testing.T) {
	repos, _ := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)

	source := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")
	copyProfile := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto") // the "Copy"
	require.True(t, source.CreatedAt.Before(copyProfile.CreatedAt),
		"precondition: the copy must be strictly newer than the source, or this test stops proving the regression")

	match := buildAgentProfileMatcher(repos, testLogger(t))
	got := match("Claude", "opus[1m]", "auto", "")
	if got != source.ID {
		t.Fatalf("matcher returned %q, want source profile %q", got, source.ID)
	}
}

func TestBuildAgentProfileMatcher_SkipsDisabledProfile(t *testing.T) {
	repos, _ := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)

	p := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")
	if _, err := repos.AgentSettings.UpdateAgentProfileEnabled(context.Background(), p.ID, false); err != nil {
		t.Fatalf("disable profile: %v", err)
	}

	match := buildAgentProfileMatcher(repos, testLogger(t))
	got := match("Claude", "opus[1m]", "auto", "")
	if got != "" {
		t.Fatalf("matcher returned %q, want empty (disabled profile must never match)", got)
	}
}

// TestBuildAgentProfileMatcher_PreservesDisabledExistingBinding is the
// reconciliation regression: a workflow step already bound to a profile that
// gets disabled after the fact must keep that binding on the next sync, per
// docs/specs/agents/requirements/profile-disable.md ("existing sessions/workflows keep
// their profile"). Excluding disabled profiles from *candidate selection*
// must not also strip a binding that was already in place.
func TestBuildAgentProfileMatcher_PreservesDisabledExistingBinding(t *testing.T) {
	repos, _ := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)

	p := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")
	if _, err := repos.AgentSettings.UpdateAgentProfileEnabled(context.Background(), p.ID, false); err != nil {
		t.Fatalf("disable profile: %v", err)
	}

	match := buildAgentProfileMatcher(repos, testLogger(t))
	got := match("Claude", "opus[1m]", "auto", p.ID)
	if got != p.ID {
		t.Fatalf("matcher returned %q, want existing binding %q preserved despite disablement", got, p.ID)
	}
}

// TestBuildAgentProfileMatcher_CurrentIDDriftFallsBackToCandidateSelection
// covers the case where the currently-bound profile no longer matches the
// descriptor (e.g. the profile itself was edited) - the matcher must not
// blindly trust currentID, it must fall through to normal candidate
// selection.
func TestBuildAgentProfileMatcher_CurrentIDDriftFallsBackToCandidateSelection(t *testing.T) {
	repos, _ := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)

	stale := createMatcherTestProfile(t, repos, agentID, "Claude", "sonnet[1m]", "auto")
	fresh := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")

	match := buildAgentProfileMatcher(repos, testLogger(t))
	got := match("Claude", "opus[1m]", "auto", stale.ID)
	if got != fresh.ID {
		t.Fatalf("matcher returned %q, want %q (currentID no longer matches the descriptor)", got, fresh.ID)
	}
}

func TestBuildAgentProfileMatcher_SkipsWorkspaceScopedProfile(t *testing.T) {
	repos, db := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)

	p := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")
	if _, err := db.Exec(`UPDATE agent_profiles SET workspace_id = ? WHERE id = ?`, "workspace-1", p.ID); err != nil {
		t.Fatalf("set workspace_id: %v", err)
	}

	match := buildAgentProfileMatcher(repos, testLogger(t))
	got := match("Claude", "opus[1m]", "auto", "")
	if got != "" {
		t.Fatalf("matcher returned %q, want empty (workspace-scoped profile must never match)", got)
	}
}

func TestBuildAgentProfileMatcher_SingleMatchUnchanged(t *testing.T) {
	repos, _ := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)

	p := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")

	match := buildAgentProfileMatcher(repos, testLogger(t))
	got := match("Claude", "opus[1m]", "auto", "")
	if got != p.ID {
		t.Fatalf("matcher returned %q, want %q", got, p.ID)
	}
}

func TestBuildAgentProfileMatcher_NoMatch(t *testing.T) {
	repos, _ := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)
	createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")

	match := buildAgentProfileMatcher(repos, testLogger(t))
	got := match("Claude", "sonnet[1m]", "auto", "")
	if got != "" {
		t.Fatalf("matcher returned %q, want empty", got)
	}
}

// TestBuildAgentProfileMatcher_EqualCreatedAtIsStable forces two candidates
// to share an identical CreatedAt (which the store API's time.Now() calls
// would otherwise make astronomically unlikely) and asserts the ID tiebreak
// makes the result deterministic across repeated calls.
func TestBuildAgentProfileMatcher_EqualCreatedAtIsStable(t *testing.T) {
	repos, db := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)

	a := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")
	b := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")
	if _, err := db.Exec(`UPDATE agent_profiles SET created_at = (SELECT created_at FROM agent_profiles WHERE id = ?) WHERE id = ?`, a.ID, b.ID); err != nil {
		t.Fatalf("force equal created_at: %v", err)
	}

	want := a.ID
	if b.ID < a.ID {
		want = b.ID
	}

	match := buildAgentProfileMatcher(repos, testLogger(t))
	for i := 0; i < 5; i++ {
		got := match("Claude", "opus[1m]", "auto", "")
		if got != want {
			t.Fatalf("call %d: matcher returned %q, want stable %q", i, got, want)
		}
	}
}

// TestBuildAgentProfileMatcher_MultipleCandidatesLogsDebugWithFields proves
// the ambiguity signal described in buildAgentProfileMatcher's doc comment:
// when more than one candidate matches, it logs at Debug with the triple,
// the candidate count, and the profile it actually selected.
func TestBuildAgentProfileMatcher_MultipleCandidatesLogsDebugWithFields(t *testing.T) {
	repos, _ := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)

	a := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")
	createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto") // newer candidate, must not be selected

	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	match := buildAgentProfileMatcher(repos, log)
	got := match("Claude", "opus[1m]", "auto", "")
	if got != a.ID {
		t.Fatalf("matcher returned %q, want oldest profile %q", got, a.ID)
	}

	entries := logs.FilterMessage("agent profile matcher: multiple candidates for workflow step sync").All()
	require.Len(t, entries, 1, "an ambiguous match must log exactly one Debug entry")
	fields := entries[0].ContextMap()
	assert.Equal(t, "Claude", fields["agent_display_name"])
	assert.Equal(t, "opus[1m]", fields["model"])
	assert.Equal(t, "auto", fields["mode"])
	assert.Equal(t, int64(2), fields["candidates"])
	assert.Equal(t, a.ID, fields["selected_profile_id"])
}
