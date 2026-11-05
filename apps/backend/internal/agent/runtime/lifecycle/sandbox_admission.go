package lifecycle

import "context"

// SetSandboxAdmission binds core policy admission before launch side effects.
func (m *Manager) SetSandboxAdmission(admit func(context.Context, *LaunchRequest) error) {
	m.sandboxAdmission = admit
}
