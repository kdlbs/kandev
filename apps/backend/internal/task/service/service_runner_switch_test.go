package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orgunit"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/worktree"
)

// fakeCloneURLProber is a test double for ExecutorCapabilityProber: it
// answers RequiresCloneURL with a fixed value regardless of executor type.
type fakeCloneURLProber struct{ requires bool }

func (f fakeCloneURLProber) RequiresCloneURL(string) bool { return f.requires }

// newRunnerSwitchTestService builds a fully-wired *Service on real SQLite
// repositories, including the two dependencies SwitchTaskRunner needs beyond
// createTestService's set: WorkspaceFolders (required for the mutability
// batch to evaluate rather than degrade) and the office-owned workspace-group
// membership reader. Kept in its own file/helper rather than widening the
// shared createTestService, whose many existing callers do not need either.
func newRunnerSwitchTestService(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository, *officesqlite.Repository) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	writerConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database (writer): %v", err)
	}
	readerConn, err := db.OpenSQLiteReader(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database (reader): %v", err)
	}
	writerDB := sqlx.NewDb(writerConn, "sqlite3")
	readerDB := sqlx.NewDb(readerConn, "sqlite3")
	// SwitchTaskRunner's mutability gate calls the office-owned
	// GroupMembershipChecker from inside the task repository's open write
	// transaction (after the row lock). That read must land on a separate
	// connection pool, exactly like production's writer/reader split: a
	// single shared handle (as most other tests in this package use) forces
	// the nested read to wait for the very transaction it is called from,
	// since the writer pool allows only one open connection.
	repo, cleanup, err := repository.Provide(writerDB, readerDB, nil)
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	if _, err := worktree.NewSQLiteStore(writerDB, readerDB); err != nil {
		t.Fatalf("failed to init worktree store: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(writerDB, readerDB, nil)
	if err != nil {
		t.Fatalf("failed to apply office migrations: %v", err)
	}
	if _, err := workflowrepo.NewWithDB(writerDB, readerDB, nil); err != nil {
		t.Fatalf("failed to initialize workflow repository: %v", err)
	}
	t.Cleanup(func() {
		if err := writerDB.Close(); err != nil {
			t.Errorf("failed to close writer pool: %v", err)
		}
		if err := readerDB.Close(); err != nil {
			t.Errorf("failed to close reader pool: %v", err)
		}
		if err := cleanup(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	})
	eventBus := NewMockEventBus()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewService(Repos{
		Workspaces:        repo,
		Tasks:             repo,
		TaskRepos:         repo,
		WorkspaceFolders:  repo,
		Workflows:         repo,
		Messages:          repo,
		Turns:             repo,
		Sessions:          repo,
		GitSnapshots:      repo,
		RepoEntities:      repo,
		RepositorySets:    repo,
		BranchPolicies:    repo,
		RepositoryCleanup: repo,
		Executors:         repo,
		Environments:      repo,
		TaskEnvironments:  repo,
		Reviews:           repo,
		ResourceCleanups:  repo,
		Usage:             repo,
	}, eventBus, log, RepositoryDiscoveryConfig{})
	svc.SetWorkspaceBootstrapper(repo)
	svc.SetWorkspaceGroupMembershipReader(officeRepo)
	unitStore, unitErr := orgunit.NewStore(db.NewPool(writerDB, readerDB))
	if unitErr != nil {
		t.Fatalf("failed to init unit store: %v", unitErr)
	}
	unitSvc := orgunit.NewService(unitStore, nil)
	unitSvc.SetWorkspaceCounter(repo)
	svc.SetUnitPlacer(unitSvc)
	svc.SetUnitReach(unitSvc)
	testUnits[t.Name()] = unitSvc
	t.Cleanup(func() { delete(testUnits, t.Name()) })
	svc.SetWorkflowStepGetter(&testWorkflowStepGetter{repo: repo})
	if err := svc.StartTaskResourceCleanupWorker(context.Background()); err != nil {
		t.Fatalf("failed to start task resource cleanup worker: %v", err)
	}
	t.Cleanup(svc.StopTaskResourceCleanupWorker)
	return svc, eventBus, repo, officeRepo
}

// seedRunnerSwitchTask creates a workspace, workflow/step, one repository and
// one task attached to it, returning the created task. Each call site uses
// its own fresh newRunnerSwitchTestService database, so the fixed IDs below
// never collide across tests.
func seedRunnerSwitchTask(t *testing.T, svc *Service, repo *sqliterepo.Repository) *models.Task {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-runner", Name: "Runner WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-runner", WorkspaceID: "ws-runner", Name: "Board"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-runner-a", WorkspaceID: "ws-runner", Name: "repo-a", DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-runner", WorkflowID: "wf-runner", WorkflowStepID: "step-1",
		Title:        "Runner switch task",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-runner-a", BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return result.Task
}

// seedRunnerSwitchExecutor creates an executor and a profile pointing at it,
// returning the profile ID.
func seedRunnerSwitchExecutor(t *testing.T, repo *sqliterepo.Repository, executorID, profileID string, status models.ExecutorStatus) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID: executorID, Name: executorID, Type: models.ExecutorTypeLocal, Status: status,
	}); err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: profileID, ExecutorID: executorID, Name: profileID,
	}); err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}
}

func TestSwitchTaskRunner_MalformedRequestRejectedBeforeAnyLookup(t *testing.T) {
	svc, _, _, _ := newRunnerSwitchTestService(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		taskID  string
		profile string
	}{
		{"empty id", "", "profile-1"},
		{"whitespace id", "   ", "profile-1"},
		{"empty profile", "task-1", ""},
		{"whitespace profile", "task-1", "\t\n"},
		{"both empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.SwitchTaskRunner(ctx, tc.taskID, tc.profile); !errors.Is(err, ErrRunnerSwitchMalformed) {
				t.Fatalf("SwitchTaskRunner(%q, %q) = %v, want ErrRunnerSwitchMalformed", tc.taskID, tc.profile, err)
			}
		})
	}
}

// TestSwitchTaskRunner_TaskNotFoundEvenForUnscopedCaller pins the deliberate
// GetTask-first ordering: authorizeTaskScope alone would skip its own
// existence check for an unscoped (internal) caller, so without the explicit
// pre-check a not-found task would fall through to the target/mutability
// stages instead of failing at stage 2.
func TestSwitchTaskRunner_TaskNotFoundEvenForUnscopedCaller(t *testing.T) {
	svc, _, _, _ := newRunnerSwitchTestService(t)
	ctx := context.Background()

	_, err := svc.SwitchTaskRunner(ctx, "task-does-not-exist", "profile-does-not-exist-either")
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("SwitchTaskRunner on missing task = %v, want ErrTaskNotFound", err)
	}
}

func TestSwitchTaskRunner_ForbiddenForViewerRole(t *testing.T) {
	svc, _, repo, _ := newRunnerSwitchTestService(t)
	seedTeamWorkspace(t, repo, true)
	seedUnitViewer(t, "user-carla")

	viewer := ctxAsRole("user-carla", authn.RoleMember)
	_, err := svc.SwitchTaskRunner(viewer, "task-team", "any-profile")
	if !IsForbidden(err) {
		t.Fatalf("SwitchTaskRunner as viewer = %v, want ErrForbidden", err)
	}
}

func TestSwitchTaskRunner_TargetProfileMissing(t *testing.T) {
	svc, _, repo, _ := newRunnerSwitchTestService(t)
	task := seedRunnerSwitchTask(t, svc, repo)

	_, err := svc.SwitchTaskRunner(context.Background(), task.ID, "profile-does-not-exist")
	if !errors.Is(err, ErrExecutorProfileInvalid) {
		t.Fatalf("SwitchTaskRunner with missing profile = %v, want ErrExecutorProfileInvalid", err)
	}
}

func TestSwitchTaskRunner_TargetExecutorNotActive(t *testing.T) {
	svc, _, repo, _ := newRunnerSwitchTestService(t)
	task := seedRunnerSwitchTask(t, svc, repo)
	seedRunnerSwitchExecutor(t, repo, "executor-disabled", "profile-disabled", models.ExecutorStatusDisabled)

	_, err := svc.SwitchTaskRunner(context.Background(), task.ID, "profile-disabled")
	if !errors.Is(err, ErrExecutorProfileInvalid) {
		t.Fatalf("SwitchTaskRunner with disabled executor = %v, want ErrExecutorProfileInvalid", err)
	}
}

func TestSwitchTaskRunner_MutabilityGateBlocksWhenSessionExists(t *testing.T) {
	svc, _, repo, _ := newRunnerSwitchTestService(t)
	task := seedRunnerSwitchTask(t, svc, repo)
	seedRunnerSwitchExecutor(t, repo, "executor-active", "profile-active", models.ExecutorStatusActive)
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "sess-runner-1", TaskID: task.ID, State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	_, err := svc.SwitchTaskRunner(context.Background(), task.ID, "profile-active")
	var conflict *repoerrors.ErrRunnerMutabilityConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("SwitchTaskRunner with an active session = %v, want ErrRunnerMutabilityConflict", err)
	}
	if conflict.Reason != models.RunnerReasonSessionExists {
		t.Fatalf("conflict reason = %q, want %q", conflict.Reason, models.RunnerReasonSessionExists)
	}
}

func TestSwitchTaskRunner_CompatibilityGateBlocksWhenNoCloneURLResolves(t *testing.T) {
	svc, _, repo, _ := newRunnerSwitchTestService(t)
	svc.SetExecutorCapabilityProber(fakeCloneURLProber{requires: true})
	task := seedRunnerSwitchTask(t, svc, repo)
	seedRunnerSwitchExecutor(t, repo, "executor-remote", "profile-remote", models.ExecutorStatusActive)

	_, err := svc.SwitchTaskRunner(context.Background(), task.ID, "profile-remote")
	if !errors.Is(err, repoerrors.ErrRunnerCompatibilityConflict) {
		t.Fatalf("SwitchTaskRunner with no resolvable clone URL = %v, want ErrRunnerCompatibilityConflict", err)
	}
}

func TestSwitchTaskRunner_NoOpSuccessWhenAlreadyAssigned(t *testing.T) {
	svc, eventBus, repo, _ := newRunnerSwitchTestService(t)
	task := seedRunnerSwitchTask(t, svc, repo)
	seedRunnerSwitchExecutor(t, repo, "executor-active-2", "profile-already-set", models.ExecutorStatusActive)
	if err := repo.SetTaskMetadataKey(context.Background(), task.ID, models.MetaKeyExecutorProfileID, "profile-already-set"); err != nil {
		t.Fatalf("SetTaskMetadataKey: %v", err)
	}
	eventBus.ClearEvents()

	updated, err := svc.SwitchTaskRunner(context.Background(), task.ID, "profile-already-set")
	if err != nil {
		t.Fatalf("SwitchTaskRunner no-op = %v, want success", err)
	}
	if got, _ := updated.Metadata[models.MetaKeyExecutorProfileID].(string); got != "profile-already-set" {
		t.Fatalf("metadata executor_profile_id = %q, want unchanged", got)
	}
	if eventBusHasType(eventBus, events.TaskUpdated) {
		t.Fatal("no-op switch published task.updated, want none")
	}
}

func TestSwitchTaskRunner_SuccessAssignsAndPublishesEvent(t *testing.T) {
	svc, eventBus, repo, _ := newRunnerSwitchTestService(t)
	task := seedRunnerSwitchTask(t, svc, repo)
	seedRunnerSwitchExecutor(t, repo, "executor-active-3", "profile-target", models.ExecutorStatusActive)
	eventBus.ClearEvents()

	updated, err := svc.SwitchTaskRunner(context.Background(), task.ID, "profile-target")
	if err != nil {
		t.Fatalf("SwitchTaskRunner = %v, want success", err)
	}
	if got, _ := updated.Metadata[models.MetaKeyExecutorProfileID].(string); got != "profile-target" {
		t.Fatalf("metadata executor_profile_id = %q, want %q", got, "profile-target")
	}
	if !eventBusHasType(eventBus, events.TaskUpdated) {
		t.Fatal("successful switch did not publish task.updated")
	}

	reloaded, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask after switch: %v", err)
	}
	if got, _ := reloaded.Metadata[models.MetaKeyExecutorProfileID].(string); got != "profile-target" {
		t.Fatalf("persisted executor_profile_id = %q, want %q", got, "profile-target")
	}
}

// TestSwitchTaskRunner_NoSessionEnvironmentOrExecutorSideEffects pins
// AC-003.2: a runner switch changes only the task's stored
// executor_profile_id metadata. It must not create a session, an
// environment, an executors_running row, or a workspace folder as a side
// effect of the write.
func TestSwitchTaskRunner_NoSessionEnvironmentOrExecutorSideEffects(t *testing.T) {
	svc, _, repo, _ := newRunnerSwitchTestService(t)
	task := seedRunnerSwitchTask(t, svc, repo)
	seedRunnerSwitchExecutor(t, repo, "executor-active-4", "profile-target-2", models.ExecutorStatusActive)

	if _, err := svc.SwitchTaskRunner(context.Background(), task.ID, "profile-target-2"); err != nil {
		t.Fatalf("SwitchTaskRunner = %v, want success", err)
	}

	sessions, err := repo.ListTaskSessions(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions after switch = %d, want 0", len(sessions))
	}

	env, err := repo.GetTaskEnvironmentByTaskID(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTaskEnvironmentByTaskID: %v", err)
	}
	if env != nil {
		t.Fatalf("task environment after switch = %+v, want none", env)
	}

	running, err := repo.ListExecutorsRunningByTaskID(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("ListExecutorsRunningByTaskID: %v", err)
	}
	if len(running) != 0 {
		t.Fatalf("executors_running after switch = %d, want 0", len(running))
	}

	folders, err := repo.ListTaskWorkspaceFolders(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("ListTaskWorkspaceFolders: %v", err)
	}
	if len(folders) != 0 {
		t.Fatalf("workspace folders after switch = %d, want 0", len(folders))
	}
}

// TestSwitchTaskRunner_EventPayloadCarriesRunnerMutabilityFields pins
// AC-003.3: the published task.updated event's payload must carry the
// task's real runner_editable/runner_ineligible_reason values, not merely
// exist as an event of the right type.
func TestSwitchTaskRunner_EventPayloadCarriesRunnerMutabilityFields(t *testing.T) {
	svc, eventBus, repo, _ := newRunnerSwitchTestService(t)
	task := seedRunnerSwitchTask(t, svc, repo)
	seedRunnerSwitchExecutor(t, repo, "executor-active-5", "profile-target-3", models.ExecutorStatusActive)
	eventBus.ClearEvents()

	if _, err := svc.SwitchTaskRunner(context.Background(), task.ID, "profile-target-3"); err != nil {
		t.Fatalf("SwitchTaskRunner = %v, want success", err)
	}

	var payload map[string]interface{}
	for _, evt := range eventBus.GetPublishedEvents() {
		if evt.Type != events.TaskUpdated {
			continue
		}
		data, ok := evt.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("task.updated event Data = %T, want map[string]interface{}", evt.Data)
		}
		payload = data
	}
	if payload == nil {
		t.Fatal("no task.updated event published")
	}
	editable, ok := payload["runner_editable"].(bool)
	if !ok || !editable {
		t.Fatalf("runner_editable = %v, want true", payload["runner_editable"])
	}
	if reason, _ := payload["runner_ineligible_reason"].(string); reason != models.RunnerReasonEligible {
		t.Fatalf("runner_ineligible_reason = %q, want %q", reason, models.RunnerReasonEligible)
	}
}

// TestReplaceTaskRepositoriesBypassesRunnerMutabilityGate pins F19's accepted
// risk: replaceTaskRepositories (reached through UpdateTask's Repositories
// field) is a third writer of task_repositories that does not consult the
// runner-mutability gate the way SwitchTaskRunner does. The same task that
// SwitchTaskRunner refuses to touch because a session already exists still
// accepts a repository replacement through UpdateTask. This is deliberately
// NOT a bug fix target — the assertion exists so a future change to either
// path is a conscious decision, not a silent behavior drift.
func TestReplaceTaskRepositoriesBypassesRunnerMutabilityGate(t *testing.T) {
	svc, _, repo, _ := newRunnerSwitchTestService(t)
	task := seedRunnerSwitchTask(t, svc, repo)
	seedRunnerSwitchExecutor(t, repo, "executor-active-4", "profile-f19", models.ExecutorStatusActive)
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "sess-runner-f19", TaskID: task.ID, State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	// Confirm the task is genuinely runner-immutable right now.
	if _, err := svc.SwitchTaskRunner(context.Background(), task.ID, "profile-f19"); err == nil {
		t.Fatal("expected SwitchTaskRunner to be blocked by the session, got success")
	}

	if err := repo.CreateRepository(context.Background(), &models.Repository{
		ID: "repo-runner-b", WorkspaceID: "ws-runner", Name: "repo-b", DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	setBranchUpdateWorkflowStep(svc)
	if _, err := svc.UpdateTask(context.Background(), task.ID, &UpdateTaskRequest{
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-runner-b", BaseBranch: "main"}},
	}); err != nil {
		t.Fatalf("UpdateTask replacing repositories on a materialized task = %v, want success (F19 pin)", err)
	}

	rows, err := repo.ListTaskRepositories(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(rows) != 1 || rows[0].RepositoryID != "repo-runner-b" {
		t.Fatalf("task repositories = %#v, want only repo-runner-b (replacement went through unguarded)", rows)
	}
}
