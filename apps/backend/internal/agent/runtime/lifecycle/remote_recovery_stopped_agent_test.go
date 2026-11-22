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
	"testing"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestValidateRemoteSessionAssociationAcceptsStoppedRetainedRemote(t *testing.T) {
	for _, runtime := range []agentruntime.Runtime{agentruntime.RuntimeRemoteDocker, agentruntime.RuntimePluginRemote} {
		t.Run(string(runtime), func(t *testing.T) {
			instance := &ExecutorInstance{
				InstanceID: "execution-stopped", SessionID: "session-stopped", RuntimeName: runtime,
				ProviderSessionID: "native-saved",
				Metadata:          map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "agentctl-stopped"},
			}
			association := stoppedRemoteAssociation()
			require.NoError(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"))

			association.NativeSessionID = "native-saved"
			require.NoError(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"))

			association.NativeSessionID = "foreign-native"
			require.Error(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"))
		})
	}
}

func TestValidateRemoteSessionAssociationKeepsRunningProofStrict(t *testing.T) {
	instance := &ExecutorInstance{
		InstanceID: "execution-stopped", SessionID: "session-stopped", RuntimeName: agentruntime.RuntimeRemoteDocker,
		Metadata: map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "agentctl-stopped"},
	}
	association := stoppedRemoteAssociation()
	association.AgentStatus = agentProcessStatusRunning
	require.Error(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"),
		"a running process without its initialized native session is not adoptable")
	association.NativeSessionID = "native-running"
	require.NoError(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"))
}

func TestFailedRemoteRecoveryWithoutDeliveryRequiresStoppedAssociation(t *testing.T) {
	for _, runtime := range []agentruntime.Runtime{agentruntime.RuntimeRemoteDocker, agentruntime.RuntimePluginRemote} {
		t.Run(string(runtime), func(t *testing.T) {
			snapshot := &RemoteRecoverySnapshot{
				Runtime: string(runtime), SessionState: models.TaskSessionStateFailed,
			}
			require.NoError(t, validateRemoteRecoverySnapshotSession(snapshot))
			require.True(t, remoteRecoveryNeedsStoppedAgentProof(snapshot))

			instance := &ExecutorInstance{agentSessionAssociation: &agentctl.AgentSessionAssociation{
				AgentStatus: agentProcessStatusRunning,
			}}
			require.Error(t, validateFailedRemoteRecoveryAgentProof(snapshot, instance),
				"a live process cannot prove a failed session without delivery evidence")
			instance.agentSessionAssociation.AgentStatus = agentProcessStatusStopped
			require.NoError(t, validateFailedRemoteRecoveryAgentProof(snapshot, instance))

			snapshot.RecoverySourcePresent = true
			require.False(t, remoteRecoveryNeedsStoppedAgentProof(snapshot),
				"the durable delivery path keeps its existing recovery requirements")
		})
	}
}

func TestFailedRecoveryWithoutDeliveryRejectsUnrelatedRuntime(t *testing.T) {
	snapshot := &RemoteRecoverySnapshot{
		Runtime: string(agentruntime.RuntimeSSH), SessionState: models.TaskSessionStateFailed,
	}
	require.Error(t, validateRemoteRecoverySnapshotSession(snapshot))
}

func TestValidateRemoteSessionAssociationRejectsStoppedIdentityConflicts(t *testing.T) {
	instance := &ExecutorInstance{
		InstanceID: "execution-stopped", SessionID: "session-stopped", RuntimeName: agentruntime.RuntimeRemoteDocker,
		Metadata: map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "agentctl-stopped"},
	}
	for _, mutate := range []struct {
		name string
		edit func(*agentSessionAssociationAlias)
	}{
		{name: "instance", edit: func(a *agentSessionAssociationAlias) { a.InstanceID = "foreign" }},
		{name: "session", edit: func(a *agentSessionAssociationAlias) { a.SessionID = "foreign" }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			association := stoppedRemoteAssociation()
			copy := agentSessionAssociationAlias{
				InstanceID: association.InstanceID, SessionID: association.SessionID,
				IncarnationID: association.IncarnationID, HarnessGeneration: association.HarnessGeneration,
				NativeSessionID: association.NativeSessionID, AgentStatus: association.AgentStatus,
			}
			mutate.edit(&copy)
			association.InstanceID, association.SessionID = copy.InstanceID, copy.SessionID
			association.IncarnationID, association.HarnessGeneration = copy.IncarnationID, copy.HarnessGeneration
			require.Error(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"))
		})
	}
	instance.RuntimeName = agentruntime.RuntimeSSH
	require.Error(t, validateRemoteSessionAssociation(instance, stoppedRemoteAssociation(), "agentctl-stopped"),
		"only retained Remote Docker and plugin runtimes may settle stopped evidence")
}

// agentSessionAssociationAlias keeps the mutation table explicit without
// adding test-only methods to the agentctl wire type.
type agentSessionAssociationAlias struct {
	InstanceID        string
	SessionID         string
	IncarnationID     string
	HarnessGeneration uint64
	NativeSessionID   string
	AgentStatus       string
}

func stoppedRemoteAssociation() *agentctl.AgentSessionAssociation {
	return &agentctl.AgentSessionAssociation{
		InstanceID: "agentctl-stopped", SessionID: "session-stopped",
		IncarnationID: "incarnation-stopped", HarnessGeneration: 2,
		AgentStatus: agentProcessStatusStopped,
	}
}

type stoppedAgentDeliveryRepository struct {
	recordingAgentDeliveryRepository
	generation   *models.HarnessSessionGeneration
	submission   *models.AgentDeliverySubmission
	transitioned int
}

func (r *stoppedAgentDeliveryRepository) GetCurrentHarnessSessionGeneration(
	context.Context, string, string,
) (*models.HarnessSessionGeneration, error) {
	copy := *r.generation
	return &copy, nil
}

func (r *stoppedAgentDeliveryRepository) ListAgentDeliverySubmissions(
	context.Context, string,
) ([]*models.AgentDeliverySubmission, error) {
	copy := *r.submission
	return []*models.AgentDeliverySubmission{&copy}, nil
}

func (r *stoppedAgentDeliveryRepository) GetAgentDeliverySubmission(
	_ context.Context, id string,
) (*models.AgentDeliverySubmission, error) {
	if r.submission.ID != id {
		return nil, errors.New("stopped recovery submission not found")
	}
	copy := *r.submission
	return &copy, nil
}

func (r *stoppedAgentDeliveryRepository) GetAgentDeliverySubmissionMetadata(
	ctx context.Context, id string,
) (*models.AgentDeliverySubmission, error) {
	return r.GetAgentDeliverySubmission(ctx, id)
}

func (r *stoppedAgentDeliveryRepository) PrepareAgentDeliverySubmission(
	context.Context, *models.AgentDeliverySubmission,
) (bool, error) {
	return false, errors.New("stopped recovery must not prepare a new submission")
}

func (r *stoppedAgentDeliveryRepository) TransitionAgentDeliverySubmission(
	_ context.Context,
	id string,
	from, to models.DeliverySubmissionState,
	outcome string,
	at time.Time,
) (bool, error) {
	if r.submission.ID != id || r.submission.State != from {
		return false, nil
	}
	r.submission.State = to
	r.submission.Outcome = outcome
	r.submission.UpdatedAt = at
	r.transitioned++
	return true, nil
}

func TestRemoteRecoverySettlesStoppedRetainedAgent(t *testing.T) {
	for _, runtime := range []agentruntime.Runtime{agentruntime.RuntimeRemoteDocker, agentruntime.RuntimePluginRemote} {
		t.Run(string(runtime), func(t *testing.T) {
			runStoppedRemoteRecoveryCase(t, runtime, false, false)
		})
	}
	t.Run("plugin retries after provider inspection is unavailable", func(t *testing.T) {
		runStoppedRemoteRecoveryCase(t, agentruntime.RuntimePluginRemote, true, false)
	})
	t.Run("plugin checkpoint for a different resource remains guarded", func(t *testing.T) {
		runStoppedRemoteRecoveryCase(t, agentruntime.RuntimePluginRemote, false, true)
	})
}

func TestRemoteRecoverySettlesRetainedTerminalOutcome(t *testing.T) {
	for _, outcome := range []models.DeliverySubmissionState{
		models.DeliverySubmissionFailed, models.DeliverySubmissionCompleted, models.DeliverySubmissionCancelled,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
				runtime: agentruntime.RuntimeRemoteDocker, realDelivery: true, terminalOutcome: outcome,
			})
		})
	}
	t.Run("completed terminal uses the saved turn binding when its effect omits the turn", func(t *testing.T) {
		runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
			runtime: agentruntime.RuntimeRemoteDocker, realDelivery: true,
			terminalOutcome: models.DeliverySubmissionCompleted, terminalTurnIDMissing: true,
		})
	})
}

func TestRemoteRecoveryCreatesInterruptedNoticeWhenMetadataWasNotWritten(t *testing.T) {
	runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
		runtime: agentruntime.RuntimeRemoteDocker, realDelivery: true, missingRecoveryNotice: true,
	})
}

type stoppedRemoteRecoveryScenario struct {
	runtime               agentruntime.Runtime
	retryUnavailable      bool
	changeResource        bool
	realDelivery          bool
	terminalOutcome       models.DeliverySubmissionState
	terminalTurnIDMissing bool
	missingRecoveryNotice bool
	stopAfterCommit       bool
}

type stoppedRemoteSettlementBarrierWriter struct {
	*tasksqlite.Repository
	committed   chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func (w *stoppedRemoteSettlementBarrierWriter) unblock() {
	w.releaseOnce.Do(func() { close(w.release) })
}

func (w *stoppedRemoteSettlementBarrierWriter) SettleStoppedRemoteExecutionIfOwnerCurrent(
	ctx context.Context,
	sessionID, expectedExecID string,
	expectedUpdatedAt time.Time,
	expectedSessionState models.TaskSessionState,
	interruptedTurnID string,
	terminalOutcome models.DeliverySubmissionState,
	owner models.RemoteRecoveryOwnerLockIDs,
	validateOwner func(context.Context) error,
) (bool, time.Time, error) {
	changed, updatedAt, err := w.Repository.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, sessionID, expectedExecID, expectedUpdatedAt, expectedSessionState,
		interruptedTurnID, terminalOutcome, owner, validateOwner,
	)
	if err != nil {
		return changed, updatedAt, err
	}
	close(w.committed)
	<-w.release
	return changed, updatedAt, nil
}

func TestStopClaimsStoppedRemoteSettlementAfterSQLCommit(t *testing.T) {
	runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
		runtime: agentruntime.RuntimeRemoteDocker, realDelivery: true,
		missingRecoveryNotice: true, stopAfterCommit: true,
	})
}

type stoppedRemoteRepositorySnapshotProvider struct {
	*task08RemoteSnapshotProvider
	repo           *tasksqlite.Repository
	mu             sync.Mutex
	cachedRecovery models.AgentDeliveryRecovery
	cached         bool
}

func (p *stoppedRemoteRepositorySnapshotProvider) GetRemoteRecoverySnapshot(
	ctx context.Context,
	sessionID string,
) (*RemoteRecoverySnapshot, error) {
	snapshot, err := p.task08RemoteSnapshotProvider.GetRemoteRecoverySnapshot(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	recovery, cached := p.cachedRecovery, p.cached
	p.mu.Unlock()
	if cached {
		applyStoppedRemoteRecoveryNotice(snapshot, recovery)
		return snapshot, nil
	}
	session, err := p.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
	if !exists {
		return snapshot, nil
	}
	p.mu.Lock()
	p.cachedRecovery = recovery
	p.cached = true
	p.mu.Unlock()
	applyStoppedRemoteRecoveryNotice(snapshot, recovery)
	return snapshot, nil
}

func applyStoppedRemoteRecoveryNotice(snapshot *RemoteRecoverySnapshot, recovery models.AgentDeliveryRecovery) {
	snapshot.RecoverySourcePresent = true
	snapshot.RecoverableDeliveryPending = recovery.Phase == models.AgentDeliveryRecoveryUncertain ||
		recovery.Phase == models.AgentDeliveryRecoveryReconnecting
	snapshot.RecoveryExecutionID = recovery.AgentExecutionID
	snapshot.RecoverySubmissionID = recovery.SubmissionID
	snapshot.RecoveryStreamID = recovery.StreamID
	snapshot.RecoveryIncarnationID = recovery.IncarnationID
	snapshot.RecoveryHarnessGeneration = recovery.HarnessGeneration
	snapshot.RecoveryRevision = recovery.Revision
}

func runStoppedRemoteRecoveryCase(t *testing.T, runtime agentruntime.Runtime, retryUnavailable, changeResource bool) {
	runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
		runtime: runtime, retryUnavailable: retryUnavailable, changeResource: changeResource,
	})
}

func runStoppedRemoteRecoveryScenario(t *testing.T, scenario stoppedRemoteRecoveryScenario) {
	t.Helper()
	ctx := context.Background()
	repo, database := runOwnerTestRepository(t)
	const (
		sessionID   = "session-stopped-recovery"
		executionID = "execution-stopped-recovery"
	)
	runtime := scenario.runtime
	if runtime == "" {
		runtime = agentruntime.RuntimeRemoteDocker
	}
	taskID := "task-stopped-recovery-" + string(runtime)
	workspaceID := "workspace-" + taskID
	environmentID := "environment-" + taskID
	incarnationID := "incarnation-stopped-recovery"
	streamID := "stream-stopped-recovery"
	const agentctlID = "agentctl-stopped-recovery"
	const submissionID = "submission-stopped-recovery"
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Stopped remote recovery"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Stopped remote recovery"}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(runtime), ExecutorID: "executor-" + taskID,
		WorkspacePath: "/workspace/stopped-recovery", Status: models.TaskEnvironmentStatusReady,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		QueueIncarnationID: incarnationID, AgentExecutionID: executionID, State: models.TaskSessionStateRunning,
	}))
	metadata := map[string]interface{}{}
	if runtime == agentruntime.RuntimeRemoteDocker {
		metadata[MetadataKeyRemoteDockerAgentctlInstanceID] = agentctlID
	} else {
		metadata[MetadataKeyPluginExecutor] = stoppedPluginInventoryMetadata(t, environmentID, agentctlID, "resource-stopped-recovery", 1)
	}
	metadata[models.SessionMetaKeyDeliveryPromptTurnBinding] = models.AgentDeliveryPromptTurnBinding{
		PromptGeneration: 7, TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		IncarnationID: incarnationID, HarnessGeneration: 2, StreamID: streamID,
		SubmissionID: submissionID, TurnID: "turn-stopped-recovery",
	}
	row := &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, Runtime: runtime,
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: executionID,
		ExecutionProfileID: "profile-stopped-recovery", ContainerID: "container-stopped-recovery",
		AgentctlURL: "https://agentctl.stopped-recovery.invalid", AgentctlPort: 41006,
		Resumable: true, ResumeToken: "resume-stopped-recovery", Metadata: metadata,
	}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	row, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	startedAt := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-stopped-recovery", TaskID: taskID, TaskSessionID: sessionID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}))
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: incarnationID, Generation: 2,
		NativeSessionID: "native-stopped-recovery", CreationReason: "initial",
	}))
	queue, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	identity, err := queue.ResolveSessionIdentity(ctx, taskID, sessionID)
	require.NoError(t, err)
	queued := &messagequeue.QueuedMessage{
		ID: "queued-stopped-recovery", TaskID: taskID, SessionID: sessionID,
		Content: "run after explicit resume", QueuedBy: "user", QueuedAt: time.Now().UTC(),
	}
	require.NoError(t, queue.InsertForSession(ctx, identity, queued, 10))

	eventType := streams.EventTypeMessageChunk
	peerState := journal.SubmissionInterruptedUnknown
	if scenario.terminalOutcome != "" {
		switch scenario.terminalOutcome {
		case models.DeliverySubmissionCompleted:
			eventType, peerState = streams.EventTypeComplete, journal.SubmissionCompleted
		case models.DeliverySubmissionFailed:
			eventType, peerState = streams.EventTypeError, journal.SubmissionFailed
		case models.DeliverySubmissionCancelled:
			eventType, peerState = streams.EventTypeComplete, journal.SubmissionCancelled
		default:
			t.Fatalf("unsupported terminal outcome %q", scenario.terminalOutcome)
		}
	}
	createdAt := time.Now().UTC()
	wireEvent := agentctl.AgentEvent{
		Type: eventType, SessionID: sessionID, TurnID: "turn-stopped-recovery", Text: "retained output",
		DeliveryStreamID: streamID, DeliverySequence: 1, DeliverySubmissionID: submissionID,
		DeliveryIncarnationID: incarnationID, DeliveryHarnessGeneration: 2,
	}
	if scenario.terminalTurnIDMissing {
		wireEvent.TurnID = ""
	}
	if scenario.terminalOutcome == models.DeliverySubmissionFailed {
		wireEvent.Error = "retained provider failure"
		wireEvent.Data = map[string]any{"is_error": true}
	}
	if scenario.terminalOutcome == models.DeliverySubmissionCancelled {
		wireEvent.Data = map[string]any{"stop_reason": string(models.DeliverySubmissionCancelled)}
	}
	payload, err := json.Marshal(wireEvent)
	require.NoError(t, err)
	stream := journal.Stream{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2,
		StreamID: streamID, HighWater: 1, FirstRetained: 1,
	}
	promptPayload := []byte("immutable prompt")
	promptHash := "payload-hash-stopped-recovery"
	if scenario.realDelivery {
		promptHash = journal.SubmissionHash(promptPayload)
	}
	peerSubmission := journal.Submission{
		ID: "submission-stopped-recovery", SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: 2, StreamID: streamID, Hash: promptHash, Payload: promptPayload, State: peerState,
		TerminalEventRetained: scenario.terminalOutcome != "", TerminalSequence: 1,
	}
	delivery := &stoppedAgentDeliveryRepository{
		recordingAgentDeliveryRepository: recordingAgentDeliveryRepository{cursor: &models.AgentDeliveryCursor{
			SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2, StreamID: streamID,
		}},
		generation: &models.HarnessSessionGeneration{
			SessionID: sessionID, IncarnationID: incarnationID, Generation: 2,
			NativeSessionID: "native-stopped-recovery",
		},
		submission: &models.AgentDeliverySubmission{
			ID: peerSubmission.ID, SessionID: sessionID, IncarnationID: incarnationID,
			HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: peerSubmission.Hash,
			Payload: promptPayload, State: models.DeliverySubmissionDispatching,
		},
	}
	var deliveryRepository AgentDeliveryRepository = delivery
	if scenario.realDelivery {
		_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
			ID: peerSubmission.ID, SessionID: sessionID, IncarnationID: incarnationID,
			HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: peerSubmission.Hash,
			Payload: promptPayload, State: models.DeliverySubmissionDispatching,
		})
		require.NoError(t, err)
		_, err = repo.DB().ExecContext(ctx, `INSERT INTO agent_delivery_cursors
			(stream_id, session_id, incarnation_id, harness_generation, received_sequence, projected_sequence, remote_high_water, updated_at)
			VALUES (?, ?, ?, ?, 0, 0, 1, ?)`, streamID, sessionID, incarnationID, 2, createdAt)
		require.NoError(t, err)
		deliveryRepository = repo
	}
	status := agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
		SessionID:         sessionID, IncarnationID: incarnationID, HarnessGeneration: 2, StreamID: streamID,
		Stream: &stream, Submissions: []journal.SubmissionSummary{{
			ID: peerSubmission.ID, SessionID: sessionID, IncarnationID: incarnationID,
			HarnessGeneration: 2, StreamID: streamID, Hash: peerSubmission.Hash,
			State: peerSubmission.State, TerminalEventRetained: peerSubmission.TerminalEventRetained,
			TerminalSequence: peerSubmission.TerminalSequence,
		}}, SubmissionCount: 1,
	}
	retainedEvent := journal.Event{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 2,
		StreamID: streamID, Sequence: 1, SubmissionID: peerSubmission.ID,
		Type: eventType, Terminal: scenario.terminalOutcome != "", Payload: payload, CreatedAt: createdAt,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer stopped-recovery-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/agent/session":
			_ = json.NewEncoder(w).Encode(agentctl.AgentSessionAssociation{
				InstanceID: agentctlID, SessionID: sessionID, IncarnationID: incarnationID,
				HarnessGeneration: 2, AgentStatus: agentProcessStatusStopped,
			})
		case "/api/v1/agent/delivery":
			_ = json.NewEncoder(w).Encode(status)
		case "/api/v1/agent/delivery/stream":
			var replay []journal.Event
			if r.URL.Query().Get("after") == "0" {
				replay = []journal.Event{retainedEvent}
			}
			_ = json.NewEncoder(w).Encode(struct {
				Events []journal.Event `json:"events"`
				Stream journal.Stream  `json:"stream"`
			}{Events: replay, Stream: stream})
		case "/api/v1/agent/submissions":
			_ = json.NewEncoder(w).Encode([]journal.Submission{peerSubmission})
		case "/api/v1/stop":
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

	var client *agentctl.Client
	backendName := executor.NameRemoteDocker
	if runtime == agentruntime.RuntimePluginRemote {
		backendName = executor.NamePluginRemote
	}
	backendCalls := 0
	backend := &task08DetailedRecoveryBackend{
		MockExecutor: &MockExecutor{name: backendName},
		recoveryResult: func(records []*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
			backendCalls++
			if len(records) != 1 || records[0].AgentExecutionID != executionID {
				t.Fatalf("recovery candidates = %+v", records)
			}
			if scenario.retryUnavailable && backendCalls == 1 {
				return nil, nil, errors.New("provider inspection unavailable")
			}
			if runtime == agentruntime.RuntimePluginRemote && (scenario.retryUnavailable || scenario.changeResource) {
				current, readErr := repo.GetExecutorRunningBySessionID(ctx, sessionID)
				require.NoError(t, readErr)
				resource := "resource-stopped-recovery"
				if scenario.changeResource {
					resource = "resource-foreign"
				}
				setStoppedPluginCheckpoint(t, repo, current, resource, 2)
			}
			client = agentctl.NewClient(host, port, newTestLogger(),
				agentctl.WithExecutionID(executionID), agentctl.WithAuthToken("stopped-recovery-token"))
			return []*ExecutorInstance{{
				InstanceID: executionID, TaskID: taskID, SessionID: sessionID,
				RuntimeName: runtime, WorkspacePath: "/workspace/stopped-recovery",
				AgentProfileID: row.ExecutionProfileID, Client: client,
				Metadata: cloneRemoteRecoveryRecord(records[0]).Metadata,
			}}, nil, nil
		},
	}
	manager, eventBus := task08ManagerWithBackend(t, backend)
	var settlementBarrier *stoppedRemoteSettlementBarrierWriter
	if scenario.stopAfterCommit {
		settlementBarrier = &stoppedRemoteSettlementBarrierWriter{
			Repository: repo, committed: make(chan struct{}), release: make(chan struct{}),
		}
		manager.SetExecutorRunningWriter(settlementBarrier)
	} else {
		manager.SetExecutorRunningWriter(repo)
	}
	manager.SetAgentDeliveryRepository(deliveryRepository)
	snapshot := task08RemoteSnapshot(row)
	snapshot.SessionState = models.TaskSessionStateRunning
	if !scenario.realDelivery {
		snapshot.RecoverySourcePresent = true
		snapshot.RecoverableDeliveryPending = true
		snapshot.RecoveryExecutionID = executionID
		snapshot.RecoverySubmissionID = peerSubmission.ID
		snapshot.RecoveryStreamID = streamID
		snapshot.RecoveryIncarnationID = incarnationID
		snapshot.RecoveryHarnessGeneration = 2
		snapshot.RecoveryRevision = 1
	}
	provider := &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{sessionID: snapshot}}
	if scenario.missingRecoveryNotice {
		manager.workspaceInfoProvider = &stoppedRemoteRepositorySnapshotProvider{task08RemoteSnapshotProvider: provider, repo: repo}
	} else {
		manager.workspaceInfoProvider = provider
	}
	manager.SetRecoveryDeadline(time.Second)
	entry := manager.queueRemoteRecovery(row, nil)
	require.NotNil(t, entry)
	t.Cleanup(func() {
		if client != nil && !client.IsClosed() {
			client.Close()
		}
		require.NoError(t, manager.Stop())
	})
	if settlementBarrier != nil {
		t.Cleanup(settlementBarrier.unblock)
	}
	var stateEventCount int
	eventBus.OnPublish = func(subject string, _ *bus.Event) {
		if subject == events.TaskSessionStateChanged {
			stateEventCount++
			require.NotNil(t, client)
			require.True(t, client.IsClosed(), "the stopped attempt transport must close before state publication")
			require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(sessionID),
				"the matching recovery guard must be released before state publication")
		}
	}

	if scenario.stopAfterCommit {
		retryDone := make(chan struct{})
		go func() {
			manager.retryRemoteRecovery(ctx, entry)
			close(retryDone)
		}()
		select {
		case <-settlementBarrier.committed:
		case <-time.After(5 * time.Second):
			t.Fatal("stopped settlement did not commit its owner CAS")
		}
		current, readErr := repo.GetExecutorRunningBySessionID(ctx, sessionID)
		require.NoError(t, readErr)
		require.Equal(t, models.ExecutorRunningStatusStopped, current.Status,
			"the committed stopped row is visible while its transport remains attached")
		require.False(t, client.IsClosed())

		stopDone := make(chan error, 1)
		go func() {
			stopDone <- manager.StopAgentWithReason(ctx, executionID, StopReasonTaskDeleted, false)
		}()
		require.Eventually(t, func() bool {
			manager.remoteRecoveryMu.Lock()
			defer manager.remoteRecoveryMu.Unlock()
			return entry.stopRequested
		}, time.Second, time.Millisecond, "Stop must claim the retained owner before transport closure")
		require.Same(t, entry, manager.pendingRemoteRecoveryForSession(sessionID))
		require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(sessionID), ErrSessionRecoveryGuarded)
		settlementBarrier.unblock()
		select {
		case <-retryDone:
		case <-time.After(5 * time.Second):
			t.Fatal("stopped recovery did not yield its pending entry to Stop")
		}
		require.NoError(t, <-stopDone, "Stop must take over the exact committed stopped owner")
		require.Nil(t, manager.pendingRemoteRecoveryForSession(sessionID))
		require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(sessionID))
		current, readErr = repo.GetExecutorRunningBySessionID(ctx, sessionID)
		require.NoError(t, readErr)
		require.Equal(t, models.ExecutorRunningStatusStopped, current.Status,
			"the winning Stop owner leaves the exact runtime row settled as stopped")
		require.Len(t, backend.stopInstanceCalls, 1, "the winning Stop owner tears down the exact runtime")
		require.True(t, client.IsClosed(), "the winning Stop owner closes its authenticated transport")
		stoppedSession, readErr := repo.GetTaskSession(ctx, sessionID)
		require.NoError(t, readErr)
		require.Equal(t, models.TaskSessionStateWaitingForInput, stoppedSession.State)
		return
	}

	manager.retryRemoteRecovery(ctx, entry)
	if scenario.retryUnavailable {
		require.Same(t, entry, manager.pendingRemoteRecoveryForSession(sessionID),
			"provider unavailability must retain the same pending owner for periodic retry")
		require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(sessionID), ErrSessionRecoveryGuarded)
		manager.retryRemoteRecovery(ctx, entry)
	}
	if scenario.changeResource {
		require.Same(t, entry, manager.pendingRemoteRecoveryForSession(sessionID),
			"a changed plugin resource handle must keep the recovery owner guarded")
		require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(sessionID), ErrSessionRecoveryGuarded)
		current, readErr := repo.GetExecutorRunningBySessionID(ctx, sessionID)
		require.NoError(t, readErr)
		require.Equal(t, models.ExecutorRunningStatusRunning, current.Status)
		session, readErr := repo.GetTaskSession(ctx, sessionID)
		require.NoError(t, readErr)
		require.Equal(t, models.TaskSessionStateRunning, session.State)
		require.Empty(t, delivery.projected, "a changed plugin resource must be fenced before replay")
		return
	}
	require.Nil(t, manager.pendingRemoteRecoveryForSession(sessionID))
	require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(sessionID))
	current, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, current.Status)
	require.Zero(t, current.LocalPID)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	turn, err := repo.GetTurn(ctx, "turn-stopped-recovery")
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt)
	if scenario.realDelivery {
		submission, readErr := repo.GetAgentDeliverySubmission(ctx, submissionID)
		require.NoError(t, readErr)
		if scenario.terminalOutcome != "" {
			require.True(t, turn.CompletedAt.After(turn.StartedAt), "a retained terminal closes the original turn as completed")
			require.Equal(t, scenario.terminalOutcome, submission.State)
			require.Equal(t, string(scenario.terminalOutcome), submission.Outcome)
			workflowOutcome, readErr := repo.GetAgentDeliveryEffect(ctx, "workflow.on_turn_complete:turn-stopped-recovery")
			require.NoError(t, readErr)
			require.NotNil(t, workflowOutcome)
			require.Equal(t, models.DeliveryWorkflowOutcomeStoppedRecovery, workflowOutcome.Outcome)
			require.Zero(t, submission.TerminalSequence, "the existing settlement consumer owns terminal-sequence acknowledgement")
			_, hasRecovery := models.LoadAgentDeliveryRecovery(session.Metadata)
			require.False(t, hasRecovery, "a proven terminal outcome must not create an interruption notice")
		} else {
			require.Equal(t, turn.StartedAt, *turn.CompletedAt, "unknown outcome closes only the original turn as interrupted")
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, submission.State)
			recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
			require.True(t, exists, "the exact interrupted submission must establish its recovery notice")
			require.Equal(t, models.AgentDeliveryRecoveryUncertain, recovery.Phase)
			require.Equal(t, submissionID, recovery.SubmissionID)
			require.Equal(t, uint64(7), recovery.PromptGeneration)
			block, readErr := repo.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, 2)
			require.NoError(t, readErr)
			require.NotNil(t, block)
			require.Equal(t, "unknown_prompt_outcome", block.Reason)
			require.Equal(t, submissionID, block.DeliverySubmissionID)
			binding, exists := models.LoadAgentDeliveryPromptTurnBinding(row.Metadata)
			require.True(t, exists)
			require.Equal(t, "turn-stopped-recovery", binding.TurnID)
		}
		cursor, readErr := repo.GetAgentDeliveryCursor(ctx, streamID)
		require.NoError(t, readErr)
		require.Equal(t, int64(1), cursor.ProjectedSequence)
	} else {
		require.Equal(t, turn.StartedAt, *turn.CompletedAt, "unknown outcome closes only the original turn as interrupted")
		require.Equal(t, models.DeliverySubmissionInterruptedUnknown, delivery.submission.State)
		require.Equal(t, 1, delivery.transitioned, "the exact dispatch is reconciled once")
		require.Len(t, delivery.projected, 1)
		require.Equal(t, int64(1), delivery.cursor.ProjectedSequence)
	}
	queuedAfter, err := queue.ListBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, []string{queued.ID}, queuedMessageIDs(queuedAfter), "stopped settlement leaves the queued instruction for Resume")
	require.Equal(t, 1, stateEventCount)
	eventBus.mu.Lock()
	published := append([]*bus.Event(nil), eventBus.PublishedEvents...)
	eventBus.mu.Unlock()
	for _, event := range published {
		require.NotEqual(t, events.AgentReady, event.Type, "stopped settlement must not publish readiness")
		require.NotEqual(t, events.AgentBootReady, event.Type, "stopped settlement must not publish recovered availability")
	}
}

func queuedMessageIDs(messages []messagequeue.QueuedMessage) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func stoppedPluginInventoryMetadata(t *testing.T, environmentID, runtimeID, resourceHandle string, revision int64) map[string]interface{} {
	t.Helper()
	inventory := pluginExecutorInventory{
		PluginID: "plugin-stopped", InstallationID: "installation-stopped", ProviderKey: "provider-stopped",
		ProviderIdentity: "plugin:stopped:provider", ContractVersion: 1, SupportedStateVersions: []int{1}, StateVersion: 1,
		EnvironmentID: environmentID, ProfileID: "profile-stopped-recovery", OperationID: "operation-stopped",
		InputDigest: "digest-stopped", Phase: pluginExecutorPhaseReady, Revision: uint64(revision), RuntimeIdentity: runtimeID,
		Resource: &pluginsdk.ExecutorResourceDescriptor{
			ResourceHandle: resourceHandle, StateJson: `{"resource":"stopped"}`, Platform: "linux-amd64", StateVersion: 1,
		},
	}
	data, err := json.Marshal(inventory)
	require.NoError(t, err)
	var metadata map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &metadata))
	return metadata
}

func setStoppedPluginCheckpoint(t *testing.T, repo *tasksqlite.Repository, row *models.ExecutorRunning, resourceHandle string, revision int64) {
	t.Helper()
	inventory, err := decodePluginExecutorInventory(row.Metadata)
	require.NoError(t, err)
	row.Metadata[MetadataKeyPluginExecutor] = stoppedPluginInventoryMetadata(
		t, inventory.EnvironmentID, inventory.RuntimeIdentity, resourceHandle, revision,
	)
	require.NoError(t, repo.UpsertExecutorRunning(context.Background(), row))
}
