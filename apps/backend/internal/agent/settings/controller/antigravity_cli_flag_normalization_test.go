package controller

import (
	"context"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestAntigravityProfileCollapsesDuplicateSkipFlagAndAllowsTurningItOff(t *testing.T) {
	const skipFlag = "--dangerously-skip-permissions"
	ctrl := newTestController(map[string]agents.Agent{
		agents.AntigravityACPAgentID: agents.NewAntigravityACP(),
	})
	store := newFakeStore()
	agent := &models.Agent{ID: "agent-antigravity", Name: agents.AntigravityACPAgentID}
	store.agents[agent.ID] = agent
	store.byName[agent.Name] = agent
	ctrl.repo = store

	created, err := ctrl.CreateProfile(context.Background(), CreateProfileRequest{
		AgentID: agent.ID,
		Name:    "Antigravity",
		CLIFlags: []dto.CLIFlagDTO{
			{Flag: skipFlag, Enabled: false},
			{Flag: "--profile-flag", Enabled: true},
			{Flag: skipFlag, Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	assertNormalizedAntigravityFlags(t, created.CLIFlags, true)
	for _, profile := range store.profiles[agent.ID] {
		if profile.ID == created.ID {
			profile.CLIFlags = []models.CLIFlag{
				{Flag: skipFlag, Enabled: false},
				{Flag: "--profile-flag", Enabled: true},
				{Flag: skipFlag, Enabled: true},
			}
		}
	}
	legacyProfile, err := store.GetAgentProfile(context.Background(), created.ID)
	if err != nil || len(legacyProfile.CLIFlags) != 3 {
		t.Fatalf("seed legacy duplicate profile: profile=%+v err=%v", legacyProfile, err)
	}

	disabledFlags := []dto.CLIFlagDTO{
		{Flag: skipFlag, Enabled: false},
		{Flag: "--profile-flag", Enabled: true},
	}
	updated, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{
		ID: created.ID, CLIFlags: &disabledFlags,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	assertNormalizedAntigravityFlags(t, updated.CLIFlags, false)

	stored, err := store.GetAgentProfile(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetAgentProfile: %v", err)
	}
	args, err := cliflags.Resolve(stored.CLIFlags)
	if err != nil {
		t.Fatalf("Resolve stored CLI flags: %v", err)
	}
	if slices.Contains(args, skipFlag) || !slices.Contains(args, "--profile-flag") {
		t.Fatalf("effective launch args = %q, want skip flag off and unrelated flag retained", args)
	}
}

func assertNormalizedAntigravityFlags(t *testing.T, flags []dto.CLIFlagDTO, wantEnabled bool) {
	t.Helper()
	if len(flags) != 2 {
		t.Fatalf("profile CLI flags = %+v, want one skip flag and one unrelated flag", flags)
	}
	var skipCount int
	for _, flag := range flags {
		if flag.Flag != "--dangerously-skip-permissions" {
			continue
		}
		skipCount++
		if flag.Enabled != wantEnabled {
			t.Fatalf("skip flag enabled = %t, want %t; flags = %+v", flag.Enabled, wantEnabled, flags)
		}
	}
	if skipCount != 1 {
		t.Fatalf("skip flag count = %d, want 1; flags = %+v", skipCount, flags)
	}
}
