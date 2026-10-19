package backendapp

import (
	"context"
	"errors"
	"testing"

	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	officeshared "github.com/kandev/kandev/internal/office/shared"
)

func taskWorkspaceModeForTest(t *testing.T, metadata map[string]interface{}) string {
	t.Helper()
	workspace, _ := metadata["workspace"].(map[string]interface{})
	mode, _ := workspace["mode"].(string)
	return mode
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.1
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.1
func TestTaskCreatorAdapterAppliesAgentTaskRelations(t *testing.T) {
	adapter, taskSvc, officeRepo := newOfficeTaskAdapterHarness(t)
	taskSvc.SetBlockerRepository(officeRepo)
	ctx := context.Background()

	producerID, err := adapter.CreateOfficeTaskAsAgent(
		ctx, "ws-1", "project-1", "", "Producer", "", nil, officeshared.AgentTaskRelations{},
	)
	if err != nil {
		t.Fatalf("create producer: %v", err)
	}
	rootID, err := adapter.CreateOfficeTaskAsAgent(ctx, "ws-1", "project-1", "", "Consumer", "", nil,
		officeshared.AgentTaskRelations{BlockedBy: []string{producerID}, WorkspaceMode: "new_workspace"})
	if err != nil {
		t.Fatalf("create root with relations: %v", err)
	}
	childID, err := adapter.CreateOfficeSubtask(ctx, rootID, "", "Batch child", "", nil,
		officeshared.AgentTaskRelations{BlockedBy: []string{producerID}, WorkspaceMode: "new_workspace"})
	if err != nil {
		t.Fatalf("create child with relations: %v", err)
	}

	for _, id := range []string{rootID, childID} {
		blockers, err := taskSvc.GetBlockers(ctx, id)
		if err != nil {
			t.Fatalf("GetBlockers(%s): %v", id, err)
		}
		if len(blockers) != 1 || blockers[0] != producerID {
			t.Fatalf("blockers(%s) = %v, want [%s]", id, blockers, producerID)
		}
		task, err := taskSvc.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", id, err)
		}
		if mode := taskWorkspaceModeForTest(t, task.Metadata); mode != "new_workspace" {
			t.Fatalf("workspace mode(%s) = %q, want new_workspace", id, mode)
		}
	}
}

// A blocker that disappears after the runtime scope check must surface as the
// same scope denial, not as an internal error.
//
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.6
func TestTaskCreatorAdapterMapsVanishedBlockerToScopeDenial(t *testing.T) {
	adapter, taskSvc, officeRepo := newOfficeTaskAdapterHarness(t)
	taskSvc.SetBlockerRepository(officeRepo)
	ctx := context.Background()
	relations := officeshared.AgentTaskRelations{BlockedBy: []string{"vanished-task"}}

	_, rootErr := adapter.CreateOfficeTaskAsAgent(ctx, "ws-1", "project-1", "", "Consumer", "", nil, relations)
	if !errors.Is(rootErr, officeruntime.ErrBlockerOutOfScope) {
		t.Fatalf("root create error = %v, want ErrBlockerOutOfScope", rootErr)
	}
	parentID, err := adapter.CreateOfficeTaskAsAgent(
		ctx, "ws-1", "project-1", "", "Parent", "", nil, officeshared.AgentTaskRelations{},
	)
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	_, childErr := adapter.CreateOfficeSubtask(ctx, parentID, "", "Child", "", nil, relations)
	if !errors.Is(childErr, officeruntime.ErrBlockerOutOfScope) {
		t.Fatalf("child create error = %v, want ErrBlockerOutOfScope", childErr)
	}
}
