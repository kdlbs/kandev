package sqlite

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestBuiltinGenerationAcceptanceCAS(t *testing.T) {
	for _, scenario := range []string{"current", "new-save", "foreign-source", "wrong-native", "not-resumed", "wider-than-saved", "wider-before-start", "partial-catalog", "excluded-catalog", "empty-tools", "duplicate-tools"} {
		t.Run(scenario, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			_, _, setupErr := settingsstore.Provide(repo.db, repo.db, nil)
			require.NoError(t, setupErr)
			ctx := context.Background()
			seedExecutorRunningCleanupTask(t, repo, "task")
			seedExecutorRunningCleanupSession(t, repo, "task", "session", models.TaskSessionStateWaitingForInput, "execution")
			task, err := repo.GetTask(ctx, "task")
			require.NoError(t, err)
			now := time.Now().UTC()
			base := p.New(p.SurfaceKanbanTask, nil, nil)
			base.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, TaskID: "task", WorkspaceID: task.WorkspaceID, Surface: base.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
			original, err := p.MarshalBuiltinBinding(base)
			require.NoError(t, err)
			metadata, err := json.Marshal(map[string]any{p.BuiltinBindingMetadataKey: original, p.ProviderMCPEffectsClosedMetadataKey: true})
			require.NoError(t, err)
			_, err = repo.db.Exec(`UPDATE executors_running SET status='running',resumable=TRUE,resume_token='native',metadata=? WHERE session_id='session'`, string(metadata))
			require.NoError(t, err)
			_, err = repo.db.Exec(`INSERT INTO task_builtin_mcp_preferences (task_id,workspace_id,schema_version,revision,packages_json,updated_by,updated_at) VALUES ('task',?,1,3,'{"configuration":"on","rich-output":"off","walkthroughs":"off","diagnostics":"off"}','human',?)`, task.WorkspaceID, now)
			require.NoError(t, err)
			_, err = repo.db.Exec(`INSERT INTO task_builtin_mcp_applications (session_id,task_id,workspace_id,target_revision,policy_revision,selection_hash,requested_by,request_id,operation_id,attempt_actor_id,attempt_selection_hash,attempt_revision,source_execution_id,attempt_source,stage,created_at,updated_at) VALUES ('session','task',?,3,0,'hash','human','request','operation','human','hash',3,'execution',?,'verifying',?,?)`, task.WorkspaceID, providerBindingSourceJSON(t), now, now)
			require.NoError(t, err)
			target := base
			target.BuiltinPackages = base.BuiltinPackages.Clone()
			target.BuiltinPackages.TaskRevision = 3
			target.BuiltinPackages.Packages = append(target.BuiltinPackages.Packages, p.PackageConfiguration)
			target.BuiltinPackages.Origins[p.PackageConfiguration] = "task"
			savedHash := mcpconfig.BuiltinSelectionHash(target.BuiltinPackages.Packages)
			_, err = repo.DB().Exec(`UPDATE task_builtin_mcp_applications SET selection_hash=?,attempt_selection_hash=? WHERE session_id='session'`, savedHash, savedHash)
			require.NoError(t, err)
			if scenario == "wider-than-saved" || scenario == "wider-before-start" {
				target.BuiltinPackages.Packages = append(target.BuiltinPackages.Packages, p.PackageDiagnostics)
				target.BuiltinPackages.Origins[p.PackageDiagnostics] = "task"
			}
			sealed, err := p.MarshalBuiltinBinding(target)
			require.NoError(t, err)
			digest, err := p.BuiltinBindingDigest(target)
			require.NoError(t, err)
			binding := p.ProviderMCPBinding{Scope: p.ProviderMCPScope{BindingID: "accepted", ExecutionID: "execution", HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 1, ProfileDigest: digest}, Profile: sealed}
			bindingJSON, err := json.Marshal(binding)
			require.NoError(t, err)
			_, err = repo.db.Exec(`UPDATE task_builtin_mcp_applications SET target_binding_id='accepted',target_binding_json=? WHERE session_id='session'`, string(bindingJSON))
			require.NoError(t, err)
			request := models.BuiltinMCPAcceptance{SessionID: "session", OperationID: "operation", RowVersion: 1, Binding: binding, Catalog: map[string][]string{"kandev": {"step_complete_kandev"}, "kandev-configuration": {"list_executors_kandev"}}, NativeSessionID: "native", NativeResumeConfirmed: true}
			switch scenario {
			case "new-save":
				_, err = repo.db.Exec(`UPDATE task_builtin_mcp_preferences SET revision=4 WHERE task_id='task'`)
				require.NoError(t, err)
			case "foreign-source":
				request.Binding.Scope.HelperID = "other"
			case "wrong-native":
				request.NativeSessionID = "other"
			case "empty-tools":
				request.Catalog["kandev-configuration"] = nil
			case "duplicate-tools":
				request.Catalog["kandev-configuration"] = []string{"list_executors_kandev", "list_executors_kandev"}
			case "partial-catalog":
				delete(request.Catalog, "kandev-configuration")
			case "excluded-catalog":
				request.Catalog["kandev-diagnostics"] = []string{"old_tool_kandev"}
			case "not-resumed":
				request.NativeResumeConfirmed = false
			}
			committer, ok := any(repo).(interface {
				CommitBuiltinProviderBinding(context.Context, models.BuiltinMCPAcceptance) (bool, error)
			})
			require.True(t, ok, "verified binding acceptance needs a saved-selection CAS")

			var committed bool
			if scenario == "wider-before-start" {
				_, err = repo.DB().Exec(`UPDATE task_builtin_mcp_applications SET stage='retiring' WHERE session_id='session'`)
				require.NoError(t, err)
				app, readErr := repo.GetBuiltinMCPApplication(ctx, "session")
				require.NoError(t, readErr)
				committed, err = repo.AdvanceBuiltinMCPApplication(ctx, app, "starting", "")
			} else {
				committed, err = committer.CommitBuiltinProviderBinding(ctx, request)
			}
			require.NoError(t, err)
			require.Equal(t, scenario == "current", committed)
			running, err := repo.GetExecutorRunningBySessionID(ctx, "session")
			require.NoError(t, err)
			require.Equal(t, original, running.Metadata[p.BuiltinBindingMetadataKey])
			require.Equal(t, "native", running.ResumeToken)
			if scenario == "current" {
				accepted, _, err := p.ParseProviderMCPBinding(running.Metadata[p.ProviderMCPBindingMetadataKey])
				require.NoError(t, err)
				require.Equal(t, binding, accepted)
				require.Equal(t, true, running.Metadata[p.ProviderMCPEffectsClosedMetadataKey], "helper acceptance must precede reopening effects")
				reader, ok := any(repo).(interface {
					GetAcceptedProviderMCPBinding(context.Context, string, string) (*models.ProviderMCPBindingSnapshot, error)
				})
				require.True(t, ok, "current provider evidence must come from the accepted SQL snapshot")
				before, err := reader.GetAcceptedProviderMCPBinding(ctx, "session", "execution")
				require.NoError(t, err)
				require.Nil(t, before, "SQL commitment alone must not report application")
				app, err := repo.GetBuiltinMCPApplication(ctx, "session")
				require.NoError(t, err)
				done, err := repo.AdvanceBuiltinMCPApplication(ctx, app, "applied", "")
				require.NoError(t, err)
				require.True(t, done)
				after, err := reader.GetAcceptedProviderMCPBinding(ctx, "session", "execution")
				require.NoError(t, err)
				require.NotNil(t, after)
				require.Equal(t, binding.Scope.BindingID, after.BindingID)
				require.NotNil(t, after.AcceptedAt)

			}
		})
	}
}

func providerBindingSourceJSON(t *testing.T) string {
	t.Helper()
	raw, err := json.Marshal(types.ProviderReplacementSource{ExecutionID: "execution", TaskID: "task", SessionID: "session", HelperID: "helper", ProcessGeneration: 1, ConfigurationRevision: 1, HarnessGeneration: 1, StreamID: "stream", IncarnationID: "incarnation", NativeSessionID: "native"})
	require.NoError(t, err)
	return string(raw)
}
