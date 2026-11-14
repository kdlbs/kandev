package lifecycle

// SetBuiltinMCPAuthEnforced sets the restart-scoped caller-auth mode for new
// agentctl instances created by this lifecycle manager.
func (m *Manager) SetBuiltinMCPAuthEnforced(enforced bool) {
	m.builtinMCPAuthEnforced = enforced
}
