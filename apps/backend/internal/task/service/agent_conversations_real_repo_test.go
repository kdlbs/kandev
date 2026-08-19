package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// The uninstall cleanup path is otherwise only proven against hand-written
// fakes: internal/plugins' uninstall tests use a fake AgentConversationService,
// and this package's own tests use in-memory task repos that hand back the
// very *models.Task pointer they were given. Neither can catch the thing most
// likely to break provenance matching in production — task metadata makes a
// round trip through JSON in SQLite, and DeleteAllForPlugin's ownership check
// type-asserts kandev.ephemeral back to a bool. These tests run Ensure and
// DeleteAllForPlugin over the real repository so that round trip is real.

func newAgentConversationServiceOverRealRepo(t *testing.T) (*AgentConversationService, *sqliterepo.Repository) {
	t.Helper()
	_, _, repo := createTestService(t)
	svc := NewAgentConversationService(repo, repo, nil, newACFakeStateRepo(), nil)
	svc.SetDispatcher(newACFakeDispatcher())
	return svc, repo
}

func seedConversationWorkspace(t *testing.T, repo *sqliterepo.Repository, workspaceID string) {
	t.Helper()
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatalf("create workspace %s: %v", workspaceID, err)
	}
}

func ensureConversation(t *testing.T, svc *AgentConversationService, pluginID, workspaceID, key string) pluginsdk.AgentConversationDescriptor {
	t.Helper()
	desc, status, err := svc.Ensure(context.Background(), pluginID, pluginsdk.AgentConversationSpec{
		WorkspaceID:     workspaceID,
		ConversationKey: key,
	})
	if err != nil {
		t.Fatalf("Ensure(%s/%s/%s): %v", pluginID, workspaceID, key, err)
	}
	if status != AgentConversationStatusCreated {
		t.Fatalf("Ensure(%s/%s/%s) status = %q, want %q", pluginID, workspaceID, key, status, AgentConversationStatusCreated)
	}
	return desc
}

func taskExists(t *testing.T, repo *sqliterepo.Repository, taskID string) bool {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	return err == nil && task != nil
}

// Uninstall cleanup has to survive the metadata JSON round trip, reach every
// workspace, and take the conversation's sessions with it.
func TestDeleteAllForPluginOverRealRepository(t *testing.T) {
	svc, repo := newAgentConversationServiceOverRealRepo(t)
	ctx := context.Background()
	seedConversationWorkspace(t, repo, "ws-one")
	seedConversationWorkspace(t, repo, "ws-two")

	first := ensureConversation(t, svc, "plugin-coordinator", "ws-one", "coordinator")
	second := ensureConversation(t, svc, "plugin-coordinator", "ws-two", "coordinator")

	// Re-read one conversation through the repository before deleting, so a
	// metadata round trip that loses kandev.ephemeral fails here rather than
	// silently making the delete a no-op.
	stored, err := repo.GetTask(ctx, first.TaskID)
	if err != nil {
		t.Fatalf("re-read conversation task: %v", err)
	}
	if !isManagedConversationOwnedByPlugin(stored, "plugin-coordinator") {
		t.Fatalf("conversation task read back from SQLite is no longer recognised as owned; metadata = %#v", stored.Metadata)
	}

	count, err := svc.DeleteAllForPlugin(ctx, "plugin-coordinator")
	if err != nil {
		t.Fatalf("DeleteAllForPlugin: %v", err)
	}
	if count != 2 {
		t.Fatalf("DeleteAllForPlugin removed %d conversations, want 2 (one per workspace)", count)
	}
	for _, desc := range []pluginsdk.AgentConversationDescriptor{first, second} {
		if taskExists(t, repo, desc.TaskID) {
			t.Errorf("conversation task %s still exists after uninstall cleanup", desc.TaskID)
		}
		session, err := repo.GetTaskSession(ctx, desc.SessionID)
		if err == nil && session != nil {
			t.Errorf("session %s survived deletion of its conversation task %s", desc.SessionID, desc.TaskID)
		}
	}
}

// The blast radius of a cleanup that runs during uninstall matters more than
// the cleanup itself: everything not owned by the uninstalled plugin has to
// survive it.
func TestDeleteAllForPluginOverRealRepositoryLeavesEverythingElse(t *testing.T) {
	svc, repo := newAgentConversationServiceOverRealRepo(t)
	ctx := context.Background()
	seedConversationWorkspace(t, repo, "ws-one")

	victim := ensureConversation(t, svc, "plugin-coordinator", "ws-one", "coordinator")
	otherPlugin := ensureConversation(t, svc, "plugin-notes", "ws-one", "notes")

	ordinary := &models.Task{
		ID:          "ordinary-user-task",
		WorkspaceID: "ws-one",
		Title:       "A real user task",
		Priority:    "medium",
		Origin:      models.TaskOriginManual,
	}
	if err := repo.CreateTask(ctx, ordinary); err != nil {
		t.Fatalf("create ordinary task: %v", err)
	}

	count, err := svc.DeleteAllForPlugin(ctx, "plugin-coordinator")
	if err != nil {
		t.Fatalf("DeleteAllForPlugin: %v", err)
	}
	if count != 1 {
		t.Fatalf("DeleteAllForPlugin removed %d conversations, want exactly 1", count)
	}
	if taskExists(t, repo, victim.TaskID) {
		t.Error("the uninstalled plugin's own conversation survived")
	}
	if !taskExists(t, repo, otherPlugin.TaskID) {
		t.Error("another plugin's managed conversation was deleted by an unrelated uninstall")
	}
	if !taskExists(t, repo, ordinary.ID) {
		t.Error("an ordinary user task was deleted by a plugin uninstall")
	}
}

// An empty plugin id must never be treated as "match everything".
func TestDeleteAllForPluginRejectsEmptyPluginIDOverRealRepository(t *testing.T) {
	svc, repo := newAgentConversationServiceOverRealRepo(t)
	seedConversationWorkspace(t, repo, "ws-one")
	kept := ensureConversation(t, svc, "plugin-coordinator", "ws-one", "coordinator")

	if _, err := svc.DeleteAllForPlugin(context.Background(), ""); err == nil {
		t.Fatal("expected an error for an empty plugin id")
	}
	if !taskExists(t, repo, kept.TaskID) {
		t.Error("an empty plugin id deleted a conversation")
	}
}
