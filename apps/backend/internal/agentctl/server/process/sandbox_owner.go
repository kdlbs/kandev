package process

import "github.com/kandev/kandev/internal/common/sandbox"

// SetSandboxOwner retains a local PTY supervisor captured by authenticated core.
// A live retained owner cannot be replaced by a different process.
func (m *Manager) SetSandboxOwner(owner *sandbox.ProcessOwner) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	launch := m.cfg.Sandbox
	if owner == nil || launch == nil || owner.AttemptID != launch.AttemptID || owner.Generation != launch.Generation || owner.PolicyDigest != launch.PolicyDigest || owner.Supervisor.SessionID != owner.Supervisor.PID || owner.Supervisor.GroupID != owner.Supervisor.PID {
		return sandbox.ErrInvalidPolicy
	}
	retained, err := owner.LiveSnapshot()
	if err != nil {
		return err
	}
	previous := m.sandboxOwner.Load()
	if previous != nil && *previous != *retained {
		if _, liveErr := previous.LiveSnapshot(); liveErr == nil {
			return sandbox.ErrInvalidPolicy
		}
	}
	m.sandboxOwner.Store(retained)
	return nil
}
