package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	backendmcp "github.com/kandev/kandev/internal/mcp/handlers"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

type taskExecutorDiscoveryIdentities map[string]authn.Identity

func (identities taskExecutorDiscoveryIdentities) IdentityForUser(_ context.Context, userID string) (authn.Identity, bool) {
	identity, ok := identities[userID]
	return identity, ok
}

type taskExecutorDiscoveryPrincipal struct {
	userID      string
	actorID     string
	workspaceID string
	workflowID  string
	taskID      string
	sessionID   string
}

type taskExecutorDiscoveryJourney struct {
	serverDeps *taskExecutorDiscoveryServerDeps
	service    *service.Service
	repo       *sqlite.Repository
	principals map[string]taskExecutorDiscoveryPrincipal
	identities taskExecutorDiscoveryIdentities
}

type taskExecutorDiscoveryServerDeps struct {
	dispatcher *ws.Dispatcher
	log        *logger.Logger
}

type taskExecutorDiscoveryScopedBackend struct {
	client   BackendClient
	resolver *mcpscope.Resolver
	taskID   string
	session  string
}

func (b taskExecutorDiscoveryScopedBackend) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	scoped, err := b.resolver.Scope(ctx, b.taskID)
	if err != nil {
		return err
	}
	scoped, err = b.resolver.ScopePrincipal(scoped, b.taskID, b.session)
	if err != nil {
		return err
	}
	return b.client.RequestPayload(scoped, action, payload, result)
}

type taskExecutorDiscoveryWorkspacePolicyAttacher struct{}

func (taskExecutorDiscoveryWorkspacePolicyAttacher) AttachWorkspacePolicy(context.Context, string, string, service.WorkspacePolicy) error {
	return nil
}

func newTaskExecutorDiscoveryJourney(
	t *testing.T,
	wrapExecutors func(repository.ExecutorRepository) repository.ExecutorRepository,
) *taskExecutorDiscoveryJourney {
	t.Helper()
	log := newTestLogger(t)
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "task-executor-discovery.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, err := sqlite.NewWithDB(database, database, log)
	require.NoError(t, err)
	executors := repository.ExecutorRepository(repo)
	if wrapExecutors != nil {
		executors = wrapExecutors(repo)
	}
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })
	identities := taskExecutorDiscoveryIdentities{
		"alice":       {UserID: "alice", Role: authn.RoleMember},
		"bob":         {UserID: "bob", Role: authn.RoleMember},
		"manager":     {UserID: "manager", Role: authn.RoleAdmin},
		"single-user": {UserID: "single-user", Role: authn.RoleAdmin, Synthetic: true},
	}
	taskSvc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, WorkspaceFolders: repo,
		Workflows: repo, Messages: repo, Turns: repo, Sessions: repo,
		GitSnapshots: repo, RepoEntities: repo, Executors: executors,
		Environments: repo, TaskEnvironments: repo, Reviews: repo,
	}, eventBus, log, service.RepositoryDiscoveryConfig{})
	taskSvc.SetWorkspacePolicyAttacher(taskExecutorDiscoveryWorkspacePolicyAttacher{})
	taskSvc.SetExecutorIdentityResolver(identities.IdentityForUser)

	workflowRepo, err := workflowrepo.NewWithDB(database, database, log)
	require.NoError(t, err)
	workflowSvc := workflowservice.NewService(workflowRepo, log)
	t.Cleanup(func() { require.NoError(t, workflowSvc.Close()) })
	principals := map[string]taskExecutorDiscoveryPrincipal{}
	for _, spec := range []struct {
		key     string
		ownerID string
		actorID string
	}{
		{key: "alice", ownerID: "alice", actorID: "alice"},
		{key: "manager", ownerID: "manager", actorID: "manager"},
		{key: "invalid", ownerID: "missing-user", actorID: "missing-user"},
		{key: "legacy", ownerID: "alice", actorID: "single-user"},
	} {
		principal := taskExecutorDiscoveryPrincipal{
			userID: spec.ownerID, actorID: spec.actorID,
			workspaceID: "workspace-discovery-" + spec.key,
			workflowID:  "workflow-discovery-" + spec.key,
			taskID:      "task-discovery-" + spec.key,
			sessionID:   "session-discovery-" + spec.key,
		}
		seedTaskExecutorDiscoveryPrincipal(t, repo, workflowRepo, principal)
		principals[spec.key] = principal
	}
	taskSvc.SetWorkflowStepGetter(workflowSvc)
	backendHandlers := backendmcp.NewHandlers(
		taskSvc, workflowcontroller.NewController(workflowSvc), nil, nil, nil,
		repo, repo, nil, nil, nil, nil, nil, log,
	)
	dispatcher := ws.NewDispatcher()
	backendHandlers.RegisterHandlers(dispatcher)
	return &taskExecutorDiscoveryJourney{
		serverDeps: &taskExecutorDiscoveryServerDeps{dispatcher: dispatcher, log: log},
		service:    taskSvc, repo: repo, principals: principals, identities: identities,
	}
}

func seedTaskExecutorDiscoveryPrincipal(
	t *testing.T,
	repo *sqlite.Repository,
	workflowRepo *workflowrepo.Repository,
	principal taskExecutorDiscoveryPrincipal,
) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: principal.workspaceID, Name: principal.workspaceID, OwnerID: principal.userID,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: principal.workflowID, WorkspaceID: principal.workspaceID, Name: principal.workflowID,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, workflowRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "step-" + principal.workflowID, WorkflowID: principal.workflowID, Name: "Todo", IsStartStep: true,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: principal.taskID, WorkspaceID: principal.workspaceID, WorkflowID: principal.workflowID,
		Title: "Discovery source", State: v1.TaskStateCreated, Priority: "medium",
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: principal.sessionID, TaskID: principal.taskID, ExecutorActorUserID: principal.actorID,
		AgentProfileID: "source-agent-profile", State: models.TaskSessionStateWaitingForInput,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))
}

func (j *taskExecutorDiscoveryJourney) serverFor(t *testing.T, principalKey string, authEnabled bool) *Server {
	t.Helper()
	principal := j.principals[principalKey]
	resolver := mcpscope.NewResolver(j.repo, j.identities, func() bool { return authEnabled }, j.serverDeps.log)
	backend := taskExecutorDiscoveryScopedBackend{
		client:   NewDispatcherBackendClient(j.serverDeps.dispatcher, j.serverDeps.log),
		resolver: resolver, taskID: principal.taskID, session: principal.sessionID,
	}
	return New(backend, principal.sessionID, principal.taskID, 10005, j.serverDeps.log, "", false, ModeTask)
}

func (j *taskExecutorDiscoveryJourney) call(t *testing.T, principalKey string, authEnabled bool, tool string, args map[string]interface{}) *mcplib.CallToolResult {
	t.Helper()
	ctx := context.Background()
	if !authEnabled {
		ctx = authn.WithIdentity(ctx, j.identities["single-user"])
	}
	return callToolContext(t, ctx, j.serverFor(t, principalKey, authEnabled), tool, args)
}

type taskExecutorDiscoveryResponse struct {
	Executors []struct {
		ID       string `json:"id"`
		Profiles []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"profiles"`
	} `json:"executors"`
	Total int `json:"total"`
}

func taskExecutorDiscoveryText(t *testing.T, result *mcplib.CallToolResult) string {
	t.Helper()
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	return text.Text
}

func decodeTaskExecutorDiscovery(t *testing.T, result *mcplib.CallToolResult) taskExecutorDiscoveryResponse {
	t.Helper()
	require.False(t, result.IsError, taskExecutorDiscoveryText(t, result))
	var response taskExecutorDiscoveryResponse
	require.NoError(t, json.Unmarshal([]byte(taskExecutorDiscoveryText(t, result)), &response))
	return response
}

func taskExecutorDiscoveryIndex(response taskExecutorDiscoveryResponse) map[string][]string {
	index := make(map[string][]string, len(response.Executors))
	for _, executor := range response.Executors {
		var profiles []string
		if executor.Profiles != nil {
			profiles = make([]string, 0, len(executor.Profiles))
		}
		for _, profile := range executor.Profiles {
			profiles = append(profiles, profile.ID)
		}
		index[executor.ID] = profiles
	}
	return index
}

// @covers AC-EXECUTORS-TASK-DISCOVERY-001.1, AC-EXECUTORS-TASK-DISCOVERY-001.3
func TestTaskExecutorDiscovery_CustomProfileSelection(t *testing.T) {
	journey := newTaskExecutorDiscoveryJourney(t, nil)
	ctx := context.Background()
	executor := &models.Executor{
		ID: "custom-executor-lab", Name: "Custom lab", Type: models.ExecutorTypeLocal,
		Status: models.ExecutorStatusActive, Resumable: true,
	}
	profile := &models.ExecutorProfile{
		ID: "custom-profile-alice", ExecutorID: executor.ID, Name: "Alice profile", OwnerUserID: "alice",
	}
	require.NoError(t, journey.repo.CreateExecutor(ctx, executor))
	require.NoError(t, journey.repo.CreateExecutorProfile(ctx, profile))

	server := journey.serverFor(t, "alice", true)
	listed := callTool(t, server, "list_executors_kandev", map[string]interface{}{})
	response := decodeTaskExecutorDiscovery(t, listed)
	responseIndex := taskExecutorDiscoveryIndex(response)
	require.Contains(t, responseIndex, executor.ID)
	require.Len(t, responseIndex[executor.ID], 1)
	selectedProfileID := responseIndex[executor.ID][0]
	require.Equal(t, profile.ID, selectedProfileID)

	created := callTool(t, server, "create_task_kandev", map[string]interface{}{
		"title": "Use discovered executor", "workspace_id": journey.principals["alice"].workspaceID,
		"workflow_id": journey.principals["alice"].workflowID, "agent_profile_id": "agent-profile-alice",
		"executor_profile_id": selectedProfileID, "start_agent": false,
	})
	require.False(t, created.IsError, taskExecutorDiscoveryText(t, created))
	var createResponse struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(taskExecutorDiscoveryText(t, created)), &createResponse))
	require.NotEmpty(t, createResponse.ID)
	createdTask, err := journey.service.GetTask(ctx, createResponse.ID)
	require.NoError(t, err)
	require.Equal(t, selectedProfileID, createdTask.Metadata[models.MetaKeyExecutorProfileID])

	_, err = journey.repo.DB().ExecContext(ctx,
		"UPDATE executor_profiles SET owner_user_id = ? WHERE id = ?", "bob", selectedProfileID)
	require.NoError(t, err)
	storedProfile, err := journey.repo.GetExecutorProfile(ctx, selectedProfileID)
	require.NoError(t, err)
	require.Equal(t, "bob", storedProfile.OwnerUserID)

	rejected := callTool(t, server, "create_task_kandev", map[string]interface{}{
		"title": "Stale executor choice", "workspace_id": journey.principals["alice"].workspaceID,
		"workflow_id": journey.principals["alice"].workflowID, "agent_profile_id": "agent-profile-alice",
		"executor_profile_id": selectedProfileID, "start_agent": false,
	})
	require.True(t, rejected.IsError)
	require.Contains(t, taskExecutorDiscoveryText(t, rejected), "Failed to create task")
	require.NotContains(t, taskExecutorDiscoveryText(t, rejected), selectedProfileID)
	tasks, err := journey.repo.ListTasks(ctx, journey.principals["alice"].workflowID)
	require.NoError(t, err)
	require.Len(t, tasks, 2, "a stale profile selection must not create another task")
}

// @covers AC-EXECUTORS-TASK-DISCOVERY-001.2, AC-EXECUTORS-TASK-DISCOVERY-001.5
func TestTaskExecutorDiscovery_Ownership(t *testing.T) {
	journey := newTaskExecutorDiscoveryJourney(t, nil)
	ctx := context.Background()
	aliceBaseline := decodeTaskExecutorDiscovery(t, journey.call(t, "alice", true, "list_executors_kandev", map[string]interface{}{}))
	managerBaseline := decodeTaskExecutorDiscovery(t, journey.call(t, "manager", true, "list_executors_kandev", map[string]interface{}{}))
	legacyBaseline := decodeTaskExecutorDiscovery(t, journey.call(t, "legacy", false, "list_executors_kandev", map[string]interface{}{}))
	executors := []*models.Executor{
		{ID: "executor-shared", Name: "Shared", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive},
		{ID: "executor-alice", Name: "Alice private", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive, OwnerUserID: "alice"},
		{ID: "executor-bob", Name: "Bob private", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive, OwnerUserID: "bob"},
		{ID: "executor-shared-hidden", Name: "Shared without visible profiles", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive},
	}
	profiles := []*models.ExecutorProfile{
		{ID: "profile-shared", ExecutorID: "executor-shared", Name: "Shared profile"},
		{ID: "profile-alice-on-shared", ExecutorID: "executor-shared", Name: "Alice profile", OwnerUserID: "alice"},
		{ID: "profile-bob-on-shared", ExecutorID: "executor-shared", Name: "Bob private profile", OwnerUserID: "bob", Config: map[string]string{"protected_marker": "bob-profile-secret"}},
		{ID: "profile-alice-only", ExecutorID: "executor-alice", Name: "Alice only", OwnerUserID: "alice"},
		{ID: "profile-bob-only", ExecutorID: "executor-bob", Name: "Bob only", OwnerUserID: "bob"},
		{ID: "profile-bob-hidden", ExecutorID: "executor-shared-hidden", Name: "Bob only on shared parent", OwnerUserID: "bob"},
	}
	for _, executor := range executors {
		require.NoError(t, journey.repo.CreateExecutor(ctx, executor))
	}
	for _, profile := range profiles {
		require.NoError(t, journey.repo.CreateExecutorProfile(ctx, profile))
	}

	aliceResult := journey.call(t, "alice", true, "list_executors_kandev", map[string]interface{}{})
	alice := decodeTaskExecutorDiscovery(t, aliceResult)
	aliceIndex := taskExecutorDiscoveryIndex(alice)
	require.Equal(t, aliceBaseline.Total+3, alice.Total)
	for executorID, profiles := range taskExecutorDiscoveryIndex(aliceBaseline) {
		require.ElementsMatch(t, profiles, aliceIndex[executorID])
	}
	require.Contains(t, aliceIndex, "executor-shared")
	require.Contains(t, aliceIndex, "executor-alice")
	require.Contains(t, aliceIndex, "executor-shared-hidden")
	require.NotContains(t, aliceIndex, "executor-bob")
	require.ElementsMatch(t, []string{"profile-shared", "profile-alice-on-shared"}, aliceIndex["executor-shared"])
	require.ElementsMatch(t, []string{"profile-alice-only"}, aliceIndex["executor-alice"])
	require.Nil(t, aliceIndex["executor-shared-hidden"], "a visible executor remains listed when all its profiles are hidden")
	require.NotContains(t, taskExecutorDiscoveryText(t, aliceResult), "profile-bob-on-shared")
	require.NotContains(t, taskExecutorDiscoveryText(t, aliceResult), "bob-profile-secret")
	require.NotContains(t, taskExecutorDiscoveryText(t, aliceResult), "executor-bob")

	managerResult := journey.call(t, "manager", true, "list_executors_kandev", map[string]interface{}{})
	manager := decodeTaskExecutorDiscovery(t, managerResult)
	managerIndex := taskExecutorDiscoveryIndex(manager)
	require.Equal(t, managerBaseline.Total+4, manager.Total)
	for executorID, profiles := range taskExecutorDiscoveryIndex(managerBaseline) {
		require.ElementsMatch(t, profiles, managerIndex[executorID])
	}
	require.Contains(t, managerIndex, "executor-shared")
	require.Contains(t, managerIndex, "executor-alice")
	require.Contains(t, managerIndex, "executor-bob")
	require.Contains(t, managerIndex, "executor-shared-hidden")
	require.ElementsMatch(t, []string{"profile-shared", "profile-alice-on-shared", "profile-bob-on-shared"}, managerIndex["executor-shared"])
	require.ElementsMatch(t, []string{"profile-bob-only"}, managerIndex["executor-bob"])
	require.ElementsMatch(t, []string{"profile-bob-hidden"}, managerIndex["executor-shared-hidden"])

	legacyResult := journey.call(t, "legacy", false, "list_executors_kandev", map[string]interface{}{})
	legacy := decodeTaskExecutorDiscovery(t, legacyResult)
	require.Equal(t, legacyBaseline.Total+4, legacy.Total, "authentication-disabled synthetic identity retains single-user visibility")
	require.Contains(t, taskExecutorDiscoveryText(t, legacyResult), "profile-bob-on-shared")

	invalidResult := journey.call(t, "invalid", true, "list_executors_kandev", map[string]interface{}{})
	require.True(t, invalidResult.IsError)
	invalidText := taskExecutorDiscoveryText(t, invalidResult)
	require.NotContains(t, invalidText, "executor-shared")
	require.NotContains(t, invalidText, "profile-bob-on-shared")
}

type taskExecutorDiscoveryFailureRepository struct {
	repository.ExecutorRepository
	listExecutorsErr error
	listProfilesErr  error
}

type taskExecutorDiscoveryEmptyRepository struct {
	repository.ExecutorRepository
}

func (taskExecutorDiscoveryEmptyRepository) ListExecutors(context.Context) ([]*models.Executor, error) {
	return []*models.Executor{}, nil
}

func (taskExecutorDiscoveryEmptyRepository) ListAllExecutorProfiles(context.Context) ([]*models.ExecutorProfile, error) {
	return []*models.ExecutorProfile{}, nil
}

func (r taskExecutorDiscoveryFailureRepository) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	if r.listExecutorsErr != nil {
		return nil, r.listExecutorsErr
	}
	return r.ExecutorRepository.ListExecutors(ctx)
}

func (r taskExecutorDiscoveryFailureRepository) ListAllExecutorProfiles(ctx context.Context) ([]*models.ExecutorProfile, error) {
	if r.listProfilesErr != nil {
		return nil, r.listProfilesErr
	}
	return r.ExecutorRepository.ListAllExecutorProfiles(ctx)
}

// @covers AC-EXECUTORS-TASK-DISCOVERY-001.6
func TestTaskExecutorDiscovery_EmptyAndErrors(t *testing.T) {
	emptyJourney := newTaskExecutorDiscoveryJourney(t, func(base repository.ExecutorRepository) repository.ExecutorRepository {
		return taskExecutorDiscoveryEmptyRepository{ExecutorRepository: base}
	})
	emptyResult := emptyJourney.call(t, "alice", true, "list_executors_kandev", map[string]interface{}{})
	empty := decodeTaskExecutorDiscovery(t, emptyResult)
	require.Equal(t, 0, empty.Total)
	require.NotNil(t, empty.Executors)
	require.Empty(t, empty.Executors)

	for _, testCase := range []struct {
		name        string
		listErr     error
		profilesErr error
		wantError   string
	}{
		{name: "executor read", listErr: errors.New("executor store unavailable"), wantError: "Failed to list executors"},
		{name: "profile read", profilesErr: errors.New("profile store unavailable"), wantError: "Failed to list executor profiles"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			journey := newTaskExecutorDiscoveryJourney(t, func(base repository.ExecutorRepository) repository.ExecutorRepository {
				return taskExecutorDiscoveryFailureRepository{
					ExecutorRepository: base, listExecutorsErr: testCase.listErr, listProfilesErr: testCase.profilesErr,
				}
			})
			require.NoError(t, journey.repo.CreateExecutor(context.Background(), &models.Executor{
				ID: "executor-no-partial-result", Name: "Must not leak", Type: models.ExecutorTypeLocal,
				Status: models.ExecutorStatusActive,
			}))
			require.NoError(t, journey.repo.CreateExecutorProfile(context.Background(), &models.ExecutorProfile{
				ID: "profile-no-partial-result", ExecutorID: "executor-no-partial-result", Name: "Must not leak",
			}))
			result := journey.call(t, "alice", true, "list_executors_kandev", map[string]interface{}{})
			require.True(t, result.IsError)
			text := taskExecutorDiscoveryText(t, result)
			require.Contains(t, text, testCase.wantError)
			require.NotContains(t, text, "executor-no-partial-result")
			require.NotContains(t, text, "profile-no-partial-result")
		})
	}
}
