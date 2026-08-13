package lsp

import (
	"reflect"
	"strings"
	"testing"
)

func TestTaskLSPDefaultStateHasNoSessionOwnership(t *testing.T) {
	state := DefaultTaskLanguageState("task-1", "kotlin")
	if state.Policy != PolicyInherit || state.Phase != PhaseOff || state.DetectionState != DetectionUnknown {
		t.Fatalf("default state = %#v", state)
	}
	if state.Revision != 0 || state.Generation != 0 {
		t.Fatalf("default revision/generation = %d/%d", state.Revision, state.Generation)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("default state is invalid: %v", err)
	}

	stateType := reflect.TypeOf(state)
	for index := 0; index < stateType.NumField(); index++ {
		field := strings.ToLower(stateType.Field(index).Name)
		for _, forbidden := range []string{"session", "browser", "editor", "execution", "environment"} {
			if strings.Contains(field, forbidden) {
				t.Fatalf("durable state field %q leaked transient ownership", field)
			}
		}
	}
}

func TestTaskLSPStateRejectsInvalidEnumsAndRestartOverlay(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TaskLanguageState)
	}{
		{name: "policy", mutate: func(state *TaskLanguageState) { state.Policy = "lease" }},
		{name: "detection", mutate: func(state *TaskLanguageState) { state.DetectionState = "walking" }},
		{name: "phase", mutate: func(state *TaskLanguageState) { state.Phase = "indexing" }},
		{name: "action", mutate: func(state *TaskLanguageState) { state.LastAction = "attach" }},
		{name: "initiator", mutate: func(state *TaskLanguageState) { state.LastInitiator = "session" }},
		{name: "process absence generation", mutate: func(state *TaskLanguageState) {
			state.ProcessAbsentGeneration = 1
		}},
		{name: "restart reason without overlay", mutate: func(state *TaskLanguageState) {
			state.RestartRequiredReason = "workspace_roots_changed"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := DefaultTaskLanguageState("task-1", "kotlin")
			test.mutate(&state)
			if err := state.Validate(); err == nil {
				t.Fatalf("invalid %s state was accepted: %#v", test.name, state)
			}
		})
	}
}
