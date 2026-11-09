package coordinator

import (
	"context"
	"errors"
	"fmt"
	taskmodels "github.com/kandev/kandev/internal/task/models"
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
	want := ToolNames(d.svc.policyFor(d.coordinator), true, false)
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

func TestIsDreamTask(t *testing.T) {
	dream := &taskmodels.Task{Metadata: map[string]interface{}{taskmodels.MetaKeyCoordinatorPurpose: taskmodels.CoordinatorPurposeDream}}
	conv := &taskmodels.Task{Metadata: map[string]interface{}{taskmodels.MetaKeyCoordinatorID: "c"}}
	if !isDreamTask(dream) || isDreamTask(conv) || isDreamTask(nil) || isDreamTask(&taskmodels.Task{}) {
		t.Fatal("only a task stamped with the dream purpose is a dream task")
	}
}

type fakeCoordinatorPluginBindings struct {
	references []mcpprofile.CoordinatorPluginTool
	err        error
}

func (f fakeCoordinatorPluginBindings) CoordinatorPluginBindings() ([]mcpprofile.CoordinatorPluginTool, error) {
	return f.references, f.err
}

func TestStampToolPolicyBindsExactPluginReferences(t *testing.T) {
	d := newConversationTestDeps(t)
	d.svc.phase2 = true
	d.svc.pluginBindings = fakeCoordinatorPluginBindings{references: []mcpprofile.CoordinatorPluginTool{{
		PluginID: "kandev-provider-usage", LocalName: "get_provider_usage",
		InstallationID: "installation-1", PolicyVersion: "policy-1",
	}}}
	meta := map[string]interface{}{}
	if err := d.svc.stampToolPolicy(meta, d.coordinator, "conversation-1"); err != nil {
		t.Fatalf("stampToolPolicy: %v", err)
	}
	binding, err := mcpprofile.ParseCoordinatorToolPolicyMetadata(meta[mcpprofile.CoordinatorToolPolicyMetadataKey])
	if err != nil {
		t.Fatalf("binding must parse: %v", err)
	}
	if binding.Version != mcpprofile.CoordinatorToolPolicyVersionV2 {
		t.Fatalf("binding version = %d, want v2", binding.Version)
	}
	if !binding.AllowsPlugin("kandev-provider-usage", "get_provider_usage", "installation-1", "policy-1") {
		t.Fatalf("binding plugin refs = %#v", binding.PluginTools)
	}
}

func TestStampToolPolicyCatalogFailureGrantsNoPlugins(t *testing.T) {
	d := newConversationTestDeps(t)
	d.svc.phase2 = true
	d.svc.pluginBindings = fakeCoordinatorPluginBindings{err: errors.New("catalog unavailable")}
	meta := map[string]interface{}{}
	if err := d.svc.stampToolPolicy(meta, d.coordinator, "conversation-1"); err != nil {
		t.Fatalf("stampToolPolicy: %v", err)
	}
	binding, err := mcpprofile.ParseCoordinatorToolPolicyMetadata(meta[mcpprofile.CoordinatorToolPolicyMetadataKey])
	if err != nil {
		t.Fatalf("binding must parse: %v", err)
	}
	if len(binding.PluginTools) != 0 {
		t.Fatalf("binding plugin refs = %#v, want none", binding.PluginTools)
	}
}

func coordinatorPluginRefs(count int) []mcpprofile.CoordinatorPluginTool {
	refs := make([]mcpprofile.CoordinatorPluginTool, 0, count)
	for i := 0; i < count; i++ {
		pluginID := "plugin-a"
		if i >= 10 {
			pluginID = "plugin-b"
		}
		refs = append(refs, mcpprofile.CoordinatorPluginTool{
			PluginID: pluginID, LocalName: fmt.Sprintf("tool_%03d", i),
			InstallationID: pluginID + "-installation", PolicyVersion: "policy-1",
		})
	}
	return refs
}

func TestStampToolPolicyAcceptsAggregateMultiPluginCatalogBeyondOnePluginLimit(t *testing.T) {
	d := newConversationTestDeps(t)
	d.svc.phase2 = true
	d.svc.pluginBindings = fakeCoordinatorPluginBindings{references: coordinatorPluginRefs(20)}
	meta := map[string]interface{}{}
	if err := d.svc.stampToolPolicy(meta, d.coordinator, "conversation-1"); err != nil {
		t.Fatalf("stampToolPolicy: %v", err)
	}
	binding, err := mcpprofile.ParseCoordinatorToolPolicyMetadata(meta[mcpprofile.CoordinatorToolPolicyMetadataKey])
	if err != nil {
		t.Fatalf("aggregate binding must parse: %v", err)
	}
	if len(binding.PluginTools) != 20 {
		t.Fatalf("plugin refs = %d, want 20", len(binding.PluginTools))
	}
	if len(binding.ToolNames) == 0 {
		t.Fatal("core tools must remain in the aggregate binding")
	}
}

func TestStampToolPolicyTrimsAggregateCatalogToBindingCapacity(t *testing.T) {
	d := newConversationTestDeps(t)
	d.svc.phase2 = true
	d.svc.pluginBindings = fakeCoordinatorPluginBindings{references: coordinatorPluginRefs(mcpprofile.MaxCoordinatorPluginTools + 1)}
	meta := map[string]interface{}{}
	if err := d.svc.stampToolPolicy(meta, d.coordinator, "conversation-1"); err != nil {
		t.Fatalf("stampToolPolicy: %v", err)
	}
	binding, err := mcpprofile.ParseCoordinatorToolPolicyMetadata(meta[mcpprofile.CoordinatorToolPolicyMetadataKey])
	if err != nil {
		t.Fatalf("trimmed binding must parse: %v", err)
	}
	if len(binding.PluginTools) != mcpprofile.MaxCoordinatorPluginTools {
		t.Fatalf("plugin refs = %d, want %d", len(binding.PluginTools), mcpprofile.MaxCoordinatorPluginTools)
	}
	if len(binding.ToolNames) == 0 {
		t.Fatal("core tools must remain after overflow trimming")
	}
}

func TestOpenConversationAcceptsAggregateMultiPluginCatalog(t *testing.T) {
	d := newConversationTestDeps(t)
	d.svc.phase2 = true
	d.svc.pluginBindings = fakeCoordinatorPluginBindings{references: coordinatorPluginRefs(20)}
	result, err := d.svc.OpenConversation(context.Background(), d.coordinator.WorkspaceID, d.coordinator.ID)
	if err != nil {
		t.Fatalf("OpenConversation: %v", err)
	}
	task := d.tasks.tasks[result.TaskID]
	binding, err := mcpprofile.ParseCoordinatorToolPolicyMetadata(task.Metadata[mcpprofile.CoordinatorToolPolicyMetadataKey])
	if err != nil {
		t.Fatalf("persisted binding must parse: %v", err)
	}
	if len(binding.PluginTools) != 20 {
		t.Fatalf("plugin refs = %d, want 20", len(binding.PluginTools))
	}
	if len(binding.ToolNames) == 0 {
		t.Fatal("core tools must remain usable")
	}
}
