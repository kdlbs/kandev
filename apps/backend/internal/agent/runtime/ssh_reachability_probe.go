package runtime

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

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

// SSHReachabilityProber is the runtime seam for a single SSH reachability
// probe. Higher-level packages depend on this contract instead of the
// lifecycle implementation details.
type SSHReachabilityProber interface {
	// Probe resolves an SSH executor's target from its persisted config and
	// probes it, bounded by timeout. A resolution failure (missing host,
	// invalid port, unresolvable alias) never dials — it is reported as
	// SSHReachabilityReasonConfig directly, the same reason a target with no
	// pinned fingerprint uses.
	Probe(ctx context.Context, config map[string]string, timeout time.Duration) SSHProbeOutcome
	// ValidateConfig reports whether config resolves to a dialable SSH
	// target, without dialing it.
	ValidateConfig(config map[string]string) error
}

type sshReachabilityProber struct {
	resolveTarget func(map[string]string) (*lifecycle.SSHTarget, error)
	probe         func(context.Context, *lifecycle.SSHTarget, time.Duration) lifecycle.SSHProbeOutcome
}

// NewSSHReachabilityProber creates the production adapter over the lifecycle
// SSH target resolver and reachability prober.
func NewSSHReachabilityProber() SSHReachabilityProber {
	return &sshReachabilityProber{
		resolveTarget: lifecycle.SSHTargetFromExecutorConfig,
		probe:         lifecycle.ProbeSSHHost,
	}
}

func (a *sshReachabilityProber) Probe(ctx context.Context, config map[string]string, timeout time.Duration) SSHProbeOutcome {
	target, err := a.resolveTarget(config)
	if err != nil {
		return SSHProbeOutcome{
			Host:    config["ssh_host"],
			Reason:  SSHReachabilityReasonConfig,
			Message: lifecycle.SanitizeSSHReachabilityMessage(err),
		}
	}
	outcome := a.probe(ctx, target, timeout)
	return SSHProbeOutcome{
		Success:   outcome.Success,
		Cancelled: outcome.Cancelled,
		Host:      outcome.Host,
		Reason:    SSHReachabilityReason(outcome.Reason),
		Message:   outcome.Message,
	}
}

func (a *sshReachabilityProber) ValidateConfig(config map[string]string) error {
	_, err := a.resolveTarget(config)
	return err
}

var _ SSHReachabilityProber = (*sshReachabilityProber)(nil)
