package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestProfileMCPSelectionRoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "cursor-acp"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	profile := &models.AgentProfile{
		AgentID:            agent.ID,
		Name:               "selected",
		MCPSelectionMode:   "selected",
		MCPSelectedServers: []string{"plugin-atlassian-atlassian", "github"},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("read created profile: %v", err)
	}
	if got.MCPSelectionMode != "selected" || !reflect.DeepEqual(got.MCPSelectedServers, profile.MCPSelectedServers) {
		t.Fatalf("created profile selection = %q / %#v", got.MCPSelectionMode, got.MCPSelectedServers)
	}

	got.MCPSelectedServers = []string{}
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("clear selected servers: %v", err)
	}
	got, err = repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("read updated profile: %v", err)
	}
	if got.MCPSelectionMode != "selected" || got.MCPSelectedServers == nil || len(got.MCPSelectedServers) != 0 {
		t.Fatalf("explicit empty selection = %q / %#v, want selected / []", got.MCPSelectionMode, got.MCPSelectedServers)
	}
}
