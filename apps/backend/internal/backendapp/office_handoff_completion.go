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
// the source and assigned agent profiles and their workspace membership.
type handoffCompletionOfficeLookup interface {
	GetAgentInstance(context.Context, string) (*officemodels.AgentInstance, error)
}

// Completion actions use the stored task/session/agent relationship plus the
// current directional report and prompt grants, never an invented principal.
func newOfficeHandoffCompletionPolicy(
	enabled bool, tasks handoffCompletionTaskLookup, office handoffCompletionOfficeLookup,
) officeservice.HandoffCompletionPolicy {
	if !enabled || tasks == nil || office == nil {
		return nil
	}
	return func(ctx context.Context, source officeservice.HandoffCompletionSource) (officeservice.HandoffCompletionAuthorization, error) {
		matches, err := officeHandoffCompletionTasksMatch(ctx, tasks, source)
		if err != nil || !matches {
			return officeservice.HandoffCompletionAuthorization{}, err
		}
		matches, err = officeHandoffAgentWorkspace(ctx, office, source.SourceAgentProfileID, source.SourceWorkspaceID)
		if err != nil || !matches {
			return officeservice.HandoffCompletionAuthorization{}, err
		}
		if source.SourceAssigneeAgentProfileID != "" && source.SourceAssigneeAgentProfileID != source.SourceAgentProfileID {
			matches, err = officeHandoffAgentWorkspace(ctx, office, source.SourceAssigneeAgentProfileID, source.SourceWorkspaceID)
			if err != nil || !matches {
				return officeservice.HandoffCompletionAuthorization{}, err
			}
		}
		grant, err := tasks.GetWorkspaceCollaborationGrant(ctx, source.DeliveryTaskID, source.DeliveryWorkspaceID, source.SourceWorkspaceID)
		if err != nil {
			return officeservice.HandoffCompletionAuthorization{}, fmt.Errorf("read handoff completion grant: %w", err)
		}
		if grant == nil {
			return officeservice.HandoffCompletionAuthorization{}, nil
		}
		report := grant.Allows(taskmodels.CollaborationReport)
		return officeservice.HandoffCompletionAuthorization{
			Report: report,
			Prompt: grant.Allows(taskmodels.CollaborationPrompt),
		}, nil
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
	return officeHandoffDeliveryAndSessionMatch(ctx, tasks, origin, source)
}

func officeHandoffDeliveryAndSessionMatch(
	ctx context.Context,
	tasks handoffCompletionTaskLookup,
	origin *taskmodels.Task,
	source officeservice.HandoffCompletionSource,
) (bool, error) {
	if !officeHandoffHasDeliveryLink(origin.Metadata, source.DeliveryTaskID) {
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
	return session != nil && session.TaskID == origin.ID && session.AgentProfileID == source.SourceAgentProfileID, nil
}

func officeHandoffHasDeliveryLink(metadata map[string]interface{}, deliveryTaskID string) bool {
	links, ok := metadata[taskmodels.MetaKeyHandoffs].([]interface{})
	if !ok {
		return false
	}
	for _, item := range links {
		link, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if id, _ := link["task_id"].(string); id == deliveryTaskID {
			return true
		}
	}
	return false
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
