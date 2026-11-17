package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	p "github.com/kandev/kandev/internal/mcp/profile"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestBuiltinApplicationViewSeparatesSavedIntentFromAcceptedBinding(t *testing.T) {
	svc, _ := newBuiltinIntentResumeFixture(t, "retired", true)
	repo := svc.repo.(*sqliterepo.Repository)
	task, err := repo.GetTask(context.Background(), "task")
	require.NoError(t, err)
	_, err = repo.DB().Exec(`INSERT INTO task_builtin_mcp_applications
 (session_id,task_id,workspace_id,target_revision,policy_revision,selection_hash,requested_by,request_id,created_at,updated_at)
 VALUES ('session','task',?,3,0,'hash','human','request',?,?)`, task.WorkspaceID, time.Now().UTC(), time.Now().UTC())
	require.NoError(t, err)
	view, err := svc.ReadBuiltinSessionMCP(context.Background(), "session")
	require.NoError(t, err)
	data, err := json.Marshal(view)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Contains(t, decoded, "application", "saved application intent must be visible independently of the accepted binding")
	application := decoded["application"].(map[string]any)
	require.Equal(t, "pending", application["stage"])
	require.EqualValues(t, 3, application["target_revision"])
	require.NotContains(t, application, "requested_by")
	require.Equal(t, "old-execution", view.Bound.ExecutionID)
}

func TestBuiltinApplicationViewAndIntentUseCurrentProviderSnapshot(t *testing.T) {
	ctx := context.Background()
	svc, manager := newBuiltinIntentResumeFixture(t, "retired", false)
	manager.isAgentRunning = true
	repo := svc.repo.(*sqliterepo.Repository)
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session")
	require.NoError(t, err)
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, TaskID: "task", WorkspaceID: "ws1", Surface: profile.Surface, TaskRevision: 3, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}}
	sealed, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	digest, err := p.BuiltinBindingDigest(profile)
	require.NoError(t, err)
	binding := p.ProviderMCPBinding{Scope: p.ProviderMCPScope{BindingID: "current", ExecutionID: running.AgentExecutionID, HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 1, ProfileDigest: digest}, Profile: sealed}
	running.Metadata[p.ProviderMCPBindingMetadataKey] = binding
	running.Metadata[p.ProviderMCPEffectsClosedMetadataKey] = false
	metadata, err := json.Marshal(running.Metadata)
	require.NoError(t, err)
	_, err = repo.DB().Exec(`UPDATE executors_running SET metadata=? WHERE session_id='session'`, string(metadata))
	require.NoError(t, err)
	raw, err := json.Marshal(binding)
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = repo.DB().Exec(`INSERT INTO agent_provider_mcp_bindings(binding_id,session_id,task_id,workspace_id,execution_id,operation_id,binding_json,catalog_json,target_revision,policy_revision,native_session_id,created_at,accepted_at) VALUES ('current','session','task','ws1','old-execution','operation',?,'{"kandev":["step_complete_kandev"]}',3,0,'native',?,?)`, string(raw), now, now)
	require.NoError(t, err)
	view, err := svc.ReadBuiltinSessionMCP(ctx, "session")
	require.NoError(t, err)
	data, err := json.Marshal(view)
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, json.Unmarshal(data, &response))
	require.Contains(t, response, "provider_binding", "the current provider must not inherit the initial generation's catalog")
	_, err = svc.PrepareBuiltinIntent(ctx, "task", "session", "walkthrough")
	require.Error(t, err, "the initial provider's Walkthrough grant was removed from the accepted generation")
}
