package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	internaldb "github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/task/models"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/testutil"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

var deletionNativeSQLiteTemplate = testutil.NewSQLiteTemplate(func(database *sqlx.DB) error {
	if _, err := tasksqlite.NewWithDB(database, database, nil); err != nil {
		return err
	}
	if _, _, err := settingsstore.Provide(database, database, nil); err != nil {
		return err
	}
	if _, err := officesqlite.NewWithDB(database, database, nil); err != nil {
		return err
	}
	if _, err := workflowrepo.NewWithDB(database, database, nil); err != nil {
		return err
	}
	_, err := messagequeue.NewSQLiteRepository(database, database)
	return err
})

func newManagedDeletionSQLitePair(t *testing.T) (*tasksqlite.Repository, *tasksqlite.Repository, *sqlx.DB) {
	t.Helper()
	first, path := deletionNativeSQLiteTemplate.Open(t)
	open := func(read bool) *sqlx.DB {
		var raw *sql.DB
		var err error
		if read {
			raw, err = internaldb.OpenSQLiteReader(path)
		} else {
			raw, err = internaldb.OpenSQLite(path)
		}
		require.NoError(t, err)
		database := sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { require.NoError(t, database.Close()) })
		require.NoError(t, database.Ping())
		return database
	}
	second, observer := open(false), open(false)
	firstReader, secondReader := open(true), open(true)
	return tasksqlite.NewWithInitializedDB(first, firstReader, nil), tasksqlite.NewWithInitializedDB(second, secondReader, nil), observer
}

func seedNativeManagedDeletion(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, driver string) (*taskservice.AgentConversationService, managed.DeleteRequest, string) {
	t.Helper()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "delete-ws", Name: "Deletion"}))
	database := sqlx.NewDb(repo.DB(), driver)
	store, err := state.NewStore(internaldb.NewPool(database, database))
	require.NoError(t, err)
	svc := taskservice.NewAgentConversationService(repo, repo, nil, store, nil)
	spec := pluginsdk.ManagedAgentConversationSpec{WorkspaceID: "delete-ws", InstanceKey: "lead", AgentProfileID: "profile", BasePrompt: "original", ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	first, _, err := svc.EnsureManaged(ctx, "plugin", "installation", spec, "create", "create-digest")
	require.NoError(t, err)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "retained-turn", TaskID: first.TaskID, TaskSessionID: first.SessionID}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: "retained-message", TaskID: first.TaskID, TaskSessionID: first.SessionID, TurnID: "retained-turn", AuthorType: models.MessageAuthorUser, Content: "retained transcript"}))
	task, err := repo.GetTask(ctx, first.TaskID)
	require.NoError(t, err)
	request := managed.DeleteRequest{Identity: managed.Identity{TaskID: task.ID, InstallationID: "installation", WorkspaceID: "delete-ws", InstanceKey: "lead"}, ExpectedRevision: first.Revision, TaskCreatedAt: task.CreatedAt, OperationID: "delete", PayloadDigest: "delete-digest"}
	return svc, request, first.SessionID
}

func nativeDeletionInventory(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, request managed.DeleteRequest) string {
	t.Helper()
	sessions, err := repo.ListTaskSessions(ctx, request.TaskID)
	require.NoError(t, err)
	snapshot, err := json.Marshal(struct {
		WorkspaceID string                `json:"workspace_id"`
		Sessions    []*models.TaskSession `json:"sessions"`
	}{request.WorkspaceID, sessions})
	require.NoError(t, err)
	return string(snapshot)
}

func assertNativeDeletionRows(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, request managed.DeleteRequest, sessionID string) {
	t.Helper()
	_, err := repo.GetTask(ctx, request.TaskID)
	require.NoError(t, err)
	primary, err := repo.GetPrimarySessionByTaskID(ctx, request.TaskID)
	require.NoError(t, err)
	require.Equal(t, sessionID, primary.ID)
	messages, err := repo.ListMessages(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "retained transcript", messages[0].Content)
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.1, AC-PLUGINS-MANAGED-COORDINATION-013.2, AC-PLUGINS-MANAGED-COORDINATION-013.3, AC-PLUGINS-MANAGED-COORDINATION-013.5, AC-PLUGINS-MANAGED-COORDINATION-013.7, AC-PLUGINS-MANAGED-COORDINATION-013.8
func TestManagedDeletionAdmissionNativeConformance(t *testing.T) {
	for _, mode := range []string{"owner_fence", "settings_replay", "pause", "detach", "invalidate", "full_task", "session_create", "session_full", "session_start", "guarded_start", "runtime", "primary", "child_create", "ordinary_delete", "cleanup_create", "cleanup_mutators", "incomplete_inventory", "cancelled_reuse", "final_rollback", "missing", "replacement", "hierarchy", "legacy_snapshot"} {
		t.Run(mode, func(t *testing.T) {
			a, b, _ := newManagedDeletionSQLitePair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			svc, request, sessionID := seedNativeManagedDeletion(t, ctx, a, "sqlite3")
			if mode == "hierarchy" {
				require.NoError(t, b.CreateTask(ctx, &models.Task{ID: "child", WorkspaceID: request.WorkspaceID, ParentID: request.TaskID, Title: "Child"}))
				_, err := a.AdmitManagedDeletion(ctx, request, "owner")
				require.ErrorIs(t, err, repoerrors.ErrTaskHierarchyConflict)
				assertNativeDeletionRows(t, ctx, b, request, sessionID)
				return
			}
			claim, err := a.AdmitManagedDeletion(ctx, request, "owner")
			require.NoError(t, err)
			if mode == "legacy_snapshot" {
				released, err := a.ReleaseManagedDeletion(ctx, *claim)
				require.NoError(t, err)
				require.True(t, released)
				task, err := b.GetTask(ctx, request.TaskID)
				require.NoError(t, err)
				task.Metadata = map[string]interface{}{"incoming": "replacement"}
				require.NoError(t, b.UpdateTask(ctx, task))
				stored, err := a.GetTask(ctx, request.TaskID)
				require.NoError(t, err)
				require.Equal(t, task.Metadata, stored.Metadata)
				return
			}
			switch mode {
			case "owner_fence":
				foreign := *claim
				foreign.Owner = "foreign"
				_, err = b.PrepareManagedDeletion(ctx, foreign, nativeDeletionInventory(t, ctx, b, request))
				require.ErrorIs(t, err, managed.ErrDeletionOwned)
				_, err = b.ReleaseManagedDeletion(ctx, foreign)
				require.ErrorIs(t, err, managed.ErrDeletionOwned)
				_, err = b.AdmitManagedDeletion(ctx, request, "foreign")
				require.ErrorIs(t, err, managed.ErrDeletionOwned)
			case "settings_replay":
				_, _, err = svc.EnsureManaged(ctx, "plugin", "installation", pluginsdk.ManagedAgentConversationSpec{WorkspaceID: request.WorkspaceID, InstanceKey: request.InstanceKey, ExpectedRevision: 1, AgentProfileID: "profile", BasePrompt: "original", ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, "create", "create-digest")
				require.Error(t, err)
			case "pause", "detach", "invalidate":
				kind := managed.PauseExact
				if mode == "detach" {
					kind = managed.Detach
				}
				if mode == "invalidate" {
					kind = managed.Invalidate
				}
				_, err = b.ChangeManagedConversationState(ctx, managed.StateRequest{Identity: request.Identity, Kind: kind, Paused: true, ExpectedRevision: 1, OperationID: "state", PayloadDigest: "state-digest"})
				require.ErrorIs(t, err, managed.ErrBusy)
			case "full_task":
				task, err := b.GetTask(ctx, request.TaskID)
				require.NoError(t, err)
				task.Title, task.Metadata = "foreign", nil
				require.ErrorIs(t, b.UpdateTask(ctx, task), repoerrors.ErrTaskCleanupInProgress)
			case "session_create":
				err = b.CreateTaskSession(ctx, &models.TaskSession{ID: "foreign", TaskID: request.TaskID, State: models.TaskSessionStateCreated})
				require.ErrorIs(t, err, repoerrors.ErrTaskCleanupInProgress)
			case "session_full":
				session, err := b.GetTaskSession(ctx, sessionID)
				require.NoError(t, err)
				session.State = models.TaskSessionStateStarting
				require.ErrorIs(t, b.UpdateTaskSession(ctx, session), repoerrors.ErrTaskCleanupInProgress)
			case "session_start":
				require.ErrorIs(t, b.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateRunning, ""), repoerrors.ErrTaskCleanupInProgress)
			case "guarded_start":
				session, err := b.GetTaskSession(ctx, sessionID)
				require.NoError(t, err)
				session.State = models.TaskSessionStateStarting
				_, err = b.UpdateTaskSessionIfCurrentStateWithStartAttempt(ctx, session, models.TaskSessionStateCreated, "attempt")
				require.ErrorIs(t, err, repoerrors.ErrTaskCleanupInProgress)
			case "runtime":
				require.ErrorIs(t, b.UpsertExecutorRunning(ctx, &models.ExecutorRunning{SessionID: sessionID, TaskID: request.TaskID, AgentExecutionID: "new-execution"}), repoerrors.ErrTaskCleanupInProgress)
			case "primary":
				require.ErrorIs(t, b.SetSessionPrimary(ctx, sessionID), repoerrors.ErrTaskCleanupInProgress)
			case "child_create":
				require.ErrorIs(t, b.CreateTask(ctx, &models.Task{ID: "child", WorkspaceID: request.WorkspaceID, ParentID: request.TaskID, Title: "Child"}), repoerrors.ErrTaskCleanupInProgress)
			case "ordinary_delete":
				require.ErrorIs(t, b.DeleteTask(ctx, request.TaskID), repoerrors.ErrTaskCleanupInProgress)
			case "cleanup_create":
				require.ErrorIs(t, b.CreateTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{OperationID: "foreign", TaskID: request.TaskID, ResourceSnapshot: `{}`}), repoerrors.ErrTaskCleanupInProgress)
			case "cleanup_mutators":
				changed, err := b.CancelTaskResourceCleanupJobIfPending(ctx, claim.JobID)
				require.NoError(t, err)
				require.False(t, changed)
				require.ErrorIs(t, b.CompleteTaskResourceCleanupJob(ctx, claim.JobID, models.TaskResourceCleanupStateCancelled, "", nil), managed.ErrDeletionOwned)
				require.Error(t, b.UpdateTaskResourceCleanupSnapshot(ctx, managed.DeleteOperationID(request.OperationID), `{}`))
				started, err := b.StartPreparedTaskResourceCleanupJob(ctx, claim.JobID)
				require.ErrorIs(t, err, managed.ErrDeletionOwned)
				require.False(t, started)
			case "incomplete_inventory":
				_, err = b.PrepareManagedDeletion(ctx, *claim, `{}`)
				require.Error(t, err)
				_, err = b.FinalizeManagedDeletion(ctx, *claim)
				require.ErrorIs(t, err, managed.ErrDeletionOwned)
			case "cancelled_reuse":
				released, err := a.ReleaseManagedDeletion(ctx, *claim)
				require.NoError(t, err)
				require.True(t, released)
				newer, err := b.AdmitManagedDeletion(ctx, request, "new-owner")
				require.NoError(t, err)
				_, err = a.ReleaseManagedDeletion(ctx, *claim)
				require.ErrorIs(t, err, managed.ErrDeletionOwned)
				claim = newer
			case "final_rollback", "missing", "replacement":
				_, err = a.PrepareManagedDeletion(ctx, *claim, nativeDeletionInventory(t, ctx, a, request))
				require.NoError(t, err)
				switch mode {
				case "final_rollback":
					_, err = a.DB().ExecContext(ctx, `CREATE TRIGGER fail_delete_final BEFORE DELETE ON tasks BEGIN SELECT RAISE(ABORT, 'purge rollback'); END`)
				case "missing":
					_, err = a.DB().ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, request.TaskID)
				default:
					_, err = a.DB().ExecContext(ctx, `UPDATE tasks SET created_at = ? WHERE id = ?`, request.TaskCreatedAt.Add(time.Second), request.TaskID)
				}
				require.NoError(t, err)
				_, err = b.FinalizeManagedDeletion(ctx, *claim)
				require.Error(t, err)
				current, job, err := a.InspectManagedDeletion(ctx, request)
				require.NoError(t, err)
				require.Equal(t, managed.DeletePrepared, current.Phase)
				require.Equal(t, models.TaskResourceCleanupStatePrepared, job.State)
				started, err := b.StartPreparedTaskResourceCleanupJob(ctx, claim.JobID)
				require.ErrorIs(t, err, managed.ErrDeletionOwned)
				require.False(t, started)
				if mode != "missing" {
					assertNativeDeletionRows(t, ctx, b, request, sessionID)
				}
				return
			}
			assertNativeDeletionRows(t, ctx, b, request, sessionID)
			current, job, err := b.InspectManagedDeletion(ctx, request)
			require.NoError(t, err)
			require.Equal(t, claim.Owner, current.Owner)
			require.Equal(t, models.TaskResourceCleanupStatePrepared, job.State)
			released, err := b.ReleaseManagedDeletion(ctx, *claim)
			require.NoError(t, err)
			require.True(t, released)
		})
	}
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.1, AC-PLUGINS-MANAGED-COORDINATION-013.2, AC-PLUGINS-MANAGED-COORDINATION-013.4, AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestManagedDeletionAdmissionSQLiteWaits(t *testing.T) {
	for _, mode := range []string{"admit_revision", "admit_detach", "admit_identity", "admit_cancel", "final_revision", "final_detach", "final_identity", "final_cancel"} {
		t.Run(mode, func(t *testing.T) {
			a, b, observer := newManagedDeletionSQLitePair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			var workers sync.WaitGroup
			var holder *sql.Tx
			t.Cleanup(func() {
				cancel()
				if holder != nil {
					_ = holder.Rollback()
				}
				workers.Wait()
			})
			_, request, sessionID := seedNativeManagedDeletion(t, ctx, a, "sqlite3")
			var claim *managed.DeleteClaim
			var err error
			if mode[:5] == "final" {
				claim, err = a.AdmitManagedDeletion(ctx, request, "owner")
				require.NoError(t, err)
				_, err = a.PrepareManagedDeletion(ctx, *claim, nativeDeletionInventory(t, ctx, a, request))
				require.NoError(t, err)
			}
			var busyTimeout int
			require.NoError(t, b.DB().QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout))
			require.Equal(t, 5000, busyTimeout)
			holder, err = a.DB().BeginTx(ctx, nil)
			require.NoError(t, err)
			_, err = holder.ExecContext(ctx, `UPDATE tasks SET title = title WHERE id = ?`, request.TaskID)
			require.NoError(t, err)
			_, err = observer.ExecContext(ctx, `PRAGMA busy_timeout=0`)
			require.NoError(t, err)
			_, err = observer.ExecContext(ctx, `UPDATE tasks SET id = id WHERE 0`)
			var busy sqlite3.Error
			require.ErrorAs(t, err, &busy)
			require.Equal(t, sqlite3.ErrBusy, busy.Code)
			waiterCtx, cancelWaiter := context.WithCancel(ctx)
			defer cancelWaiter()
			done := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				if claim == nil {
					_, err := b.AdmitManagedDeletion(waiterCtx, request, "waiter")
					done <- err
				} else {
					_, err := b.FinalizeManagedDeletion(waiterCtx, *claim)
					done <- err
				}
			}()
			sqlWaiter := waitManagedDeletionSQLiteSQL(t, ctx, done)
			require.Equal(t, 1, b.DB().Stats().InUse)
			t.Logf("%s: native writer goroutine %s waits inside sqlite3_step; independent observer proves SQLITE_BUSY writer reservation", mode, sqlWaiter)
			started := time.Now()
			if mode == "admit_cancel" || mode == "final_cancel" {
				_, err = holder.ExecContext(ctx, `UPDATE tasks SET title=title WHERE id=?`, request.TaskID)
				require.NoError(t, err)
			} else {
				key, value := models.MetaKeyManagedConversationRevision, `"2"`
				if mode == "admit_detach" || mode == "final_detach" {
					key, value = models.MetaKeyManagedConversationDetached, `true`
				}
				if mode == "admit_identity" || mode == "final_identity" {
					key, value = models.MetaKeyManagedInstallationID, `"foreign"`
				}
				_, err = holder.ExecContext(ctx, `UPDATE tasks SET metadata=json_set(metadata, ?, json(?)) WHERE id=?`, fmt.Sprintf(`$."%s"`, key), value, request.TaskID)
				require.NoError(t, err)
			}
			_, probeErr := observer.ExecContext(ctx, `UPDATE tasks SET id=id WHERE 0`)
			require.ErrorAs(t, probeErr, &busy)
			require.Equal(t, sqlite3.ErrBusy, busy.Code)
			require.Equal(t, sqlWaiter, managedDeletionSQLiteWorker(), "same BEGIN worker across canonical mutation and actual BUSY probe")
			cancelled := mode == "admit_cancel" || mode == "final_cancel"
			if cancelled {
				cancelWaiter()
			} else {
				require.NoError(t, holder.Commit())
			}
			select {
			case err = <-done:
			case <-ctx.Done():
				t.Fatal("native writer did not settle")
			}
			workers.Wait()
			if cancelled {
				t.Logf("cancelled BEGIN settled while holder remains locked after %s: %#v", time.Since(started), err)
				_, probeErr = observer.ExecContext(ctx, `UPDATE tasks SET id=id WHERE 0`)
				require.ErrorAs(t, probeErr, &busy)
				require.Equal(t, sqlite3.ErrBusy, busy.Code)
				require.NoError(t, holder.Commit())
			}
			require.NoError(t, b.DB().PingContext(ctx))
			require.Zero(t, b.DB().Stats().InUse)
			current, job, inspectErr := a.InspectManagedDeletion(ctx, request)
			require.NoError(t, inspectErr)
			if claim == nil {
				require.Nil(t, current)
				require.Nil(t, job)
			} else {
				require.Equal(t, managed.DeletePrepared, current.Phase)
				require.Equal(t, models.TaskResourceCleanupStatePrepared, job.State)
			}
			switch mode {
			case "admit_cancel", "final_cancel":
				require.Error(t, err)
			case "admit_revision", "final_revision":
				require.ErrorIs(t, err, managed.ErrRevision)
			default:
				require.ErrorIs(t, err, managed.ErrNotFound)
			}
			assertNativeDeletionRows(t, ctx, b, request, sessionID)
		})
	}
}

// The waiter must remain inside the same actual BEGIN C worker across holder
// SQL and an independent BUSY probe; a transient sample is insufficient.
func managedDeletionSQLiteWorker() string {
	buffer := make([]byte, 1<<20)
	sections := strings.Split(string(buffer[:runtime.Stack(buffer, true)]), "\n\n")
	for _, parent := range sections {
		native := strings.Contains(parent, ".AdmitManagedDeletion(") || strings.Contains(parent, ".deleteTaskWithVacatedStep(")
		if !native || !strings.Contains(parent, "(*SQLiteConn).begin(") {
			continue
		}
		owner := strings.Fields(parent)[1]
		for _, child := range sections {
			if strings.Contains(child, "_Cfunc__sqlite3_step_row_internal(") && strings.Contains(child, "created by github.com/mattn/go-sqlite3.(*SQLiteStmt).exec in goroutine "+owner+"\n") {
				return owner + "/" + strings.Fields(child)[1]
			}
		}
	}
	return ""
}

func waitManagedDeletionSQLiteSQL(t *testing.T, ctx context.Context, done <-chan error) string {
	t.Helper()
	for {
		if worker := managedDeletionSQLiteWorker(); worker != "" {
			return worker
		}
		select {
		case err := <-done:
			t.Fatalf("native writer returned before physical BEGIN wait: %#v", err)
		case <-ctx.Done():
			t.Fatal("native SQL wait not observed", ctx.Err())
		default:
			runtime.Gosched()
		}
	}
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.5
func TestManagedDeletionAdmissionTransientBarrier(t *testing.T) {
	for _, mode := range []string{"storage", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			a, b, _ := newManagedDeletionSQLitePair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, request, sessionID := seedNativeManagedDeletion(t, ctx, a, "sqlite3")
			attempt, cancelAttempt := context.WithCancel(ctx)
			defer cancelAttempt()
			setNativeDeletionAuthorizer(t, a, func(op int, table, _, _ string) int {
				if op == sqlite3.SQLITE_READ && table == "task_resource_cleanup_jobs" {
					if mode == "cancel" {
						cancelAttempt()
					}
					return sqlite3.SQLITE_DENY
				}
				return sqlite3.SQLITE_OK
			})
			input := managed.StateRequest{Identity: request.Identity, Kind: managed.PauseExact, Paused: true, ExpectedRevision: 1, OperationID: "pause", PayloadDigest: "pause-digest"}
			_, err := a.ChangeManagedConversationState(attempt, input)
			require.Error(t, err)
			require.NotErrorIs(t, err, managed.ErrBusy)
			if mode == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			}
			setNativeDeletionAuthorizer(t, a, nil)
			assertNativeDeletionRows(t, ctx, b, request, sessionID)
			result, err := a.ChangeManagedConversationState(ctx, input)
			require.NoError(t, err)
			require.True(t, result.Changed)
		})
	}
}

func setNativeDeletionAuthorizer(t *testing.T, repo *tasksqlite.Repository, authorize func(int, string, string, string) int) {
	t.Helper()
	conn, err := repo.DB().Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, conn.Raw(func(raw interface{}) error {
		raw.(*sqlite3.SQLiteConn).RegisterAuthorizer(authorize)
		return nil
	}))
	require.NoError(t, conn.Close())
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestManagedDeletionAdmissionSnapshotAuthority(t *testing.T) {
	a, b, _ := newManagedDeletionSQLitePair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, request, sessionID := seedNativeManagedDeletion(t, ctx, a, "sqlite3")
	job := &models.TaskResourceCleanupJob{OperationID: "ordinary", TaskID: request.TaskID, Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStatePrepared, ResourceSnapshot: "{}"}
	require.NoError(t, a.CreateTaskResourceCleanupJob(ctx, job))
	forged, err := managed.WithDeletionEnvelope("{}", managed.DeleteClaim{DeleteRequest: request, Version: 1, JobID: job.ID, Owner: "forged", Phase: managed.DeleteCommitted})
	require.NoError(t, err)
	require.ErrorIs(t, b.UpdateTaskResourceCleanupSnapshot(ctx, job.OperationID, forged), managed.ErrUnavailable)
	stored, err := a.GetTaskResourceCleanupJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "{}", stored.ResourceSnapshot)
	require.Equal(t, models.TaskResourceCleanupStatePrepared, stored.State)
	require.NoError(t, b.UpdateTaskResourceCleanupSnapshot(ctx, job.OperationID, `{"workspace_id":"delete-ws"}`))
	stored, err = a.GetTaskResourceCleanupJob(ctx, job.ID)
	require.NoError(t, err)
	require.JSONEq(t, `{"workspace_id":"delete-ws"}`, stored.ResourceSnapshot)
	assertNativeDeletionRows(t, ctx, a, request, sessionID)
}
