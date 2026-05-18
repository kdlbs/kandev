// Package engine_adapters provides office-specific implementations of the
// workflow engine's adapter interfaces.
//
// The engine declares CEOAgentResolver in internal/workflow/engine/adapters.go;
// office is the only domain that knows how to look up a workspace's CEO agent,
// so the implementation lives here. cmd/kandev/main.go composes this adapter
// when constructing the office-grade engine.
package engine_adapters

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// OfficeRepo captures the subset of *sqlite.Repository methods the CEO
// resolver needs. Defined as an interface so tests can swap fakes.
type OfficeRepo interface {
	GetTaskExecutionFields(ctx context.Context, taskID string) (*sqlite.TaskExecutionFields, error)
	ListAgentInstancesFiltered(
		ctx context.Context, workspaceID string, filter sqlite.AgentListFilter,
	) ([]*models.AgentInstance, error)
}

// CEOAgentAdapter implements engine.CEOAgentResolver. Given a task id, it
// resolves the task's workspace then the workspace's first agent with role
// "ceo" and returns its agent_profile_id.
//
// Returning an empty string with a nil error signals "no CEO configured" —
// the engine surfaces that as a "queue_run: workspace has no CEO agent
// profile" error rather than silently swallowing it.
type CEOAgentAdapter struct {
	Repo OfficeRepo
}

// NewCEOAgentAdapter builds a CEOAgentAdapter wrapping the office repo.
func NewCEOAgentAdapter(repo OfficeRepo) *CEOAgentAdapter {
	return &CEOAgentAdapter{Repo: repo}
}

// ResolveCEOAgentProfileID satisfies engine.CEOAgentResolver.
func (a *CEOAgentAdapter) ResolveCEOAgentProfileID(
	ctx context.Context, taskID string,
) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("task_id is required to resolve workspace CEO")
	}
	fields, err := a.Repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("get task workspace: %w", err)
	}
	if fields.WorkspaceID == "" {
		return "", fmt.Errorf("task %s has no workspace", taskID)
	}
	ceos, err := a.Repo.ListAgentInstancesFiltered(ctx, fields.WorkspaceID, sqlite.AgentListFilter{
		Role: string(models.AgentRoleCEO),
	})
	if err != nil {
		return "", fmt.Errorf("list CEO agents: %w", err)
	}
	if len(ceos) == 0 {
		return "", nil
	}
	// Wave G: under the unified agent_profiles model the row id IS the
	// profile id, so AgentInstance.ID is the canonical reference for
	// downstream callers that previously consulted AgentProfileID.
	return ceos[0].ID, nil
}

// Compile-time interface assertion.
var _ engine.CEOAgentResolver = (*CEOAgentAdapter)(nil)
