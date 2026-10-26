package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	publishTaskID        = "task-nomad-executor-inventory"
	publishEnvironmentID = "environment-nomad-inventory"
	publishSessionID     = "session-nomad-inventory"
	publishExecutionID   = "execution-publish"
	publishGeneration    = int64(3)
)

// publishFixture is a Created session bound to a creating Nomad environment
// whose retained inventory is in phase ready.
type publishFixture struct {
	repo          *Repository
	revision      uint64
	workspacePath string
}

func newPublishFixture(t *testing.T) *publishFixture {
	t.Helper()
	return newPublishFixtureOn(t, newRepoForSessionTests(t))
}

func newPublishFixtureOn(t *testing.T, repo *Repository) *publishFixture {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: publishTaskID, Title: "Nomad publish"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: publishEnvironmentID, TaskID: publishTaskID, ExecutorType: string(models.ExecutorTypeNomad),
		OwnershipGeneration: publishGeneration, Status: models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: publishSessionID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: publishSessionID, TaskID: publishTaskID, TaskEnvironmentID: publishEnvironmentID,
		State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatal(err)
	}
	fx := &publishFixture{repo: repo}
	var revision uint64
	for _, phase := range []string{"intent", "registered", "allocated", "ready"} {
		running := nomadExecutorRunningFixture(publishExecutionID, publishGeneration, phase)
		running.ExpectedNomadExecutorRevision = revision
		state, err := nomad.EnvironmentStateFromMetadata(running.Metadata)
		if err != nil {
			t.Fatal(err)
		}
		if phase == "allocated" || phase == "ready" {
			state.AllocationID, state.AllocationCreateIndex, state.ServerIdentity = "allocation-publish", 7, "server-publish"
			if err := nomad.PutEnvironmentState(running.Metadata, state); err != nil {
				t.Fatal(err)
			}
		}
		fx.workspacePath = state.WorkspacePath
		if err := repo.CheckpointNomadExecutorInventory(ctx, running); err != nil {
			t.Fatalf("checkpoint %s: %v", phase, err)
		}
		revision = running.ExpectedNomadExecutorRevision
	}
	fx.revision = revision
	return fx
}

func (fx *publishFixture) params() PublishRecoveredNomadEnvironmentReadyParams {
	return PublishRecoveredNomadEnvironmentReadyParams{
		SessionID: publishSessionID, EnvironmentID: publishEnvironmentID, ExecutionID: publishExecutionID,
		OwnershipGeneration: publishGeneration, ExpectedRevision: fx.revision,
		Repos: []*models.TaskEnvironmentRepo{{
			RepositoryID: "repository-one", BranchSlug: "main", WorkspaceRelativePath: "repo-one", Position: 0,
		}},
	}
}

func (fx *publishFixture) publish(params PublishRecoveredNomadEnvironmentReadyParams) (bool, error) {
	return fx.repo.PublishRecoveredNomadEnvironmentReady(context.Background(), params)
}

// rewriteInventory edits the stored typed state in place without touching the
// revision, to reach states the checkpoint writer refuses to produce.
func (fx *publishFixture) rewriteInventory(t *testing.T, mutate func(*nomad.EnvironmentState)) {
	t.Helper()
	ctx := context.Background()
	var raw string
	if err := fx.repo.db.QueryRowContext(ctx, fx.repo.db.Rebind(`SELECT metadata FROM executors_running WHERE session_id = ?`), publishSessionID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		t.Fatal(err)
	}
	state, err := nomad.EnvironmentStateFromMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&state)
	// Marshal directly: the typed writer refuses the invalid states this
	// helper exists to reach.
	rawState, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	metadata[nomad.EnvironmentStateMetadataKey] = json.RawMessage(rawState)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	fx.exec(t, `UPDATE executors_running SET metadata = ? WHERE session_id = ?`, string(encoded), publishSessionID)
}

func (fx *publishFixture) exec(t *testing.T, query string, args ...interface{}) {
	t.Helper()
	if _, err := fx.repo.db.Exec(fx.repo.db.Rebind(query), args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func (fx *publishFixture) addCleanupBarrier(t *testing.T) {
	t.Helper()
	now := time.Now().UTC()
	fx.exec(t, `INSERT INTO task_resource_cleanup_jobs (id, operation_id, task_id, trigger, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "cleanup-publish", "cleanup-publish-op", publishTaskID, "archive", "running", now, now)
}

func (fx *publishFixture) addRecoveryClaim(t *testing.T) {
	t.Helper()
	now := time.Now().UTC()
	fx.exec(t, `INSERT INTO task_environment_recovery_claims (task_environment_id, owner_task_id, ownership_generation, session_id, operation_id, executor_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, publishEnvironmentID, publishTaskID, publishGeneration, publishSessionID, "claim-operation", string(models.ExecutorTypeNomad), now, now)
}

func (fx *publishFixture) environment(t *testing.T) *models.TaskEnvironment {
	t.Helper()
	env, err := fx.repo.GetTaskEnvironment(context.Background(), publishEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func (fx *publishFixture) assertUnpublished(t *testing.T) {
	t.Helper()
	env := fx.environment(t)
	if env.Status != models.TaskEnvironmentStatusCreating || env.MaterializationSessionID != publishSessionID || env.WorkspacePath != "" || len(env.Repos) != 0 {
		t.Fatalf("environment changed by a refused publish: status=%q materializer=%q path=%q repos=%d", env.Status, env.MaterializationSessionID, env.WorkspacePath, len(env.Repos))
	}
}

func (fx *publishFixture) assertPublished(t *testing.T) {
	t.Helper()
	env := fx.environment(t)
	if env.Status != models.TaskEnvironmentStatusReady || env.MaterializationSessionID != "" || env.WorkspacePath != fx.workspacePath {
		t.Fatalf("environment = status %q materializer %q path %q, want ready with path %q", env.Status, env.MaterializationSessionID, env.WorkspacePath, fx.workspacePath)
	}
}

func wantPublishError(t *testing.T, published bool, err, want error) {
	t.Helper()
	if published || !errors.Is(err, want) {
		t.Fatalf("publish = (%v, %v), want (false, %v)", published, err, want)
	}
}

func TestPublishRecoveredNomadEnvironmentReady_Success(t *testing.T) {
	fx := newPublishFixture(t)
	ctx := context.Background()
	before, err := fx.repo.GetExecutorRunningBySessionID(ctx, publishSessionID)
	if err != nil {
		t.Fatal(err)
	}
	params := fx.params()
	params.Repos = append(params.Repos, &models.TaskEnvironmentRepo{RepositoryID: "repository-two", BranchSlug: "dev", WorkspaceRelativePath: "repo-two", Position: 1})

	published, err := fx.publish(params)
	if err != nil || !published {
		t.Fatalf("publish = (%v, %v), want (true, nil)", published, err)
	}
	fx.assertPublished(t)
	env := fx.environment(t)
	if len(env.Repos) != 2 {
		t.Fatalf("repo rows = %d, want 2", len(env.Repos))
	}
	for i, want := range []struct{ id, slug, rel string }{{"repository-one", "main", "repo-one"}, {"repository-two", "dev", "repo-two"}} {
		got := env.Repos[i]
		if got.RepositoryID != want.id || got.BranchSlug != want.slug || got.WorkspaceRelativePath != want.rel || got.Position != i || got.WorktreePath != "" || got.WorktreeID != "" {
			t.Fatalf("repo row %d = %+v, want %+v with no worktree path or id", i, got, want)
		}
	}
	session, err := fx.repo.GetTaskSession(ctx, publishSessionID)
	if err != nil || session.State != models.TaskSessionStateCreated {
		t.Fatalf("session = %+v, %v, want state Created", session, err)
	}
	after, err := fx.repo.GetExecutorRunningBySessionID(ctx, publishSessionID)
	if err != nil || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("publish rewrote the inventory row: before %v after %v (%v)", before.UpdatedAt, after.UpdatedAt, err)
	}
}

func TestPublishRecoveredNomadEnvironmentReady_StaleGeneration(t *testing.T) {
	t.Run("param", func(t *testing.T) {
		fx := newPublishFixture(t)
		params := fx.params()
		params.OwnershipGeneration++
		published, err := fx.publish(params)
		wantPublishError(t, published, err, models.ErrExecutionRotated)
		fx.assertUnpublished(t)
	})
	t.Run("environment", func(t *testing.T) {
		fx := newPublishFixture(t)
		fx.exec(t, `UPDATE task_environments SET ownership_generation = ownership_generation + 1 WHERE id = ?`, publishEnvironmentID)
		published, err := fx.publish(fx.params())
		wantPublishError(t, published, err, models.ErrExecutionRotated)
	})
	t.Run("inventory", func(t *testing.T) {
		fx := newPublishFixture(t)
		fx.rewriteInventory(t, func(s *nomad.EnvironmentState) { s.Generation++ })
		published, err := fx.publish(fx.params())
		wantPublishError(t, published, err, models.ErrExecutionRotated)
		fx.assertUnpublished(t)
	})
}

func TestPublishRecoveredNomadEnvironmentReady_WrongOperation(t *testing.T) {
	tests := map[string]func(*testing.T, *publishFixture, *PublishRecoveredNomadEnvironmentReadyParams){
		"param execution": func(_ *testing.T, _ *publishFixture, p *PublishRecoveredNomadEnvironmentReadyParams) {
			p.ExecutionID = "execution-other"
		},
		"inventory operation": func(t *testing.T, fx *publishFixture, _ *PublishRecoveredNomadEnvironmentReadyParams) {
			fx.rewriteInventory(t, func(s *nomad.EnvironmentState) { s.OperationID = "operation-other" })
		},
		"inventory execution": func(t *testing.T, fx *publishFixture, _ *PublishRecoveredNomadEnvironmentReadyParams) {
			fx.rewriteInventory(t, func(s *nomad.EnvironmentState) { s.ExecutionID = "execution-other" })
		},
		"inventory environment": func(t *testing.T, fx *publishFixture, _ *PublishRecoveredNomadEnvironmentReadyParams) {
			fx.rewriteInventory(t, func(s *nomad.EnvironmentState) { s.EnvironmentID = "environment-other" })
		},
		"row execution": func(t *testing.T, fx *publishFixture, _ *PublishRecoveredNomadEnvironmentReadyParams) {
			fx.exec(t, `UPDATE executors_running SET agent_execution_id = ? WHERE session_id = ?`, "execution-rotated", publishSessionID)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fx := newPublishFixture(t)
			params := fx.params()
			mutate(t, fx, &params)
			published, err := fx.publish(params)
			wantPublishError(t, published, err, models.ErrExecutionRotated)
			fx.assertUnpublished(t)
		})
	}
}

func TestPublishRecoveredNomadEnvironmentReady_StaleRevision(t *testing.T) {
	for _, delta := range []int64{-1, 1} {
		fx := newPublishFixture(t)
		params := fx.params()
		params.ExpectedRevision = uint64(int64(params.ExpectedRevision) + delta)
		published, err := fx.publish(params)
		wantPublishError(t, published, err, models.ErrExecutionRotated)
		fx.assertUnpublished(t)
	}
}

func TestPublishRecoveredNomadEnvironmentReady_PreReadyPhase(t *testing.T) {
	for _, phase := range []string{"intent", "registered", "allocated"} {
		t.Run(phase, func(t *testing.T) {
			fx := newPublishFixture(t)
			// Allocation and server identity are legitimately empty before
			// ready, and the revision is stale: the phase still decides.
			fx.rewriteInventory(t, func(s *nomad.EnvironmentState) {
				s.Phase, s.AllocationID, s.AllocationCreateIndex, s.ServerIdentity = phase, "", 0, ""
			})
			params := fx.params()
			params.ExpectedRevision = 0
			published, err := fx.publish(params)
			wantPublishError(t, published, err, models.ErrNomadRecoveryNotReady)
			if errors.Is(err, models.ErrExecutionRotated) {
				t.Fatalf("pre-ready phase must not be terminal: %v", err)
			}
			fx.assertUnpublished(t)
		})
	}
}

func TestPublishRecoveredNomadEnvironmentReady_PostReadyPhase(t *testing.T) {
	for _, phase := range []string{"cleanup_pending", "quarantined", "absent"} {
		t.Run(phase, func(t *testing.T) {
			fx := newPublishFixture(t)
			fx.rewriteInventory(t, func(s *nomad.EnvironmentState) {
				s.Phase = phase
				if phase == "absent" {
					s.DrainReceiptDigest, s.DrainReceiptGeneration = s.InputDigest, s.Generation
				}
			})
			published, err := fx.publish(fx.params())
			wantPublishError(t, published, err, models.ErrExecutionRotated)
			if errors.Is(err, models.ErrNomadRecoveryNotReady) {
				t.Fatalf("post-ready phase must not be retryable: %v", err)
			}
			fx.assertUnpublished(t)
		})
	}
}

func TestPublishRecoveredNomadEnvironmentReady_ReadyInventoryEmptyFields(t *testing.T) {
	tests := map[string]func(*nomad.EnvironmentState){
		"workspace path":  func(s *nomad.EnvironmentState) { s.WorkspacePath = "" },
		"allocation":      func(s *nomad.EnvironmentState) { s.AllocationID, s.AllocationCreateIndex = "", 0 },
		"server identity": func(s *nomad.EnvironmentState) { s.ServerIdentity = "" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fx := newPublishFixture(t)
			fx.rewriteInventory(t, mutate)
			published, err := fx.publish(fx.params())
			wantPublishError(t, published, err, models.ErrExecutionRotated)
			fx.assertUnpublished(t)
		})
	}
}

func TestPublishRecoveredNomadEnvironmentReady_ForeignMaterializer(t *testing.T) {
	fx := newPublishFixture(t)
	fx.exec(t, `UPDATE task_environments SET materialization_session_id = ? WHERE id = ?`, "session-foreign", publishEnvironmentID)
	published, err := fx.publish(fx.params())
	wantPublishError(t, published, err, models.ErrWorkspaceReuseUnsafe)
	env := fx.environment(t)
	if env.Status != models.TaskEnvironmentStatusCreating || len(env.Repos) != 0 {
		t.Fatalf("foreign materializer publish wrote: %+v", env)
	}

	fx = newPublishFixture(t)
	fx.exec(t, `UPDATE task_environments SET status = 'ready', workspace_path = ?, materialization_session_id = ? WHERE id = ?`, fx.workspacePath, "session-foreign", publishEnvironmentID)
	published, err = fx.publish(fx.params())
	wantPublishError(t, published, err, models.ErrWorkspaceReuseUnsafe)
}

func TestPublishRecoveredNomadEnvironmentReady_CleanupOrRecoveryClaimFirst(t *testing.T) {
	t.Run("cleanup barrier", func(t *testing.T) {
		fx := newPublishFixture(t)
		fx.addCleanupBarrier(t)
		params := fx.params()
		params.ExpectedRevision = 0
		params.OwnershipGeneration = 0
		published, err := fx.publish(params)
		wantPublishError(t, published, err, repoerrors.ErrTaskCleanupInProgress)
		fx.assertUnpublished(t)
	})
	t.Run("recovery claim", func(t *testing.T) {
		fx := newPublishFixture(t)
		fx.addRecoveryClaim(t)
		params := fx.params()
		params.ExpectedRevision = 0
		published, err := fx.publish(params)
		wantPublishError(t, published, err, recoveryclaim.ErrBusy)
		fx.assertUnpublished(t)
	})
}

func TestPublishRecoveredNomadEnvironmentReady_NonCreatedSession(t *testing.T) {
	for _, state := range []models.TaskSessionState{models.TaskSessionStateRunning, models.TaskSessionStateCancelled, models.TaskSessionStateWaitingForInput} {
		t.Run(string(state), func(t *testing.T) {
			fx := newPublishFixture(t)
			fx.exec(t, `UPDATE task_sessions SET state = ? WHERE id = ?`, string(state), publishSessionID)
			published, err := fx.publish(fx.params())
			wantPublishError(t, published, err, models.ErrWorkspaceReuseUnsafe)
			fx.assertUnpublished(t)
		})
	}
}

func TestPublishRecoveredNomadEnvironmentReady_TaskMismatch(t *testing.T) {
	t.Run("session task", func(t *testing.T) {
		fx := newPublishFixture(t)
		if err := fx.repo.CreateTask(context.Background(), &models.Task{ID: "task-other", Title: "other"}); err != nil {
			t.Fatal(err)
		}
		fx.exec(t, `UPDATE task_sessions SET task_id = ? WHERE id = ?`, "task-other", publishSessionID)
		published, err := fx.publish(fx.params())
		wantPublishError(t, published, err, models.ErrExecutionRotated)
		fx.assertUnpublished(t)
	})
	t.Run("session bound to another environment", func(t *testing.T) {
		fx := newPublishFixture(t)
		ctx := context.Background()
		if err := fx.repo.CreateTask(ctx, &models.Task{ID: "task-other", Title: "other"}); err != nil {
			t.Fatal(err)
		}
		if err := fx.repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
			ID: "environment-other", TaskID: "task-other", ExecutorType: string(models.ExecutorTypeNomad),
			Status: models.TaskEnvironmentStatusCreating,
		}); err != nil {
			t.Fatal(err)
		}
		fx.exec(t, `UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`, "environment-other", publishSessionID)
		published, err := fx.publish(fx.params())
		wantPublishError(t, published, err, models.ErrExecutionRotated)
		fx.assertUnpublished(t)
	})
}

func TestPublishRecoveredNomadEnvironmentReady_MissingRows(t *testing.T) {
	t.Run("session", func(t *testing.T) {
		fx := newPublishFixture(t)
		params := fx.params()
		params.SessionID = "session-missing"
		published, err := fx.publish(params)
		wantPublishError(t, published, err, models.ErrExecutionRotated)
		fx.assertUnpublished(t)
	})
	t.Run("executors_running", func(t *testing.T) {
		fx := newPublishFixture(t)
		fx.exec(t, `DELETE FROM executors_running WHERE session_id = ?`, publishSessionID)
		published, err := fx.publish(fx.params())
		wantPublishError(t, published, err, models.ErrExecutionRotated)
		fx.assertUnpublished(t)
	})
}

func TestPublishRecoveredNomadEnvironmentReady_MissingEnvironmentRow(t *testing.T) {
	fx := newPublishFixture(t)
	fx.addCleanupBarrier(t)
	params := fx.params()
	params.EnvironmentID = "environment-missing"
	published, err := fx.publish(params)
	wantPublishError(t, published, err, models.ErrExecutionRotated)
	if errors.Is(err, repoerrors.ErrTaskCleanupInProgress) {
		t.Fatalf("a missing environment must surface before the barrier step: %v", err)
	}
}

func TestPublishRecoveredNomadEnvironmentReady_IdempotentReplay(t *testing.T) {
	fx := newPublishFixture(t)
	published, err := fx.publish(fx.params())
	if err != nil || !published {
		t.Fatalf("first publish = (%v, %v)", published, err)
	}
	published, err = fx.publish(fx.params())
	if err != nil || published {
		t.Fatalf("replay = (%v, %v), want (false, nil)", published, err)
	}
	fx.assertPublished(t)
	if got := len(fx.environment(t).Repos); got != 1 {
		t.Fatalf("replay changed repo rows: %d", got)
	}

	t.Run("stale revision still checked", func(t *testing.T) {
		params := fx.params()
		params.ExpectedRevision++
		published, err := fx.publish(params)
		wantPublishError(t, published, err, models.ErrExecutionRotated)
	})
	t.Run("non-created session still checked", func(t *testing.T) {
		fx.exec(t, `UPDATE task_sessions SET state = 'RUNNING' WHERE id = ?`, publishSessionID)
		published, err := fx.publish(fx.params())
		wantPublishError(t, published, err, models.ErrWorkspaceReuseUnsafe)
		fx.exec(t, `UPDATE task_sessions SET state = 'CREATED' WHERE id = ?`, publishSessionID)
	})
	t.Run("barrier still checked", func(t *testing.T) {
		fx.addCleanupBarrier(t)
		published, err := fx.publish(fx.params())
		wantPublishError(t, published, err, repoerrors.ErrTaskCleanupInProgress)
	})
}

func TestPublishRecoveredNomadEnvironmentReady_DifferentPath(t *testing.T) {
	t.Run("creating", func(t *testing.T) {
		fx := newPublishFixture(t)
		fx.exec(t, `UPDATE task_environments SET workspace_path = ? WHERE id = ?`, "/elsewhere", publishEnvironmentID)
		published, err := fx.publish(fx.params())
		wantPublishError(t, published, err, models.ErrWorkspaceReuseUnsafe)
		if env := fx.environment(t); env.Status != models.TaskEnvironmentStatusCreating || env.WorkspacePath != "/elsewhere" {
			t.Fatalf("environment changed: %+v", env)
		}
	})
	t.Run("ready", func(t *testing.T) {
		fx := newPublishFixture(t)
		fx.exec(t, `UPDATE task_environments SET status = 'ready', materialization_session_id = '', workspace_path = ? WHERE id = ?`, "/elsewhere", publishEnvironmentID)
		published, err := fx.publish(fx.params())
		wantPublishError(t, published, err, models.ErrWorkspaceReuseUnsafe)
	})
	t.Run("identical path while creating", func(t *testing.T) {
		fx := newPublishFixture(t)
		fx.exec(t, `UPDATE task_environments SET workspace_path = ? WHERE id = ?`, fx.workspacePath, publishEnvironmentID)
		published, err := fx.publish(fx.params())
		if err != nil || !published {
			t.Fatalf("publish with identical pre-set path = (%v, %v)", published, err)
		}
	})
}

func TestPublishRecoveredNomadEnvironmentReady_OtherEnvironmentStates(t *testing.T) {
	for name, update := range map[string]string{
		"failed":       `UPDATE task_environments SET status = 'failed', materialization_session_id = '' WHERE id = ?`,
		"stopped":      `UPDATE task_environments SET status = 'stopped' WHERE id = ?`,
		"not nomad":    `UPDATE task_environments SET executor_type = 'local' WHERE id = ?`,
		"ready no pth": `UPDATE task_environments SET status = 'ready', materialization_session_id = '' WHERE id = ?`,
	} {
		t.Run(name, func(t *testing.T) {
			fx := newPublishFixture(t)
			fx.exec(t, update, publishEnvironmentID)
			published, err := fx.publish(fx.params())
			wantPublishError(t, published, err, models.ErrWorkspaceReuseUnsafe)
		})
	}
}

func TestPublishRecoveredNomadEnvironmentReady_RepoBackedEmptyProjection(t *testing.T) {
	fx := newPublishFixture(t)
	ctx := context.Background()
	seedWorkspace(t, fx.repo, "workspace-publish")
	if err := fx.repo.CreateRepository(ctx, &models.Repository{ID: "repository-one", WorkspaceID: "workspace-publish", Name: "repository-one"}); err != nil {
		t.Fatal(err)
	}
	if err := fx.repo.CreateTaskRepository(ctx, &models.TaskRepository{
		TaskID: publishTaskID, RepositoryID: "repository-one", Position: 0,
	}); err != nil {
		t.Fatal(err)
	}
	params := fx.params()
	params.Repos = nil
	published, err := fx.publish(params)
	wantPublishError(t, published, err, models.ErrWorkspaceReuseUnsafe)
	fx.assertUnpublished(t)

	// Without repositories configured, an empty projection publishes.
	other := newPublishFixture(t)
	params = other.params()
	params.Repos = nil
	if published, err := other.publish(params); err != nil || !published {
		t.Fatalf("repo-less publish = (%v, %v)", published, err)
	}
	other.assertPublished(t)
}

func TestPublishRecoveredNomadEnvironmentReady_PartialRepoRowsReplaced(t *testing.T) {
	fx := newPublishFixture(t)
	ctx := context.Background()
	for _, row := range []*models.TaskEnvironmentRepo{
		{ID: "stale-one", TaskEnvironmentID: publishEnvironmentID, RepositoryID: "repository-one", BranchSlug: "old", WorkspaceRelativePath: "old", Status: "active"},
		{ID: "stale-two", TaskEnvironmentID: publishEnvironmentID, RepositoryID: "repository-stale", BranchSlug: "old", Status: "active", Position: 1},
	} {
		if err := fx.repo.CreateTaskEnvironmentRepo(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	published, err := fx.publish(fx.params())
	if err != nil || !published {
		t.Fatalf("publish = (%v, %v)", published, err)
	}
	repos := fx.environment(t).Repos
	if len(repos) != 1 || repos[0].RepositoryID != "repository-one" || repos[0].BranchSlug != "main" || repos[0].WorkspaceRelativePath != "repo-one" {
		t.Fatalf("repo rows = %+v, want exactly the projected row", repos)
	}
}

func TestPublishRecoveredNomadEnvironmentReady_ConcurrentOneWinner(t *testing.T) {
	fx := newPublishFixture(t)
	const callers = 6
	type result struct {
		published bool
		err       error
	}
	results := make(chan result, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			published, err := fx.publish(fx.params())
			results <- result{published, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for r := range results {
		if r.err != nil {
			t.Fatalf("concurrent publish error: %v", r.err)
		}
		if r.published {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1", winners)
	}
	fx.assertPublished(t)
	if got := len(fx.environment(t).Repos); got != 1 {
		t.Fatalf("repo rows = %d, want 1", got)
	}
}

func TestPublishRecoveredNomadEnvironmentReady_CanceledContext(t *testing.T) {
	fx := newPublishFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	published, err := fx.repo.PublishRecoveredNomadEnvironmentReady(ctx, fx.params())
	wantPublishError(t, published, err, context.Canceled)
	fx.assertUnpublished(t)
}
