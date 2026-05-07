package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestEnvVars_RoundTrip persists a profile with mixed value-only and
// secret-id env vars, reads it back, and checks every field round-trips.
func TestEnvVars_RoundTrip(t *testing.T) {
	t.Parallel()
	repo, ctx := newTestEnvVarsRepo(t)

	agent := &models.Agent{
		ID:          "agent-claude",
		Name:        "claude-acp",
		SupportsMCP: true,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	profile := &models.AgentProfile{
		ID:               "profile-the-one",
		AgentID:          agent.ID,
		Name:             "TheOne",
		AgentDisplayName: "Claude",
		Model:            "opus",
		EnvVars: []models.EnvVar{
			{Key: "THEONE_ROOT", Value: "/src/theone"},
			{Key: "JIRA_TOKEN", SecretID: "secret-1"},
		},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("CreateAgentProfile: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfile: %v", err)
	}
	if !reflect.DeepEqual(got.EnvVars, profile.EnvVars) {
		t.Errorf("EnvVars round-trip mismatch:\n got:  %#v\n want: %#v", got.EnvVars, profile.EnvVars)
	}
}

// TestEnvVars_UpdateReplacesList confirms an update overwrites the entire
// env_vars list (no merging) — which matches the API contract where a
// non-nil EnvVars on UpdateProfile means "replace".
func TestEnvVars_UpdateReplacesList(t *testing.T) {
	t.Parallel()
	repo, ctx := newTestEnvVarsRepo(t)

	agent := &models.Agent{ID: "agent-1", Name: "claude-acp", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	profile := &models.AgentProfile{
		ID:      "p1",
		AgentID: agent.ID,
		Name:    "p",
		EnvVars: []models.EnvVar{{Key: "A", Value: "1"}, {Key: "B", Value: "2"}},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("CreateAgentProfile: %v", err)
	}

	profile.EnvVars = []models.EnvVar{{Key: "C", Value: "3"}}
	if err := repo.UpdateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("UpdateAgentProfile: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfile: %v", err)
	}
	if !reflect.DeepEqual(got.EnvVars, profile.EnvVars) {
		t.Errorf("EnvVars not replaced:\n got:  %#v\n want: %#v", got.EnvVars, profile.EnvVars)
	}
}

// TestEnvVars_EmptyListPersists ensures an explicitly empty list survives
// round-trip as an empty slice, not nil — important for the "user cleared
// all env vars" UI state.
func TestEnvVars_EmptyListPersists(t *testing.T) {
	t.Parallel()
	repo, ctx := newTestEnvVarsRepo(t)

	agent := &models.Agent{ID: "agent-1", Name: "claude-acp", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	profile := &models.AgentProfile{
		ID:      "p1",
		AgentID: agent.ID,
		Name:    "p",
		EnvVars: []models.EnvVar{},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("CreateAgentProfile: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfile: %v", err)
	}
	if got.EnvVars == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(got.EnvVars) != 0 {
		t.Errorf("expected zero env vars, got %d: %#v", len(got.EnvVars), got.EnvVars)
	}
}

// TestEnvVars_NilTreatedAsEmpty confirms that a nil EnvVars on a fresh
// profile reads back as an empty slice (not nil) so downstream code never
// needs a nil check.
func TestEnvVars_NilTreatedAsEmpty(t *testing.T) {
	t.Parallel()
	repo, ctx := newTestEnvVarsRepo(t)

	agent := &models.Agent{ID: "agent-1", Name: "claude-acp", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	profile := &models.AgentProfile{
		ID:      "p1",
		AgentID: agent.ID,
		Name:    "p",
		// EnvVars deliberately nil
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("CreateAgentProfile: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfile: %v", err)
	}
	if got.EnvVars == nil {
		t.Fatal("expected empty slice, got nil")
	}
}

func newTestEnvVarsRepo(t *testing.T) (Repository, context.Context) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sqlx.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := newSQLiteRepository(db, db, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository: %v", err)
	}
	return repo, context.Background()
}
