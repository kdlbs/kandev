package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestReorderAgentProfilesMapsStaleAndRejectsDynamic(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "order-agent"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	profile := &models.AgentProfile{AgentID: agent.ID, Name: "profile", Model: "model"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.ReorderAgentProfiles(ctx, agent.ID, []string{"foreign"}); !errors.Is(err, ErrProfileOrderStale) {
		t.Fatalf("stale reorder error = %v, want ErrProfileOrderStale", err)
	}
	if err := repo.CreateAgent(ctx, &models.Agent{ID: agents.DynamicAgentID, Name: "dynamic"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.ReorderAgentProfiles(ctx, agents.DynamicAgentID, []string{profile.ID}); !errors.Is(err, ErrProfileOrderUnsupported) {
		t.Fatalf("dynamic reorder error = %v, want ErrProfileOrderUnsupported", err)
	}
}

func TestReorderAgentProfilesPreservesReadCancellation(t *testing.T) {
	ctrl, _ := newSQLiteBackedController(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ctrl.ReorderAgentProfiles(ctx, "cancelled-agent", []string{"profile"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}
