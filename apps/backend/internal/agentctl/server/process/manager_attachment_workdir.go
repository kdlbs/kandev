package process

import "fmt"

// WithAttachmentWorkDir pins the active adapter directory during local file
// installation. The callback must not perform network reads or lifecycle calls.
func (m *Manager) WithAttachmentWorkDir(install func(string) error) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	release, err := m.admitStart()
	if err != nil {
		return err
	}
	defer release()
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.adapterCfg != nil {
		if m.adapter == nil || m.Status() != StatusRunning {
			return fmt.Errorf("attachment delivery requires a running agent")
		}
		return install(m.adapterCfg.WorkDir)
	}
	// Before the first adapter exists, materialization retains the configured
	// workspace behavior. A rescan or rebind cannot move it during installation.
	m.repoTrackersMu.RLock()
	defer m.repoTrackersMu.RUnlock()
	return install(m.cfg.WorkDir)
}
