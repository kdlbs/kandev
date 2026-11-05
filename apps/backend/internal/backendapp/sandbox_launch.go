package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/agent/sandboxruntime"
	"github.com/kandev/kandev/internal/auth/authn"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

var errSandboxRuntimeUnavailable = errors.New("sandbox runtime is unavailable")

type sandboxLaunchAdmission struct {
	runtime  *sandboxruntime.Manager
	policies *sandbox.Service
	tasks    *taskrepo.Repository
}

func (a sandboxLaunchAdmission) admit(ctx context.Context, req *runtime.LaunchRequest) error {
	req.SandboxSelection = nil
	selection, err := a.selection(ctx, req)
	if err != nil {
		return err
	}
	if req.SandboxRestartReview != nil {
		return a.admitReviewedRestart(ctx, req, selection)
	}
	if unscopedSandboxCaller(ctx, selection.WorkspaceID) {
		hasBindings, err := a.policies.HasAnyBindings(ctx)
		if err != nil {
			return err
		}
		if hasBindings {
			return sandbox.ErrIneligible
		}
		return nil
	}
	if selection.ExecutorID == "" {
		hasBindings, err := a.policies.HasBindings(ctx, selection.OrgID)
		if err != nil {
			return err
		}
		if hasBindings {
			return sandbox.ErrIneligible
		}
		return nil
	}
	resolved, err := a.resolve(ctx, selection)
	if err != nil {
		return err
	}
	if resolved != nil {
		if a.runtime == nil {
			return errSandboxRuntimeUnavailable
		}
		selection.Prior = resolved
		req.SandboxSelection = &selection
	}
	return nil
}

func (a sandboxLaunchAdmission) resolve(ctx context.Context, selection sandbox.SelectionContext) (*sandbox.Resolved, error) {
	if selection.TaskID != "" {
		return a.policies.AdmitTask(ctx, selection, selection.ParentTaskID)
	}
	return a.policies.Resolve(ctx, selection)
}

func (a sandboxLaunchAdmission) selection(ctx context.Context, req *runtime.LaunchRequest) (sandbox.SelectionContext, error) {
	identity, _ := authn.IdentityFromContext(ctx)
	selection := sandbox.SelectionContext{OrgID: identity.OrgID, WorkspaceID: req.WorkspaceID, TaskID: req.TaskID, AgentProfileID: req.AgentProfileID}
	if req.ExecutionProfileID != "" {
		selection.AgentProfileID = req.ExecutionProfileID
	}
	selection.ExecutorID, _ = req.Metadata["executor_id"].(string)
	if req.TaskID != "" {
		task, err := a.tasks.GetTask(ctx, req.TaskID)
		if err != nil {
			return selection, err
		}
		if task == nil {
			return selection, sandbox.ErrIneligible
		}
		if req.WorkspaceID != "" && req.WorkspaceID != task.WorkspaceID {
			return selection, sandbox.ErrIneligible
		}
		selection.WorkspaceID = task.WorkspaceID
		selection.WorkflowStepID = task.WorkflowStepID
		selection.ParentTaskID = task.ParentID
	}
	if selection.WorkspaceID != "" {
		workspace, err := a.tasks.GetWorkspace(ctx, selection.WorkspaceID)
		if err != nil {
			return selection, err
		}
		if workspace == nil {
			return selection, sandbox.ErrIneligible
		}
		selection.OrgID = workspace.OrgID
	}
	if err := a.sessionExecutor(ctx, req, &selection); err != nil {
		return selection, err
	}
	return selection, nil
}

func (a sandboxLaunchAdmission) sessionExecutor(ctx context.Context, req *runtime.LaunchRequest, selection *sandbox.SelectionContext) error {
	if req.SessionID != "" {
		session, err := a.tasks.GetTaskSession(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if session == nil || session.TaskID != req.TaskID {
			return sandbox.ErrIneligible
		}
		if selection.ExecutorID != "" && session.ExecutorID != "" && selection.ExecutorID != session.ExecutorID {
			return sandbox.ErrIneligible
		}
		if session.ExecutorID != "" {
			selection.ExecutorID = session.ExecutorID
		}
	}
	if selection.ExecutorID != "" {
		return nil
	}
	profileID, _ := req.Metadata[runtime.MetadataKeyExecutorProfileID].(string)
	if profileID == "" {
		return nil
	}
	profile, err := a.tasks.GetExecutorProfile(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return sandbox.ErrIneligible
	}
	selection.ExecutorID = profile.ExecutorID
	return nil
}

func unscopedSandboxCaller(ctx context.Context, workspaceID string) bool {
	identity, identified := authn.IdentityFromContext(ctx)
	return workspaceID == "" && (!identified || identity.Synthetic || identity.Instance || identity.UserID == "")
}
