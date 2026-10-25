package executor

import (
	"context"
	"errors"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

type preparedMCPContextKey struct{}
type preparedMCPContext struct {
	taskID, sessionID string
	profile           p.Context
	continuation      bool
}

// PrepareMCPLaunch captures one immutable selection for prompt composition and
// the matching launch. It does not publish accepted execution evidence.
func (e *Executor) PrepareMCPLaunch(ctx context.Context, taskID string, session *models.TaskSession, title bool) (context.Context, error) {
	if session == nil {
		return nil, errors.New("session is required for MCP launch preparation")
	}
	profile, err := e.PrepareTaskSessionMCPProfile(ctx, taskID, session, title)
	if err != nil {
		return nil, err
	}
	value := preparedMCPContext{taskID: taskID, sessionID: session.ID, profile: p.Normalize(profile)}
	return context.WithValue(ctx, preparedMCPContextKey{}, value), nil
}

// PreparedMCPProfile returns a detached copy only for the matching launch owner.
func PreparedMCPProfile(ctx context.Context, taskID, sessionID string) *p.Context {
	value, ok := ctx.Value(preparedMCPContextKey{}).(preparedMCPContext)
	if !ok || value.taskID != taskID || value.sessionID != sessionID {
		return nil
	}
	profile := p.Normalize(value.profile)
	return &profile
}

func preparedMCPLaunchProfile(ctx context.Context, taskID, sessionID string) *p.Context {
	value, ok := ctx.Value(preparedMCPContextKey{}).(preparedMCPContext)
	if !ok || value.continuation {
		return nil
	}
	return PreparedMCPProfile(ctx, taskID, sessionID)
}

// PrepareMCPContinuation uses the retained execution's binding, never current
// task preferences. The resulting snapshot cannot be reused for a fresh start.
func (e *Executor) PrepareMCPContinuation(ctx context.Context, taskID string, session *models.TaskSession) (context.Context, error) {
	if session == nil || session.ID == "" || session.TaskID != taskID {
		return nil, errors.New("matching session is required for MCP continuation")
	}
	profile, err := e.resolveTaskSessionMCPProfile(ctx, taskID, session, false)
	if err != nil {
		return nil, err
	}
	running, err := e.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, err
	}
	if running != nil {
		if value, bound := running.Metadata[p.BuiltinBindingMetadataKey]; bound {
			profile, err = e.retainedBuiltinProfile(ctx, taskID, session.ID, running, profile.Surface, value)
			if err != nil {
				return nil, err
			}
		}
	}
	value := preparedMCPContext{taskID: taskID, sessionID: session.ID, profile: p.Normalize(profile), continuation: true}
	return context.WithValue(ctx, preparedMCPContextKey{}, value), nil
}

func (e *Executor) retainedBuiltinProfile(ctx context.Context, taskID, sessionID string, running *models.ExecutorRunning, surface p.Surface, value any) (p.Context, error) {
	profile, err := p.ParseBuiltinBindingMetadata(value)
	if err != nil {
		return p.Context{}, err
	}
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return p.Context{}, err
	}
	manifest := profile.BuiltinPackages
	if task == nil || running.TaskID != taskID || running.SessionID != sessionID || running.AgentExecutionID == "" ||
		manifest.TaskID != taskID || manifest.WorkspaceID != task.WorkspaceID || profile.Surface != surface {
		return p.Context{}, errors.New("retained built-in MCP binding belongs to another owner")
	}
	return *profile, nil
}
