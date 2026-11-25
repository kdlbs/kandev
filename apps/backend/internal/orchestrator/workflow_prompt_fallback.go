package orchestrator

import (
	"context"
	"sync"
)

type workflowPromptFallbackKey struct{}

type workflowPromptFallback struct {
	mu            sync.Mutex
	taskID        string
	stepID        string
	sessionID     string
	incarnationID string
	text          string
	claimed       bool
}

// withWorkflowPromptFallback retains a successful first-prompt claim across
// pre-admission retries of one workflow action.
func withWorkflowPromptFallback(ctx context.Context, taskID, stepID string) context.Context {
	return context.WithValue(ctx, workflowPromptFallbackKey{}, &workflowPromptFallback{taskID: taskID, stepID: stepID})
}

func (s *Service) claimWorkflowPromptFallback(
	ctx context.Context, taskID, stepID, sessionID, incarnationID, description string,
) (string, error) {
	fallback, _ := ctx.Value(workflowPromptFallbackKey{}).(*workflowPromptFallback)
	if fallback == nil || fallback.taskID != taskID || fallback.stepID != stepID {
		claimed, err := s.repo.ClaimInitialPromptFallback(ctx, sessionID, incarnationID)
		if !claimed {
			return "", err
		}
		return description, err
	}
	fallback.mu.Lock()
	defer fallback.mu.Unlock()
	if fallback.claimed && fallback.sessionID == sessionID && fallback.incarnationID == incarnationID {
		return fallback.text, nil
	}
	claimed, err := s.repo.ClaimInitialPromptFallback(ctx, sessionID, incarnationID)
	if !claimed || err != nil {
		return "", err
	}
	fallback.sessionID, fallback.incarnationID = sessionID, incarnationID
	fallback.text, fallback.claimed = description, true
	return description, nil
}
