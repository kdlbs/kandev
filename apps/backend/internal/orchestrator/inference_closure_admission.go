package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

type inferenceClosureCheckpointReader interface {
	ReadInferenceClosure(context.Context, string) (json.RawMessage, error)
}

type inferenceClosureBlockReader interface {
	GetInferenceClosureBlock(context.Context, string) (*models.SessionRecoveryBlock, error)
}

type inferenceClosureStore interface {
	inferenceClosureCheckpointReader
	inferenceClosureBlockReader
	BeginInferenceClosure(context.Context, string, string, string, string) (json.RawMessage, error)
	AdvanceInferenceClosure(context.Context, string, string, json.RawMessage, json.RawMessage) (bool, error)
}

type inferenceClosurePermitKey struct{}

type legacyRecoveryBlockReader interface {
	GetOpenSessionRecoveryBlock(context.Context, string, string, int64) (*models.SessionRecoveryBlock, error)
}

// A genuinely legacy repository without recovery storage retains ordinary
// behavior. An adapter over the native store forwards the context request,
// allowing that store to report independent closure blocks across generations.
func (s *Service) legacyWithoutClosureState(ctx context.Context, sessionID, sourceInstanceID string) error {
	if permit, _ := ctx.Value(inferenceClosurePermitKey{}).(*models.InferenceClosureCheckpoint); permit != nil {
		return models.ErrInferenceClosureBlocked
	}
	if _, ok := s.repo.(inferenceClosureBlockReader); ok {
		return models.ErrInferenceClosureBlocked
	}
	if s.repo == nil {
		// Without a task repository there is no persisted closure state to
		// consult; services built without one keep the existing admission.
		return nil
	}
	_, hasCheckpoint := s.repo.(inferenceClosureCheckpointReader)
	_, hasBlockReader := s.repo.(legacyRecoveryBlockReader)
	_, hasBlockList := s.repo.(taskrepo.SessionRecoveryBlockListRepository)
	_, hasBlockLookup := s.repo.(sessionRecoveryBlockLookup)
	needsConfirmation := hasCheckpoint || hasBlockReader || hasBlockList || hasBlockLookup
	session, err := s.repo.GetTaskSession(models.WithInferenceClosurePresenceCheck(ctx), sessionID)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, models.ErrTaskSessionNotFound) {
		if needsConfirmation {
			return models.ErrInferenceClosureBlocked
		}
		return nil
	}
	if err != nil {
		return err
	}
	if needsConfirmation && (session == nil || !session.InferenceClosureAbsenceConfirmed) {
		return models.ErrInferenceClosureBlocked
	}
	return checkLegacyClosureMetadata(session, sessionID, sourceInstanceID)
}

func checkLegacyClosureMetadata(session *models.TaskSession, sessionID, sourceInstanceID string) error {
	if session == nil || session.Metadata == nil {
		return nil
	}
	value, present := session.Metadata[models.InferenceClosureMetadataKey]
	if !present {
		return nil
	}
	if !session.InferenceClosureAbsenceConfirmed {
		return models.ErrInferenceClosureBlocked
	}
	raw, err := json.Marshal(value)
	var checkpoint models.InferenceClosureCheckpoint
	if err != nil || json.Unmarshal(raw, &checkpoint) != nil || !checkpoint.ReadyForSuccessor() ||
		checkpoint.SessionID != sessionID || checkpoint.Stage != models.InferenceClosureConsumed ||
		(sourceInstanceID != "" && sourceInstanceID == checkpoint.SourceExecutionID) {
		return models.ErrInferenceClosureBlocked
	}
	return nil
}

// Only the internally validated committed checkpoint permits its exact successor.
func (s *Service) checkInferenceClosureAdmission(ctx context.Context, sessionID string, allowAdoption bool) error {
	reader, ok := s.repo.(inferenceClosureBlockReader)
	if sessionID == "" {
		return nil
	}
	if !ok {
		return s.legacyWithoutClosureState(ctx, sessionID, "")
	}
	block, err := reader.GetInferenceClosureBlock(ctx, sessionID)
	if err != nil {
		return err
	}
	permit, _ := ctx.Value(inferenceClosurePermitKey{}).(*models.InferenceClosureCheckpoint)
	if block == nil {
		if allowAdoption && permit != nil {
			return models.ErrInferenceClosureBlocked
		}
		return nil
	}
	if !allowAdoption || permit == nil || permit.SessionID != sessionID || permit.OperationID != block.ID {
		return models.ErrInferenceClosureBlocked
	}
	checkpoints, ok := s.repo.(inferenceClosureCheckpointReader)
	if !ok {
		return models.ErrInferenceClosureBlocked
	}
	raw, err := checkpoints.ReadInferenceClosure(ctx, sessionID)
	if err != nil {
		return err
	}
	var saved models.InferenceClosureCheckpoint
	if json.Unmarshal(raw, &saved) != nil || !matchesInferenceClosurePermit(&saved, permit) {
		return models.ErrInferenceClosureBlocked
	}
	return nil
}

func matchesInferenceClosurePermit(saved, permit *models.InferenceClosureCheckpoint) bool {
	return saved.ReadyForSuccessor() && saved.Stage == models.InferenceClosureCommitted && saved.OperationID == permit.OperationID &&
		saved.SessionID == permit.SessionID && saved.SourceExecutionID == permit.SourceExecutionID &&
		saved.CandidateExecutionID == permit.CandidateExecutionID && saved.IncarnationID == permit.IncarnationID
}

func (s *Service) inferenceClosureResumeOptions(ctx context.Context, sessionID string, options executor.ResumeOptions) executor.ResumeOptions {
	permit, _ := ctx.Value(inferenceClosurePermitKey{}).(*models.InferenceClosureCheckpoint)
	if permit == nil || permit.SessionID != sessionID {
		return options
	}
	options.CandidateExecutionID = permit.CandidateExecutionID
	previousAllocation := options.OnExecutionAllocated
	options.OnExecutionAllocated = func(allocationCtx context.Context, executionID string) error {
		if executionID != permit.CandidateExecutionID {
			return models.ErrInferenceClosureConflict
		}
		verifiedCtx := context.WithValue(allocationCtx, inferenceClosurePermitKey{}, permit)
		if err := s.checkInferenceClosureAdmission(verifiedCtx, sessionID, true); err != nil {
			return err
		}
		if previousAllocation != nil {
			return previousAllocation(allocationCtx, executionID)
		}
		return nil
	}
	options.NoInitialPrompt = true
	options.SuppressInitialMessageBackfill = true
	options.StartAgentSynchronously = true
	return options
}

func (s *Service) checkClosedInferenceSource(ctx context.Context, binding sessioncapacity.InferenceBinding) error {
	if binding.SessionID == "" {
		return nil
	}
	store, ok := s.repo.(inferenceClosureCheckpointReader)
	if !ok {
		return s.legacyWithoutClosureState(ctx, binding.SessionID, binding.InstanceID)
	}
	raw, err := store.ReadInferenceClosure(ctx, binding.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved models.InferenceClosureCheckpoint
	if json.Unmarshal(raw, &saved) != nil || !saved.Valid() {
		return models.ErrInferenceClosureBlocked
	}
	if saved.SourceExecutionID == binding.InstanceID {
		return models.ErrInferenceClosureBlocked
	}
	return nil
}

func (s *Service) checkInferenceClosureBinding(ctx context.Context, binding sessioncapacity.InferenceBinding) error {
	if err := s.checkInferenceClosureAdmission(ctx, binding.SessionID, false); err != nil {
		return err
	}
	return s.checkClosedInferenceSource(ctx, binding)
}

// A closure's recovery block is released only after the exact successor has
// consumed its receipt. Ordinary resume retains its existing resolution path.
func (s *Service) completeResumeRecoveryBlock(ctx context.Context, sessionID string) error {
	permit, _ := ctx.Value(inferenceClosurePermitKey{}).(*models.InferenceClosureCheckpoint)
	if permit == nil {
		return s.resolveSessionRecoveryBlock(ctx, sessionID, recoveryActionResume)
	}
	if permit.SessionID != sessionID {
		return models.ErrInferenceClosureBlocked
	}
	return s.checkSessionRecoveryBlock(ctx, sessionID)
}

// The closure permit bypasses only its own already-validated block. Other
// recovery causes remain authoritative and must not be hidden by row ordering.
func (s *Service) checkOtherRecoveryBlocksForClosure(ctx context.Context, sid, incarnation string, generation int64, permit *models.InferenceClosureCheckpoint) error {
	lister, ok := s.repo.(taskrepo.SessionRecoveryBlockListRepository)
	if !ok {
		return models.ErrInferenceClosureBlocked
	}
	blocks, err := lister.ListOpenSessionRecoveryBlocks(ctx, sid, incarnation, generation)
	if err != nil {
		return err
	}
	found := false
	for _, block := range blocks {
		if block == nil {
			return models.ErrInferenceClosureBlocked
		}
		if block.ID == permit.OperationID && block.ConsumerReference == models.InferenceClosureConsumer {
			found = true
			continue
		}
		if resolved, _ := s.finishLoadedNativeDeliveryBlock(ctx, block); !resolved {
			return &sessionRecoveryRequiredError{Block: block}
		}
	}
	if !found {
		return models.ErrInferenceClosureBlocked
	}
	return nil
}
