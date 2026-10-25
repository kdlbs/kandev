package configsync

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/office/models"
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

// fakeSessionTerminator records every TerminateAllForAgent call so a test
// can assert config sync's deletion sweep cascades session termination the
// same way AgentService.DeleteAgentInstance does for the manual path.
type fakeSessionTerminator struct {
	calls   []string
	failIDs map[string]bool
}

func (f *fakeSessionTerminator) TerminateAllForAgent(_ context.Context, agentInstanceID, _ string) error {
	f.calls = append(f.calls, agentInstanceID)
	if f.failIDs[agentInstanceID] {
		return assert.AnError
	}
	return nil
}

func TestReconcile_DeletionSweepCascadesSessionTermination(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{fileEntry("cfg/agents/ceo.yml")}
	fg.files["cfg/agents/ceo.yml"] = []byte("name: ceo\nrole: manager\n")

	runner := NewRunner(fg, nil, repo, store)
	term := &fakeSessionTerminator{}
	runner.SetSessionTerminator(term)
	ctx := context.Background()
	_, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.Empty(t, term.calls, "no deletion happened yet")

	agents, err := repo.ListAgentInstances(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, agents, 1)
	ceoID := agents[0].ID

	fg.dirs["cfg/agents"] = []github.RepoContentEntry{}
	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.Equal(t, []string{"ceo"}, result.Deleted)
	assert.Equal(t, []string{ceoID}, term.calls,
		"a config-sync deletion sweep must cascade-terminate the deleted agent's sessions, "+
			"mirroring AgentService.DeleteAgentInstance's cascade for the manual delete path")
}

func TestReconcile_DeletionSweepSessionTerminationFailureWarnsButDoesNotFailRun(t *testing.T) {
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

	agents, err := repo.ListAgentInstances(ctx, "ws-1")
	require.NoError(t, err)
	ceoID := agents[0].ID

	term := &fakeSessionTerminator{failIDs: map[string]bool{ceoID: true}}
	runner.SetSessionTerminator(term)
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{}
	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err, "the entity delete already committed; a cascade failure must not fail the run")
	assert.Equal(t, []string{"ceo"}, result.Deleted)
	assert.Contains(t, strings.Join(result.Warnings, "\n"), "terminating its sessions failed")
}

func TestReconcile_ForeignCollisionLoserWarnsFetchedButNotAppliedForReportsTo(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")
	ctx := context.Background()

	// An unmanaged agent already holds the name "lead" before this run.
	require.NoError(t, repo.CreateAgentInstance(ctx, &models.AgentInstance{
		WorkspaceID: "ws-1", Name: "lead", Role: "manager",
	}))

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{
		fileEntry("cfg/agents/lead.yml"),
		fileEntry("cfg/agents/dev.yml"),
	}
	// "lead" collides with the unmanaged agent above (decisionForeign);
	// "dev" declares reports_to: lead, so its reference must be warned as
	// "fetched but not applied", not "not managed by this sync"
	// (AC-OFFICE-CONFIG-SYNC-003.10b) — lead's file was fetched this run,
	// it just lost the name collision.
	fg.files["cfg/agents/lead.yml"] = []byte("name: lead\nrole: manager\n")
	fg.files["cfg/agents/dev.yml"] = []byte("name: dev\nrole: contributor\nreports_to: lead\n")

	runner := NewRunner(fg, nil, repo, store)
	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)

	warnings := strings.Join(result.Warnings, "\n")
	assert.Contains(t, warnings, `reports_to "lead" was fetched but not applied`)
	assert.NotContains(t, warnings, `reports_to "lead" is not managed by this sync`)
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

func TestReconcile_RenamedSkillDirectoryWithUnreadableFileIsCoarseExemptNotDeleted(t *testing.T) {
	// AC-OFFICE-CONFIG-SYNC-003.6a: a file both moved and made unreadable
	// since the last run has a new path the manifest never recorded and an
	// old path the listing no longer contains, so neither end matches and
	// the entity must not be deleted for a failure that is not a removal.
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/old-name")}
	fg.dirs["cfg/skills/old-name"] = []github.RepoContentEntry{fileEntry("cfg/skills/old-name/SKILL.md")}
	fg.files["cfg/skills/old-name/SKILL.md"] = []byte("---\nname: old-name\n---\nBody.\n")

	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()
	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	require.Equal(t, []string{"old-name"}, result.Created)

	// The directory is renamed and its SKILL.md becomes unreadable in the
	// same commit: the old manifest entry's path is gone from the listing,
	// and the new directory's path was never recorded in the manifest.
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/new-name")}
	fg.dirs["cfg/skills/new-name"] = []github.RepoContentEntry{fileEntry("cfg/skills/new-name/SKILL.md")}
	// No content registered for cfg/skills/new-name/SKILL.md: 404 on fetch.

	result, err = runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.Empty(t, result.Deleted, "the still-existing skill must not be deleted for an unreadable file that may be its renamed self")
	assert.NotEmpty(t, result.Warnings)

	skills, err := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "old-name", skills[0].Slug, "the entity must survive under its original identity")
}

func TestReconcile_UnreadableReferenceOnKnownSkillDoesNotBlockUnrelatedDeletion(t *testing.T) {
	// AC-OFFICE-CONFIG-SYNC-003.6a's coarse fallback exists because an
	// unreadable file's contents cannot say which entity it defines. That
	// ambiguity is absent when a skill's SKILL.md parses fine: the skill's
	// identity is already known and it applies normally, so an unreadable
	// reference file under it must not suppress deletion of an unrelated
	// skill genuinely removed upstream.
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/keep-me")}
	fg.dirs["cfg/skills/keep-me"] = []github.RepoContentEntry{fileEntry("cfg/skills/keep-me/SKILL.md")}
	fg.files["cfg/skills/keep-me/SKILL.md"] = []byte("---\nname: keep-me\n---\nBody.\n")

	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()
	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	require.Equal(t, []string{"keep-me"}, result.Created)

	// keep-me is genuinely removed upstream. A brand-new, unrelated skill
	// arrives whose SKILL.md parses fine but whose one reference file is
	// oversized (unreadable).
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/new-skill")}
	fg.dirs["cfg/skills/new-skill"] = []github.RepoContentEntry{
		fileEntry("cfg/skills/new-skill/SKILL.md"),
		dirEntry("cfg/skills/new-skill/references"),
	}
	fg.files["cfg/skills/new-skill/SKILL.md"] = []byte("---\nname: new-skill\n---\nBody.\n")
	fg.dirs["cfg/skills/new-skill/references"] = []github.RepoContentEntry{
		fileEntry("cfg/skills/new-skill/references/big.md"),
	}
	fg.files["cfg/skills/new-skill/references/big.md"] = bytes.Repeat([]byte("x"), MaxFileBytes+1)

	result, err = runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.Equal(t, []string{"keep-me"}, result.Deleted, "keep-me was genuinely removed and is unrelated to new-skill's unreadable reference file")
	assert.Equal(t, []string{"new-skill"}, result.Created)

	skills, err := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "new-skill", skills[0].Slug)
}

func TestReconcile_UnreadableSkillMDExemptsOnlyThatEntityFromDeletionWhenPathMatchesManifest(t *testing.T) {
	// Narrow (per-entity) exemption must still work for skills after
	// AC-OFFICE-CONFIG-SYNC-003.6a's coarse fallback was added: an
	// unreadable SKILL.md at a path the manifest already carries must
	// exempt only that skill, not suppress deletion of an unrelated,
	// genuinely removed skill in the same run.
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{
		dirEntry("cfg/skills/flaky"),
		dirEntry("cfg/skills/removed"),
	}
	fg.dirs["cfg/skills/flaky"] = []github.RepoContentEntry{fileEntry("cfg/skills/flaky/SKILL.md")}
	fg.files["cfg/skills/flaky/SKILL.md"] = []byte("---\nname: flaky\n---\nBody.\n")
	fg.dirs["cfg/skills/removed"] = []github.RepoContentEntry{fileEntry("cfg/skills/removed/SKILL.md")}
	fg.files["cfg/skills/removed/SKILL.md"] = []byte("---\nname: removed\n---\nBody.\n")

	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()
	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"flaky", "removed"}, result.Created)

	// flaky's SKILL.md stays listed at its known path but becomes
	// unreadable-content; removed's directory disappears entirely.
	delete(fg.files, "cfg/skills/flaky/SKILL.md")
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/flaky")}

	result, err = runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.Equal(t, []string{"removed"}, result.Deleted, "removed must still be deleted; flaky's unreadable SKILL.md must exempt only flaky")

	skills, err := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "flaky", skills[0].Slug)
}

// TestReconcile_SkillDirectoryDisappearingWithinRunExemptsFromDeletion covers
// the race between the round-1 skills/ listing (which still shows the
// directory) and the round-2 listing of that same directory (which 404s):
// unlike a genuine removal, where the parent listing itself stops naming
// the directory, this must be treated as unreadable and exempt the skill
// from this run's deletion sweep rather than deleting a still-real,
// previously managed skill.
func TestReconcile_SkillDirectoryDisappearingWithinRunExemptsFromDeletion(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/reviewer")}
	fg.dirs["cfg/skills/reviewer"] = []github.RepoContentEntry{fileEntry("cfg/skills/reviewer/SKILL.md")}
	fg.files["cfg/skills/reviewer/SKILL.md"] = []byte("---\nname: reviewer\n---\nBody.\n")

	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()
	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	require.Equal(t, []string{"reviewer"}, result.Created)

	// The parent listing still names "reviewer" as an existing directory
	// entry, but listing the directory itself now 404s.
	delete(fg.dirs, "cfg/skills/reviewer")

	result, err = runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.Empty(t, result.Deleted, "reviewer must survive: a within-run disappearance is a race, not a confirmed removal")
	assert.NotEmpty(t, result.Warnings)

	skills, err := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "reviewer", skills[0].Slug)
}

func TestReconcile_OrdinarySkillDeletionIsNotSuppressedByDefault(t *testing.T) {
	// Negative control: with zero unreadable/unparsed skill files anywhere,
	// skillCoarse must default to false and a genuinely removed skill must
	// still delete. Guards against a regression that always suppresses
	// skill deletion regardless of cause.
	repo, store := newReconcileTestRepo(t)
	seedTestConfig(t, store, "ws-1", "cfg")

	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{
		dirEntry("cfg/skills/keep"),
		dirEntry("cfg/skills/remove"),
	}
	fg.dirs["cfg/skills/keep"] = []github.RepoContentEntry{fileEntry("cfg/skills/keep/SKILL.md")}
	fg.files["cfg/skills/keep/SKILL.md"] = []byte("---\nname: keep\n---\nBody.\n")
	fg.dirs["cfg/skills/remove"] = []github.RepoContentEntry{fileEntry("cfg/skills/remove/SKILL.md")}
	fg.files["cfg/skills/remove/SKILL.md"] = []byte("---\nname: remove\n---\nBody.\n")

	runner := NewRunner(fg, nil, repo, store)
	ctx := context.Background()
	_, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)

	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/keep")}
	result, err := runner.Reconcile(ctx, newRunnerTestConfig("ws-1", "cfg"))
	require.NoError(t, err)
	assert.Equal(t, []string{"remove"}, result.Deleted)
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
