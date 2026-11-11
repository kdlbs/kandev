package executor

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type inheritedTransitionIntegrationStore struct {
	*mockRepository
	database                  *tasksqlite.Repository
	readRuntimeFromDatabase   bool
	onCreatingEnvironmentRead func(string)
}

func (s *inheritedTransitionIntegrationStore) ClaimInheritedTaskEnvironmentTransition(
	ctx context.Context,
	taskID, sessionID, inheritedEnvironmentID, executorType, executorID, executorProfileID string,
) (*models.TaskEnvironment, bool, bool, error) {
	return s.database.ClaimInheritedTaskEnvironmentTransition(
		ctx, taskID, sessionID, inheritedEnvironmentID, executorType, executorID, executorProfileID,
	)
}

func (s *inheritedTransitionIntegrationStore) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	return s.database.GetTaskSession(ctx, sessionID)
}

func (s *inheritedTransitionIntegrationStore) GetExecutorRunningBySessionID(
	ctx context.Context,
	sessionID string,
) (*models.ExecutorRunning, error) {
	if s.readRuntimeFromDatabase {
		return s.database.GetExecutorRunningBySessionID(ctx, sessionID)
	}
	return s.mockRepository.GetExecutorRunningBySessionID(ctx, sessionID)
}

func (s *inheritedTransitionIntegrationStore) GetTaskEnvironment(ctx context.Context, environmentID string) (*models.TaskEnvironment, error) {
	environment, err := s.database.GetTaskEnvironment(ctx, environmentID)
	if err == nil && environment != nil && environment.Status == models.TaskEnvironmentStatusCreating && s.onCreatingEnvironmentRead != nil {
		s.onCreatingEnvironmentRead(environment.ID)
	}
	return environment, err
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.8
func TestLaunchPreparedSession_InheritedSameExecutorRaceWaitsAndAttachesToOneSQLiteWorkspace(t *testing.T) {
	ctx := context.Background()
	connection, err := kandevdb.OpenSQLite(filepath.Join(t.TempDir(), "inherited-same-executor.db"))
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(connection, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	store, err := tasksqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-same-executor", Name: "Transition"}); err != nil {
		t.Fatal(err)
	}
	parent := &models.Task{ID: "task-same-executor-parent", WorkspaceID: "workspace-same-executor", Title: "Parent"}
	child := &models.Task{ID: "task-same-executor-child", WorkspaceID: "workspace-same-executor", ParentID: parent.ID, Title: "Child"}
	for _, task := range []*models.Task{parent, child} {
		if err := store.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	foreign := &models.TaskEnvironment{
		ID: "environment-same-executor-parent", TaskID: parent.ID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/parent/workspace",
	}
	if err := store.CreateTaskEnvironment(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	sessions := []*models.TaskSession{
		{ID: "session-same-executor-owner", TaskID: child.ID, TaskEnvironmentID: foreign.ID, WorkspacePath: foreign.WorkspacePath, State: models.TaskSessionStateCreated},
		{ID: "session-same-executor-follower", TaskID: child.ID, TaskEnvironmentID: foreign.ID, WorkspacePath: foreign.WorkspacePath, State: models.TaskSessionStateCreated},
	}
	for _, session := range sessions {
		if err := store.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("create session %s: %v", session.ID, err)
		}
	}

	mock := newMockRepository()
	mock.tasks[parent.ID] = parent
	mock.tasks[child.ID] = child
	mock.executors[models.ExecutorIDLocalDocker] = &models.Executor{
		ID: models.ExecutorIDLocalDocker, Type: models.ExecutorTypeLocalDocker, Status: models.ExecutorStatusActive,
	}
	waitObserved := make(chan string, 1)
	repo := &inheritedTransitionIntegrationStore{
		mockRepository: mock,
		database:       store,
		onCreatingEnvironmentRead: func(id string) {
			select {
			case waitObserved <- id:
			default:
			}
		},
	}
	ownerEntered := make(chan *LaunchAgentRequest, 1)
	followerEntered := make(chan *LaunchAgentRequest, 1)
	launchRequests := make(chan *LaunchAgentRequest, 2)
	releaseOwner := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseOwner) }) }
	t.Cleanup(release)
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, request *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchRequests <- request
			if request.SessionID == sessions[0].ID {
				ownerEntered <- request
				<-releaseOwner
			} else {
				followerEntered <- request
			}
			return &LaunchAgentResponse{
				AgentExecutionID: "execution-" + request.SessionID,
				Status:           v1.AgentStatusStarting,
				WorkspacePath:    "/child/workspace",
			}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)
	task := &v1.Task{
		ID: child.ID, WorkspaceID: child.WorkspaceID,
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}
	type launchResult struct{ err error }
	ownerResult := make(chan launchResult, 1)
	go func() {
		_, launchErr := exec.LaunchPreparedSession(ctx, task, sessions[0].ID,
			LaunchOptions{AgentProfileID: "profile", ExecutorID: models.ExecutorIDLocalDocker})
		ownerResult <- launchResult{err: launchErr}
	}()
	var ownerRequest *LaunchAgentRequest
	select {
	case ownerRequest = <-ownerEntered:
	case outcome := <-ownerResult:
		t.Fatalf("owner launch returned before workspace allocation: %v", outcome.err)
	case <-time.After(5 * time.Second):
		t.Fatal("owner launch did not reach workspace allocation")
	}
	if ownerRequest.WorkspaceReuseRequired {
		t.Fatal("owner launch unexpectedly required reuse before materializing the canonical workspace")
	}
	canonical, err := store.GetTaskEnvironmentByTaskID(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if canonical == nil || canonical.Status != models.TaskEnvironmentStatusCreating || canonical.MaterializationSessionID != sessions[0].ID {
		t.Fatalf("child claim at allocation barrier = %+v, want owner %s", canonical, sessions[0].ID)
	}
	for {
		select {
		case <-waitObserved:
		default:
			goto ownerObservationsDrained
		}
	}

ownerObservationsDrained:

	followerResult := make(chan launchResult, 1)
	go func() {
		_, launchErr := exec.LaunchPreparedSession(ctx, task, sessions[1].ID,
			LaunchOptions{AgentProfileID: "profile", ExecutorID: models.ExecutorIDLocalDocker})
		followerResult <- launchResult{err: launchErr}
	}()
	select {
	case gotID := <-waitObserved:
		if gotID != canonical.ID {
			t.Fatalf("follower waited on environment %q, want canonical %q", gotID, canonical.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follower did not wait for the canonical owner to publish READY")
	}
	select {
	case request := <-followerEntered:
		t.Fatalf("second session reached workspace preparation before owner release: %+v", request)
	default:
	}

	release()
	for name, result := range map[string]<-chan launchResult{"owner": ownerResult, "follower": followerResult} {
		select {
		case outcome := <-result:
			if outcome.err != nil {
				t.Fatalf("%s launch failed: %v", name, outcome.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s launch did not finish after releasing the owner", name)
		}
	}
	close(launchRequests)
	var fullPreparations, attachPreparations int
	for request := range launchRequests {
		if request.TaskEnvironmentID != canonical.ID {
			t.Errorf("session %s selected environment %q, want canonical %q", request.SessionID, request.TaskEnvironmentID, canonical.ID)
		}
		if request.WorkspaceReuseRequired {
			attachPreparations++
		} else {
			fullPreparations++
		}
	}
	if fullPreparations != 1 || attachPreparations != 1 {
		t.Fatalf("workspace preparations: full=%d attach=%d, want one physical materializer and one attach-only session", fullPreparations, attachPreparations)
	}
	ready, err := store.GetTaskEnvironmentByTaskID(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready == nil || ready.ID != canonical.ID || ready.Status != models.TaskEnvironmentStatusReady || ready.WorkspacePath != "/child/workspace" {
		t.Fatalf("materialized canonical child environment = %+v, want ready env %q", ready, canonical.ID)
	}
	for _, session := range sessions {
		persisted, err := store.GetTaskSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.TaskEnvironmentID != canonical.ID || persisted.WorkspacePath != "/child/workspace" {
			t.Errorf("session %s binding = env %q path %q, want canonical %q and child path", session.ID, persisted.TaskEnvironmentID, persisted.WorkspacePath, canonical.ID)
		}
	}
	var childEnvironmentCount int
	if err := database.Get(&childEnvironmentCount, database.Rebind(`SELECT COUNT(*) FROM task_environments WHERE task_id = ?`), child.ID); err != nil {
		t.Fatal(err)
	}
	if childEnvironmentCount != 1 {
		t.Fatalf("child environment count = %d, want one canonical environment", childEnvironmentCount)
	}
	parentAfterClaim, err := store.GetTaskEnvironment(ctx, foreign.ID)
	if err != nil {
		t.Fatal(err)
	}
	if parentAfterClaim.TaskID != parent.ID || parentAfterClaim.ExecutorType != foreign.ExecutorType || parentAfterClaim.WorkspacePath != foreign.WorkspacePath {
		t.Fatalf("foreign parent environment changed during child materialization: %+v", parentAfterClaim)
	}
}

func TestLaunchPreparedSession_LiveInheritedTransitionRefusalDoesNotCommitBinding(t *testing.T) {
	for _, testCase := range []struct {
		name               string
		state              models.TaskSessionState
		startAgent         bool
		inMemoryAgentAlive bool
	}{
		{name: "agent start", state: models.TaskSessionStateRunning, startAgent: true, inMemoryAgentAlive: true},
		{name: "workspace only in-memory runtime", state: models.TaskSessionStateCreated, inMemoryAgentAlive: true},
		{name: "workspace only persisted runtime", state: models.TaskSessionStateCreated},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			connection, err := kandevdb.OpenSQLite(filepath.Join(t.TempDir(), "active-inherited-transition.db"))
			if err != nil {
				t.Fatal(err)
			}
			database := sqlx.NewDb(connection, "sqlite3")
			t.Cleanup(func() { _ = database.Close() })
			store, err := tasksqlite.NewWithDB(database, database, nil)
			if err != nil {
				t.Fatal(err)
			}
			const workspaceID = "workspace-active-inherited-transition"
			if err := store.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Active transition"}); err != nil {
				t.Fatal(err)
			}
			parent := &models.Task{ID: "task-active-transition-parent", WorkspaceID: workspaceID, Title: "Parent"}
			child := &models.Task{ID: "task-active-transition-child", WorkspaceID: workspaceID, ParentID: parent.ID, Title: "Child"}
			for _, task := range []*models.Task{parent, child} {
				if err := store.CreateTask(ctx, task); err != nil {
					t.Fatalf("create task %s: %v", task.ID, err)
				}
			}
			parentEnvironment := &models.TaskEnvironment{
				ID: "environment-active-transition-parent", TaskID: parent.ID,
				ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
				WorkspacePath: "/parent/active-transition",
			}
			if err := store.CreateTaskEnvironment(ctx, parentEnvironment); err != nil {
				t.Fatal(err)
			}
			session := &models.TaskSession{
				ID: "session-active-inherited-transition", TaskID: child.ID,
				TaskEnvironmentID: parentEnvironment.ID, WorkspacePath: parentEnvironment.WorkspacePath,
				State: testCase.state,
			}
			if err := store.CreateTaskSession(ctx, session); err != nil {
				t.Fatal(err)
			}

			mock := newMockRepository()
			mock.tasks[parent.ID] = parent
			mock.tasks[child.ID] = child
			mock.executors[models.ExecutorIDLocalDocker] = &models.Executor{
				ID: models.ExecutorIDLocalDocker, Type: models.ExecutorTypeLocalDocker, Status: models.ExecutorStatusActive,
			}
			runtime := &models.ExecutorRunning{
				ID: session.ID, SessionID: session.ID, TaskID: child.ID, AgentExecutionID: "execution-inherited",
				Status: models.ExecutorRunningStatusRunning, UpdatedAt: time.Date(2026, 11, 8, 19, 0, 0, 0, time.UTC),
				Metadata: map[string]interface{}{"task_environment_id": parentEnvironment.ID},
			}
			if err := store.UpsertExecutorRunning(ctx, runtime); err != nil {
				t.Fatalf("persist running runtime: %v", err)
			}
			repo := &inheritedTransitionIntegrationStore{
				mockRepository: mock, database: store, readRuntimeFromDatabase: true,
			}
			var agentChecks, cleanupCalls int
			runtimeActive := testCase.inMemoryAgentAlive
			agentManager := &mockAgentManager{
				isAgentRunningForSessionFunc: func(context.Context, string) bool {
					agentChecks++
					return runtimeActive
				},
				cleanupStaleExecutionIfCurrentFunc: func(context.Context, string, string, time.Time) error {
					cleanupCalls++
					return nil
				},
				launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
					t.Error("live inherited transition reached executor launch")
					return nil, nil
				},
			}
			exec := newTestExecutor(t, agentManager, repo)
			task := &v1.Task{
				ID: child.ID, WorkspaceID: child.WorkspaceID,
				Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
			}
			_, err = exec.LaunchPreparedSession(ctx, task, session.ID, LaunchOptions{
				AgentProfileID: "profile", ExecutorID: models.ExecutorIDLocalDocker, StartAgent: testCase.startAgent,
			})
			if !errors.Is(err, ErrExecutionAlreadyRunning) {
				t.Fatalf("launch error = %v, want live-agent refusal", err)
			}
			if !testCase.startAgent && agentChecks == 0 {
				t.Fatal("transition refusal did not check for a live agent")
			}
			if cleanupCalls != 0 || agentManager.cleanupStaleExecutionCallCount != 0 {
				t.Fatalf("live runtime cleanup calls = %d/%d, want zero", cleanupCalls, agentManager.cleanupStaleExecutionCallCount)
			}
			if got := agentManager.IsAgentRunningForSession(ctx, session.ID); got != testCase.inMemoryAgentAlive {
				t.Fatalf("in-memory runtime state after refusal = %v, want %v", got, testCase.inMemoryAgentAlive)
			}
			persisted, err := store.GetTaskSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if persisted.TaskEnvironmentID != parentEnvironment.ID || persisted.WorkspacePath != parentEnvironment.WorkspacePath {
				t.Fatalf("persisted session after refusal = env %q path %q, want inherited env %q path %q",
					persisted.TaskEnvironmentID, persisted.WorkspacePath, parentEnvironment.ID, parentEnvironment.WorkspacePath)
			}
			var childEnvironmentCount int
			if err := database.Get(&childEnvironmentCount, database.Rebind(`SELECT COUNT(*) FROM task_environments WHERE task_id = ?`), child.ID); err != nil {
				t.Fatal(err)
			}
			if childEnvironmentCount != 0 {
				t.Fatalf("child environment count after refusal = %d, want zero", childEnvironmentCount)
			}
			parentAfterRefusal, err := store.GetTaskEnvironment(ctx, parentEnvironment.ID)
			if err != nil {
				t.Fatal(err)
			}
			if parentAfterRefusal.TaskID != parent.ID || parentAfterRefusal.ExecutorType != parentEnvironment.ExecutorType || parentAfterRefusal.WorkspacePath != parentEnvironment.WorkspacePath {
				t.Fatalf("parent environment changed after refusal: %+v", parentAfterRefusal)
			}
			persistedRuntime, err := store.GetExecutorRunningBySessionID(ctx, session.ID)
			if err != nil {
				t.Fatalf("load persisted runtime after refusal: %v", err)
			}
			if persistedRuntime == nil || persistedRuntime.AgentExecutionID != "execution-inherited" ||
				persistedRuntime.Status != models.ExecutorRunningStatusRunning || persistedRuntime.Metadata["task_environment_id"] != parentEnvironment.ID {
				t.Fatalf("inherited runtime inventory changed after refusal: %+v", persistedRuntime)
			}
		})
	}
}

func (s *inheritedTransitionIntegrationStore) GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error) {
	return s.database.GetTaskEnvironmentByTaskID(ctx, taskID)
}

func (s *inheritedTransitionIntegrationStore) FinalizeTaskEnvironmentMaterialization(
	ctx context.Context,
	environment *models.TaskEnvironment,
	repositories []*models.TaskEnvironmentRepo,
	materializerSessionID string,
) error {
	return s.database.FinalizeTaskEnvironmentMaterialization(ctx, environment, repositories, materializerSessionID)
}

func (s *inheritedTransitionIntegrationStore) UpdateTaskSessionIfCurrentState(
	ctx context.Context,
	session *models.TaskSession,
	expected models.TaskSessionState,
) (bool, error) {
	return s.database.UpdateTaskSessionIfCurrentState(ctx, session, expected)
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.8
func TestLaunchPreparedSession_InheritedExecutorRaceUsesOneSQLiteClaim(t *testing.T) {
	ctx := context.Background()
	connection, err := kandevdb.OpenSQLite(filepath.Join(t.TempDir(), "inherited-transition.db"))
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(connection, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	store, err := tasksqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-transition", Name: "Transition"}); err != nil {
		t.Fatal(err)
	}
	parent := &models.Task{ID: "task-transition-parent", WorkspaceID: "workspace-transition", Title: "Parent"}
	child := &models.Task{ID: "task-transition-child", WorkspaceID: "workspace-transition", ParentID: parent.ID, Title: "Child"}
	for _, task := range []*models.Task{parent, child} {
		if err := store.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	foreign := &models.TaskEnvironment{
		ID: "environment-transition-parent", TaskID: parent.ID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/parent/workspace",
	}
	if err := store.CreateTaskEnvironment(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	sessions := []*models.TaskSession{
		{ID: "session-transition-docker", TaskID: child.ID, TaskEnvironmentID: foreign.ID, WorkspacePath: foreign.WorkspacePath, State: models.TaskSessionStateCreated},
		{ID: "session-transition-ssh", TaskID: child.ID, TaskEnvironmentID: foreign.ID, WorkspacePath: foreign.WorkspacePath, State: models.TaskSessionStateCreated},
	}
	for _, session := range sessions {
		if err := store.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("create session %s: %v", session.ID, err)
		}
	}

	mock := newMockRepository()
	mock.tasks[parent.ID] = parent
	mock.tasks[child.ID] = child
	mock.executors[models.ExecutorIDLocalDocker] = &models.Executor{
		ID: models.ExecutorIDLocalDocker, Type: models.ExecutorTypeLocalDocker, Status: models.ExecutorStatusActive,
	}
	mock.executors["executor-transition-ssh"] = &models.Executor{
		ID: "executor-transition-ssh", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive,
	}
	repo := &inheritedTransitionIntegrationStore{mockRepository: mock, database: store}

	entered := make(chan string, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var physicalLaunches atomic.Int32
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, request *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			physicalLaunches.Add(1)
			entered <- request.SessionID
			<-release
			return &LaunchAgentResponse{
				AgentExecutionID: "execution-" + request.SessionID,
				Status:           v1.AgentStatusStarting,
				WorkspacePath:    "/child/workspace",
			}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)
	task := &v1.Task{
		ID: child.ID, WorkspaceID: child.WorkspaceID,
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}
	type launchResult struct{ err error }
	firstResult := make(chan launchResult, 1)
	go func() {
		_, launchErr := exec.LaunchPreparedSession(ctx, task, sessions[0].ID,
			LaunchOptions{AgentProfileID: "profile", ExecutorID: models.ExecutorIDLocalDocker})
		firstResult <- launchResult{err: launchErr}
	}()

	select {
	case sessionID := <-entered:
		if sessionID != sessions[0].ID {
			t.Fatalf("first physical launch belongs to %q, want %q", sessionID, sessions[0].ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first launch did not reach the physical allocation barrier")
	}

	canonical, err := store.GetTaskEnvironmentByTaskID(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if canonical == nil || canonical.Status != models.TaskEnvironmentStatusCreating || canonical.MaterializationSessionID != sessions[0].ID {
		t.Fatalf("child claim at allocation barrier = %+v, want owner %s", canonical, sessions[0].ID)
	}
	firstSession, err := store.GetTaskSession(ctx, sessions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstSession.TaskEnvironmentID != canonical.ID || firstSession.WorkspacePath != "" {
		t.Fatalf("first session at allocation barrier = env %q path %q, want child claim %q and empty path", firstSession.TaskEnvironmentID, firstSession.WorkspacePath, canonical.ID)
	}

	secondResult := make(chan error, 1)
	go func() {
		_, launchErr := exec.LaunchPreparedSession(ctx, task, sessions[1].ID,
			LaunchOptions{AgentProfileID: "profile", ExecutorID: "executor-transition-ssh"})
		secondResult <- launchErr
	}()
	var secondErr error
	select {
	case secondErr = <-secondResult:
	case sessionID := <-entered:
		releaseOnce.Do(func() { close(release) })
		t.Fatalf("second executor reached physical allocation for session %q", sessionID)
	case <-time.After(5 * time.Second):
		t.Fatal("second executor launch did not refuse or reach the allocation barrier")
	}
	if !errors.Is(secondErr, models.ErrWorkspaceExecutorMismatch) {
		t.Fatalf("second executor launch error = %v, want clear canonical executor mismatch", secondErr)
	}
	if got := physicalLaunches.Load(); got != 1 {
		t.Fatalf("physical launches = %d before releasing canonical owner, want exactly one", got)
	}
	secondSession, err := store.GetTaskSession(ctx, sessions[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if secondSession.TaskEnvironmentID != canonical.ID || secondSession.WorkspacePath != "" {
		t.Fatalf("refused session binding = env %q path %q, want canonical child env %q and empty path", secondSession.TaskEnvironmentID, secondSession.WorkspacePath, canonical.ID)
	}
	parentAfterClaim, err := store.GetTaskEnvironment(ctx, foreign.ID)
	if err != nil {
		t.Fatal(err)
	}
	if parentAfterClaim.TaskID != parent.ID || parentAfterClaim.ExecutorType != foreign.ExecutorType || parentAfterClaim.WorkspacePath != foreign.WorkspacePath {
		t.Fatalf("foreign parent environment changed during child election: %+v", parentAfterClaim)
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case outcome := <-firstResult:
		if outcome.err != nil {
			t.Fatalf("canonical launch failed after release: %v", outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canonical launch did not finish after releasing its allocation barrier")
	}

	ready, err := store.GetTaskEnvironmentByTaskID(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready == nil || ready.ID != canonical.ID || ready.Status != models.TaskEnvironmentStatusReady || ready.WorkspacePath != "/child/workspace" {
		t.Fatalf("materialized canonical child environment = %+v, want ready env %q at child path", ready, canonical.ID)
	}
	materializerSession, err := store.GetTaskSession(ctx, sessions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if materializerSession.TaskEnvironmentID != canonical.ID || materializerSession.WorkspacePath != "/child/workspace" {
		t.Fatalf("workspace-only launch returned with session binding env %q path %q, want canonical child environment and path", materializerSession.TaskEnvironmentID, materializerSession.WorkspacePath)
	}
	var childEnvCount int
	if err := database.Get(&childEnvCount, database.Rebind(`SELECT COUNT(*) FROM task_environments WHERE task_id = ?`), child.ID); err != nil {
		t.Fatal(err)
	}
	if childEnvCount != 1 {
		t.Fatalf("child task environment count = %d, want one canonical environment", childEnvCount)
	}
}
