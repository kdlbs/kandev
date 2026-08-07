package lifecycle

import (
	"context"
	"errors"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

type fakeTaskHostStandaloneControl struct {
	instance    *agentctl.InstanceInfo
	getErr      error
	createCalls int
}

func (c *fakeTaskHostStandaloneControl) GetInstance(context.Context, string) (*agentctl.InstanceInfo, error) {
	return c.instance, c.getErr
}

func (c *fakeTaskHostStandaloneControl) CreateInstance(
	_ context.Context,
	req *agentctl.CreateInstanceRequest,
) (*agentctl.CreateInstanceResponse, error) {
	c.createCalls++
	return &agentctl.CreateInstanceResponse{ID: req.ID, Port: 42001}, nil
}

func TestEnsureStandaloneTaskHostInstanceReattachesAfterBackendRestart(t *testing.T) {
	request := &agentctl.CreateInstanceRequest{ID: "task-host-1", WorkspacePath: "/workspace/task-1"}

	t.Run("reattaches existing", func(t *testing.T) {
		control := &fakeTaskHostStandaloneControl{instance: &agentctl.InstanceInfo{
			ID: request.ID, Port: 41001, WorkspacePath: request.WorkspacePath,
		}}
		response, reused, err := ensureStandaloneTaskHostInstance(context.Background(), control, request)
		if err != nil {
			t.Fatalf("ensureStandaloneTaskHostInstance: %v", err)
		}
		if !reused || response.Port != 41001 || control.createCalls != 0 {
			t.Fatalf("response=%#v reused=%v creates=%d, want existing instance", response, reused, control.createCalls)
		}
	})

	t.Run("creates when absent", func(t *testing.T) {
		control := &fakeTaskHostStandaloneControl{getErr: agentctl.ErrInstanceNotFound}
		response, reused, err := ensureStandaloneTaskHostInstance(context.Background(), control, request)
		if err != nil {
			t.Fatalf("ensureStandaloneTaskHostInstance: %v", err)
		}
		if reused || response.Port != 42001 || control.createCalls != 1 {
			t.Fatalf("response=%#v reused=%v creates=%d, want one create", response, reused, control.createCalls)
		}
	})

	t.Run("rejects mismatched workspace", func(t *testing.T) {
		control := &fakeTaskHostStandaloneControl{instance: &agentctl.InstanceInfo{
			ID: request.ID, Port: 41001, WorkspacePath: "/workspace/other-task",
		}}
		_, _, err := ensureStandaloneTaskHostInstance(context.Background(), control, request)
		if err == nil || errors.Is(err, agentctl.ErrInstanceNotFound) {
			t.Fatalf("workspace mismatch error = %v", err)
		}
		if control.createCalls != 0 {
			t.Fatalf("created replacement over mismatched live instance")
		}
	})
}
