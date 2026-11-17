package process

// ProviderMCPProtocolRequired changes only provider control compatibility. The
// workspace stream, terminals, previews, and editor transport remain compatible.
func (m *Manager) ProviderMCPProtocolRequired() bool { return m.providerScoped.Load() }

func (m *Manager) ProviderMCPProtocolActivation() <-chan struct{} { return m.providerScopeActivated }

func (m *Manager) requireProviderMCPProtocol() {
	m.providerScoped.Store(true)
	m.providerScopeOnce.Do(func() {
		if m.providerScopeActivated != nil {
			close(m.providerScopeActivated)
		}
	})
}
