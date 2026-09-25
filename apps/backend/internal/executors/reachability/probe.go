package reachability

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

// reachabilityProber is the production seam over the runtime's SSH
// reachability prober. Tests substitute probeFunc directly, so this stays
// unexported and unmocked.
var reachabilityProber = agentruntime.NewSSHReachabilityProber()

// defaultProbe resolves executor's SSH target from its persisted config and
// probes it. A resolution failure (missing host, invalid port, unresolvable
// alias) never dials — it is reported as
// agentruntime.SSHReachabilityReasonConfig directly, the same reason a
// target with no pinned fingerprint uses.
func defaultProbe(ctx context.Context, executor *models.Executor) agentruntime.SSHProbeOutcome {
	return reachabilityProber.Probe(ctx, executor.Config, probeTimeout)
}
