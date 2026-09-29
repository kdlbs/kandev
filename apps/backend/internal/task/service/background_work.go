package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func truncateUTF8End(s string, maxBytes int) (string, bool) {
	if len(s) <= maxBytes {
		return s, false
	}
	truncated := s[len(s)-maxBytes:]
	for len(truncated) > 0 && !utf8.RuneStart(truncated[0]) {
		truncated = truncated[1:]
	}
	return truncated, true
}

// ListBackgroundWorkloads returns all background workloads for a session.
func (s *Service) ListBackgroundWorkloads(ctx context.Context, sessionID string) ([]*models.BackgroundWorkload, error) {
	if s.backgroundWork == nil {
		return []*models.BackgroundWorkload{}, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if err := s.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	return s.backgroundWork.ListBackgroundWorkloadsBySession(ctx, sessionID)
}

// GetBackgroundWorkload returns a specific workload by ID within a session.
func (s *Service) GetBackgroundWorkload(ctx context.Context, sessionID, workID string) (*models.BackgroundWorkload, error) {
	if s.backgroundWork == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	workID = strings.TrimSpace(workID)
	if sessionID == "" || workID == "" {
		return nil, errors.New("session id and work id are required")
	}
	if err := s.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	return s.backgroundWork.GetBackgroundWorkload(ctx, sessionID, workID)
}

func applyWorkloadOutputBounds(work *models.BackgroundWorkload, obsOutput string, obsTruncated bool, obsOffset int64, existing *models.BackgroundWorkload) {
	const maxOutputLength = 200 * 1024
	if obsOutput != "" {
		out, trunc := truncateUTF8End(obsOutput, maxOutputLength)
		work.Output = out
		if trunc || obsTruncated {
			work.OutputTruncated = true
		}
		work.OutputOffset = obsOffset
		return
	}
	if existing != nil {
		work.Output = existing.Output
		work.OutputTruncated = existing.OutputTruncated
		work.OutputOffset = existing.OutputOffset
	}
}

func reconcileTerminalState(work *models.BackgroundWorkload, existing *models.BackgroundWorkload, obs streams.WorkloadRunObservation) {
	switch {
	case !obs.State.IsTerminal() && obs.State != "":
		work.State = existing.State
		work.FinishedAt = existing.FinishedAt
		work.ExitCode = existing.ExitCode
		work.Revision = existing.Revision
	case obs.Revision > existing.Revision:
		work.Revision = obs.Revision
	default:
		work.Revision = existing.Revision
	}
}

func reconcileNonTerminalState(work *models.BackgroundWorkload, existing *models.BackgroundWorkload, obs streams.WorkloadRunObservation) {
	switch {
	case obs.Revision > existing.Revision:
		work.Revision = obs.Revision
	case obs.State.IsTerminal():
		work.Revision = existing.Revision + 1
	case work.Revision <= existing.Revision:
		work.Revision = existing.Revision
	}
}

func reconcileExistingWorkload(work *models.BackgroundWorkload, existing *models.BackgroundWorkload, obs streams.WorkloadRunObservation) {
	if existing.OriginTurnID != "" {
		work.OriginTurnID = existing.OriginTurnID
	}
	if existing.StartedAt != nil {
		work.StartedAt = existing.StartedAt
	}
	if work.CreatedAt.IsZero() {
		work.CreatedAt = existing.CreatedAt
	}
	if streams.RunState(existing.State).IsTerminal() {
		reconcileTerminalState(work, existing, obs)
	} else {
		reconcileNonTerminalState(work, existing, obs)
	}
}

func (s *Service) buildBackgroundWorkloadModel(
	obs streams.WorkloadRunObservation,
	existing *models.BackgroundWorkload,
	taskID, sessionID, workID string,
) *models.BackgroundWorkload {
	work := &models.BackgroundWorkload{
		ID:              workID,
		TaskID:          taskID,
		SessionID:       sessionID,
		Kind:            string(obs.Kind),
		Title:           obs.Title,
		State:           string(obs.State),
		OriginTurnID:    obs.OriginTurnID,
		SourceMessageID: obs.SourceMessageID,
		SourceCallID:    obs.SourceCallID,
		ExitCode:        obs.ExitCode,
		StartedAt:       obs.StartedAt,
		FinishedAt:      obs.FinishedAt,
		Revision:        obs.Revision,
	}
	if obs.ParentWorkID != "" {
		parentID := obs.ParentWorkID
		work.ParentWorkID = &parentID
	}

	applyWorkloadOutputBounds(work, obs.Output, obs.OutputTruncated, obs.OutputOffset, existing)

	if existing != nil {
		reconcileExistingWorkload(work, existing, obs)
	} else if work.Revision == 0 {
		work.Revision = 1
	}

	return work
}

// RecordBackgroundWorkloadObservation records an observed background workload or run state.
func (s *Service) RecordBackgroundWorkloadObservation(
	ctx context.Context,
	obs streams.WorkloadRunObservation,
	taskID, sessionID string,
) error {
	if s.backgroundWork == nil {
		return nil
	}
	workID := strings.TrimSpace(obs.WorkID)
	if workID == "" {
		return nil
	}
	existing, err := s.backgroundWork.GetBackgroundWorkload(ctx, sessionID, workID)
	if err != nil {
		return err
	}

	work := s.buildBackgroundWorkloadModel(obs, existing, taskID, sessionID, workID)
	if err := s.backgroundWork.UpsertBackgroundWorkload(ctx, work); err != nil {
		return err
	}

	if obs.RunID != "" {
		run := &models.BackgroundRun{
			ID:             obs.RunID,
			WorkloadID:     workID,
			SessionID:      sessionID,
			ProviderRunKey: obs.SourceCallID,
			State:          string(obs.State),
			ExitCode:       obs.ExitCode,
			StartedAt:      obs.StartedAt,
			FinishedAt:     obs.FinishedAt,
		}
		_ = s.backgroundWork.UpsertBackgroundRun(ctx, run)
	}
	return nil
}

func mergeChunkIntoOutput(currentOutput string, workOffset int64, chunk string, chunkOffset int64) (string, int64, bool) {
	if chunkOffset <= 0 || workOffset <= 0 {
		return currentOutput + chunk, workOffset + int64(len(chunk)), false
	}
	if chunkOffset+int64(len(chunk)) <= workOffset {
		return currentOutput, workOffset, true
	}
	if chunkOffset < workOffset {
		overlap := workOffset - chunkOffset
		if int(overlap) < len(chunk) {
			currentOutput += chunk[overlap:]
		}
		return currentOutput, chunkOffset + int64(len(chunk)), false
	}
	return currentOutput + chunk, chunkOffset + int64(len(chunk)), false
}

// AppendBackgroundWorkloadOutput appends an incremental output stream chunk to a workload.
func (s *Service) AppendBackgroundWorkloadOutput(
	ctx context.Context,
	chunk streams.WorkloadOutputChunk,
	sessionID string,
) error {
	if s.backgroundWork == nil {
		return nil
	}
	workID := strings.TrimSpace(chunk.WorkID)
	if workID == "" || chunk.Chunk == "" {
		return nil
	}
	work, err := s.backgroundWork.GetBackgroundWorkload(ctx, sessionID, workID)
	if err != nil || work == nil {
		return err
	}

	newOutput, newOffset, isDuplicate := mergeChunkIntoOutput(work.Output, work.OutputOffset, chunk.Chunk, chunk.Offset)
	if isDuplicate {
		return nil
	}

	const maxOutputLength = 200 * 1024
	out, trunc := truncateUTF8End(newOutput, maxOutputLength)
	work.Output = out
	work.OutputOffset = newOffset
	if trunc || chunk.Truncated {
		work.OutputTruncated = true
	}
	work.Revision++
	work.UpdatedAt = time.Now().UTC()
	return s.backgroundWork.UpsertBackgroundWorkload(ctx, work)
}
