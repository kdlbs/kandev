package orchestrator

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/workflow/routing"
	"github.com/stretchr/testify/require"
)

type renewingRouteEffectRepository struct {
	mu       sync.Mutex
	renewals int
}

func (*renewingRouteEffectRepository) GetWorkflowRouteEffectByTransition(context.Context, string, int64) (routing.Effect, bool, error) {
	return routing.Effect{}, false, nil
}

func (*renewingRouteEffectRepository) GetCurrentWorkflowRouteEffect(context.Context, string, string) (routing.Effect, bool, error) {
	return routing.Effect{}, false, nil
}

func (*renewingRouteEffectRepository) ClaimWorkflowRouteEffect(context.Context, string, string, time.Time, time.Duration) (bool, error) {
	return false, nil
}

func (r *renewingRouteEffectRepository) RenewWorkflowRouteEffect(context.Context, string, string, time.Time) (bool, error) {
	r.mu.Lock()
	r.renewals++
	r.mu.Unlock()
	return true, nil
}

func (*renewingRouteEffectRepository) CompleteWorkflowRouteEffect(context.Context, string, string, time.Time) (bool, error) {
	return false, nil
}

func TestRenewRouteEffectClaimKeepsLongRunningOwnerLive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := &renewingRouteEffectRepository{}
		stop := make(chan struct{})
		stopped := make(chan struct{})
		svc := &Service{logger: testLogger()}
		go svc.renewRouteEffectClaim(context.Background(), repo, "effect", "token", stop, stopped)

		time.Sleep(routeEffectLease + routeEffectLease/2)
		close(stop)
		<-stopped

		repo.mu.Lock()
		renewals := repo.renewals
		repo.mu.Unlock()
		require.GreaterOrEqual(t, renewals, 4,
			"a legitimate on_enter owner must renew well before the one-minute lease expires")
	})
}
