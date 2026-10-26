package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/nomad"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type publishResult struct {
	published bool
	err       error
}

// recordingNomadPublisher scripts PublishRecoveredNomadEnvironmentReady.
type recordingNomadPublisher struct {
	mu        sync.Mutex
	calls     []NomadEnvironmentReadyPublication
	results   []publishResult
	onPublish func(NomadEnvironmentReadyPublication)
}

func (p *recordingNomadPublisher) PublishRecoveredNomadEnvironmentReady(
	_ context.Context,
	params NomadEnvironmentReadyPublication,
) (bool, error) {
	p.mu.Lock()
	p.calls = append(p.calls, params)
	index := len(p.calls) - 1
	hook := p.onPublish
	var result publishResult
	if index < len(p.results) {
		result = p.results[index]
	} else {
		result = publishResult{published: true}
	}
	p.mu.Unlock()
	if hook != nil && result.err == nil {
		hook(params)
	}
	return result.published, result.err
}

func (p *recordingNomadPublisher) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// countingInventoryStore counts durable inventory reads.
type countingInventoryStore struct {
	*nomadInventoryStoreFake
	mu    sync.Mutex
	reads int
	// afterNextRead runs once, after the next read returns.
	afterNextRead func()
}

func (s *countingInventoryStore) GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error) {
	s.mu.Lock()
	s.reads++
	after := s.afterNextRead
	s.afterNextRead = nil
	s.mu.Unlock()
	row, err := s.nomadInventoryStoreFake.GetExecutorRunningBySessionID(ctx, sessionID)
	if after != nil {
		after()
	}
	return row, err
}

func (s *countingInventoryStore) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

// advancingNomadExecutor runs a hook after the runtime boundary, emulating the
// original operation reaching a later inventory phase.
type advancingNomadExecutor struct {
	*createInstanceExecutor
	onCreate func()
}

func (e *advancingNomadExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	instance, err := e.createInstanceExecutor.CreateInstance(ctx, req)
	if err == nil && e.onCreate != nil {
		e.onCreate()
	}
	return instance, err
}

type nomadPublishOptions struct {
	phase       string
	repos       []WorkspaceRepositorySpec
	materialize func(w http.ResponseWriter, r *http.Request, req agentctl.MaterializeRepositoryRequest)
	// envStatus is the environment status the admission reader reports.
	envStatus models.TaskEnvironmentStatus
}

type nomadPublishFixture struct {
	t          *testing.T
	manager    *Manager
	backend    *advancingNomadExecutor
	provider   *nomadCreatedContinuationProvider
	reader     *fakeExecutorProfileReader
	store      *countingInventoryStore
	publisher  *recordingNomadPublisher
	row        *models.ExecutorRunning
	entry      *remoteRecoveryRetry
	info       *WorkspaceInfo
	state      nomad.EnvironmentState
	mu         sync.Mutex
	requests   []agentctl.MaterializeRepositoryRequest
	removeHits int
}

const (
	publishTaskID      = "task-publish-nomad"
	publishSessionID   = "session-publish-nomad"
	publishEnvironment = "environment-publish-nomad"
	publishExecutionID = "execution-publish-nomad"
)

func newNomadPublishClient(t *testing.T, f *nomadPublishFixture, o nomadPublishOptions) *agentctl.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/workspace/materialize-repository":
			var req agentctl.MaterializeRepositoryRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.mu.Lock()
			f.requests = append(f.requests, req)
			f.mu.Unlock()
			if o.materialize != nil {
				o.materialize(w, r, req)
				return
			}
			_ = json.NewEncoder(w).Encode(agentctl.MaterializeRepositoryResponse{Destination: req.Destination, Reused: true})
		case "/api/v1/workspace/materialize-repository/remove":
			f.mu.Lock()
			f.removeHits++
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"removed": true})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	host, portString, err := net.SplitHostPort(parsed.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portString)
	require.NoError(t, err)
	return agentctl.NewClient(host, port, newTestLogger())
}

func (f *nomadPublishFixture) setInventory(phase string, revision uint64) {
	f.t.Helper()
	state := f.state
	state.Phase = phase
	state.Revision = revision
	require.NoError(f.t, nomad.PutEnvironmentState(f.row.Metadata, state))
	// The workspace info provider derives its Nomad state from the same inventory.
	require.NoError(f.t, nomad.PutEnvironmentState(f.provider.info.Metadata, state))
}

func (f *nomadPublishFixture) materializeRequests() []agentctl.MaterializeRepositoryRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]agentctl.MaterializeRepositoryRequest(nil), f.requests...)
}

func (f *nomadPublishFixture) attempt() {
	f.manager.retryRemoteRecovery(context.Background(), f.entry)
}

func newNomadPublishFixture(t *testing.T, o nomadPublishOptions) *nomadPublishFixture {
	t.Helper()
	if o.phase == "" {
		o.phase = "ready"
	}
	if o.envStatus == "" {
		o.envStatus = models.TaskEnvironmentStatusCreating
	}
	f := &nomadPublishFixture{t: t}
	manager := newTestManager(t)
	manager.executorRegistry = NewExecutorRegistry(manager.logger)
	f.backend = &advancingNomadExecutor{createInstanceExecutor: &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameNomad},
		client:       newNomadPublishClient(t, f, o),
	}}
	manager.executorRegistry.Register(f.backend)
	workspaceRoot := t.TempDir()
	workspacePath, err := nomad.EnvironmentWorkspacePath(workspaceRoot, publishEnvironment, 1)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(workspacePath, 0o700))
	info := &WorkspaceInfo{
		TaskID: publishTaskID, SessionID: publishSessionID, TaskEnvironmentID: publishEnvironment,
		ValidatedTaskEnvironmentID: publishEnvironment, ValidatedExecutorType: string(models.ExecutorTypeNomad),
		ValidatedTaskEnvironmentGeneration: 1, EnvironmentOwnerTaskID: publishTaskID, OwnershipGeneration: 1,
		WorkspaceID: "workspace-publish-nomad", WorkspacePath: workspacePath,
		ExecutorType: string(models.ExecutorTypeNomad), AgentExecutionID: publishExecutionID,
		ExecutionProfileID: "agent-profile-publish-nomad", ExecutorProfileID: "executor-profile-publish-nomad",
		AgentID: "auggie", WorkspaceRepositories: o.repos,
		Metadata: map[string]interface{}{
			MetadataKeyExecutorType:      string(models.ExecutorTypeNomad),
			MetadataKeyExecutorProfileID: "executor-profile-publish-nomad",
			"executor_id":                "executor-id-publish-nomad",
			"address":                    "http://127.0.0.1:4646", "namespace": "default", "region": "global",
			"datacenter": "dc1", "node_id": "node-publish-nomad", "platform": "darwin/arm64",
			"ssh_target": "runner@127.0.0.1", "ssh_port": "22",
			"ssh_host_fingerprint": "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
			"workspace_root":       workspaceRoot, "allocation_root": t.TempDir(),
			"cpu_mhz": "1000", "memory_mb": "2048", "nomad_token": "$kandev-secret:nomad-token-ref",
			MetadataKeyBootstrapNonceSecret: "$kandev-secret:bootstrap-nonce-ref",
			MetadataKeyAuthTokenSecret:      "$kandev-secret:agentctl-auth-ref",
		},
	}
	state := nomad.EnvironmentState{
		Version: 1, Revision: 4, EnvironmentID: publishEnvironment, Generation: 1,
		OperationID: publishExecutionID, InstallationID: info.ExecutorProfileID, ExecutionID: publishExecutionID,
		Namespace: "default", Region: "global", Datacenter: "dc1",
		NodeID: "node-publish-nomad", Platform: "darwin/arm64", SSHHost: "127.0.0.1", SSHUser: "runner", SSHPort: 22,
		SSHHostFingerprint: info.Metadata["ssh_host_fingerprint"].(string), WorkspaceRoot: workspaceRoot,
		WorkspacePath: workspacePath, WorkspaceInputCaptured: true,
		AllocationRoot: info.Metadata["allocation_root"].(string),
		CPUMHz:         1000, MemoryMB: 2048, AllocationID: "allocation-publish-nomad",
		AllocationCreateIndex: 10, ControlPort: 23001, InstancePort: 23002, Phase: o.phase,
		ServerIdentity: "server-identity-publish-nomad",
	}
	state.JobID, err = nomad.StableJobID(nomad.JobIdentity{
		EnvironmentID: publishEnvironment, Generation: 1, OperationID: publishExecutionID, InstallationID: info.ExecutorProfileID,
	})
	require.NoError(t, err)
	profile, err := nomadProfileFromMetadata(info.Metadata)
	require.NoError(t, err)
	preparation, err := manager.prepareExecutionCreateRequest(context.Background(), publishTaskID, info, publishExecutionID)
	require.NoError(t, err)
	state.InputDigest, err = nomadLaunchDigest(profile, preparation.request, state.WorkspaceInputPath, workspacePath)
	require.NoError(t, err)
	addFixtureJobEvidence(t, profile, &state)
	require.NoError(t, nomad.PutEnvironmentState(info.Metadata, state))
	f.state, f.info, f.manager = state, info, manager

	f.provider = &nomadCreatedContinuationProvider{
		info: info,
		snapshot: &RemoteRecoverySnapshot{
			TaskID: publishTaskID, SessionID: publishSessionID, SessionState: models.TaskSessionStateCreated,
			AgentExecutionID: publishExecutionID, Runtime: string(agentruntime.RuntimeNomad),
			TaskWorkspaceID: info.WorkspaceID, TaskEnvironmentID: publishEnvironment,
			EnvironmentOwnerTaskID: publishTaskID, EnvironmentOwnershipGen: 1,
			EnvironmentExecutorType: string(models.ExecutorTypeNomad), EnvironmentExecutorID: "executor-id-publish-nomad",
			EnvironmentStatus: string(o.envStatus), RouteState: "active",
		},
	}
	manager.workspaceInfoProvider = f.provider
	f.reader = &fakeExecutorProfileReader{
		task: &models.Task{ID: publishTaskID},
		session: &models.TaskSession{
			ID: publishSessionID, TaskID: publishTaskID, TaskEnvironmentID: publishEnvironment,
			State: models.TaskSessionStateCreated,
		},
		env: &models.TaskEnvironment{
			ID: publishEnvironment, TaskID: publishTaskID, ExecutorID: "executor-id-publish-nomad",
			ExecutorType: string(models.ExecutorTypeNomad), OwnershipGeneration: 1, Status: o.envStatus,
		},
	}
	manager.SetExecutorProfileReader(f.reader)
	f.row = &models.ExecutorRunning{
		ID: publishSessionID, SessionID: publishSessionID, TaskID: publishTaskID, AgentExecutionID: publishExecutionID,
		Runtime: agentruntime.RuntimeNomad, ExecutionProfileID: info.ExecutionProfileID,
		Status: models.ExecutorRunningStatusReady, Metadata: clonePluginExecutorMetadata(info.Metadata),
	}
	f.store = &countingInventoryStore{nomadInventoryStoreFake: &nomadInventoryStoreFake{row: f.row}}
	manager.SetExecutorRunningWriter(f.store)
	f.publisher = &recordingNomadPublisher{onPublish: func(NomadEnvironmentReadyPublication) {
		f.reader.env.Status = models.TaskEnvironmentStatusReady
	}}
	manager.SetNomadEnvironmentReadyPublisher(f.publisher)
	pinned := *f.row
	pinned.Metadata = clonePluginExecutorMetadata(f.row.Metadata)
	f.entry = manager.queueRemoteRecovery(&pinned, f.provider.snapshot)
	require.NotNil(t, f.entry)
	return f
}

func (f *nomadPublishFixture) requireGuardHeld() {
	f.t.Helper()
	require.True(f.t, f.manager.HasPendingRemoteRecovery(publishSessionID, publishExecutionID), "recovery guard must be retained")
}

func (f *nomadPublishFixture) requireGuardReleased() {
	f.t.Helper()
	require.False(f.t, f.manager.HasPendingRemoteRecovery(publishSessionID, publishExecutionID), "recovery guard must be released")
	require.Nil(f.t, f.manager.pendingRemoteRecoveryForSession(publishSessionID))
}

func twoRepoSpecs() []WorkspaceRepositorySpec {
	return []WorkspaceRepositorySpec{
		{
			RepositoryID: "repo-a", RepoName: "alpha", RepositoryURL: "https://example.test/org/alpha.git",
			BaseBranch: "main", BranchIdentitySlug: "main", WorkspaceRelativePath: "alpha",
		},
		{
			RepositoryID: "repo-b", RepoName: "beta", RepositoryURL: "https://example.test/org/beta.git",
			BaseBranch: "develop", DefaultBranch: "main", CheckoutBranch: "feature/x", BranchSlug: "feature-x",
		},
	}
}

func TestCreatedNomadRecoveryPublish_PublishesBeforeGuardReleaseAndAdmissionSucceedsAfter(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{repos: twoRepoSpecs()})
	guardHeldDuringPublish := false
	f.publisher.onPublish = func(NomadEnvironmentReadyPublication) {
		guardHeldDuringPublish = f.manager.HasPendingRemoteRecovery(publishSessionID, publishExecutionID)
		f.reader.env.Status = models.TaskEnvironmentStatusReady
	}
	require.ErrorIs(t, f.manager.ensureWorkspaceSessionAdmitted(context.Background(), publishTaskID, f.info), ErrSessionWorkspaceNotReady,
		"ordinary admission rejects the creating environment before publication")

	f.attempt()

	require.True(t, guardHeldDuringPublish, "publication must precede guard release")
	f.requireGuardReleased()
	require.Equal(t, 1, f.publisher.callCount())
	call := f.publisher.calls[0]
	require.Equal(t, NomadEnvironmentReadyPublication{
		SessionID: publishSessionID, EnvironmentID: publishEnvironment, ExecutionID: publishExecutionID,
		OwnershipGeneration: 1, ExpectedRevision: 4,
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-a", BranchSlug: "main", WorkspaceRelativePath: "alpha", Position: 0},
			{RepositoryID: "repo-b", BranchSlug: "feature-x", Position: 1},
		},
	}, call)
	require.NoError(t, f.manager.ensureWorkspaceSessionAdmitted(context.Background(), publishTaskID, f.info),
		"ordinary admission succeeds once the environment is ready and the guard is released")
	execution, exists := f.manager.executionStore.GetBySessionID(publishSessionID)
	require.True(t, exists)
	require.False(t, execution.isSessionInitialized(), "no prompt or conversation is started")
}

func TestCreatedNomadRecoveryPublish_NoPublishForNonRecoveryOrStoppedCaller(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{})
	state := f.state

	stopped := f.entry
	f.manager.remoteRecoveryMu.Lock()
	stopped.stopRequested = true
	f.manager.remoteRecoveryMu.Unlock()
	_, err := f.manager.publishCreatedNomadRecoveredEnvironment(context.Background(), stopped, f.info, state)
	require.Error(t, err)

	replaced := &remoteRecoveryRetry{record: f.entry.record, snapshot: f.entry.snapshot}
	_, err = f.manager.publishCreatedNomadRecoveredEnvironment(context.Background(), replaced, f.info, state)
	require.Error(t, err)

	require.Zero(t, f.publisher.callCount(), "a stopped or non-recovery caller never reaches the publisher")
}

func TestCreatedNomadRecoveryPublish_FailureRetainsGuardAndNextTickPublishes(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{})
	f.publisher.results = []publishResult{{err: errors.New("database is locked")}}

	f.attempt()
	f.requireGuardHeld()
	require.Equal(t, nomadRecoveryClassStarting, f.manager.createdNomadRecoveryClass(f.entry))
	require.Equal(t, models.TaskEnvironmentStatusCreating, f.reader.env.Status)

	f.attempt()
	require.Equal(t, 2, f.publisher.callCount())
	f.requireGuardReleased()
}

func TestCreatedNomadRecoveryPublish_FirstAttemptAdvancesToReadyThenNextTickPublishesWithReReadRevision(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{phase: "allocated"})
	f.backend.onCreate = func() { f.setInventory("ready", 9) }
	f.publisher.results = []publishResult{{err: errors.New("transient database error")}}

	f.attempt()
	f.requireGuardHeld()
	require.Equal(t, uint64(9), f.publisher.calls[0].ExpectedRevision, "revision comes from the inventory re-read, not the pinned entry")

	f.setInventory("ready", 10)
	f.attempt()
	require.Equal(t, 2, f.publisher.callCount())
	require.Equal(t, uint64(10), f.publisher.calls[1].ExpectedRevision, "the retry re-reads the inventory revision")
	f.requireGuardReleased()
}

func TestCreatedNomadRecoveryPublish_ReplayAfterCommittedPublishDoesNotDoublePublish(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{})
	// The first attempt commits but the final owner snapshot read fails.
	f.provider.failSnapshotCallAt = 3
	f.attempt()
	f.requireGuardHeld()
	require.Equal(t, 1, f.publisher.callCount())
	require.Equal(t, models.TaskEnvironmentStatusReady, f.reader.env.Status)

	// The replay reports the idempotent already-published outcome.
	f.publisher.results = []publishResult{{}, {published: false}}
	f.attempt()
	require.Equal(t, 2, f.publisher.callCount())
	f.requireGuardReleased()
}

func TestCreatedNomadRecoveryPublish_RestartFromEnvironmentReadyCompletesWithoutWrite(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{envStatus: models.TaskEnvironmentStatusReady})
	f.publisher.results = []publishResult{{published: false}}

	f.attempt()

	require.Equal(t, 1, f.publisher.callCount(), "the replay reaches publish, which reports no write")
	f.requireGuardReleased()
	execution, exists := f.manager.executionStore.GetBySessionID(publishSessionID)
	require.True(t, exists)
	require.False(t, execution.isSessionInitialized())
	require.Empty(t, execution.ACPSessionID, "no prompt is sent")
}

func TestCreatedNomadRecoveryPublish_PreReadyPhaseDoesNotPublishAndClassesStarting(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{phase: "allocated"})

	f.attempt()

	require.Zero(t, f.publisher.callCount())
	require.Equal(t, nomadRecoveryClassStarting, f.manager.createdNomadRecoveryClass(f.entry))
	f.requireGuardHeld()
}

func TestCreatedNomadRecoveryPublish_PostReadyPhaseClassesOperatorActionAndTickerSkips(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{phase: "allocated"})
	f.backend.onCreate = func() { f.setInventory("cleanup_pending", 9) }

	f.attempt()
	require.Zero(t, f.publisher.callCount())
	require.Equal(t, nomadRecoveryClassOperatorAction, f.manager.createdNomadRecoveryClass(f.entry))
	f.requireGuardHeld()
	creates, reads := f.backend.createCount.Load(), f.store.readCount()

	f.attempt()
	require.Equal(t, creates, f.backend.createCount.Load(), "an operator_action entry makes no further Nomad contact")
	require.Equal(t, reads, f.store.readCount(), "an operator_action entry does no further reconciliation or publish")
	require.Zero(t, f.publisher.callCount())
	f.requireGuardHeld()
}

func TestCreatedNomadRecoveryPublish_StopProceedsOnOperatorActionEntry(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{})
	f.manager.recordCreatedNomadRecoveryClass(f.entry, nomadRecoveryClassOperatorAction)
	f.manager.remoteRecoveryMu.Lock()
	f.entry.stopRequested, f.entry.stopReason = true, StopReasonTaskDeleted
	f.manager.remoteRecoveryMu.Unlock()
	before := f.provider.snapshotCalls.Load()

	f.attempt()

	require.Greater(t, f.provider.snapshotCalls.Load(), before+1, "the stop retry still validates the owner after the operator_action class")
	require.Zero(t, f.publisher.callCount())
	require.Zero(t, f.backend.createCount.Load(), "no recovery attempt runs for a stopping operator_action entry")
}

func TestCreatedNomadRecoveryPublish_TypedGuardErrorCarriesStableCode(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{})

	err := f.manager.rejectWorkspaceDuringCreatedNomadRecovery(publishSessionID)
	require.ErrorIs(t, err, ErrSessionRecoveryGuarded)
	var guardErr *NomadRecoveryGuardError
	require.ErrorAs(t, err, &guardErr)
	require.Equal(t, NomadRecoveryCodeStarting, guardErr.Code, "an entry with no recorded attempt reports starting")
	require.Equal(t, "workspace recovery guarded: nomad_recovery_starting", err.Error())

	f.manager.recordCreatedNomadRecoveryClass(f.entry, nomadRecoveryClassOperatorAction)
	err = f.manager.rejectWorkspaceDuringCreatedNomadRecovery(publishSessionID)
	require.ErrorIs(t, err, ErrSessionRecoveryGuarded)
	require.ErrorAs(t, err, &guardErr)
	require.Equal(t, NomadRecoveryCodeOperatorAction, guardErr.Code)
}

func TestCreatedNomadRecoveryPublish_WorkspaceReadNeverStartsAttempt(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := f.manager.GetOrEnsureExecution(ctx, publishSessionID)
	require.ErrorIs(t, err, ErrSessionRecoveryGuarded)
	_, err = f.manager.GetOrEnsureExecutionForEnvironment(ctx, publishEnvironment)
	require.ErrorIs(t, err, ErrSessionRecoveryGuarded)

	require.Zero(t, f.publisher.callCount())
	require.Zero(t, f.backend.createCount.Load())
	require.Empty(t, f.manager.createdNomadRecoveryClass(f.entry), "a read records no attempt outcome")
}

func TestCreatedNomadRecoveryPublish_ReconcileCollisionKeepsGuardWithOperatorAction(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusUnprocessableEntity} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			f := newNomadPublishFixture(t, nomadPublishOptions{
				repos: twoRepoSpecs()[:1],
				materialize: func(w http.ResponseWriter, _ *http.Request, _ agentctl.MaterializeRepositoryRequest) {
					w.WriteHeader(status)
					_ = json.NewEncoder(w).Encode(agentctl.MaterializeRepositoryResponse{Error: "destination is not a valid checkout"})
				},
			})
			f.attempt()
			require.Zero(t, f.publisher.callCount(), "nothing is published after a reconcile collision")
			require.Equal(t, nomadRecoveryClassOperatorAction, f.manager.createdNomadRecoveryClass(f.entry))
			f.requireGuardHeld()
			require.Equal(t, models.TaskEnvironmentStatusCreating, f.reader.env.Status)
		})
	}
}

func TestCreatedNomadRecoveryPublish_TransientReconcileFailureClassesStarting(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{
		repos: twoRepoSpecs()[:1],
		materialize: func(w http.ResponseWriter, _ *http.Request, _ agentctl.MaterializeRepositoryRequest) {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(agentctl.MaterializeRepositoryResponse{Error: "dns lookup failed"})
		},
	})
	f.attempt()
	require.Zero(t, f.publisher.callCount())
	require.Equal(t, nomadRecoveryClassStarting, f.manager.createdNomadRecoveryClass(f.entry))
	f.requireGuardHeld()
}

func TestCreatedNomadRecoveryPublish_ReconcileBudgetExpiryClassesStartingAndBudgetContextReachesClient(t *testing.T) {
	sawCancel := make(chan struct{})
	f := newNomadPublishFixture(t, nomadPublishOptions{
		repos: twoRepoSpecs()[:1],
		materialize: func(_ http.ResponseWriter, r *http.Request, _ agentctl.MaterializeRepositoryRequest) {
			<-r.Context().Done()
			close(sawCancel)
		},
	})
	f.manager.nomadRecoveryReconcileBudget = 50 * time.Millisecond

	f.attempt()

	select {
	case <-sawCancel:
	case <-time.After(2 * time.Second):
		t.Fatal("the reconcile budget context did not reach the agentctl client call")
	}
	require.Zero(t, f.publisher.callCount())
	require.Equal(t, nomadRecoveryClassStarting, f.manager.createdNomadRecoveryClass(f.entry))
	f.requireGuardHeld()
}

func TestCreatedNomadRecoveryPublish_ReconcileLongerThanAttemptTimeoutPublishes(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{
		repos: twoRepoSpecs()[:1],
		materialize: func(w http.ResponseWriter, _ *http.Request, req agentctl.MaterializeRepositoryRequest) {
			time.Sleep(300 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(agentctl.MaterializeRepositoryResponse{Destination: req.Destination, Reused: true})
		},
	})
	f.manager.remoteAttemptBudget = 100 * time.Millisecond
	f.manager.nomadRecoveryReconcileBudget = 5 * time.Second

	f.attempt()

	require.Equal(t, 1, f.publisher.callCount(), "a reconcile slower than the attempt timeout still publishes")
	f.requireGuardReleased()
}

func TestCreatedNomadRecoveryPublish_ReconcileReusesOriginalCheckout(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{repos: twoRepoSpecs()})

	f.attempt()

	requests := f.materializeRequests()
	require.Len(t, requests, 2)
	require.Equal(t, agentctl.MaterializeRepositoryRequest{
		RepositoryURL: "https://example.test/org/alpha.git", Destination: "alpha-main", BaseBranch: "main",
	}, requests[0])
	require.Equal(t, agentctl.MaterializeRepositoryRequest{
		RepositoryURL: "https://example.test/org/beta.git", Destination: "beta-feature-x", BaseBranch: "develop",
		CheckoutBranch: "feature/x",
	}, requests[1])
	require.Zero(t, f.removeHits, "reusing every original checkout makes no write")
	require.Equal(t, 1, f.publisher.callCount())
}

func TestCreatedNomadRecoveryPublish_ReconcileLocatorMismatchIsOperatorAction(t *testing.T) {
	const originOnDisk = "https://example.test/org/alpha.git"
	specs := twoRepoSpecs()[:1]
	specs[0].RepositoryURL = originOnDisk + "x" // one byte off the checkout origin
	f := newNomadPublishFixture(t, nomadPublishOptions{
		repos: specs,
		materialize: func(w http.ResponseWriter, _ *http.Request, req agentctl.MaterializeRepositoryRequest) {
			if req.RepositoryURL != originOnDisk {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(agentctl.MaterializeRepositoryResponse{Error: "destination origin does not match"})
				return
			}
			_ = json.NewEncoder(w).Encode(agentctl.MaterializeRepositoryResponse{Destination: req.Destination, Reused: true})
		},
	})

	f.attempt()

	require.Zero(t, f.publisher.callCount())
	require.Equal(t, nomadRecoveryClassOperatorAction, f.manager.createdNomadRecoveryClass(f.entry))
	f.requireGuardHeld()
}

func TestCreatedNomadRecoveryPublish_UnreproducibleOrUnlocatableRepositoryFailsClosed(t *testing.T) {
	cases := map[string]func(*WorkspaceRepositorySpec){
		"empty clone url":   func(spec *WorkspaceRepositorySpec) { spec.RepositoryURL = "" },
		"pull request":      func(spec *WorkspaceRepositorySpec) { spec.PRNumber = 7 },
		"qualified pr base": func(spec *WorkspaceRepositorySpec) { spec.QualifiedPRBase = &models.PRBase{} },
		"sparse checkout": func(spec *WorkspaceRepositorySpec) {
			spec.CheckoutOptions = &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadStandard, SparseDirectories: []string{"src"}}
		},
		"on demand checkout": func(spec *WorkspaceRepositorySpec) {
			spec.CheckoutOptions = &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadOnDemand}
		},
		"remote contribution": func(spec *WorkspaceRepositorySpec) { spec.RemoteContribution = &models.RemoteContribution{} },
		"contribution target": func(spec *WorkspaceRepositorySpec) { spec.ContributionDestination = &models.ContributionDestination{} },
		"unsafe destination": func(spec *WorkspaceRepositorySpec) {
			spec.RepoName, spec.BaseBranch, spec.DefaultBranch = "///", "", ""
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			specs := twoRepoSpecs()[:1]
			mutate(&specs[0])
			f := newNomadPublishFixture(t, nomadPublishOptions{repos: specs})

			f.attempt()

			require.Empty(t, f.materializeRequests(), "agentctl is never contacted for a repository recovery cannot reproduce")
			require.Zero(t, f.publisher.callCount())
			require.Equal(t, nomadRecoveryClassOperatorAction, f.manager.createdNomadRecoveryClass(f.entry))
			f.requireGuardHeld()
		})
	}
}

func TestCreatedNomadRecoveryPublish_PublishErrorClasses(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"rotated":       {models.ErrExecutionRotated, nomadRecoveryClassOperatorAction},
		"reuse unsafe":  {models.ErrWorkspaceReuseUnsafe, nomadRecoveryClassOperatorAction},
		"not ready":     {models.ErrNomadRecoveryNotReady, nomadRecoveryClassStarting},
		"database":      {errors.New("database is locked"), nomadRecoveryClassStarting},
		"canceled":      {context.Canceled, nomadRecoveryClassStarting},
		"wrapped fence": {errors.Join(errors.New("publish"), models.ErrExecutionRotated), nomadRecoveryClassOperatorAction},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newNomadPublishFixture(t, nomadPublishOptions{})
			f.publisher.results = []publishResult{{err: tc.err}}
			f.attempt()
			require.Equal(t, tc.want, f.manager.createdNomadRecoveryClass(f.entry))
			f.requireGuardHeld()
			require.Equal(t, models.TaskEnvironmentStatusCreating, f.reader.env.Status)
		})
	}
}

func TestCreatedNomadRecoveryPublish_MissingPublisherAndUnreadyEnvironmentFailClosed(t *testing.T) {
	t.Run("no publisher", func(t *testing.T) {
		f := newNomadPublishFixture(t, nomadPublishOptions{})
		f.manager.SetNomadEnvironmentReadyPublisher(nil)
		f.attempt()
		f.requireGuardHeld()
		require.Equal(t, nomadRecoveryClassStarting, f.manager.createdNomadRecoveryClass(f.entry))
	})
	t.Run("environment still creating after publish", func(t *testing.T) {
		f := newNomadPublishFixture(t, nomadPublishOptions{})
		f.publisher.onPublish = nil
		f.attempt()
		require.Equal(t, 1, f.publisher.callCount())
		f.requireGuardHeld()
	})
}

func TestCreatedNomadRecoveryPublish_FenceLossClassification(t *testing.T) {
	f := newNomadPublishFixture(t, nomadPublishOptions{})
	m, entry := f.manager, f.entry

	require.Equal(t, nomadRecoveryClassStarting, m.classifyCreatedNomadRecoveryError(entry, errRemoteRecoveryFenced, context.DeadlineExceeded),
		"a loss caused only by the attempt window ending classes starting")
	require.Equal(t, nomadRecoveryClassStarting, m.classifyCreatedNomadRecoveryError(entry, errors.New("snapshot read failed"), nil))

	m.remoteRecoveryMu.Lock()
	entry.stopRequested = true
	m.remoteRecoveryMu.Unlock()
	require.Equal(t, nomadRecoveryClassOperatorAction, m.classifyCreatedNomadRecoveryError(entry, errRemoteRecoveryFenced, nil),
		"a requested stop is a fence loss")

	other := &remoteRecoveryRetry{record: entry.record}
	require.Equal(t, nomadRecoveryClassOperatorAction, m.classifyCreatedNomadRecoveryError(other, errRemoteRecoveryFenced, nil),
		"a removed or replaced entry is a fence loss")
	require.Equal(t, nomadRecoveryClassOperatorAction,
		m.classifyCreatedNomadRecoveryError(entry, operatorActionRecoveryError(errors.New("owner changed")), nil))
}

func TestCreatedNomadRecoveryPublish_ProjectionMatchesLaunchEnvironmentRepos(t *testing.T) {
	info := &WorkspaceInfo{WorkspaceRepositories: []WorkspaceRepositorySpec{
		{RepositoryID: "", BranchIdentitySlug: "skipped"},
		{RepositoryID: "repo-a", BranchIdentitySlug: "Feature/X Y", BranchSlug: "ignored", WorkspaceRelativePath: "a"},
		{RepositoryID: "repo-b", BranchSlug: "Fallback/Slug"},
		{RepositoryID: "repo-c"},
	}}

	repos := createdNomadEnvironmentRepos(info)

	require.Len(t, repos, 3)
	require.Equal(t, &models.TaskEnvironmentRepo{RepositoryID: "repo-a", BranchSlug: NomadRecoveryRepoBranchSlug("Feature/X Y", "ignored"), WorkspaceRelativePath: "a", Position: 1}, repos[0])
	require.Equal(t, &models.TaskEnvironmentRepo{RepositoryID: "repo-b", BranchSlug: NomadRecoveryRepoBranchSlug("", "Fallback/Slug"), Position: 2}, repos[1])
	require.Equal(t, 3, repos[2].Position, "position is the index in the full list, as in the launch projection")
	require.Equal(t, "", repos[2].BranchSlug)
}

func TestCreatedNomadRecoveryPublish_AttemptWindowEndingBeforePublishClassesStarting(t *testing.T) {
	var f *nomadPublishFixture
	f = newNomadPublishFixture(t, nomadPublishOptions{
		repos: twoRepoSpecs()[:1],
		// The attempt window ends right after reconciliation, before the ready
		// inventory read that precedes publication.
		materialize: func(w http.ResponseWriter, _ *http.Request, req agentctl.MaterializeRepositoryRequest) {
			f.store.mu.Lock()
			f.store.afterNextRead = func() { f.manager.finishRemoteRecoveryAttempt(f.entry) }
			f.store.mu.Unlock()
			_ = json.NewEncoder(w).Encode(agentctl.MaterializeRepositoryResponse{Destination: req.Destination, Reused: true})
		},
	})

	f.attempt()

	require.Zero(t, f.publisher.callCount(), "publication is not attempted once the window has ended")
	require.Equal(t, nomadRecoveryClassStarting, f.manager.createdNomadRecoveryClass(f.entry))
	f.requireGuardHeld()
}
