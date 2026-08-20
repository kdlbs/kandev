package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// AC-29: the automations YAML export opens one read transaction spanning
// several stores. GetAgentProfileTx is GetAgentProfile's counterpart that
// accepts the caller's *sqlx.Tx instead of opening its own, and reports a
// three-outcome (value, found, err) result so a missing profile is
// distinguishable from a lookup failure without string-matching an error.

func TestGetAgentProfileTx_FoundAndMissing(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	profile := &models.AgentProfile{AgentID: "agent-1", Name: "Default"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("CreateAgentProfile: %v", err)
	}

	sqliteRepo, ok := repo.(*sqliteRepository)
	if !ok {
		t.Fatalf("repo is %T, want *sqliteRepository", repo)
	}
	tx, err := sqliteRepo.ro.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	got, found, err := repo.GetAgentProfileTx(ctx, tx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfileTx(%s): %v", profile.ID, err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if got == nil || got.ID != profile.ID {
		t.Errorf("profile = %+v, want ID %s", got, profile.ID)
	}

	missing, found, err := repo.GetAgentProfileTx(ctx, tx, "does-not-exist")
	if err != nil {
		t.Fatalf("GetAgentProfileTx(missing): %v", err)
	}
	if found {
		t.Error("found = true, want false")
	}
	if missing != nil {
		t.Errorf("profile = %+v, want nil", missing)
	}
}
