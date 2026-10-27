package hostutility

import "context"

// SetSandboxAdmission binds policy admission before utility work or cache reads.
func (m *Manager) SetSandboxAdmission(admit func(context.Context, *ProfileProbeContext) error) {
	m.sandboxAdmission = admit
}

func (m *Manager) admitSandbox(ctx context.Context, probe *ProfileProbeContext) error {
	if m != nil && m.sandboxAdmission != nil {
		return m.sandboxAdmission(ctx, probe)
	}
	return nil
}

// Discovery must not publish a result under a policy context that changed in flight.
func (m *Manager) validateSandboxProbeContext(ctx context.Context, previous ProfileProbeContext) error {
	current := previous
	if err := m.admitSandbox(ctx, &current); err != nil {
		return err
	}
	if current.SandboxDigest != previous.SandboxDigest || (current.SandboxSelection == nil) != (previous.SandboxSelection == nil) {
		return errProfileContextChanged
	}
	return nil
}
