package process

import (
	"errors"
	"fmt"
)

func (m *Manager) connectStartedAdapter() error {
	// Connect adapter to the process stdin/stdout pipes
	if err := m.adapter.Connect(m.stdin, m.stdout); err != nil {
		reapErr, owned := m.reapMainProcessLifecycle()
		switch {
		case !owned:
			reapErr = killAndWaitStartedCommand(m.cmd)
		case reapErr != nil:
			fallbackErr := killAndWaitStartedCommand(m.cmd)
			reapErr = errors.Join(reapErr, fallbackErr)
		default:
			reapErr = killAndWaitStartedCommand(m.cmd)
		}
		_ = m.closeStderrReader()
		m.status.Store(StatusError)
		return errors.Join(fmt.Errorf("failed to connect adapter: %w", err), reapErr)
	}

	return nil
}
