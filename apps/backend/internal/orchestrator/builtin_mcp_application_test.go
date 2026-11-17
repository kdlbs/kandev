package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type applicationControl struct {
	source              types.ProviderReplacementSource
	ready, closed, lost bool
	unsupported         bool
	commands            []string
	statuses            map[string]types.ProviderReplacementStatus
	binding             p.ProviderMCPBinding
	afterCommand        func(types.ProviderReplacementRequest)
	failAction          string
	lostAction          string
}

func (c *applicationControl) Status(_ context.Context, id string) (types.ProviderReplacementStatus, error) {
	if id == "" {
		if c.unsupported {
			return types.ProviderReplacementStatus{Phase: "idle", Source: c.source}, nil
		}
		return types.ProviderReplacementStatus{Phase: "idle", Source: c.source, Ready: c.ready, Capability: types.ProviderMCPReplacementCapability}, nil
	}
	if s, ok := c.statuses[id]; ok {
		return s, nil
	}
	return types.ProviderReplacementStatus{Phase: "missing", OperationID: id, Source: c.source}, nil
}
func (c *applicationControl) Command(_ context.Context, r types.ProviderReplacementRequest) (types.ProviderReplacementStatus, error) {
	c.commands = append(c.commands, r.Action)
	s := c.statuses[r.OperationID]
	s.OperationID = r.OperationID
	s.Source = r.Source
	s.Version++
	switch r.Action {
	case "abandon":
		s.Phase = "failed"
		s.FailureCode = "target_superseded"
	case "prepare":
		s.Phase = "prepared"
	case "retire":
		s.Phase = "retired"
		s.SourceTerminated = true
	case "activate":
		s.Phase = "started"
		scope := r.Scope
		s.Scope = &scope
		s.NativeResumeConfirmed = true
		s.Catalog = map[string][]string{}
		for _, id := range r.Profile.BuiltinPackages.Packages {
			name := "kandev-" + string(id)
			if id == p.PackageCore {
				name = "kandev"
			}
			s.Catalog[name] = []string{"fixture_" + string(id) + "_kandev"}
		}
	case "accept":
		s.Phase = "accepted"
	case "release":
		s.Phase = "released"
	}
	if r.Action == c.failAction {
		s.Phase = "failed"
		s.FailureCode = "provider_start_failed"
	}
	c.statuses[r.OperationID] = s
	if c.afterCommand != nil {
		c.afterCommand(r)
	}
	if c.lost && (r.Action == c.lostAction || c.lostAction == "" && r.Action == "activate") {
		c.lost = false
		return types.ProviderReplacementStatus{}, errors.New("response lost")
	}
	return s, nil
}
func (c *applicationControl) CloseEffects() error                       { c.closed = true; return nil }
func (c *applicationControl) OpenEffects()                              { c.closed = false }
func (c *applicationControl) AdoptBinding(b p.ProviderMCPBinding) error { c.binding = b; return nil }

type applicationManager struct {
	*builtinIntentProbeManager
	control *applicationControl
}

func (m *applicationManager) WithProviderMCPReplacement(ctx context.Context, _, _ string, fn func(agentruntime.ProviderMCPControl) error) error {
	return fn(m.control)
}

func TestBuiltinApplicationCoordinatorPreservesIdentityAndNeverPrompts(t *testing.T) {
	for _, scenario := range []string{"idle", "running", "busy", "lost-activation", "unsupported"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			svc, base := newBuiltinIntentResumeFixture(t, "retired", false)
			repo := svc.repo.(*sqliterepo.Repository)
			settings, _, err := settingsstore.Provide(sqlx.NewDb(repo.DB(), "sqlite3"), sqlx.NewDb(repo.DB(), "sqlite3"), logger.Default())
			require.NoError(t, err)
			var policyRepo mcpconfig.BuiltinPolicyRepository = settings
			svc.SetBuiltinPolicyReader(policyRepo)
			_, err = repo.DB().Exec(`UPDATE executors_running SET resumable=TRUE,resume_token='native' WHERE session_id='session'`)
			require.NoError(t, err)
			if scenario == "running" {
				session, err := repo.GetTaskSession(ctx, "session")
				require.NoError(t, err)
				session.State = models.TaskSessionStateRunning
				require.NoError(t, repo.UpdateTaskSession(ctx, session))
			}
			_, err = policyRepo.PutBuiltinTaskPreferences(ctx, &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "ws1", TaskID: "task", UpdatedBy: "human", UpdatedAt: time.Now().UTC(), Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}, 0, 0, p.SurfaceKanbanTask)
			require.NoError(t, err)
			control := &applicationControl{source: types.ProviderReplacementSource{ExecutionID: "old-execution", TaskID: "task", SessionID: "session", HelperID: "helper", ProcessGeneration: 1, ConfigurationRevision: 1, HarnessGeneration: 1, StreamID: "stream", IncarnationID: "incarnation", NativeSessionID: "native"}, ready: scenario != "busy" && scenario != "unsupported", unsupported: scenario == "unsupported", lost: scenario == "lost-activation", statuses: map[string]types.ProviderReplacementStatus{}}
			runningBefore, err := repo.GetExecutorRunningBySessionID(ctx, "session")
			require.NoError(t, err)
			initialBefore, err := p.ParseBuiltinBindingMetadata(runningBefore.Metadata[p.BuiltinBindingMetadataKey])
			require.NoError(t, err)
			control.source.ProfileDigest, err = p.BuiltinBindingDigest(*initialBefore)
			require.NoError(t, err)
			svc.agentManager = &applicationManager{builtinIntentProbeManager: base, control: control}
			coordinator, ok := any(svc).(interface {
				SetBuiltinApplicationAuthorizer(mcpconfig.BuiltinPolicyAuthorizer)
				ReconcileBuiltinMCPApplications(context.Context) error
			})
			require.True(t, ok, "saved settings need an automatic, recoverable provider coordinator")
			coordinator.SetBuiltinApplicationAuthorizer(func(context.Context, string, authz.Scope) error { return nil })
			require.NoError(t, coordinator.ReconcileBuiltinMCPApplications(ctx))
			require.NoError(t, coordinator.ReconcileBuiltinMCPApplications(ctx))
			app, err := repo.GetBuiltinMCPApplication(ctx, "session")
			require.NoError(t, err)
			if scenario == "running" || scenario == "busy" || scenario == "unsupported" {
				require.Equal(t, "pending", app.Stage)
				require.Empty(t, control.commands)
				if scenario == "unsupported" {
					require.Equal(t, "unsupported_resume", app.Reason)
				}
				if scenario == "busy" {
					require.Equal(t, "waiting_for_safe_boundary", app.Reason)
				}
				return
			}
			require.Equal(t, "applied", app.Stage)
			require.Equal(t, []string{"prepare", "retire", "activate", "accept"}, control.commands)
			require.False(t, control.closed)
			running, err := repo.GetExecutorRunningBySessionID(ctx, "session")
			require.NoError(t, err)
			require.Equal(t, "old-execution", running.AgentExecutionID)
			require.Equal(t, "native", running.ResumeToken)
			require.NotEmpty(t, control.binding.Scope.BindingID)
			require.Empty(t, base.capturedPrompts, "provider refresh must not dispatch a turn")
		})
	}
}
