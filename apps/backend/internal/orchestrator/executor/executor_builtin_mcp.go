package executor

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

type BuiltinPolicyReader interface {
	GetBuiltinPolicySnapshot(context.Context, string, string) (*mcpconfig.BuiltinPolicySnapshot, error)
}

func (e *Executor) SetBuiltinPolicyReader(reader BuiltinPolicyReader) { e.builtinPolicyReader = reader }

func (e *Executor) PrepareTaskSessionMCPProfile(ctx context.Context, taskID string, session *models.TaskSession, title bool) (p.Context, error) {
	if session != nil {
		if prepared := preparedMCPLaunchProfile(ctx, taskID, session.ID); prepared != nil {
			return *prepared, nil
		}
	}
	profile, err := e.resolveTaskSessionMCPProfile(ctx, taskID, session, title)
	if err != nil || e.builtinPolicyReader == nil {
		return profile, err
	}
	switch profile.Surface {
	case p.SurfaceExternal, p.SurfaceAutomation, p.SurfaceCoordinator, p.SurfaceManagedConversation:
		return profile, nil
	}
	if session == nil || session.TaskID != taskID || session.ID == "" {
		return p.Context{}, fmt.Errorf("built-in MCP preparation requires a matching task session")
	}
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return p.Context{}, err
	}
	if task == nil || task.WorkspaceID == "" {
		return p.Context{}, fmt.Errorf("built-in MCP preparation requires a workspace owner")
	}
	snapshot, err := e.builtinPolicySnapshot(ctx, task.WorkspaceID, taskID, profile.Surface)
	if err != nil {
		return p.Context{}, err
	}
	profile.BuiltinPackages, err = mcpconfig.ResolveBuiltinPackages(task.WorkspaceID, taskID, profile.Surface, snapshot.Policy, snapshot.Preferences)
	return profile, err
}

func (e *Executor) builtinPolicySnapshot(ctx context.Context, workspaceID, taskID string, surface p.Surface) (*mcpconfig.BuiltinPolicySnapshot, error) {
	if surface == p.SurfaceConfiguration {
		return &mcpconfig.BuiltinPolicySnapshot{}, nil
	}
	snapshot, err := e.builtinPolicyReader.GetBuiltinPolicySnapshot(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil || snapshot.Policy == nil || snapshot.Preferences == nil {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	return snapshot, nil
}

func (e *Executor) launchMCPProfile(ctx context.Context, taskID string, session *models.TaskSession, startAgent bool) (p.Context, error) {
	if startAgent {
		return e.PrepareTaskSessionMCPProfile(ctx, taskID, session, true)
	}
	return e.resolveTaskSessionMCPProfile(ctx, taskID, session, false)
}
