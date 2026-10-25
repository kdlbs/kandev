package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestProtectedTaskMetadataUpdatePreservesStepHandoffCarry(t *testing.T) {
	existing := map[string]interface{}{
		"ordinary": "old",
		models.MetaKeyStepHandoffCarry: models.StepHandoffCarryToken{
			Handoff: "trusted handoff",
			StepID:  "step-next",
			Stamp:   "trusted-stamp",
		},
	}
	requested := map[string]interface{}{
		"ordinary": "new",
		models.MetaKeyStepHandoffCarry: models.StepHandoffCarryToken{
			Handoff: "forged handoff",
			StepID:  "step-next",
			Stamp:   "forged-stamp",
		},
	}

	updated := protectedTaskMetadataUpdate(existing, requested)
	if got := updated["ordinary"]; got != "new" {
		t.Fatalf("ordinary metadata = %v, want new", got)
	}
	if got := updated[models.MetaKeyStepHandoffCarry]; got != existing[models.MetaKeyStepHandoffCarry] {
		t.Fatalf("carry token = %#v, want existing token", got)
	}
}

func TestProtectedTaskMetadataUpdateRejectsNewStepHandoffCarry(t *testing.T) {
	requested := map[string]interface{}{
		models.MetaKeyStepHandoffCarry: models.StepHandoffCarryToken{
			Handoff: "forged handoff",
			StepID:  "step-next",
			Stamp:   "forged-stamp",
		},
	}

	updated := protectedTaskMetadataUpdate(nil, requested)
	if _, ok := updated[models.MetaKeyStepHandoffCarry]; ok {
		t.Fatalf("new carry token must not be accepted from client metadata")
	}
}

func TestProtectedTaskMetadataUpdatePreservesTaskHandoffRecords(t *testing.T) {
	existing := map[string]interface{}{
		models.MetaKeyHandoffSource: map[string]interface{}{"source_task_id": "source-1"},
		models.MetaKeyHandoffs:      []interface{}{map[string]interface{}{"task_id": "delivery-1"}},
	}
	requested := map[string]interface{}{
		"ordinary":                  "new",
		models.MetaKeyHandoffSource: map[string]interface{}{"source_task_id": "forged"},
		models.MetaKeyHandoffs:      []interface{}{},
	}

	updated := protectedTaskMetadataUpdate(existing, requested)
	if got := updated[models.MetaKeyHandoffSource]; !reflect.DeepEqual(got, existing[models.MetaKeyHandoffSource]) {
		t.Fatalf("handoff source = %#v, want existing value", got)
	}
	if got := updated[models.MetaKeyHandoffs]; !reflect.DeepEqual(got, existing[models.MetaKeyHandoffs]) {
		t.Fatalf("handoffs = %#v, want existing value", got)
	}
}

func TestProtectedTaskMetadataUpdateRejectsNewTaskHandoffRecords(t *testing.T) {
	requested := map[string]interface{}{
		models.MetaKeyHandoffSource: map[string]interface{}{"source_task_id": "forged"},
		models.MetaKeyHandoffs:      []interface{}{map[string]interface{}{"task_id": "forged"}},
	}

	updated := protectedTaskMetadataUpdate(nil, requested)
	if _, ok := updated[models.MetaKeyHandoffSource]; ok {
		t.Fatal("new handoff source must not be accepted from client metadata")
	}
	if _, ok := updated[models.MetaKeyHandoffs]; ok {
		t.Fatal("new handoffs must not be accepted from client metadata")
	}
}

func TestProtectedTaskMetadataForCreateRequiresTrustedHandoff(t *testing.T) {
	metadata := map[string]interface{}{
		"ordinary":                  "keep",
		models.MetaKeyHandoffSource: map[string]interface{}{"source_task_id": "forged"},
		models.MetaKeyHandoffs:      []interface{}{map[string]interface{}{"task_id": "forged"}},
	}

	untrusted := protectedTaskMetadataForCreate(metadata, false)
	if _, ok := untrusted[models.MetaKeyHandoffSource]; ok {
		t.Fatal("untrusted handoff source must not be persisted")
	}
	if _, ok := untrusted[models.MetaKeyHandoffs]; ok {
		t.Fatal("untrusted handoffs must not be persisted")
	}
	if got := metadata[models.MetaKeyHandoffSource]; got == nil {
		t.Fatal("sanitizing metadata must not mutate the request map")
	}

	trusted := protectedTaskMetadataForCreate(metadata, true)
	if _, ok := trusted[models.MetaKeyHandoffSource]; !ok {
		t.Fatal("trusted handoff source must be persisted")
	}
	if _, ok := trusted[models.MetaKeyHandoffs]; !ok {
		t.Fatal("trusted handoffs must be persisted")
	}
}

func TestProtectedTaskMetadataForCreateRejectsStepHandoffCarry(t *testing.T) {
	metadata := map[string]interface{}{
		"ordinary": "keep",
		models.MetaKeyStepHandoffCarry: models.StepHandoffCarryToken{
			Handoff: "forged handoff",
			StepID:  "step-next",
			Stamp:   "forged-stamp",
		},
	}

	untrusted := protectedTaskMetadataForCreate(metadata, false)
	if _, ok := untrusted[models.MetaKeyStepHandoffCarry]; ok {
		t.Fatal("forged step handoff carry must not be accepted from an ordinary create call")
	}
	if got := untrusted["ordinary"]; got != "keep" {
		t.Fatalf("ordinary metadata = %v, want keep", got)
	}

	trusted := protectedTaskMetadataForCreate(metadata, true)
	if _, ok := trusted[models.MetaKeyStepHandoffCarry]; ok {
		t.Fatal("step handoff carry is server-only and must never be set through create, trusted or not")
	}
}

func TestProtectedTaskMetadataForCreateRejectsAutomationStartOwnership(t *testing.T) {
	metadata := map[string]interface{}{
		"ordinary":                         "keep",
		models.MetaKeyAutomationStartOwned: true,
	}

	for _, trustedHandoff := range []bool{false, true} {
		created := protectedTaskMetadataForCreate(metadata, trustedHandoff)
		if _, ok := created[models.MetaKeyAutomationStartOwned]; ok {
			t.Fatalf("trustedHandoff=%v: metadata kept caller-supplied %s", trustedHandoff, models.MetaKeyAutomationStartOwned)
		}
		if got := created["ordinary"]; got != "keep" {
			t.Fatalf("trustedHandoff=%v: ordinary metadata = %v, want keep", trustedHandoff, got)
		}
	}
}

func TestBuildTaskStampsAutomationStartOwnershipOnlyFromInternalField(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	forged := svc.buildTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "forged",
		Metadata: map[string]interface{}{models.MetaKeyAutomationStartOwned: true},
	}, "")
	if models.AutomationStartOwned(forged.Metadata) {
		t.Fatal("a create request's metadata must not reserve the task's start")
	}

	owned := svc.buildTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "automation", AutomationStartOwned: true, AutomationStartOwner: "admitted-run",
	}, "")
	if !models.AutomationStartOwned(owned.Metadata) {
		t.Fatalf("automation create metadata = %v, want %s=true", owned.Metadata, models.MetaKeyAutomationStartOwned)
	}
	if owner := owned.Metadata[models.MetaKeyAutomationStartOwned]; owner != "admitted-run" {
		t.Fatalf("initial start owner = %v, want admitted-run", owner)
	}
}

func TestUpdateTaskMetadataPreservesAutomationStartOwnership(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-owned", Name: "Owned workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	for _, task := range []*models.Task{
		{ID: "task-owned", Metadata: map[string]interface{}{models.MetaKeyAutomationStartOwned: true}},
		{ID: "task-plain"},
	} {
		task.WorkspaceID, task.Title, task.Priority = "ws-owned", task.ID, "medium"
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}

	owned, err := svc.UpdateTaskMetadata(ctx, "task-owned", map[string]interface{}{"ordinary": "new"})
	if err != nil {
		t.Fatalf("UpdateTaskMetadata owned: %v", err)
	}
	if !models.AutomationStartOwned(owned.Metadata) {
		t.Fatal("an ordinary metadata update must not release the automation start reservation")
	}
	plain, err := svc.UpdateTaskMetadata(ctx, "task-plain", map[string]interface{}{models.MetaKeyAutomationStartOwned: true})
	if err != nil {
		t.Fatalf("UpdateTaskMetadata plain: %v", err)
	}
	if models.AutomationStartOwned(plain.Metadata) {
		t.Fatal("an ordinary metadata update must not reserve a task's start")
	}
}

func TestUpdateTaskMetadataPreservesTaskHandoffRecords(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-handoff", Name: "Handoff workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-handoff", WorkspaceID: "ws-handoff", Title: "Handoff task", Priority: "medium",
		Metadata: map[string]interface{}{
			models.MetaKeyHandoffSource: map[string]interface{}{"source_task_id": "source-1"},
			models.MetaKeyHandoffs:      []interface{}{map[string]interface{}{"task_id": "delivery-1"}},
		},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	updated, err := svc.UpdateTaskMetadata(ctx, "task-handoff", map[string]interface{}{
		"ordinary":                  "new",
		models.MetaKeyHandoffSource: map[string]interface{}{"source_task_id": "forged"},
		models.MetaKeyHandoffs:      []interface{}{},
	})
	if err != nil {
		t.Fatalf("UpdateTaskMetadata: %v", err)
	}
	if got := updated.Metadata[models.MetaKeyHandoffSource].(map[string]interface{})["source_task_id"]; got != "source-1" {
		t.Fatalf("updated handoff source = %v, want source-1", got)
	}
	if got := updated.Metadata[models.MetaKeyHandoffs].([]interface{}); len(got) != 1 {
		t.Fatalf("updated handoffs = %#v, want one existing entry", got)
	}
	if got := updated.Metadata["ordinary"]; got != "new" {
		t.Fatalf("updated ordinary metadata = %v, want new", got)
	}
}
