package questions

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// WaitStatus reports the outcome of an explicit dependency wait.
type WaitStatus string

const (
	// WaitStatusReady means at least one named dependency already resolved, so
	// the caller must not pause.
	WaitStatusReady WaitStatus = "ready"
	// WaitStatusWaiting means every named dependency is still pending and the
	// gate was persisted for the caller's guarded pause.
	WaitStatusWaiting WaitStatus = "waiting"
)

// WaitInput is one explicit dependency declaration.
type WaitInput struct {
	TaskID       string
	WorkspaceID  string
	SessionID    string
	TurnID       string
	Dependencies []string
	Reason       string
}

// WaitResult reports ready snapshots or the persisted gate.
type WaitResult struct {
	Status    WaitStatus
	Questions []*models.AsyncQuestion
	Wait      *models.AsyncQuestionWait
}

// Wait registers an explicit dependency gate. Only pending requests owned by
// the current session are valid dependencies; any terminal outcome returns
// ready without pausing.
func (s *Service) Wait(ctx context.Context, input WaitInput) (*WaitResult, error) {
	unique, err := validateWaitInput(input)
	if err != nil {
		return nil, err
	}
	page, ready, err := s.loadWaitDependencies(ctx, input.SessionID, unique)
	if err != nil || ready != nil {
		return ready, err
	}
	wait, err := s.createWaitGate(ctx, input, unique)
	if err != nil {
		return nil, err
	}
	raced, err := s.hasTerminalDependency(ctx, wait)
	if err != nil {
		return nil, err
	}
	if raced {
		return s.releaseRacedWait(ctx, input.SessionID, unique, wait.ID)
	}
	s.notifyWait(ctx, wait)
	return &WaitResult{Status: WaitStatusWaiting, Wait: wait, Questions: page.Questions}, nil
}

func validateWaitInput(input WaitInput) ([]string, error) {
	if input.SessionID == "" || len(input.Dependencies) == 0 {
		return nil, fmt.Errorf("%w: session and at least one question id are required", ErrValidation)
	}
	if len(input.Dependencies) > maxQuestionIDs {
		return nil, fmt.Errorf("%w: at most %d question ids", ErrValidation, maxQuestionIDs)
	}
	seen := make(map[string]bool, len(input.Dependencies))
	unique := make([]string, 0, len(input.Dependencies))
	for _, id := range input.Dependencies {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	return unique, nil
}

func (s *Service) loadWaitDependencies(
	ctx context.Context, sessionID string, unique []string,
) (*models.AsyncQuestionPage, *WaitResult, error) {
	page, err := s.store.ListAsyncQuestions(ctx, models.ListAsyncQuestionsOptions{
		AuthorSessionID: sessionID,
		QuestionIDs:     unique,
		Limit:           maxPageLimit,
	})
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]*models.AsyncQuestion, len(page.Questions))
	for _, question := range page.Questions {
		byID[question.ID] = question
	}
	if len(byID) != len(unique) {
		return nil, nil, fmt.Errorf("%w: a dependency does not belong to this session", ErrUnauthorized)
	}
	resolved := make([]*models.AsyncQuestion, 0, len(unique))
	for _, id := range unique {
		if byID[id].Status.Terminal() {
			resolved = append(resolved, byID[id])
		}
	}
	if len(resolved) > 0 {
		return page, &WaitResult{Status: WaitStatusReady, Questions: resolved}, nil
	}
	return page, nil, nil
}

func (s *Service) createWaitGate(ctx context.Context, input WaitInput, unique []string) (*models.AsyncQuestionWait, error) {
	wait, err := s.store.CreateAsyncQuestionWait(ctx, &models.AsyncQuestionWait{
		TaskID:        input.TaskID,
		WorkspaceID:   input.WorkspaceID,
		SessionID:     input.SessionID,
		TurnID:        input.TurnID,
		DependencyIDs: unique,
		Status:        models.AsyncQuestionWaitRequested,
		Reason:        input.Reason,
	})
	if err != nil {
		if err.Error() == ErrWaitActive.Error() {
			return nil, ErrWaitActive
		}
		return nil, err
	}
	return wait, nil
}

// releaseRacedWait closes a fresh gate whose dependency resolved before the
// pause started, so the caller resumes instead of stranding the turn.
func (s *Service) releaseRacedWait(ctx context.Context, sessionID string, unique []string, waitID string) (*WaitResult, error) {
	won, err := s.store.SetAsyncQuestionWaitStatusFrom(ctx, waitID,
		models.AsyncQuestionWaitRequested, models.AsyncQuestionWaitReleased, "answer_raced_wait")
	if err != nil {
		return nil, err
	}
	if !won {
		if _, err := s.store.SetAsyncQuestionWaitStatusFrom(ctx, waitID,
			models.AsyncQuestionWaitReleaseRequested, models.AsyncQuestionWaitReleased, "answer_raced_wait"); err != nil {
			return nil, err
		}
	}
	snapshots, err := s.store.ListAsyncQuestions(ctx, models.ListAsyncQuestionsOptions{
		AuthorSessionID: sessionID, QuestionIDs: unique, Limit: maxPageLimit,
	})
	if err != nil {
		return nil, err
	}
	return &WaitResult{Status: WaitStatusReady, Questions: snapshots.Questions}, nil
}

// hasTerminalDependency reports whether any named dependency already reached a
// terminal outcome.
func (s *Service) hasTerminalDependency(ctx context.Context, wait *models.AsyncQuestionWait) (bool, error) {
	if wait == nil || len(wait.DependencyIDs) == 0 {
		return false, nil
	}
	page, err := s.store.ListAsyncQuestions(ctx, models.ListAsyncQuestionsOptions{
		AuthorSessionID: wait.SessionID,
		QuestionIDs:     wait.DependencyIDs,
		Limit:           maxPageLimit,
	})
	if err != nil {
		return false, err
	}
	for _, question := range page.Questions {
		if question.Status.Terminal() {
			return true, nil
		}
	}
	return false, nil
}

// MarkWaitParked records a completed guarded pause and acknowledges a fenced
// release. It is the handshake boundary between the pause and answer
// admission: a request that resolved while the pause was settling becomes
// released here instead of parked.
func (s *Service) MarkWaitParked(ctx context.Context, waitID string) error {
	_, err := s.AcknowledgeWaitPause(ctx, waitID)
	return err
}

// AcknowledgeWaitPause settles one requested gate after its guarded pause.
func (s *Service) AcknowledgeWaitPause(ctx context.Context, waitID string) (models.AsyncQuestionWaitStatus, error) {
	if waitID == "" {
		return "", nil
	}
	wait, err := s.store.GetAsyncQuestionWait(ctx, waitID)
	if err != nil || wait == nil {
		return "", err
	}
	switch wait.Status {
	case models.AsyncQuestionWaitReleaseRequested:
		return s.acknowledgeFencedRelease(ctx, wait)
	case models.AsyncQuestionWaitRequested:
		return s.acknowledgeRequestedPause(ctx, wait)
	default:
		return wait.Status, nil
	}
}

func (s *Service) acknowledgeFencedRelease(ctx context.Context, wait *models.AsyncQuestionWait) (models.AsyncQuestionWaitStatus, error) {
	won, err := s.store.SetAsyncQuestionWaitStatusFrom(ctx, wait.ID,
		models.AsyncQuestionWaitReleaseRequested, models.AsyncQuestionWaitReleased, "pause_acknowledged")
	if err != nil {
		return "", err
	}
	if won {
		s.wakeWaitDestination(ctx, wait)
	}
	return s.canonicalWaitStatus(ctx, wait.ID)
}

func (s *Service) acknowledgeRequestedPause(ctx context.Context, wait *models.AsyncQuestionWait) (models.AsyncQuestionWaitStatus, error) {
	raced, err := s.hasTerminalDependency(ctx, wait)
	if err != nil {
		return "", err
	}
	if raced {
		if _, err := s.acknowledgeFencedRelease(ctx, wait); err != nil {
			return "", err
		}
		return s.canonicalWaitStatus(ctx, wait.ID)
	}
	won, err := s.store.SetAsyncQuestionWaitStatusFrom(ctx, wait.ID,
		models.AsyncQuestionWaitRequested, models.AsyncQuestionWaitParked, "pause")
	if err != nil {
		return "", err
	}
	if won {
		return models.AsyncQuestionWaitParked, nil
	}
	latest, err := s.store.GetAsyncQuestionWait(ctx, wait.ID)
	if err != nil || latest == nil {
		return "", err
	}
	if latest.Status == models.AsyncQuestionWaitReleaseRequested {
		return s.acknowledgeFencedRelease(ctx, latest)
	}
	return latest.Status, nil
}

func (s *Service) canonicalWaitStatus(ctx context.Context, waitID string) (models.AsyncQuestionWaitStatus, error) {
	wait, err := s.store.GetAsyncQuestionWait(ctx, waitID)
	if err != nil || wait == nil {
		return "", err
	}
	return wait.Status, nil
}

func (s *Service) wakeWaitDestination(ctx context.Context, wait *models.AsyncQuestionWait) {
	if s.wakeDestination == nil || wait == nil {
		return
	}
	s.wakeDestination(ctx, wait.TaskID, wait.SessionID)
}

// MarkWaitCancelled cancels a gate whose pause could not be established.
func (s *Service) MarkWaitCancelled(ctx context.Context, waitID, reason string) error {
	if waitID == "" {
		return nil
	}
	return s.store.SetAsyncQuestionWaitStatus(ctx, waitID, models.AsyncQuestionWaitCancelled, reason)
}

func (s *Service) notifyWait(ctx context.Context, wait *models.AsyncQuestionWait) {
	if s.notifier == nil || wait == nil {
		return
	}
	questionID := ""
	change := QuestionChange{
		WorkspaceID: wait.WorkspaceID,
		TaskID:      wait.TaskID,
		SessionID:   wait.SessionID,
		Kind:        "wait_changed",
	}
	if len(wait.DependencyIDs) > 0 {
		questionID = wait.DependencyIDs[0]
		if question, err := s.Get(ctx, questionID); err == nil && question != nil {
			change.TaskID = question.TaskID
			change.ParentTaskID = question.ParentTaskID
			change.SessionID = question.SessionID
			change.Recipient = question.Recipient
		}
	}
	change.QuestionID = questionID
	s.notifier.AsyncQuestionChanged(ctx, change)
}

// ReconcileWaitGates closes gates that a restart cannot safely resume. It runs
// before the delivery worker and before startup prompt admission.
func (s *Service) ReconcileWaitGates(ctx context.Context) error {
	waits, err := s.store.ListAsyncQuestionWaits(ctx, []models.AsyncQuestionWaitStatus{
		models.AsyncQuestionWaitRequested,
		models.AsyncQuestionWaitReleaseRequested,
		models.AsyncQuestionWaitParked,
	}, 500)
	if err != nil {
		return err
	}
	for _, wait := range waits {
		terminal, stateErr := s.waitDestinationTerminal(ctx, wait.SessionID)
		if stateErr != nil {
			s.logger.Warn("failed to reconcile async question wait; leaving it active",
				zap.String("wait_id", wait.ID), zap.Error(stateErr))
			continue
		}
		if terminal {
			if err := s.store.SetAsyncQuestionWaitStatus(ctx, wait.ID, models.AsyncQuestionWaitCancelled, "destination_terminal"); err != nil {
				return err
			}
			continue
		}
		switch wait.Status {
		case models.AsyncQuestionWaitReleaseRequested:
			if err := s.store.SetAsyncQuestionWaitStatus(ctx, wait.ID, models.AsyncQuestionWaitReleased, "restart_release"); err != nil {
				return err
			}
			s.wakeWaitDestination(ctx, wait)
		case models.AsyncQuestionWaitRequested:
			// The pause that registered this gate did not survive the restart.
			// Close it so completion is not stranded; the agent can declare a
			// new wait after it resumes.
			if err := s.store.SetAsyncQuestionWaitStatus(ctx, wait.ID, models.AsyncQuestionWaitCancelled, "restart_without_pause"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) waitDestinationTerminal(ctx context.Context, sessionID string) (bool, error) {
	if s.sessions == nil {
		return false, nil
	}
	state, archived, found, err := s.sessions.AsyncQuestionSessionState(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if !found || archived {
		return true, nil
	}
	return terminalSessionState(state), nil
}
