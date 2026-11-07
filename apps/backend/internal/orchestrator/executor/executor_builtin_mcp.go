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

// PrepareTaskSessionMCPProfile reuses an admitted launch profile or resolves packages for the
// matching task session.
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
	case p.SurfaceAutomation:
		return e.prepareAutomationMCPProfile(ctx, taskID, session, profile)
	case p.SurfaceExternal, p.SurfaceCoordinator, p.SurfaceManagedConversation:
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

// prepareExistingWorkspaceBuiltin binds the admitted package snapshot before an
// already prepared workspace starts its first agent process.
func (e *Executor) prepareExistingWorkspaceBuiltin(ctx context.Context, executionID string, profile *p.Context) error {
	if profile == nil || profile.BuiltinPackages == nil {
		return nil
	}
	preparer, ok := e.agentManager.(interface {
		PrepareBuiltinWorkspace(context.Context, string, p.Context) error
	})
	if !ok {
		return fmt.Errorf("agent manager cannot prepare built-in MCP packages")
	}
	return preparer.PrepareBuiltinWorkspace(ctx, executionID, *profile)
}

// AutomationMCPReader resolves the durable automation/task relationship at fresh start.
type AutomationMCPReader interface {
	GetAutomationMCPBinding(context.Context, string, string) (*p.BuiltinManifest, error)
}

// prepareAutomationMCPProfile validates the durable automation grant against the launch workspace,
// task, and surface.
func (e *Executor) prepareAutomationMCPProfile(ctx context.Context, taskID string, session *models.TaskSession, profile p.Context) (p.Context, error) {
	reader, ok := e.builtinPolicyReader.(AutomationMCPReader)
	if !ok {
		return profile, nil
	}
	if session == nil || session.TaskID != taskID || session.ID == "" {
		return p.Context{}, fmt.Errorf("automation MCP preparation requires a matching session")
	}
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return p.Context{}, err
	}
	if task == nil {
		return p.Context{}, fmt.Errorf("automation task unavailable")
	}
	manifest, err := reader.GetAutomationMCPBinding(ctx, task.WorkspaceID, taskID)
	if err != nil {
		return p.Context{}, err
	}
	if manifest != nil {
		if err := manifest.Validate(); err != nil {
			return p.Context{}, err
		}
		if manifest.TaskID != taskID || manifest.WorkspaceID != task.WorkspaceID || manifest.Surface != profile.Surface {
			return p.Context{}, fmt.Errorf("automation MCP binding owner mismatch")
		}
	}
	profile.BuiltinPackages = manifest.Clone()
	return profile, nil
}
