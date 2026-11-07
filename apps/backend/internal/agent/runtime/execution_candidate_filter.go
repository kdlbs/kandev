package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

// ErrExecutionCandidateExcluded distinguishes an explicitly excluded execution
// from provider health or capacity failures, whose routing policy is unchanged.
var ErrExecutionCandidateExcluded = errors.New("execution candidate is excluded by this request")

type executionCandidateFilterKey struct{}

// WithExecutionCandidateFilter restricts only this request's immutable selection
// view. It does not change stored profiles, route order, health or policy.
func WithExecutionCandidateFilter(ctx context.Context, allowed func(string, string, string) bool) context.Context {
	return context.WithValue(ctx, executionCandidateFilterKey{}, allowed)
}

func executionCandidateAllowed(ctx context.Context, profile *models.AgentProfile, agentName string) bool {
	allowed, _ := ctx.Value(executionCandidateFilterKey{}).(func(string, string, string) bool)
	return allowed == nil || allowed(profile.ID, agentName, profile.Model)
}

func (r *ProfileExecutionResolver) filterExecutionCandidates(ctx context.Context, profile *dynamic.Profile) error {
	if ctx.Value(executionCandidateFilterKey{}) == nil {
		return nil
	}
	excluded, remaining := false, false
	for i := range profile.Candidates {
		candidate := &profile.Candidates[i]
		if !candidate.Enabled {
			continue
		}
		concrete, err := r.profiles.GetAgentProfile(ctx, candidate.ID)
		if err != nil {
			return fmt.Errorf("read execution candidate: %w", err)
		}
		if concrete == nil {
			return fmt.Errorf("execution candidate unavailable")
		}
		name, err := r.agentNameForProfile(ctx, concrete)
		if err != nil {
			return err
		}
		if !executionCandidateAllowed(ctx, concrete, name) {
			candidate.Enabled = false
			excluded = true
		} else {
			remaining = true
		}
	}
	if excluded && !remaining {
		return ErrExecutionCandidateExcluded
	}
	return nil
}
