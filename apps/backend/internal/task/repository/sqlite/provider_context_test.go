package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestBuiltinGenerationRuntimeContextCannotRotateBinding(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task")
	seedExecutorRunningCleanupSession(t, repo, "task", "session", models.TaskSessionStateWaitingForInput, "execution")
	task, err := repo.GetTask(ctx, "task")
	require.NoError(t, err)
	base := p.New(p.SurfaceKanbanTask, nil, nil)
	base.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, TaskID: "task", WorkspaceID: task.WorkspaceID, Surface: base.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	sealed, err := p.MarshalBuiltinBinding(base)
	require.NoError(t, err)
	digest, err := p.BuiltinBindingDigest(base)
	require.NoError(t, err)
	binding := p.ProviderMCPBinding{Scope: p.ProviderMCPScope{BindingID: "accepted", ExecutionID: "execution", HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 1, ProfileDigest: digest}, Profile: sealed}
	metadata := map[string]any{p.BuiltinBindingMetadataKey: sealed, p.ProviderMCPBindingMetadataKey: binding, "unrelated": "kept"}
	payload, err := json.Marshal(metadata)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE executors_running SET metadata=? WHERE session_id=?`), string(payload), "session")
	require.NoError(t, err)
	stale, err := repo.GetExecutorRunningBySessionID(ctx, "session")
	require.NoError(t, err)
	writer, ok := any(repo).(interface {
		UpdateProviderMCPRuntimeProfile(context.Context, string, string, string, any) error
	})
	require.True(t, ok, "scoped context needs a binding-fenced narrow writer")
	live := base.WithCapability(p.CapabilityTaskTitle)
	value, err := p.MarshalBuiltinBinding(live)
	require.NoError(t, err)
	require.ErrorIs(t, writer.UpdateProviderMCPRuntimeProfile(ctx, "session", "execution", "previous", value), models.ErrExecutionRotated)
	require.NoError(t, writer.UpdateProviderMCPRuntimeProfile(ctx, "session", "execution", "accepted", value))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, stale))
	current, err := repo.GetExecutorRunningBySessionID(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, value, current.Metadata[p.ProviderMCPRuntimeProfileMetadataKey], "late runtime snapshots cannot erase a context update")
	actual, _, err := p.ParseProviderMCPBinding(current.Metadata[p.ProviderMCPBindingMetadataKey])
	require.NoError(t, err)
	require.Equal(t, binding, actual)
	require.Equal(t, sealed, current.Metadata[p.BuiltinBindingMetadataKey])
	require.Equal(t, "kept", current.Metadata["unrelated"])
	changed := live
	changed.BuiltinPackages = live.BuiltinPackages.Clone()
	changed.BuiltinPackages.Packages = append(changed.BuiltinPackages.Packages, p.PackageConfiguration)
	changed.BuiltinPackages.Origins[p.PackageConfiguration] = "task"
	bad, err := p.MarshalBuiltinBinding(changed)
	require.NoError(t, err)
	require.Error(t, writer.UpdateProviderMCPRuntimeProfile(ctx, "session", "execution", "accepted", bad), "context updates cannot grant a package")
}
