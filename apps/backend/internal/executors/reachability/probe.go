package reachability

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

func defaultProbe(context.Context, *models.Executor) agentruntime.SSHProbeOutcome {
	return agentruntime.SSHProbeOutcome{Reason: agentruntime.SSHReachabilityReasonConfig, Message: "managed SSH connections unavailable"}
}
