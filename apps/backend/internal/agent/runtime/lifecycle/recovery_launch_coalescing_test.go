package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

type coalescedRecoveryWaitContext struct {
	context.Context
	onWait func()
	once   sync.Once
}

func (c *coalescedRecoveryWaitContext) Done() <-chan struct{} {
	c.once.Do(c.onWait)
	return c.Context.Done()
}

type launchWaitNotifyingContext struct {
	context.Context
	waits chan<- struct{}
}

func (c *launchWaitNotifyingContext) Done() <-chan struct{} {
	c.waits <- struct{}{}
	return c.Context.Done()
}

type launchRetryGateContext struct {
	context.Context
	errCalls    atomic.Int32
	gates       map[int32]chan struct{}
	gateEntered chan<- int32
	waits       chan<- struct{}
}

func (c *launchRetryGateContext) Err() error {
	call := c.errCalls.Add(1)
	if gate, ok := c.gates[call]; ok {
		c.gateEntered <- call
		<-gate
	}
	return c.Context.Err()
}

func (c *launchRetryGateContext) Done() <-chan struct{} {
	c.waits <- struct{}{}
	return c.Context.Done()
}

type blockedLaunchWorkspaceInfoProvider struct {
	info        *WorkspaceInfo
	entered     chan struct{}
	release     chan struct{}
	calls       atomic.Int32
	releaseOnce sync.Once
}

func (p *blockedLaunchWorkspaceInfoProvider) releaseRead() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func (p *blockedLaunchWorkspaceInfoProvider) GetWorkspaceInfoForSession(
	ctx context.Context,
	_, _ string,
) (*WorkspaceInfo, error) {
	if p.calls.Add(1) == 1 {
		close(p.entered)
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	info := *p.info
	return &info, nil
}

func (p *blockedLaunchWorkspaceInfoProvider) GetWorkspaceInfoForEnvironment(
	context.Context,
	string,
) (*WorkspaceInfo, error) {
	return nil, nil
}

type sequencedLaunchWorkspaceInfoProvider struct {
	info    *WorkspaceInfo
	entered chan int
	release chan struct{}
	calls   atomic.Int32
}

func (p *sequencedLaunchWorkspaceInfoProvider) GetWorkspaceInfoForSession(
	ctx context.Context,
	_, _ string,
) (*WorkspaceInfo, error) {
	call := int(p.calls.Add(1))
	if call <= 3 {
		p.entered <- call
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	info := *p.info
	return &info, nil
}

func (p *sequencedLaunchWorkspaceInfoProvider) GetWorkspaceInfoForEnvironment(
	context.Context,
	string,
) (*WorkspaceInfo, error) {
	return nil, nil
}

func newFreshLaunchWorkspaceRaceFixture(
	t *testing.T,
	resolver ProfileResolver,
) (*Manager, *createInstanceExecutor, *blockedLaunchWorkspaceInfoProvider) {
	t.Helper()
	const (
		taskID        = "task-fresh-launch-race"
		sessionID     = "session-fresh-launch-race"
		environmentID = "env-fresh-launch-race"
	)
	provider := &blockedLaunchWorkspaceInfoProvider{
		info: &WorkspaceInfo{
			TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
			WorkspacePath: "/workspace/task", ExecutorType: string(models.ExecutorTypeWorktree),
			AgentID: "auggie",
		},
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	mgr, backend := newEnvironmentExecutionTestManagerWithProfileResolver(t, provider, resolver)
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{
		session: &models.TaskSession{
			ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
			State: models.TaskSessionStateCreated,
		},
		env: &models.TaskEnvironment{
			ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
			Status: models.TaskEnvironmentStatusCreating,
		},
	})
	mgr.preparerRegistry = NewPreparerRegistry(mgr.logger)
	mgr.preparerRegistry.Register(models.ExecutorTypeWorktree, &progressPreparer{})
	backend.entered = make(chan struct{}, 1)
	backend.barrier = make(chan struct{})
	return mgr, backend, provider
}

func newFreshLaunchWorkspaceRaceManager(
	t *testing.T,
	resolver ProfileResolver,
	provider WorkspaceInfoProvider,
) (*Manager, *createInstanceExecutor) {
	t.Helper()
	mgr, backend := newEnvironmentExecutionTestManagerWithProfileResolver(t, provider, resolver)
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{
		session: &models.TaskSession{
			ID: "session-fresh-launch-race", TaskID: "task-fresh-launch-race",
			TaskEnvironmentID: "env-fresh-launch-race", State: models.TaskSessionStateCreated,
		},
		env: &models.TaskEnvironment{
			ID: "env-fresh-launch-race", TaskID: "task-fresh-launch-race",
			ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusCreating,
		},
	})
	mgr.preparerRegistry = NewPreparerRegistry(mgr.logger)
	mgr.preparerRegistry.Register(models.ExecutorTypeWorktree, &progressPreparer{})
	backend.entered = make(chan struct{}, 1)
	backend.barrier = make(chan struct{})
	return mgr, backend
}

func freshWorktreeLaunchRequest() *LaunchRequest {
	return &LaunchRequest{
		TaskID: "task-fresh-launch-race", SessionID: "session-fresh-launch-race",
		TaskEnvironmentID: "env-fresh-launch-race", AgentProfileID: "profile-1",
		ExecutorType: string(models.ExecutorTypeWorktree), RepositoryPath: "/tmp/repo",
		UseWorktree: true,
	}
}

func TestFreshLaunchContinuesAfterWorkspaceReadNotReady(t *testing.T) {
	t.Run("three workspace refusals do not exhaust launch retries", func(t *testing.T) {
		resolver := &countingProfileResolver{info: &AgentProfileInfo{
			ProfileID: "profile-1", AgentName: "auggie",
		}}
		provider := &sequencedLaunchWorkspaceInfoProvider{
			info: &WorkspaceInfo{
				TaskID: "task-fresh-launch-race", SessionID: "session-fresh-launch-race",
				TaskEnvironmentID: "env-fresh-launch-race", WorkspacePath: "/workspace/task",
				ExecutorType: string(models.ExecutorTypeWorktree), AgentID: "auggie",
			},
			entered: make(chan int, 3), release: make(chan struct{}, 3),
		}
		mgr, backend := newFreshLaunchWorkspaceRaceManager(t, resolver, provider)
		var releaseRuntime sync.Once
		defer releaseRuntime.Do(func() { close(backend.barrier) })
		defer func() {
			for range 3 {
				provider.release <- struct{}{}
			}
		}()

		waits := make(chan struct{}, 8)
		gateEntered := make(chan int32, 2)
		gate5, gate9 := make(chan struct{}), make(chan struct{})
		launchCtx := &launchRetryGateContext{
			Context: context.Background(), waits: waits, gateEntered: gateEntered,
			gates: map[int32]chan struct{}{5: gate5, 9: gate9},
		}
		launchStarted := make(chan struct{})
		type launchResult struct {
			execution *AgentExecution
			err       error
		}
		launchResultCh := make(chan launchResult, 1)
		go func() {
			<-launchStarted
			execution, err := mgr.coalesceSessionLaunch(launchCtx, freshWorktreeLaunchRequest())
			if execution == nil {
				launchResultCh <- launchResult{err: err}
				return
			}
			launchResultCh <- launchResult{execution: execution.(*AgentExecution), err: err}
		}()

		startWorkspaceRead := func(expectedCall int) <-chan error {
			result := make(chan error, 1)
			go func() {
				_, err := mgr.GetOrEnsureExecution(context.Background(), "session-fresh-launch-race")
				result <- err
			}()
			select {
			case call := <-provider.entered:
				require.Equal(t, expectedCall, call)
			case <-time.After(5 * time.Second):
				t.Fatalf("workspace read %d did not reach its barrier", expectedCall)
			}
			return result
		}
		awaitWorkspaceNotReady := func(result <-chan error, call int) {
			t.Helper()
			select {
			case err := <-result:
				require.ErrorIs(t, err, ErrSessionWorkspaceNotReady)
			case <-time.After(5 * time.Second):
				t.Fatalf("workspace read %d did not return its not-ready result", call)
			}
		}
		awaitLaunchWait := func(call int) {
			t.Helper()
			select {
			case <-waits:
			case result := <-launchResultCh:
				require.NoError(t, result.err, "launch must outlive reader-owned refusals")
				t.Fatal("launch returned before the runtime registration barrier")
			case <-time.After(5 * time.Second):
				t.Fatalf("launch attempt %d did not join the workspace operation", call)
			}
		}
		awaitGate := func(call int32) {
			t.Helper()
			select {
			case got := <-gateEntered:
				require.Equal(t, call, got)
			case <-time.After(5 * time.Second):
				t.Fatalf("launch did not reach retry gate %d", call)
			}
		}

		firstRead := startWorkspaceRead(1)
		close(launchStarted)
		awaitLaunchWait(1)
		provider.release <- struct{}{}
		awaitWorkspaceNotReady(firstRead, 1)

		awaitGate(5)
		secondRead := startWorkspaceRead(2)
		close(gate5)
		awaitLaunchWait(2)
		provider.release <- struct{}{}
		awaitWorkspaceNotReady(secondRead, 2)

		awaitGate(9)
		thirdRead := startWorkspaceRead(3)
		close(gate9)
		awaitLaunchWait(3)
		provider.release <- struct{}{}
		awaitWorkspaceNotReady(thirdRead, 3)

		select {
		case <-backend.entered:
		case result := <-launchResultCh:
			require.NoError(t, result.err)
			t.Fatalf("launch returned before runtime creation: %v", result.execution)
		case <-time.After(5 * time.Second):
			t.Fatal("launch did not reach runtime creation after three reader refusals")
		}
		releaseRuntime.Do(func() { close(backend.barrier) })
		select {
		case result := <-launchResultCh:
			require.NoError(t, result.err)
			require.NotNil(t, result.execution)
		case <-time.After(5 * time.Second):
			t.Fatal("launch did not complete after runtime creation")
		}
		require.EqualValues(t, 4, provider.calls.Load())
		require.EqualValues(t, 1, backend.createCount.Load())
		require.Len(t, mgr.ListExecutions(), 1)
		require.EqualValues(t, 1, resolver.calls.Load())
	})

	t.Run("concurrent launch waiters share one runtime and cancellation stays local", func(t *testing.T) {
		resolver := &countingProfileResolver{info: &AgentProfileInfo{
			ProfileID: "profile-1", AgentName: "auggie",
		}}
		mgr, backend, provider := newFreshLaunchWorkspaceRaceFixture(t, resolver)
		defer provider.releaseRead()
		var releaseRuntime sync.Once
		defer releaseRuntime.Do(func() { close(backend.barrier) })

		workspaceResult := make(chan error, 1)
		go func() {
			_, err := mgr.GetOrEnsureExecution(context.Background(), "session-fresh-launch-race")
			workspaceResult <- err
		}()
		select {
		case <-provider.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("workspace read did not reach its barrier")
		}

		type launchResult struct {
			name      string
			execution *AgentExecution
			err       error
		}
		waits := make(chan struct{}, 16)
		results := make(chan launchResult, 3)
		cancelledCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for _, caller := range []struct {
			name string
			ctx  context.Context
		}{
			{name: "first", ctx: context.Background()},
			{name: "second", ctx: context.Background()},
			{name: "cancelled", ctx: cancelledCtx},
		} {
			caller := caller
			ctx := &launchWaitNotifyingContext{Context: caller.ctx, waits: waits}
			go func() {
				execution, err := mgr.Launch(ctx, freshWorktreeLaunchRequest())
				results <- launchResult{name: caller.name, execution: execution, err: err}
			}()
		}
		for range 3 {
			select {
			case <-waits:
			case <-time.After(5 * time.Second):
				t.Fatal("launch waiter did not join the workspace operation")
			}
		}
		cancel()
		select {
		case result := <-results:
			require.Equal(t, "cancelled", result.name)
			require.ErrorIs(t, result.err, context.Canceled)
		case <-time.After(5 * time.Second):
			t.Fatal("cancelled launch waiter did not stop waiting")
		}

		provider.releaseRead()
		select {
		case err := <-workspaceResult:
			require.ErrorIs(t, err, ErrSessionWorkspaceNotReady)
		case <-time.After(5 * time.Second):
			t.Fatal("workspace read did not return its not-ready result")
		}

		for range 2 {
			select {
			case <-waits:
			case result := <-results:
				require.NoError(t, result.err, "a live launch must retry with its own admission")
				t.Fatalf("launch %q returned before the shared runtime barrier", result.name)
			case <-time.After(5 * time.Second):
				t.Fatal("live launch waiter did not rejoin the session operation")
			}
		}
		select {
		case <-backend.entered:
		case result := <-results:
			require.NoError(t, result.err)
			t.Fatalf("launch %q returned before runtime creation reached its barrier", result.name)
		case <-time.After(5 * time.Second):
			t.Fatal("launch retry did not reach runtime creation")
		}
		releaseRuntime.Do(func() { close(backend.barrier) })

		launched := make(map[string]*AgentExecution, 2)
		for range 2 {
			select {
			case result := <-results:
				require.NoError(t, result.err)
				require.NotNil(t, result.execution)
				launched[result.name] = result.execution
			case <-time.After(5 * time.Second):
				t.Fatal("live launch did not complete")
			}
		}
		require.Same(t, launched["first"], launched["second"])
		require.EqualValues(t, 1, backend.createCount.Load())
		require.Len(t, mgr.ListExecutions(), 1)
		require.EqualValues(t, 1, resolver.calls.Load())
	})

	t.Run("genuine launch failure is preserved", func(t *testing.T) {
		launchFailure := errors.New("profile unavailable")
		resolver := &countingProfileResolver{err: launchFailure}
		mgr, backend, provider := newFreshLaunchWorkspaceRaceFixture(t, resolver)
		defer provider.releaseRead()
		var releaseRuntime sync.Once
		defer releaseRuntime.Do(func() { close(backend.barrier) })

		workspaceResult := make(chan error, 1)
		go func() {
			_, err := mgr.GetOrEnsureExecution(context.Background(), "session-fresh-launch-race")
			workspaceResult <- err
		}()
		select {
		case <-provider.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("workspace read did not reach its barrier")
		}

		waits := make(chan struct{}, 4)
		result := make(chan error, 1)
		go func() {
			_, err := mgr.Launch(&launchWaitNotifyingContext{
				Context: context.Background(), waits: waits,
			}, freshWorktreeLaunchRequest())
			result <- err
		}()
		select {
		case <-waits:
		case <-time.After(5 * time.Second):
			t.Fatal("launch did not join the workspace operation")
		}

		provider.releaseRead()
		require.ErrorIs(t, <-workspaceResult, ErrSessionWorkspaceNotReady)
		select {
		case err := <-result:
			require.ErrorIs(t, err, launchFailure)
		case <-time.After(5 * time.Second):
			t.Fatal("launch did not return its own profile failure")
		}
		require.EqualValues(t, 1, resolver.calls.Load())
		require.Zero(t, backend.createCount.Load())
	})
}

func TestRecoveryLaunchDoesNotInheritConcurrentWorkspaceAdmissionFailure(t *testing.T) {
	unrelatedFailure := errors.New("workspace backend unavailable")
	for _, tc := range []struct {
		name           string
		action         string
		candidateID    string
		peerError      error
		alreadyRunning bool
		wantError      error
	}{
		{name: "native resume", action: "resume"},
		{name: "explicit continuation", action: "continue_from_history"},
		{name: "silent native restore", candidateID: "workspace-execution"},
		{name: "silent restore unrelated peer error", candidateID: "workspace-execution", peerError: unrelatedFailure, wantError: unrelatedFailure},
		{name: "silent restore own launch failure", candidateID: "workspace-execution", alreadyRunning: true, wantError: ErrAgentAlreadyRunning},
		{name: "normal launch remains blocked", wantError: ErrSessionRecoveryRequired},
		{name: "unrelated peer error", action: "resume", peerError: unrelatedFailure, wantError: unrelatedFailure},
		{name: "own launch failure", action: "resume", alreadyRunning: true, wantError: ErrAgentAlreadyRunning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{
				ProfileID: "profile-1", AgentName: "auggie",
			}}
			mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{
				session: &models.TaskSession{
					ID: workspaceRecoverySessionID, TaskID: workspaceRecoveryTaskID,
					State:    models.TaskSessionStateStarting,
					Metadata: activeWorkspaceRecoveryMetadata(models.AgentDeliveryRecoveryUncertain, 1),
				},
			})
			// The successful launch reuses existing infrastructure. Workspace access
			// must still reject the unresolved notice; recovery owns its settlement.
			existing := &AgentExecution{
				ID: "workspace-execution", SessionID: workspaceRecoverySessionID,
				TaskID: workspaceRecoveryTaskID, AgentProfileID: "profile-1",
			}
			if tc.alreadyRunning {
				existing.AgentCommand = "auggie"
			}
			require.NoError(t, mgr.executionStore.Add(existing))
			entered, release := make(chan struct{}), make(chan struct{})
			peer := mgr.ensureExecutionGroup.DoChan(workspaceRecoverySessionID, func() (interface{}, error) {
				close(entered)
				<-release
				if tc.peerError != nil {
					return nil, tc.peerError
				}
				return nil, mgr.ensureWorkspaceRecoveryResolved(context.Background(), workspaceRecoverySessionID, "")
			})
			<-entered
			ctx := &coalescedRecoveryWaitContext{Context: context.Background(), onWait: func() { close(release) }}
			got, err := mgr.Launch(ctx, &LaunchRequest{
				TaskID: workspaceRecoveryTaskID, SessionID: workspaceRecoverySessionID,
				AgentProfileID: "profile-1", ACPSessionID: "original-native-session",
				RecoveryAction: tc.action, CandidateExecutionID: tc.candidateID,
			})
			peerResult := <-peer
			if tc.peerError != nil {
				require.ErrorIs(t, peerResult.Err, tc.peerError)
			} else {
				require.ErrorIs(t, peerResult.Err, ErrSessionRecoveryRequired)
			}
			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
				require.Nil(t, got)
				retained, ok := mgr.executionStore.GetBySessionID(workspaceRecoverySessionID)
				require.True(t, ok)
				require.Same(t, existing, retained)
				return
			}
			require.NoError(t, err, "the recovery launch must use its own admission after the workspace peer fails")
			require.Same(t, existing, got)
			require.NotEmpty(t, got.AgentCommand)
			require.Equal(t, "original-native-session", got.ACPSessionID)
			require.ErrorIs(t, mgr.ensureWorkspaceRecoveryResolved(context.Background(), workspaceRecoverySessionID, ""), ErrSessionRecoveryRequired)
		})
	}
}

func TestCancelledRecoveryLaunchDoesNotStartSharedOperation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		action      string
		candidateID string
	}{
		{name: "explicit resume", action: "resume"},
		{name: "silent restore", candidateID: "cancelled-candidate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManager(t)
			resolver := &countingProfileResolver{err: errors.New("profile unavailable")}
			mgr.profileResolver = resolver
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := mgr.coalesceSessionLaunch(ctx, &LaunchRequest{
				SessionID: "cancelled-session", AgentProfileID: "profile",
				RecoveryAction: tc.action, CandidateExecutionID: tc.candidateID,
			})

			require.ErrorIs(t, err, context.Canceled)
			// Joining the same singleflight key waits for any operation admitted above.
			_, err = mgr.doCoalescedExecution(context.Background(), "cancelled-session", func(context.Context) (interface{}, error) { return nil, nil })
			if err != nil {
				require.ErrorContains(t, err, "profile unavailable")
			}
			require.Zero(t, resolver.calls.Load(), "an already cancelled recovery must not launch caller-independent work")
		})
	}
}
