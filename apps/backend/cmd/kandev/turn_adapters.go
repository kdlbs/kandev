package main

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// turnServiceAdapter adapts the task service to the orchestrator.TurnService interface.
type turnServiceAdapter struct {
	svc *taskservice.Service
}

func (a *turnServiceAdapter) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return a.svc.StartTurn(ctx, sessionID)
}

func (a *turnServiceAdapter) CompleteTurn(ctx context.Context, turnID string) error {
	return a.svc.CompleteTurn(ctx, turnID)
}

func (a *turnServiceAdapter) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return a.svc.GetActiveTurn(ctx, sessionID)
}

func (a *turnServiceAdapter) AbandonOpenTurns(ctx context.Context, sessionID string) error {
	return a.svc.AbandonOpenTurns(ctx, sessionID)
}

func newTurnServiceAdapter(svc *taskservice.Service) *turnServiceAdapter {
	return &turnServiceAdapter{svc: svc}
}

// taskSessionCheckerAdapter adapts the task repository for github.TaskSessionChecker.
type taskSessionCheckerAdapter struct {
	repo interface {
		ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
		ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error)
	}
}

// HasUserAuthoredMessage reports whether the user has authored any message
// on this task that wasn't created by an automated trigger (workflow
// auto-start, PR/issue watch, Jira/Linear integration). Auto-start messages
// are tagged with metadata.auto_start = true; the check ignores them so a
// task whose only "user" message is the agent's auto-injected prompt counts
// as untouched and is eligible for cleanup when its PR/issue merges.
func (a *taskSessionCheckerAdapter) HasUserAuthoredMessage(ctx context.Context, taskID string) (bool, error) {
	sessions, err := a.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, sess := range sessions {
		messages, err := a.repo.ListMessages(ctx, sess.ID)
		if err != nil {
			return false, err
		}
		for _, m := range messages {
			if m.AuthorType != models.MessageAuthorUser {
				continue
			}
			// New code paths tag both auto_start and workflow_auto_start.
			// Legacy rows (pre-cleanup-policy upgrade) carry only the
			// workflow_auto_start tag from the old recordAutoStartMessage
			// implementation — recognize it too so the install-wide
			// cleanup button actually drains piled-up tasks after upgrade.
			if metaFlag(m.Metadata, "auto_start") || metaFlag(m.Metadata, "workflow_auto_start") {
				continue
			}
			return true, nil
		}
	}
	return false, nil
}

// metaFlag returns true when meta[key] is a bool with value true. Returns
// false for missing keys, nil maps, non-bool values, and false values.
func metaFlag(meta map[string]interface{}, key string) bool {
	v, ok := meta[key].(bool)
	return ok && v
}
