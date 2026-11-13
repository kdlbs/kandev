package executor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestPreparedProfileGradeSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, grade string
		failed      bool
	}{
		{"configured", "Grade-A", false}, {"unconfigured", "", false}, {"unknown", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepository()
			info := &AgentProfileInfo{ProfileID: "profile-grade", Model: "unrelated-model"}
			raw, _ := json.Marshal(map[string]any{"ExecutionGrade": tc.grade})
			if err := json.Unmarshal(raw, info); err != nil {
				t.Fatal(err)
			}
			manager := &mockAgentManager{resolveAgentProfileFunc: func(context.Context, string) (*AgentProfileInfo, error) {
				if tc.failed {
					return nil, errors.New("profile unavailable")
				}
				return info, nil
			}}
			exec := newTestExecutor(t, manager, repo)
			task := &v1.Task{ID: "grade-task", WorkspaceID: "grade-workspace", ExecutionGrade: "Task-Override"}
			if _, err := exec.PrepareSession(context.Background(), task, info.ProfileID, "executor", "", ""); err != nil {
				t.Fatal(err)
			}
			snapshot := repo.createTaskSessionCalls[0].AgentProfileSnapshot
			grade, present := snapshot["execution_grade"]
			if tc.failed {
				if present {
					t.Fatalf("failed profile manufactured grade=%v", grade)
				}
				return
			}
			if !present || grade != tc.grade || snapshot["id"] != info.ProfileID {
				t.Fatalf("snapshot=%v want explicit profile grade %q", snapshot, tc.grade)
			}
			if err := json.Unmarshal([]byte(`{"ExecutionGrade":"Changed"}`), info); err != nil {
				t.Fatal(err)
			}
			if snapshot["execution_grade"] != tc.grade {
				t.Fatal("later profile info mutation changed prepared snapshot")
			}
		})
	}
}
