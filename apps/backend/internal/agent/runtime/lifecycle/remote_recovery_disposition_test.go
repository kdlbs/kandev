package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type remoteRecoveryCASWriter struct {
	*tasksqlite.Repository
	repair func(context.Context, string, string, time.Time) error
}

type remoteRecoveryReadOnlyWriter struct {
	ExecutorRunningWriter
	repository *tasksqlite.Repository
}

func (w *remoteRecoveryReadOnlyWriter) GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error) {
	return w.repository.GetExecutorRunningBySessionID(ctx, sessionID)
}

func (w *remoteRecoveryReadOnlyWriter) ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error) {
	return w.repository.ListExecutorsRunning(ctx)
}

func (w *remoteRecoveryCASWriter) RepairExecutorRunningDeadIfCurrent(
	ctx context.Context,
	sessionID, executionID string,
	updatedAt time.Time,
) error {
	if w.repair != nil {
		return w.repair(ctx, sessionID, executionID, updatedAt)
	}
	return w.Repository.RepairExecutorRunningDeadIfCurrent(ctx, sessionID, executionID, updatedAt)
}

func TestRemoteRecoveryConfirmedAbsenceSettlesGuard(t *testing.T) {
	for _, runtime := range []agentruntime.Runtime{
		agentruntime.RuntimeRemoteDocker,
		agentruntime.RuntimeSprites,
	} {
		t.Run(string(runtime), func(t *testing.T) {
			ctx := context.Background()
			const (
				taskID      = "task-confirmed-absence"
				sessionID   = "session-confirmed-absence"
				executionID = "execution-confirmed-absence"
			)
			repo, _ := runOwnerTestRepository(t)
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-absence", Name: "Absence"}))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-absence", Title: "Absence"}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, QueueIncarnationID: "incarnation-absence"}))
			record := &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
				Runtime: runtime, Status: models.ExecutorRunningStatusRunning, Resumable: true,
				ResumeToken: "resume-absence", WorktreePath: "/workspace/absence", LocalPID: 73,
				Metadata: map[string]interface{}{"preserved": "metadata"},
			}
			require.NoError(t, repo.UpsertExecutorRunning(ctx, record))

			backendName := executor.NameRemoteDocker
			if runtime == agentruntime.RuntimeSprites {
				backendName = executor.NameSprites
			}
			backend := &task08DetailedRecoveryBackend{
				MockExecutor: &MockExecutor{name: backendName},
				recoveryResult: func(records []*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
					outcomes := map[string]RecoveryCandidateOutcome{sessionID: RecoveryOutcomeNoMatchingInstance}
					return nil, outcomes, nil
				},
			}
			manager, _ := task08ManagerWithBackend(t, backend)
			manager.SetExecutorRunningWriter(repo)
			manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{
				snapshots: map[string]*RemoteRecoverySnapshot{sessionID: task08RemoteSnapshot(record)},
			}
			manager.SetPassthroughLookup(alwaysNotPassthrough)
			manager.SetRecoveryDeadline(time.Second)
			t.Cleanup(func() { require.NoError(t, manager.Stop()) })

			require.NoError(t, manager.Start(ctx))

			require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(sessionID),
				"a provider-confirmed absent owner must no longer retain the startup launch guard")
			current, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
			require.NoError(t, err)
			require.Equal(t, models.ExecutorRunningStatusStopped, current.Status)
			require.Equal(t, "resume-absence", current.ResumeToken)
			require.Equal(t, "/workspace/absence", current.WorktreePath)
			require.Equal(t, "metadata", current.Metadata["preserved"])
		})
	}
}

func TestDeferredRemoteRecoveryRetriesUntilConfirmedAbsence(t *testing.T) {
	ctx := startup.WithReporter(context.Background(), startup.New(nil))
	repo, _ := runOwnerTestRepository(t)
	row := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeSprites, "deferred")
	var calls atomic.Int32
	backend := &task08DetailedRecoveryBackend{
		MockExecutor: &MockExecutor{name: executor.NameSprites},
		recoveryResult: func([]*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
			outcome := RecoveryOutcomeUnknown
			if calls.Add(1) > 1 {
				outcome = RecoveryOutcomeNoMatchingInstance
			}
			return nil, map[string]RecoveryCandidateOutcome{row.SessionID: outcome}, nil
		},
	}
	manager, _ := task08ManagerWithBackend(t, backend)
	manager.SetExecutorRunningWriter(repo)
	manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{
		row.SessionID: task08RemoteSnapshot(row),
	}}
	manager.SetPassthroughLookup(alwaysNotPassthrough)
	manager.SetRecoveryDeadline(time.Second)
	t.Cleanup(func() { require.NoError(t, manager.Stop()) })

	require.NoError(t, manager.StartDeferredRemoteRecovery(ctx))
	manager.pollRemoteStatuses(ctx)
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded,
		"unknown evidence from the deferred first attempt must retain its guard")
	manager.pollRemoteStatuses(ctx)

	require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID))
	require.EqualValues(t, 2, calls.Load(), "the second deferred retry must consume its detailed absence result")
	current, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, current.Status)
}

func TestRemoteRecoveryAbsenceRequiresSupportedSuccessfulEvidence(t *testing.T) {
	tests := []struct {
		name      string
		runtime   agentruntime.Runtime
		outcome   RecoveryCandidateOutcome
		omit      bool
		returnErr bool
		legacy    bool
	}{
		{name: "unknown", runtime: agentruntime.RuntimeRemoteDocker, outcome: RecoveryOutcomeUnknown},
		{name: "omitted", runtime: agentruntime.RuntimeRemoteDocker, omit: true},
		{name: "failed attempt", runtime: agentruntime.RuntimeRemoteDocker, outcome: RecoveryOutcomeNoMatchingInstance, returnErr: true},
		{name: "legacy backend", runtime: agentruntime.RuntimeRemoteDocker, legacy: true},
		{name: "unsupported SSH proof", runtime: agentruntime.RuntimeSSH, outcome: RecoveryOutcomeNoMatchingInstance},
		{name: "unsupported Kubernetes proof", runtime: agentruntime.RuntimeKubernetes, outcome: RecoveryOutcomeNoMatchingInstance},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo, _ := runOwnerTestRepository(t)
			row := seedRemoteAbsenceRecord(t, repo, tt.runtime, tt.name)
			backendName := remoteRecoveryExecutorName(tt.runtime)
			var backend ExecutorBackend = &MockExecutor{name: backendName}
			if !tt.legacy {
				backend = &task08DetailedRecoveryBackend{
					MockExecutor: &MockExecutor{name: backendName},
					recoveryResult: func([]*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
						outcomes := map[string]RecoveryCandidateOutcome{}
						if !tt.omit {
							outcomes[row.SessionID] = tt.outcome
						}
						if tt.returnErr {
							return nil, outcomes, errors.New("provider enumeration failed")
						}
						return nil, outcomes, nil
					},
				}
			}
			manager, _ := task08ManagerWithBackend(t, backend)
			manager.SetExecutorRunningWriter(repo)
			manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{
				row.SessionID: task08RemoteSnapshot(row),
			}}
			manager.SetPassthroughLookup(alwaysNotPassthrough)
			manager.SetRecoveryDeadline(time.Second)
			t.Cleanup(func() { require.NoError(t, manager.Stop()) })

			require.NoError(t, manager.Start(ctx))
			require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
			current, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
			require.NoError(t, err)
			require.Equal(t, models.ExecutorRunningStatusRunning, current.Status)
		})
	}
}

func TestRemoteRecoveryAbsenceRetriesAfterRepairFailure(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	row := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeRemoteDocker, "cas-retry")
	writer := &remoteRecoveryCASWriter{Repository: repo, repair: func(context.Context, string, string, time.Time) error {
		return errors.New("injected compare-and-set failure")
	}}
	backend := remoteRecoveryNoMatchBackend(row)
	manager, _ := task08ManagerWithBackend(t, backend)
	manager.SetExecutorRunningWriter(writer)
	manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{
		row.SessionID: task08RemoteSnapshot(row),
	}}
	manager.SetPassthroughLookup(alwaysNotPassthrough)
	manager.SetRecoveryDeadline(time.Second)
	t.Cleanup(func() { require.NoError(t, manager.Stop()) })

	require.NoError(t, manager.Start(ctx))
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
	current, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, current.Status)

	writer.repair = nil
	manager.pollRemoteStatuses(ctx)
	require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID))
	current, err = repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, current.Status)
}

func TestRemoteRecoveryAbsenceConvergesAfterLostRepairResponse(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	row := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeSprites, "lost-response")
	var repairCalls atomic.Int32
	writer := &remoteRecoveryCASWriter{Repository: repo, repair: func(ctx context.Context, sessionID, executionID string, updatedAt time.Time) error {
		if err := repo.RepairExecutorRunningDeadIfCurrent(ctx, sessionID, executionID, updatedAt); err != nil {
			return err
		}
		if repairCalls.Add(1) == 1 {
			return errors.New("repair committed but response was lost")
		}
		return nil
	}}
	backend := remoteRecoveryNoMatchBackend(row)
	manager, _ := task08ManagerWithBackend(t, backend)
	manager.SetExecutorRunningWriter(writer)
	manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{
		row.SessionID: task08RemoteSnapshot(row),
	}}
	manager.SetPassthroughLookup(alwaysNotPassthrough)
	manager.SetRecoveryDeadline(time.Second)
	t.Cleanup(func() { require.NoError(t, manager.Stop()) })

	require.NoError(t, manager.Start(ctx))
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
	current, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, current.Status,
		"the committed repair must survive a lost response without prematurely releasing the guard")

	manager.pollRemoteStatuses(ctx)
	require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID))
	require.EqualValues(t, 2, repairCalls.Load(), "retry must re-read the repaired row timestamp before its CAS")

	restarted, _ := task08ManagerWithBackend(t, remoteRecoveryNoMatchBackend(row))
	restarted.SetExecutorRunningWriter(repo)
	require.NoError(t, restarted.PrepareStartupRecovery(ctx))
	require.Nil(t, restarted.pendingRemoteRecoveryForSession(row.SessionID),
		"a restart must not requeue the settled stopped row")
	require.NoError(t, restarted.recoveryGuard.CheckLaunchAllowed(row.SessionID))
	t.Cleanup(func() { require.NoError(t, restarted.Stop()) })
}

func TestRemoteRecoveryAbsencePreservesUnknownSibling(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	absent := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeRemoteDocker, "mixed-absent")
	unknown := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeSSH, "mixed-unknown")
	docker := remoteRecoveryNoMatchBackend(absent)
	ssh := &task08DetailedRecoveryBackend{
		MockExecutor:   &MockExecutor{name: executor.NameSSH},
		recoveryResult: task08UnknownOutcomes(agentruntime.RuntimeSSH),
	}
	manager, _ := task08ManagerWithBackend(t, docker)
	manager.executorRegistry.Register(ssh)
	manager.SetExecutorRunningWriter(repo)
	manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{
		absent.SessionID: task08RemoteSnapshot(absent), unknown.SessionID: task08RemoteSnapshot(unknown),
	}}
	manager.SetPassthroughLookup(alwaysNotPassthrough)
	manager.SetRecoveryDeadline(time.Second)
	t.Cleanup(func() { require.NoError(t, manager.Stop()) })

	require.NoError(t, manager.Start(ctx))
	require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(absent.SessionID))
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(unknown.SessionID), ErrSessionRecoveryGuarded)
	absentCurrent, err := repo.GetExecutorRunningBySessionID(ctx, absent.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, absentCurrent.Status)
	unknownCurrent, err := repo.GetExecutorRunningBySessionID(ctx, unknown.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, unknownCurrent.Status)
}

func TestRemoteRecoveryAbsencePreservesTrackedSuccessor(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	row := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeRemoteDocker, "tracked-successor")
	var repairCalls atomic.Int32
	writer := &remoteRecoveryCASWriter{Repository: repo, repair: func(ctx context.Context, sessionID, executionID string, updatedAt time.Time) error {
		repairCalls.Add(1)
		return repo.RepairExecutorRunningDeadIfCurrent(ctx, sessionID, executionID, updatedAt)
	}}
	manager, _ := task08ManagerWithBackend(t, remoteRecoveryNoMatchBackend(row))
	manager.SetExecutorRunningWriter(writer)
	entry := manager.queueRemoteRecovery(row, task08RemoteSnapshot(row))
	require.True(t, manager.beginRemoteRecovery(entry))
	successor := &AgentExecution{
		ID: "execution-successor", TaskID: row.TaskID, SessionID: row.SessionID,
		RuntimeName: agentruntime.RuntimeRemoteDocker,
	}
	require.NoError(t, manager.executionStore.Add(successor))
	attemptRecord, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.NoError(t, manager.settleRemoteRecoveryAbsence(ctx, entry, attemptRecord, RecoveryOutcomeNoMatchingInstance))
	manager.finishRemoteRecoveryAttempt(entry)

	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
	require.Zero(t, repairCalls.Load())
	current, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, current.Status)
}

func TestRemoteRecoveryAbsenceWithoutCASSupportRemainsGuarded(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	row := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeSprites, "no-cas")
	manager, _ := task08ManagerWithBackend(t, remoteRecoveryNoMatchBackend(row))
	manager.SetExecutorRunningWriter(&remoteRecoveryReadOnlyWriter{ExecutorRunningWriter: repo, repository: repo})
	manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{
		row.SessionID: task08RemoteSnapshot(row),
	}}
	manager.SetPassthroughLookup(alwaysNotPassthrough)
	manager.SetRecoveryDeadline(time.Second)
	t.Cleanup(func() { require.NoError(t, manager.Stop()) })

	require.NoError(t, manager.Start(ctx))
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
	current, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, current.Status)
}

func TestRemoteRecoveryAbsenceRejectsChangedRuntimeRow(t *testing.T) {
	for _, rotateExecution := range []bool{false, true} {
		name := "updated row"
		if rotateExecution {
			name = "rotated execution"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo, _ := runOwnerTestRepository(t)
			row := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeRemoteDocker, "changed-"+name)
			entered := make(chan struct{})
			release := make(chan struct{})
			var repairCalls atomic.Int32
			writer := &remoteRecoveryCASWriter{Repository: repo, repair: func(ctx context.Context, sessionID, executionID string, updatedAt time.Time) error {
				repairCalls.Add(1)
				return repo.RepairExecutorRunningDeadIfCurrent(ctx, sessionID, executionID, updatedAt)
			}}
			backend := &task08DetailedRecoveryBackend{
				MockExecutor: &MockExecutor{name: executor.NameRemoteDocker},
				recoveryResult: func([]*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
					close(entered)
					<-release
					return nil, map[string]RecoveryCandidateOutcome{row.SessionID: RecoveryOutcomeNoMatchingInstance}, nil
				},
			}
			manager, _ := task08ManagerWithBackend(t, backend)
			manager.SetExecutorRunningWriter(writer)
			manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{
				row.SessionID: task08RemoteSnapshot(row),
			}}
			manager.SetPassthroughLookup(alwaysNotPassthrough)
			manager.SetRecoveryDeadline(time.Second)
			t.Cleanup(func() { require.NoError(t, manager.Stop()) })
			startDone := make(chan error, 1)
			go func() { startDone <- manager.Start(ctx) }()
			<-entered

			successor := cloneRemoteRecoveryRecord(row)
			if rotateExecution {
				successor.AgentExecutionID += "-successor"
			} else {
				successor.ResumeToken = "resume-successor"
			}
			require.NoError(t, repo.UpsertExecutorRunning(ctx, successor))
			close(release)
			require.NoError(t, <-startDone)

			require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
			require.Zero(t, repairCalls.Load(), "a row changed after provider evidence must never reach CAS repair")
			current, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
			require.NoError(t, err)
			require.Equal(t, models.ExecutorRunningStatusRunning, current.Status)
			if rotateExecution {
				require.Equal(t, successor.AgentExecutionID, current.AgentExecutionID)
			} else {
				require.Equal(t, "resume-successor", current.ResumeToken)
			}
		})
	}
}

func TestRemoteRecoveryAbsenceRespectsStopAndShutdownFences(t *testing.T) {
	for _, fence := range []string{"stop", "shutdown"} {
		t.Run(fence, func(t *testing.T) {
			ctx := startup.WithReporter(context.Background(), startup.New(nil))
			repo, _ := runOwnerTestRepository(t)
			row := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeSprites, "fence-"+fence)
			entered := make(chan struct{})
			release := make(chan struct{})
			var repairCalls atomic.Int32
			writer := &remoteRecoveryCASWriter{Repository: repo, repair: func(ctx context.Context, sessionID, executionID string, updatedAt time.Time) error {
				repairCalls.Add(1)
				return repo.RepairExecutorRunningDeadIfCurrent(ctx, sessionID, executionID, updatedAt)
			}}
			backend := &task08DetailedRecoveryBackend{
				MockExecutor: &MockExecutor{name: executor.NameSprites},
				recoveryResult: func([]*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
					close(entered)
					<-release
					return nil, map[string]RecoveryCandidateOutcome{row.SessionID: RecoveryOutcomeNoMatchingInstance}, nil
				},
			}
			manager, _ := task08ManagerWithBackend(t, backend)
			manager.SetExecutorRunningWriter(writer)
			manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{
				row.SessionID: task08RemoteSnapshot(row),
			}}
			manager.SetPassthroughLookup(alwaysNotPassthrough)
			manager.SetRecoveryDeadline(time.Second)
			t.Cleanup(func() { require.NoError(t, manager.Stop()) })
			require.NoError(t, manager.StartDeferredRemoteRecovery(ctx))
			pollDone := make(chan struct{})
			go func() {
				manager.pollRemoteStatuses(ctx)
				close(pollDone)
			}()
			<-entered
			var attemptDone <-chan struct{}
			if fence == "stop" {
				pending, done := manager.requestPendingRemoteRecoveryStop(row.AgentExecutionID, StopReasonTaskDeleted, false)
				require.NotNil(t, pending)
				attemptDone = done
				require.NotNil(t, attemptDone, "Stop must wait for the in-flight attachment disposition")
			} else {
				manager.shuttingDown.Store(true)
			}
			close(release)
			<-pollDone
			if attemptDone != nil {
				<-attemptDone
			}

			require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
			require.Zero(t, repairCalls.Load(), "accepted Stop and shutdown prevent absence repair")
			current, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
			require.NoError(t, err)
			require.Equal(t, models.ExecutorRunningStatusRunning, current.Status)
		})
	}
}

func seedRemoteAbsenceRecord(t *testing.T, repo *tasksqlite.Repository, runtime agentruntime.Runtime, suffix string) *models.ExecutorRunning {
	t.Helper()
	ctx := context.Background()
	workspaceID := "workspace-absence-" + suffix
	taskID := "task-absence-" + suffix
	sessionID := "session-absence-" + suffix
	executionID := "execution-absence-" + suffix
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: suffix}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: suffix}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, QueueIncarnationID: "incarnation-" + suffix}))
	record := &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Runtime: runtime, Status: models.ExecutorRunningStatusRunning, Resumable: true,
		ResumeToken: "resume-" + suffix, WorktreePath: "/workspace/" + suffix, LocalPID: 73,
		Metadata: map[string]interface{}{"preserved": suffix},
	}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, record))
	return record
}

func remoteRecoveryExecutorName(runtime agentruntime.Runtime) executor.Name {
	switch runtime {
	case agentruntime.RuntimeRemoteDocker:
		return executor.NameRemoteDocker
	case agentruntime.RuntimeSprites:
		return executor.NameSprites
	case agentruntime.RuntimeKubernetes:
		return executor.NameKubernetes
	default:
		return executor.NameSSH
	}
}

func remoteRecoveryNoMatchBackend(record *models.ExecutorRunning) *task08DetailedRecoveryBackend {
	return &task08DetailedRecoveryBackend{
		MockExecutor: &MockExecutor{name: remoteRecoveryExecutorName(record.Runtime)},
		recoveryResult: func([]*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
			return nil, map[string]RecoveryCandidateOutcome{record.SessionID: RecoveryOutcomeNoMatchingInstance}, nil
		},
	}
}
