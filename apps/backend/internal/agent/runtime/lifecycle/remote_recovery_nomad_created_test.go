package lifecycle

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type nomadCreatedContinuationProvider struct {
	info               *WorkspaceInfo
	snapshot           *RemoteRecoverySnapshot
	snapshotCalls      atomic.Int32
	failSnapshotCallAt int32
}

func TestUnpinnedNomadRecoveryOwnerBlocksWorkspaceUntilSnapshotIsAvailable(t *testing.T) {
	manager := newTestManager(t)
	row := &models.ExecutorRunning{
		SessionID: "session-unpinned-nomad", TaskID: "task-unpinned-nomad",
		AgentExecutionID: "execution-unpinned-nomad", Runtime: agentruntime.RuntimeNomad,
	}
	entry := manager.queueRemoteRecovery(row, nil)
	require.NotNil(t, entry)
	require.ErrorIs(t, manager.rejectWorkspaceDuringCreatedNomadRecovery(row.SessionID), ErrSessionRecoveryGuarded,
		"a queued Nomad owner with no snapshot is unknown and must not expose a new workspace path")

	entry.snapshot = &RemoteRecoverySnapshot{SessionState: models.TaskSessionStateCreated}
	require.ErrorIs(t, manager.rejectWorkspaceDuringCreatedNomadRecovery(row.SessionID), ErrSessionRecoveryGuarded)

	entry.snapshot = &RemoteRecoverySnapshot{SessionState: models.TaskSessionStateRunning}
	require.NoError(t, manager.rejectWorkspaceDuringCreatedNomadRecovery(row.SessionID),
		"this helper only adds the Created/pre-ACP gate; running-session recovery keeps its existing policy")
}

func (p *nomadCreatedContinuationProvider) GetWorkspaceInfoForSession(context.Context, string, string) (*WorkspaceInfo, error) {
	copyInfo := *p.info
	copyInfo.Metadata = clonePluginExecutorMetadata(p.info.Metadata)
	return &copyInfo, nil
}

func (p *nomadCreatedContinuationProvider) GetWorkspaceInfoForEnvironment(context.Context, string) (*WorkspaceInfo, error) {
	return p.GetWorkspaceInfoForSession(context.Background(), "", p.info.SessionID)
}

func (p *nomadCreatedContinuationProvider) GetRemoteRecoverySnapshot(context.Context, string) (*RemoteRecoverySnapshot, error) {
	if call := p.snapshotCalls.Add(1); p.failSnapshotCallAt == call {
		return nil, errors.New("fixture transient post-publication snapshot failure")
	}
	copySnapshot := *p.snapshot
	return &copySnapshot, nil
}

func TestCreatedNomadRecoveryPublishesWorkspaceOnlyExecutionForOriginalOperation(t *testing.T) {
	const (
		taskID      = "task-created-nomad"
		sessionID   = "session-created-nomad"
		environment = "environment-created-nomad"
		executionID = "execution-created-nomad"
	)

	manager := newTestManager(t)
	manager.executorRegistry = NewExecutorRegistry(manager.logger)
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameNomad},
		client:       newReadyAgentctlClient(t, manager.logger),
		entered:      make(chan struct{}, 1),
		barrier:      make(chan struct{}),
	}
	manager.executorRegistry.Register(backend)
	workspaceRoot := t.TempDir()
	workspacePath, err := nomad.EnvironmentWorkspacePath(workspaceRoot, environment, 1)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(workspacePath, 0o700))
	info := &WorkspaceInfo{
		TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environment,
		ValidatedTaskEnvironmentID: environment, ValidatedExecutorType: string(models.ExecutorTypeNomad),
		ValidatedTaskEnvironmentGeneration: 1, EnvironmentOwnerTaskID: taskID, OwnershipGeneration: 1,
		WorkspaceID: "workspace-created-nomad", WorkspacePath: workspacePath,
		ExecutorType: string(models.ExecutorTypeNomad), AgentExecutionID: executionID,
		ExecutionProfileID: "agent-profile-created-nomad", ExecutorProfileID: "executor-profile-created-nomad",
		AgentID: "auggie",
		Metadata: map[string]interface{}{
			MetadataKeyExecutorType:      string(models.ExecutorTypeNomad),
			MetadataKeyExecutorProfileID: "executor-profile-created-nomad",
			"executor_id":                "executor-id-created-nomad",
			"address":                    "http://127.0.0.1:4646", "namespace": "default", "region": "global",
			"datacenter": "dc1", "node_id": "node-created-nomad", "platform": "darwin/arm64",
			"ssh_target": "runner@127.0.0.1", "ssh_port": "22",
			"ssh_host_fingerprint": "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
			"workspace_root":       workspaceRoot, "allocation_root": t.TempDir(),
			"cpu_mhz": "1000", "memory_mb": "2048", "nomad_token": "$kandev-secret:nomad-token-ref",
			MetadataKeyBootstrapNonceSecret: "$kandev-secret:bootstrap-nonce-ref",
			MetadataKeyAuthTokenSecret:      "$kandev-secret:agentctl-auth-ref",
		},
	}

	state := nomad.EnvironmentState{
		Version: 1, Revision: 4, EnvironmentID: environment, Generation: 1,
		OperationID: executionID, InstallationID: info.ExecutorProfileID, ExecutionID: executionID,
		JobID: "kandev-created-nomad", Namespace: "default", Region: "global", Datacenter: "dc1",
		NodeID: "node-created-nomad", Platform: "darwin/arm64", SSHHost: "127.0.0.1", SSHUser: "runner", SSHPort: 22,
		SSHHostFingerprint: info.Metadata["ssh_host_fingerprint"].(string), WorkspaceRoot: workspaceRoot,
		WorkspacePath: workspacePath, WorkspaceInputCaptured: true,
		AllocationRoot: info.Metadata["allocation_root"].(string),
		CPUMHz:         1000, MemoryMB: 2048, AllocationID: "allocation-created-nomad",
		AllocationCreateIndex: 10, ControlPort: 23001, InstancePort: 23002, Phase: "allocated",
	}
	state.JobID, err = nomad.StableJobID(nomad.JobIdentity{
		EnvironmentID: environment, Generation: 1, OperationID: executionID, InstallationID: info.ExecutorProfileID,
	})
	require.NoError(t, err)
	profile, err := nomadProfileFromMetadata(info.Metadata)
	require.NoError(t, err)
	preparation, err := manager.prepareExecutionCreateRequest(context.Background(), taskID, info, executionID)
	require.NoError(t, err)
	require.Equal(t, "1", getMetadataString(preparation.request.Metadata, "nomad_environment_generation"),
		"retained Nomad request must project generation from the validated environment owner, even when the provider metadata omitted the launch-only field")
	mismatchedInfo := *info
	mismatchedInfo.Metadata = clonePluginExecutorMetadata(info.Metadata)
	mismatchedInfo.Metadata["nomad_environment_generation"] = "2"
	_, err = manager.prepareExecutionCreateRequest(context.Background(), taskID, &mismatchedInfo, executionID)
	require.ErrorContains(t, err, "generation metadata conflicts",
		"a conflicting launch-only generation must fail closed instead of being silently overwritten")
	state.InputDigest, err = nomadLaunchDigest(profile, preparation.request, state.WorkspaceInputPath, workspacePath)
	require.NoError(t, err)
	addFixtureJobEvidence(t, profile, &state)
	require.NoError(t, nomad.PutEnvironmentState(info.Metadata, state))

	provider := &nomadCreatedContinuationProvider{
		info: info,
		snapshot: &RemoteRecoverySnapshot{
			TaskID: taskID, SessionID: sessionID, SessionState: models.TaskSessionStateCreated,
			AgentExecutionID: executionID, Runtime: string(agentruntime.RuntimeNomad),
			TaskWorkspaceID: info.WorkspaceID, TaskEnvironmentID: environment,
			EnvironmentOwnerTaskID: taskID, EnvironmentOwnershipGen: 1,
			EnvironmentExecutorType: string(models.ExecutorTypeNomad), EnvironmentExecutorID: "executor-id-created-nomad",
			EnvironmentStatus: "creating", RouteState: "active",
		},
		failSnapshotCallAt: 3, // Fail after the exact workspace-only execution is published.
	}
	manager.workspaceInfoProvider = provider
	admissionReader := &fakeExecutorProfileReader{
		task: &models.Task{ID: taskID},
		session: &models.TaskSession{
			ID: sessionID, TaskID: taskID, TaskEnvironmentID: environment,
			State: models.TaskSessionStateCreated,
		},
		env: &models.TaskEnvironment{
			ID: environment, TaskID: taskID, ExecutorID: "executor-id-created-nomad",
			ExecutorType: string(models.ExecutorTypeNomad), OwnershipGeneration: 1,
			Status: models.TaskEnvironmentStatusCreating,
		},
	}
	manager.SetExecutorProfileReader(admissionReader)
	require.ErrorIs(t, manager.ensureWorkspaceSessionAdmitted(context.Background(), taskID, info), ErrSessionWorkspaceNotReady,
		"ordinary workspace admission must continue rejecting a creating environment")
	row := &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Runtime: agentruntime.RuntimeNomad, ExecutionProfileID: info.ExecutionProfileID,
		Status: models.ExecutorRunningStatusReady, Metadata: clonePluginExecutorMetadata(info.Metadata),
	}
	if _, ok := createdNomadRecoveryState(row); !ok {
		t.Fatal("valid retained Nomad operation with original nonce and auth references must be recoverable")
	}
	require.NoError(t, validateCreatedNomadRecoverySnapshot(provider.snapshot, row, state),
		"a nonterminal Nomad intent may resume while its task environment is still creating")
	for name, mutate := range map[string]func(*RemoteRecoverySnapshot){
		"terminal environment": func(snapshot *RemoteRecoverySnapshot) { snapshot.EnvironmentStatus = "stopped" },
		"archived task":        func(snapshot *RemoteRecoverySnapshot) { snapshot.TaskArchived = true },
		"active cleanup":       func(snapshot *RemoteRecoverySnapshot) { snapshot.TaskResourceCleanupRunning = true },
		"delivery already owned": func(snapshot *RemoteRecoverySnapshot) {
			snapshot.RecoverySourcePresent = true
		},
		"non-created session": func(snapshot *RemoteRecoverySnapshot) {
			snapshot.SessionState = models.TaskSessionStateRunning
		},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := *provider.snapshot
			mutate(&snapshot)
			require.Error(t, validateCreatedNomadRecoverySnapshot(&snapshot, row, state))
		})
	}
	missingNonce := *row
	missingNonce.Metadata = clonePluginExecutorMetadata(row.Metadata)
	delete(missingNonce.Metadata, MetadataKeyBootstrapNonceSecret)
	if _, ok := createdNomadRecoveryState(&missingNonce); ok {
		t.Fatal("recovery must reject an operation whose original bootstrap nonce reference was not persisted")
	}
	readyCheckpoint := *row
	readyCheckpoint.Metadata = clonePluginExecutorMetadata(row.Metadata)
	readyCheckpointState := state
	readyCheckpointState.Phase = "ready"
	require.NoError(t, nomad.PutEnvironmentState(readyCheckpoint.Metadata, readyCheckpointState))
	missingAuth := readyCheckpoint
	missingAuth.Metadata = clonePluginExecutorMetadata(readyCheckpoint.Metadata)
	delete(missingAuth.Metadata, MetadataKeyAuthTokenSecret)
	if _, ok := createdNomadRecoveryState(&missingAuth); ok {
		t.Fatal("ready attach must reject an operation without its persisted agentctl auth-token reference")
	}
	terminal := *row
	terminal.Status = models.ExecutorRunningStatusFailed
	if _, ok := createdNomadRecoveryState(&terminal); ok {
		t.Fatal("terminal operation must not be resurrected by Created-session continuation")
	}
	workspaceInfoWithDifferentNonce := *info
	workspaceInfoWithDifferentNonce.Metadata = clonePluginExecutorMetadata(info.Metadata)
	workspaceInfoWithDifferentNonce.Metadata[MetadataKeyBootstrapNonceSecret] = "$kandev-secret:other-nonce-ref"
	if createdNomadWorkspaceMatches(&workspaceInfoWithDifferentNonce, row, &RemoteRecoverySnapshot{
		TaskID: taskID, SessionID: sessionID, SessionState: models.TaskSessionStateCreated,
		AgentExecutionID: executionID, Runtime: string(agentruntime.RuntimeNomad),
		TaskWorkspaceID: info.WorkspaceID, TaskEnvironmentID: environment,
		EnvironmentOwnerTaskID: taskID, EnvironmentOwnershipGen: 1,
		EnvironmentExecutorType: string(models.ExecutorTypeNomad), EnvironmentExecutorID: "executor-id-created-nomad",
		EnvironmentStatus: "ready", RouteState: "active",
	}, state) {
		t.Fatal("continuation must use the persisted credential references for the exact inventory owner")
	}
	manager.SetExecutorRunningWriter(&nomadInventoryStoreFake{row: row})
	entry := manager.queueRemoteRecovery(row, provider.snapshot)
	require.ErrorIs(t, manager.ensureWorkspaceSessionAdmittedForCreatedNomadRecovery(context.Background(), taskID, info, entry), ErrSessionWorkspaceNotReady,
		"the creating-environment exception must require the active in-flight recovery attempt")

	completed := make(chan struct{})
	go func() {
		manager.retryRemoteRecovery(context.Background(), entry)
		close(completed)
	}()
	select {
	case <-backend.entered:
	case <-time.After(time.Second):
		t.Fatal("Nomad workspace-only continuation did not reach the runtime boundary")
	}
	guardCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := manager.GetOrEnsureExecution(guardCtx, sessionID); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("unrelated workspace request during Created recovery = %v, want ErrSessionRecoveryGuarded", err)
	}
	if _, err := manager.GetOrEnsureExecutionForEnvironment(guardCtx, environment); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("environment workspace request during Created recovery = %v, want ErrSessionRecoveryGuarded", err)
	}
	close(backend.barrier)
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("Created Nomad continuation did not finish after the runtime boundary was released")
	}
	require.True(t, manager.HasPendingRemoteRecovery(sessionID, executionID), "a transient owner-snapshot failure must keep the guard after publication")
	// Creating the instance advances the inventory to ready; the next tick
	// publishes the environment and only then releases the guard.
	advanced := state
	advanced.Phase, advanced.Revision = "ready", state.Revision+1
	require.NoError(t, nomad.PutEnvironmentState(row.Metadata, advanced))
	require.NoError(t, nomad.PutEnvironmentState(info.Metadata, advanced))
	manager.SetNomadEnvironmentReadyPublisher(&recordingNomadPublisher{onPublish: func(NomadEnvironmentReadyPublication) {
		admissionReader.env.Status = models.TaskEnvironmentStatusReady
	}})
	for attempt := 0; attempt < 3 && manager.HasPendingRemoteRecovery(sessionID, executionID); attempt++ {
		manager.retryRemoteRecovery(context.Background(), entry)
	}

	require.EqualValues(t, 1, backend.createCount.Load(), "created-session continuation must use the normal workspace-only runtime path once")
	require.Equal(t, executionID, backend.lastRequest.InstanceID, "continuation must retain the original execution identity")
	require.Equal(t, executionID, backend.lastRequest.PreviousExecutionID, "continuation must not mint a replacement operation")
	require.False(t, backend.lastRequest.RequireExistingAgentctlInstance, "allocated-but-not-ready state may create the original agentctl instance")
	execution, exists := manager.executionStore.GetBySessionID(sessionID)
	require.True(t, exists, "workspace-only execution must be published")
	readyCtx, readyCancel := context.WithTimeout(context.Background(), time.Second)
	defer readyCancel()
	require.NoError(t, execution.waitForAgentctlReady(readyCtx))
	require.Equal(t, executionID, execution.ID)
	require.False(t, execution.isSessionInitialized(), "pre-ACP continuation must not initialize a conversation")
	require.Empty(t, execution.ACPSessionID)
	require.False(t, manager.HasPendingRemoteRecovery(sessionID, executionID), "guard releases only after the exact execution is published")

	readyInfo := *info
	readyInfo.Metadata = clonePluginExecutorMetadata(info.Metadata)
	readyState := state
	readyState.Phase = "ready"
	require.NoError(t, nomad.PutEnvironmentState(readyInfo.Metadata, readyState))
	admissionReader.env.Status = models.TaskEnvironmentStatusReady
	readyInputs, err := manager.prepareExecutionCreation(context.Background(), taskID, &readyInfo, false)
	require.NoError(t, err)
	require.True(t, readyInputs.preparation.request.RequireExistingAgentctlInstance,
		"ready phase must attach the original agentctl instance and refuse to create a replacement")
}
