package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func newApplicationCoordinatorFixture(t *testing.T) (*Service, *sqliterepo.Repository, mcpconfig.BuiltinPolicyRepository, *applicationControl) {
	t.Helper()
	svc, base := newBuiltinIntentResumeFixture(t, "retired", false)
	repo := svc.repo.(*sqliterepo.Repository)
	settings, _, err := settingsstore.Provide(sqlx.NewDb(repo.DB(), "sqlite3"), sqlx.NewDb(repo.DB(), "sqlite3"), logger.Default())
	require.NoError(t, err)
	var policy mcpconfig.BuiltinPolicyRepository = settings
	svc.SetBuiltinPolicyReader(policy)
	_, err = repo.DB().Exec(`UPDATE executors_running SET resumable=TRUE,resume_token='native' WHERE session_id='session'`)
	require.NoError(t, err)
	saveApplicationSelection(t, policy, 0, "on")
	control := &applicationControl{source: types.ProviderReplacementSource{ExecutionID: "old-execution", TaskID: "task", SessionID: "session", HelperID: "helper", ProcessGeneration: 1, ConfigurationRevision: 1, HarnessGeneration: 1, StreamID: "stream", IncarnationID: "incarnation", NativeSessionID: "native"}, ready: true, statuses: map[string]types.ProviderReplacementStatus{}}
	runningBefore, err := repo.GetExecutorRunningBySessionID(context.Background(), "session")
	require.NoError(t, err)
	initialBefore, err := p.ParseBuiltinBindingMetadata(runningBefore.Metadata[p.BuiltinBindingMetadataKey])
	require.NoError(t, err)
	control.source.ProfileDigest, err = p.BuiltinBindingDigest(*initialBefore)
	require.NoError(t, err)
	svc.agentManager = &applicationManager{builtinIntentProbeManager: base, control: control}
	svc.SetBuiltinApplicationAuthorizer(func(context.Context, string, authz.Scope) error { return nil })
	svc.messageQueue = messagequeue.NewServiceMemory(logger.Default())
	svc.messageQueue.SetMergeEnabled(false)
	require.NoError(t, svc.messageQueue.SetAutoRun(context.Background(), "session", false))
	return svc, repo, policy, control
}
func saveApplicationSelection(t *testing.T, policy mcpconfig.BuiltinPolicyRepository, version int64, choice string) {
	t.Helper()
	_, err := policy.PutBuiltinTaskPreferences(context.Background(), &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "ws1", TaskID: "task", UpdatedBy: "human", UpdatedAt: time.Now().UTC(), Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: choice}}, version, 0, p.SurfaceKanbanTask)
	require.NoError(t, err)
}

func TestBuiltinApplicationConcurrentSaveAppliesOnlyLatestSelection(t *testing.T) {
	for _, boundary := range []string{"prepare", "retire", "activate", "accept"} {
		t.Run(boundary, func(t *testing.T) {
			svc, repo, policy, control := newApplicationCoordinatorFixture(t)
			changed := false
			control.afterCommand = func(r types.ProviderReplacementRequest) {
				if r.Action == boundary && !changed {
					changed = true
					saveApplicationSelection(t, policy, 1, "off")
				}
				if r.Action == "accept" {
					control.source.ProcessGeneration = r.Scope.ProcessGeneration
					control.source.BindingID = r.Scope.BindingID
					control.source.ProfileDigest = r.Scope.ProfileDigest
				}
			}
			for range 5 {
				require.NoError(t, svc.ReconcileBuiltinMCPApplications(context.Background()))
			}
			require.True(t, changed)
			app, err := repo.GetBuiltinMCPApplication(context.Background(), "session")
			require.NoError(t, err)
			require.Equal(t, "applied", app.Stage)
			_, profile, err := p.ParseProviderMCPBinding(control.binding)
			require.NoError(t, err)
			require.NotContains(t, profile.BuiltinPackages.Packages, p.PackageConfiguration)
			require.EqualValues(t, 2, profile.BuiltinPackages.TaskRevision)
			starts := 0
			for _, command := range control.commands {
				if command == "activate" {
					starts++
				}
			}
			require.LessOrEqual(t, starts, 2, "one saved successor may require one additional replacement")
			before := len(control.commands)
			require.NoError(t, svc.ReconcileBuiltinMCPApplications(context.Background()))
			require.Len(t, control.commands, before, "settled settings must not restart in a loop")
		})
	}
}

func TestBuiltinApplicationInputArrivesDuringActivationAndRemainsInOrder(t *testing.T) {
	svc, repo, _, control := newApplicationCoordinatorFixture(t)
	var ids []string
	control.afterCommand = func(r types.ProviderReplacementRequest) {
		if r.Action != "activate" {
			return
		}
		blocked, err := svc.BuiltinMCPDispatchBlocked(context.Background(), "session")
		require.NoError(t, err)
		require.True(t, blocked)
		for _, content := range []string{"first input", "second input"} {
			entry, err := svc.messageQueue.QueueMessage(context.Background(), "session", "task", content, "", "human", false, nil)
			require.NoError(t, err)
			ids = append(ids, entry.ID)
		}
		status := svc.messageQueue.GetStatus(context.Background(), "session")
		require.NotEmpty(t, status.Entries)
		require.Contains(t, status.Entries[0].Content, "first input")
	}
	require.NoError(t, svc.ReconcileBuiltinMCPApplications(context.Background()))
	app, err := repo.GetBuiltinMCPApplication(context.Background(), "session")
	require.NoError(t, err)
	require.Equal(t, "applied", app.Stage)
	status := svc.messageQueue.GetStatus(context.Background(), "session")
	raw, err := json.Marshal(status.Entries)
	require.NoError(t, err)
	require.Contains(t, string(raw), "first input")
	require.Contains(t, string(raw), "second input")
	require.Len(t, ids, 2)
	if len(status.Entries) == 2 {
		require.Equal(t, ids[0], status.Entries[0].ID)
		require.Equal(t, ids[1], status.Entries[1].ID)
	}
}

func TestBuiltinApplicationLostResponsesReconcileWithoutDuplicateStart(t *testing.T) {
	for _, boundary := range []string{"prepare", "retire", "activate", "accept"} {
		t.Run(boundary, func(t *testing.T) {
			svc, repo, _, control := newApplicationCoordinatorFixture(t)
			control.lost = true
			control.lostAction = boundary
			for range 4 {
				require.NoError(t, svc.ReconcileBuiltinMCPApplications(context.Background()))
			}
			app, err := repo.GetBuiltinMCPApplication(context.Background(), "session")
			require.NoError(t, err)
			require.Equal(t, "applied", app.Stage)
			starts := 0
			for _, action := range control.commands {
				if action == "activate" {
					starts++
				}
			}
			require.Equal(t, 1, starts, "a lost control response cannot create another provider")
		})
	}
}

func TestBuiltinApplicationFailureNeedsOneExplicitRetry(t *testing.T) {
	svc, repo, _, control := newApplicationCoordinatorFixture(t)
	control.failAction = "activate"
	ctx := context.Background()
	require.NoError(t, svc.ReconcileBuiltinMCPApplications(ctx))
	failed, err := repo.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, "failed", failed.Stage)
	before := len(control.commands)
	for range 3 {
		require.NoError(t, svc.ReconcileBuiltinMCPApplications(ctx))
	}
	require.Len(t, control.commands, before, "failed replacement cannot retry automatically")
	target := models.BuiltinMCPRetryTarget{TaskRevision: failed.TargetRevision, PolicyRevision: failed.PolicyRevision, SelectionHash: failed.SelectionHash}
	accepted, err := repo.RetryBuiltinMCPApplication(ctx, "session", "human", failed.RequestID, failed.RowVersion, target)
	require.NoError(t, err)
	require.True(t, accepted)
	replay, err := repo.RetryBuiltinMCPApplication(ctx, "session", "human", failed.RequestID, failed.RowVersion, target)
	require.NoError(t, err)
	require.False(t, replay)
	control.failAction = ""
	require.NoError(t, svc.ReconcileBuiltinMCPApplications(ctx))
	app, err := repo.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, "applied", app.Stage)
}

func TestBuiltinApplicationPendingQuestionDefersWithoutProviderControl(t *testing.T) {
	svc, repo, _, control := newApplicationCoordinatorFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "question-turn", TaskID: "task", TaskSessionID: "session", StartedAt: now, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: "pending-permission", AuthorType: models.MessageAuthorAgent, TaskID: "task", TaskSessionID: "session", TurnID: "question-turn", Type: models.MessageTypePermissionRequest, Content: "Permission required", Metadata: map[string]interface{}{"pending_id": "permission", "tool_call_id": "tool", "options": []interface{}{map[string]interface{}{"option_id": "allow", "name": "Allow", "kind": "allow_once"}}}, CreatedAt: now}))
	require.NoError(t, svc.ReconcileBuiltinMCPApplications(ctx))
	app, err := repo.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, "pending", app.Stage)
	require.Empty(t, control.commands)
}

func TestBuiltinApplicationRevertBeforeReplacementDoesNotRestartProvider(t *testing.T) {
	svc, repo, policy, control := newApplicationCoordinatorFixture(t)
	// Match the saved source manifest before enabling and reverting Configuration.
	running, err := repo.GetExecutorRunningBySessionID(context.Background(), "session")
	require.NoError(t, err)
	profile, err := svc.executor.PrepareTaskSessionMCPProfile(context.Background(), "task", &models.TaskSession{ID: "session", TaskID: "task"}, false)
	require.NoError(t, err)
	profile.BuiltinPackages.Packages = []p.PackageID{p.PackageCore, p.PackageRichOutput, p.PackageWalkthroughs, p.PackageDiagnostics}
	delete(profile.BuiltinPackages.Origins, p.PackageConfiguration)
	sealed, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	control.source.ProfileDigest, err = p.BuiltinBindingDigest(profile)
	require.NoError(t, err)
	running.Metadata[p.BuiltinBindingMetadataKey] = sealed
	metadata, err := json.Marshal(running.Metadata)
	require.NoError(t, err)
	_, err = repo.DB().Exec(`UPDATE executors_running SET metadata=? WHERE session_id='session'`, string(metadata))
	require.NoError(t, err)
	saveApplicationSelection(t, policy, 1, "off")
	require.NoError(t, svc.ReconcileBuiltinMCPApplications(context.Background()))
	app, err := repo.GetBuiltinMCPApplication(context.Background(), "session")
	require.NoError(t, err)
	require.Equal(t, "superseded", app.Stage)
	require.Equal(t, "already_current", app.Reason)
	require.Empty(t, control.commands, "reverting to accepted tools must not replace the provider")
}
