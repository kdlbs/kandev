package lsp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPolicyInheritanceRequiresDetectionAndGlobalDefault(t *testing.T) {
	store := newMemoryLSPStore()
	state := DefaultTaskLanguageState("task-1", "kotlin")
	state.Detected = true
	state.DetectionState = DetectionComplete
	if _, err := store.CompareAndUpdateTaskLSPLanguage(context.Background(), state, 0); err != nil {
		t.Fatal(err)
	}
	controller := newTestController(&fakeControllerTasks{}, store, &fakeLSPSettings{
		settings: TaskSettings{AutoStartLanguages: []string{"kotlin"}},
	}, &fakeLSPRuntimes{})

	snapshot, err := controller.Snapshot(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Languages) != 5 {
		t.Fatalf("languages = %d, want all five registered languages", len(snapshot.Languages))
	}
	if got := languageFromSnapshot(t, snapshot, "kotlin").EffectivePolicy; got != PolicyKeepWarm {
		t.Fatalf("detected Kotlin effective policy = %q", got)
	}
	if got := languageFromSnapshot(t, snapshot, "go").EffectivePolicy; got != PolicyDisabled {
		t.Fatalf("undetected Go effective policy = %q", got)
	}
}

func TestSetPolicyInheritPreservesRequestedPolicyWhenDefaultDisabled(t *testing.T) {
	store := newMemoryLSPStore()
	state := DefaultTaskLanguageState("task-1", "kotlin")
	state.Detected = true
	state.DetectionState = DetectionComplete
	state.Policy = PolicyKeepWarm
	state.Phase = PhaseReady
	state.Generation = 1
	if _, err := store.CompareAndUpdateTaskLSPLanguage(context.Background(), state, 0); err != nil {
		t.Fatal(err)
	}
	host := newFakeLSPHost()
	host.setReady("kotlin", 1)
	settings := &fakeLSPSettings{}
	controller := newTestController(
		&fakeControllerTasks{}, store, settings, &fakeLSPRuntimes{host: host},
	)
	controller.capacity.Adopt(TaskLanguageKey{TaskID: "task-1", Language: "kotlin"}, 1)

	snapshot, err := controller.SetPolicy(context.Background(), "task-1", "kotlin", PolicyInherit, Origin{
		Initiator: InitiatorUser, Reason: "user_policy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Policy != PolicyInherit || snapshot.EffectivePolicy != PolicyDisabled || snapshot.Phase != PhaseOff {
		t.Fatalf("inherit stop snapshot = %#v", snapshot)
	}
	if controller.capacity.Active() != 0 {
		t.Fatalf("active capacity = %d, want 0", controller.capacity.Active())
	}

	settings.settings.AutoStartLanguages = []string{"kotlin"}
	if err := controller.ApplySettings(context.Background()); err != nil {
		t.Fatalf("apply enabled default: %v", err)
	}
	state = storedLSPState(t, store, "task-1", "kotlin")
	if state.Policy != PolicyInherit || state.Phase != PhaseReady || state.Generation != 2 {
		t.Fatalf("inherited restart state = %#v", state)
	}
}

func TestApplySettingsStartsNewlyInheritedDetectedLanguage(t *testing.T) {
	store := newMemoryLSPStore()
	state := DefaultTaskLanguageState("task-1", "kotlin")
	state.Detected = true
	state.DetectionState = DetectionComplete
	if _, err := store.CompareAndUpdateTaskLSPLanguage(context.Background(), state, 0); err != nil {
		t.Fatal(err)
	}
	settings := &fakeLSPSettings{settings: TaskSettings{AutoStartLanguages: []string{"kotlin"}}}
	host := newFakeLSPHost()
	controller := newTestController(
		&fakeControllerTasks{}, store, settings, &fakeLSPRuntimes{host: host},
	)

	if err := controller.ApplySettings(context.Background()); err != nil {
		t.Fatalf("apply settings: %v", err)
	}
	host.mu.Lock()
	startCalls := host.startCalls
	host.mu.Unlock()
	if startCalls != 1 {
		t.Fatalf("start calls = %d, want 1", startCalls)
	}
}

func TestApplySettingsPushesLiveConfigurationOnce(t *testing.T) {
	store := newMemoryLSPStore()
	state := DefaultTaskLanguageState("task-1", "kotlin")
	state.Detected = true
	state.Policy = PolicyKeepWarm
	state.Phase = PhaseReady
	state.Generation = 7
	if _, err := store.CompareAndUpdateTaskLSPLanguage(context.Background(), state, 0); err != nil {
		t.Fatal(err)
	}
	host := newFakeLSPHost()
	host.setReady("kotlin", 7)
	settings := &fakeLSPSettings{settings: TaskSettings{ServerConfigs: map[string]json.RawMessage{
		"kotlin": json.RawMessage(`{"compiler":{"jvmTarget":"21"}}`),
	}}}
	controller := newTestController(
		&fakeControllerTasks{}, store, settings, &fakeLSPRuntimes{host: host},
	)

	if err := controller.ApplySettings(context.Background()); err != nil {
		t.Fatalf("apply settings: %v", err)
	}
	if err := controller.ApplySettings(context.Background()); err != nil {
		t.Fatalf("repeat settings: %v", err)
	}
	host.mu.Lock()
	configurationCalls := host.configurationCalls
	lastConfiguration := append(json.RawMessage(nil), host.lastConfiguration.Configuration...)
	host.mu.Unlock()
	if configurationCalls != 1 || string(lastConfiguration) != `{"compiler":{"jvmTarget":"21"}}` {
		t.Fatalf("configuration calls=%d last=%s", configurationCalls, lastConfiguration)
	}
}
