package storeconformance

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

type silentRestoreScenarioStore interface {
	CreateWorkspace(ctx context.Context, workspace *models.Workspace) error
	CreateTask(ctx context.Context, task *models.Task) error
	CreateTaskSession(ctx context.Context, session *models.TaskSession) error
	CreateHarnessSessionGeneration(ctx context.Context, generation *models.HarnessSessionGeneration) error
	PrepareAgentDeliverySubmission(ctx context.Context, submission *models.AgentDeliverySubmission) (bool, error)
	UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error
	UpsertAgentDeliveryRecovery(ctx context.Context, recovery *models.AgentDeliveryRecovery, block *models.SessionRecoveryBlock) (bool, error)
	GetCurrentHarnessSessionGeneration(ctx context.Context, sessionID, incarnationID string) (*models.HarnessSessionGeneration, error)
	GetSessionRecoveryBlock(ctx context.Context, id string) (*models.SessionRecoveryBlock, error)
	GetAgentDeliverySubmission(ctx context.Context, id string) (*models.AgentDeliverySubmission, error)
}

type silentRestoreFixture struct {
	id           string
	workspace    *models.Workspace
	taskID       string
	session      *models.TaskSession
	submissionID string
	blockID      string
	candidateID  string
	attemptID    string
	recovery     models.AgentDeliveryRecovery
	generation   *models.HarnessSessionGeneration
	block        *models.SessionRecoveryBlock
}

func silentRestoreRepositoryCheck(s testconformance.ScenarioContext, id string) error {
	store, setup, err := silentRestoreStores(s)
	if err != nil {
		return err
	}
	fixture := newSilentRestoreFixture(id)
	if err := createSilentRestoreOwner(s, setup, fixture); err != nil {
		return err
	}
	if err := persistSilentRestoreSource(s, setup, fixture); err != nil {
		return err
	}
	launching, err := prepareSilentRestoreCandidate(s, store, setup, fixture)
	if err != nil {
		return err
	}
	return commitAndVerifySilentRestore(s, store, setup, fixture, launching)
}

func silentRestoreStores(
	s testconformance.ScenarioContext,
) (taskrepository.SilentRestoreRepository, silentRestoreScenarioStore, error) {
	value, err := taskFactory(s)
	if err != nil {
		return nil, nil, err
	}
	store, ok := value.(taskrepository.SilentRestoreRepository)
	if !ok {
		return nil, nil, fmt.Errorf("task store %T does not implement SilentRestoreRepository", value)
	}
	setup, ok := value.(silentRestoreScenarioStore)
	if !ok {
		return nil, nil, fmt.Errorf("task store %T does not implement silent-restore fixture methods", value)
	}
	return store, setup, nil
}

func newSilentRestoreFixture(id string) *silentRestoreFixture {
	return &silentRestoreFixture{
		id: id,
		workspace: &models.Workspace{
			ID: id + "-silent-workspace", Name: "Silent restore conformance", OwnerID: id, OrgID: "conformance-org",
		},
		taskID:       id + "-silent-task",
		session:      &models.TaskSession{ID: id + "-silent-session", TaskID: id + "-silent-task", State: models.TaskSessionStateRunning},
		submissionID: id + "-silent-submission",
		blockID:      id + "-silent-block",
		candidateID:  id + "-silent-candidate",
		attemptID:    id + "-silent-attempt",
	}
}

func createSilentRestoreOwner(s testconformance.ScenarioContext, setup silentRestoreScenarioStore, fixture *silentRestoreFixture) error {
	now := time.Now().UTC()
	fixture.session.StartedAt = now
	fixture.session.UpdatedAt = now
	if err := setup.CreateWorkspace(s.Context, fixture.workspace); err != nil {
		return fmt.Errorf("create silent-restore workspace: %w", err)
	}
	if err := setup.CreateTask(s.Context, &models.Task{
		ID: fixture.taskID, WorkspaceID: fixture.workspace.ID, Title: "Silent restore conformance",
	}); err != nil {
		return fmt.Errorf("create silent-restore task: %w", err)
	}
	if err := setup.CreateTaskSession(s.Context, fixture.session); err != nil {
		return fmt.Errorf("create silent-restore session: %w", err)
	}
	if err := setup.CreateHarnessSessionGeneration(s.Context, &models.HarnessSessionGeneration{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		Generation: 1, NativeSessionID: fixture.id + "-silent-native", CreationReason: "initial",
	}); err != nil {
		return fmt.Errorf("create silent-restore source generation: %w", err)
	}
	if err := setup.UpsertExecutorRunning(s.Context, &models.ExecutorRunning{
		ID: fixture.session.ID, SessionID: fixture.session.ID, TaskID: fixture.taskID,
		ExecutorID: "conformance-executor", AgentExecutionID: fixture.id + "-silent-old-execution", Status: "running",
	}); err != nil {
		return fmt.Errorf("create silent-restore source execution: %w", err)
	}
	return nil
}

func persistSilentRestoreSource(s testconformance.ScenarioContext, setup silentRestoreScenarioStore, fixture *silentRestoreFixture) error {
	_, err := setup.PrepareAgentDeliverySubmission(s.Context, &models.AgentDeliverySubmission{
		ID: fixture.submissionID, SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: fixture.id + "-silent-hash",
		Payload: []byte("immutable conformance prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	})
	if err != nil {
		return fmt.Errorf("create silent-restore source submission: %w", err)
	}
	fixture.recovery = models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryUncertain, SessionID: fixture.session.ID,
		AgentExecutionID: fixture.id + "-silent-old-execution", SubmissionID: fixture.submissionID,
		StreamID: fixture.id + "-silent-stream", IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: 1, PromptGeneration: 1,
	}
	fixture.block = &models.SessionRecoveryBlock{
		ID: fixture.blockID, SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: fixture.submissionID, DeliveryStreamID: fixture.recovery.StreamID,
	}
	if _, err := setup.UpsertAgentDeliveryRecovery(s.Context, &fixture.recovery, fixture.block); err != nil {
		return fmt.Errorf("persist silent-restore recovery source: %w", err)
	}
	fixture.generation, err = setup.GetCurrentHarnessSessionGeneration(s.Context, fixture.session.ID, fixture.session.QueueIncarnationID)
	if err != nil {
		return fmt.Errorf("read silent-restore source generation: %w", err)
	}
	if fixture.generation == nil {
		return fmt.Errorf("read silent-restore source generation: repository returned nil")
	}
	fixture.block, err = setup.GetSessionRecoveryBlock(s.Context, fixture.blockID)
	if err != nil {
		return fmt.Errorf("read silent-restore source block: %w", err)
	}
	if fixture.block == nil {
		return fmt.Errorf("read silent-restore source block: repository returned nil")
	}
	return nil
}

func prepareSilentRestoreCandidate(
	s testconformance.ScenarioContext,
	store taskrepository.SilentRestoreRepository,
	setup silentRestoreScenarioStore,
	fixture *silentRestoreFixture,
) (models.SilentRestoreCheckpoint, error) {
	attempt := newSilentRestoreAttempt(fixture)
	storedAttempt, created, err := store.EnsureRestoreAttempt(s.Context, attempt)
	if err != nil || !created || storedAttempt == nil || storedAttempt.Checkpoint == nil {
		return models.SilentRestoreCheckpoint{}, fmt.Errorf("create silent-restore attempt: created=%t error=%v", created, err)
	}
	retriedAttempt, created, err := store.EnsureRestoreAttempt(s.Context, attempt)
	if err != nil || created || retriedAttempt == nil || retriedAttempt.ID != attempt.ID || retriedAttempt.Checkpoint == nil {
		id := ""
		if retriedAttempt != nil {
			id = retriedAttempt.ID
		}
		return models.SilentRestoreCheckpoint{}, fmt.Errorf("retry silent-restore attempt: id=%q created=%t error=%v", id, created, err)
	}
	if err := verifySilentRestoreCandidateListed(s, store, fixture); err != nil {
		return models.SilentRestoreCheckpoint{}, err
	}
	return advanceSilentRestoreCandidate(s, setup, store, fixture, storedAttempt.Checkpoint)
}

func newSilentRestoreAttempt(fixture *silentRestoreFixture) *models.RestoreAttempt {
	return &models.RestoreAttempt{
		ID: fixture.attemptID, SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		ExpectedGeneration: 1, Action: models.SilentRestoreAction, Outcome: models.SilentRestoreOutcomePending,
		TargetWorkspace: fixture.workspace.ID,
		Checkpoint: &models.SilentRestoreCheckpoint{
			Version: 1, Stage: models.SilentRestoreStagePrepared,
			SourceRecovery: fixture.recovery, SourceGeneration: *fixture.generation, SourceBlock: *fixture.block,
			TaskID: fixture.taskID, WorkspaceID: fixture.workspace.ID,
			WorkspaceOwnerID: fixture.workspace.OwnerID, WorkspaceOrgID: fixture.workspace.OrgID,
		},
	}
}

func verifySilentRestoreCandidateListed(
	s testconformance.ScenarioContext,
	store taskrepository.SilentRestoreRepository,
	fixture *silentRestoreFixture,
) error {
	candidates, err := store.ListSilentRestoreCandidates(s.Context, "", 200)
	if err != nil {
		return fmt.Errorf("list silent-restore candidates: %w", err)
	}
	for _, candidate := range candidates {
		if candidate.SessionID == fixture.session.ID && candidate.TaskID == fixture.taskID &&
			candidate.WorkspaceID == fixture.workspace.ID {
			return nil
		}
	}
	return fmt.Errorf("silent-restore candidate %q was not listed", fixture.session.ID)
}

func advanceSilentRestoreCandidate(
	s testconformance.ScenarioContext,
	setup silentRestoreScenarioStore,
	store taskrepository.SilentRestoreRepository,
	fixture *silentRestoreFixture,
	prepared *models.SilentRestoreCheckpoint,
) (models.SilentRestoreCheckpoint, error) {
	allocated := *prepared
	allocated.Stage = models.SilentRestoreStageCandidateAllocated
	allocated.CandidateExecutionID = fixture.candidateID
	changed, err := store.CompareAndSwapSilentRestoreCheckpoint(s.Context, fixture.attemptID, *prepared, allocated)
	if err != nil || !changed {
		return models.SilentRestoreCheckpoint{}, fmt.Errorf("allocate silent-restore candidate: changed=%t error=%v", changed, err)
	}
	launching := allocated
	launching.Stage = models.SilentRestoreStageCandidateLaunching
	changed, err = store.CompareAndSwapSilentRestoreCheckpoint(s.Context, fixture.attemptID, allocated, launching)
	if err != nil || !changed {
		return models.SilentRestoreCheckpoint{}, fmt.Errorf("mark silent-restore candidate launching: changed=%t error=%v", changed, err)
	}
	if err := setup.UpsertExecutorRunning(s.Context, &models.ExecutorRunning{
		ID: fixture.session.ID, SessionID: fixture.session.ID, TaskID: fixture.taskID,
		ExecutorID: "conformance-executor", AgentExecutionID: fixture.candidateID, Status: "running",
	}); err != nil {
		return models.SilentRestoreCheckpoint{}, fmt.Errorf("persist silent-restore candidate execution: %w", err)
	}
	return launching, nil
}

func commitAndVerifySilentRestore(
	s testconformance.ScenarioContext,
	store taskrepository.SilentRestoreRepository,
	setup silentRestoreScenarioStore,
	fixture *silentRestoreFixture,
	launching models.SilentRestoreCheckpoint,
) error {
	next := *fixture.generation
	next.Generation = 2
	next.PredecessorGeneration = 1
	next.CreationReason = models.SilentRestoreAction
	committed, err := store.CommitSilentRestore(s.Context, &models.SilentRestoreCommit{
		AttemptID: fixture.attemptID, Checkpoint: launching, Generation: next,
	})
	if err != nil || !committed {
		return fmt.Errorf("commit silent restore: committed=%t error=%v", committed, err)
	}
	if err := verifySilentRestoreTerminal(s, store, setup, fixture); err != nil {
		return err
	}
	return nil
}

func verifySilentRestoreTerminal(
	s testconformance.ScenarioContext,
	store taskrepository.SilentRestoreRepository,
	setup silentRestoreScenarioStore,
	fixture *silentRestoreFixture,
) error {
	terminal, err := store.GetRestoreAttempt(s.Context, fixture.attemptID)
	if err != nil || terminal == nil || terminal.Outcome != models.SilentRestoreOutcomeRestored ||
		terminal.Checkpoint == nil || terminal.Checkpoint.Stage != models.SilentRestoreStageTerminal {
		return fmt.Errorf("read terminal silent-restore attempt: attempt=%+v error=%v", terminal, err)
	}
	resolved, err := setup.GetSessionRecoveryBlock(s.Context, fixture.blockID)
	if err != nil || resolved == nil || resolved.State != models.RecoveryBlockResolved {
		return fmt.Errorf("read resolved silent-restore block: block=%+v error=%v", resolved, err)
	}
	oldSubmission, err := setup.GetAgentDeliverySubmission(s.Context, fixture.submissionID)
	if err != nil || oldSubmission == nil || oldSubmission.State != models.DeliverySubmissionInterruptedUnknown ||
		string(oldSubmission.Payload) != "immutable conformance prompt" {
		return fmt.Errorf("read preserved uncertain submission: submission=%+v error=%v", oldSubmission, err)
	}
	return nil
}
