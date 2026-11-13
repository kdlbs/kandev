package backendapp

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
)

type nativeReviewExecutionResolver interface {
	NativeRouteOwnersEnabled() bool
	PreviewExecutionDetails(context.Context, string) (agentruntime.ProfileExecution, error)
}

func (l reviewProfileLookup) NativeRouteOwnersEnabled() bool {
	native, ok := l.execution.(nativeReviewExecutionResolver)
	return ok && native.NativeRouteOwnersEnabled()
}

func (l reviewProfileLookup) resolveReviewerExecution(ctx context.Context, profileID, runID string) (agentruntime.ProfileExecution, error) {
	ctx = reviewExecutionContext(ctx)
	if native, ok := l.execution.(nativeReviewExecutionResolver); ok && native.NativeRouteOwnersEnabled() {
		if runID == "" {
			return native.PreviewExecutionDetails(ctx, profileID)
		}
		return l.execution.ResolveExecutionDetails(ctx, "native:review-run:"+runID, profileID)
	}
	return l.execution.ResolveExecutionDetails(ctx, "", profileID)
}

func (l reviewProfileLookup) ReleaseReviewRun(runID string) {
	if release, ok := l.execution.(interface{ ReleaseNativeRoute(string) }); ok {
		release.ReleaseNativeRoute("native:review-run:" + runID)
	}
}
