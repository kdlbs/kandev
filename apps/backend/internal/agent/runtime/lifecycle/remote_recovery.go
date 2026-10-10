package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const (
	remoteRecoveryAttemptTimeout = 15 * time.Second
	remoteRecoveryBatchSize      = 4
)

var errRemoteRecoveryFenced = errors.New("remote recovery candidate was fenced")

type remoteRecoveryRetry struct {
	record               *models.ExecutorRunning
	snapshot             *RemoteRecoverySnapshot
	inProgress           bool
	attemptDone          chan struct{}
	stopRequested        bool
	stopReason           string
	stopForce            bool
	stopRuntimeCompleted bool
}

func isRemoteRecoveryRuntime(runtime agentruntime.Runtime) bool {
	switch runtime {
	case agentruntime.RuntimeSprites, agentruntime.RuntimeSSH, agentruntime.RuntimeRemoteDocker,
		agentruntime.RuntimeKubernetes, agentruntime.RuntimePluginRemote:
		return true
	default:
		return false
	}
}

func isRemoteRecoveryBackendShutdown(runtime executor.Name) bool {
	return isRemoteRecoveryRuntime(runtime)
}

func cloneRemoteRecoveryRecord(record *models.ExecutorRunning) *models.ExecutorRunning {
	if record == nil {
		return nil
	}
	copyRecord := *record
	if record.Metadata != nil {
		copyRecord.Metadata = make(map[string]interface{}, len(record.Metadata))
		for key, value := range record.Metadata {
			copyRecord.Metadata[key] = value
		}
	}
	if record.LastSeenAt != nil {
		lastSeen := *record.LastSeenAt
		copyRecord.LastSeenAt = &lastSeen
	}
	return &copyRecord
}

func (m *Manager) remoteRecoverySnapshot(ctx context.Context, sessionID string) (*RemoteRecoverySnapshot, error) {
	provider, ok := m.workspaceInfoProvider.(RemoteRecoverySnapshotProvider)
	if !ok || provider == nil {
		return nil, errors.New("read-only remote recovery snapshot provider is unavailable")
	}
	return provider.GetRemoteRecoverySnapshot(ctx, sessionID)
}

func remoteRecoveryOwnerUnsupported(record *models.ExecutorRunning) bool {
	if record == nil || record.Metadata == nil {
		return false
	}
	_, isRunOwned := record.Metadata[runExecutionOwnerMetadataKey]
	return isRunOwned
}

func (m *Manager) pendingRemoteRecoveryForSession(sessionID string) *remoteRecoveryRetry {
	if sessionID == "" {
		return nil
	}
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	return m.remoteRecoveryPending[sessionID]
}

func (m *Manager) remoteRecoveryForExecution(executionID string) *remoteRecoveryRetry {
	if executionID == "" {
		return nil
	}
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	for _, entry := range m.remoteRecoveryPending {
		if entry != nil && entry.record != nil && entry.record.AgentExecutionID == executionID {
			return entry
		}
	}
	return nil
}

func (m *Manager) removeRemoteRecovery(entry *remoteRecoveryRetry, releaseGuard bool) {
	if entry == nil || entry.record == nil {
		return
	}
	m.remoteRecoveryMu.Lock()
	if m.remoteRecoveryPending[entry.record.SessionID] != entry {
		m.remoteRecoveryMu.Unlock()
		return
	}
	delete(m.remoteRecoveryPending, entry.record.SessionID)
	if releaseGuard {
		m.recoveryGuard.Release(entry.record.SessionID)
	}
	m.remoteRecoveryMu.Unlock()
}

func (m *Manager) establishRemoteRecoveryCandidates(
	ctx context.Context,
	records []*models.ExecutorRunning,
) []*models.ExecutorRunning {
	eligible := make([]*models.ExecutorRunning, 0, len(records))
	for _, record := range records {
		if remoteRecoveryOwnerUnsupported(record) {
			m.logger.Debug("run-owned remote inventory remains on the run-owner recovery path",
				zap.String("session_id", record.SessionID), zap.String("execution_id", record.AgentExecutionID))
			continue
		}
		entry := m.queueRemoteRecovery(record, nil)
		if entry == nil {
			continue
		}
		attemptCtx, cancel := m.remoteAttemptContext(ctx)
		snapshot, err := m.remoteRecoverySnapshot(attemptCtx, record.SessionID)
		cancel()
		if err != nil {
			m.logger.Warn("remote recovery owner snapshot is unavailable; preserving the remote execution behind a retryable guard",
				zap.String("session_id", record.SessionID), zap.Error(err))
			continue
		}
		if !remoteRecoverySnapshotMatchesRecord(snapshot, record) || remoteRecoverySnapshotFences(snapshot) {
			m.removeRemoteRecovery(entry, true)
			continue
		}
		if !m.pinRemoteRecoverySnapshot(entry, snapshot) {
			continue
		}
		if err := validateRemoteRecoverySnapshot(snapshot, record); err != nil {
			m.logger.Warn("remote recovery owner snapshot is not eligible yet; preserving the remote execution",
				zap.String("session_id", record.SessionID), zap.Error(err))
			continue
		}
		if remoteRecoveryOwnerUnsupported(record) {
			m.logger.Warn("remote recovery for a run-owned execution is not supported by the task-session snapshot path; preserving its owner guard",
				zap.String("session_id", record.SessionID), zap.String("task_id", record.TaskID))
			continue
		}
		eligible = append(eligible, cloneRemoteRecoveryRecord(record))
	}
	return eligible
}

func (m *Manager) retryPendingRemoteRecoveries(ctx context.Context) {
	for _, entry := range m.pendingRemoteRecoveryBatch() {
		m.retryRemoteRecovery(ctx, entry)
	}
}

func (m *Manager) retryRemoteRecovery(parent context.Context, entry *remoteRecoveryRetry) {
	if entry == nil || entry.record == nil {
		m.finishRemoteRecoveryAttempt(entry)
		return
	}
	if !m.beginRemoteRecovery(entry) {
		return
	}
	defer m.finishRemoteRecoveryAttempt(entry)
	ctx, cancel := m.remoteAttemptContext(parent)
	defer cancel()
	if ctx.Err() != nil {
		return
	}
	current, ready := m.currentRemoteRecoveryOwner(ctx, entry)
	if !ready {
		return
	}
	if m.retryPendingRemoteStop(ctx, entry) {
		return
	}
	m.attachRemoteRecovery(ctx, entry, current)
}

func (m *Manager) currentRemoteRecoveryOwner(
	ctx context.Context,
	entry *remoteRecoveryRetry,
) (*RemoteRecoverySnapshot, bool) {
	current, err := m.remoteRecoverySnapshot(ctx, entry.record.SessionID)
	if err != nil {
		m.logger.Debug("remote recovery owner snapshot retry failed", zap.String("session_id", entry.record.SessionID), zap.Error(err))
		return nil, false
	}
	if entry.snapshot == nil {
		if !remoteRecoverySnapshotMatchesRecord(current, entry.record) || remoteRecoverySnapshotFences(current) {
			m.removeRemoteRecovery(entry, true)
			return nil, false
		}
		if !m.pinRemoteRecoverySnapshot(entry, current) {
			return nil, false
		}
	}
	if !sameRemoteRecoveryOwner(entry.snapshot, current, entry.record) {
		m.removeRemoteRecovery(entry, true)
		return nil, false
	}
	return current, true
}

func (m *Manager) retryPendingRemoteStop(ctx context.Context, entry *remoteRecoveryRetry) bool {
	m.remoteRecoveryMu.Lock()
	stopRequested, reason, force := entry.stopRequested, entry.stopReason, entry.stopForce
	m.remoteRecoveryMu.Unlock()
	if !stopRequested {
		return false
	}
	if execution, exists := m.executionStore.GetBySessionID(entry.record.SessionID); exists && execution.ID == entry.record.AgentExecutionID {
		return true
	}
	stopped, err := m.stopPendingRemoteRecovery(ctx, entry, reason, force)
	if err != nil {
		m.logger.Debug("pending remote stop remains retryable", zap.String("session_id", entry.record.SessionID), zap.Error(err))
	} else if stopped {
		m.completeRemoteRecovery(entry)
	}
	return true
}

func (m *Manager) attachRemoteRecovery(ctx context.Context, entry *remoteRecoveryRetry, snapshot *RemoteRecoverySnapshot) {
	if err := validateRemoteRecoverySnapshot(snapshot, entry.record); err != nil || remoteRecoveryOwnerUnsupported(entry.record) {
		if err != nil {
			m.logger.Debug("remote recovery remains ineligible", zap.String("session_id", entry.record.SessionID), zap.Error(err))
		}
		return
	}
	if m.executorRegistry == nil {
		return
	}
	backend, err := m.executorRegistry.GetBackend(entry.record.Runtime)
	if err != nil {
		m.logger.Debug("remote recovery runtime is unavailable", zap.String("session_id", entry.record.SessionID), zap.Error(err))
		return
	}
	attemptRecord, err := m.remoteRecoveryAttemptRecord(ctx, entry)
	if err != nil {
		m.logger.Debug("remote recovery inventory refresh failed; preserving owner",
			zap.String("session_id", entry.record.SessionID), zap.Error(err))
		return
	}
	credentialed := m.revealRemoteRecoveryCredentials(ctx, attemptRecord)
	var recovered []*ExecutorInstance
	if detailed, ok := backend.(DetailedRecoveryBackend); ok {
		recovered, _, err = detailed.RecoverInstancesDetailed(ctx, []*models.ExecutorRunning{credentialed})
	} else {
		recovered, err = backend.RecoverInstances(ctx, []*models.ExecutorRunning{credentialed})
	}
	if err != nil {
		m.logger.Debug("remote recovery attach failed; preserving owner", zap.String("session_id", entry.record.SessionID), zap.Error(err))
		m.discardRemoteRecoveryInstances(recovered)
		return
	}
	ri := exactRecoveredRemoteInstance(recovered, entry.record)
	for _, candidate := range recovered {
		if candidate != ri {
			m.discardRemoteRecoveryInstance(candidate)
		}
	}
	if ri == nil {
		return
	}
	if err := m.processRecoveredRemoteInstance(ctx, entry, ri); err != nil {
		m.logger.Debug("remote recovery proof or registration failed; preserving owner",
			zap.String("session_id", entry.record.SessionID), zap.Error(err))
		m.discardRemoteRecoveryInstance(ri)
	}
}

func (m *Manager) processRecoveredRemoteInstance(ctx context.Context, entry *remoteRecoveryRetry, ri *ExecutorInstance) error {
	if err := m.proveRecoveredRemoteInstance(ctx, entry, ri); err != nil {
		return err
	}
	current, err := m.remoteRecoverySnapshot(ctx, entry.record.SessionID)
	if err != nil {
		return fmt.Errorf("re-read remote owner before tracking: %w", err)
	}
	if !sameRemoteRecoveryOwner(entry.snapshot, current, entry.record) {
		m.removeRemoteRecovery(entry, true)
		return errRemoteRecoveryFenced
	}
	if err := validateRemoteRecoverySnapshot(current, entry.record); err != nil {
		return err
	}
	if remoteRecoveryOwnerUnsupported(entry.record) {
		return errors.New("run-owned remote recovery is unsupported by this task-session path")
	}
	execution, err := m.prepareRecoveredRemoteExecution(ctx, entry.record, current, ri)
	if err != nil {
		return fmt.Errorf("reconstruct recovered remote execution: %w", err)
	}
	return m.trackRecoveredRemoteExecution(ctx, entry, execution, ri)
}

func (m *Manager) proveRecoveredRemoteInstance(ctx context.Context, entry *remoteRecoveryRetry, ri *ExecutorInstance) error {
	if entry == nil || entry.record == nil || entry.snapshot == nil || ri == nil {
		return errors.New("remote recovery candidate is incomplete")
	}
	if ri.RuntimeName != entry.record.Runtime || ri.SessionID != entry.record.SessionID ||
		ri.InstanceID != entry.record.AgentExecutionID || ri.TaskID != entry.record.TaskID {
		return errors.New("runtime returned a different persisted remote execution identity")
	}
	if entry.snapshot.RecoverySourcePresent {
		if ri.expectedDeliveryStreamID != "" && ri.expectedDeliveryStreamID != entry.snapshot.RecoveryStreamID {
			return errors.New("remote recovery stream expectation changed")
		}
		ri.expectedDeliveryStreamID = entry.snapshot.RecoveryStreamID
	}
	if err := m.captureAuthenticatedAgentSessionEvidence(ctx, ri); err != nil {
		return fmt.Errorf("capture authenticated agent-session evidence: %w", err)
	}
	if err := validateRemoteRecoveryDeliveryEvidence(entry.snapshot, ri); err != nil {
		return err
	}
	return nil
}

func (m *Manager) prepareRecoveredRemoteExecution(
	ctx context.Context,
	record *models.ExecutorRunning,
	snapshot *RemoteRecoverySnapshot,
	ri *ExecutorInstance,
) (*AgentExecution, error) {
	if remoteRecoveryOwnerUnsupported(record) {
		return nil, errors.New("run-owned remote execution requires its owner admission path")
	}
	metadata := recoveredRemoteMetadata(record, ri)
	workspacePath := ri.WorkspacePath
	if workspacePath == "" {
		workspacePath = record.WorktreePath
	}
	originalWorkspacePath := getMetadataString(metadata, MetadataKeyOriginalWorkspacePath)
	if originalWorkspacePath == "" {
		originalWorkspacePath = workspacePath
	}
	profileID := strings.TrimSpace(record.ExecutionProfileID)
	if profileID == "" || ri.AgentProfileID != "" && ri.AgentProfileID != profileID {
		return nil, errors.New("remote recovery profile identity changed")
	}
	execution := &AgentExecution{
		ID:                    record.AgentExecutionID,
		TaskID:                record.TaskID,
		SessionID:             record.SessionID,
		WorkspaceID:           snapshot.TaskWorkspaceID,
		TaskEnvironmentID:     snapshot.TaskEnvironmentID,
		AgentProfileID:        profileID,
		ExecutorType:          getMetadataString(metadata, MetadataKeyExecutorType),
		ContainerID:           ri.ContainerID,
		ContainerIP:           ri.ContainerIP,
		WorkspacePath:         workspacePath,
		OriginalWorkspacePath: originalWorkspacePath,
		RuntimeName:           record.Runtime,
		Status:                v1.AgentStatusRunning,
		StartedAt:             time.Now(),
		metadata:              metadata,
		agentctl:              ri.Client,
		promptDoneCh:          make(chan PromptCompletionSignal, 1),
		RunID:                 ri.Env["KANDEV_RUN_ID"],
		WorkspaceSourceRoots:  ri.WorkspaceSourceRoots,
		ACPSessionID:          ri.ProviderSessionID,
	}
	if execution.TaskID == "" || execution.SessionID == "" || execution.ID == "" {
		return nil, errors.New("remote recovery execution identity is incomplete")
	}
	execution.setSessionInitialized(true)
	execution.setRuntimeEnvironment(ri.Env)
	execution.OfficeAgentProfileID = getMetadataString(metadata, MetadataKeyOfficeAgentProfileID)
	applyRemoteLegacySessionEvidence(execution, ri)
	if err := m.reDeriveRecoveredAgentIdentity(ctx, execution); err != nil {
		return nil, err
	}
	if err := m.restoreRecoveredDelivery(ctx, execution, ri); err != nil {
		return nil, err
	}
	if err := m.restoreRecoveredSessionSettingsSource(ctx, execution); err != nil {
		return nil, err
	}
	return execution, nil
}

func recoveredRemoteMetadata(record *models.ExecutorRunning, ri *ExecutorInstance) map[string]interface{} {
	metadata := cloneRemoteRecoveryRecord(record).Metadata
	if ri.Metadata == nil {
		return metadata
	}
	metadata = make(map[string]interface{}, len(ri.Metadata))
	for key, value := range ri.Metadata {
		metadata[key] = value
	}
	return metadata
}

func applyRemoteLegacySessionEvidence(execution *AgentExecution, ri *ExecutorInstance) {
	if !ri.DeliveryLegacyEvidence || ri.agentSessionAssociation == nil {
		return
	}
	execution.DeliveryIncarnationID = ri.agentSessionAssociation.IncarnationID
	execution.DeliveryHarnessGeneration = ri.agentSessionAssociation.HarnessGeneration
	execution.ACPSessionID = ri.agentSessionAssociation.NativeSessionID
}

func (m *Manager) trackRecoveredRemoteExecution(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	execution *AgentExecution,
	ri *ExecutorInstance,
) error {
	if entry == nil || entry.record == nil || execution == nil || ri == nil {
		return errors.New("remote recovery tracking input is incomplete")
	}
	if err := m.validateCurrentRemoteRecoveryOwner(ctx, entry); err != nil {
		return err
	}
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	if err := m.registerRecoveredRemoteExecution(entry, execution); err != nil {
		return err
	}
	endInitSpan := m.initializeTrackedRemoteExecution(execution)
	defer endInitSpan()
	if err := m.replayAndPublishRecoveredRemoteExecution(ctx, entry, execution); err != nil {
		return err
	}
	m.finishTrackedRemoteExecution(ctx, entry, execution)
	return nil
}

func (m *Manager) validateCurrentRemoteRecoveryOwner(ctx context.Context, entry *remoteRecoveryRetry) error {
	// Read storage before taking the per-session admission lock. The pending
	// entry identity is the generation fence shared with explicit Stop.
	current, err := m.remoteRecoverySnapshot(ctx, entry.record.SessionID)
	if err != nil {
		return fmt.Errorf("re-read remote owner before tracking: %w", err)
	}
	if !sameRemoteRecoveryOwner(entry.snapshot, current, entry.record) {
		m.removeRemoteRecovery(entry, true)
		return errRemoteRecoveryFenced
	}
	return validateRemoteRecoverySnapshot(current, entry.record)
}

func (m *Manager) registerRecoveredRemoteExecution(entry *remoteRecoveryRetry, execution *AgentExecution) error {
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	if m.remoteRecoveryPending[entry.record.SessionID] != entry || entry.stopRequested || m.shuttingDown.Load() {
		return errRemoteRecoveryFenced
	}
	select {
	case <-m.stopCh:
		return errRemoteRecoveryFenced
	default:
	}
	if _, exists := m.executionStore.GetBySessionID(entry.record.SessionID); exists {
		return errors.New("another execution is already tracked for the remote session")
	}
	if err := m.executionStore.Add(execution); err != nil {
		return fmt.Errorf("track recovered remote execution: %w", err)
	}
	// Keep the pending generation until replay and publication complete. Stop
	// can remove it while waiting on remoteInstanceLifecycleMu, which fences a
	// failed attempt from retrying after that explicit stop.
	return nil
}

func (m *Manager) initializeTrackedRemoteExecution(execution *AgentExecution) func() {
	if client, releaseClient := execution.AcquireAgentCtlClient(); client != nil {
		client.SetTraceContext(execution.SessionTraceContext())
		releaseClient()
	}
	m.setRuntimeInterest(execution.SessionID, true)
	m.markSessionRetracked(execution.SessionID)
	_, recoverySpan := tracing.TraceSessionRecovered(context.Background(), execution.TaskID, execution.SessionID, execution.ID)
	execution.SetSessionSpan(recoverySpan)
	_, initSpan := tracing.TraceSessionInit(execution.SessionTraceContext(), execution.TaskID, execution.SessionID, execution.ID)
	return func() { initSpan.End() }
}

func (m *Manager) replayAndPublishRecoveredRemoteExecution(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	execution *AgentExecution,
) error {
	if execution.DeliveryMode != DurableDeliveryV1 {
		m.publishRecoveredExecutionRunning(ctx, execution)
		return nil
	}
	if durableRecoveryHasPendingWork(execution) {
		m.publishRecoveredExecutionRunning(ctx, execution)
	}
	if err := m.streamManager.ReplayRecoveredDelivery(ctx, execution); err != nil {
		m.removeFailedRemoteReplay(entry, execution)
		return fmt.Errorf("replay recovered remote delivery: %w", err)
	}
	if !durableRecoveryHasPendingWork(execution) && execution.Status != v1.AgentStatusReady {
		m.publishRecoveredExecutionReady(ctx, execution)
	}
	return nil
}

func (m *Manager) removeFailedRemoteReplay(entry *remoteRecoveryRetry, execution *AgentExecution) {
	m.remoteRecoveryMu.Lock()
	stopFenced := m.remoteRecoveryPending[entry.record.SessionID] != entry
	if !stopFenced {
		m.RemoveExecution(execution.ID)
	}
	m.remoteRecoveryMu.Unlock()
	if !stopFenced {
		m.setRuntimeInterest(execution.SessionID, false)
	}
}

func (m *Manager) finishTrackedRemoteExecution(ctx context.Context, entry *remoteRecoveryRetry, execution *AgentExecution) {
	if client, releaseClient := execution.AcquireAgentCtlClient(); client != nil {
		m.pushTaskBaseBranches(ctx, execution.TaskID, execution.ID, client)
		m.pushTaskComparisonTargets(ctx, execution.TaskID, execution.ID, client)
		releaseClient()
	}
	m.completeRemoteRecovery(entry)
	if m.streamManager != nil {
		go m.streamManager.ReconnectAll(execution)
	}
}

func exactRecoveredRemoteInstance(instances []*ExecutorInstance, record *models.ExecutorRunning) *ExecutorInstance {
	for _, instance := range instances {
		if instance != nil && record != nil && instance.SessionID == record.SessionID &&
			instance.InstanceID == record.AgentExecutionID && instance.RuntimeName == record.Runtime {
			return instance
		}
	}
	return nil
}

func validateRemoteRecoveryDeliveryEvidence(snapshot *RemoteRecoverySnapshot, instance *ExecutorInstance) error {
	if snapshot == nil || !snapshot.RecoverySourcePresent {
		return nil
	}
	status := instance.DeliveryStatus
	if status == nil || !status.Durable || status.StreamID != snapshot.RecoveryStreamID ||
		status.IncarnationID != snapshot.RecoveryIncarnationID ||
		status.HarnessGeneration != uint64(snapshot.RecoveryHarnessGeneration) {
		return errors.New("authenticated remote delivery evidence does not match the pinned interrupted submission")
	}
	for _, submission := range status.Submissions {
		if submission.ID == snapshot.RecoverySubmissionID && submission.SessionID == snapshot.SessionID &&
			submission.IncarnationID == snapshot.RecoveryIncarnationID &&
			submission.HarnessGeneration == uint64(snapshot.RecoveryHarnessGeneration) {
			return nil
		}
	}
	return errors.New("authenticated remote delivery descriptor omits the pinned interrupted submission")
}

func (m *Manager) discardRemoteRecoveryInstances(instances []*ExecutorInstance) {
	for _, instance := range instances {
		m.discardRemoteRecoveryInstance(instance)
	}
}

func (m *Manager) discardRemoteRecoveryInstance(instance *ExecutorInstance) {
	if instance == nil {
		return
	}
	if instance.DiscardRecovery != nil {
		instance.DiscardRecovery()
		instance.DiscardRecovery = nil
	}
	if instance.Client != nil {
		instance.Client.Close()
		instance.Client = nil
	}
}

func (m *Manager) queueRemoteRecovery(record *models.ExecutorRunning, snapshot *RemoteRecoverySnapshot) *remoteRecoveryRetry {
	if record == nil || record.SessionID == "" || !isRemoteRecoveryRuntime(record.Runtime) {
		return nil
	}
	entry := &remoteRecoveryRetry{record: cloneRemoteRecoveryRecord(record), snapshot: snapshot}
	m.remoteRecoveryMu.Lock()
	if m.remoteRecoveryPending == nil {
		m.remoteRecoveryPending = make(map[string]*remoteRecoveryRetry)
	}
	if current := m.remoteRecoveryPending[record.SessionID]; current != nil &&
		current.record != nil && current.record.AgentExecutionID == record.AgentExecutionID {
		entry = current
	} else {
		m.remoteRecoveryPending[record.SessionID] = entry
	}
	m.recoveryGuard.RetainAsRemotePending(record.SessionID)
	m.remoteRecoveryMu.Unlock()
	return entry
}

func (m *Manager) beginRemoteRecovery(entry *remoteRecoveryRetry) bool {
	if entry == nil || entry.record == nil {
		return false
	}
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	if m.remoteRecoveryPending[entry.record.SessionID] != entry || entry.inProgress || m.shuttingDown.Load() {
		return false
	}
	select {
	case <-m.stopCh:
		return false
	default:
	}
	entry.inProgress = true
	entry.attemptDone = make(chan struct{})
	return true
}

func (m *Manager) finishRemoteRecoveryAttempt(entry *remoteRecoveryRetry) {
	if entry == nil || entry.record == nil {
		return
	}
	m.remoteRecoveryMu.Lock()
	entry.inProgress = false
	if entry.attemptDone != nil {
		close(entry.attemptDone)
		entry.attemptDone = nil
	}
	m.remoteRecoveryMu.Unlock()
}

func (m *Manager) completeRemoteRecovery(entry *remoteRecoveryRetry) bool {
	if entry == nil || entry.record == nil {
		return false
	}
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	if m.remoteRecoveryPending[entry.record.SessionID] != entry {
		return false
	}
	delete(m.remoteRecoveryPending, entry.record.SessionID)
	m.recoveryGuard.Release(entry.record.SessionID)
	return true
}

func (m *Manager) pendingRemoteRecoveryBatch() []*remoteRecoveryRetry {
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	if len(m.remoteRecoveryPending) == 0 || m.shuttingDown.Load() {
		return nil
	}
	ids := make([]string, 0, len(m.remoteRecoveryPending))
	for sessionID, entry := range m.remoteRecoveryPending {
		if entry != nil && !entry.inProgress {
			ids = append(ids, sessionID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	start := sort.SearchStrings(ids, m.remoteRecoveryCursor)
	if start >= len(ids) {
		start = 0
	}
	count := len(ids)
	if count > remoteRecoveryBatchSize {
		count = remoteRecoveryBatchSize
	}
	batch := make([]*remoteRecoveryRetry, 0, count)
	for index := 0; index < count; index++ {
		sessionID := ids[(start+index)%len(ids)]
		entry := m.remoteRecoveryPending[sessionID]
		batch = append(batch, entry)
		m.remoteRecoveryCursor = sessionID + "\x00"
	}
	return batch
}

func (m *Manager) requestPendingRemoteRecoveryStop(executionID, reason string, force bool) (*remoteRecoveryRetry, <-chan struct{}) {
	if executionID == "" {
		return nil, nil
	}
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	for _, entry := range m.remoteRecoveryPending {
		if entry == nil || entry.record == nil || entry.record.AgentExecutionID != executionID {
			continue
		}
		entry.stopRequested = true
		entry.stopReason = reason
		entry.stopForce = force
		if entry.inProgress {
			return entry, entry.attemptDone
		}
		return entry, nil
	}
	return nil, nil
}

func (m *Manager) revealRemoteRecoveryCredentials(ctx context.Context, record *models.ExecutorRunning) *models.ExecutorRunning {
	copyRecord := cloneRemoteRecoveryRecord(record)
	if copyRecord == nil {
		return nil
	}
	copyRecord.TransientAuthToken = m.revealRuntimeSecret(ctx, copyRecord.Metadata, MetadataKeyAuthTokenSecret)
	copyRecord.TransientBootstrapNonce = m.revealRuntimeSecret(ctx, copyRecord.Metadata, MetadataKeyBootstrapNonceSecret)
	return copyRecord
}

func (m *Manager) remoteAttemptContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, remoteRecoveryAttemptTimeout)
	stopCtx, stopCancel := context.WithCancel(ctx)
	managerStop := m.stopContext
	if managerStop == nil {
		managerStop = context.Background()
	}
	stop := context.AfterFunc(managerStop, stopCancel)
	return stopCtx, func() {
		stop()
		stopCancel()
		cancel()
	}
}
