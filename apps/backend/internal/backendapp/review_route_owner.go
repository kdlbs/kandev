package backendapp

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
)

type nativeReviewExecutionResolver interface {
	NativeRouteOwnersEnabled() bool
}

func (l reviewProfileLookup) NativeRouteOwnersEnabled() bool {
	native, ok := l.execution.(nativeReviewExecutionResolver)
	return ok && native.NativeRouteOwnersEnabled()
}

// resolveReviewerExecution resolves the concrete execution for a real run. The
// caller never passes an empty runID while native owners are enabled: the
// durable run must exist before the first selection.
func (l reviewProfileLookup) resolveReviewerExecution(ctx context.Context, profileID, runID string) (agentruntime.ProfileExecution, error) {
	if l.NativeRouteOwnersEnabled() && runID != "" {
		return l.execution.ResolveExecutionDetails(ctx, "native:review-run:"+runID, profileID)
	}
	return l.execution.ResolveExecutionDetails(ctx, "", profileID)
}

func (l reviewProfileLookup) ReleaseReviewRun(runID string) {
	if release, ok := l.execution.(interface{ ReleaseNativeRoute(string) }); ok {
		release.ReleaseNativeRoute("native:review-run:" + runID)
	}
}
