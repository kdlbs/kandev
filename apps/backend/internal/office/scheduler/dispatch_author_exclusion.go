package scheduler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// RouteAttemptOutcomeSkippedAuthorProvider records the provider that did the
// task's work, skipped for a seat run at a step declaring
// exclude_author_provider.
const RouteAttemptOutcomeSkippedAuthorProvider = models.RouteAttemptOutcomeSkippedAuthorProvider

// runEventAuthorExclusion is the run event written on every resolution of a
// seat run whose step declares exclude_author_provider.
const runEventAuthorExclusion = "routing.author_exclusion"

// Reasons recorded when the exclusion applies to a seat run but excludes
// nothing.
const (
	authorExclusionReasonNoAuthor        = "no_author_provider"
	authorExclusionReasonRoutingDisabled = "routing_disabled"
)

// authorExclusion is the dispatch-time exclusion policy for one run. Applies
// is true only for a seat run: queued at a step declaring
// exclude_author_provider for a role the run's agent holds a seat in.
type authorExclusion struct {
	Applies        bool
	AuthorProvider routing.ProviderID
}

// resolveAuthorExclusion evaluates the policy from the run payload's task and
// queuing step on every resolution, so fallbacks, park lifts and retries all
// re-apply it and a work-stage revisit is seen on the next resolution.
func (ss *SchedulerService) resolveAuthorExclusion(
	ctx context.Context, run *runsmodels.Run, agent *models.AgentInstance,
) (authorExclusion, error) {
	taskID, stepID := runTaskAndStep(run.Payload)
	if taskID == "" || stepID == "" {
		return authorExclusion{}, nil
	}
	events, err := ss.repo.GetWorkflowStepEvents(ctx, stepID)
	if err != nil {
		return authorExclusion{}, err
	}
	roles := wfmodels.AuthorProviderExclusionRoles(events)
	if len(roles) == 0 {
		return authorExclusion{}, nil
	}
	seated, err := ss.agentHoldsSeatInRoles(ctx, taskID, stepID, agent.ID, roles)
	if err != nil || !seated {
		return authorExclusion{}, err
	}
	author, err := ss.repo.GetTaskAuthorProvider(ctx, taskID)
	if err != nil {
		return authorExclusion{}, err
	}
	return authorExclusion{Applies: true, AuthorProvider: routing.ProviderID(author)}, nil
}

func (ss *SchedulerService) agentHoldsSeatInRoles(
	ctx context.Context, taskID, stepID, agentID string, roles map[string]bool,
) (bool, error) {
	seats, err := ss.repo.ListTaskParticipantsAtStep(ctx, taskID, stepID, "")
	if err != nil {
		return false, fmt.Errorf("list seats: %w", err)
	}
	for _, seat := range seats {
		if seat.AgentProfileID == agentID && roles[seat.Role] {
			return true, nil
		}
	}
	return false, nil
}

// recordAuthorExclusion writes one run event per resolution of a seat run.
func (ss *SchedulerService) recordAuthorExclusion(
	ctx context.Context, runID string, excl authorExclusion, routingEnabled bool,
) {
	payload := map[string]interface{}{"applied": false}
	switch {
	case !routingEnabled:
		payload["reason"] = authorExclusionReasonRoutingDisabled
		payload["author_provider_id"] = string(excl.AuthorProvider)
	case excl.AuthorProvider == "":
		payload["reason"] = authorExclusionReasonNoAuthor
	default:
		payload["applied"] = true
		payload["author_provider_id"] = string(excl.AuthorProvider)
	}
	ss.svc.AppendRunEvent(ctx, runID, runEventAuthorExclusion, "info", payload)
}

func runTaskAndStep(payload string) (string, string) {
	if payload == "" {
		return "", ""
	}
	var p struct {
		TaskID         string `json:"task_id"`
		WorkflowStepID string `json:"workflow_step_id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return "", ""
	}
	return p.TaskID, p.WorkflowStepID
}
