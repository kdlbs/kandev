package lifecycle

// SetAutoContinueFromHistoryEnabled binds the restart-required effective
// installation policy before any agent runtime is constructed.
func (m *Manager) SetAutoContinueFromHistoryEnabled(enabled bool) {
	m.autoContinueFromHistoryEnabled = enabled
}

func (m *Manager) automaticHistoryEnvironment(env map[string]string) map[string]string {
	if env == nil {
		env = make(map[string]string)
	}
	const key = "KANDEV_FEATURES_AUTO_CONTINUE_FROM_HISTORY"
	if m.autoContinueFromHistoryEnabled {
		env[key] = "true"
	} else {
		delete(env, key)
	}
	return env
}
