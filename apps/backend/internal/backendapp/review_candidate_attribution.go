package backendapp

import (
	"context"
	"fmt"
	"strings"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
)

type reviewCandidateWriter interface {
	SetRunningRunReviewer(context.Context, string, string, string) error
}

// recordNativeReviewCandidate persists the changed concrete candidate on the
// live run before that candidate executes. A failed write prevents inference.
func (r reviewInference) recordNativeReviewCandidate(ctx context.Context, route *utilityservice.PromptRequest) error {
	if !strings.HasPrefix(route.RouteSessionID, "native:review-run:") {
		return nil
	}
	owned := agentruntime.InferenceIdentityFromContext(ctx)
	runID := strings.TrimPrefix(route.RouteSessionID, "native:review-run:")
	if owned.ReviewRunID != runID || owned.TaskID == "" || owned.SessionID == "" || owned.LogicalProfileID != route.AgentProfileID || owned.ExecutionProfileID != route.ExecutionProfileID {
		return fmt.Errorf("native review attribution identity does not match")
	}
	if r.runs == nil {
		return fmt.Errorf("native review attribution writer is unavailable")
	}
	return r.runs.SetRunningRunReviewer(ctx, runID, route.AgentCLI, route.Model)
}
