package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/task/models"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type managedPrimaryReadGate struct {
	*sqliterepo.Repository
	armed         atomic.Bool
	seen, release chan struct{}
}

func (g *managedPrimaryReadGate) GetPrimarySessionByTaskID(ctx context.Context, id string) (*models.TaskSession, error) {
	row, err := g.Repository.GetPrimarySessionByTaskID(ctx, id)
	if err == nil && g.armed.CompareAndSwap(true, false) {
		close(g.seen)
		select {
		case <-g.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return row, err
}

func managedAdmissionPair(t *testing.T) ([2]*AgentConversationService, [2]*sqliterepo.Repository, *managedPrimaryReadGate) {
	t.Helper()
	first, path := serviceTestSQLiteTemplate.Open(t)
	raw, err := internaldb.OpenSQLite(path)
	require.NoError(t, err)
	second := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	var services [2]*AgentConversationService
	var repos [2]*sqliterepo.Repository
	gate := &managedPrimaryReadGate{seen: make(chan struct{}), release: make(chan struct{})}
	for i, database := range []*sqlx.DB{first, second} {
		repos[i] = sqliterepo.NewWithInitializedDB(database, database, nil)
		store, err := state.NewStore(internaldb.NewPool(database, database))
		require.NoError(t, err)
		services[i] = NewAgentConversationService(repos[i], repos[i], nil, store, nil)
	}
	gate.Repository = repos[0]
	services[0].sess = gate
	seedConversationWorkspace(t, repos[0], "admission-workspace")
	return services, repos, gate
}

func managedAdmissionSpec() pluginsdk.ManagedAgentConversationSpec {
	return pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "admission-workspace", InstanceKey: "lead",
		AgentProfileID: "test-workspace-default-profile", ApprovalRevision: 1,
		ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BasePrompt:     "original instructions",
	}
}

type acManagedTestDeps struct {
	tasks      *acManagedTaskFixture
	sess       *acManagedSessionFixture
	dispatcher *acFakeDispatcher
}

type acManagedTaskFixture struct {
	*sqliterepo.Repository
	t *testing.T
}

func (f *acManagedTaskFixture) count() int {
	f.t.Helper()
	var count int
	require.NoError(f.t, f.DB().QueryRow(`SELECT count(*) FROM tasks`).Scan(&count))
	return count
}

type acManagedSessionFixture struct {
	*sqliterepo.Repository
	t *testing.T
}

func (f *acManagedSessionFixture) setState(id string, state models.TaskSessionState) {
	f.t.Helper()
	require.NoError(f.t, f.UpdateTaskSessionState(context.Background(), id, state, ""))
}

func (f *acManagedSessionFixture) setExecution(id string, state models.TaskSessionState, execution string) {
	f.t.Helper()
	ctx := context.Background()
	f.setState(id, state)
	if execution == "" {
		require.NoError(f.t, f.DeleteExecutorRunningBySessionID(ctx, id))
		return
	}
	session, err := f.GetTaskSession(ctx, id)
	require.NoError(f.t, err)
	require.NoError(f.t, f.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: id, TaskID: session.TaskID, AgentExecutionID: execution, Status: models.ExecutorRunningStatusRunning,
	}))
}

func newACManagedTestService(t *testing.T) (*AgentConversationService, acManagedTestDeps) {
	t.Helper()
	services, repos, _ := managedAdmissionPair(t)
	for _, workspace := range []string{"ws-1", "ws-2"} {
		seedConversationWorkspace(t, repos[1], workspace)
	}
	deps := acManagedTestDeps{
		tasks:      &acManagedTaskFixture{Repository: repos[1], t: t},
		sess:       &acManagedSessionFixture{Repository: repos[1], t: t},
		dispatcher: newACFakeDispatcher(),
	}
	services[1].SetDispatcher(deps.dispatcher)
	return services[1], deps
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-002.1, AC-PLUGINS-MANAGED-COORDINATION-002.4, AC-PLUGINS-MANAGED-COORDINATION-002.5
func TestManagedConversationAdmissionInterleavings(t *testing.T) {
	for _, mode := range []string{"launch_after_idle_read", "competing_revision_after_read", "runtime_attempt_and_execution", "already_running_control", "idle_update_control"} {
		t.Run(mode, func(t *testing.T) {
			services, repos, gate := managedAdmissionPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			var workers sync.WaitGroup
			var once sync.Once
			release := func() { once.Do(func() { close(gate.release) }) }
			t.Cleanup(func() { cancel(); release(); workers.Wait() })
			spec := managedAdmissionSpec()
			first, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "create", "create-digest")
			require.NoError(t, err)
			changed := spec
			changed.ExpectedRevision, changed.BasePrompt = first.Revision, "delayed instructions"
			if mode == "already_running_control" {
				require.NoError(t, repos[1].UpdateTaskSessionState(ctx, first.SessionID, models.TaskSessionStateRunning, ""))
				_, _, err = services[0].EnsureManaged(ctx, "plugin", "installation", changed, "update", "update-digest")
				require.Equal(t, codes.FailedPrecondition, status.Code(err))
				return
			}
			if mode == "idle_update_control" {
				current, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", changed, "update", "update-digest")
				require.NoError(t, err)
				require.Equal(t, first.Revision+1, current.Revision)
				require.Equal(t, changed.BasePrompt, current.BasePrompt)
				return
			}
			gate.armed.Store(true)
			done := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				_, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", changed, "delayed", "delayed-digest")
				done <- err
			}()
			select {
			case <-gate.seen:
			case <-ctx.Done():
				t.Fatal("primary read not reached")
			}
			wantCode, wantPrompt := codes.FailedPrecondition, spec.BasePrompt
			switch mode {
			case "competing_revision_after_read":
				winner := changed
				winner.BasePrompt = "accepted instructions"
				accepted, _, err := services[1].EnsureManaged(ctx, "plugin", "installation", winner, "winner", "winner-digest")
				require.NoError(t, err)
				require.Equal(t, first.Revision+1, accepted.Revision)
				readback, err := services[0].GetManaged(ctx, "installation", spec.WorkspaceID, spec.InstanceKey)
				require.NoError(t, err)
				require.Equal(t, winner.BasePrompt, readback.BasePrompt)
				wantCode, wantPrompt = codes.Aborted, winner.BasePrompt
			case "runtime_attempt_and_execution":
				current, err := repos[1].GetTaskSession(ctx, first.SessionID)
				require.NoError(t, err)
				current.State = models.TaskSessionStateStarting
				applied, err := repos[1].UpdateTaskSessionIfCurrentStateWithStartAttempt(ctx, current, models.TaskSessionStateCreated, "accepted-attempt")
				require.NoError(t, err)
				require.True(t, applied)
				require.NoError(t, repos[1].UpsertExecutorRunning(ctx, &models.ExecutorRunning{
					SessionID: first.SessionID, TaskID: first.TaskID, AgentExecutionID: "accepted-execution", Status: models.ExecutorRunningStatusStarting,
				}))
			default:
				require.NoError(t, repos[1].UpdateTaskSessionState(ctx, first.SessionID, models.TaskSessionStateRunning, ""))
				accepted, err := repos[0].GetTaskSession(ctx, first.SessionID)
				require.NoError(t, err)
				require.Equal(t, models.TaskSessionStateRunning, accepted.State)
			}
			release()
			select {
			case err = <-done:
			case <-ctx.Done():
				t.Fatal("delayed admission did not finish")
			}
			workers.Wait()
			assertManagedAdmissionWinner(t, ctx, repos[1], first, mode, wantPrompt)
			require.Equal(t, wantCode, status.Code(err))
		})
	}
}

func assertManagedAdmissionWinner(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, first pluginsdk.ManagedAgentConversationDescriptor, mode, prompt string) {
	t.Helper()
	stored, err := repo.GetTask(ctx, first.TaskID)
	require.NoError(t, err)
	require.Equal(t, prompt, stored.Metadata["kandev.base_prompt"])
	primary, err := repo.GetTaskSession(ctx, first.SessionID)
	require.NoError(t, err)
	switch mode {
	case "launch_after_idle_read":
		require.Equal(t, models.TaskSessionStateRunning, primary.State)
	case "runtime_attempt_and_execution":
		require.Equal(t, models.TaskSessionStateStarting, primary.State)
		require.Equal(t, "accepted-attempt", primary.Metadata[models.SessionMetaKeyAgentStartAttemptID])
		require.Equal(t, "accepted-execution", primary.AgentExecutionID)
	case "competing_revision_after_read":
		require.Equal(t, "winner", stored.Metadata[metaKeyManagedOperation])
	}
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-002.6, AC-PLUGINS-MANAGED-COORDINATION-002.8
func TestManagedConversationAdmissionRollback(t *testing.T) {
	for _, mode := range []string{"create", "update", "repair"} {
		t.Run(mode, func(t *testing.T) {
			services, repos, _ := managedAdmissionPair(t)
			ctx := context.Background()
			spec := managedAdmissionSpec()
			var before *models.TaskSession
			if mode == "create" {
				_, err := repos[1].DB().ExecContext(ctx, `CREATE TRIGGER fail_managed_session BEFORE INSERT ON task_sessions BEGIN SELECT RAISE(ABORT, 'session failure'); END`)
				require.NoError(t, err)
			} else {
				first, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "create", "create-digest")
				require.NoError(t, err)
				before, err = repos[1].GetTaskSession(ctx, first.SessionID)
				require.NoError(t, err)
				if mode == "repair" {
					require.NoError(t, repos[1].DeleteTaskSession(ctx, before))
				}
				spec.ExpectedRevision = first.Revision
				spec.AgentProfileID, spec.BasePrompt = "replacement-profile", "replacement instructions"
				_, err = repos[1].DB().ExecContext(ctx, `CREATE TRIGGER fail_managed_task BEFORE UPDATE OF metadata ON tasks BEGIN SELECT RAISE(ABORT, 'task failure'); END`)
				require.NoError(t, err)
			}
			_, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "failed", "failed-digest")
			require.Error(t, err)
			var tasks, sessions int
			require.NoError(t, repos[1].DB().QueryRowContext(ctx, `SELECT count(*) FROM tasks`).Scan(&tasks))
			require.NoError(t, repos[1].DB().QueryRowContext(ctx, `SELECT count(*) FROM task_sessions`).Scan(&sessions))
			switch mode {
			case "create":
				require.Zero(t, tasks)
				require.Zero(t, sessions)
			case "repair":
				require.Equal(t, 1, tasks)
				require.Zero(t, sessions)
			case "update":
				current, err := repos[1].GetTaskSession(ctx, before.ID)
				require.NoError(t, err)
				require.Equal(t, before, current)
			}
		})
	}
}

// Legacy-only fixtures have no native managed admission authority.
func (unsupportedTaskFieldUpdater) EnsureManagedConversation(context.Context, managed.EnsureRequest) (managed.Result, error) {
	return managed.Result{}, managed.ErrUnavailable
}
func (unsupportedTaskFieldUpdater) ChangeManagedConversationState(context.Context, managed.StateRequest) (managed.Result, error) {
	return managed.Result{}, managed.ErrUnavailable
}
