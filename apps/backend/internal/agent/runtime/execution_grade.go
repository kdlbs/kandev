package runtime

import (
	"context"
	"errors"
)

// LogicalExecutionGrade reads only explicit logical settings. It never selects
// a dynamic candidate or derives a grade from the candidate's model or tier.
func (r *ProfileExecutionResolver) LogicalExecutionGrade(ctx context.Context, profileID string) (string, error) {
	if r == nil || r.profiles == nil || profileID == "" {
		return "", errors.New("logical execution profile unavailable")
	}
	profile, err := r.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		return "", err
	}
	if profile == nil {
		return "", errors.New("logical execution profile unavailable")
	}
	if profile.ExecutionAgentProfileID != "" {
		profile, err = r.profiles.GetAgentProfile(ctx, profile.ExecutionAgentProfileID)
		if err != nil {
			return "", err
		}
		if profile == nil {
			return "", errors.New("bound logical execution profile unavailable")
		}
	}
	return profile.ExecutionGrade, nil
}
