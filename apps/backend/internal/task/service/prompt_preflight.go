package service

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
)

// Prompt preflights inspect placement/session state for dispatch, not conversation
// content. They retain native prompt authority without requiring a read grant.
func (s *Service) GetTaskForPrompt(ctx context.Context, taskID string) (*models.Task, error) {
	if !s.collaborationApplies(ctx) {
		return s.GetTask(ctx, taskID)
	}
	if err := s.AuthorizeTaskPromptScope(ctx, taskID); err != nil {
		return nil, err
	}
	return s.tasks.GetTask(ctx, taskID)
}

func (s *Service) GetTaskSessionForPrompt(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	if !s.collaborationApplies(ctx) {
		return s.GetTaskSession(ctx, sessionID)
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := s.AuthorizeTaskPromptScope(ctx, session.TaskID); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Service) GetPrimarySessionForPrompt(ctx context.Context, taskID string) (*models.TaskSession, error) {
	if !s.collaborationApplies(ctx) {
		return s.GetPrimarySession(ctx, taskID)
	}
	if err := s.AuthorizeTaskPromptScope(ctx, taskID); err != nil {
		return nil, err
	}
	return s.sessions.GetPrimarySessionByTaskID(ctx, taskID)
}

func (s *Service) ListTaskSessionsForPrompt(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	if !s.collaborationApplies(ctx) {
		return s.ListTaskSessions(ctx, taskID)
	}
	if err := s.AuthorizeTaskPromptScope(ctx, taskID); err != nil {
		return nil, err
	}
	return s.sessions.ListTaskSessions(ctx, taskID)
}
