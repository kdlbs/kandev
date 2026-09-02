package lsp

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type memoryLSPStore struct {
	mu              sync.Mutex
	states          map[TaskLanguageKey]TaskLanguageState
	allocations     int
	compareCalls    int
	compareErrAt    int
	compareErrPhase Phase
	compareErr      error
	getContextHook  func(context.Context)
}

func newMemoryLSPStore() *memoryLSPStore {
	return &memoryLSPStore{states: make(map[TaskLanguageKey]TaskLanguageState)}
}

func (m *memoryLSPStore) GetTaskLSPLanguage(
	ctx context.Context,
	taskID, language string,
) (*TaskLanguageState, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getContextHook != nil {
		m.getContextHook(ctx)
	}
	state, ok := m.states[TaskLanguageKey{TaskID: taskID, Language: language}]
	if !ok {
		state = DefaultTaskLanguageState(taskID, language)
	}
	return &state, ok, nil
}

func (m *memoryLSPStore) ListTaskLSPLanguages(
	_ context.Context,
	taskID string,
) ([]TaskLanguageState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]TaskLanguageState, 0)
	for key, state := range m.states {
		if key.TaskID == taskID {
			result = append(result, state)
		}
	}
	return result, nil
}

func (m *memoryLSPStore) ListAllTaskLSPLanguages(context.Context) ([]TaskLanguageState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]TaskLanguageState, 0, len(m.states))
	for _, state := range m.states {
		result = append(result, state)
	}
	return result, nil
}

func (m *memoryLSPStore) CompareAndUpdateTaskLSPLanguage(
	_ context.Context,
	next TaskLanguageState,
	expected uint64,
) (*TaskLanguageState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compareCalls++
	if (m.compareErrAt != 0 && m.compareCalls == m.compareErrAt) ||
		(m.compareErrPhase != "" && next.Phase == m.compareErrPhase) {
		return nil, m.compareErr
	}
	key := TaskLanguageKey{TaskID: next.TaskID, Language: next.Language}
	current, ok := m.states[key]
	if (!ok && expected != 0) || (ok && current.Revision != expected) {
		return nil, ErrRevisionConflict
	}
	next.Revision = expected + 1
	m.states[key] = next
	return &next, nil
}

func (m *memoryLSPStore) AllocateTaskLSPGeneration(
	_ context.Context,
	taskID, language string,
	action Action,
	initiator Initiator,
	reason string,
	acceptedAt time.Time,
) (*TaskLanguageState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := TaskLanguageKey{TaskID: taskID, Language: language}
	state, ok := m.states[key]
	if !ok {
		state = DefaultTaskLanguageState(taskID, language)
	}
	state.Generation++
	state.Revision++
	state.RuntimeIncarnation = ""
	state.RuntimeStartedAt = nil
	state.RuntimeRevision = 0
	state.ProcessAbsentGeneration = 0
	state.Phase = PhaseStarting
	state.LastAction = action
	state.LastActionAt = &acceptedAt
	state.LastTransitionAt = acceptedAt
	state.LastInitiator = initiator
	switch action {
	case ActionStart:
		state.Policy = PolicyKeepWarm
	case ActionRestart:
		state.LastRestartReason = reason
	}
	m.states[key] = state
	m.allocations++
	return &state, nil
}

func (m *memoryLSPStore) String() string { return fmt.Sprint(m.states) }
