package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/scheduler/cron"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	maxSessionWakeMarker = 128
	maxSessionWakePrompt = 16384
)

type UpsertSessionWakeRequest struct {
	Marker         string
	Prompt         string
	CronExpression string
	Timezone       string
	ExpiresAt      time.Time
}

// UpsertSessionWake validates the authenticated task/session pair before
// persisting one stable schedule for its marker.
func (s *Service) UpsertSessionWake(ctx context.Context, taskID, sessionID string, req UpsertSessionWakeRequest) (*models.TaskSessionWake, string, error) {
	if s.sessionWakes == nil {
		return nil, "", fmt.Errorf("session wakes are not configured")
	}
	if err := validateSessionWakeRequest(req); err != nil {
		return nil, "", err
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return nil, "", fmt.Errorf("session does not belong to task")
	}
	now := time.Now().UTC()
	next, err := cron.NextScheduleTime(req.CronExpression, req.Timezone, now)
	if err != nil {
		return nil, "", err
	}
	wake := &models.TaskSessionWake{TaskID: taskID, SessionID: sessionID, Marker: strings.TrimSpace(req.Marker), Prompt: req.Prompt, CronExpression: strings.TrimSpace(req.CronExpression), Timezone: normalizedTimezone(req.Timezone), NextRunAt: next.UTC(), ExpiresAt: req.ExpiresAt.UTC(), LastDeliveryStatus: models.SessionWakeDeliveryPending}
	existing, _ := s.sessionWakes.ListTaskSessionWakes(ctx, taskID, sessionID)
	action := "created"
	for _, item := range existing {
		if item.Marker == wake.Marker {
			if item.Expired(now) {
				action = "reactivated"
			} else {
				action = "updated"
			}
			break
		}
	}
	if _, err := s.sessionWakes.UpsertTaskSessionWake(ctx, wake); err != nil {
		return nil, "", err
	}
	return wake, action, nil
}

func (s *Service) ListSessionWakes(ctx context.Context, taskID, sessionID string) ([]*models.TaskSessionWake, error) {
	if s.sessionWakes == nil {
		return nil, fmt.Errorf("session wakes are not configured")
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return nil, fmt.Errorf("session does not belong to task")
	}
	return s.sessionWakes.ListTaskSessionWakes(ctx, taskID, sessionID)
}

func (s *Service) DeleteSessionWake(ctx context.Context, taskID, sessionID, marker string) (bool, error) {
	if s.sessionWakes == nil {
		return false, fmt.Errorf("session wakes are not configured")
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return false, fmt.Errorf("session does not belong to task")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" {
		return false, fmt.Errorf("marker is required")
	}
	return s.sessionWakes.DeleteTaskSessionWake(ctx, taskID, sessionID, marker)
}

func validateSessionWakeRequest(req UpsertSessionWakeRequest) error {
	if marker := strings.TrimSpace(req.Marker); marker == "" || len(marker) > maxSessionWakeMarker {
		return fmt.Errorf("marker must be between 1 and %d characters", maxSessionWakeMarker)
	}
	if strings.TrimSpace(req.Prompt) == "" || len(req.Prompt) > maxSessionWakePrompt {
		return fmt.Errorf("prompt must be between 1 and %d characters", maxSessionWakePrompt)
	}
	if !req.ExpiresAt.After(time.Now()) {
		return fmt.Errorf("expires_at must be in the future")
	}
	_, err := cron.NextScheduleTime(req.CronExpression, normalizedTimezone(req.Timezone), time.Now().UTC())
	return err
}

func normalizedTimezone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "UTC"
	}
	return value
}
