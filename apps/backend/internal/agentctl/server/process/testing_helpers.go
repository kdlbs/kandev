package process

import "time"

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

// SetVscodeTransitionForTest injects a VscodeManager that starts in the given
// status and transitions to VscodeStatusRunning after the specified delay.
// Useful for testing code that calls WaitForRunning.
func (m *Manager) SetVscodeTransitionForTest(initialStatus VscodeStatus, port int, delay time.Duration) {
	m.vscodeMu.Lock()
	defer m.vscodeMu.Unlock()
	vm := &VscodeManager{
		status: initialStatus,
		port:   port,
	}
	m.vscode = vm
	go func() {
		time.Sleep(delay)
		vm.mu.Lock()
		vm.status = VscodeStatusRunning
		vm.mu.Unlock()
	}()
}
