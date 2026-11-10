package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type mcpUsageProfileReader interface {
	GetMCPUsageProfile(context.Context, string, string) (*mcpconfig.UsageOwner, error)
}
type mcpUsageTaskReader interface {
	GetRepository(context.Context, string) (*models.Repository, error)
	GetTask(context.Context, string) (*models.Task, error)
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
}

func mcpUsageOwnerReader(profiles mcpUsageProfileReader, tasks mcpUsageTaskReader) mcpconfig.UsageOwnerReader {
	return func(ctx context.Context, w string, ref mcpconfig.UsageReference) (*mcpconfig.UsageOwner, error) {
		if tasks == nil || profiles == nil {
			return nil, mcpconfig.ErrMCPUsageUnavailable
		}
		owner, err := readMCPUsageOwner(ctx, profiles, tasks, w, ref)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, repoerrors.ErrTaskNotFound) || errors.Is(err, repoerrors.ErrRepositoryNotFound) || errors.Is(err, models.ErrTaskSessionNotFound) || errors.Is(err, taskservice.ErrForbidden) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return owner, nil
	}
}

func readMCPUsageOwner(ctx context.Context, profiles mcpUsageProfileReader, tasks mcpUsageTaskReader, w string, ref mcpconfig.UsageReference) (*mcpconfig.UsageOwner, error) {
	owner := &mcpconfig.UsageOwner{UsageReference: ref, WorkspaceID: w, OwnerState: mcpconfig.UsageOwnerAvailable}
	switch ref.Scope {
	case mcpconfig.SelectionScopeProfile:
		return profiles.GetMCPUsageProfile(ctx, w, ref.OwnerID)
	case mcpconfig.SelectionScopeRepository:
		repository, err := tasks.GetRepository(ctx, ref.OwnerID)
		if err != nil {
			return nil, err
		}
		if repository == nil || repository.WorkspaceID != w || repository.ID != ref.OwnerID {
			return nil, nil
		}
		owner.DisplayName = repository.Name
	case mcpconfig.SelectionScopeTask, mcpconfig.SelectionScopeTaskSession:
		return readMCPUsageTaskOwner(ctx, tasks, owner)
	default:
		return nil, mcpconfig.ErrMCPInvalidSelection
	}
	return owner, nil
}

func readMCPUsageTaskOwner(ctx context.Context, tasks mcpUsageTaskReader, owner *mcpconfig.UsageOwner) (*mcpconfig.UsageOwner, error) {
	taskID := owner.OwnerID
	if owner.Scope == mcpconfig.SelectionScopeTaskSession {
		session, err := tasks.GetTaskSession(ctx, owner.OwnerID)
		if err != nil {
			return nil, err
		}
		if session == nil || session.ID != owner.OwnerID || session.TaskID == "" {
			return nil, nil
		}
		taskID = session.TaskID
		owner.TaskID = taskID
		owner.DisplayName = session.Name
	}
	task, err := tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil || task.ID != taskID || task.WorkspaceID != owner.WorkspaceID {
		return nil, nil
	}
	if owner.Scope == mcpconfig.SelectionScopeTask {
		owner.DisplayName = task.Title
	}
	if task.ArchivedAt != nil {
		owner.OwnerState = mcpconfig.UsageOwnerArchived
	}
	return owner, nil
}
