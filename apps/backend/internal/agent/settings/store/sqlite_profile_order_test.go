package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestListAgentProfiles_UsesSavedOrderAndNewestFirstDefault(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "profile-order"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	profiles := []*models.AgentProfile{
		{AgentID: agent.ID, Name: "older", Model: "model"},
		{AgentID: agent.ID, Name: "newer", Model: "model"},
	}
	for _, profile := range profiles {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != profiles[1].ID {
		t.Fatalf("default ordering = %#v, want newest profile first", got)
	}
	updatedAtByID := map[string]time.Time{got[0].ID: got[0].UpdatedAt, got[1].ID: got[1].UpdatedAt}
	revision, changed, err := repo.(*sqliteRepository).ReorderAgentProfiles(ctx, agent.ID, []string{profiles[0].ID, profiles[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || revision != 1 {
		t.Fatalf("reorder = revision %d, changed %t, want revision 1 changed", revision, changed)
	}
	got, err = repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != profiles[0].ID || got[1].ID != profiles[1].ID {
		t.Fatalf("saved ordering = %#v, want older then newer", got)
	}
	for _, profile := range got {
		if !profile.UpdatedAt.Equal(updatedAtByID[profile.ID]) {
			t.Fatalf("profile %s updated_at changed during reorder: %v -> %v", profile.ID, updatedAtByID[profile.ID], profile.UpdatedAt)
		}
	}
	revision, changed, err = repo.(*sqliteRepository).ReorderAgentProfiles(ctx, agent.ID, []string{profiles[0].ID, profiles[1].ID})
	if err != nil || changed || revision != 1 {
		t.Fatalf("unchanged reorder = revision %d, changed %t, err %v; want revision 1 unchanged", revision, changed, err)
	}
	if _, _, err := repo.(*sqliteRepository).ReorderAgentProfiles(ctx, agent.ID, []string{profiles[0].ID, "foreign"}); err != ErrProfileOrderSetMismatch {
		t.Fatalf("mismatched reorder error = %v, want ErrProfileOrderSetMismatch", err)
	}
	if _, _, err := repo.(*sqliteRepository).ReorderAgentProfiles(ctx, agent.ID, []string{profiles[0].ID, profiles[0].ID}); err != ErrProfileOrderSetMismatch {
		t.Fatalf("duplicate reorder error = %v, want ErrProfileOrderSetMismatch", err)
	}
	created := &models.AgentProfile{AgentID: agent.ID, Name: "created-after-order", Model: "model"}
	if err := repo.CreateAgentProfile(ctx, created); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.GetAgentProfileOrderSnapshots(ctx, []string{agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot[agent.ID].Profiles[0].ID != created.ID || snapshot[agent.ID].Revision != 1 {
		t.Fatalf("create snapshot = %#v, want new profile first at revision 1", snapshot[agent.ID])
	}
	if err := repo.DeleteAgentProfile(ctx, profiles[0].ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err = repo.GetAgentProfileOrderSnapshots(ctx, []string{agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot[agent.ID].Profiles) != 2 || snapshot[agent.ID].Profiles[1].ID != profiles[1].ID || snapshot[agent.ID].Revision != 1 {
		t.Fatalf("delete snapshot = %#v, want surviving saved order at revision 1", snapshot[agent.ID])
	}

}

func TestReorderAgentProfilesMissingAgentIsNotAStaleMembershipSet(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	_, _, err := repo.ReorderAgentProfiles(context.Background(), "missing-agent", nil)
	if err == nil || !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("reorder missing agent error = %v, want missing-agent error", err)
	}
}

func TestProfileMembershipReadPreservesCancellation(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	agent := &models.Agent{Name: "cancelled-membership"}
	if err := repo.CreateAgent(context.Background(), agent); err != nil {
		t.Fatal(err)
	}
	profile := &models.AgentProfile{AgentID: agent.ID, Name: "Profile", Model: "model"}
	if err := repo.CreateAgentProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, operation := range map[string]func() error{
		"delete": func() error { return repo.DeleteAgentProfile(ctx, profile.ID) },
		"update": func() error { return repo.UpdateAgentProfile(ctx, profile) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context cancellation", err)
			}
		})
	}
}

func TestDefaultProfileRanksBreakEqualCreationTimesByID(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	agent := &models.Agent{Name: "equal-profile-ranks"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"z-profile", "a-profile"} {
		if err := repo.CreateAgentProfile(ctx, &models.AgentProfile{ID: id, AgentID: agent.ID, Name: id, Model: "model"}); err != nil {
			t.Fatal(err)
		}
	}
	timestamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := repo.db.ExecContext(ctx, `UPDATE agent_profiles SET created_at = ? WHERE agent_id = ?`, timestamp, agent.ID); err != nil {
		t.Fatal(err)
	}
	profiles, err := repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || profiles[0].ID != "a-profile" || profiles[1].ID != "z-profile" {
		t.Fatalf("equal default ranks = %#v, want stable ID tie-break", profiles)
	}
}

func TestProfileMoveDoesNotCarrySavedRankToAnotherGroup(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	source, target := &models.Agent{Name: "move-source"}, &models.Agent{Name: "move-target"}
	for _, agent := range []*models.Agent{source, target} {
		if err := repo.CreateAgent(ctx, agent); err != nil {
			t.Fatal(err)
		}
	}
	profiles := []*models.AgentProfile{
		{ID: "source-first", AgentID: source.ID, Name: "First", Model: "model"},
		{ID: "moved-profile", AgentID: source.ID, Name: "Moved", Model: "model"},
		{ID: "target-first", AgentID: target.ID, Name: "Target first", Model: "model"},
		{ID: "target-second", AgentID: target.ID, Name: "Target second", Model: "model"},
	}
	for _, profile := range profiles {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := repo.ReorderAgentProfiles(ctx, source.ID, []string{profiles[0].ID, profiles[1].ID}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.ReorderAgentProfiles(ctx, target.ID, []string{profiles[2].ID, profiles[3].ID}); err != nil {
		t.Fatal(err)
	}
	moved := profiles[1]
	moved.AgentID = target.ID
	if err := repo.UpdateAgentProfileWithEnabledIntent(ctx, moved, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.GetAgentProfileOrderSnapshots(ctx, []string{source.ID, target.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := snapshot[target.ID]
	if len(got.Profiles) != 3 || got.Profiles[0].ID != moved.ID || got.Profiles[1].ID != profiles[2].ID || got.Profiles[2].ID != profiles[3].ID || got.Revision != 1 {
		t.Fatalf("target snapshot = %#v, want moved profile before saved group at revision 1", got)
	}
	if len(snapshot[source.ID].Profiles) != 1 || snapshot[source.ID].Profiles[0].ID != profiles[0].ID || snapshot[source.ID].Revision != 1 {
		t.Fatalf("source snapshot = %#v, want surviving saved order at revision 1", snapshot[source.ID])
	}
}
