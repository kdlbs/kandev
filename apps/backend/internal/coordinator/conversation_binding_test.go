package coordinator

import (
	"context"
	"errors"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

func TestOpenConversationStampsBindingWithPhase2(t *testing.T) {
	d := newConversationTestDeps(t)
	d.svc.phase2 = true
	result, err := d.svc.OpenConversation(context.Background(), d.coordinator.WorkspaceID, d.coordinator.ID)
	if err != nil {
		t.Fatalf("OpenConversation: %v", err)
	}
	task := d.tasks.tasks[result.TaskID]
	binding, err := mcpprofile.ParseCoordinatorToolPolicyMetadata(task.Metadata[mcpprofile.CoordinatorToolPolicyMetadataKey])
	if err != nil {
		t.Fatalf("binding must parse and validate: %v", err)
	}
	if binding.ConversationTaskID != task.ID || binding.CoordinatorID != d.coordinator.ID || binding.WorkspaceID != d.coordinator.WorkspaceID {
		t.Fatalf("binding identity = %+v, want the conversation's own ids", binding)
	}
	want := ToolNames(d.svc.policyFor(d.coordinator), true)
	if len(binding.ToolNames) != len(want) {
		t.Fatalf("ToolNames = %v, want %v", binding.ToolNames, want)
	}
}

func TestOpenConversationWithoutPhase2WritesNoBinding(t *testing.T) {
	d := newConversationTestDeps(t)
	result, err := d.svc.OpenConversation(context.Background(), d.coordinator.WorkspaceID, d.coordinator.ID)
	if err != nil {
		t.Fatalf("OpenConversation: %v", err)
	}
	if _, present := d.tasks.tasks[result.TaskID].Metadata[mcpprofile.CoordinatorToolPolicyMetadataKey]; present {
		t.Fatal("flag-off conversation must carry no binding")
	}
}

func TestOpenConversationBindFailureDeletesTask(t *testing.T) {
	d := newConversationTestDeps(t)
	d.svc.phase2 = true
	d.tasks.updateErr = errors.New("update failed")
	if _, err := d.svc.OpenConversation(context.Background(), d.coordinator.WorkspaceID, d.coordinator.ID); err == nil {
		t.Fatal("OpenConversation must fail when the binding cannot be written")
	}
	if len(d.tasks.deletedIDs) != 1 || len(d.tasks.createdIDs) != 1 || d.tasks.deletedIDs[0] != d.tasks.createdIDs[0] {
		t.Fatalf("created %v deleted %v, want the unbound task deleted", d.tasks.createdIDs, d.tasks.deletedIDs)
	}
	got, _ := d.svc.store.GetCoordinatorByID(context.Background(), d.coordinator.ID)
	if got.ConversationTaskID != nil {
		t.Fatal("an unbound task must never become current")
	}
}
