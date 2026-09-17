package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestNativeConversationReusesTaskAndRunner(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `INSERT INTO workspaces (id, office_workflow_id) VALUES ('ws-1', 'office-1')`); err != nil {
		t.Fatal(err)
	}
	agent := &models.AgentInstance{ID: "assistant-1", WorkspaceID: "ws-1", Name: "Chief", Role: models.AgentRoleAssistant}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatal(err)
	}
	first, err := repo.EnsureAgentConversation(ctx, agent)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.EnsureAgentConversation(ctx, agent)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.TaskID != second.TaskID || first.TaskID == "" {
		t.Fatal("reopening must preserve the conversation and its task")
	}
	var count int
	if err := repo.ReaderDB().GetContext(ctx, &count, `SELECT COUNT(*) FROM workflow_step_participants WHERE task_id = ? AND agent_profile_id = ?`, first.TaskID, agent.ID); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("got %d runners, want one", count)
	}
	if first.Platform != "web" || first.WorkspaceID != agent.WorkspaceID {
		t.Fatalf("unexpected channel: %+v", first)
	}
}

func TestWorkspaceChiefPersistsAndRejectsForeignAgent(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	for _, id := range []string{"ws-1", "ws-2"} {
		if _, err := repo.ExecRaw(ctx, `INSERT INTO workspaces (id, office_workflow_id) VALUES (?, 'office')`, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []*models.AgentInstance{
		{ID: "chief", WorkspaceID: "ws-1", Name: "Chief", Role: models.AgentRoleAssistant},
		{ID: "other", WorkspaceID: "ws-2", Name: "Other", Role: models.AgentRoleAssistant},
	} {
		if err := repo.CreateAgentInstance(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SetWorkspaceChief(ctx, "ws-1", "chief"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetWorkspaceChief(ctx, "ws-1", "other"); err == nil {
		t.Fatal("accepted foreign chief")
	}
	got, err := repo.GetWorkspaceChief(ctx, "ws-1")
	if err != nil || got != "chief" {
		t.Fatalf("chief = %q, %v", got, err)
	}
	if err := repo.SetWorkspaceChief(ctx, "ws-1", ""); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetWorkspaceChief(ctx, "ws-1")
	if err != nil || got != "" {
		t.Fatalf("clear = %q, %v", got, err)
	}
}

func TestNativeConversationInExistingKanbanWorkspace(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `INSERT INTO workspaces (id, office_workflow_id) VALUES ('board', '')`); err != nil {
		t.Fatal(err)
	}
	agent := &models.AgentInstance{ID: "chief-board", WorkspaceID: "board", Name: "Chief", Role: models.AgentRoleAssistant}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatal(err)
	}
	channel, err := repo.EnsureAgentConversation(ctx, agent)
	if err != nil {
		t.Fatalf("existing Kanban workspace must support a conversation: %v", err)
	}
	var workflow string
	if err := repo.ReaderDB().GetContext(ctx, &workflow, `SELECT workflow_id FROM tasks WHERE id = ?`, channel.TaskID); err != nil {
		t.Fatal(err)
	}
	if workflow != "" {
		t.Fatalf("conversation must not create a delivery workflow: %s", workflow)
	}
}

func TestDeletedChiefCannotRemainSelected(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	agent := &models.AgentInstance{ID: "deleted-chief", WorkspaceID: "ws", Name: "Chief", Role: models.AgentRoleAssistant}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetWorkspaceChief(ctx, "ws", agent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExecRaw(ctx, `UPDATE agent_profiles SET deleted_at=CURRENT_TIMESTAMP WHERE id=?`, agent.ID); err != nil {
		t.Fatal(err)
	}
	id, err := repo.GetWorkspaceChief(ctx, "ws")
	if err != nil || id != "" {
		t.Fatalf("deleted chief returned: %s %v", id, err)
	}
	if err := repo.SetWorkspaceChief(ctx, "ws", agent.ID); err == nil {
		t.Fatal("deleted chief accepted")
	}
}
