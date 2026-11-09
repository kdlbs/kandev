package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type builtinSnapshotReader struct {
	reads    int
	snapshot *mcpconfig.BuiltinPolicySnapshot
	err      error
}

func (r *builtinSnapshotReader) GetBuiltinPolicySnapshot(_ context.Context, workspaceID, taskID string) (*mcpconfig.BuiltinPolicySnapshot, error) {
	r.reads++
	return r.snapshot, r.err
}

func TestBuiltinLaunchPreparationUsesOnePolicySnapshot(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task"] = &models.Task{ID: "task", WorkspaceID: "workspace"}
	session := &models.TaskSession{ID: "session", TaskID: "task"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	reader := &builtinSnapshotReader{snapshot: &mcpconfig.BuiltinPolicySnapshot{Policy: mcpconfig.DefaultBuiltinWorkspacePolicy("workspace"), Preferences: &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "workspace", TaskID: "task", Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}}}
	exec.SetBuiltinPolicyReader(reader)
	view, err := exec.ResolveTaskSessionMCPProfile(context.Background(), "task", session, true)
	require.NoError(t, err)
	require.Nil(t, view.BuiltinPackages)
	require.Zero(t, reader.reads)
	prepared, err := exec.PrepareTaskSessionMCPProfile(context.Background(), "task", session, true)
	require.NoError(t, err)
	require.NotNil(t, prepared.BuiltinPackages)
	require.Contains(t, prepared.BuiltinPackages.Packages, p.PackageConfiguration)
	reader.snapshot.Policy.Packages[p.PackageDiagnostics] = "blocked"
	require.Contains(t, prepared.BuiltinPackages.Packages, p.PackageDiagnostics, "a later policy save cannot rewrite the prepared value")
	require.Equal(t, 1, reader.reads)
	reader.err = errors.New("store unavailable")
	_, err = exec.PrepareTaskSessionMCPProfile(context.Background(), "task", session, true)
	require.Error(t, err, "a read failure must not choose defaults")
	session.Metadata = map[string]any{"config_mode": true}
	configured, err := exec.PrepareTaskSessionMCPProfile(context.Background(), "task", session, true)
	require.NoError(t, err)
	require.ElementsMatch(t, []p.PackageID{p.PackageCore, p.PackageConfiguration}, configured.BuiltinPackages.Packages)
	require.Equal(t, 2, reader.reads, "configuration chats have a fixed catalog")
}

func TestBuiltinPreparedContextKeepsLaunchSnapshot(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task"] = &models.Task{ID: "task", WorkspaceID: "workspace"}
	session := &models.TaskSession{ID: "session", TaskID: "task"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	reader := &builtinSnapshotReader{snapshot: &mcpconfig.BuiltinPolicySnapshot{Policy: mcpconfig.DefaultBuiltinWorkspacePolicy("workspace"), Preferences: &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "workspace", TaskID: "task"}}}
	exec.SetBuiltinPolicyReader(reader)
	ctx, err := exec.PrepareMCPLaunch(context.Background(), "task", session, true)
	require.NoError(t, err)
	reader.err = errors.New("policy changed or unavailable")
	profile, err := exec.PrepareTaskSessionMCPProfile(ctx, "task", session, true)
	require.NoError(t, err)
	require.Contains(t, profile.BuiltinPackages.Packages, p.PackageRichOutput)
	require.Equal(t, 1, reader.reads)
	profile.BuiltinPackages.Packages = nil
	cached := PreparedMCPProfile(ctx, "task", "session")
	require.NotEmpty(t, cached.BuiltinPackages.Packages, "callers cannot mutate the captured launch")
	require.Nil(t, PreparedMCPProfile(ctx, "other", "session"))
	require.Nil(t, PreparedMCPProfile(ctx, "task", "other"))
}

func TestBuiltinFreshResumeAndModelSwitchResolveCurrentPolicy(t *testing.T) {
	repo := newMockRepository()
	task := &models.Task{ID: "task", WorkspaceID: "workspace"}
	session := &models.TaskSession{ID: "session", TaskID: "task", AgentProfileID: "profile"}
	repo.tasks[task.ID], repo.sessions[session.ID] = task, session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	reader := &builtinSnapshotReader{snapshot: &mcpconfig.BuiltinPolicySnapshot{Policy: mcpconfig.DefaultBuiltinWorkspacePolicy("workspace"), Preferences: &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "workspace", TaskID: "task", Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}}}
	exec.SetBuiltinPolicyReader(reader)
	req := &LaunchAgentRequest{StartAgent: true, TaskDescription: "resume this work"}
	require.NoError(t, exec.applyResumeMCPSettings(context.Background(), task.ID, session, req))
	require.NotNil(t, req.McpProfile.BuiltinPackages)
	require.Contains(t, req.McpProfile.BuiltinPackages.Packages, p.PackageConfiguration)
	require.Contains(t, req.TaskDescription, "resume this work")
	require.Contains(t, req.TaskDescription, "kandev-configuration")
	require.Contains(t, req.TaskDescription, "supersedes earlier optional-tool guidance")
	reader.snapshot.Preferences.Packages[p.PackageConfiguration] = "off"
	switched, err := exec.buildSwitchModelRequest(context.Background(), task, session, session.ID, "model", "request", "", executorConfig{Metadata: map[string]any{}}, nil)
	require.NoError(t, err)
	require.NotNil(t, switched.McpProfile.BuiltinPackages)
	require.NotContains(t, switched.McpProfile.BuiltinPackages.Packages, p.PackageConfiguration)
	require.Contains(t, switched.TaskDescription, "request")
	require.Contains(t, switched.TaskDescription, "BUILT-IN PACKAGES:")
	require.NotContains(t, switched.TaskDescription, "kandev-configuration")
	require.Equal(t, 2, reader.reads)
	for _, startAgent := range []bool{false, true} {
		empty := &LaunchAgentRequest{StartAgent: startAgent}
		require.NoError(t, exec.applyResumeMCPSettings(context.Background(), task.ID, session, empty))
		require.Empty(t, empty.TaskDescription, "workspace preparation and deferred prompts must remain empty")
	}
}
