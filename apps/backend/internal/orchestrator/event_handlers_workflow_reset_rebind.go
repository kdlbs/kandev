package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

var errResetNativeRebindRefused = errors.New("context reset native rebind was refused")

type harnessNativeRebindStore interface {
	GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error)
	RebindHarnessGenerationNativeSession(context.Context, models.HarnessNativeRebind) (bool, error)
}

// replaceResetResumeToken moves the session to the native conversation that a
// successful provider reset created. With a committed harness generation, the
// token and the generation move together or not at all. Without one, the
// token is cleared and the current ACP session ID is stored, which tolerates
// a fresh session event that raced the clear.
func (s *Service) replaceResetResumeToken(
	ctx context.Context, taskID string, session *models.TaskSession, executionID, previousACPSessionID string,
) error {
	rebound, err := s.rebindResetNativeSession(ctx, taskID, session, executionID, previousACPSessionID)
	if err != nil || rebound {
		return err
	}
	if err := s.clearResumeToken(ctx, session.ID); err != nil {
		return fmt.Errorf("clear resume token after context reset: %w", err)
	}
	if acpSessionID := s.currentACPSessionID(session.ID); acpSessionID != "" {
		if !s.storeResumeToken(ctx, taskID, session.ID, executionID, acpSessionID, "") {
			return fmt.Errorf("store resume token after context reset")
		}
	}
	return nil
}

// rebindResetNativeSession reports whether it owned the token replacement.
// It declines sessions without a committed generation or without a changed
// native conversation, and returns an error when the guarded rebind refuses.
func (s *Service) rebindResetNativeSession(
	ctx context.Context, taskID string, session *models.TaskSession, executionID, previousACPSessionID string,
) (bool, error) {
	store, ok := s.repo.(harnessNativeRebindStore)
	if !ok {
		return false, nil
	}
	if executionID == "" {
		return false, nil
	}
	currentACPSessionID := s.currentACPSessionID(session.ID)
	if currentACPSessionID == "" {
		return false, nil
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	current, err := s.currentResetGeneration(ctx, store, session.ID, incarnationID)
	if err != nil {
		return false, err
	}
	if current == nil {
		return false, nil
	}
	previousNativeSessionID, err := s.previousResetNativeSessionID(
		ctx, session.ID, current, previousACPSessionID, currentACPSessionID,
	)
	if err != nil {
		return false, err
	}
	if previousNativeSessionID == "" || previousNativeSessionID == currentACPSessionID {
		return false, nil
	}
	rebound, err := store.RebindHarnessGenerationNativeSession(ctx, models.HarnessNativeRebind{
		SessionID: session.ID, IncarnationID: incarnationID, Generation: current.Generation, TaskID: taskID,
		ExecutionID: executionID, PreviousNativeSessionID: previousNativeSessionID, NativeSessionID: currentACPSessionID,
	})
	if err != nil {
		return false, fmt.Errorf("rebind harness generation after context reset: %w", err)
	}
	if !rebound {
		return false, errResetNativeRebindRefused
	}
	s.persistACPSessionID(ctx, session.ID, currentACPSessionID)
	return true, nil
}

func (s *Service) currentResetGeneration(
	ctx context.Context,
	store harnessNativeRebindStore,
	sessionID, incarnationID string,
) (*models.HarnessSessionGeneration, error) {
	current, err := store.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
	if errors.Is(err, models.ErrTaskSessionNotFound) || (err == nil && current == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read harness generation for context reset: %w", err)
	}
	if current.Generation <= 0 || current.NativeSessionID == "" {
		return nil, fmt.Errorf("current harness generation has incomplete native identity")
	}
	return current, nil
}

func (s *Service) previousResetNativeSessionID(
	ctx context.Context,
	sessionID string,
	generation *models.HarnessSessionGeneration,
	previousACPSessionID, currentACPSessionID string,
) (string, error) {
	if generation.NativeSessionID != currentACPSessionID {
		// The committed generation owns the durable prior identity. It can differ
		// from the live ID after an earlier reset failed to commit.
		return generation.NativeSessionID, nil
	}
	if previousACPSessionID != "" && previousACPSessionID != currentACPSessionID {
		return previousACPSessionID, nil
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("read executor identity for context reset: %w", err)
	}
	if running == nil {
		return "", nil
	}
	return running.ResumeToken, nil
}
