package main

import (
	"testing"
)

func TestGoalScenariosEmitGoalLifecycleFrames(t *testing.T) {
	tests := []struct {
		name             string
		wantGoalUpdates  int
		wantActive       bool
		wantFinalStatus  string
		wantFinalCleared bool
	}{
		{name: "goal-active", wantGoalUpdates: 1, wantActive: true},
		{name: "goal-complete", wantGoalUpdates: 2, wantActive: true, wantFinalStatus: "complete"},
		{name: "goal-clear", wantGoalUpdates: 2, wantActive: true, wantFinalCleared: true},
		{name: "goal-long", wantGoalUpdates: 1, wantActive: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			emit, updates := newTestEmitter()
			scenarioRegistry[tt.name](emit)

			goalUpdates := make([]map[string]any, 0)
			for _, update := range updates.getUpdates() {
				info := update.notification.Update.SessionInfoUpdate
				if info == nil {
					continue
				}
				if value, ok := info.Meta["goal"]; ok {
					if value == nil {
						goalUpdates = append(goalUpdates, nil)
						continue
					}
					goal, ok := value.(map[string]any)
					if !ok {
						t.Fatalf("goal metadata has type %T", value)
					}
					goalUpdates = append(goalUpdates, goal)
				}
			}

			if len(goalUpdates) != tt.wantGoalUpdates {
				t.Fatalf("goal update count = %d, want %d", len(goalUpdates), tt.wantGoalUpdates)
			}
			if tt.wantActive {
				if goalUpdates[0]["status"] != "active" {
					t.Fatalf("first goal status = %v, want active", goalUpdates[0]["status"])
				}
			}
			if tt.wantFinalCleared {
				if goalUpdates[len(goalUpdates)-1] != nil {
					t.Fatalf("final goal = %v, want clear", goalUpdates[len(goalUpdates)-1])
				}
			}
			if tt.wantFinalStatus != "" && goalUpdates[len(goalUpdates)-1]["status"] != tt.wantFinalStatus {
				t.Fatalf("final goal status = %v, want %s", goalUpdates[len(goalUpdates)-1]["status"], tt.wantFinalStatus)
			}
		})
	}
}
