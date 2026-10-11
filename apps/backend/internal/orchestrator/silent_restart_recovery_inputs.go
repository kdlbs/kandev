package orchestrator

import (
	"context"
	"errors"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

type silentRestoreOwnerContextResolver func(context.Context, string, string) (context.Context, error)

type silentRestoreWorkspaceReader interface {
	GetWorkspace(context.Context, string) (*models.Workspace, error)
}

type silentRestoreInputs struct {
	session        *models.TaskSession
	recovery       models.AgentDeliveryRecovery
	generation     *models.HarnessSessionGeneration
	block          *models.SessionRecoveryBlock
	task           *models.Task
	workspace      *models.Workspace
	ownerCtx       context.Context
	checkpoint     models.SilentRestoreCheckpoint
	launchAttempts int
}

func (s *Service) loadSilentRestoreInputs(
	ctx context.Context,
	candidate models.SilentRestoreCandidate,
) (*silentRestoreInputs, error) {
	owner, eligible, err := s.loadSilentRestoreOwner(ctx, candidate)
	if err != nil || !eligible {
		return nil, err
	}
	session, recovery, eligible, err := s.loadSilentRestoreSession(
		owner.ctx, candidate, owner.task.ID,
	)
	if err != nil || !eligible {
		return nil, err
	}
	generation, block, eligible, err := s.loadSilentRestoreContinuity(
		owner.ctx, session, recovery,
	)
	if err != nil || !eligible || !silentRestoreSessionRouteEligible(session) {
		return nil, err
	}
	checkpoint := models.SilentRestoreCheckpoint{
		Version: 1, Stage: models.SilentRestoreStagePrepared,
		SourceRecovery: recovery, SourceGeneration: *generation, SourceBlock: *block,
		TaskID: owner.task.ID, WorkspaceID: owner.workspace.ID,
		WorkspaceOwnerID: owner.workspace.OwnerID, WorkspaceOrgID: owner.workspace.OrgID,
	}
	return &silentRestoreInputs{
		session: session, recovery: recovery, generation: generation, block: block,
		task: owner.task, workspace: owner.workspace, ownerCtx: owner.ctx, checkpoint: checkpoint,
	}, nil
}

type silentRestoreOwner struct {
	ctx       context.Context
	task      *models.Task
	workspace *models.Workspace
}

func (s *Service) loadSilentRestoreOwner(
	ctx context.Context,
	candidate models.SilentRestoreCandidate,
) (*silentRestoreOwner, bool, error) {
	if candidate.SessionID == "" || candidate.TaskID == "" || candidate.WorkspaceID == "" {
		return nil, false, errors.New("silent restart recovery candidate is incomplete")
	}
	workspace, ownerCtx, eligible, err := s.loadSilentRestoreWorkspace(ctx, candidate.WorkspaceID)
	if err != nil || !eligible {
		return nil, false, err
	}
	if err := s.authorizeTaskSessionPair(ownerCtx, candidate.TaskID, candidate.SessionID); err != nil {
		return nil, false, err
	}
	if err := s.authorizeSessionControl(ownerCtx, candidate.SessionID); err != nil {
		return nil, false, err
	}
	if err := s.ensureTaskNotArchived(ownerCtx, candidate.TaskID); err != nil {
		return nil, false, err
	}
	task, err := s.repo.GetTask(ownerCtx, candidate.TaskID)
	if err != nil || !silentRestoreTaskMatches(task, candidate) {
		return nil, false, err
	}
	officeTask, err := s.lookupOfficeTask(ownerCtx, task.ID)
	if err != nil || officeTask {
		return nil, false, err
	}
	return &silentRestoreOwner{ctx: ownerCtx, task: task, workspace: workspace}, true, nil
}

func (s *Service) loadSilentRestoreWorkspace(
	ctx context.Context,
	workspaceID string,
) (*models.Workspace, context.Context, bool, error) {
	reader, ok := s.repo.(silentRestoreWorkspaceReader)
	if !ok {
		return nil, nil, false, errors.New("workspace reader is unavailable")
	}
	workspace, err := reader.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil {
		return nil, nil, false, err
	}
	ownerCtx, err := s.silentRestoreOwnerContext(ctx, workspace.OwnerID, workspace.OrgID)
	if err != nil {
		return nil, nil, false, err
	}
	return workspace, ownerCtx, true, nil
}

func silentRestoreTaskMatches(task *models.Task, candidate models.SilentRestoreCandidate) bool {
	return task != nil && task.ArchivedAt == nil && task.WorkspaceID == candidate.WorkspaceID &&
		!models.IsAutomationTaskOrigin(task.Origin)
}

func (s *Service) loadSilentRestoreSession(
	ctx context.Context,
	candidate models.SilentRestoreCandidate,
	taskID string,
) (*models.TaskSession, models.AgentDeliveryRecovery, bool, error) {
	session, err := s.repo.GetTaskSession(ctx, candidate.SessionID)
	if err != nil || session == nil || session.TaskID != taskID || !silentRestoreSessionEligible(session) {
		return nil, models.AgentDeliveryRecovery{}, false, err
	}
	recovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	if !ok || !silentRestoreRecoveryPhase(recovery.Phase) || !silentRestoreRecoveryMatchesSession(recovery, session) {
		return nil, models.AgentDeliveryRecovery{}, false, nil
	}
	return session, recovery, true, nil
}

func silentRestoreRecoveryMatchesSession(recovery models.AgentDeliveryRecovery, session *models.TaskSession) bool {
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	return recovery.SessionID == session.ID && recovery.IncarnationID == incarnationID
}

func (s *Service) loadSilentRestoreContinuity(
	ctx context.Context,
	session *models.TaskSession,
	recovery models.AgentDeliveryRecovery,
) (*models.HarnessSessionGeneration, *models.SessionRecoveryBlock, bool, error) {
	continuity, ok := s.repo.(sessionContinuityStore)
	if !ok {
		return nil, nil, false, errors.New("session continuity store is unavailable")
	}
	generation, err := continuity.GetCurrentHarnessSessionGeneration(ctx, session.ID, recovery.IncarnationID)
	if err != nil || generation == nil || generation.Generation != recovery.HarnessGeneration ||
		generation.NativeSessionID == "" {
		return nil, nil, false, err
	}
	blocks, ok := s.repo.(sessionRecoveryBlockStore)
	if !ok {
		return nil, nil, false, errors.New("session recovery block store is unavailable")
	}
	block, err := blocks.GetOpenSessionRecoveryBlock(ctx, session.ID, recovery.IncarnationID, generation.Generation)
	if err != nil || !silentRestoreBlockMatches(block, recovery) {
		return nil, nil, false, err
	}
	if err := s.validateSilentRestoreSubmission(ctx, session, recovery, generation.Generation); err != nil {
		return nil, nil, false, err
	}
	return generation, block, true, nil
}

func silentRestoreSessionRouteEligible(session *models.TaskSession) bool {
	return session != nil && !session.IsPassthrough &&
		(session.RouteState == "" || session.RouteState == dynamicRouteStatusActive)
}

func silentRestoreRecoveryPhase(phase string) bool {
	return phase == models.AgentDeliveryRecoveryUncertain ||
		phase == models.AgentDeliveryRecoveryReconnecting
}

func silentRestoreBlockMatches(block *models.SessionRecoveryBlock, recovery models.AgentDeliveryRecovery) bool {
	return block != nil && block.State == models.RecoveryBlockOpen &&
		block.ConsumerReference == agentDeliveryConsumer &&
		block.IncarnationID == recovery.IncarnationID &&
		block.ExpectedGeneration == recovery.HarnessGeneration &&
		block.DeliverySubmissionID == recovery.SubmissionID &&
		block.DeliveryStreamID == recovery.StreamID
}

func (s *Service) validateSilentRestoreSubmission(
	ctx context.Context,
	session *models.TaskSession,
	recovery models.AgentDeliveryRecovery,
	generation int64,
) error {
	repo, ok := s.repo.(taskrepo.AgentDeliveryRepository)
	if !ok {
		return errors.New("agent delivery repository is unavailable")
	}
	submission, err := repo.GetAgentDeliverySubmission(ctx, recovery.SubmissionID)
	if err != nil {
		return err
	}
	if !canonicalRecoverySubmissionMatches(session, recovery, submission, generation) ||
		terminalDeliverySubmission(submission.State) {
		return errors.New("silent restart source submission is not recoverable")
	}
	return nil
}

func (s *Service) authorizeSilentRestoreOwner(ctx context.Context, taskID, sessionID string) error {
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return err
	}
	if err := s.authorizeSessionControl(ctx, sessionID); err != nil {
		return err
	}
	return s.ensureTaskNotArchived(ctx, taskID)
}

func (s *Service) restoreSilentRestartCandidate(
	ctx context.Context,
	store taskrepo.SilentRestoreRepository,
	candidate models.SilentRestoreCandidate,
) error {
	inputs, err := s.loadSilentRestoreInputs(ctx, candidate)
	if err != nil || inputs == nil {
		return err
	}
	attemptID := silentRestoreAttemptID(inputs.session.ID, inputs.recovery.IncarnationID, inputs.generation.Generation)
	attempt, err := s.ensureSilentRestoreAttempt(ctx, store, attemptID, inputs)
	if err != nil || attempt == nil || attempt.Checkpoint == nil {
		return err
	}
	checkpoint := *attempt.Checkpoint
	if !sameSilentRestorePinnedSource(checkpoint, inputs.checkpoint) {
		return nil
	}
	if checkpoint.Stage == models.SilentRestoreStageTerminal {
		return nil
	}
	if checkpoint.Stage == models.SilentRestoreStagePrepared {
		if err := s.reconcileSilentRestoreSource(inputs.ownerCtx, inputs); err != nil {
			return err
		}
		ownerCtx, err := s.revalidateSilentRestoreInputs(ctx, inputs, checkpoint)
		if err != nil {
			return err
		}
		inputs.ownerCtx = ownerCtx
	}
	return s.ensureSilentRestoreCandidate(inputs.ownerCtx, store, attemptID, checkpoint, inputs)
}

func (s *Service) ensureSilentRestoreAttempt(
	_ context.Context,
	store taskrepo.SilentRestoreRepository,
	attemptID string,
	inputs *silentRestoreInputs,
) (*models.RestoreAttempt, error) {
	ownerCtx, err := s.revalidateSilentRestoreInputs(inputs.ownerCtx, inputs, inputs.checkpoint)
	if err != nil {
		return nil, err
	}
	inputs.ownerCtx = ownerCtx
	checkpoint := inputs.checkpoint
	attempt, _, err := store.EnsureRestoreAttempt(ownerCtx, &models.RestoreAttempt{
		ID: attemptID, SessionID: inputs.session.ID, IncarnationID: inputs.recovery.IncarnationID,
		ExpectedGeneration: inputs.generation.Generation, Action: models.SilentRestoreAction,
		Outcome: models.SilentRestoreOutcomePending, TargetWorkspace: inputs.workspace.ID,
		Authorized: true, Checkpoint: &checkpoint,
	})
	return attempt, err
}

func sameSilentRestorePinnedSource(a, b models.SilentRestoreCheckpoint) bool {
	return a.Version == b.Version && sameSilentRestoreRecovery(a.SourceRecovery, b.SourceRecovery) &&
		sameSilentRestoreGeneration(a.SourceGeneration, b.SourceGeneration) &&
		sameSilentRestoreBlock(a.SourceBlock, b.SourceBlock) &&
		a.TaskID == b.TaskID && a.WorkspaceID == b.WorkspaceID &&
		a.WorkspaceOwnerID == b.WorkspaceOwnerID && a.WorkspaceOrgID == b.WorkspaceOrgID
}

func sameSilentRestoreRecovery(a, b models.AgentDeliveryRecovery) bool {
	return a.OriginalRuntime == b.OriginalRuntime && a.Phase == b.Phase && a.Revision == b.Revision &&
		a.SessionID == b.SessionID && a.AgentExecutionID == b.AgentExecutionID &&
		a.SubmissionID == b.SubmissionID && a.StreamID == b.StreamID &&
		a.IncarnationID == b.IncarnationID && a.HarnessGeneration == b.HarnessGeneration &&
		a.PromptGeneration == b.PromptGeneration && a.Message == b.Message && a.UpdatedAt.Equal(b.UpdatedAt)
}

func sameSilentRestoreGeneration(a, b models.HarnessSessionGeneration) bool {
	return a.SessionID == b.SessionID && a.IncarnationID == b.IncarnationID &&
		a.Generation == b.Generation && a.PredecessorGeneration == b.PredecessorGeneration &&
		a.NativeSessionID == b.NativeSessionID && a.AgentType == b.AgentType &&
		a.AdapterVersion == b.AdapterVersion && a.OriginalWorkspace == b.OriginalWorkspace &&
		a.CurrentWorkspace == b.CurrentWorkspace && a.NativeStateReference == b.NativeStateReference &&
		a.CreationReason == b.CreationReason && a.CreatedAt.Equal(b.CreatedAt) && a.CommittedAt.Equal(b.CommittedAt)
}

func sameSilentRestoreBlock(a, b models.SessionRecoveryBlock) bool {
	return sameSilentRestoreBlockOwner(a, b) && sameSilentRestoreBlockDelivery(a, b) &&
		a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt) &&
		sameSilentRestoreBlockResolution(a.ResolvedAt, b.ResolvedAt)
}

func sameSilentRestoreBlockOwner(a, b models.SessionRecoveryBlock) bool {
	return a.ID == b.ID && a.SessionID == b.SessionID && a.IncarnationID == b.IncarnationID &&
		a.ExpectedGeneration == b.ExpectedGeneration && a.Reason == b.Reason && a.State == b.State &&
		a.ConsumerReference == b.ConsumerReference
}

func sameSilentRestoreBlockDelivery(a, b models.SessionRecoveryBlock) bool {
	return a.DeliverySubmissionID == b.DeliverySubmissionID && a.DeliveryStreamID == b.DeliveryStreamID &&
		a.DeliverySequence == b.DeliverySequence && a.DeliveryTurnID == b.DeliveryTurnID &&
		a.DeliveryOutcome == b.DeliveryOutcome && a.AuthorizedAction == b.AuthorizedAction
}

func sameSilentRestoreBlockResolution(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func (s *Service) reconcileSilentRestoreSource(ctx context.Context, inputs *silentRestoreInputs) error {
	identity := agentruntime.AgentDeliveryRecoveryIdentity{
		OriginalRuntime: inputs.recovery.OriginalRuntime, TaskID: inputs.task.ID,
		SessionID: inputs.session.ID, ExecutionID: inputs.recovery.AgentExecutionID,
		SubmissionID: inputs.recovery.SubmissionID, StreamID: inputs.recovery.StreamID,
		IncarnationID:     inputs.recovery.IncarnationID,
		HarnessGeneration: uint64(inputs.recovery.HarnessGeneration),
		PromptGeneration:  inputs.recovery.PromptGeneration,
	}
	result := s.runSessionDeliveryRecovery(ctx, identity)
	if result.outcome != SessionDeliveryRecoveryUncertain || !result.processTerminated {
		return errors.New("interrupted durable owner is not proven terminated")
	}
	return nil
}

func (s *Service) revalidateSilentRestoreInputs(
	ctx context.Context,
	inputs *silentRestoreInputs,
	checkpoint models.SilentRestoreCheckpoint,
) (context.Context, error) {
	ownerCtx, err := s.revalidateSilentRestoreOwner(ctx, inputs.session.ID, checkpoint)
	if err != nil {
		return nil, err
	}
	if err := s.revalidateSilentRestoreContinuity(ownerCtx, inputs.session.ID, checkpoint); err != nil {
		return nil, err
	}
	return ownerCtx, nil
}

func (s *Service) revalidateSilentRestoreOwner(
	ctx context.Context,
	sessionID string,
	checkpoint models.SilentRestoreCheckpoint,
) (context.Context, error) {
	if s.silentRestoreOwnerContext == nil {
		return nil, errors.New("startup recovery owner resolver is unavailable")
	}
	workspace, err := s.readSilentRestoreCheckpointWorkspace(ctx, checkpoint)
	if err != nil {
		return nil, err
	}
	ownerCtx, err := s.silentRestoreOwnerContext(ctx, workspace.OwnerID, workspace.OrgID)
	if err != nil {
		return nil, err
	}
	if err := s.revalidateSilentRestoreTask(ownerCtx, sessionID, checkpoint); err != nil {
		return nil, err
	}
	return ownerCtx, nil
}

func (s *Service) readSilentRestoreCheckpointWorkspace(
	ctx context.Context,
	checkpoint models.SilentRestoreCheckpoint,
) (*models.Workspace, error) {
	reader, ok := s.repo.(silentRestoreWorkspaceReader)
	if !ok {
		return nil, errors.New("workspace reader is unavailable")
	}
	workspace, err := reader.GetWorkspace(ctx, checkpoint.WorkspaceID)
	if err != nil || workspace == nil || workspace.OwnerID != checkpoint.WorkspaceOwnerID ||
		workspace.OrgID != checkpoint.WorkspaceOrgID {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("workspace ownership changed during silent restart recovery")
	}
	return workspace, nil
}

func (s *Service) revalidateSilentRestoreTask(
	ownerCtx context.Context,
	sessionID string,
	checkpoint models.SilentRestoreCheckpoint,
) error {
	if err := s.authorizeSilentRestoreOwner(ownerCtx, checkpoint.TaskID, sessionID); err != nil {
		return err
	}
	task, err := s.repo.GetTask(ownerCtx, checkpoint.TaskID)
	if err != nil || task == nil || task.ArchivedAt != nil || task.WorkspaceID != checkpoint.WorkspaceID ||
		models.IsAutomationTaskOrigin(task.Origin) {
		if err != nil {
			return err
		}
		return errors.New("task is no longer eligible for silent restart recovery")
	}
	officeTask, err := s.lookupOfficeTask(ownerCtx, task.ID)
	if err != nil {
		return err
	}
	if officeTask {
		return errors.New("office tasks cannot use silent restart recovery")
	}
	return nil
}

func (s *Service) revalidateSilentRestoreContinuity(
	ownerCtx context.Context,
	sessionID string,
	checkpoint models.SilentRestoreCheckpoint,
) error {
	session, recovery, err := s.readSilentRestoreContinuitySession(ownerCtx, sessionID, checkpoint)
	if err != nil {
		return err
	}
	generation, err := s.readSilentRestoreContinuityGeneration(ownerCtx, session, recovery, checkpoint)
	if err != nil {
		return err
	}
	if err := s.revalidateSilentRestoreBlock(ownerCtx, session, recovery, generation, checkpoint); err != nil {
		return err
	}
	return s.validateSilentRestoreSubmission(ownerCtx, session, recovery, generation.Generation)
}

func (s *Service) readSilentRestoreContinuitySession(
	ownerCtx context.Context,
	sessionID string,
	checkpoint models.SilentRestoreCheckpoint,
) (*models.TaskSession, models.AgentDeliveryRecovery, error) {
	session, err := s.repo.GetTaskSession(ownerCtx, sessionID)
	if err != nil || session == nil || session.TaskID != checkpoint.TaskID || !silentRestoreSessionEligible(session) {
		if err != nil {
			return nil, models.AgentDeliveryRecovery{}, err
		}
		return nil, models.AgentDeliveryRecovery{}, errors.New("session is no longer eligible for silent restart recovery")
	}
	recovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	if !ok || !sameSilentRestoreRecovery(recovery, checkpoint.SourceRecovery) {
		return nil, models.AgentDeliveryRecovery{}, errors.New("silent restart recovery identity changed")
	}
	return session, recovery, nil
}

func (s *Service) readSilentRestoreContinuityGeneration(
	ownerCtx context.Context,
	session *models.TaskSession,
	recovery models.AgentDeliveryRecovery,
	checkpoint models.SilentRestoreCheckpoint,
) (*models.HarnessSessionGeneration, error) {
	continuity, ok := s.repo.(sessionContinuityStore)
	if !ok {
		return nil, errors.New("session continuity store is unavailable")
	}
	generation, err := continuity.GetCurrentHarnessSessionGeneration(ownerCtx, session.ID, recovery.IncarnationID)
	if err != nil || generation == nil || !sameSilentRestoreGeneration(*generation, checkpoint.SourceGeneration) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("native conversation generation changed")
	}
	return generation, nil
}

func (s *Service) revalidateSilentRestoreBlock(
	ownerCtx context.Context,
	session *models.TaskSession,
	recovery models.AgentDeliveryRecovery,
	generation *models.HarnessSessionGeneration,
	checkpoint models.SilentRestoreCheckpoint,
) error {
	blocks, ok := s.repo.(sessionRecoveryBlockStore)
	if !ok {
		return errors.New("session recovery block store is unavailable")
	}
	block, err := blocks.GetOpenSessionRecoveryBlock(ownerCtx, session.ID, recovery.IncarnationID, generation.Generation)
	if err != nil || block == nil || !sameSilentRestoreBlock(*block, checkpoint.SourceBlock) {
		if err != nil {
			return err
		}
		return errors.New("source recovery block changed")
	}
	return nil
}

func silentRestoreSessionEligible(session *models.TaskSession) bool {
	if session == nil || session.IsPassthrough ||
		(session.RouteState != "" && session.RouteState != dynamicRouteStatusActive) {
		return false
	}
	switch session.State {
	case models.TaskSessionStateStarting, models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput, models.TaskSessionStateFailed:
		return true
	default:
		return false
	}
}
