package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// seatSpyDispatcher satisfies shared.WorkflowEngineDispatcher plus the
// package-local decisionSeatDispatcher capability HoldsDecisionSeat reaches
// via type assertion, mirroring dashboard's spyDecisionDispatcher.
type seatSpyDispatcher struct {
	err error
}

func (s *seatSpyDispatcher) HandleTrigger(
	_ context.Context, _ string, _ engine.Trigger, _ any, _ string,
) error {
	return nil
}

func (s *seatSpyDispatcher) ResolveParticipantRoleReadOnly(
	_ context.Context, _, _, _ string,
) (string, string, error) {
	if s.err != nil {
		return "", "", s.err
	}
	return "reviewer", "participant-1", nil
}

func insertTestTaskWithStep(t *testing.T, svc *service.Service, taskID, workspaceID, stepID string) {
	t.Helper()
	svc.ExecSQL(t,
		`INSERT OR IGNORE INTO tasks (id, workspace_id, workflow_step_id) VALUES (?, ?, ?)`,
		taskID, workspaceID, stepID)
}

func TestHoldsDecisionSeat_SeatHolderReturnsTrue(t *testing.T) {
	svc := newTestService(t)
	insertTestTaskWithStep(t, svc, "task-1", "ws-1", "step-1")
	svc.SetWorkflowEngineDispatcher(&seatSpyDispatcher{})

	held, err := svc.HoldsDecisionSeat(context.Background(), "task-1", "agent-1")
	if err != nil {
		t.Fatalf("HoldsDecisionSeat: %v", err)
	}
	if !held {
		t.Fatal("expected seat holder to return true")
	}
}

func TestHoldsDecisionSeat_ParticipantNotFoundReturnsFalseNoError(t *testing.T) {
	svc := newTestService(t)
	insertTestTaskWithStep(t, svc, "task-1", "ws-1", "step-1")
	svc.SetWorkflowEngineDispatcher(&seatSpyDispatcher{err: engine.ErrParticipantNotFound})

	held, err := svc.HoldsDecisionSeat(context.Background(), "task-1", "agent-1")
	if err != nil {
		t.Fatalf("expected no error for ErrParticipantNotFound, got %v", err)
	}
	if held {
		t.Fatal("expected non-holder to return false")
	}
}

func TestHoldsDecisionSeat_UnrelatedErrorPropagates(t *testing.T) {
	svc := newTestService(t)
	insertTestTaskWithStep(t, svc, "task-1", "ws-1", "step-1")
	boom := errors.New("boom")
	svc.SetWorkflowEngineDispatcher(&seatSpyDispatcher{err: boom})

	_, err := svc.HoldsDecisionSeat(context.Background(), "task-1", "agent-1")
	if !errors.Is(err, boom) {
		t.Fatalf("expected unrelated error to propagate, got %v", err)
	}
}

func TestHoldsDecisionSeat_EmptyStepIDReturnsFalse(t *testing.T) {
	svc := newTestService(t)
	insertTestTaskWithStep(t, svc, "task-1", "ws-1", "")
	svc.SetWorkflowEngineDispatcher(&seatSpyDispatcher{})

	held, err := svc.HoldsDecisionSeat(context.Background(), "task-1", "agent-1")
	if err != nil {
		t.Fatalf("HoldsDecisionSeat: %v", err)
	}
	if held {
		t.Fatal("expected empty workflow step to return false")
	}
}

func TestHoldsDecisionSeat_EmptyTaskIDReturnsFalse(t *testing.T) {
	svc := newTestService(t)
	svc.SetWorkflowEngineDispatcher(&seatSpyDispatcher{})

	held, err := svc.HoldsDecisionSeat(context.Background(), "", "agent-1")
	if err != nil {
		t.Fatalf("HoldsDecisionSeat: %v", err)
	}
	if held {
		t.Fatal("expected empty task id to return false")
	}
}

// legacyDispatcher satisfies shared.WorkflowEngineDispatcher but not the
// package-local decisionSeatDispatcher capability, exercising the type
// assertion's failure branch.
type legacyDispatcher struct{}

func (legacyDispatcher) HandleTrigger(
	_ context.Context, _ string, _ engine.Trigger, _ any, _ string,
) error {
	return nil
}

func TestHoldsDecisionSeat_DispatcherWithoutCapabilityReturnsFalse(t *testing.T) {
	svc := newTestService(t)
	insertTestTaskWithStep(t, svc, "task-1", "ws-1", "step-1")
	svc.SetWorkflowEngineDispatcher(legacyDispatcher{})

	held, err := svc.HoldsDecisionSeat(context.Background(), "task-1", "agent-1")
	if err != nil {
		t.Fatalf("HoldsDecisionSeat: %v", err)
	}
	if held {
		t.Fatal("expected dispatcher without decisionSeatDispatcher to return false")
	}
}
