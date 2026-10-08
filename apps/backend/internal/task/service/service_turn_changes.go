package service

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/changes"
	"github.com/kandev/kandev/internal/task/models"
)

var ErrTurnChangeContentExpired = errors.New("turn change content expired")

type TurnChangeContentExpiredError struct {
	Reason models.TurnChangeReason
}

func (e *TurnChangeContentExpiredError) Error() string { return ErrTurnChangeContentExpired.Error() }

func (e *TurnChangeContentExpiredError) Unwrap() error { return ErrTurnChangeContentExpired }

const (
	defaultTurnChangePageSize = 50
	maxTurnChangePageSize     = 100
)

func normalizeTurnChangePage(offset, limit int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > maxTurnChangePageSize {
		limit = defaultTurnChangePageSize
	}
	return offset, limit
}

func (s *Service) ListTurnChangeHistory(ctx context.Context, sessionID string, offset, limit int) ([]*models.TurnChangeSet, int, error) {
	if s.turnChanges == nil {
		return nil, 0, errors.New("turn change history is unavailable")
	}
	session, err := s.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, 0, err
	}
	offset, limit = normalizeTurnChangePage(offset, limit)
	return s.turnChanges.ListTurnChangeSets(ctx, session.TaskID, session.ID, offset, limit)
}

func (s *Service) GetTurnChangeHistory(ctx context.Context, sessionID, changeSetID string) (*models.TurnChangeSet, error) {
	if s.turnChanges == nil {
		return nil, errors.New("turn change history is unavailable")
	}
	session, err := s.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return s.turnChanges.GetTurnChangeSet(ctx, session.TaskID, session.ID, changeSetID)
}

func (s *Service) ListTurnChangeRepositories(ctx context.Context, sessionID, changeSetID string) ([]*models.TurnRepositoryChangeSet, error) {
	if _, err := s.GetTurnChangeHistory(ctx, sessionID, changeSetID); err != nil {
		return nil, err
	}
	return s.turnChanges.ListTurnRepositoryChanges(ctx, changeSetID)
}

func (s *Service) ListTurnChangeFiles(ctx context.Context, sessionID, changeSetID, repositoryChangeID string, offset, limit int) ([]*models.TurnFileChange, int, error) {
	if _, err := s.GetTurnChangeHistory(ctx, sessionID, changeSetID); err != nil {
		return nil, 0, err
	}
	if _, err := s.turnChanges.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID); err != nil {
		return nil, 0, err
	}
	offset, limit = normalizeTurnChangePage(offset, limit)
	return s.turnChanges.ListTurnFileChanges(ctx, changeSetID, repositoryChangeID, offset, limit)
}

func (s *Service) ReadTurnChangeContent(ctx context.Context, sessionID, changeSetID, fileChangeID string, variant models.TurnChangeContentVariant) (*models.TurnChangeContentPayload, error) {
	changeSet, err := s.GetTurnChangeHistory(ctx, sessionID, changeSetID)
	if err != nil {
		return nil, err
	}
	if changeSet.Availability == models.TurnChangeAvailabilityExpired ||
		(changeSet.RetainUntil != nil && !changeSet.RetainUntil.After(time.Now().UTC())) {
		reason := changeSet.ExpiryReason
		if reason == "" {
			reason = string(models.TurnChangeReasonExpiredAge)
		}
		return nil, &TurnChangeContentExpiredError{Reason: models.TurnChangeReason(reason)}
	}
	return changes.NewContentService(s.turnChanges, nil).Read(ctx, changeSetID, fileChangeID, variant)
}

func (s *Service) PublishTurnChangeSummary(ctx context.Context, taskID, sessionID, changeSetID string) error {
	if s.turnChanges == nil || s.eventBus == nil {
		return errors.New("turn change summary publisher is unavailable")
	}
	changeSet, err := s.turnChanges.GetTurnChangeSet(ctx, taskID, sessionID, changeSetID)
	if err != nil {
		return err
	}
	repositories, err := s.turnChanges.ListTurnRepositoryChanges(ctx, changeSetID)
	if err != nil {
		return err
	}
	summary := projectTurnChangeSummaryEvent(changeSet, repositories)
	return s.eventBus.Publish(ctx, events.SessionTurnChangesUpdated, bus.NewEvent(
		events.SessionTurnChangesUpdated, "task-service", map[string]interface{}{
			"session_id": sessionID,
			"change_set": summary,
		},
	))
}

type turnChangeSummaryEvent struct {
	ID                      string                             `json:"id"`
	TaskID                  string                             `json:"task_id"`
	SessionID               string                             `json:"session_id"`
	TurnID                  string                             `json:"turn_id"`
	Revision                int64                              `json:"revision"`
	Availability            models.TurnChangeAvailability      `json:"availability"`
	Reason                  models.TurnChangeReason            `json:"reason,omitempty"`
	Complete                bool                               `json:"complete"`
	SummaryComplete         bool                               `json:"summary_complete"`
	ContentComplete         bool                               `json:"content_complete"`
	TurnOrdinal             int64                              `json:"turn_ordinal"`
	TerminalAt              *time.Time                         `json:"terminal_at,omitempty"`
	TerminalOutcome         string                             `json:"terminal_outcome,omitempty"`
	FinalAssistantMessageID string                             `json:"final_assistant_message_id,omitempty"`
	FallbackAnchor          string                             `json:"fallback_anchor"`
	FileCount               int64                              `json:"file_count"`
	AddedLines              *int64                             `json:"added_lines,omitempty"`
	DeletedLines            *int64                             `json:"deleted_lines,omitempty"`
	BinaryFileCount         int64                              `json:"binary_file_count"`
	UnknownCountFileCount   int64                              `json:"unknown_count_file_count"`
	RepositoryCount         int64                              `json:"repository_count"`
	RetainUntil             *time.Time                         `json:"retain_until,omitempty"`
	ExpiryReason            string                             `json:"expiry_reason,omitempty"`
	OverlapIntervals        []models.TurnChangeOverlap         `json:"overlap_intervals,omitempty"`
	Repositories            []turnRepositoryChangeSummaryEvent `json:"repositories"`
}

type turnRepositoryChangeSummaryEvent struct {
	ID                  string                        `json:"id"`
	CheckoutID          string                        `json:"checkout_id"`
	RepositoryID        string                        `json:"repository_id,omitempty"`
	WorktreeID          string                        `json:"worktree_id,omitempty"`
	DisplayName         string                        `json:"display_name,omitempty"`
	RepositorySubpath   string                        `json:"repository_subpath,omitempty"`
	Availability        models.TurnChangeAvailability `json:"availability"`
	Reason              models.TurnChangeReason       `json:"reason,omitempty"`
	EnumerationComplete bool                          `json:"enumeration_complete"`
	ComparisonComplete  bool                          `json:"comparison_complete"`
	ContentComplete     bool                          `json:"content_complete"`
	OverlapIntervals    []models.TurnChangeOverlap    `json:"overlap_intervals,omitempty"`
}

func projectTurnChangeSummaryEvent(changeSet *models.TurnChangeSet, repositories []*models.TurnRepositoryChangeSet) turnChangeSummaryEvent {
	if changeSet == nil {
		return turnChangeSummaryEvent{Repositories: []turnRepositoryChangeSummaryEvent{}}
	}
	projected := turnChangeSummaryEvent{
		ID: changeSet.ID, TaskID: changeSet.TaskID, SessionID: changeSet.TaskSessionID, TurnID: changeSet.TurnID,
		Revision: changeSet.Revision, Availability: changeSet.Availability, Reason: changeSet.Reason,
		Complete: changeSet.Complete, SummaryComplete: changeSet.SummaryComplete, ContentComplete: changeSet.ContentComplete,
		TurnOrdinal: changeSet.TurnOrdinal, TerminalAt: changeSet.TerminalAt, TerminalOutcome: changeSet.TerminalOutcome,
		FinalAssistantMessageID: changeSet.FinalAssistantMessageID, FallbackAnchor: changeSet.FallbackAnchor,
		FileCount: changeSet.FileCount, AddedLines: changeSet.AddedLines, DeletedLines: changeSet.DeletedLines,
		BinaryFileCount: changeSet.BinaryFileCount, UnknownCountFileCount: changeSet.UnknownCountFileCount,
		RepositoryCount: changeSet.RepositoryCount, RetainUntil: changeSet.RetainUntil, ExpiryReason: changeSet.ExpiryReason,
		OverlapIntervals: changeSet.OverlapIntervals, Repositories: make([]turnRepositoryChangeSummaryEvent, 0, len(repositories)),
	}
	for _, repository := range repositories {
		if repository == nil {
			continue
		}
		projected.Repositories = append(projected.Repositories, turnRepositoryChangeSummaryEvent{
			ID: repository.ID, CheckoutID: repository.CheckoutID, RepositoryID: repository.RepositoryID,
			WorktreeID: repository.WorktreeID, DisplayName: repository.DisplayName,
			RepositorySubpath: repository.RepositorySubpath, Availability: repository.Availability,
			Reason: repository.Reason, EnumerationComplete: repository.EnumerationComplete,
			ComparisonComplete: repository.ComparisonComplete, ContentComplete: repository.ContentComplete,
			OverlapIntervals: repository.OverlapIntervals,
		})
	}
	return projected
}
