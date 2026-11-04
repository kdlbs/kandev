package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userservice "github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	contentionAgentProfileID  = "profile-contention"
	contentionRepositoryID    = "repo-contention"
	contentionExecutorID      = "executor-contention"
	contentionUnrelatedTaskID = "unrelated-color-task"
)

type contentionStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
}

func (g contentionStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	return g.steps[stepID], nil
}

func (g contentionStepGetter) GetNextStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func (g contentionStepGetter) GetPreviousStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func (g contentionStepGetter) GetWorkflowMeta(context.Context, string) (orchestrator.WorkflowMeta, error) {
	return orchestrator.WorkflowMeta{}, nil
}

// contentionAgentManager supplies the profile lookup used during real
// orchestrator preparation. The embedded interface keeps the remaining
// AgentManagerClient methods available without starting a provider process.
type contentionAgentManager struct {
	executor.AgentManagerClient
}

func (m *contentionAgentManager) ResolveAgentProfile(_ context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	return &executor.AgentProfileInfo{ProfileID: profileID, ProfileName: profileID}, nil
}

// contentionSchedulerRepo adapts the real task repository and service to the
// scheduler surface consumed by the real orchestrator instance.
type contentionSchedulerRepo struct {
	repo *taskrepo.Repository
	svc  *taskservice.Service
}

func (a contentionSchedulerRepo) GetTask(ctx context.Context, taskID string) (*v1.Task, error) {
	task, err := a.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, err
	}
	return task.ToAPI(), nil
}

func (a contentionSchedulerRepo) UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error {
	_, err := a.svc.UpdateTaskState(ctx, taskID, state)
	return err
}

func (a contentionSchedulerRepo) UpdateTaskStateIfCurrentIn(
	ctx context.Context, taskID string, state v1.TaskState, allowed []v1.TaskState,
) (bool, error) {
	return a.svc.UpdateTaskStateIfCurrentIn(ctx, taskID, state, allowed)
}

func (a contentionSchedulerRepo) UpdateTaskStateIfNotArchived(
	ctx context.Context, taskID string, state v1.TaskState,
) (bool, error) {
	return a.svc.UpdateTaskStateIfNotArchived(ctx, taskID, state)
}

func (a contentionSchedulerRepo) UpdateTaskStateIfSessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
) (bool, error) {
	return a.svc.UpdateTaskStateIfSessionState(ctx, taskID, sessionID, expectedSessionState, state)
}

// contentionOrchestrator delegates every preparation operation to the real
// orchestrator service and substitutes only the asynchronous provider launch.
type contentionOrchestrator struct {
	service *orchestrator.Service
}

func (o *contentionOrchestrator) LaunchSession(ctx context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	if req.Intent == orchestrator.IntentStartCreated {
		return &orchestrator.LaunchSessionResponse{
			Success: true, TaskID: req.TaskID, SessionID: req.SessionID,
			AgentProfileID: req.AgentProfileID, State: string(models.TaskSessionStateCreated),
		}, nil
	}
	return o.service.LaunchSession(ctx, req)
}

func (o *contentionOrchestrator) EnsureSession(ctx context.Context, taskID string, opts ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	return o.service.EnsureSession(ctx, taskID, opts...)
}

type contentionDurableTarget struct {
	taskID         string
	sessionID      string
	incarnationID  string
	executionID    string
	submissionID   string
	streamID       string
	payload        []byte
	startSequence  atomic.Int64
	mu             sync.Mutex
	offered        atomic.Int64
	projected      atomic.Int64
	dropped        atomic.Int64
	streamFailures atomic.Int64
	ackFailures    atomic.Int64
}

type taskCreateContentionFixture struct {
	t                testing.TB
	router           *gin.Engine
	handler          *TaskHandlers
	repo             *taskrepo.Repository
	writer           *sqlx.DB
	taskService      *taskservice.Service
	orchestrator     *orchestrator.Service
	userService      *userservice.Service
	workspaceID      string
	workflowID       string
	stepID           string
	repositoryPath   string
	sequence         atomic.Int64
	primaryCallbacks *atomic.Int64
	unrelatedTaskID  string
	measurementEnd   atomic.Int64
}

func newTaskCreateContentionFixture(tb testing.TB) *taskCreateContentionFixture {
	tb.Helper()
	gin.SetMode(gin.TestMode)
	path := filepath.Join(tb.TempDir(), "task-create-contention.db")
	writerConn, err := db.OpenSQLite(path)
	require.NoError(tb, err)
	readerConn, err := db.OpenSQLiteReader(path)
	require.NoError(tb, err)
	writer := sqlx.NewDb(writerConn, "sqlite3")
	reader := sqlx.NewDb(readerConn, "sqlite3")
	tb.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(tb, err)
	repo, cleanup, err := taskrepository.Provide(writer, reader, log)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = cleanup() })

	const (
		workspaceID = "ws-create-contention"
		workflowID  = "wf-create-contention"
		stepID      = "step-create-contention"
	)
	ctx := context.Background()
	runnerID := contentionExecutorID
	require.NoError(tb, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: workspaceID, Name: "Create contention", DefaultExecutorID: &runnerID,
	}))
	require.NoError(tb, repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Create contention"}))
	now := time.Now().UTC()
	_, err = writer.Exec(`INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		stepID, workflowID, stepID, 0, now, now)
	require.NoError(tb, err)
	require.NoError(tb, repo.CreateExecutor(ctx, &models.Executor{
		ID: contentionExecutorID, Name: "Contention local", Type: models.ExecutorTypeLocal,
		Status: models.ExecutorStatusActive, IsSystem: true,
	}))
	repositoryPath := initContentionGitRepository(tb)
	require.NoError(tb, repo.CreateRepository(ctx, &models.Repository{
		ID: contentionRepositoryID, WorkspaceID: workspaceID, Name: "contention-repo",
		SourceType: "local", LocalPath: repositoryPath, DefaultBranch: "main",
	}))
	require.NoError(tb, repo.CreateTask(ctx, &models.Task{
		ID: contentionUnrelatedTaskID, WorkspaceID: workspaceID, WorkflowID: workflowID,
		WorkflowStepID: stepID, Title: "Unrelated color task", Description: "Unrelated color task",
	}))

	eventBus := bus.NewMemoryEventBus(log)
	tb.Cleanup(eventBus.Close)
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
		RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	stepGetter := contentionStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		stepID: {ID: stepID, WorkflowID: workflowID, Name: stepID, Position: 0},
	}}
	taskSvc.SetWorkflowStepGetter(stepGetter)

	userRepo, userCleanup, err := userstore.Provide(writer, reader)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = userCleanup() })
	userSvc := userservice.NewService(userRepo, eventBus, log)

	orch := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), eventBus, &contentionAgentManager{},
		contentionSchedulerRepo{repo: repo, svc: taskSvc}, repo, nil, nil, nil, log,
	)
	orch.SetWorkflowStepGetter(stepGetter)
	orch.SetTaskEventPublisher(taskSvc)
	primaryCallbacks := &atomic.Int64{}
	orch.SetOnPrimarySessionSet(func(ctx context.Context, taskID, _ string) {
		primaryCallbacks.Add(1)
		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			return
		}
		taskSvc.PublishTaskUpdated(ctx, task)
	})

	handler := NewTaskHandlers(taskSvc, &contentionOrchestrator{service: orch}, repo, nil, log)
	handler.SetTaskCreateLastUsedRecorder(userSvc)
	router := gin.New()
	handler.registerHTTP(router)

	return &taskCreateContentionFixture{
		t: tb, router: router, handler: handler, repo: repo, writer: writer,
		taskService: taskSvc, orchestrator: orch, userService: userSvc,
		workspaceID: workspaceID, workflowID: workflowID, stepID: stepID,
		repositoryPath: repositoryPath, unrelatedTaskID: contentionUnrelatedTaskID,
		primaryCallbacks: primaryCallbacks,
	}
}

func initContentionGitRepository(tb testing.TB) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "contention-repo")
	require.NoError(tb, os.MkdirAll(path, 0o755))
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "contention@example.com"},
		{"config", "user.name", "Contention"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = path
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		output, err := cmd.CombinedOutput()
		require.NoErrorf(tb, err, "git %v failed: %s", args, output)
	}
	return path
}

func (f *taskCreateContentionFixture) createTaskHTTP(startAgent bool) (int, time.Duration, []byte) {
	sequence := f.sequence.Add(1)
	payload := map[string]interface{}{
		"workspace_id":     f.workspaceID,
		"workflow_id":      f.workflowID,
		"workflow_step_id": f.stepID,
		"title":            fmt.Sprintf("Contention task %d", sequence),
		"description":      fmt.Sprintf("Contention prompt %d", sequence),
		"start_agent":      startAgent,
		"repositories": []map[string]interface{}{
			{"repository_id": contentionRepositoryID, "base_branch": "main"},
		},
	}
	if startAgent {
		payload["agent_profile_id"] = contentionAgentProfileID
	}
	raw, err := json.Marshal(payload)
	require.NoError(f.t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	start := time.Now()
	f.router.ServeHTTP(recorder, request)
	return recorder.Code, time.Since(start), recorder.Body.Bytes()
}

// seedDurableStream creates a production-shaped session identity for durable
// agent delivery: executor running row, harness generation, and a dispatching
// prompt submission.
func (f *taskCreateContentionFixture) seedDurableStream(index int) *contentionDurableTarget {
	sequence := f.sequence.Add(1)
	suffix := fmt.Sprintf("%d-%d", index, sequence)
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "stream event"})
	require.NoError(f.t, err)
	target := &contentionDurableTarget{
		taskID:        "stream-task-" + suffix,
		sessionID:     "stream-session-" + suffix,
		incarnationID: "stream-incarnation-" + suffix,
		executionID:   "stream-execution-" + suffix,
		submissionID:  "prompt:" + suffix,
		streamID:      "stream-" + suffix,
		payload:       payload,
	}
	ctx := context.Background()
	require.NoError(f.t, f.repo.CreateTask(ctx, &models.Task{
		ID: target.taskID, WorkspaceID: f.workspaceID, WorkflowID: f.workflowID,
		WorkflowStepID: f.stepID, Title: target.taskID, Description: target.taskID,
	}))
	require.NoError(f.t, f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: target.sessionID, TaskID: target.taskID, QueueIncarnationID: target.incarnationID,
		State: models.TaskSessionStateRunning, AgentProfileID: contentionAgentProfileID,
	}))
	require.NoError(f.t, f.repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-" + suffix, SessionID: target.sessionID, TaskID: target.taskID,
		ExecutorID: contentionExecutorID, AgentExecutionID: target.executionID, Status: "running",
	}))
	now := time.Now().UTC()
	stored, err := f.repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: target.sessionID, IncarnationID: target.incarnationID, Generation: 1,
		NativeSessionID: "native-" + suffix, CreationReason: "initial", CreatedAt: now, CommittedAt: now,
	}, 0)
	require.NoError(f.t, err)
	require.True(f.t, stored)
	_, err = f.repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: target.submissionID, SessionID: target.sessionID, IncarnationID: target.incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash-" + suffix,
		Payload: []byte("prompt"), State: models.DeliverySubmissionDispatching,
	})
	require.NoError(f.t, err)
	return target
}

func (f *taskCreateContentionFixture) deliveryEvent(target *contentionDurableTarget, sequence int64) *models.AgentDeliveryEvent {
	return &models.AgentDeliveryEvent{
		SessionID: target.sessionID, IncarnationID: target.incarnationID, HarnessGeneration: 1,
		StreamID: target.streamID, Sequence: sequence, EventType: streams.EventTypeMessageChunk,
		Payload: target.payload, ReceivedAt: time.Now().UTC(), TaskID: target.taskID,
		ExecutionID: target.executionID, OwnerGeneration: 1,
		ExpectedStreamID: target.streamID, SubmissionID: target.submissionID,
	}
}

func deliveryEffects(batch []*models.AgentDeliveryEvent) []*models.AgentDeliveryEffect {
	effects := make([]*models.AgentDeliveryEffect, len(batch))
	for index, event := range batch {
		now := event.ReceivedAt
		effects[index] = &models.AgentDeliveryEffect{
			EffectKey: fmt.Sprintf("agent_delivery.event:%s:%d", event.StreamID, event.Sequence),
			StreamID:  event.StreamID, Sequence: event.Sequence, EffectType: "agent_delivery.event",
			State: models.DeliveryEffectCompleted, CreatedAt: now, CompletedAt: &now,
		}
	}
	return effects
}

func isDeliveryShutdown(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, context.Canceled)
}

func TestTaskCreateContentionFixture(t *testing.T) {
	fixture := newTaskCreateContentionFixture(t)
	t.Cleanup(func() {
		if fixture.orchestrator != nil {
			_ = fixture.orchestrator.Stop()
		}
	})
	status, _, body := fixture.createTaskHTTP(true)
	require.Equal(t, http.StatusOK, status, string(body))

	var response struct {
		TaskSessionID string `json:"session_id"`
		ID            string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	require.NotEmpty(t, response.ID)
	require.NotEmpty(t, response.TaskSessionID)

	session, err := fixture.repo.GetTaskSession(context.Background(), response.TaskSessionID)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.True(t, session.IsPrimary)
	preview, found := session.Metadata[models.SessionMetaKeyInitialPromptPreview]
	require.True(t, found)
	require.NotNil(t, preview)
	submission, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, models.InitialPromptSubmissionPending, submission.State)
	require.Equal(t, int64(1), fixture.primaryCallbacks.Load())

	taskRepos, err := fixture.repo.ListTaskRepositories(context.Background(), response.ID)
	require.NoError(t, err)
	require.Len(t, taskRepos, 1)
	require.Equal(t, contentionRepositoryID, taskRepos[0].RepositoryID)

	settings, err := fixture.userService.GetUserSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, contentionAgentProfileID, settings.TaskCreateLastUsed.AgentProfileID)

	blue := "blue"
	settings, err = fixture.userService.UpdateUserSettings(context.Background(), &userservice.UpdateUserSettingsRequest{
		SidebarTaskColorPatch: &usermodels.SidebarTaskColorPatch{Colors: map[string]*string{fixture.unrelatedTaskID: &blue}},
	})
	require.NoError(t, err)
	require.NotNil(t, settings.SidebarTaskColors[fixture.unrelatedTaskID])
	require.Equal(t, "blue", *settings.SidebarTaskColors[fixture.unrelatedTaskID])

	target := fixture.seedDurableStream(0)
	batch := []*models.AgentDeliveryEvent{fixture.deliveryEvent(target, 1)}
	_, err = fixture.repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(
		context.Background(), batch, deliveryEffects(batch), 1,
	)
	require.NoError(t, err)
	require.NoError(t, fixture.repo.AwaitAgentDeliveryDurability(context.Background()))
	cursor, err := fixture.repo.GetAgentDeliveryCursor(context.Background(), target.streamID)
	require.NoError(t, err)
	require.NotNil(t, cursor)
	require.Equal(t, int64(1), cursor.ProjectedSequence)
}
