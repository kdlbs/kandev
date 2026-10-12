package controller

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

type divergentProfileOrderRepo struct {
	store.Repository
	legacyProfiles []*models.AgentProfile
	legacyCalls    int
}

func (r *divergentProfileOrderRepo) ListAgentProfiles(context.Context, string) ([]*models.AgentProfile, error) {
	r.legacyCalls++
	return r.legacyProfiles, nil
}

func TestAgentGETsUseProfileOrderSnapshot(t *testing.T) {
	baseController, repo := newSQLiteBackedController(t)
	_ = baseController
	ctx := context.Background()
	agent := &models.Agent{Name: "snapshot-agent"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	first := &models.AgentProfile{AgentID: agent.ID, Name: "First", Model: "model"}
	second := &models.AgentProfile{AgentID: agent.ID, Name: "Second", Model: "model"}
	for _, profile := range []*models.AgentProfile{first, second} {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := repo.ReorderAgentProfiles(ctx, agent.ID, []string{first.ID, second.ID}); err != nil {
		t.Fatal(err)
	}
	divergent := &divergentProfileOrderRepo{Repository: repo, legacyProfiles: []*models.AgentProfile{{ID: "wrong", Name: "Legacy getter"}}}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	ctrl := NewController(divergent, nil, registry.NewRegistry(log), nil, log)

	for _, get := range []struct {
		name string
		call func() ([]string, int64, error)
	}{
		{name: "single", call: func() ([]string, int64, error) {
			dto, err := ctrl.GetAgent(ctx, agent.ID)
			if err != nil {
				return nil, 0, err
			}
			return []string{dto.Profiles[0].Name, dto.Profiles[1].Name}, dto.ProfileOrderRevision, nil
		}},
		{name: "list", call: func() ([]string, int64, error) {
			dto, err := ctrl.ListAgents(ctx)
			if err != nil {
				return nil, 0, err
			}
			return []string{dto.Agents[0].Profiles[0].Name, dto.Agents[0].Profiles[1].Name}, dto.Agents[0].ProfileOrderRevision, nil
		}},
	} {
		t.Run(get.name, func(t *testing.T) {
			names, revision, err := get.call()
			if err != nil {
				t.Fatal(err)
			}
			if len(names) != 2 || names[0] != "First" || names[1] != "Second" || revision != 1 {
				t.Fatalf("snapshot = %v revision %d, want [First Second] revision 1", names, revision)
			}
		})
	}
	if divergent.legacyCalls != 0 {
		t.Fatalf("legacy ListAgentProfiles called %d times, want none", divergent.legacyCalls)
	}
}
