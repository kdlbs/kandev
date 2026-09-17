package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	reachabilitypkg "github.com/kandev/kandev/internal/executors/reachability"
)

// startSSHReachabilityPoller launches the SSH executor reachability poller
// and registers its Stop on addCleanup, mirroring the third-party
// integration poller wiring alongside it in startAgentInfrastructure.
func startSSHReachabilityPoller(
	ctx context.Context,
	repo reachabilitypkg.Repository,
	intervalSeconds int,
	log *logger.Logger,
	publisher *reachabilitypkg.Publisher,
	addCleanup func(func() error) func() error,
) *reachabilitypkg.Poller {
	poller := reachabilitypkg.New(repo, intervalSeconds, log)
	poller.SetPublisher(publisher)
	poller.Start(ctx)
	addCleanup(func() error { poller.Stop(); return nil })
	return poller
}
