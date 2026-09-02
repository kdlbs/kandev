package lsp

import (
	"context"
	"sync"
	"testing"
)

type taskScopedFakeSettings struct {
	mu        sync.Mutex
	byTask    map[string]TaskSettings
	requested []string
}

func (f *taskScopedFakeSettings) TaskLSPSettings(
	_ context.Context,
	taskID string,
) (TaskSettings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requested = append(f.requested, taskID)
	return f.byTask[taskID], nil
}

func (f *taskScopedFakeSettings) set(taskID string, settings TaskSettings) {
	f.mu.Lock()
	f.byTask[taskID] = settings
	f.mu.Unlock()
}

func (f *taskScopedFakeSettings) requestedTasks() map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make(map[string]bool, len(f.requested))
	for _, taskID := range f.requested {
		result[taskID] = true
	}
	return result
}

func TestApplySettingsUsesDefaultsForEachTaskOwner(t *testing.T) {
	store := newMemoryLSPStore()
	for _, state := range []TaskLanguageState{
		{TaskID: "task-a", Language: "kotlin"},
		{TaskID: "task-b", Language: "go"},
	} {
		state.Detected = true
		state.DetectionState = DetectionComplete
		state.Policy = PolicyInherit
		state.Phase = PhaseOff
		seedLSPState(t, store, state)
	}
	settings := &taskScopedFakeSettings{byTask: map[string]TaskSettings{
		"task-a": {AutoStartLanguages: []string{"kotlin"}},
		"task-b": {AutoStartLanguages: []string{"go"}},
	}}
	host := newFakeLSPHost()
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{}, Store: store, Settings: settings,
		Runtimes: &fakeLSPRuntimes{host: host}, Capacity: NewCapacity(8),
	})

	if err := controller.ApplySettings(context.Background()); err != nil {
		t.Fatalf("apply settings: %v", err)
	}
	for _, key := range []TaskLanguageKey{
		{TaskID: "task-a", Language: "kotlin"},
		{TaskID: "task-b", Language: "go"},
	} {
		if state := storedLSPState(t, store, key.TaskID, key.Language); state.Phase != PhaseReady {
			t.Fatalf("%s/%s phase = %q, want ready", key.TaskID, key.Language, state.Phase)
		}
	}
	requested := settings.requestedTasks()
	if !requested["task-a"] || !requested["task-b"] || requested[""] {
		t.Fatalf("settings requested for tasks = %v", requested)
	}

	settings.set("task-a", TaskSettings{})
	if err := controller.ApplySettings(context.Background()); err != nil {
		t.Fatalf("apply task-a disabled settings: %v", err)
	}
	if state := storedLSPState(t, store, "task-a", "kotlin"); state.Phase != PhaseOff {
		t.Fatalf("task-a phase = %q, want off", state.Phase)
	}
	if state := storedLSPState(t, store, "task-b", "go"); state.Phase != PhaseReady {
		t.Fatalf("task-b phase = %q, want ready", state.Phase)
	}
}
