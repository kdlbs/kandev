package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/canvas"
	internaldb "github.com/kandev/kandev/internal/db"
	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/task/models"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type managedDeleteLifecycleGate struct {
	*Service
	seen, release chan struct{}
}

func (g *managedDeleteLifecycleGate) DeleteManagedConversationTask(ctx context.Context, request managed.DeleteRequest) error {
	close(g.seen)
	select {
	case <-g.release:
		return g.Service.DeleteManagedConversationTask(ctx, request)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *managedDeleteLifecycleGate) DeleteTask(ctx context.Context, id string) error {
	close(g.seen)
	select {
	case <-g.release:
		return g.Service.DeleteTask(ctx, id)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func managedDeletionPair(t *testing.T) ([2]*AgentConversationService, [2]*sqliterepo.Repository, *Service, *MockEventBus) {
	t.Helper()
	lifecycle, events, first := createTestService(t)
	var sequence int
	var name, path string
	require.NoError(t, first.DB().QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path))
	raw, err := internaldb.OpenSQLite(path)
	require.NoError(t, err)
	second := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	repos := [2]*sqliterepo.Repository{first, sqliterepo.NewWithInitializedDB(second, second, nil)}
	var conversations [2]*AgentConversationService
	for i, repo := range repos {
		database := sqlx.NewDb(repo.DB(), "sqlite3")
		store, err := state.NewStore(internaldb.NewPool(database, database))
		require.NoError(t, err)
		conversations[i] = NewAgentConversationService(repo, repo, nil, store, nil)
		conversations[i].SetTaskDeleter(lifecycle)
	}
	seedConversationWorkspace(t, first, "admission-workspace")
	return conversations, repos, lifecycle, events
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.1, AC-PLUGINS-MANAGED-COORDINATION-013.2, AC-PLUGINS-MANAGED-COORDINATION-013.4
func TestManagedDeletionAdmissionInterleavings(t *testing.T) {
	for _, mode := range []string{"update_after_read", "detach_after_read", "already_stale_control", "current_delete_control"} {
		t.Run(mode, func(t *testing.T) {
			services, repos, lifecycle, events := managedDeletionPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			gate := &managedDeleteLifecycleGate{Service: lifecycle, seen: make(chan struct{}), release: make(chan struct{})}
			var workers sync.WaitGroup
			var once sync.Once
			release := func() { once.Do(func() { close(gate.release) }) }
			t.Cleanup(func() { cancel(); release(); workers.Wait() })
			spec := managedAdmissionSpec()
			first, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "create", "create-digest")
			require.NoError(t, err)
			require.NoError(t, repos[0].CreateTurn(ctx, &models.Turn{
				ID: "retained-turn", TaskID: first.TaskID, TaskSessionID: first.SessionID,
			}))
			require.NoError(t, repos[0].CreateMessage(ctx, &models.Message{
				ID: "retained-message", TaskID: first.TaskID, TaskSessionID: first.SessionID, TurnID: "retained-turn",
				AuthorType: models.MessageAuthorUser, Content: "accepted transcript",
			}))
			events.ClearEvents()
			changed := spec
			changed.ExpectedRevision, changed.BasePrompt = first.Revision, "independently accepted instructions"
			if mode == "already_stale_control" {
				_, _, err = services[1].EnsureManaged(ctx, "plugin", "installation", changed, "winner", "winner-digest")
				require.NoError(t, err)
			}
			if mode == "current_delete_control" || mode == "already_stale_control" {
				err = services[0].DeleteManaged(ctx, "installation", spec.WorkspaceID, spec.InstanceKey, first.Revision, "delete", "delete-digest")
			} else {
				services[0].SetTaskDeleter(gate)
				done := make(chan error, 1)
				workers.Add(1)
				go func() {
					defer workers.Done()
					done <- services[0].DeleteManaged(ctx, "installation", spec.WorkspaceID, spec.InstanceKey, first.Revision, "delete", "delete-digest")
				}()
				select {
				case <-gate.seen:
				case <-ctx.Done():
					t.Fatal("lifecycle admission boundary not reached")
				}
				if mode == "detach_after_read" {
					require.NoError(t, services[1].DetachManagedForInstallation(ctx, "installation"))
					stored, readErr := repos[0].GetTask(ctx, first.TaskID)
					require.NoError(t, readErr)
					require.Equal(t, true, stored.Metadata[models.MetaKeyManagedConversationDetached])
				} else {
					accepted, _, updateErr := services[1].EnsureManaged(ctx, "plugin", "installation", changed, "winner", "winner-digest")
					require.NoError(t, updateErr)
					require.Equal(t, first.Revision+1, accepted.Revision)
					stored, readErr := repos[0].GetTask(ctx, first.TaskID)
					require.NoError(t, readErr)
					require.Equal(t, changed.BasePrompt, stored.Metadata["kandev.base_prompt"])
				}
				release()
				select {
				case err = <-done:
				case <-ctx.Done():
					t.Fatal("delete did not settle")
				}
				workers.Wait()
			}
			if mode == "current_delete_control" {
				require.NoError(t, err)
				_, err = repos[1].GetTask(ctx, first.TaskID)
				require.Error(t, err)
				require.NotEmpty(t, events.GetPublishedEvents())
				return
			}
			want := codes.Aborted
			if mode == "detach_after_read" {
				want = codes.NotFound
			}
			// Check preservation before the result so a falsely successful
			// deletion reports the lost accepted identity explicitly.
			_, readErr := repos[1].GetTask(ctx, first.TaskID)
			require.NoError(t, readErr, "accepted task must survive rejected deletion")
			primary, readErr := repos[1].GetPrimarySessionByTaskID(ctx, first.TaskID)
			require.NoError(t, readErr)
			require.Equal(t, first.SessionID, primary.ID)
			messages, readErr := repos[1].ListMessages(ctx, first.SessionID)
			require.NoError(t, readErr)
			require.Len(t, messages, 1)
			require.Equal(t, "accepted transcript", messages[0].Content)
			require.Equal(t, want, status.Code(err))
			require.Empty(t, events.GetPublishedEvents())
			jobs, readErr := repos[1].ListTaskResourceCleanupJobs(ctx, first.TaskID)
			require.NoError(t, readErr)
			require.Empty(t, jobs)
		})
	}
}

type managedDeleteAdmissionGate struct {
	*sqliterepo.Repository
	seen, release chan struct{}
}

func (g *managedDeleteAdmissionGate) AdmitManagedDeletion(ctx context.Context, request managed.DeleteRequest, owner string) (*managed.DeleteClaim, error) {
	claim, err := g.Repository.AdmitManagedDeletion(ctx, request, owner)
	if err != nil {
		return claim, err
	}
	close(g.seen)
	select {
	case <-g.release:
		return claim, nil
	case <-ctx.Done():
		return claim, ctx.Err()
	}
}

func seedManagedDeletionTranscript(t *testing.T, ctx context.Context, svc *AgentConversationService, repo *sqliterepo.Repository) pluginsdk.ManagedAgentConversationDescriptor {
	t.Helper()
	first, _, err := svc.EnsureManaged(ctx, "plugin", "installation", managedAdmissionSpec(), "create", "create-digest")
	require.NoError(t, err)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "retained-turn", TaskID: first.TaskID, TaskSessionID: first.SessionID}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: "retained-message", TaskID: first.TaskID, TaskSessionID: first.SessionID, TurnID: "retained-turn", AuthorType: models.MessageAuthorUser, Content: "accepted transcript"}))
	return first
}

func assertManagedDeletionRows(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, first pluginsdk.ManagedAgentConversationDescriptor) {
	t.Helper()
	_, err := repo.GetTask(ctx, first.TaskID)
	require.NoError(t, err)
	primary, err := repo.GetPrimarySessionByTaskID(ctx, first.TaskID)
	require.NoError(t, err)
	require.Equal(t, first.SessionID, primary.ID)
	messages, err := repo.ListMessages(ctx, first.SessionID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "accepted transcript", messages[0].Content)
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.3, AC-PLUGINS-MANAGED-COORDINATION-013.4, AC-PLUGINS-MANAGED-COORDINATION-013.5, AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestManagedDeletionAdmissionFailureEffects(t *testing.T) {
	for _, mode := range []string{"admission_rollback", "canvas_abort", "final_rollback", "current_delete", "cancel_before_admission", "cancel_after_admission"} {
		t.Run(mode, func(t *testing.T) {
			conversations, repos, lifecycle, events := managedDeletionPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			first := seedManagedDeletionTranscript(t, ctx, conversations[0], repos[0])
			canvasRepo, canvasSvc, canvasID := createManagedDeletionCanvas(t, ctx, repos[0], first.TaskID)
			lifecycle.SetCanvasCleanup(canvasSvc)
			events.ClearEvents()
			switch mode {
			case "admission_rollback":
				_, err := repos[0].DB().ExecContext(ctx, `CREATE TRIGGER fail_delete_admission BEFORE INSERT ON task_resource_cleanup_jobs BEGIN SELECT RAISE(ABORT, 'admission rollback'); END`)
				require.NoError(t, err)
			case "canvas_abort":
				_, err := repos[0].DB().ExecContext(ctx, `CREATE TRIGGER fail_canvas_removal BEFORE DELETE ON canvas_lifecycle_metadata BEGIN SELECT RAISE(ABORT, 'canvas abort'); END`)
				require.NoError(t, err)
			case "final_rollback":
				_, err := repos[0].DB().ExecContext(ctx, `CREATE TRIGGER fail_task_removal BEFORE DELETE ON tasks BEGIN SELECT RAISE(ABORT, 'final rollback'); END`)
				require.NoError(t, err)
			}
			deleteCtx, cancelDelete := context.WithCancel(ctx)
			defer cancelDelete()
			if mode == "cancel_before_admission" {
				cancelDelete()
			}
			var deleteErr error
			if mode == "cancel_after_admission" {
				gate := &managedDeleteAdmissionGate{Repository: repos[0], seen: make(chan struct{}), release: make(chan struct{})}
				lifecycle.tasks = gate
				done := make(chan error, 1)
				var workers sync.WaitGroup
				var once sync.Once
				release := func() { once.Do(func() { close(gate.release) }) }
				t.Cleanup(func() { cancelDelete(); release(); workers.Wait() })
				workers.Add(1)
				go func() {
					defer workers.Done()
					done <- conversations[0].DeleteManaged(deleteCtx, "installation", "admission-workspace", "lead", first.Revision, "delete", "delete-digest")
				}()
				select {
				case <-gate.seen:
				case <-ctx.Done():
					t.Fatal("exclusive admission not reached")
				}
				cancelDelete()
				select {
				case deleteErr = <-done:
				case <-ctx.Done():
					t.Fatal("cancelled owner did not settle")
				}
				workers.Wait()
			} else {
				deleteErr = conversations[0].DeleteManaged(deleteCtx, "installation", "admission-workspace", "lead", first.Revision, "delete", "delete-digest")
			}
			if mode == "current_delete" {
				require.NoError(t, deleteErr)
				_, err := repos[1].GetTask(ctx, first.TaskID)
				require.Error(t, err)
				_, err = repos[1].GetTaskSession(ctx, first.SessionID)
				require.Error(t, err)
				require.NotEmpty(t, events.GetPublishedEvents())
			} else {
				require.Error(t, deleteErr)
				assertManagedDeletionRows(t, ctx, repos[1], first)
				require.Empty(t, events.GetPublishedEvents())
			}
			_, canvasErr := canvasRepo.Get(ctx, canvasID)
			if mode == "final_rollback" || mode == "current_delete" {
				require.Error(t, canvasErr, "actual admitted canvas removal persists")
			} else {
				require.NoError(t, canvasErr)
			}
			jobs, err := repos[1].ListTaskResourceCleanupJobs(ctx, first.TaskID)
			require.NoError(t, err)
			if mode == "admission_rollback" || mode == "cancel_before_admission" {
				require.Empty(t, jobs)
				return
			}
			require.Len(t, jobs, 1)
			claim, err := managed.DeletionEnvelope(jobs[0].ResourceSnapshot)
			require.NoError(t, err)
			require.NotNil(t, claim)
			if mode == "current_delete" {
				require.Equal(t, managed.DeleteCommitted, claim.Phase)
				return
			}
			require.Equal(t, models.TaskResourceCleanupStateCancelled, jobs[0].State)
			require.NotEqual(t, managed.DeleteCommitted, claim.Phase)
			require.Equal(t, codes.Unavailable, status.Code(deleteErr))
			var admitted *ManagedDeletionError
			require.ErrorAs(t, deleteErr, &admitted)
			require.Equal(t, "admitted_failed", admitted.Outcome)
		})
	}
}

func createManagedDeletionCanvas(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID string) (*canvas.Repository, *canvas.Service, string) {
	t.Helper()
	database := sqlx.NewDb(repo.DB(), "sqlite3")
	pool := internaldb.NewPool(database, database)
	canvasRepo, err := canvas.NewRepository(pool)
	require.NoError(t, err)
	instances, err := plugininstances.NewStore(pool)
	require.NoError(t, err)
	svc := canvas.NewService(canvasRepo, instances)
	created, err := svc.Create(ctx, canvas.CreateCanvasRequest{WorkspaceID: "admission-workspace", TaskID: taskID, Title: "Retained task canvas", CreatedBySessionID: "author"})
	require.NoError(t, err)
	return canvasRepo, svc, created.ID
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.3, AC-PLUGINS-MANAGED-COORDINATION-013.5, AC-PLUGINS-MANAGED-COORDINATION-013.7, AC-PLUGINS-MANAGED-COORDINATION-013.8
func TestManagedDeletionAdmissionOwnershipRecovery(t *testing.T) {
	for _, mode := range []string{"update_loses", "detach_loses", "same_operation_foreign", "independent_restart", "cancelled_reuse", "committed_replay"} {
		t.Run(mode, func(t *testing.T) {
			conversations, repos, lifecycle, events := managedDeletionPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			first := seedManagedDeletionTranscript(t, ctx, conversations[0], repos[0])
			task, err := repos[0].GetTask(ctx, first.TaskID)
			require.NoError(t, err)
			request := managed.DeleteRequest{Identity: managedTaskIdentity(task), ExpectedRevision: first.Revision, TaskCreatedAt: task.CreatedAt, OperationID: "delete", PayloadDigest: "delete-digest"}
			if mode == "cancelled_reuse" || mode == "committed_replay" {
				claim, err := repos[0].AdmitManagedDeletion(ctx, request, "original-owner")
				require.NoError(t, err)
				if mode == "cancelled_reuse" {
					released, err := repos[0].ReleaseManagedDeletion(ctx, *claim)
					require.NoError(t, err)
					require.True(t, released)
					next, err := repos[1].AdmitManagedDeletion(ctx, request, "new-owner")
					require.NoError(t, err)
					require.Equal(t, claim.JobID, next.JobID)
					_, err = repos[0].PrepareManagedDeletion(ctx, *claim, `{}`)
					require.ErrorIs(t, err, managed.ErrDeletionOwned)
					_, err = repos[0].ReleaseManagedDeletion(ctx, *claim)
					require.ErrorIs(t, err, managed.ErrDeletionOwned)
					released, err = repos[1].ReleaseManagedDeletion(ctx, *next)
					require.NoError(t, err)
					require.True(t, released)
					assertManagedDeletionRows(t, ctx, repos[1], first)
				} else {
					_, err = repos[0].PrepareManagedDeletion(ctx, *claim, fmt.Sprintf(`{"workspace_id":"admission-workspace","sessions":[{"id":%q,"task_id":%q}]}`, first.SessionID, first.TaskID))
					require.NoError(t, err)
					_, err = repos[0].FinalizeManagedDeletion(ctx, *claim)
					require.NoError(t, err)
					events.ClearEvents()
					replayErr := lifecycle.DeleteManagedConversationTask(ctx, request)
					var outcome *ManagedDeletionError
					require.ErrorAs(t, replayErr, &outcome)
					require.Equal(t, "committed", outcome.Outcome)
					require.Empty(t, events.GetPublishedEvents())
				}
				return
			}
			gate := &managedDeleteAdmissionGate{Repository: repos[0], seen: make(chan struct{}), release: make(chan struct{})}
			lifecycle.tasks = gate
			done := make(chan error, 1)
			var workers sync.WaitGroup
			var once sync.Once
			release := func() { once.Do(func() { close(gate.release) }) }
			t.Cleanup(func() { cancel(); release(); workers.Wait() })
			workers.Add(1)
			go func() {
				defer workers.Done()
				done <- conversations[0].DeleteManaged(ctx, "installation", "admission-workspace", "lead", first.Revision, "delete", "delete-digest")
			}()
			select {
			case <-gate.seen:
			case <-ctx.Done():
				t.Fatal("exclusive admission not reached")
			}
			switch mode {
			case "update_loses":
				changed := managedAdmissionSpec()
				changed.ExpectedRevision, changed.BasePrompt = first.Revision, "losing instructions"
				_, _, err = conversations[1].EnsureManaged(ctx, "plugin", "installation", changed, "update", "update-digest")
				require.Equal(t, codes.FailedPrecondition, status.Code(err))
			case "detach_loses":
				err = conversations[1].DetachManagedForInstallation(ctx, "installation")
				require.Equal(t, codes.FailedPrecondition, status.Code(err))
			case "same_operation_foreign":
				err = conversations[1].DeleteManaged(ctx, "installation", "admission-workspace", "lead", first.Revision, "delete", "delete-digest")
				require.Equal(t, codes.Unavailable, status.Code(err))
			case "independent_restart":
				// Use a distinct production Service with a different native pool.
				independent := NewService(Repos{Tasks: repos[1], Sessions: repos[1], ResourceCleanups: repos[1]}, NewMockEventBus(), lifecycle.logger, RepositoryDiscoveryConfig{})
				require.NoError(t, independent.StartTaskResourceCleanupWorker(ctx))
				independent.StopTaskResourceCleanupWorker()
			}
			currentClaim, job, err := repos[1].InspectManagedDeletion(ctx, request)
			require.NoError(t, err)
			require.NotNil(t, currentClaim)
			require.Equal(t, models.TaskResourceCleanupStatePrepared, job.State)
			require.Equal(t, managed.DeleteReserved, currentClaim.Phase)
			assertManagedDeletionRows(t, ctx, repos[1], first)
			release()
			select {
			case err = <-done:
			case <-ctx.Done():
				t.Fatal("admitted delete did not settle")
			}
			workers.Wait()
			require.NoError(t, err)
			_, err = repos[1].GetTask(ctx, first.TaskID)
			require.Error(t, err)
		})
	}
}
