package service

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// Uninstall can race a plugin still doing work, and two uninstall attempts can
// overlap on retry. Neither may double-count, panic, or delete anything the
// plugin does not own.
func TestDeleteAllForPluginConcurrentCallsOverRealRepository(t *testing.T) {
	svc, repo := newAgentConversationServiceOverRealRepo(t)
	ctx := context.Background()
	seedConversationWorkspace(t, repo, "ws-one")
	for _, key := range []string{"a", "b", "c", "d", "e"} {
		ensureConversation(t, svc, "plugin-coordinator", "ws-one", key)
	}
	survivor := ensureConversation(t, svc, "plugin-other", "ws-one", "keep")

	var wg sync.WaitGroup
	counts := make([]int32, 4)
	errs := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			counts[i], errs[i] = svc.DeleteAllForPlugin(ctx, "plugin-coordinator")
		}(i)
	}
	wg.Wait()

	var total int32
	for i, c := range counts {
		// A conversation another caller already removed is the goal state, not
		// a failure. Reporting it as one would abort an uninstall that
		// succeeded, because Service.Uninstall aborts on any error here.
		if errs[i] != nil {
			t.Errorf("concurrent cleanup call %d reported an error for work another caller completed: %v", i, errs[i])
		}
		total += c
	}
	if total != 5 {
		t.Errorf("concurrent cleanup reported %d deletions across all callers, want exactly 5 (no double counting, none lost)", total)
	}
	remaining, err := repo.ListEphemeralTasksAllWorkspaces(ctx)
	if err != nil {
		t.Fatalf("list after cleanup: %v", err)
	}
	for _, task := range remaining {
		if isManagedConversationOwnedByPlugin(task, "plugin-coordinator") {
			t.Errorf("conversation %s survived concurrent cleanup", task.ID)
		}
	}
	if !taskExists(t, repo, survivor.TaskID) {
		t.Error("another plugin's conversation was deleted by concurrent cleanup")
	}
}

// Metadata is free-form JSON. A task carrying hostile or wrong-typed
// provenance must never be matched as an uninstalling plugin's conversation.
func TestDeleteAllForPluginIgnoresMalformedProvenance(t *testing.T) {
	svc, repo := newAgentConversationServiceOverRealRepo(t)
	ctx := context.Background()
	seedConversationWorkspace(t, repo, "ws-one")

	decoys := []*models.Task{
		{ID: "decoy-ephemeral-string", WorkspaceID: "ws-one", Title: "d1", Priority: "medium",
			IsEphemeral: true, Origin: models.TaskOriginManual,
			Metadata: map[string]interface{}{"kandev.plugin_id": "plugin-coordinator", "kandev.ephemeral": "true"}},
		{ID: "decoy-plugin-id-number", WorkspaceID: "ws-one", Title: "d2", Priority: "medium",
			IsEphemeral: true, Origin: models.TaskOriginManual,
			Metadata: map[string]interface{}{"kandev.plugin_id": 42, "kandev.ephemeral": true}},
		{ID: "decoy-no-provenance", WorkspaceID: "ws-one", Title: "d3", Priority: "medium",
			IsEphemeral: true, Origin: models.TaskOriginManual,
			Metadata: map[string]interface{}{"kandev.ephemeral": true}},
		{ID: "decoy-other-plugin", WorkspaceID: "ws-one", Title: "d4", Priority: "medium",
			IsEphemeral: true, Origin: models.TaskOriginManual,
			Metadata: map[string]interface{}{"kandev.plugin_id": "plugin-coordinator-2", "kandev.ephemeral": true}},
	}
	for _, task := range decoys {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}
	real := ensureConversation(t, svc, "plugin-coordinator", "ws-one", "coordinator")

	count, err := svc.DeleteAllForPlugin(ctx, "plugin-coordinator")
	if err != nil {
		t.Fatalf("DeleteAllForPlugin: %v", err)
	}
	if count != 1 {
		t.Errorf("DeleteAllForPlugin removed %d tasks, want exactly 1 (only the genuine conversation)", count)
	}
	if taskExists(t, repo, real.TaskID) {
		t.Error("the genuine conversation was not deleted")
	}
	for _, task := range decoys {
		if !taskExists(t, repo, task.ID) {
			t.Errorf("%s was deleted despite not being a conversation owned by the uninstalled plugin", task.ID)
		}
	}
}
