package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

const maxAgentProfileRecentUseCASAttempts = 3

var errAgentProfileRecentUseRepositoryUnavailable = errors.New("agent profile recent-use repository unavailable")

// GetAgentProfileRecentUse returns the persisted operational profile histories
// for the current user. Missing contexts are omitted and read as empty.
func (s *Service) GetAgentProfileRecentUse(ctx context.Context) ([]*models.AgentProfileRecentUse, error) {
	if s.recentUseRepo == nil {
		return nil, errAgentProfileRecentUseRepositoryUnavailable
	}
	return s.recentUseRepo.ListAgentProfileRecentUse(ctx, s.settingsUserID(ctx))
}

// RecordAgentProfileRecentUse moves a successfully used profile to the front
// of its context history. It retries a bounded number of stale revisions so
// concurrent clients converge without making the launch path synchronous.
func (s *Service) RecordAgentProfileRecentUse(
	ctx context.Context,
	contextValue models.AgentProfileRecentUseContext,
	profileID string,
) (*models.AgentProfileRecentUse, error) {
	if err := validateAgentProfileRecentUseInput(contextValue, profileID); err != nil {
		return nil, err
	}
	if s.recentUseRepo == nil {
		return nil, errAgentProfileRecentUseRepositoryUnavailable
	}
	userID := s.settingsUserID(ctx)
	for attempt := 0; attempt < maxAgentProfileRecentUseCASAttempts; attempt++ {
		current, err := s.currentAgentProfileRecentUse(ctx, userID, contextValue)
		if err != nil {
			return nil, err
		}
		if len(current.ProfileIDs) > 0 && current.ProfileIDs[0] == profileID {
			return current, nil
		}
		next := buildAgentProfileRecentUse(current, profileID)
		updated, err := s.recentUseRepo.UpsertAgentProfileRecentUse(ctx, next, current.Revision)
		if errors.Is(err, store.ErrAgentProfileRecentUseRevisionConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		s.publishAgentProfileRecentUseEvent(ctx, updated)
		return updated, nil
	}
	return nil, store.ErrAgentProfileRecentUseRevisionConflict
}

func validateAgentProfileRecentUseInput(
	contextValue models.AgentProfileRecentUseContext,
	profileID string,
) error {
	if !models.IsAgentProfileRecentUseContext(contextValue) {
		return fmt.Errorf("%w: unsupported context", ErrValidation)
	}
	if profileID == "" || len(profileID) > models.AgentProfileRecentUseMaxProfileIDBytes {
		return fmt.Errorf("%w: profile id must be between 1 and %d bytes", ErrValidation, models.AgentProfileRecentUseMaxProfileIDBytes)
	}
	return nil
}

func (s *Service) currentAgentProfileRecentUse(
	ctx context.Context,
	userID string,
	contextValue models.AgentProfileRecentUseContext,
) (*models.AgentProfileRecentUse, error) {
	record, err := s.recentUseRepo.GetAgentProfileRecentUse(ctx, userID, contextValue)
	if errors.Is(err, sql.ErrNoRows) {
		return &models.AgentProfileRecentUse{
			UserID:     userID,
			Context:    contextValue,
			ProfileIDs: []string{},
		}, nil
	}
	return record, err
}

func buildAgentProfileRecentUse(
	current *models.AgentProfileRecentUse,
	profileID string,
) *models.AgentProfileRecentUse {
	profileIDs := make([]string, 0, models.AgentProfileRecentUseMaxProfiles)
	profileIDs = append(profileIDs, profileID)
	for _, existingID := range current.ProfileIDs {
		if existingID == profileID || len(profileIDs) == models.AgentProfileRecentUseMaxProfiles {
			continue
		}
		profileIDs = append(profileIDs, existingID)
	}
	return &models.AgentProfileRecentUse{
		UserID:     current.UserID,
		Context:    current.Context,
		ProfileIDs: profileIDs,
		Revision:   current.Revision + 1,
		UpdatedAt:  time.Now().UTC(),
	}
}

func (s *Service) publishAgentProfileRecentUseEvent(
	ctx context.Context,
	record *models.AgentProfileRecentUse,
) {
	if s.eventBus == nil || record == nil {
		return
	}
	data := map[string]interface{}{
		"user_id":     record.UserID,
		"context":     record.Context,
		"profile_ids": record.ProfileIDs,
		"revision":    record.Revision,
		"updated_at":  record.UpdatedAt.Format(time.RFC3339),
	}
	if err := s.eventBus.Publish(
		ctx,
		events.UserAgentProfileRecentUseUpdated,
		bus.NewEvent(events.UserAgentProfileRecentUseUpdated, "user-service", data),
	); err != nil {
		s.logger.Error("failed to publish agent profile recent-use event", zap.Error(err))
	}
}
