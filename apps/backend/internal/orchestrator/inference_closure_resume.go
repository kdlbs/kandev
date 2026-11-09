package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) recoverInferenceClosure(ctx context.Context, taskID, sid, action string, options RecoverSessionOptions) (*LaunchSessionResponse, error) {
	if err := s.authorizeInferenceClosureResume(ctx, taskID, sid, action); err != nil {
		return nil, err
	}
	store, ok := s.repo.(inferenceClosureStore)
	if !ok {
		return nil, models.ErrInferenceClosureBlocked
	}
	runtime, ok := s.agentManager.(inferenceClosureRuntime)
	if !ok {
		return nil, models.ErrInferenceClosureBlocked
	}
	checkpoint, err := readInferenceClosureResume(ctx, store, sid, options.InferenceClosureOperationID)
	if err != nil {
		return nil, err
	}
	if err = runtime.VerifyInferenceClosureClaims(ctx, sid, checkpoint.SourceExecutionID); err != nil {
		return nil, err
	}
	current, lookupErr := s.agentManager.GetExecutionIDForSession(ctx, sid)
	if lookupErr != nil && !errors.Is(lookupErr, agentruntime.ErrNoExecutionForSession) {
		return nil, lookupErr
	}
	if current == checkpoint.CandidateExecutionID {
		return s.finishInferenceClosureAdoption(ctx, store, runtime, taskID, checkpoint)
	}
	if checkpoint.Stage == models.InferenceClosureConsumed || current != "" {
		return nil, models.ErrInferenceClosureBlocked
	}
	launchCtx := context.WithValue(ctx, inferenceClosurePermitKey{}, checkpoint)
	launchCtx = agentruntime.WithInferenceClosureAdoption(launchCtx, checkpoint.OperationID)
	options.InferenceClosureOperationID = ""
	response, err := s.RecoverSessionWithOptions(launchCtx, taskID, sid, action, options)
	if err != nil {
		return nil, err
	}
	if response == nil || response.AgentExecutionID != checkpoint.CandidateExecutionID {
		return nil, models.ErrInferenceClosureConflict
	}
	return s.finishInferenceClosureAdoption(ctx, store, runtime, taskID, checkpoint)
}

func (s *Service) finishInferenceClosureAdoption(ctx context.Context, store inferenceClosureStore, runtime inferenceClosureRuntime, taskID string, c *models.InferenceClosureCheckpoint) (*LaunchSessionResponse, error) {
	observation, err := runtime.InferenceClosureStep(ctx, c.SessionID, c.CandidateExecutionID, utility.InferenceClosureRequest{OperationID: c.OperationID, Action: "status"})
	if err != nil {
		return nil, err
	}
	if !closureObservationMatches(c, observation, models.InferenceClosureConsumed) || observation.Snapshot.OwnerID != observation.Snapshot.Closure.SuccessorOwnerID {
		return nil, models.ErrInferenceClosureConflict
	}
	if err = runtime.VerifyInferenceClosureClaims(ctx, c.SessionID, c.SourceExecutionID); err != nil {
		return nil, err
	}
	if c.Stage != models.InferenceClosureConsumed {
		next := *c
		next.Stage = models.InferenceClosureConsumed
		next.SuccessorOwnerID = observation.Snapshot.OwnerID
		if err = advanceInferenceClosure(ctx, store, c, next); err != nil {
			return nil, err
		}
	}
	return &LaunchSessionResponse{Success: true, TaskID: taskID, SessionID: c.SessionID, AgentExecutionID: c.CandidateExecutionID}, nil
}

func (s *Service) authorizeInferenceClosureResume(ctx context.Context, taskID, sid, action string) error {
	if action != recoveryActionResume {
		return models.ErrInferenceClosureConflict
	}
	if err := s.authorizeTaskSessionPair(ctx, taskID, sid); err != nil {
		return err
	}
	if err := s.authorizeTaskPrompt(ctx, taskID); err != nil {
		return err
	}
	if err := s.authorizeSessionControl(ctx, sid); err != nil {
		return err
	}
	office, err := s.lookupOfficeTask(ctx, taskID)
	if err != nil {
		return err
	}
	if office {
		return models.ErrInferenceClosureBlocked
	}
	return nil
}

func readInferenceClosureResume(ctx context.Context, store inferenceClosureStore, sid, operation string) (*models.InferenceClosureCheckpoint, error) {
	raw, err := store.ReadInferenceClosure(ctx, sid)
	if err != nil {
		return nil, err
	}
	var checkpoint models.InferenceClosureCheckpoint
	if json.Unmarshal(raw, &checkpoint) != nil || !checkpoint.ReadyForSuccessor() || checkpoint.SessionID != sid || checkpoint.OperationID != operation {
		return nil, models.ErrInferenceClosureBlocked
	}
	return &checkpoint, nil
}
