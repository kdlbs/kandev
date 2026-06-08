package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestGetAgentProfile_DeletedRowIsHidden pins the existing semantic: the
// deleted_at-aware lookup keeps returning sql.ErrNoRows for a soft-deleted
// row. This is the trap that orphaned the watchers in the first place — if
// this assertion ever changes, the watcher self-heal flow needs to be
// rethought.
func TestGetAgentProfile_DeletedRowIsHidden(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	id := seedAgentProfile(t, repo, "deleted-profile", "removed-kilo")

	if err := repo.DeleteAgentProfile(ctx, id); err != nil {
		t.Fatalf("soft-delete failed: %v", err)
	}

	_, err := repo.GetAgentProfile(ctx, id)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

// TestGetAgentProfileIncludingDeleted_ReturnsRowWithDeletedAtSet is the
// counterpart: the new method MUST surface the soft-deleted row so the
// resolver can disambiguate "removed" from "never existed".
func TestGetAgentProfileIncludingDeleted_ReturnsRowWithDeletedAtSet(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	id := seedAgentProfile(t, repo, "deleted-profile", "removed-kilo")

	if err := repo.DeleteAgentProfile(ctx, id); err != nil {
		t.Fatalf("soft-delete failed: %v", err)
	}

	got, err := repo.GetAgentProfileIncludingDeleted(ctx, id)
	if err != nil {
		t.Fatalf("expected row to be returned, got error %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil profile")
	}
	if got.DeletedAt == nil {
		t.Fatal("expected DeletedAt to be set on the returned row")
	}
	if got.Name != "deleted-profile" {
		t.Errorf("Name = %q, want %q", got.Name, "deleted-profile")
	}
}

// TestGetAgentProfileIncludingDeleted_MissingRowStillErrors guards against
// regressions where the includes-deleted variant accidentally silences a
// genuine "row never existed" lookup.
func TestGetAgentProfileIncludingDeleted_MissingRowStillErrors(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, err := repo.GetAgentProfileIncludingDeleted(ctx, "this-id-was-never-created")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

// TestHasDeletedAgentProfiles pins the "has been provisioned before" signal the
// boot-time seeders rely on: false while a profile is live (or never existed),
// true once the user soft-deletes it. This is what stops a deleted profile from
// being resurrected on the next restart.
func TestHasDeletedAgentProfiles(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	id := seedAgentProfile(t, repo, "kilo-default", "kilocode-acp")

	live, err := repo.GetAgentProfileIncludingDeleted(ctx, id)
	if err != nil {
		t.Fatalf("lookup profile: %v", err)
	}
	agentID := live.AgentID

	// Fresh agent with no deleted rows -> false.
	has, err := repo.HasDeletedAgentProfiles(ctx, agentID)
	if err != nil {
		t.Fatalf("HasDeletedAgentProfiles (live): %v", err)
	}
	if has {
		t.Fatal("expected false while the only profile is still live")
	}

	// Unknown agent -> false (not an error).
	has, err = repo.HasDeletedAgentProfiles(ctx, "agent-that-never-existed")
	if err != nil {
		t.Fatalf("HasDeletedAgentProfiles (unknown): %v", err)
	}
	if has {
		t.Fatal("expected false for an agent with no profile rows at all")
	}

	if err := repo.DeleteAgentProfile(ctx, id); err != nil {
		t.Fatalf("soft-delete failed: %v", err)
	}

	has, err = repo.HasDeletedAgentProfiles(ctx, agentID)
	if err != nil {
		t.Fatalf("HasDeletedAgentProfiles (deleted): %v", err)
	}
	if !has {
		t.Fatal("expected true once the agent has a soft-deleted profile")
	}
}

// seedAgentProfile creates a parent agent + a profile referencing it and
// returns the profile id. Centralised so the table+FK setup stays in one
// place even if the schema grows new required columns.
func seedAgentProfile(t *testing.T, repo Repository, profileName, agentName string) string {
	t.Helper()
	ctx := context.Background()
	parent := &models.Agent{Name: agentName}
	if err := repo.CreateAgent(ctx, parent); err != nil {
		t.Fatalf("create parent agent: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID:          parent.ID,
		Name:             profileName,
		AgentDisplayName: agentName,
		Model:            "test-model",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return profile.ID
}
