package controller

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
)

func newCopilotProfileController(t *testing.T) (*Controller, *fakeStore, *models.Agent, *models.Agent) {
	t.Helper()
	st := newFakeStore()
	copilot := &models.Agent{ID: "agent-copilot", Name: agents.CopilotACPAgentID}
	claude := &models.Agent{ID: "agent-claude", Name: "claude-acp"}
	for _, agent := range []*models.Agent{copilot, claude} {
		st.agents[agent.ID] = agent
		st.byName[agent.Name] = agent
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.NewRegistry(log)
	for _, agent := range []agents.Agent{agents.NewCopilotACP(), agents.NewClaudeACP()} {
		if err := reg.Register(agent); err != nil {
			t.Fatal(err)
		}
	}
	return NewController(st, nil, reg, nil, log), st, copilot, claude
}

func TestCopilotPluginSelectionCreateUpdateDuplicate(t *testing.T) {
	ctrl, st, copilot, _ := newCopilotProfileController(t)
	ctx := context.Background()
	selection := []string{"live:fixture-market/harness", "installed:copilot-plugins/spark"}
	created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: copilot.ID, Name: "Work",
		CopilotSelectedPlugins: &selection,
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	want := []string{"installed:copilot-plugins/spark", "live:fixture-market/harness"}
	if !reflect.DeepEqual(created.CopilotSelectedPlugins, want) {
		t.Fatalf("created selection = %#v, want %#v", created.CopilotSelectedPlugins, want)
	}

	name := "Renamed"
	updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, Name: &name})
	if err != nil {
		t.Fatalf("UpdateProfile omitted: %v", err)
	}
	if !reflect.DeepEqual(updated.CopilotSelectedPlugins, want) {
		t.Fatalf("omitted update selection = %#v, want %#v", updated.CopilotSelectedPlugins, want)
	}

	cleared := []string{}
	updated, err = ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, CopilotSelectedPlugins: &cleared})
	if err != nil {
		t.Fatalf("UpdateProfile clear: %v", err)
	}
	if updated.CopilotSelectedPlugins == nil || len(updated.CopilotSelectedPlugins) != 0 {
		t.Fatalf("cleared selection = %#v, want []", updated.CopilotSelectedPlugins)
	}

	seed := []string{"live:fixture-market/harness", "installed:copilot-plugins/spark"}
	seeded, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, CopilotSelectedPlugins: &seed})
	if err != nil {
		t.Fatalf("UpdateProfile seed: %v", err)
	}
	if !reflect.DeepEqual(seeded.CopilotSelectedPlugins, want) {
		t.Fatalf("seeded update selection = %#v, want sorted %#v", seeded.CopilotSelectedPlugins, want)
	}
	duplicated, err := ctrl.DuplicateProfile(ctx, DuplicateProfileRequest{ID: created.ID})
	if err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}
	if !reflect.DeepEqual(duplicated.CopilotSelectedPlugins, want) {
		t.Fatalf("duplicated selection = %#v, want %#v", duplicated.CopilotSelectedPlugins, want)
	}
	stored, err := st.GetAgentProfile(ctx, created.ID)
	if err != nil {
		t.Fatalf("read stored profile: %v", err)
	}
	if !reflect.DeepEqual(stored.CopilotSelectedPlugins, want) {
		t.Fatalf("stored selection = %#v, want %#v", stored.CopilotSelectedPlugins, want)
	}
}

func TestCopilotPluginSelectionApplicability(t *testing.T) {
	ctrl, st, copilot, claude := newCopilotProfileController(t)
	ctx := context.Background()
	selection := []string{"installed:m/p"}
	if _, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: claude.ID, Name: "Claude", CopilotSelectedPlugins: &selection,
	}); err == nil {
		t.Fatal("non-Copilot create with plugins unexpectedly succeeded")
	}
	empty := []string{}
	created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: claude.ID, Name: "Claude", CopilotSelectedPlugins: &empty,
	})
	if err != nil {
		t.Fatalf("non-Copilot empty create: %v", err)
	}
	if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, CopilotSelectedPlugins: &selection}); err == nil {
		t.Fatal("non-Copilot update with plugins unexpectedly succeeded")
	}
	copilotProfile := &models.AgentProfile{ID: "dynamic", AgentID: agents.DynamicAgentID, Name: "Dynamic"}
	st.agents[agents.DynamicAgentID] = &models.Agent{ID: agents.DynamicAgentID, Name: agents.DynamicAgentID}
	st.byName[agents.DynamicAgentID] = st.agents[agents.DynamicAgentID]
	st.profiles[agents.DynamicAgentID] = []*models.AgentProfile{copilotProfile}
	if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: "dynamic", CopilotSelectedPlugins: &selection}); err == nil {
		t.Fatal("dynamic profile with plugins unexpectedly succeeded")
	}
	if _, err := ctrl.CreateProfile(ctx, CreateProfileRequest{AgentID: copilot.ID, Name: "Bad", CopilotSelectedPlugins: &[]string{"not-an-identity"}}); err == nil {
		t.Fatal("malformed selection unexpectedly succeeded")
	}
}

func TestCopilotPluginSelectionDuplicateRejected(t *testing.T) {
	ctrl, _, copilot, _ := newCopilotProfileController(t)
	duplicate := []string{"installed:m/p", "installed:m/p"}
	if _, err := ctrl.CreateProfile(context.Background(), CreateProfileRequest{
		AgentID: copilot.ID, Name: "Work", CopilotSelectedPlugins: &duplicate,
	}); err == nil {
		t.Fatal("duplicate selection unexpectedly succeeded")
	}
}
