package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// Pins the transitions the Office agent recovery control depends on, before
// any caller of validateStatusTransition exists for that control. A future
// change to allowedTransitions that silently drops paused/stopped -> idle
// should fail here rather than surface as a UI regression.
func TestValidateStatusTransitionRecoveryPaths(t *testing.T) {
	cases := []struct {
		name string
		from models.AgentStatus
		to   models.AgentStatus
	}{
		{"paused to idle", models.AgentStatusPaused, models.AgentStatusIdle},
		{"stopped to idle", models.AgentStatusStopped, models.AgentStatusIdle},
		{"idle to idle is a same-status no-op", models.AgentStatusIdle, models.AgentStatusIdle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateStatusTransition(tc.from, tc.to); err != nil {
				t.Fatalf("validateStatusTransition(%q, %q) = %v, want nil", tc.from, tc.to, err)
			}
		})
	}
}

func TestValidateStatusTransitionRefusesIllegalOrUnknown(t *testing.T) {
	cases := []struct {
		name string
		from models.AgentStatus
		to   models.AgentStatus
	}{
		{"idle to working is not in the transition table", models.AgentStatusIdle, models.AgentStatusWorking},
		{"unknown source status", models.AgentStatus("bogus"), models.AgentStatusIdle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateStatusTransition(tc.from, tc.to)
			if !errors.Is(err, ErrAgentStatusTransition) {
				t.Fatalf("validateStatusTransition(%q, %q) = %v, want ErrAgentStatusTransition", tc.from, tc.to, err)
			}
		})
	}
}

func TestUpdateAgentStatusIfCurrent_RecoversAndIsIdempotent(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()
	agent := createAndGetAgent(t, svc, repo, &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "Recoverable",
		Role:        models.AgentRoleWorker,
	})
	if err := repo.UpdateAgentStatusFields(ctx, agent.ID, string(models.AgentStatusPaused), "manual pause"); err != nil {
		t.Fatalf("pause agent: %v", err)
	}

	updated, err := svc.UpdateAgentStatusIfCurrent(
		ctx, agent.ID, models.AgentStatusPaused, models.AgentStatusIdle, "",
	)
	if err != nil {
		t.Fatalf("recover paused agent: %v", err)
	}
	if updated.Status != models.AgentStatusIdle || updated.PauseReason != "" {
		t.Fatalf("recovered agent = status %q, pause reason %q; want idle and empty",
			updated.Status, updated.PauseReason)
	}

	updated, err = svc.UpdateAgentStatusIfCurrent(
		ctx, agent.ID, models.AgentStatusPaused, models.AgentStatusIdle, "",
	)
	if err != nil {
		t.Fatalf("repeat recovery: %v", err)
	}
	if updated.Status != models.AgentStatusIdle {
		t.Fatalf("repeat recovery status = %q, want idle", updated.Status)
	}
}

func TestUpdateAgentStatusIfCurrent_DoesNotClearWorkingOwner(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()
	agent := createAndGetAgent(t, svc, repo, &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "Working",
		Role:        models.AgentRoleWorker,
	})
	changed, err := repo.MarkAgentWorking(ctx, agent.ID, "live-run")
	if err != nil {
		t.Fatalf("mark working: %v", err)
	}
	if !changed {
		t.Fatal("mark working = false, want true")
	}

	_, err = svc.UpdateAgentStatusIfCurrent(
		ctx, agent.ID, models.AgentStatusPaused, models.AgentStatusIdle, "",
	)
	if !errors.Is(err, ErrAgentStatusStale) {
		t.Fatalf("recover working agent error = %v, want ErrAgentStatusStale", err)
	}

	current, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get working agent: %v", err)
	}
	if current.Status != models.AgentStatusWorking {
		t.Fatalf("working agent status = %q, want working", current.Status)
	}
	cleared, err := repo.ClearAgentWorking(ctx, agent.ID, "live-run")
	if err != nil {
		t.Fatalf("clear working owner: %v", err)
	}
	if !cleared {
		t.Fatal("working owner was not preserved")
	}
}
