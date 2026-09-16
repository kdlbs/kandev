package reachability

import (
	"context"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

// defaultProbe resolves executor's SSH target from its persisted config and
// probes it. A resolution failure (missing host, invalid port, unresolvable
// alias) never reaches ProbeSSHHost — it is reported as
// SSHReachabilityReasonConfig directly, the same reason ProbeSSHHost itself
// uses for a target with no pinned fingerprint.
func defaultProbe(ctx context.Context, executor *models.Executor) lifecycle.SSHProbeOutcome {
	target, err := lifecycle.SSHTargetFromExecutorConfig(executor.Config)
	if err != nil {
		return lifecycle.SSHProbeOutcome{
			Host:    executor.Config["ssh_host"],
			Reason:  lifecycle.SSHReachabilityReasonConfig,
			Message: err.Error(),
		}
	}
	return lifecycle.ProbeSSHHost(ctx, target, probeTimeout)
}
