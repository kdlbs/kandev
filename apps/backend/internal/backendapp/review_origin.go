package backendapp

import (
	"context"
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/review"
)

type reviewOriginProfileLookup interface {
	ReviewerOriginProfile(context.Context, string) (review.ReviewerProfile, bool, error)
}

// ReviewerOriginProfile reads the concrete writer directly. ReviewerProfile would preview
// a dynamic profile and could identify a new reviewer candidate as the writer.
func (l reviewProfileLookup) ReviewerOriginProfile(ctx context.Context, id string) (review.ReviewerProfile, bool, error) {
	if l.profiles == nil {
		return review.ReviewerProfile{}, false, nil
	}
	profile, err := l.profiles.GetAgentProfile(ctx, id)
	if err != nil || profile == nil {
		return review.ReviewerProfile{}, false, err
	}
	agents, ok := l.profiles.(interface {
		GetAgent(context.Context, string) (*agentsettingsmodels.Agent, error)
	})
	if !ok {
		return review.ReviewerProfile{}, false, fmt.Errorf("review origin agent identity unavailable")
	}
	agent, err := agents.GetAgent(ctx, profile.AgentID)
	if err != nil || agent == nil {
		return review.ReviewerProfile{}, false, err
	}
	return review.ReviewerProfile{AgentID: agent.Name, Model: profile.Model, Mode: profile.Mode, CLIPassthrough: profile.CLIPassthrough}, true, nil
}

func reviewExecutionContext(ctx context.Context) context.Context {
	if !review.OriginExclusionActive(ctx) {
		return ctx
	}
	return agentruntime.WithExecutionCandidateFilter(ctx, func(id, agent, model string) bool {
		return review.ReviewerCandidateAllowed(ctx, id, agent, model)
	})
}

func (r reviewInference) validateReviewExecution(ctx context.Context, id, agent, model string) error {
	if !review.OriginExclusionActive(ctx) {
		return nil
	}
	if id != "" {
		if r.originProfiles == nil {
			return fmt.Errorf("%w: concrete reviewer identity unavailable", review.ErrAgentUnavailable)
		}
		actual, found, err := r.originProfiles.ReviewerOriginProfile(ctx, id)
		if err != nil {
			return fmt.Errorf("%w: read concrete reviewer: %w", review.ErrAgentUnavailable, err)
		}
		if !found || actual.CLIPassthrough || actual.AgentID != agent || actual.Model != model {
			return fmt.Errorf("%w: concrete reviewer changed after selection", review.ErrAgentUnavailable)
		}
		agent, model = actual.AgentID, actual.Model
	}
	if !review.ReviewerCandidateAllowed(ctx, id, agent, model) {
		return fmt.Errorf("%w: %w", review.ErrAgentUnavailable, review.ErrNoDistinctReviewer)
	}
	return nil
}
