package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func TestPostgresTaskEnvironmentRecoveryClaimSerializesIndependentRepositories(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID        = "task-postgres-recovery-claim"
		environmentID = "environment-postgres-recovery-claim"
	)
	seedRecoveryClaimEnvironment(t, repoA, taskID, environmentID)

	requestA := recoveryClaimRequest(environmentID, taskID, "session-postgres-recovery-a", "operation-postgres-recovery-a", 1)
	claimA, err := repoA.AcquireTaskEnvironmentRecoveryClaim(ctx, requestA)
	if err != nil {
		t.Fatalf("acquire recovery claim from repository A: %v", err)
	}

	requestB := recoveryClaimRequest(environmentID, taskID, "session-postgres-recovery-b", "operation-postgres-recovery-b", 1)
	if _, err := repoB.AcquireTaskEnvironmentRecoveryClaim(ctx, requestB); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("competing claim from repository B error = %v, want ErrBusy", err)
	}

	if err := repoB.ReleaseTaskEnvironmentRecoveryClaim(ctx, claimA); err != nil {
		t.Fatalf("release recovery claim from repository B: %v", err)
	}
	claimB, err := repoB.AcquireTaskEnvironmentRecoveryClaim(ctx, requestB)
	if err != nil {
		t.Fatalf("acquire recovery claim after release: %v", err)
	}
	if err := repoA.ReleaseTaskEnvironmentRecoveryClaim(ctx, claimB); err != nil {
		t.Fatalf("release replacement recovery claim from repository A: %v", err)
	}
}
