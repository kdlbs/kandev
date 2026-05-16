// Package adapters implements the workflow engine's participant, decision,
// and primary-agent adapter interfaces against the workflow repository.
//
// The engine declares the interfaces in internal/workflow/engine/adapters.go
// and internal/workflow/engine/phase2_callbacks.go; this package wires them
// to the workflow_step_participants, workflow_step_decisions, and
// workflow_steps tables so callers (cmd/kandev/main.go) can compose an
// office-grade engine without leaking model imports into the engine package.
package adapters

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/engine"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

// WorkflowRepo captures the subset of *repository.Repository methods the
// adapters need. Defined as an interface so tests can swap in fakes.
type WorkflowRepo interface {
	ListStepParticipantsForTask(ctx context.Context, stepID, taskID string) ([]*models.WorkflowStepParticipant, error)
	ListStepDecisions(ctx context.Context, taskID, stepID string) ([]*models.WorkflowStepDecision, error)
	RecordStepDecision(ctx context.Context, d *models.WorkflowStepDecision) error
	ClearStepDecisions(ctx context.Context, taskID, stepID string) (int64, error)
	GetStep(ctx context.Context, id string) (*models.WorkflowStep, error)
}

// Compile-time check that *repository.Repository satisfies WorkflowRepo.
var _ WorkflowRepo = (*repository.Repository)(nil)

// ParticipantAdapter implements engine.ParticipantStore.
type ParticipantAdapter struct {
	Repo WorkflowRepo
}

// NewParticipantAdapter builds a ParticipantAdapter wrapping the workflow repo.
func NewParticipantAdapter(repo WorkflowRepo) *ParticipantAdapter {
	return &ParticipantAdapter{Repo: repo}
}

// ListStepParticipants satisfies engine.ParticipantStore. The Wave 8 (ADR-0004)
// dual-scoped table — template-level rows (task_id=”) merged with per-task
// overrides — is resolved by the repository's ListStepParticipantsForTask.
func (a *ParticipantAdapter) ListStepParticipants(
	ctx context.Context, stepID, taskID string,
) ([]engine.ParticipantInfo, error) {
	rows, err := a.Repo.ListStepParticipantsForTask(ctx, stepID, taskID)
	if err != nil {
		return nil, fmt.Errorf("list step participants: %w", err)
	}
	out := make([]engine.ParticipantInfo, 0, len(rows))
	for _, p := range rows {
		out = append(out, engine.ParticipantInfo{
			ID:               p.ID,
			StepID:           p.StepID,
			TaskID:           p.TaskID,
			Role:             string(p.Role),
			AgentProfileID:   p.AgentProfileID,
			DecisionRequired: p.DecisionRequired,
			Position:         p.Position,
		})
	}
	return out, nil
}

// DecisionAdapter implements engine.DecisionStore.
type DecisionAdapter struct {
	Repo WorkflowRepo
}

// NewDecisionAdapter builds a DecisionAdapter wrapping the workflow repo.
func NewDecisionAdapter(repo WorkflowRepo) *DecisionAdapter {
	return &DecisionAdapter{Repo: repo}
}

// ListStepDecisions satisfies engine.DecisionStore.
func (a *DecisionAdapter) ListStepDecisions(
	ctx context.Context, taskID, stepID string,
) ([]engine.DecisionInfo, error) {
	rows, err := a.Repo.ListStepDecisions(ctx, taskID, stepID)
	if err != nil {
		return nil, fmt.Errorf("list step decisions: %w", err)
	}
	out := make([]engine.DecisionInfo, 0, len(rows))
	for _, d := range rows {
		out = append(out, engine.DecisionInfo{
			ID:            d.ID,
			TaskID:        d.TaskID,
			StepID:        d.StepID,
			ParticipantID: d.ParticipantID,
			Decision:      d.Decision,
			Note:          d.Note,
		})
	}
	return out, nil
}

// RecordStepDecision satisfies engine.DecisionStore.
func (a *DecisionAdapter) RecordStepDecision(
	ctx context.Context, d engine.DecisionInfo,
) error {
	row := &models.WorkflowStepDecision{
		ID:            d.ID,
		TaskID:        d.TaskID,
		StepID:        d.StepID,
		ParticipantID: d.ParticipantID,
		Decision:      d.Decision,
		Note:          d.Note,
	}
	return a.Repo.RecordStepDecision(ctx, row)
}

// ClearStepDecisions satisfies engine.DecisionStore.
func (a *DecisionAdapter) ClearStepDecisions(
	ctx context.Context, taskID, stepID string,
) (int64, error) {
	return a.Repo.ClearStepDecisions(ctx, taskID, stepID)
}

// PrimaryAgentAdapter implements engine.PrimaryAgentResolver. The "primary"
// agent for a workflow step is the one stored on workflow_steps.agent_profile_id.
type PrimaryAgentAdapter struct {
	Repo WorkflowRepo
}

// NewPrimaryAgentAdapter builds a PrimaryAgentAdapter wrapping the workflow repo.
func NewPrimaryAgentAdapter(repo WorkflowRepo) *PrimaryAgentAdapter {
	return &PrimaryAgentAdapter{Repo: repo}
}

// PrimaryAgentProfileID satisfies engine.PrimaryAgentResolver.
func (a *PrimaryAgentAdapter) PrimaryAgentProfileID(
	ctx context.Context, stepID string,
) (string, error) {
	step, err := a.Repo.GetStep(ctx, stepID)
	if err != nil {
		return "", fmt.Errorf("get step %s: %w", stepID, err)
	}
	return step.AgentProfileID, nil
}

// Compile-time interface assertions.
var (
	_ engine.ParticipantStore     = (*ParticipantAdapter)(nil)
	_ engine.DecisionStore        = (*DecisionAdapter)(nil)
	_ engine.PrimaryAgentResolver = (*PrimaryAgentAdapter)(nil)
)
