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

	// Source and parent identity are immutable once observed. A later state
	// update (for example the unknown settlement that has no turn context)
	// must not erase the launch association.
	if existing != nil {
		if work.SourceCallID == "" {
			work.SourceCallID = existing.SourceCallID
		}
		if work.SourceMessageID == "" {
			work.SourceMessageID = existing.SourceMessageID
		}
		if work.ParentWorkID == nil && existing.ParentWorkID != nil {
			work.ParentWorkID = existing.ParentWorkID
		}
	}

	// Capability declarations are adapter evidence. Keep the recorded one when
	// an update does not carry a new declaration.
	if encoded := streams.EncodeWorkloadCapabilities(obs.Capabilities); encoded != "" {
		work.Capabilities = encoded
	} else if existing != nil {
		work.Capabilities = existing.Capabilities
	}

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
	_, err := s.RecordBackgroundWorkloadObservationWithResult(ctx, obs, taskID, sessionID)
	return err
}

// RecordBackgroundWorkloadObservationWithResult records an observation and
// returns the committed projection, so publishers can carry the durable
// revision, output, and exit status instead of the adapter's raw frame.
func (s *Service) RecordBackgroundWorkloadObservationWithResult(
	ctx context.Context,
	obs streams.WorkloadRunObservation,
	taskID, sessionID string,
) (streams.WorkloadRunObservation, error) {
	if s.backgroundWork == nil {
		return obs, nil
	}
	workID := strings.TrimSpace(obs.WorkID)
	if workID == "" {
		return obs, nil
	}
	existing, err := s.backgroundWork.GetBackgroundWorkload(ctx, sessionID, workID)
	if err != nil {
		return streams.WorkloadRunObservation{}, err
	}

	work := s.buildBackgroundWorkloadModel(obs, existing, taskID, sessionID, workID)
	if err := s.backgroundWork.UpsertBackgroundWorkload(ctx, work); err != nil {
		return streams.WorkloadRunObservation{}, err
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
		if err := s.backgroundWork.UpsertBackgroundRun(ctx, run); err != nil {
			return streams.WorkloadRunObservation{}, err
		}
	}
	return committedWorkloadObservation(work), nil
}

// committedWorkloadObservation projects the durable row back onto the
// observation contract for publication.
func committedWorkloadObservation(work *models.BackgroundWorkload) streams.WorkloadRunObservation {
	obs := settledWorkloadObservation(work)
	obs.Output = work.Output
	obs.OutputTruncated = work.OutputTruncated
	obs.OutputOffset = work.OutputOffset
	return obs
}

// SettleOpenBackgroundWorkloads atomically ends open workloads and runs after
// the owning agent process is gone, returning committed observations to publish.
func (s *Service) SettleOpenBackgroundWorkloads(ctx context.Context, sessionID string) ([]streams.WorkloadRunObservation, error) {
	if s.backgroundWork == nil || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	workloads, err := s.backgroundWork.SettleBackgroundWorkloads(ctx, sessionID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	settled := make([]streams.WorkloadRunObservation, 0, len(workloads))
	for _, work := range workloads {
		settled = append(settled, settledWorkloadObservation(work))
	}
	return settled, nil
}

// ListSessionIDsWithOpenBackgroundWork returns the sessions that still have a
// non-terminal workload.
func (s *Service) ListSessionIDsWithOpenBackgroundWork(ctx context.Context) ([]string, error) {
	if s.backgroundWork == nil {
		return nil, nil
	}
	return s.backgroundWork.ListSessionIDsWithOpenBackgroundWork(ctx)
}

func settledWorkloadObservation(work *models.BackgroundWorkload) streams.WorkloadRunObservation {
	obs := streams.WorkloadRunObservation{
		SessionID:       work.SessionID,
		WorkID:          work.ID,
		Kind:            streams.NormalizeWorkloadKind(work.Kind),
		Title:           work.Title,
		State:           streams.RunState(work.State),
		OriginTurnID:    work.OriginTurnID,
		SourceMessageID: work.SourceMessageID,
		SourceCallID:    work.SourceCallID,
		ExitCode:        work.ExitCode,
		StartedAt:       work.StartedAt,
		FinishedAt:      work.FinishedAt,
		Revision:        work.Revision,
	}
	if work.ParentWorkID != nil {
		obs.ParentWorkID = *work.ParentWorkID
	}
	if capabilities, ok := streams.DecodeWorkloadCapabilities(work.Capabilities); ok {
		obs.Capabilities = streams.ApplyWorkloadActionAvailability(capabilities, obs.State)
	}
	return obs
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
