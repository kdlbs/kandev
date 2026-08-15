package lsp

import "sync"

type taskWorkspaceUpdateLock struct {
	mu         sync.Mutex
	references int
}

func (m *Manager) lockTaskWorkspaceUpdate(taskID string) func() {
	m.workspaceUpdateMu.Lock()
	entry := m.workspaceUpdateLocks[taskID]
	if entry == nil {
		entry = &taskWorkspaceUpdateLock{}
		m.workspaceUpdateLocks[taskID] = entry
	}
	entry.references++
	m.workspaceUpdateMu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		m.workspaceUpdateMu.Lock()
		entry.references--
		if entry.references == 0 && m.workspaceUpdateLocks[taskID] == entry {
			delete(m.workspaceUpdateLocks, taskID)
		}
		m.workspaceUpdateMu.Unlock()
	}
}
