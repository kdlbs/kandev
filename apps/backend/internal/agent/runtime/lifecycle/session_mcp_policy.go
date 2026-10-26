package lifecycle

import "slices"

// SessionMCPPolicy answers whether the per-session MCP token and Git broker
// relay are in force. The backend composition builds it once from the runtime
// flag; with it disabled every launch, recovery and adoption path behaves as
// it does without the capability.
type SessionMCPPolicy struct {
	enabled bool
}

// NewSessionMCPPolicy returns the policy for the flag value.
func NewSessionMCPPolicy(enabled bool) SessionMCPPolicy {
	return SessionMCPPolicy{enabled: enabled}
}

// Enabled reports whether the capability is in force.
func (p SessionMCPPolicy) Enabled() bool { return p.enabled }

// RequiredCapabilities returns the capability set a control server must
// advertise before this backend adopts it. The package-level
// RequiredSurvivalCapabilities is left unmodified.
func (p SessionMCPPolicy) RequiredCapabilities() []string {
	required := slices.Clone(RequiredSurvivalCapabilities)
	if p.enabled {
		required = append(required, agentctlSessionMCPCapability)
	}
	return required
}

// BrokerUpstream returns the broker URL an instance should relay to, or ""
// when the policy is disabled.
func (p SessionMCPPolicy) BrokerUpstream(executorType string, env map[string]string) string {
	if !p.enabled {
		return ""
	}
	return sameHostBrokerUpstream(executorType, env)
}
