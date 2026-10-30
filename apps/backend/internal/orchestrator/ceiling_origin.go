package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// Existing callers without an explicit admission origin are automatic. Only a
// trusted manual admission can record manual authority; payload fields cannot.
func ceilingAdmissionOrigin(origins ...launchOrigin) launchOrigin {
	if len(origins) > 0 && origins[0] == launchOriginManual {
		return launchOriginManual
	}
	return launchOriginAutomatic
}

// Empty records predate manual deferral and retain automatic policy. Unknown
// values are uncertainty, never permission to bypass auto-run or drop history.
func ceilingStoredOrigin(deferral models.CeilingDeferral) (launchOrigin, error) {
	switch launchOrigin(deferral.Origin) {
	case launchOriginManual:
		return launchOriginManual, nil
	case "", launchOriginAutomatic:
		return launchOriginAutomatic, nil
	default:
		return "", fmt.Errorf("unrecognized deferred launch origin %q", deferral.Origin)
	}
}

func sameCeilingAdmissionOrigin(a, b models.CeilingDeferral) bool {
	left, leftErr := ceilingStoredOrigin(a)
	right, rightErr := ceilingStoredOrigin(b)
	return leftErr == nil && rightErr == nil && left == right
}

// Replay reuses the existing exact claim's authority without serializing an
// origin override into the original payload. Send Now remains a new manual
// caller and does not inherit an automatic replay origin.
func ceilingClaimedReplayOrigin(ctx context.Context, taskID string, fallback launchOrigin) launchOrigin {
	claim, _ := ctx.Value(ceilingDispatchClaimContextKey{}).(*ceilingDeferredLaunchClaim)
	if claim == nil || !claim.held || claim.taskID != taskID || claim.owner != ceilingClaimOwnerReplay {
		return fallback
	}
	origin, err := ceilingStoredOrigin(claim.deferral)
	if err != nil {
		return launchOriginAutomatic
	}
	return origin
}
