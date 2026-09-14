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
