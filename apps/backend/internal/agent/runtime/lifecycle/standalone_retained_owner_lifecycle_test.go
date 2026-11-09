package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// standaloneAdoptionExecutor models a runtime that returned a retained peer
// instead of allocating a fresh instance.
type standaloneAdoptionExecutor struct {
	MockExecutor

	mu            sync.Mutex
	lastRequest   *ExecutorCreateRequest
	lastInstance  *ExecutorInstance
	stopInstances []*ExecutorInstance
	createCount   atomic.Int32
	stopCount     atomic.Int32
	retainedID    string
	env           map[string]string
	sourceRoots   []string
	entered       chan struct{}
	barrier       chan struct{}
	returnAsFresh bool
	workspacePath string
}

func (e *standaloneAdoptionExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	e.mu.Lock()
	e.lastRequest = req
	e.mu.Unlock()
	e.createCount.Add(1)
	if e.entered != nil {
		select {
		case e.entered <- struct{}{}:
		default:
		}
	}
	if e.barrier != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-e.barrier:
		}
	}
	if e.returnAsFresh {
		instance := &ExecutorInstance{
			InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
			RuntimeName: e.Name(), WorkspacePath: req.WorkspacePath,
		}
		e.mu.Lock()
		e.lastInstance = instance
		e.mu.Unlock()
		return instance, nil
	}
	workspacePath := e.workspacePath
	if workspacePath == "" {
		workspacePath = req.WorkspacePath
	}
	instance := &ExecutorInstance{
		InstanceID:           e.retainedID,
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		RuntimeName:          e.Name(),
		StandaloneInstanceID: e.retainedID,
		StandalonePort:       41001,
		WorkspacePath:        workspacePath,
		ReusedWorkspace:      true,
		Metadata: map[string]interface{}{
			MetadataKeyReuseExistingProcess: true,
			"standalone_port":               41001,
		},
		Env:                  e.env,
		WorkspaceSourceRoots: e.sourceRoots,
	}
	e.mu.Lock()
	e.lastInstance = instance
	e.mu.Unlock()
	return instance, nil
}

func (e *standaloneAdoptionExecutor) StopInstance(_ context.Context, instance *ExecutorInstance, _ bool) error {
	e.stopCount.Add(1)
	e.mu.Lock()
	e.stopInstances = append(e.stopInstances, instance)
	e.mu.Unlock()
	return nil
}

func (e *standaloneAdoptionExecutor) request() *ExecutorCreateRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastRequest
}

func (e *standaloneAdoptionExecutor) instance() *ExecutorInstance {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastInstance
}

func (e *standaloneAdoptionExecutor) stoppedInstances() []*ExecutorInstance {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]*ExecutorInstance(nil), e.stopInstances...)
}

type persistedStandaloneWriter struct {
	*captureExecutorRunningWriter
	rows []*models.ExecutorRunning
}

func (w *persistedStandaloneWriter) ListExecutorsRunning(context.Context) ([]*models.ExecutorRunning, error) {
	return w.rows, nil
}

func newStandaloneAdoptionTestManager(t *testing.T, backend *standaloneAdoptionExecutor) *Manager {
	t.Helper()
	return newStandaloneAdoptionTestManagerWithEventBus(t, backend, &MockEventBus{})
}

func newStandaloneAdoptionTestManagerWithEventBus(t *testing.T, backend *standaloneAdoptionExecutor, eventBus bus.EventBus) *Manager {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	registry := NewExecutorRegistry(log)
	registry.Register(backend)
	mgr := NewManager(
		newTestRegistry(), eventBus, registry,
		&MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	mgr.dataDir = t.TempDir()
	cleanupManagerStopCh(t, mgr)
	return mgr
}

func launchRetainedOwner(t *testing.T, mgr *Manager, backend *standaloneAdoptionExecutor) *AgentExecution {
	t.Helper()
	execution, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		IsEphemeral:    true,
	})
	require.NoError(t, err)
	require.NotNil(t, execution)
	require.NotNil(t, backend.instance())
	return execution
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
func TestStandaloneRetainedOwnerUsesActualExecutionIdentity(t *testing.T) {
	backend := &standaloneAdoptionExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		retainedID:   "retained-execution",
		env:          map[string]string{"PEER_ENV": "retained", "KANDEV_TASK_ID": "task-1"},
		sourceRoots:  []string{"/retained/source"},
	}
	mgr := newStandaloneAdoptionTestManager(t, backend)
	execution := launchRetainedOwner(t, mgr, backend)

	require.Equal(t, "retained-execution", execution.ID)
	require.Equal(t, "retained-execution", execution.standaloneInstanceID)
	require.Equal(t, AgentStartupReattachedExisting, execution.startupDispositionSnapshot())
	require.Equal(t, AgentStartupReattachedExisting, mgr.StartupDisposition(execution.ID))
	require.Equal(t, "retained", execution.RuntimeEnvironment()["PEER_ENV"])
	require.Equal(t, []string{"/retained/source"}, execution.WorkspaceSourceRoots)

	requestedID := backend.request().InstanceID
	require.NotEmpty(t, requestedID)
	require.NotEqual(t, requestedID, execution.ID)
	_, tracked := mgr.executionStore.Get(execution.ID)
	require.True(t, tracked)
	_, trackedUnderRequested := mgr.executionStore.Get(requestedID)
	require.False(t, trackedUnderRequested, "the unused requested identity must not own the execution")

	require.NoError(t, mgr.StopAgent(context.Background(), execution.ID, false))
	stopped := backend.stoppedInstances()
	require.Len(t, stopped, 1)
	require.Equal(t, execution.ID, stopped[0].InstanceID, "Stop must use the selected execution identity")
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
func TestStandaloneRetainedAdoptionFailurePreservesPeer(t *testing.T) {
	backend := &standaloneAdoptionExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		retainedID:   "retained-execution",
	}
	mgr := newStandaloneAdoptionTestManager(t, backend)
	execution := launchRetainedOwner(t, mgr, backend)
	instance := backend.instance()

	t.Run("before execution construction", func(t *testing.T) {
		require.NoError(t, mgr.stopLaunchRuntimeOnRollback(context.Background(), backend, instance, nil))
		require.Zero(t, backend.stopCount.Load(), "a failed adoption owns only its attempt-local transport")
	})

	t.Run("after execution construction", func(t *testing.T) {
		mgr.rollbackLaunchExecution(context.Background(), backend, instance, execution, "post-construction failure")
		require.Zero(t, backend.stopCount.Load(), "a failed adoption owns only its attempt-local transport")
	})

	t.Run("after registration", func(t *testing.T) {
		mgr.rollbackRegisteredLaunch(backend, instance, execution, "registration failure")
		require.Zero(t, backend.stopCount.Load(), "registered rollback must leave the selected peer running")
		_, tracked := mgr.executionStore.Get(execution.ID)
		require.False(t, tracked)
	})

	t.Run("failed adoption stop preserves peer", func(t *testing.T) {
		stopBackend := &standaloneAdoptionExecutor{
			MockExecutor: MockExecutor{name: executor.NameStandalone},
			retainedID:   "retained-execution",
		}
		eventBus := &MockEventBusWithTracking{}
		stopManager := newStandaloneAdoptionTestManagerWithEventBus(t, stopBackend, eventBus)
		row := &models.ExecutorRunning{
			ID: "session-1", SessionID: "session-1", TaskID: "task-1",
			AgentExecutionID: "retained-execution", Runtime: agentruntime.RuntimeStandalone,
			Status: models.ExecutorRunningStatusRunning, Resumable: true, AgentctlPort: 41001,
		}
		writer := &persistedStandaloneWriter{
			captureExecutorRunningWriter: &captureExecutorRunningWriter{prior: row},
			rows:                         []*models.ExecutorRunning{row},
		}
		stopManager.SetExecutorRunningWriter(writer)
		stopExecution := launchRetainedOwner(t, stopManager, stopBackend)
		require.NotNil(t, writer.running, "launch must persist the retained execution row")

		require.NoError(t, stopManager.StopAgentWithReason(context.Background(), stopExecution.ID, StopReasonRecoverableAgentFailure, false))
		require.Zero(t, stopBackend.stopCount.Load(), "failed adoption cleanup must not stop the selected peer process")
		require.Zero(t, writer.deleteCalls, "failed adoption cleanup must preserve the authoritative runtime row")
		require.NotNil(t, writer.running)
		require.Equal(t, models.ExecutorRunningStatusRunning, writer.running.Status)
		for _, published := range eventBus.PublishedEvents {
			require.NotEqual(t, events.AgentStopped, published.Subject,
				"a surviving peer must not publish a terminal agent.stopped event")
		}
		_, tracked := stopManager.executionStore.Get(stopExecution.ID)
		require.False(t, tracked)

		require.NoError(t, stopManager.StopBySessionID(context.Background(), "session-1", false))
		require.Equal(t, int32(1), stopBackend.stopCount.Load(),
			"explicit Stop must reach the detached peer through its persisted runtime row")
		require.Equal(t, 0, writer.deleteCalls, "the stopped row must remain resumable")
		require.Equal(t, 1, writer.repairCalls)
	})
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
func TestStandaloneRetainedOwnerConcurrentAdmission(t *testing.T) {
	backend := &standaloneAdoptionExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		retainedID:   "retained-execution",
		entered:      make(chan struct{}, 1),
		barrier:      make(chan struct{}),
	}
	mgr := newStandaloneAdoptionTestManager(t, backend)

	leaderResult := make(chan error, 1)
	var leaderExecution *AgentExecution
	go func() {
		execution, err := mgr.Launch(context.Background(), &LaunchRequest{
			TaskID: "task-1", SessionID: "session-1", AgentProfileID: "profile-1", IsEphemeral: true,
		})
		leaderExecution = execution
		leaderResult <- err
	}()
	select {
	case <-backend.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("leader did not reach CreateInstance")
	}

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiterResult := make(chan error, 1)
	go func() {
		_, err := mgr.Launch(waiterCtx, &LaunchRequest{
			TaskID: "task-1", SessionID: "session-1", AgentProfileID: "profile-1", IsEphemeral: true,
		})
		waiterResult <- err
	}()
	cancelWaiter()
	close(backend.barrier)

	require.NoError(t, <-leaderResult)
	require.ErrorIs(t, <-waiterResult, context.Canceled)
	require.Equal(t, int32(1), backend.createCount.Load(), "concurrent callers must converge on one retained owner")
	require.NotNil(t, leaderExecution)
	require.Equal(t, "retained-execution", leaderExecution.ID)
	require.Equal(t, 1, len(mgr.executionStore.List()))
	require.Zero(t, backend.stopCount.Load())
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
func TestStandaloneRetainedOwnerRejectsPinnedCandidateSubstitution(t *testing.T) {
	backend := &standaloneAdoptionExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		retainedID:   "retained-execution",
	}
	mgr := newStandaloneAdoptionTestManager(t, backend)
	var allocatedID string
	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:               "task-1",
		SessionID:            "session-1",
		AgentProfileID:       "profile-1",
		IsEphemeral:          true,
		CandidateExecutionID: "candidate-execution",
		OnExecutionAllocated: func(_ context.Context, id string) error {
			allocatedID = id
			return nil
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot adopt retained execution")
	require.Equal(t, "candidate-execution", allocatedID)
	require.Zero(t, backend.stopCount.Load(), "a rejected pinned candidate must preserve the selected peer")
	require.Equal(t, 0, len(mgr.executionStore.List()))
	require.Nil(t, findExecutorRunningForTest(t, mgr, "session-1"))
}

func findExecutorRunningForTest(t *testing.T, mgr *Manager, sessionID string) *models.ExecutorRunning {
	t.Helper()
	if mgr.runningWriter == nil {
		return nil
	}
	reader, ok := mgr.runningWriter.(executorRunningReader)
	if !ok {
		return nil
	}
	running, err := reader.GetExecutorRunningBySessionID(context.Background(), sessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil
	}
	require.NoError(t, err)
	return running
}
