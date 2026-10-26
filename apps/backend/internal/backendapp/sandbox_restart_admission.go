package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/sandbox"
)

func (a sandboxLaunchAdmission) admitReviewedRestart(ctx context.Context, req *runtime.LaunchRequest, selection sandbox.SelectionContext) error {
	review := *req.SandboxRestartReview
	selection.RestartReview = &review
	resolved, _, err := a.policies.ResolveReviewedRestart(ctx, selection, req.SessionID, req.WorkspacePath, review.AttemptID)
	if err != nil {
		return err
	}
	if a.runtime == nil {
		return errSandboxRuntimeUnavailable
	}
	if selection.TaskID != "" {
		// Explicit operator admission advances the private task guard. Saved defaults
		// and policy revisions remain unchanged.
		if _, err = a.policies.AdmitTask(ctx, selection, selection.ParentTaskID); err != nil {
			return err
		}
	}
	selection.Prior = resolved
	req.SandboxSelection = &selection
	return nil
}
