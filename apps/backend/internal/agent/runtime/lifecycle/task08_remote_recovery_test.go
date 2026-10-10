package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

type task08ExecutorInventoryWriter struct {
	invariantWriter
	rows []*models.ExecutorRunning
}

type task08RemoteSnapshotProvider struct {
	snapshots map[string]*RemoteRecoverySnapshot
}

type task08BlockingSnapshotProvider struct {
	task08RemoteSnapshotProvider
	mu      sync.Mutex
	calls   int
	blockAt int
	entered chan struct{}
	release chan struct{}
}

func (*task08RemoteSnapshotProvider) GetWorkspaceInfoForSession(context.Context, string, string) (*WorkspaceInfo, error) {
	return nil, nil
}

func (*task08RemoteSnapshotProvider) GetWorkspaceInfoForEnvironment(context.Context, string) (*WorkspaceInfo, error) {
	return nil, nil
}

func (p *task08RemoteSnapshotProvider) GetRemoteRecoverySnapshot(_ context.Context, sessionID string) (*RemoteRecoverySnapshot, error) {
	snapshot := p.snapshots[sessionID]
	if snapshot == nil {
		return nil, errors.New("snapshot not found")
	}
	copySnapshot := *snapshot
	return &copySnapshot, nil
}

func (p *task08BlockingSnapshotProvider) GetRemoteRecoverySnapshot(ctx context.Context, sessionID string) (*RemoteRecoverySnapshot, error) {
	p.mu.Lock()
	p.calls++
	block := p.calls == p.blockAt
	p.mu.Unlock()
	if block {
		close(p.entered)
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return p.task08RemoteSnapshotProvider.GetRemoteRecoverySnapshot(ctx, sessionID)
}

func task08RemoteSnapshot(record *models.ExecutorRunning) *RemoteRecoverySnapshot {
	return &RemoteRecoverySnapshot{
		TaskID: record.TaskID, SessionID: record.SessionID, SessionState: models.TaskSessionStateWaitingForInput,
		AgentExecutionID: record.AgentExecutionID, Runtime: string(record.Runtime), RouteState: "active",
		TaskWorkspaceID: "workspace-" + record.TaskID, TaskEnvironmentID: "environment-" + record.TaskID,
		EnvironmentOwnerTaskID: record.TaskID, EnvironmentOwnershipGen: 1,
		EnvironmentExecutorType: string(record.Runtime), EnvironmentExecutorID: "executor-" + record.TaskID,
		EnvironmentStatus: "ready",
	}
}

func task08SetSnapshots(mgr *Manager, rows []*models.ExecutorRunning) {
	provider := &task08RemoteSnapshotProvider{snapshots: make(map[string]*RemoteRecoverySnapshot)}
	for _, row := range rows {
		if row != nil && isRemoteRecoveryRuntime(row.Runtime) {
			provider.snapshots[row.SessionID] = task08RemoteSnapshot(row)
		}
	}
	mgr.workspaceInfoProvider = provider
}

func (w *task08ExecutorInventoryWriter) ListExecutorsRunning(context.Context) ([]*models.ExecutorRunning, error) {
	return append([]*models.ExecutorRunning(nil), w.rows...), nil
}

func (w *task08ExecutorInventoryWriter) ListExecutorsRunningLiveStandalone(context.Context) ([]*models.ExecutorRunning, error) {
	return task08RowsForRuntime(w.rows, agentruntime.RuntimeStandalone), nil
}

func (w *task08ExecutorInventoryWriter) ListExecutorsRunningPluginRemote(context.Context) ([]*models.ExecutorRunning, error) {
	return task08RowsForRuntime(w.rows, agentruntime.RuntimePluginRemote), nil
}

func task08RowsForRuntime(rows []*models.ExecutorRunning, runtime agentruntime.Runtime) []*models.ExecutorRunning {
	filtered := make([]*models.ExecutorRunning, 0, len(rows))
	for _, row := range rows {
		if row != nil && row.Runtime == runtime {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

type task08DetailedRecoveryBackend struct {
	*MockExecutor
	mu             sync.Mutex
	recoveryCalls  [][]*models.ExecutorRunning
	recoveryResult func([]*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error)
}

func (b *task08DetailedRecoveryBackend) RecoverInstancesDetailed(
	_ context.Context,
	records []*models.ExecutorRunning,
) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
	b.mu.Lock()
	b.recoveryCalls = append(b.recoveryCalls, append([]*models.ExecutorRunning(nil), records...))
	b.mu.Unlock()
	if b.recoveryResult != nil {
		return b.recoveryResult(records)
	}
	return nil, nil, nil
}

func (b *task08DetailedRecoveryBackend) callsSnapshot() [][]*models.ExecutorRunning {
	b.mu.Lock()
	defer b.mu.Unlock()
	calls := make([][]*models.ExecutorRunning, len(b.recoveryCalls))
	for index, call := range b.recoveryCalls {
		calls[index] = append([]*models.ExecutorRunning(nil), call...)
	}
	return calls
}

type task08CloseableMockExecutor struct {
	*MockExecutor
	closed bool
}

func (e *task08CloseableMockExecutor) Close() error {
	e.closed = true
	return nil
}

func task08ManagerWithBackend(t *testing.T, backend ExecutorBackend) (*Manager, *MockEventBus) {
	t.Helper()
	log := newTestLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(backend)
	events := &MockEventBus{}
	mgr := NewManager(newTestRegistry(), events, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	return mgr, events
}

func task08UnknownOutcomes(runtime agentruntime.Runtime) func([]*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
	return func(records []*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
		outcomes := make(map[string]RecoveryCandidateOutcome)
		for _, record := range records {
			if record != nil && record.Runtime == runtime {
				outcomes[record.SessionID] = RecoveryOutcomeUnknown
			}
		}
		return nil, outcomes, nil
	}
}

func TestManagerStartInventoriesSupportedRemoteExecutorsWithoutLocalDocker(t *testing.T) {
	rows := []*models.ExecutorRunning{
		{SessionID: "session-local", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusReady},
		{SessionID: "session-sprites", TaskID: "task-sprites", AgentExecutionID: "exec-sprites", Runtime: agentruntime.RuntimeSprites, Status: models.ExecutorRunningStatusReady},
		{SessionID: "session-ssh", TaskID: "task-ssh", AgentExecutionID: "exec-ssh", Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusReady},
		{SessionID: "session-remote-docker", TaskID: "task-remote-docker", AgentExecutionID: "exec-remote-docker", Runtime: agentruntime.RuntimeRemoteDocker, Status: models.ExecutorRunningStatusReady},
		{SessionID: "session-k8s", TaskID: "task-k8s", AgentExecutionID: "exec-k8s", Runtime: agentruntime.RuntimeKubernetes, Status: models.ExecutorRunningStatusReady},
		{SessionID: "session-plugin", TaskID: "task-plugin", AgentExecutionID: "exec-plugin", Runtime: agentruntime.RuntimePluginRemote, Status: models.ExecutorRunningStatusReady},
		{SessionID: "session-local-docker", Runtime: agentruntime.RuntimeDocker, Status: models.ExecutorRunningStatusReady},
	}
	backend := &MockExecutor{name: executor.NameSSH}
	mgr, _ := task08ManagerWithBackend(t, backend)
	mgr.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{rows: rows})
	task08SetSnapshots(mgr, rows)
	mgr.SetPassthroughLookup(alwaysNotPassthrough)
	mgr.SetRecoveryDeadline(time.Second)
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })

	require.NoError(t, mgr.Start(context.Background()))

	got := make(map[string]int)
	for _, record := range backend.recoverRecords {
		if record != nil {
			got[record.SessionID]++
		}
	}
	for _, sessionID := range []string{
		"session-local", "session-sprites", "session-ssh", "session-remote-docker", "session-k8s", "session-plugin",
	} {
		require.Equal(t, 1, got[sessionID], "expected one recovery candidate for %s", sessionID)
	}
	require.NotContains(t, got, "session-local-docker", "the local Docker daemon is outside remote survival inventory")
}

func TestUnknownRemoteRecoveryRemainsRetryablyGuardedAndRetriesSameRecord(t *testing.T) {
	const (
		sessionID   = "session-pending-remote"
		executionID = "execution-pending-remote"
	)
	row := &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: "task-pending-remote", AgentExecutionID: executionID,
		Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusReady,
	}
	backend := &task08DetailedRecoveryBackend{
		MockExecutor:   &MockExecutor{name: executor.NameSSH},
		recoveryResult: task08UnknownOutcomes(agentruntime.RuntimeSSH),
	}
	mgr, _ := task08ManagerWithBackend(t, backend)
	mgr.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{rows: []*models.ExecutorRunning{row}})
	task08SetSnapshots(mgr, []*models.ExecutorRunning{row})
	mgr.SetPassthroughLookup(alwaysNotPassthrough)
	mgr.SetRecoveryDeadline(time.Second)
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })

	require.NoError(t, mgr.Start(context.Background()))
	guardErr := mgr.recoveryGuard.CheckLaunchAllowed(sessionID)
	require.ErrorIs(t, guardErr, ErrSessionRecoveryGuarded,
		"an unknown remote owner must block a new launch while remaining eligible for another attach attempt")
	require.NotErrorIs(t, guardErr, ErrSessionUnstoppableAgent,
		"transport or ownership uncertainty is retryable, not an unstoppable local process")
	require.Empty(t, backend.stopInstanceCalls,
		"unknown remote evidence must preserve the exact remote instance")

	mgr.pollRemoteStatuses(context.Background())
	calls := backend.callsSnapshot()
	require.GreaterOrEqual(t, len(calls), 2, "the existing remote status pass must retry pending adoption")
	seenSameOwner := false
	for _, call := range calls[1:] {
		for _, record := range call {
			if record != nil && record.SessionID == sessionID && record.AgentExecutionID == executionID {
				seenSameOwner = true
			}
		}
	}
	require.True(t, seenSameOwner, "retry must use the same persisted session and execution identity")
}

func TestPluginRemoteRecoveryRetryUsesAdvancedCheckpointRevision(t *testing.T) {
	const (
		sessionID   = "session-plugin-checkpoint-retry"
		taskID      = "task-plugin-checkpoint-retry"
		executionID = "execution-plugin-checkpoint-retry"
	)
	initialInventory := pluginExecutorInventory{
		PluginID: "plugin-checkpoint-retry", InstallationID: "installation-checkpoint-retry",
		ProviderKey: "remote", ProviderIdentity: "provider-checkpoint-retry",
		EnvironmentID: "environment-checkpoint-retry", EnvironmentGeneration: 9,
		ContractVersion: 1, ProfileID: "profile-checkpoint-retry",
		OperationID: "operation-checkpoint-retry", InputDigest: "digest-checkpoint-retry",
		Phase: pluginExecutorPhaseBootstrapping, Revision: 1, Generation: 1,
	}
	row := &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Runtime: agentruntime.RuntimePluginRemote, Status: models.ExecutorRunningStatusReady,
		Metadata: map[string]interface{}{MetadataKeyPluginExecutor: initialInventory},
	}
	writer := &task08ExecutorInventoryWriter{
		invariantWriter: invariantWriter{prior: row}, rows: []*models.ExecutorRunning{row},
	}
	var attemptedRevisions []uint64
	backend := &task08DetailedRecoveryBackend{
		MockExecutor: &MockExecutor{name: executor.NamePluginRemote},
	}
	backend.recoveryResult = func(records []*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
		if len(records) != 1 {
			t.Fatalf("recovery records = %d, want one", len(records))
		}
		inventory, err := decodePluginExecutorInventory(records[0].Metadata)
		require.NoError(t, err)
		attemptedRevisions = append(attemptedRevisions, inventory.Revision)
		if len(attemptedRevisions) == 1 {
			inventory.Revision++
			inventory.Phase = pluginExecutorPhaseReady
			records[0].Metadata[MetadataKeyPluginExecutor] = inventory
			persisted := cloneRemoteRecoveryRecord(writer.prior)
			persisted.Metadata[MetadataKeyPluginExecutor] = inventory
			writer.prior = persisted
			writer.rows[0] = persisted
			return nil, nil, errors.New("transient authenticated attach failure after checkpoint")
		}
		return nil, nil, nil
	}
	mgr, _ := task08ManagerWithBackend(t, backend)
	mgr.SetExecutorRunningWriter(writer)
	task08SetSnapshots(mgr, []*models.ExecutorRunning{row})
	entry := mgr.queueRemoteRecovery(row, task08RemoteSnapshot(row))
	require.NotNil(t, entry)
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })

	mgr.retryRemoteRecovery(context.Background(), entry)
	mgr.retryRemoteRecovery(context.Background(), entry)

	require.Equal(t, []uint64{1, 2}, attemptedRevisions,
		"a later retry must use the exact persisted plugin checkpoint rather than a stale per-attempt clone")
}

func TestPendingPluginStopAttachUsesAdvancedCheckpointRevision(t *testing.T) {
	const (
		sessionID   = "session-plugin-stop-checkpoint"
		taskID      = "task-plugin-stop-checkpoint"
		executionID = "execution-plugin-stop-checkpoint"
	)
	initialInventory := pluginExecutorInventory{
		PluginID: "plugin-stop-checkpoint", InstallationID: "installation-stop-checkpoint",
		ProviderKey: "remote", ProviderIdentity: "provider-stop-checkpoint",
		EnvironmentID: "environment-stop-checkpoint", EnvironmentGeneration: 4,
		ContractVersion: 1, ProfileID: "profile-stop-checkpoint",
		OperationID: "operation-stop-checkpoint", InputDigest: "digest-stop-checkpoint",
		Phase: pluginExecutorPhaseBootstrapping, Revision: 1, Generation: 1,
	}
	row := &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Runtime: agentruntime.RuntimePluginRemote, Status: models.ExecutorRunningStatusReady,
		Metadata: map[string]interface{}{MetadataKeyPluginExecutor: initialInventory},
	}
	persisted := cloneRemoteRecoveryRecord(row)
	advanced := initialInventory
	advanced.Revision = 2
	advanced.Phase = pluginExecutorPhaseReady
	persisted.Metadata[MetadataKeyPluginExecutor] = advanced
	writer := &task08ExecutorInventoryWriter{
		invariantWriter: invariantWriter{prior: persisted}, rows: []*models.ExecutorRunning{persisted},
	}
	var attemptedRevision uint64
	backend := &task08DetailedRecoveryBackend{
		MockExecutor: &MockExecutor{name: executor.NamePluginRemote},
		recoveryResult: func(records []*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
			if len(records) != 1 {
				t.Fatalf("recovery records = %d, want one", len(records))
			}
			inventory, err := decodePluginExecutorInventory(records[0].Metadata)
			require.NoError(t, err)
			attemptedRevision = inventory.Revision
			return nil, map[string]RecoveryCandidateOutcome{sessionID: RecoveryOutcomeUnknown}, nil
		},
	}
	mgr, _ := task08ManagerWithBackend(t, backend)
	mgr.SetExecutorRunningWriter(writer)
	entry := mgr.queueRemoteRecovery(row, task08RemoteSnapshot(row))
	require.NotNil(t, entry)
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })

	_, outcome, err := mgr.recoverPendingRemoteStopInstance(context.Background(), backend, entry)

	require.NoError(t, err)
	require.Equal(t, RecoveryOutcomeUnknown, outcome)
	require.Equal(t, uint64(2), attemptedRevision,
		"a stop attach must use the current exact-owner plugin checkpoint as a normal retry does")
}

func TestRemoteRecoveryRetriesWhenEnvironmentBecomesReady(t *testing.T) {
	row := &models.ExecutorRunning{
		ID: "session-not-ready", SessionID: "session-not-ready", TaskID: "task-not-ready",
		AgentExecutionID: "execution-not-ready", Runtime: agentruntime.RuntimeSprites,
		Status: models.ExecutorRunningStatusReady,
	}
	snapshot := task08RemoteSnapshot(row)
	snapshot.EnvironmentStatus = "provisioning"
	provider := &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{row.SessionID: snapshot}}
	backend := &task08DetailedRecoveryBackend{
		MockExecutor:   &MockExecutor{name: executor.NameSprites},
		recoveryResult: task08UnknownOutcomes(agentruntime.RuntimeSprites),
	}
	mgr, _ := task08ManagerWithBackend(t, backend)
	mgr.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{rows: []*models.ExecutorRunning{row}})
	mgr.workspaceInfoProvider = provider
	mgr.SetPassthroughLookup(alwaysNotPassthrough)
	mgr.SetRecoveryDeadline(time.Second)
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })

	require.NoError(t, mgr.Start(context.Background()))
	for _, call := range backend.callsSnapshot() {
		for _, record := range call {
			require.NotEqual(t, row.SessionID, record.SessionID,
				"an ineligible environment must not receive an attach attempt")
		}
	}
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)

	provider.snapshots[row.SessionID].EnvironmentStatus = "ready"
	mgr.retryPendingRemoteRecoveries(context.Background())

	var retriedCandidate bool
	for _, call := range backend.callsSnapshot() {
		for _, record := range call {
			if record != nil && record.SessionID == row.SessionID && record.AgentExecutionID == row.AgentExecutionID {
				retriedCandidate = true
			}
		}
	}
	require.True(t, retriedCandidate, "the retained candidate should retry after its environment becomes ready")
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded,
		"an unknown attach result remains guarded for a later retry")
}

func TestStaleRemoteRecoveryEntryCannotReleaseSuccessorGuard(t *testing.T) {
	mgr, _ := task08ManagerWithBackend(t, &MockExecutor{name: executor.NameSSH})
	oldRecord := &models.ExecutorRunning{
		SessionID: "session-remote-generation", TaskID: "task-remote-generation",
		AgentExecutionID: "execution-old", Runtime: agentruntime.RuntimeSSH,
	}
	newRecord := *oldRecord
	newRecord.AgentExecutionID = "execution-new"
	oldEntry := mgr.queueRemoteRecovery(oldRecord, nil)
	newEntry := mgr.queueRemoteRecovery(&newRecord, nil)

	mgr.removeRemoteRecovery(oldEntry, true)

	require.Same(t, newEntry, mgr.pendingRemoteRecoveryForSession(oldRecord.SessionID),
		"stale removal must not clear a newer recovery generation")
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(oldRecord.SessionID), ErrSessionRecoveryGuarded,
		"the successor recovery must retain its launch guard")
}

func TestExplicitStopBetweenRemoteDialAndTrackFencesCandidate(t *testing.T) {
	row := &models.ExecutorRunning{
		SessionID: "session-stop-before-track", TaskID: "task-stop-before-track",
		AgentExecutionID: "execution-stop-before-track", Runtime: agentruntime.RuntimeSprites,
	}
	provider := &task08BlockingSnapshotProvider{
		task08RemoteSnapshotProvider: task08RemoteSnapshotProvider{
			snapshots: map[string]*RemoteRecoverySnapshot{row.SessionID: task08RemoteSnapshot(row)},
		},
		blockAt: 1, entered: make(chan struct{}), release: make(chan struct{}),
	}
	var stopRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/session":
			_ = json.NewEncoder(w).Encode(agentctl.AgentSessionAssociation{
				InstanceID: row.AgentExecutionID, SessionID: row.SessionID, IncarnationID: "incarnation-1",
				HarnessGeneration: 2, NativeSessionID: "native-1", AgentStatus: agentProcessStatusRunning,
			})
		case "/api/v1/agent/delivery":
			_ = json.NewEncoder(w).Encode(agentctl.DeliveryStatus{
				StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
				SessionID:         row.SessionID, IncarnationID: "incarnation-1", HarnessGeneration: 2, StreamID: "stream-1",
			})
		case "/api/v1/stop":
			stopRequests.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	backend := &task08DetailedRecoveryBackend{
		MockExecutor: &MockExecutor{name: executor.NameSprites},
		recoveryResult: func(records []*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
			return []*ExecutorInstance{{
				InstanceID: row.AgentExecutionID, TaskID: row.TaskID, SessionID: row.SessionID,
				RuntimeName: row.Runtime, Client: agentctl.NewClient(host, port, newTestLogger()),
			}}, nil, nil
		},
	}
	mgr, _ := task08ManagerWithBackend(t, backend)
	mgr.workspaceInfoProvider = provider
	entry := mgr.queueRemoteRecovery(row, task08RemoteSnapshot(row))
	require.True(t, mgr.beginRemoteRecovery(entry))
	var discardCalls atomic.Int32
	ri := &ExecutorInstance{
		InstanceID: row.AgentExecutionID, TaskID: row.TaskID, SessionID: row.SessionID,
		RuntimeName: row.Runtime, Client: agentctl.NewClient("127.0.0.1", 1, newTestLogger()),
		DiscardRecovery: func() { discardCalls.Add(1) },
	}
	execution := &AgentExecution{
		ID: row.AgentExecutionID, TaskID: row.TaskID, SessionID: row.SessionID,
		RuntimeName: executor.NameSprites, agentctl: ri.Client,
	}
	trackDone := make(chan error, 1)
	go func() {
		trackDone <- mgr.trackRecoveredRemoteExecution(context.Background(), entry, execution, ri)
	}()
	<-provider.entered // The remote candidate has been connected; pause before the final generation check.

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- mgr.StopAgentWithReason(context.Background(), row.AgentExecutionID, StopReasonTaskDeleted, false)
	}()
	require.Eventually(t, func() bool {
		mgr.remoteRecoveryMu.Lock()
		defer mgr.remoteRecoveryMu.Unlock()
		return entry.stopRequested
	}, time.Second, time.Millisecond, "explicit Stop must fence the in-flight candidate before it can register")

	close(provider.release)
	trackErr := <-trackDone
	require.ErrorIs(t, trackErr, errRemoteRecoveryFenced,
		"a candidate whose pending owner was stopped must fail its final tracking fence")
	mgr.finishRemoteRecoveryAttempt(entry)
	mgr.discardRemoteRecoveryInstance(ri)
	require.NoError(t, <-stopDone, "explicit stop must attach again and stop the exact authenticated owner")
	require.Len(t, backend.stopInstanceCalls, 1)
	require.Equal(t, row.AgentExecutionID, backend.stopInstanceCalls[0].InstanceID)
	require.EqualValues(t, 1, stopRequests.Load(), "the authenticated agentctl Stop endpoint must be called")
	require.EqualValues(t, 1, discardCalls.Load(), "a fenced candidate must release host-side transport once")
	_, tracked := mgr.executionStore.Get(row.AgentExecutionID)
	require.False(t, tracked, "a stopped candidate must not be resurrected in the execution store")
	require.NoError(t, mgr.recoveryGuard.CheckLaunchAllowed(row.SessionID),
		"successful explicit cleanup releases the pending launch guard")
}

func TestStopDuringReplayWakesAfterRemoteRecoveryCompletes(t *testing.T) {
	row := &models.ExecutorRunning{
		SessionID: "session-stop-during-replay", TaskID: "task-stop-during-replay",
		AgentExecutionID: "execution-stop-during-replay", Runtime: agentruntime.RuntimeSprites,
	}
	backend := &MockExecutor{name: executor.NameSprites}
	mgr, _ := task08ManagerWithBackend(t, backend)
	entry := mgr.queueRemoteRecovery(row, task08RemoteSnapshot(row))
	require.True(t, mgr.beginRemoteRecovery(entry))
	execution := &AgentExecution{
		ID: row.AgentExecutionID, TaskID: row.TaskID, SessionID: row.SessionID,
		RuntimeName: executor.NameSprites,
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- mgr.StopAgentWithReason(context.Background(), row.AgentExecutionID, StopReasonTaskDeleted, true)
	}()
	require.Eventually(t, func() bool {
		mgr.remoteRecoveryMu.Lock()
		defer mgr.remoteRecoveryMu.Unlock()
		return entry.stopRequested
	}, time.Second, time.Millisecond)

	// Successful recovery removes the pending map entry before its attempt
	// owner returns. Stop still waits for the attempt-owned completion signal.
	require.True(t, mgr.completeRemoteRecovery(entry))
	mgr.finishRemoteRecoveryAttempt(entry)
	require.NoError(t, <-stopDone, "Stop must wake after replay completion and stop the tracked execution")
	require.Len(t, backend.stopInstanceCalls, 1)
}

func TestRunOwnedRemoteInventoryStaysOnRunOwnerRecoveryPath(t *testing.T) {
	owner := ExecutionOwner{
		Kind: ExecutionOwnerRun, WorkspaceID: "workspace-run", RunID: "run-id",
		RunSessionID: "run-session-id", Attempt: 2, AgentProfileID: "office-agent",
	}
	row := &models.ExecutorRunning{
		SessionID: owner.RunSessionID, AgentExecutionID: "run-execution", Runtime: agentruntime.RuntimeSprites,
		Status:   models.ExecutorRunningStatusRunning,
		Metadata: map[string]interface{}{runExecutionOwnerMetadataKey: owner},
	}
	writer := &task08ExecutorInventoryWriter{invariantWriter: invariantWriter{prior: row}, rows: []*models.ExecutorRunning{row}}
	backend := &MockExecutor{name: executor.NameSprites}
	mgr, _ := task08ManagerWithBackend(t, backend)
	mgr.SetExecutorRunningWriter(writer)
	mgr.SetPassthroughLookup(alwaysNotPassthrough)
	mgr.SetRecoveryDeadline(time.Second)
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })

	require.NoError(t, mgr.Start(context.Background()))
	require.Nil(t, mgr.pendingRemoteRecoveryForSession(owner.RunSessionID),
		"run-owned inventory must not enter task-session remote adoption")
	require.NoError(t, mgr.recoveryGuard.CheckLaunchAllowed(owner.RunSessionID),
		"the task-session guard must not block run-owner admission")
	require.Empty(t, backend.recoverRecords,
		"run-owned inventory must remain outside task-session remote recovery")

	require.NoError(t, mgr.StopRunOwnerForRecovery(context.Background(), owner),
		"the existing run-owner recovery path should still stop its persisted predecessor")
	require.Len(t, backend.stopInstanceCalls, 1)
}

func TestStopAllAgentsAndManagerStopDetachRemoteExecutionsRegardlessOfLocalFlag(t *testing.T) {
	remoteRuntimes := []executor.Name{
		executor.NameSprites, executor.NameSSH, executor.NameRemoteDocker, executor.NameKubernetes, executor.NamePluginRemote,
	}
	for _, runtime := range remoteRuntimes {
		for _, survivalEnabled := range []bool{false, true} {
			t.Run(string(runtime)+"/survival="+map[bool]string{false: "off", true: "on"}[survivalEnabled], func(t *testing.T) {
				backend := &task08CloseableMockExecutor{MockExecutor: &MockExecutor{name: runtime}}
				mgr, eventBus := task08ManagerWithBackend(t, backend)
				mgr.SetAgentSurvivalEnabled(survivalEnabled)
				execution := &AgentExecution{
					ID: "exec-" + string(runtime), SessionID: "session-" + string(runtime), TaskID: "task-" + string(runtime),
					RuntimeName: runtime, agentctl: agentctl.NewClient("127.0.0.1", 12345, newTestLogger()),
				}
				require.NoError(t, mgr.executionStore.Add(execution))

				require.NoError(t, mgr.StopAllAgents(context.Background()))
				require.NoError(t, mgr.Stop())

				require.Empty(t, backend.stopInstanceCalls, "graceful shutdown must preserve remote compute")
				require.True(t, backend.closed, "manager shutdown must close host-side runtime connections")
				_, exists := mgr.executionStore.Get(execution.ID)
				require.False(t, exists, "shutdown must release local execution tracking")
				require.False(t, hasAgentStoppedEvent(eventBus.PublishedEvents),
					"a preserved remote conversation must not publish a terminal stop")
			})
		}
	}

	localDocker := &task08CloseableMockExecutor{MockExecutor: &MockExecutor{name: executor.NameDocker}}
	mgr, eventBus := task08ManagerWithBackend(t, localDocker)
	execution := &AgentExecution{ID: "exec-local-docker", SessionID: "session-local-docker", RuntimeName: executor.NameDocker}
	require.NoError(t, mgr.executionStore.Add(execution))
	require.NoError(t, mgr.StopAllAgents(context.Background()))
	require.NoError(t, mgr.Stop())
	require.Len(t, localDocker.stopInstanceCalls, 1, "local Docker continues to stop with its backend")
	require.True(t, hasAgentStoppedEvent(eventBus.PublishedEvents))
}

func TestExplicitStopOfPendingRemoteExecutionStopsExactPersistedOwnerAndClearsGuard(t *testing.T) {
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
		sshScriptRule{match: "cat -- '/remote/session/agentctl.pid'", result: sshOut("4242")},
		sshScriptRule{match: "kill 4242", result: sshOK},
	).handle)
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(NewSSHExecutor(nil, nil, nil, log))
	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
	row := &models.ExecutorRunning{
		ID: "session-pending-stop", SessionID: "session-pending-stop", TaskID: "task-pending-stop",
		AgentExecutionID: "execution-pending-stop", Runtime: agentruntime.RuntimeSSH, Metadata: metadata,
	}
	store := &restartSSHInventoryStore{rows: []*models.ExecutorRunning{row}}
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(store)
	task08SetSnapshots(mgr, []*models.ExecutorRunning{row})
	mgr.queueRemoteRecovery(row, task08RemoteSnapshot(row))

	err := mgr.StopAgentWithReason(context.Background(), "execution-pending-stop", StopReasonTaskDeleted, true)

	require.NoError(t, err)
	_, exactKillIssued := server.lastCommandContaining("kill 4242")
	require.True(t, exactKillIssued, "explicit stop must reach the persisted remote owner even while adoption is pending")
	require.NoError(t, mgr.recoveryGuard.CheckLaunchAllowed("session-pending-stop"),
		"a successful explicit stop must remove that session's pending-recovery guard")
}

func TestRemoteRetryCancellationDoesNotStopRemoteInstance(t *testing.T) {
	backend := &task08DetailedRecoveryBackend{
		MockExecutor:   &MockExecutor{name: executor.NameSSH},
		recoveryResult: task08UnknownOutcomes(agentruntime.RuntimeSSH),
	}
	mgr, _ := task08ManagerWithBackend(t, backend)
	mgr.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{rows: []*models.ExecutorRunning{{
		ID: "session-canceled", SessionID: "session-canceled", TaskID: "task-canceled",
		AgentExecutionID: "execution-canceled", Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusReady,
	}}})
	task08SetSnapshots(mgr, []*models.ExecutorRunning{{
		ID: "session-canceled", SessionID: "session-canceled", TaskID: "task-canceled",
		AgentExecutionID: "execution-canceled", Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusReady,
	}})
	mgr.SetPassthroughLookup(alwaysNotPassthrough)
	mgr.SetRecoveryDeadline(time.Second)
	require.NoError(t, mgr.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })
	initialCalls := len(backend.callsSnapshot())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mgr.pollRemoteStatuses(ctx)

	require.Len(t, backend.callsSnapshot(), initialCalls, "a canceled retry pass must not start another remote attach")
	require.Empty(t, backend.stopInstanceCalls,
		"canceling a bounded remote attach attempt must preserve the exact remote instance")
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed("session-canceled"), ErrSessionRecoveryGuarded)
	require.NotErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed("session-canceled"), ErrSessionUnstoppableAgent)
}
