package process

// SetVscodeForTest injects a VscodeManager with the given status and port.
// Intended for use in tests where starting a real code-server is not feasible.
func (m *Manager) SetVscodeForTest(status VscodeStatus, port int) {
	m.vscodeMu.Lock()
	defer m.vscodeMu.Unlock()
	m.vscode = &VscodeManager{
		status: status,
		port:   port,
	}
}
