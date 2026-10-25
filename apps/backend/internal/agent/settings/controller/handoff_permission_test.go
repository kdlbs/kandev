package controller

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

// TestAgentProfileBelongsToWorkspace exercises the three-way membership
// predicate the handoff action's AC-14b relies on: a global profile belongs
// everywhere, a profile scoped to the target workspace belongs there, and a
// profile scoped to a *different* workspace (source-scoped, not the target)
// is refused rather than silently allowed. This last case is the one an
// existence-only implementation gets wrong.
func TestAgentProfileBelongsToWorkspace(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()

	global := &models.AgentProfile{AgentID: "agent-1", Name: "Global profile", Model: "model-a"}
	if err := repo.CreateAgentProfile(ctx, global); err != nil {
		t.Fatalf("seed global profile: %v", err)
	}
	targetScoped := &models.AgentProfile{AgentID: "agent-1", Name: "Target-scoped profile", Model: "model-a", WorkspaceID: "ws-target"}
	if err := repo.CreateAgentProfile(ctx, targetScoped); err != nil {
		t.Fatalf("seed target-scoped profile: %v", err)
	}
	sourceScoped := &models.AgentProfile{AgentID: "agent-1", Name: "Source-scoped profile", Model: "model-a", WorkspaceID: "ws-source"}
	if err := repo.CreateAgentProfile(ctx, sourceScoped); err != nil {
		t.Fatalf("seed source-scoped profile: %v", err)
	}

	tests := []struct {
		name      string
		profileID string
		want      bool
	}{
		{"empty profile id is refused, not treated as global", "", false},
		{"unknown profile id is refused", "does-not-exist", false},
		{"global (empty WorkspaceID) profile belongs to any workspace", global.ID, true},
		{"profile scoped to the target workspace belongs there", targetScoped.ID, true},
		{"profile scoped to a different (source) workspace does not belong to the target", sourceScoped.ID, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ctrl.AgentProfileBelongsToWorkspace(ctx, tt.profileID, "ws-target")
			if err != nil {
				t.Fatalf("AgentProfileBelongsToWorkspace() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Errorf("AgentProfileBelongsToWorkspace(%q, \"ws-target\") = %v, want %v", tt.profileID, got, tt.want)
			}
		})
	}
}

// TestAgentProfileBelongsToWorkspace_ReadErrorIsNotFoldedIntoFalse proves the
// documented distinction between "does not belong" (false, nil) and "the
// lookup itself failed" (false, non-nil): a genuine read error must reach
// the caller as an error, not be indistinguishable from a valid refusal.
func TestAgentProfileBelongsToWorkspace_ReadErrorIsNotFoldedIntoFalse(t *testing.T) {
	ctrl := &Controller{repo: failingAgentProfileRepo{}}

	_, err := ctrl.AgentProfileBelongsToWorkspace(context.Background(), "any-id", "ws-target")
	if err == nil {
		t.Fatal("error = nil, want a propagated read error")
	}
	if errors.Is(err, sql.ErrNoRows) {
		t.Errorf("error = %v, want a genuine failure, not sql.ErrNoRows folded through", err)
	}
}

// failingAgentProfileRepo implements only the store.Repository method
// AgentProfileBelongsToWorkspace needs, returning a non-sql.ErrNoRows
// failure from GetAgentProfile to prove it is not folded into false.
type failingAgentProfileRepo struct {
	store.Repository
}

func (failingAgentProfileRepo) GetAgentProfile(_ context.Context, _ string) (*models.AgentProfile, error) {
	return nil, errors.New("database is unavailable")
}
