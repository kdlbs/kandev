package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	guardRaceTaskID         = "task-materialized-route-guard-race"
	guardRaceProfileA       = "profile-dynamic-race-a"
	guardRaceProfileB       = "profile-dynamic-race-b"
	guardRaceCandidateA     = "profile-candidate-race-a"
	guardRaceCandidateB     = "profile-candidate-race-b"
	guardRaceOperationStart = "materialized-route-guard-race-start"
)

type guardRaceOperationKey struct{}

type guardRaceRouteRead struct {
	state *dynamicruntime.RouteState
	err   error
}

type materializedRouteRaceRepo struct {
	*sqliterepo.Repository

	prepareRouteRead  chan guardRaceRouteRead
	prepareReadResume chan struct{}
	profileProjection chan struct{}
	startRouteRead    chan guardRaceRouteRead
	startReadResume   chan struct{}

	prepareReadOnce sync.Once
	profileSaveOnce sync.Once
	startReadOnce   sync.Once
	prepareResume   sync.Once
	startResume     sync.Once
}

func newMaterializedRouteRaceRepo(repo *sqliterepo.Repository) *materializedRouteRaceRepo {
	return &materializedRouteRaceRepo{
		Repository:        repo,
		prepareRouteRead:  make(chan guardRaceRouteRead, 1),
		prepareReadResume: make(chan struct{}),
		profileProjection: make(chan struct{}),
		startRouteRead:    make(chan guardRaceRouteRead, 1),
		startReadResume:   make(chan struct{}),
	}
}

func (r *materializedRouteRaceRepo) LoadRouteState(
	ctx context.Context,
	sessionID string,
) (*dynamicruntime.RouteState, error) {
	state, err := r.Repository.LoadRouteState(ctx, sessionID)
	if operation, _ := ctx.Value(guardRaceOperationKey{}).(string); operation == guardRaceOperationStart {
		r.startReadOnce.Do(func() {
			r.startRouteRead <- guardRaceRouteRead{state: state, err: err}
			<-r.startReadResume
		})
		return state, err
	}
	r.prepareReadOnce.Do(func() {
		r.prepareRouteRead <- guardRaceRouteRead{state: state, err: err}
		<-r.prepareReadResume
	})
	return state, err
}

func (r *materializedRouteRaceRepo) UpdateTaskSession(ctx context.Context, session *models.TaskSession) error {
	if err := r.Repository.UpdateTaskSession(ctx, session); err != nil {
		return err
	}
	if session.AgentProfileID == guardRaceProfileA && session.ExecutionProfileID == guardRaceCandidateA && session.RouteGeneration > 0 {
		r.profileSaveOnce.Do(func() { close(r.profileProjection) })
	}
	return nil
}

func (r *materializedRouteRaceRepo) releasePreparationRead() {
	r.prepareResume.Do(func() { close(r.prepareReadResume) })
}

func (r *materializedRouteRaceRepo) releaseStartRead() {
	r.startResume.Do(func() { close(r.startReadResume) })
}

func newMaterializedRouteGuardRaceService(
	t *testing.T,
) (*Service, *sqliterepo.Repository, *materializedRouteRaceRepo) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-materialized-route-race", Name: "Race", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: guardRaceTaskID, WorkspaceID: "ws-materialized-route-race", Title: "Race",
		State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now,
	}))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[guardRaceTaskID] = &v1.Task{
		ID: guardRaceTaskID, WorkspaceID: "ws-materialized-route-race", Title: "Race",
		State: v1.TaskStateCreated,
	}
	agentManager := &mockAgentManager{launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		return &executor.LaunchAgentResponse{AgentExecutionID: "execution-materialized-route-race"}, nil
	}}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	barrierRepo := newMaterializedRouteRaceRepo(repo)
	t.Cleanup(func() {
		barrierRepo.releasePreparationRead()
		barrierRepo.releaseStartRead()
	})
	svc.repo = barrierRepo
	engine := dynamicruntime.NewEngine(
		dynamicruntime.WithPersistence(barrierRepo),
		dynamicruntime.WithStateLoader(barrierRepo),
	)
	svc.profileExecutionResolver = agentruntime.NewProfileExecutionResolver(
		newMaterializedGuardRaceProfileStore(t), engine, true,
	)
	return svc, repo, barrierRepo
}

func newMaterializedGuardRaceProfileStore(t *testing.T) agentsettingsstore.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	repo, cleanup, err := agentsettingsstore.Provide(db, db, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	ctx := context.Background()
	for _, agent := range []*agentsettingsmodels.Agent{
		{ID: "dynamic-race", Name: "dynamic"},
		{ID: "candidate-race", Name: "Candidate race"},
	} {
		require.NoError(t, repo.CreateAgent(ctx, agent))
	}
	for _, profile := range []*agentsettingsmodels.AgentProfile{
		{ID: guardRaceProfileA, AgentID: "dynamic-race", Name: "Dynamic A", Enabled: true},
		{ID: guardRaceProfileB, AgentID: "dynamic-race", Name: "Dynamic B", Enabled: true},
		{ID: guardRaceCandidateA, AgentID: "candidate-race", Name: "Candidate A", Enabled: true},
		{ID: guardRaceCandidateB, AgentID: "candidate-race", Name: "Candidate B", Enabled: true},
	} {
		require.NoError(t, repo.CreateAgentProfile(ctx, profile))
	}
	for _, route := range []struct{ logicalID, candidateID string }{
		{logicalID: guardRaceProfileA, candidateID: guardRaceCandidateA},
		{logicalID: guardRaceProfileB, candidateID: guardRaceCandidateB},
	} {
		require.NoError(t, repo.CreateDynamicAgentProfile(ctx,
			&agentsettingsmodels.DynamicAgentProfile{ProfileID: route.logicalID, Version: 1},
			[]agentsettingsmodels.DynamicAgentRoute{{
				DynamicProfileID: route.logicalID, Position: 0,
				ExecutionProfileID: route.candidateID, Enabled: true,
			}},
		))
	}
	return repo
}

func awaitGuardRaceRead(t *testing.T, reads <-chan guardRaceRouteRead) guardRaceRouteRead {
	t.Helper()
	select {
	case read := <-reads:
		return read
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for route-state read")
		return guardRaceRouteRead{}
	}
}

func TestPreparedRouteMaterializationCannotRaceProfileChange(t *testing.T) {
	ctx := context.Background()
	svc, repo, barrierRepo := newMaterializedRouteGuardRaceService(t)
	sessionID, err := svc.PrepareTaskSession(ctx, guardRaceTaskID, guardRaceProfileA, "", "", "", true)
	require.NoError(t, err)
	prepareRead := awaitGuardRaceRead(t, barrierRepo.prepareRouteRead)
	require.NoError(t, prepareRead.err)
	require.Nil(t, prepareRead.state, "prepared profile A must start with no durable route")

	unlock, preparationOwnsSession := svc.tryAcquireSessionLifecycleLock(sessionID)
	if preparationOwnsSession {
		unlock()
	}
	startCtx := context.WithValue(ctx, guardRaceOperationKey{}, guardRaceOperationStart)
	startDone := make(chan error, 1)
	go func() {
		_, startErr := svc.StartCreatedSession(startCtx, guardRaceTaskID, sessionID,
			guardRaceProfileB, "continue", false, false, true, nil, nil)
		startDone <- startErr
	}()

	if preparationOwnsSession {
		// Before the fix, preparation does not own the lifecycle lock. Hold the
		// start just after its empty-route read so preparation can materialize A.
		startRead := awaitGuardRaceRead(t, barrierRepo.startRouteRead)
		require.NoError(t, startRead.err)
		require.Nil(t, startRead.state)
		barrierRepo.releasePreparationRead()
		select {
		case <-barrierRepo.profileProjection:
		case <-time.After(3 * time.Second):
			session, _ := repo.GetTaskSession(ctx, sessionID)
			route, _ := repo.LoadRouteState(ctx, sessionID)
			t.Fatalf("preparation did not persist route A: session=%#v route=%#v", session, route)
		}
		barrierRepo.releaseStartRead()
	} else {
		// With the fix, start waits for preparation's route claim and projection.
		barrierRepo.releasePreparationRead()
		select {
		case <-barrierRepo.profileProjection:
		case <-time.After(3 * time.Second):
			session, _ := repo.GetTaskSession(ctx, sessionID)
			route, _ := repo.LoadRouteState(ctx, sessionID)
			t.Fatalf("preparation did not persist route A: session=%#v route=%#v", session, route)
		}
		startRead := awaitGuardRaceRead(t, barrierRepo.startRouteRead)
		require.NoError(t, startRead.err)
		require.NotNil(t, startRead.state)
		require.Equal(t, guardRaceProfileA, startRead.state.LogicalProfileID)
		barrierRepo.releaseStartRead()
	}

	startErr := <-startDone
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	route, err := repo.LoadRouteState(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, route)
	require.Equal(t, route.LogicalProfileID, session.AgentProfileID,
		"the session label and durable route must retain the same logical profile")
	require.Equal(t, route.ExecutionProfileID, session.ExecutionProfileID,
		"the session projection must retain the durable route's concrete candidate")
	require.Equal(t, route.Generation, session.RouteGeneration,
		"the session projection must retain the durable route generation")
	if !preparationOwnsSession {
		require.ErrorIs(t, startErr, ErrMaterializedRouteProfileChange)
	}
	barrierRepo.releasePreparationRead()
	barrierRepo.releaseStartRead()
}

func TestProfileChangeBeforePreparedRouteMaterializationKeepsNewRoute(t *testing.T) {
	ctx := context.Background()
	svc, repo, barrierRepo := newMaterializedRouteGuardRaceService(t)
	const sessionID = "session-materialized-route-start-first"
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: guardRaceTaskID,
		State: models.TaskSessionStateCreated, StartedAt: now, UpdatedAt: now,
	}))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = guardRaceProfileA
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	startCtx := context.WithValue(ctx, guardRaceOperationKey{}, guardRaceOperationStart)
	startDone := make(chan error, 1)
	go func() {
		_, startErr := svc.StartCreatedSession(startCtx, guardRaceTaskID, sessionID,
			guardRaceProfileB, "continue", false, false, true, nil, nil)
		startDone <- startErr
	}()
	startRead := awaitGuardRaceRead(t, barrierRepo.startRouteRead)
	require.NoError(t, startRead.err)
	require.Nil(t, startRead.state)

	preparedDone := make(chan error, 1)
	go func() {
		_, prepareErr := svc.resolvePreparedSessionDynamicRoute(ctx, sessionID, guardRaceProfileA)
		preparedDone <- prepareErr
	}()
	barrierRepo.releaseStartRead()
	require.NoError(t, <-startDone)

	preparedRead := awaitGuardRaceRead(t, barrierRepo.prepareRouteRead)
	require.NoError(t, preparedRead.err)
	require.NotNil(t, preparedRead.state)
	require.Equal(t, guardRaceProfileB, preparedRead.state.LogicalProfileID)
	barrierRepo.releasePreparationRead()
	require.NoError(t, <-preparedDone)

	updated, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	route, err := repo.LoadRouteState(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, route)
	require.Equal(t, guardRaceProfileB, updated.AgentProfileID)
	require.Equal(t, guardRaceCandidateB, updated.ExecutionProfileID)
	require.Equal(t, route.LogicalProfileID, updated.AgentProfileID)
	require.Equal(t, route.ExecutionProfileID, updated.ExecutionProfileID)
	require.Equal(t, route.Generation, updated.RouteGeneration)
}
