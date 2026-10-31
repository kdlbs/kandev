package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	officemodels "github.com/kandev/kandev/internal/office/models"
	officeservice "github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// handoffCompletionTaskLookup is the task-owned state the completion report
// policy reads: the stored tasks and session, plus the directional
// collaboration grant.
type handoffCompletionTaskLookup interface {
	GetTask(context.Context, string) (*taskmodels.Task, error)
	GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
	GetWorkspaceCollaborationGrant(context.Context, string, string, string) (*taskmodels.WorkspaceCollaborationGrant, error)
}

// handoffCompletionOfficeLookup is the Office-owned state the policy reads:
// actual run participation and the participating agent's workspace.
type handoffCompletionOfficeLookup interface {
	HasHandoffSourceRun(context.Context, string, string, string) (bool, error)
	GetAgentInstance(context.Context, string) (*officemodels.AgentInstance, error)
}

// Reporting is passive and may arrive after the originating run finishes.
// Its authority is the stored task/session/agent relationship plus the current
// delivery-task -> source-workspace report grant, never an invented principal.
func newOfficeHandoffCompletionPolicy(
	enabled bool, tasks handoffCompletionTaskLookup, office handoffCompletionOfficeLookup,
) officeservice.HandoffCompletionPolicy {
	if !enabled || tasks == nil || office == nil {
		return nil
	}
	return func(ctx context.Context, source officeservice.HandoffCompletionSource) (bool, error) {
		matches, err := officeHandoffCompletionTasksMatch(ctx, tasks, source)
		if err != nil || !matches {
			return false, err
		}
		matches, err = office.HasHandoffSourceRun(ctx, source.SourceTaskID, source.SourceSessionID, source.SourceAgentProfileID)
		if err != nil || !matches {
			return false, err
		}
		matches, err = officeHandoffAgentWorkspace(ctx, office, source.SourceAgentProfileID, source.SourceWorkspaceID)
		if err != nil || !matches {
			return false, err
		}
		if source.SourceAssigneeAgentProfileID != source.SourceAgentProfileID {
			matches, err = officeHandoffAgentWorkspace(ctx, office, source.SourceAssigneeAgentProfileID, source.SourceWorkspaceID)
			if err != nil || !matches {
				return false, err
			}
		}
		grant, err := tasks.GetWorkspaceCollaborationGrant(ctx, source.DeliveryTaskID, source.DeliveryWorkspaceID, source.SourceWorkspaceID)
		if err != nil {
			return false, fmt.Errorf("read handoff completion report grant: %w", err)
		}
		return grant != nil && grant.Allows(taskmodels.CollaborationReport), nil
	}
}

func officeHandoffCompletionTasksMatch(
	ctx context.Context, tasks handoffCompletionTaskLookup, source officeservice.HandoffCompletionSource,
) (bool, error) {
	origin, err := tasks.GetTask(ctx, source.SourceTaskID)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read handoff completion source task: %w", err)
	}
	if origin == nil || !origin.IsFromOffice || origin.WorkspaceID != source.SourceWorkspaceID {
		return false, nil
	}
	delivery, err := tasks.GetTask(ctx, source.DeliveryTaskID)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read handoff completion delivery task: %w", err)
	}
	if delivery == nil || delivery.WorkspaceID != source.DeliveryWorkspaceID ||
		string(delivery.State) != source.DeliveryState || delivery.WorkspaceID == origin.WorkspaceID {
		return false, nil
	}
	session, err := tasks.GetTaskSession(ctx, source.SourceSessionID)
	if errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read originating handoff session: %w", err)
	}
	return session != nil && session.TaskID == origin.ID, nil
}

func officeHandoffAgentWorkspace(ctx context.Context, office handoffCompletionOfficeLookup, agentID, workspaceID string) (bool, error) {
	if agentID == "" {
		return false, nil
	}
	agent, err := office.GetAgentInstance(ctx, agentID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read handoff Office agent: %w", err)
	}
	return agent != nil && agent.WorkspaceID == workspaceID, nil
}
