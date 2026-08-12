package lsp

import (
	"fmt"
	"sort"
)

type taskLanguageSlot struct {
	key  string
	slot *languageSlot
}

// PurgeTask removes stopped task-scoped state from a shared task host. The
// caller must first stop every language generation for the task. Other tasks'
// runtimes and cached workspace projections remain untouched.
func (m *Manager) PurgeTask(taskID string) error {
	taskID = normalizeTaskID(taskID, m.cfg.OwnerID)
	slots, err := m.beginTaskPurge(taskID)
	if err != nil {
		return err
	}
	lockTaskLanguageSlots(slots)
	defer m.finishTaskPurge(taskID, slots)
	if err := ensureTaskSlotsStopped(taskID, slots); err != nil {
		return err
	}
	m.removeTaskState(taskID, slots)
	return nil
}

func (m *Manager) beginTaskPurge(taskID string) ([]taskLanguageSlot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrManagerClosed
	}
	if _, purging := m.purgingTasks[taskID]; purging {
		return nil, ErrTaskStatePurging
	}
	m.purgingTasks[taskID] = struct{}{}
	slots := make([]taskLanguageSlot, 0)
	for key, slot := range m.slots {
		keyTaskID, _ := splitTaskLanguageRuntimeKey(key)
		if keyTaskID == taskID {
			slots = append(slots, taskLanguageSlot{key: key, slot: slot})
		}
	}
	sort.Slice(slots, func(left, right int) bool { return slots[left].key < slots[right].key })
	return slots, nil
}

func lockTaskLanguageSlots(slots []taskLanguageSlot) {
	for _, entry := range slots {
		entry.slot.opMu.Lock()
	}
}

func ensureTaskSlotsStopped(taskID string, slots []taskLanguageSlot) error {
	for _, entry := range slots {
		entry.slot.startMu.Lock()
		operationPending := len(entry.slot.pendingStarts) != 0 || len(entry.slot.pendingStops) != 0
		entry.slot.startMu.Unlock()
		if entry.slot.runtime != nil || operationPending {
			return fmt.Errorf("%w for task %q", ErrTaskRuntimeActive, taskID)
		}
	}
	return nil
}

func (m *Manager) removeTaskState(taskID string, slots []taskLanguageSlot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range slots {
		if m.slots[entry.key] == entry.slot {
			// The operation lock is held by PurgeTask. Retiring before removal
			// makes a caller that already obtained this pointer fail after it
			// eventually acquires the lock instead of launching into an orphaned
			// slot that is no longer tracked by the manager.
			entry.slot.retired = true
			delete(m.slots, entry.key)
		}
	}
	for key := range m.snapshots {
		keyTaskID, _ := splitTaskLanguageRuntimeKey(key)
		if keyTaskID == taskID {
			delete(m.snapshots, key)
		}
	}
	for key, subscribers := range m.subscribers {
		keyTaskID, _ := splitTaskLanguageRuntimeKey(key)
		if keyTaskID != taskID {
			continue
		}
		for id, subscriber := range subscribers {
			delete(subscribers, id)
			close(subscriber)
		}
		delete(m.subscribers, key)
	}
	delete(m.taskConfigs, taskID)
}

func (m *Manager) finishTaskPurge(taskID string, slots []taskLanguageSlot) {
	for index := len(slots) - 1; index >= 0; index-- {
		slots[index].slot.opMu.Unlock()
	}
	m.mu.Lock()
	delete(m.purgingTasks, taskID)
	m.mu.Unlock()
}
