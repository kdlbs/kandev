package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func taskPairService(t *testing.T) *Service {
	agents := map[string]*settingsmodels.AgentProfile{
		"ap-1":    {ID: "ap-1", WorkspaceID: testWorkspaceID},
		"ap-2":    {ID: "ap-2", WorkspaceID: testWorkspaceID},
		"ap-pass": {ID: "ap-pass", WorkspaceID: testWorkspaceID, CLIPassthrough: true},
	}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}, "ep-2": {ID: "ep-2"}}
	return newServiceForTest(t, agents, executors, nil)
}

func createWithPair(t *testing.T, svc *Service, agent, executor string) (*Coordinator, error) {
	return svc.CreateCoordinator(context.Background(), testWorkspaceID, CreateCoordinatorRequest{
		Name: "C", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		TaskAgentProfileID: agent, TaskExecutorProfileID: executor,
	})
}

func patchReq(t *testing.T, body string) PatchCoordinatorRequest {
	var req PatchCoordinatorRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestCreateCoordinator_TaskPairValidation(t *testing.T) {
	cases := []struct {
		name, agent, executor, field, msg string
	}{
		{"empty agent", "", "ep-1", "task_agent_profile_id", ""},
		{"blank executor", "ap-2", "  ", "task_executor_profile_id", ""},
		{"unknown agent", "nope", "ep-1", "task_agent_profile_id", "task agent profile not found"},
		{"unknown executor", "ap-2", "nope", "task_executor_profile_id", "task executor profile not found"},
		{"passthrough", "ap-pass", "ep-1", "task_agent_profile_id", "this agent profile uses CLI passthrough, which created tasks cannot use here"},
		{"unknown agent, empty executor names executor", "nope", "", "task_executor_profile_id", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := createWithPair(t, taskPairService(t), tc.agent, tc.executor)
			assertFieldError(t, err, tc.field)
			var fe *FieldError
			if tc.msg != "" && errors.As(err, &fe) && fe.Message != tc.msg {
				t.Errorf("message = %q, want %q", fe.Message, tc.msg)
			}
		})
	}
	svc := taskPairService(t)
	c, err := createWithPair(t, svc, " ap-2 ", "ep-2")
	if err != nil {
		t.Fatal(err)
	}
	if c.TaskAgentProfileID != "ap-2" || c.TaskExecutorProfileID != "ep-2" {
		t.Fatalf("pair = %q/%q, want trimmed ap-2/ep-2", c.TaskAgentProfileID, c.TaskExecutorProfileID)
	}
}

func TestPatchCoordinator_TaskPair(t *testing.T) {
	svc := taskPairService(t)
	c, err := createWithPair(t, svc, "ap-1", "ep-1")
	if err != nil {
		t.Fatal(err)
	}
	patch := func(body string) (*Coordinator, error) {
		return svc.PatchCoordinator(context.Background(), testWorkspaceID, c.ID, patchReq(t, body))
	}

	for _, body := range []string{`{"task_agent_profile_id":null}`, `{"task_agent_profile_id":"  "}`, `{"task_executor_profile_id":""}`} {
		if _, err := patch(body); err == nil {
			t.Fatalf("%s: want error", body)
		}
	}
	_, err = patch(`{"task_agent_profile_id":"nope","task_executor_profile_id":null}`)
	assertFieldError(t, err, "task_executor_profile_id")
	_, err = patch(`{"task_agent_profile_id":"ap-pass"}`)
	assertFieldError(t, err, "task_agent_profile_id")
	_, err = patch(`{"agent_profile_id":"nope","task_agent_profile_id":"nope"}`)
	assertFieldError(t, err, "agent_profile_id")

	got, err := patch(`{"task_agent_profile_id":"ap-2","task_executor_profile_id":"ep-2"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskAgentProfileID != "ap-2" || got.TaskExecutorProfileID != "ep-2" || got.ConfigRevision != c.ConfigRevision {
		t.Fatalf("got %+v", got)
	}
}

func TestPatchCoordinator_UnchangedMissingTaskPairIsNotValidated(t *testing.T) {
	svc := taskPairService(t)
	c, err := createWithPair(t, svc, "ap-1", "ep-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.store.db.ExecContext(context.Background(), svc.store.db.Rebind(`UPDATE coordinators SET task_agent_profile_id = ? WHERE id = ?`), "gone", c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PatchCoordinator(context.Background(), testWorkspaceID, c.ID, patchReq(t, `{"context":"x","task_agent_profile_id":"gone"}`)); err != nil {
		t.Fatalf("unchanged stored value must not be validated: %v", err)
	}
	found, _, _, err := svc.GetCoordinator(context.Background(), testWorkspaceID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, e, err := svc.TaskPairStatuses(context.Background(), found)
	if err != nil || a != ProfileStatusMissing || e != ProfileStatusOK {
		t.Fatalf("statuses = %s/%s err=%v", a, e, err)
	}
}
