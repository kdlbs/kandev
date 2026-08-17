package backendapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

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
	_ = createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto") // the "Copy"

	match := buildAgentProfileMatcher(repos, testLogger(t))
	got := match("Claude", "opus[1m]", "auto")
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
	got := match("Claude", "opus[1m]", "auto")
	if got != "" {
		t.Fatalf("matcher returned %q, want empty (disabled profile must never match)", got)
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
	got := match("Claude", "opus[1m]", "auto")
	if got != "" {
		t.Fatalf("matcher returned %q, want empty (workspace-scoped profile must never match)", got)
	}
}

func TestBuildAgentProfileMatcher_SingleMatchUnchanged(t *testing.T) {
	repos, _ := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)

	p := createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")

	match := buildAgentProfileMatcher(repos, testLogger(t))
	got := match("Claude", "opus[1m]", "auto")
	if got != p.ID {
		t.Fatalf("matcher returned %q, want %q", got, p.ID)
	}
}

func TestBuildAgentProfileMatcher_NoMatch(t *testing.T) {
	repos, _ := newMatcherTestRepos(t)
	agentID := createMatcherTestAgent(t, repos)
	createMatcherTestProfile(t, repos, agentID, "Claude", "opus[1m]", "auto")

	match := buildAgentProfileMatcher(repos, testLogger(t))
	got := match("Claude", "sonnet[1m]", "auto")
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
		got := match("Claude", "opus[1m]", "auto")
		if got != want {
			t.Fatalf("call %d: matcher returned %q, want stable %q", i, got, want)
		}
	}
}
