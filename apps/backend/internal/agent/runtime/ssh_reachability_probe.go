package runtime

import "github.com/kandev/kandev/internal/agent/runtime/lifecycle"

// SSHReachabilityReason classifies why an SSH reachability probe failed. It
// mirrors lifecycle.SSHReachabilityReason so callers outside the runtime
// package never import the lifecycle tier directly.
type SSHReachabilityReason string

const (
	SSHReachabilityReasonConfig  SSHReachabilityReason = "config"
	SSHReachabilityReasonTimeout SSHReachabilityReason = "timeout"
	SSHReachabilityReasonHostKey SSHReachabilityReason = "host_key"
	SSHReachabilityReasonAuth    SSHReachabilityReason = "auth"
	SSHReachabilityReasonNetwork SSHReachabilityReason = "network"
	SSHReachabilityReasonUnknown SSHReachabilityReason = "unknown"
)

// SSHProbeOutcome is the runtime-facade mirror of lifecycle.SSHProbeOutcome.
type SSHProbeOutcome struct {
	Success   bool
	Cancelled bool
	Host      string
	Reason    SSHReachabilityReason
	Message   string
}

// SanitizeSSHReachabilityMessage exposes the runtime-safe SSH diagnostic
// projection without making higher-level adapters import lifecycle directly.
func SanitizeSSHReachabilityMessage(err error) string {
	return lifecycle.SanitizeSSHReachabilityMessage(err)
}
