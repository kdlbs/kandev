package controller

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
)

func newHandoffPermissionTestController(profiles ...*models.AgentProfile) *Controller {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	st := newFakeStore()
	for _, p := range profiles {
		st.profiles[p.AgentID] = append(st.profiles[p.AgentID], p)
	}
	return &Controller{repo: st, logger: log}
}

func TestAgentHasHandoffPermission_CEODefaultGranted(t *testing.T) {
	ctrl := newHandoffPermissionTestController(&models.AgentProfile{
		ID: "p-ceo", AgentID: "agent-1", Role: models.AgentRoleCEO,
	})

	granted, err := ctrl.AgentHasHandoffPermission(context.Background(), "p-ceo")
	if err != nil {
		t.Fatalf("AgentHasHandoffPermission: %v", err)
	}
	if !granted {
		t.Fatal("expected the ceo role default to grant handoff permission")
	}
}

func TestAgentHasHandoffPermission_WorkerDefaultDenied(t *testing.T) {
	ctrl := newHandoffPermissionTestController(&models.AgentProfile{
		ID: "p-worker", AgentID: "agent-1", Role: models.AgentRoleWorker,
	})

	granted, err := ctrl.AgentHasHandoffPermission(context.Background(), "p-worker")
	if err != nil {
		t.Fatalf("AgentHasHandoffPermission: %v", err)
	}
	if granted {
		t.Fatal("expected the worker role default to deny handoff permission")
	}
}

func TestAgentHasHandoffPermission_OverrideCanRevokeAndGrant(t *testing.T) {
	ctrl := newHandoffPermissionTestController(
		&models.AgentProfile{ID: "p-ceo-off", AgentID: "agent-1", Role: models.AgentRoleCEO, Permissions: `{"can_handoff_tasks":false}`},
		&models.AgentProfile{ID: "p-worker-on", AgentID: "agent-1", Role: models.AgentRoleWorker, Permissions: `{"can_handoff_tasks":true}`},
	)

	granted, err := ctrl.AgentHasHandoffPermission(context.Background(), "p-ceo-off")
	if err != nil {
		t.Fatalf("AgentHasHandoffPermission: %v", err)
	}
	if granted {
		t.Fatal("expected the override to revoke the ceo default")
	}

	granted, err = ctrl.AgentHasHandoffPermission(context.Background(), "p-worker-on")
	if err != nil {
		t.Fatalf("AgentHasHandoffPermission: %v", err)
	}
	if !granted {
		t.Fatal("expected the override to grant permission to a worker profile")
	}
}

func TestAgentHasHandoffPermission_UnknownProfileFailsClosed(t *testing.T) {
	ctrl := newHandoffPermissionTestController()

	granted, err := ctrl.AgentHasHandoffPermission(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("AgentHasHandoffPermission: %v", err)
	}
	if granted {
		t.Fatal("expected an unknown profile id to fail closed")
	}
}

func TestAgentHasHandoffPermission_EmptyProfileIDFailsClosed(t *testing.T) {
	ctrl := newHandoffPermissionTestController()

	granted, err := ctrl.AgentHasHandoffPermission(context.Background(), "")
	if err != nil {
		t.Fatalf("AgentHasHandoffPermission: %v", err)
	}
	if granted {
		t.Fatal("expected an empty profile id to fail closed")
	}
}
