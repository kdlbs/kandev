package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	workspaceRecoveryTaskID        = "task-workspace-recovery"
	workspaceRecoverySessionID     = "session-workspace-recovery"
	workspaceRecoveryEnvironmentID = "environment-workspace-recovery"
	workspaceRecoveryIncarnationID = "incarnation-workspace-recovery"
)

type workspaceRecoveryReader struct {
	*fakeExecutorProfileReader
	generation     *models.HarnessSessionGeneration
	generationErr  error
	block          *models.SessionRecoveryBlock
	blockErr       error
	generationRead int
	blockRead      int
}

func (r *workspaceRecoveryReader) GetCurrentHarnessSessionGeneration(
	context.Context,
	string,
	string,
) (*models.HarnessSessionGeneration, error) {
	r.generationRead++
	return r.generation, r.generationErr
}

func (r *workspaceRecoveryReader) GetOpenSessionRecoveryBlock(
	context.Context,
	string,
	string,
	int64,
) (*models.SessionRecoveryBlock, error) {
	r.blockRead++
	return r.block, r.blockErr
}

func newWorkspaceRecoveryFixture(
	t *testing.T,
	metadata map[string]interface{},
	continuity *workspaceRecoveryReader,
) (*Manager, *createInstanceExecutor, *captureExecutorRunningWriter) {
	t.Helper()
	info := &WorkspaceInfo{
		TaskID:            workspaceRecoveryTaskID,
		SessionID:         workspaceRecoverySessionID,
		TaskEnvironmentID: workspaceRecoveryEnvironmentID,
		WorkspacePath:     "/workspace/task",
		AgentID:           "auggie",
	}
	provider := &mockWorkspaceInfoProvider{
		infos:    map[string]*WorkspaceInfo{workspaceRecoverySessionID: info},
		envInfos: map[string]*WorkspaceInfo{workspaceRecoveryEnvironmentID: info},
	}
	manager, backend := newEnvironmentExecutionTestManager(t, provider)
	baseReader := &fakeExecutorProfileReader{
		task: &models.Task{ID: workspaceRecoveryTaskID},
		session: &models.TaskSession{
			ID: workspaceRecoverySessionID, TaskID: workspaceRecoveryTaskID,
			TaskEnvironmentID:  workspaceRecoveryEnvironmentID,
			QueueIncarnationID: workspaceRecoveryIncarnationID,
			Metadata:           metadata,
		},
		env: &models.TaskEnvironment{
			ID: workspaceRecoveryEnvironmentID, TaskID: workspaceRecoveryTaskID,
		},
	}
	if continuity == nil {
		manager.SetExecutorProfileReader(baseReader)
	} else {
		continuity.fakeExecutorProfileReader = baseReader
		manager.SetExecutorProfileReader(continuity)
	}
	writer := &captureExecutorRunningWriter{prior: &models.ExecutorRunning{
		ID: "running-prior", SessionID: workspaceRecoverySessionID,
		TaskID: workspaceRecoveryTaskID, AgentExecutionID: "execution-prior",
	}}
	manager.SetExecutorRunningWriter(writer)
	return manager, backend, writer
}

func activeWorkspaceRecoveryMetadata(phase string, generation int64) map[string]interface{} {
	return map[string]interface{}{
		models.SessionMetaKeyAgentDeliveryRecovery: models.AgentDeliveryRecovery{
			Phase: phase, Revision: 1, SessionID: workspaceRecoverySessionID,
			SubmissionID:      "submission-workspace-recovery",
			IncarnationID:     workspaceRecoveryIncarnationID,
			HarnessGeneration: generation,
		},
	}
}

func currentWorkspaceRecoveryGeneration(generation int64) *models.HarnessSessionGeneration {
	return &models.HarnessSessionGeneration{
		SessionID:     workspaceRecoverySessionID,
		IncarnationID: workspaceRecoveryIncarnationID,
		Generation:    generation,
	}
}

func TestWorkspaceOnlyAllocationRejectsUnresolvedRecovery(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(context.Context, *Manager) error
	}{
		{
			name: "get or ensure by session",
			invoke: func(ctx context.Context, manager *Manager) error {
				_, err := manager.GetOrEnsureExecution(ctx, workspaceRecoverySessionID)
				return err
			},
		},
		{
			name: "ensure by session",
			invoke: func(ctx context.Context, manager *Manager) error {
				_, err := manager.EnsureWorkspaceExecutionForSession(ctx, workspaceRecoveryTaskID, workspaceRecoverySessionID)
				return err
			},
		},
		{
			name: "get or ensure by environment",
			invoke: func(ctx context.Context, manager *Manager) error {
				_, err := manager.GetOrEnsureExecutionForEnvironment(ctx, workspaceRecoveryEnvironmentID)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &workspaceRecoveryReader{
				generation: currentWorkspaceRecoveryGeneration(3),
				blockErr:   sql.ErrNoRows,
			}
			manager, backend, writer := newWorkspaceRecoveryFixture(
				t, activeWorkspaceRecoveryMetadata(models.AgentDeliveryRecoveryUncertain, 3), reader,
			)
			if err := tt.invoke(context.Background(), manager); !errors.Is(err, ErrSessionRecoveryRequired) {
				t.Fatalf("workspace allocation error = %v, want %v", err, ErrSessionRecoveryRequired)
			}
			if got := backend.createCount.Load(); got != 0 {
				t.Fatalf("runtime creation count = %d, want 0", got)
			}
			if writer.running != nil || writer.prior.AgentExecutionID != "execution-prior" {
				t.Fatalf("executor-running row changed: prior=%+v current=%+v", writer.prior, writer.running)
			}
			if _, exists := manager.executionStore.GetBySessionID(workspaceRecoverySessionID); exists {
				t.Fatal("blocked workspace allocation registered an execution")
			}
		})
	}
}

func TestWorkspaceOnlyAllocationFailsClosedOnMalformedRecoveryMetadata(t *testing.T) {
	manager, backend, writer := newWorkspaceRecoveryFixture(t, map[string]interface{}{
		models.SessionMetaKeyAgentDeliveryRecovery: map[string]interface{}{"phase": models.AgentDeliveryRecoveryUncertain},
	}, &workspaceRecoveryReader{generation: currentWorkspaceRecoveryGeneration(3)})

	_, err := manager.GetOrEnsureExecution(context.Background(), workspaceRecoverySessionID)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("workspace allocation error = %v, want %v", err, ErrSessionRecoveryRequired)
	}
	if got := backend.createCount.Load(); got != 0 || writer.running != nil {
		t.Fatalf("malformed recovery metadata allowed allocation: creates=%d row=%+v", got, writer.running)
	}
}

func TestWorkspaceOnlyAllocationFailsClosedWhenRecoveryReaderIsUnavailable(t *testing.T) {
	manager, backend, writer := newWorkspaceRecoveryFixture(
		t, activeWorkspaceRecoveryMetadata(models.AgentDeliveryRecoveryReconnecting, 3), nil,
	)

	_, err := manager.GetOrEnsureExecution(context.Background(), workspaceRecoverySessionID)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("workspace allocation error = %v, want %v", err, ErrSessionRecoveryRequired)
	}
	if got := backend.createCount.Load(); got != 0 || writer.running != nil {
		t.Fatalf("missing recovery reader allowed allocation: creates=%d row=%+v", got, writer.running)
	}
}

func TestWorkspaceOnlyAllocationRejectsActiveRecoveryBeforeGenerationCheck(t *testing.T) {
	reader := &workspaceRecoveryReader{
		generation: currentWorkspaceRecoveryGeneration(4),
		blockErr:   sql.ErrNoRows,
	}
	manager, backend, writer := newWorkspaceRecoveryFixture(
		t, activeWorkspaceRecoveryMetadata(models.AgentDeliveryRecoveryUncertain, 3), reader,
	)

	_, err := manager.GetOrEnsureExecution(context.Background(), workspaceRecoverySessionID)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("workspace allocation error = %v, want %v", err, ErrSessionRecoveryRequired)
	}
	if got := backend.createCount.Load(); got != 0 || writer.running != nil {
		t.Fatalf("stale recovery generation allowed allocation: creates=%d row=%+v", got, writer.running)
	}
	if reader.generationRead != 0 || reader.blockRead != 0 {
		t.Fatalf("active recovery reached durable block check: generation reads=%d block reads=%d", reader.generationRead, reader.blockRead)
	}
}

func TestWorkspaceOnlyAllocationFailsClosedOnUnknownRecoveryPhase(t *testing.T) {
	manager, backend, writer := newWorkspaceRecoveryFixture(
		t, activeWorkspaceRecoveryMetadata("future-phase", 3),
		&workspaceRecoveryReader{generation: currentWorkspaceRecoveryGeneration(3), blockErr: sql.ErrNoRows},
	)

	_, err := manager.GetOrEnsureExecution(context.Background(), workspaceRecoverySessionID)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("workspace allocation error = %v, want %v", err, ErrSessionRecoveryRequired)
	}
	if got := backend.createCount.Load(); got != 0 || writer.running != nil {
		t.Fatalf("unknown recovery phase allowed allocation: creates=%d row=%+v", got, writer.running)
	}
}

func TestWorkspaceOnlyAllocationFailsClosedWhenRecoveryReadFails(t *testing.T) {
	reader := &workspaceRecoveryReader{
		generationErr: errors.New("generation store unavailable"),
	}
	manager, backend, writer := newWorkspaceRecoveryFixture(t, nil, reader)

	_, err := manager.GetOrEnsureExecution(context.Background(), workspaceRecoverySessionID)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("workspace allocation error = %v, want %v", err, ErrSessionRecoveryRequired)
	}
	if got := backend.createCount.Load(); got != 0 || writer.running != nil {
		t.Fatalf("recovery read failure allowed allocation: creates=%d row=%+v", got, writer.running)
	}
}

func TestWorkspaceOnlyAllocationAllowsResolvedHistoricalRecovery(t *testing.T) {
	metadata := activeWorkspaceRecoveryMetadata(models.AgentDeliveryRecoveryContinued, 1)
	reader := &workspaceRecoveryReader{
		generation: currentWorkspaceRecoveryGeneration(2),
		blockErr:   sql.ErrNoRows,
	}
	manager, backend, _ := newWorkspaceRecoveryFixture(t, metadata, reader)

	execution, err := manager.GetOrEnsureExecution(context.Background(), workspaceRecoverySessionID)
	if err != nil {
		t.Fatalf("workspace allocation returned error: %v", err)
	}
	if execution == nil {
		t.Fatal("workspace allocation returned nil execution")
	}
	if got := backend.createCount.Load(); got != 1 {
		t.Fatalf("runtime creation count = %d, want 1", got)
	}
	if reader.generationRead == 0 || reader.blockRead == 0 {
		t.Fatalf("resolved history skipped current block check: generation reads=%d block reads=%d", reader.generationRead, reader.blockRead)
	}
}

func TestWorkspaceOnlyAllocationChecksCurrentBlockAfterResolvedRecovery(t *testing.T) {
	block := &models.SessionRecoveryBlock{
		ID: "block-workspace-recovery", SessionID: workspaceRecoverySessionID,
		IncarnationID: workspaceRecoveryIncarnationID, ExpectedGeneration: 2,
		Reason: "recovery pending", State: models.RecoveryBlockOpen,
	}
	reader := &workspaceRecoveryReader{
		generation: currentWorkspaceRecoveryGeneration(2),
		block:      block,
	}
	manager, backend, writer := newWorkspaceRecoveryFixture(
		t, activeWorkspaceRecoveryMetadata(models.AgentDeliveryRecoveryContinued, 1), reader,
	)

	_, err := manager.GetOrEnsureExecution(context.Background(), workspaceRecoverySessionID)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("workspace allocation error = %v, want %v", err, ErrSessionRecoveryRequired)
	}
	if got := backend.createCount.Load(); got != 0 || writer.running != nil {
		t.Fatalf("open current recovery block allowed allocation: creates=%d row=%+v", got, writer.running)
	}
}

func TestWorkspaceOnlyAllocationFailsClosedWhenResolvedRecoveryHasNoCurrentGeneration(t *testing.T) {
	reader := &workspaceRecoveryReader{generationErr: models.ErrTaskSessionNotFound}
	manager, backend, writer := newWorkspaceRecoveryFixture(
		t, activeWorkspaceRecoveryMetadata(models.AgentDeliveryRecoveryRestored, 1), reader,
	)

	_, err := manager.GetOrEnsureExecution(context.Background(), workspaceRecoverySessionID)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("workspace allocation error = %v, want %v", err, ErrSessionRecoveryRequired)
	}
	if got := backend.createCount.Load(); got != 0 || writer.running != nil {
		t.Fatalf("missing current generation allowed allocation: creates=%d row=%+v", got, writer.running)
	}
}

func TestWorkspaceOnlyAllocationLeavesCachedExecutionUsableDuringRecovery(t *testing.T) {
	reader := &workspaceRecoveryReader{
		generation: currentWorkspaceRecoveryGeneration(3),
		block: &models.SessionRecoveryBlock{
			ID: "block-workspace-recovery", SessionID: workspaceRecoverySessionID,
			IncarnationID: workspaceRecoveryIncarnationID, ExpectedGeneration: 3,
			State: models.RecoveryBlockOpen,
		},
	}
	manager, _, _ := newWorkspaceRecoveryFixture(
		t, activeWorkspaceRecoveryMetadata(models.AgentDeliveryRecoveryUncertain, 3), reader,
	)
	retained := &AgentExecution{
		ID: "execution-cached", TaskID: workspaceRecoveryTaskID, SessionID: workspaceRecoverySessionID,
	}
	if err := manager.executionStore.Add(retained); err != nil {
		t.Fatalf("add retained execution: %v", err)
	}

	got, err := manager.GetOrEnsureExecution(context.Background(), workspaceRecoverySessionID)
	if err != nil {
		t.Fatalf("cached workspace access returned error: %v", err)
	}
	if got != retained {
		t.Fatalf("cached execution = %p, want %p", got, retained)
	}
	if reader.generationRead != 0 || reader.blockRead != 0 {
		t.Fatalf("cached access queried recovery admission: generation reads=%d block reads=%d", reader.generationRead, reader.blockRead)
	}
}

func TestWorkspaceOnlyAllocationKeepsOrdinaryReaderCompatible(t *testing.T) {
	manager, backend, _ := newWorkspaceRecoveryFixture(t, nil, nil)

	execution, err := manager.GetOrEnsureExecution(context.Background(), workspaceRecoverySessionID)
	if err != nil {
		t.Fatalf("ordinary workspace allocation returned error: %v", err)
	}
	if execution == nil || backend.createCount.Load() != 1 {
		t.Fatalf("ordinary workspace allocation = %v, runtime creations = %d", execution, backend.createCount.Load())
	}
}
