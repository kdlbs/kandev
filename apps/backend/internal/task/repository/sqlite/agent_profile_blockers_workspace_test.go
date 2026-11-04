package sqlite

import (
	"context"
	"testing"

	agentdto "github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/task/models"
)

// Each blocker row names its workspace so the delete-conflict dialog can tell
// the user where to find the task.
func TestAgentProfileBlockersIncludeWorkspace(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()

	const profileID = "agent-profile-ws"
	for _, spec := range []struct{ sessionID, taskID string }{
		{"sess-board", f.boardTaskID},
		{"sess-quick", f.quickTaskID},
	} {
		if err := f.repo.CreateTaskSession(ctx, &models.TaskSession{
			ID:             spec.sessionID,
			TaskID:         spec.taskID,
			AgentProfileID: profileID,
			State:          models.TaskSessionStateRunning,
		}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", spec.sessionID, err)
		}
	}

	blockers, err := f.repo.GetActiveTaskInfoByAgentProfile(ctx, profileID)
	if err != nil {
		t.Fatalf("GetActiveTaskInfoByAgentProfile: %v", err)
	}
	if len(blockers) != 2 {
		t.Fatalf("expected 2 blockers, got %+v", blockers)
	}
	for _, info := range blockers {
		if info.WorkspaceID != autoOriginWorkspaceID || info.WorkspaceName != "Automation origin" {
			t.Fatalf("blocker %s: want workspace %q/%q, got %q/%q",
				info.TaskID, autoOriginWorkspaceID, "Automation origin", info.WorkspaceID, info.WorkspaceName)
		}
	}
}

// Blockers in more than one workspace must each name their own workspace: a
// query that copies one row's workspace onto every result would otherwise pass
// the single-workspace test above.
func TestAgentProfileBlockersNameEachTasksWorkspace(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()

	const profileID = "agent-profile-two-ws"
	const secondWorkspaceID = "ws-second-origin"
	if err := f.repo.CreateWorkspace(ctx, &models.Workspace{ID: secondWorkspaceID, Name: "Second origin"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := f.repo.CreateTask(ctx, &models.Task{
		ID:          "t-second",
		WorkspaceID: secondWorkspaceID,
		Title:       "second workspace work",
		State:       "BACKLOG",
		Priority:    "medium",
		Origin:      models.TaskOriginManual,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	for _, spec := range []struct{ sessionID, taskID string }{
		{"sess-first", f.boardTaskID},
		{"sess-second", "t-second"},
	} {
		if err := f.repo.CreateTaskSession(ctx, &models.TaskSession{
			ID:             spec.sessionID,
			TaskID:         spec.taskID,
			AgentProfileID: profileID,
			State:          models.TaskSessionStateRunning,
		}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", spec.sessionID, err)
		}
	}

	blockers, err := f.repo.GetActiveTaskInfoByAgentProfile(ctx, profileID)
	if err != nil {
		t.Fatalf("GetActiveTaskInfoByAgentProfile: %v", err)
	}
	byTask := make(map[string]agentdto.ActiveTaskInfo, len(blockers))
	for _, info := range blockers {
		byTask[info.TaskID] = info
	}
	want := map[string]struct{ id, name string }{
		f.boardTaskID: {autoOriginWorkspaceID, "Automation origin"},
		"t-second":    {secondWorkspaceID, "Second origin"},
	}
	if len(blockers) != len(want) {
		t.Fatalf("expected %d blockers, got %+v", len(want), blockers)
	}
	for taskID, expected := range want {
		info, ok := byTask[taskID]
		if !ok {
			t.Fatalf("missing blocker for task %s: %+v", taskID, blockers)
		}
		if info.WorkspaceID != expected.id || info.WorkspaceName != expected.name {
			t.Fatalf("blocker %s: want workspace %q/%q, got %q/%q",
				taskID, expected.id, expected.name, info.WorkspaceID, info.WorkspaceName)
		}
	}
}

// A task whose workspace row is gone must still be reported, with an empty
// workspace name, rather than silently dropped from the blocker list.
func TestAgentProfileBlockersKeepTaskWithMissingWorkspace(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()

	const profileID = "agent-profile-orphan"
	if err := f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:             "sess-orphan",
		TaskID:         f.boardTaskID,
		AgentProfileID: profileID,
		State:          models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if _, err := f.repo.db.ExecContext(ctx, `UPDATE tasks SET workspace_id = 'ws-missing' WHERE id = ?`, f.boardTaskID); err != nil {
		t.Fatalf("detach workspace: %v", err)
	}

	blockers, err := f.repo.GetActiveTaskInfoByAgentProfile(ctx, profileID)
	if err != nil {
		t.Fatalf("GetActiveTaskInfoByAgentProfile: %v", err)
	}
	if len(blockers) != 1 {
		t.Fatalf("expected 1 blocker, got %+v", blockers)
	}
	if blockers[0].WorkspaceID != "ws-missing" || blockers[0].WorkspaceName != "" {
		t.Fatalf("want workspace ws-missing with empty name, got %+v", blockers[0])
	}
}
