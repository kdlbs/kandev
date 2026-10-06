package sqlite_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// @covers AC-PLUGINS-MANAGED-COORDINATION-002.4, AC-PLUGINS-MANAGED-COORDINATION-002.5, AC-PLUGINS-MANAGED-COORDINATION-002.6, AC-PLUGINS-MANAGED-COORDINATION-002.8
func TestManagedConversationAdmissionPostgresWaits(t *testing.T) {
	for _, mode := range []string{"task", "session", "executor", "cancel", "identity", "replacement", "disposal"} {
		t.Run(mode, func(t *testing.T) {
			a, b, _ := newManagedAdmissionPostgresRepoPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			var workers sync.WaitGroup
			var holder *sql.Tx
			t.Cleanup(func() {
				cancel()
				if holder != nil {
					_ = holder.Rollback()
				}
				workers.Wait()
			})
			require.NoError(t, a.CreateWorkspace(ctx, &models.Workspace{ID: "managed-ws", Name: "Managed"}))
			firstSvc := taskservice.NewAgentConversationService(a, a, nil, nil, nil)
			secondSvc := taskservice.NewAgentConversationService(b, b, nil, nil, nil)
			spec := pluginsdk.ManagedAgentConversationSpec{WorkspaceID: "managed-ws", InstanceKey: "lead", AgentProfileID: "profile", BasePrompt: "original", ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
			first, _, err := firstSvc.EnsureManaged(ctx, "plugin", "installation", spec, "create", "create-digest")
			require.NoError(t, err)
			if mode == "executor" {
				require.NoError(t, a.UpsertExecutorRunning(ctx, &models.ExecutorRunning{TaskID: first.TaskID, SessionID: first.SessionID, Status: models.ExecutorRunningStatusRunning}))
			}
			pid := hierarchyBackendPID(t, b.DB())
			holder, err = a.DB().BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
			require.NoError(t, err)
			var isolation string
			require.NoError(t, holder.QueryRowContext(ctx, `SHOW transaction_isolation`).Scan(&isolation))
			require.Equal(t, "read committed", isolation)
			table, key, id := "tasks", "id", first.TaskID
			if mode == "session" || mode == "cancel" {
				table, key, id = "task_sessions", "id", first.SessionID
			}
			if mode == "executor" {
				table, key, id = "executors_running", "session_id", first.SessionID
			}
			_, err = holder.ExecContext(ctx, `SELECT id FROM `+table+` WHERE `+key+`=$1 FOR UPDATE`, id)
			require.NoError(t, err)
			spec.ExpectedRevision, spec.BasePrompt = first.Revision, "delayed"
			waiterCtx, cancelWaiter := context.WithCancel(ctx)
			defer cancelWaiter()
			done := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				_, _, err := secondSvc.EnsureManaged(waiterCtx, "plugin", "installation", spec, "delayed", "delayed-digest")
				done <- err
			}()
			waitHierarchyPostgresLocks(t, holder, pid, 1, "managed "+mode)
			t.Logf("%s: actual backend %d transaction row wait, isolation=%s", mode, pid, isolation)
			want := codes.FailedPrecondition
			switch mode {
			case "task":
				_, err = holder.ExecContext(ctx, `UPDATE tasks SET metadata=jsonb_set(jsonb_set(metadata::jsonb,'{"kandev.conversation_revision"}','"2"'),'{"kandev.base_prompt"}','"winner"')::text WHERE id=$1`, first.TaskID)
				want = codes.Aborted
			case "identity":
				_, err = holder.ExecContext(ctx, `UPDATE tasks SET metadata=jsonb_set(metadata::jsonb,'{"kandev.installation_id"}','"other-installation"')::text WHERE id=$1`, first.TaskID)
				want = codes.NotFound
			case "replacement":
				_, err = holder.ExecContext(ctx, `UPDATE task_sessions SET is_primary=0 WHERE id=$1`, first.SessionID)
				require.NoError(t, err)
				_, err = holder.ExecContext(ctx, `INSERT INTO task_sessions(id,task_id,is_primary,state,agent_profile_id,started_at,updated_at) VALUES ('replacement',$1,1,'CREATED','replacement-profile',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, first.TaskID)
				want = codes.OK
			case "disposal":
				_, err = holder.ExecContext(ctx, `UPDATE tasks SET metadata=jsonb_set(metadata::jsonb,'{"kandev.detached"}','true')::text WHERE id=$1`, first.TaskID)
				want = codes.NotFound
			case "session":
				_, err = holder.ExecContext(ctx, `UPDATE task_sessions SET state='RUNNING' WHERE id=$1`, first.SessionID)
			case "executor":
				_, err = holder.ExecContext(ctx, `UPDATE executors_running SET agent_execution_id='accepted-execution' WHERE session_id=$1`, first.SessionID)
			case "cancel":
				cancelWaiter()
			}
			require.NoError(t, err)
			if mode == "cancel" {
				err = <-done
				workers.Wait()
				require.ErrorIs(t, err, context.Canceled)
				require.NoError(t, holder.Commit())
			} else {
				require.NoError(t, holder.Commit())
				err = <-done
				workers.Wait()
				require.Equal(t, want, status.Code(err))
			}
			task, err := b.GetTask(ctx, first.TaskID)
			require.NoError(t, err)
			switch mode {
			case "replacement":
				require.Equal(t, spec.BasePrompt, task.Metadata["kandev.base_prompt"])
			case "task":
				require.Equal(t, "winner", task.Metadata["kandev.base_prompt"])
			default:
				require.Equal(t, "original", task.Metadata["kandev.base_prompt"])
			}
			if mode == "replacement" {
				replacement, err := b.GetPrimarySessionByTaskID(ctx, first.TaskID)
				require.NoError(t, err)
				require.Equal(t, "replacement", replacement.ID)
				require.Equal(t, spec.AgentProfileID, replacement.AgentProfileID)
			}
			primary, err := b.GetTaskSession(ctx, first.SessionID)
			require.NoError(t, err)
			if mode == "session" {
				require.Equal(t, models.TaskSessionStateRunning, primary.State)
			}
			if mode == "executor" {
				require.Equal(t, "accepted-execution", primary.AgentExecutionID)
			}
		})
	}
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-002.4, AC-PLUGINS-MANAGED-COORDINATION-002.6
func TestManagedConversationAdmissionPostgresRegistrationFence(t *testing.T) {
	a, b, third := newManagedAdmissionPostgresRepoPair(t)
	c := tasksqlite.NewWithInitializedDB(third, third, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	var workers sync.WaitGroup
	var holder *sql.Tx
	t.Cleanup(func() {
		cancel()
		if holder != nil {
			_ = holder.Rollback()
		}
		workers.Wait()
	})
	require.NoError(t, a.CreateWorkspace(ctx, &models.Workspace{ID: "fence-ws", Name: "Fence"}))
	svc := taskservice.NewAgentConversationService(a, a, nil, nil, nil)
	delayedSvc := taskservice.NewAgentConversationService(b, b, nil, nil, nil)
	spec := pluginsdk.ManagedAgentConversationSpec{WorkspaceID: "fence-ws", InstanceKey: "lead", AgentProfileID: "profile", BasePrompt: "original", ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	first, _, err := svc.EnsureManaged(ctx, "plugin", "installation", spec, "create", "digest")
	require.NoError(t, err)
	admissionPID, registrationPID := hierarchyBackendPID(t, b.DB()), hierarchyBackendPID(t, c.DB())
	holder, err = a.DB().BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = holder.ExecContext(ctx, `SELECT id FROM task_sessions WHERE id=$1 FOR UPDATE`, first.SessionID)
	require.NoError(t, err)
	spec.ExpectedRevision, spec.BasePrompt = first.Revision, "accepted settings"
	admissionDone, registrationDone := make(chan error, 1), make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, _, err := delayedSvc.EnsureManaged(ctx, "plugin", "installation", spec, "update", "update-digest")
		admissionDone <- err
	}()
	waitHierarchyPostgresLocks(t, holder, admissionPID, 1, "managed session admission")
	workers.Add(1)
	go func() {
		defer workers.Done()
		registrationDone <- c.UpsertExecutorRunning(ctx, &models.ExecutorRunning{TaskID: first.TaskID, SessionID: first.SessionID, AgentExecutionID: "registered-after-admission", Status: models.ExecutorRunningStatusRunning})
	}()
	waitHierarchyPostgresLocks(t, holder, registrationPID, 1, "absent executor registration task fence")
	t.Logf("admission PID %d waits session; absent-row canonical registration PID %d waits physical task", admissionPID, registrationPID)
	require.NoError(t, holder.Commit())
	require.NoError(t, <-admissionDone)
	require.NoError(t, <-registrationDone)
	workers.Wait()
	current, err := c.GetTaskSession(ctx, first.SessionID)
	require.NoError(t, err)
	require.Equal(t, "registered-after-admission", current.AgentExecutionID)
	task, err := c.GetTask(ctx, first.TaskID)
	require.NoError(t, err)
	require.Equal(t, spec.BasePrompt, task.Metadata["kandev.base_prompt"])
}

func newManagedAdmissionPostgresRepoPair(t *testing.T) (*tasksqlite.Repository, *tasksqlite.Repository, *sqlx.DB) {
	t.Helper()
	a, b, observer := newHierarchyPostgresRepoPair(t)
	database := sqlx.NewDb(a.DB(), "pgx")
	_, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	return a, b, observer
}
