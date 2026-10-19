package coordinator

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type bindingContextKey struct{}

type contextCheckingConversationTasks struct {
	*fakeConversationTasks
	value any
}

func (f *contextCheckingConversationTasks) UpdateTask(ctx context.Context, id string, req *taskservice.UpdateTaskRequest) (*taskmodels.Task, error) {
	f.value = ctx.Value(bindingContextKey{})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.fakeConversationTasks.UpdateTask(ctx, id, req)
}

func TestCommittedConversationBindingSurvivesRequestCancellation(t *testing.T) {
	d := newConversationTestDeps(t)
	d.svc.phase2 = true
	task := &taskmodels.Task{ID: "created", WorkspaceID: d.coordinator.WorkspaceID, Metadata: map[string]interface{}{}}
	d.tasks.tasks[task.ID] = task
	checker := &contextCheckingConversationTasks{fakeConversationTasks: d.tasks}
	d.svc.conversationTasks = checker
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), bindingContextKey{}, "request identity"))
	cancel()
	if err := d.svc.bindConversationTask(ctx, task, d.coordinator, map[string]interface{}{}); err != nil {
		t.Fatalf("required post-create binding abandoned: %v", err)
	}
	binding, err := mcpprofile.ParseCoordinatorToolPolicyMetadata(task.Metadata[mcpprofile.CoordinatorToolPolicyMetadataKey])
	if err != nil || binding.ConversationTaskID != task.ID || checker.value != "request identity" {
		t.Fatalf("binding or request identity lost: %+v, %v, %v", binding, err, checker.value)
	}
}
