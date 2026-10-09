package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func forgedCarrierMetadata() map[string]interface{} {
	return map[string]interface{}{
		models.MetaKeyOfficeCarrierCausationID:    "attacker-chosen-causation-id",
		models.MetaKeyOfficeCarrierCausationDepth: 0,
		models.MetaKeyOfficeCarrierCreatingRunID:  "attacker-chosen-run-id",
		models.MetaKeyOfficeCarrierHumanRooted:    true,
		models.MetaKeyOfficeCarrierRoutineID:      "",
		models.MetaKeyOfficeCarrierActorKind:      "system",
		models.MetaKeyOfficeCarrierActorID:        "",
	}
}

func assertNoCarrierKeys(t *testing.T, metadata map[string]interface{}) {
	t.Helper()
	for _, key := range []string{
		models.MetaKeyOfficeCarrierCausationID,
		models.MetaKeyOfficeCarrierCausationDepth,
		models.MetaKeyOfficeCarrierCreatingRunID,
		models.MetaKeyOfficeCarrierHumanRooted,
		models.MetaKeyOfficeCarrierRoutineID,
		models.MetaKeyOfficeCarrierActorKind,
		models.MetaKeyOfficeCarrierActorID,
	} {
		if v, ok := metadata[key]; ok {
			t.Fatalf("caller-supplied metadata key %q survived into persisted task metadata: %v", key, v)
		}
	}
}

func TestCreateTask_StripsCallerSuppliedOfficeCarrierMetadata(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-carrier-forge", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{ //nolint:exhaustruct
		WorkspaceID: "ws-carrier-forge",
		Title:       "Forged carrier attempt",
		IsEphemeral: true,
		Metadata:    forgedCarrierMetadata(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	assertNoCarrierKeys(t, result.Task.Metadata)

	reloaded, err := svc.GetTask(ctx, result.Task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	assertNoCarrierKeys(t, reloaded.Metadata)
}

func TestCreateTask_AppliesTrustedOfficeCarrierMetadataOverForgedOnes(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-carrier-trusted", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	trusted := map[string]interface{}{
		models.MetaKeyOfficeCarrierCausationID:    "real-causation-id",
		models.MetaKeyOfficeCarrierCausationDepth: 2,
		models.MetaKeyOfficeCarrierCreatingRunID:  "real-run-id",
		models.MetaKeyOfficeCarrierHumanRooted:    false,
		models.MetaKeyOfficeCarrierRoutineID:      "real-routine-id",
		models.MetaKeyOfficeCarrierActorKind:      "agent",
		models.MetaKeyOfficeCarrierActorID:        "real-agent-id",
	}

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{ //nolint:exhaustruct
		WorkspaceID:           "ws-carrier-trusted",
		Title:                 "Trusted carrier write",
		IsEphemeral:           true,
		Metadata:              forgedCarrierMetadata(),
		OfficeCarrierMetadata: trusted,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	for key, want := range trusted {
		// Task metadata round-trips through the repository's JSON encoding,
		// which turns a Go int into float64 on the way back out, so compare
		// formatted values rather than exact types.
		if got := result.Task.Metadata[key]; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("metadata[%q] = %v, want trusted value %v (request-body value must never win)", key, got, want)
		}
	}
}

func TestUpdateTask_StripsCallerSuppliedOfficeCarrierMetadata(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-carrier-update", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	created, err := svc.CreateTask(ctx, &CreateTaskRequest{ //nolint:exhaustruct
		WorkspaceID: "ws-carrier-update",
		Title:       "Task later patched",
		IsEphemeral: true,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	title := "Task later patched"
	updated, err := svc.UpdateTask(ctx, created.Task.ID, &UpdateTaskRequest{ //nolint:exhaustruct
		Title:    &title,
		Metadata: forgedCarrierMetadata(),
	})
	if err != nil {
		t.Fatalf("update task: %v", err)
	}
	assertNoCarrierKeys(t, updated.Metadata)

	reloaded, err := svc.GetTask(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	assertNoCarrierKeys(t, reloaded.Metadata)
}

// TestUpdateTask_PreservesExistingOfficeCarrierMetadataAcrossAGenericUpdate
// pins Review round 16's RR16-F2: protectedTaskMetadataUpdate stripped
// caller-supplied carrier keys but never restored the task's own
// server-written carrier from existing, so any UpdateTask carrying non-nil
// metadata deleted it — after which a wake caused by this task would root a
// fresh causation chain at depth 0 instead of advancing the one already on
// the task, defeating REQ-OFFICE-LAUNCH-SAFETY-003's depth ceiling. The
// carrier is set here the same trusted way task creation sets it
// (OfficeCarrierMetadata, never request metadata), then an ordinary update
// — unrelated field, no carrier keys in the request — must leave it intact.
func TestUpdateTask_PreservesExistingOfficeCarrierMetadataAcrossAGenericUpdate(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-carrier-preserve", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	trusted := map[string]interface{}{
		models.MetaKeyOfficeCarrierCausationID:    "real-causation-id",
		models.MetaKeyOfficeCarrierCausationDepth: 2,
		models.MetaKeyOfficeCarrierCreatingRunID:  "real-run-id",
		models.MetaKeyOfficeCarrierHumanRooted:    false,
		models.MetaKeyOfficeCarrierRoutineID:      "real-routine-id",
		models.MetaKeyOfficeCarrierActorKind:      "agent",
		models.MetaKeyOfficeCarrierActorID:        "real-agent-id",
	}
	created, err := svc.CreateTask(ctx, &CreateTaskRequest{ //nolint:exhaustruct
		WorkspaceID:           "ws-carrier-preserve",
		Title:                 "Task carrying a real carrier",
		IsEphemeral:           true,
		OfficeCarrierMetadata: trusted,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	updated, err := svc.UpdateTask(ctx, created.Task.ID, &UpdateTaskRequest{ //nolint:exhaustruct
		Metadata: map[string]interface{}{"unrelated_key": "unrelated_value"},
	})
	if err != nil {
		t.Fatalf("update task: %v", err)
	}
	for key, want := range trusted {
		// Task metadata round-trips through the repository's JSON encoding,
		// which turns a Go int into float64 on the way back out, so compare
		// formatted values rather than exact types.
		if got := updated.Metadata[key]; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("metadata[%q] = %v, want preserved existing value %v (a generic update must never delete the carrier)", key, got, want)
		}
	}
	if got := updated.Metadata["unrelated_key"]; got != "unrelated_value" {
		t.Fatalf("metadata[unrelated_key] = %v, want unrelated_value survived alongside the preserved carrier", got)
	}

	reloaded, err := svc.GetTask(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	for key, want := range trusted {
		if got := reloaded.Metadata[key]; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("reloaded metadata[%q] = %v, want preserved existing value %v", key, got, want)
		}
	}
}
