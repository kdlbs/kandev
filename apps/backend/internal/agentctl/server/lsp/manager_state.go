package lsp

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

func (m *Manager) checkOpen() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return ErrManagerClosed
	}
	return nil
}

func (m *Manager) isClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

func (m *Manager) snapshotLocked(taskID, language string) Snapshot {
	key := taskLanguageRuntimeKey(taskID, language)
	if snapshot, ok := m.snapshots[key]; ok {
		return snapshot
	}
	workspace := m.cfg
	if configured, ok := m.taskConfigs[taskID]; ok {
		workspace = configured
	}
	return Snapshot{
		Language:         language,
		Incarnation:      m.incarnation,
		RuntimeStartedAt: m.startedAt,
		Phase:            sharedlsp.PhaseOff,
		Activity:         ActivityIdle,
		Work:             []WorkItem{},
		Diagnostics:      []json.RawMessage{},
		WorkspacePath:    workspace.WorkDir,
		WorkspaceURI:     workspace.WorkspaceURI,
		WorkspaceFolders: append([]WorkspaceFolder(nil), workspace.WorkspaceFolders...),
	}
}

func (m *Manager) publishForTaskGeneration(
	taskID, language string,
	generation uint64,
	mutate func(*Snapshot),
) Snapshot {
	taskID = normalizeTaskID(taskID, m.cfg.OwnerID)
	key := taskLanguageRuntimeKey(taskID, language)
	m.mu.Lock()
	snapshot := m.snapshotLocked(taskID, language)
	snapshot.Incarnation = m.incarnation
	snapshot.RuntimeStartedAt = m.startedAt
	if snapshot.Generation > generation {
		m.mu.Unlock()
		return cloneSnapshot(snapshot)
	}
	if snapshot.Generation < generation {
		snapshot = m.snapshotLocked(taskID, language)
		snapshot.Generation = generation
	}
	mutate(&snapshot)
	snapshot.Revision++
	snapshot.LastTransitionAt = time.Now().UTC()
	m.snapshots[key] = snapshot
	result := cloneSnapshot(snapshot)
	for _, subscriber := range m.subscribers[key] {
		select {
		case subscriber <- cloneSnapshot(snapshot):
		default:
			select {
			case <-subscriber:
			default:
			}
			subscriber <- cloneSnapshot(snapshot)
		}
	}
	m.mu.Unlock()
	return result
}

func (m *Manager) publishTaskPhase(taskID, language string, generation uint64, phase sharedlsp.Phase) Snapshot {
	return m.publishForTaskGeneration(taskID, language, generation, func(snapshot *Snapshot) {
		snapshot.Phase = phase
	})
}

func (m *Manager) publishTaskError(taskID, language string, generation uint64, code string, err error) Snapshot {
	return m.publishForTaskGeneration(taskID, language, generation, func(snapshot *Snapshot) {
		snapshot.Phase = sharedlsp.PhaseError
		snapshot.Activity = ActivityIdle
		snapshot.Work = []WorkItem{}
		snapshot.LastCompletedWork = nil
		snapshot.ErrorCode = code
		snapshot.ErrorMessage = err.Error()
	})
}

func (m *Manager) publishTaskOff(taskID, language string, generation uint64) Snapshot {
	return m.publishForTaskGeneration(taskID, language, generation, func(snapshot *Snapshot) {
		snapshot.Phase = sharedlsp.PhaseOff
		snapshot.Activity = ActivityIdle
		snapshot.Work = []WorkItem{}
		snapshot.LastCompletedWork = nil
		snapshot.ErrorCode = ""
		snapshot.ErrorMessage = ""
	})
}

func normalizeTaskID(taskID, fallback string) string {
	if taskID != "" {
		return taskID
	}
	return fallback
}

func taskLanguageRuntimeKey(taskID, language string) string {
	return taskID + "\x00" + language
}

func splitTaskLanguageRuntimeKey(key string) (string, string) {
	taskID, language, _ := strings.Cut(key, "\x00")
	return taskID, language
}

func resetRuntimeSnapshot(snapshot *Snapshot) {
	snapshot.Activity = ActivityIdle
	snapshot.ProcessStartedAt = nil
	snapshot.InitializeStartedAt = nil
	snapshot.ReadyAt = nil
	snapshot.Work = []WorkItem{}
	snapshot.LastCompletedWork = nil
	snapshot.Capabilities = nil
	snapshot.Diagnostics = []json.RawMessage{}
	snapshot.ErrorCode = ""
	snapshot.ErrorMessage = ""
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	copy := snapshot
	copy.Work = append([]WorkItem(nil), snapshot.Work...)
	if snapshot.LastCompletedWork != nil {
		completed := *snapshot.LastCompletedWork
		copy.LastCompletedWork = &completed
	}
	copy.Capabilities = append([]byte(nil), snapshot.Capabilities...)
	copy.Diagnostics = make([]json.RawMessage, len(snapshot.Diagnostics))
	for index := range snapshot.Diagnostics {
		copy.Diagnostics[index] = append(json.RawMessage(nil), snapshot.Diagnostics[index]...)
	}
	copy.WorkspaceFolders = append([]WorkspaceFolder(nil), snapshot.WorkspaceFolders...)
	return copy
}

func sortWork(items []WorkItem) {
	sort.Slice(items, func(left, right int) bool {
		if items[left].StartedAt.Equal(items[right].StartedAt) {
			return items[left].Token < items[right].Token
		}
		return items[left].StartedAt.Before(items[right].StartedAt)
	})
}
