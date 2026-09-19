package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

var (
	ErrCoordinatorHandoffConflict = models.ErrCoordinatorHandoffConflict
	ErrCoordinatorSuccessorModel  = errors.New("coordinator successor model is not verified")
)

// CoordinatorHandoffRequest describes one idempotent, same-task primary
// transition. OperationID is chosen by the caller and must be reused after a
// lost response.
type CoordinatorHandoffRequest struct {
	TaskID                        string
	PredecessorSessionID          string
	SuccessorSessionID            string
	PredecessorQueueIncarnationID string
	SuccessorQueueIncarnationID   string
	ExpectedAgentProfileID        string
	ExpectedModel                 string
	OperationID                   string
}

type CoordinatorHandoffResult struct {
	OperationID          string   `json:"operation_id"`
	TaskID               string   `json:"task_id"`
	PredecessorSessionID string   `json:"predecessor_session_id"`
	PrimarySessionID     string   `json:"primary_session_id"`
	AgentProfileID       string   `json:"agent_profile_id"`
	EffectiveModel       string   `json:"effective_model"`
	AssignmentGeneration int64    `json:"assignment_generation"`
	Changed              bool     `json:"changed"`
	PredecessorFenced    bool     `json:"predecessor_fenced"`
	QueueEntryIDs        []string `json:"queue_entry_ids"`
	QueueDigest          string   `json:"queue_digest"`
	QueueCount           int      `json:"queue_count"`
	AutomationTarget     string   `json:"automation_target"`
}

type coordinatorHandoffStore interface {
	PromoteCoordinatorSuccessor(context.Context, string, string, string, string, string, string) (bool, error)
	RollbackCoordinatorSuccessor(context.Context, string, string, string, string, string, string) error
}

// HandoffCoordinatorPrimary verifies a bootstrapped exact-model successor,
// atomically promotes/fences it, and transfers unread FIFO state through the
// existing durable session-transfer machinery. A failure rolls the promotion
// back before queue admissions reopen.
func (s *Service) HandoffCoordinatorPrimary(
	ctx context.Context, request CoordinatorHandoffRequest,
) (*CoordinatorHandoffResult, error) {
	s.coordinatorHandoffMu.Lock()
	defer s.coordinatorHandoffMu.Unlock()
	if err := validateCoordinatorHandoffRequest(request); err != nil {
		return nil, err
	}
	if err := s.authorizeTask(ctx, request.TaskID); err != nil {
		return nil, err
	}
	store, ok := s.repo.(coordinatorHandoffStore)
	if !ok || s.messageQueue == nil {
		return nil, errors.New("coordinator handoff is unavailable")
	}
	predecessor, successor, replay, err := s.coordinatorHandoffSessions(ctx, request)
	if err != nil {
		return nil, err
	}
	receipt, err := s.verifiedCoordinatorSuccessor(ctx, request, successor)
	if err != nil {
		return nil, err
	}
	if replay {
		return coordinatorHandoffResult(request, receipt, false, s.messageQueue.GetStatus(ctx, successor.ID)), nil
	}
	if status := s.messageQueue.GetStatus(ctx, successor.ID); status.Count != 0 {
		return nil, fmt.Errorf("%w: successor queue must be empty", ErrCoordinatorHandoffConflict)
	}
	before := s.messageQueue.GetStatus(ctx, predecessor.ID)
	changed := false
	err = s.messageQueue.TransferSessionWithDurablePreparation(
		ctx, request.TaskID, predecessor.ID, successor.ID,
		func(prepareCtx context.Context) error {
			var promoteErr error
			changed, promoteErr = store.PromoteCoordinatorSuccessor(
				prepareCtx, request.TaskID, predecessor.ID, successor.ID,
				request.PredecessorQueueIncarnationID, request.SuccessorQueueIncarnationID,
				request.OperationID,
			)
			return promoteErr
		},
		func(rollbackCtx context.Context) error {
			if !changed {
				return nil
			}
			return store.RollbackCoordinatorSuccessor(
				rollbackCtx, request.TaskID, predecessor.ID, successor.ID,
				request.PredecessorQueueIncarnationID, request.SuccessorQueueIncarnationID,
				request.OperationID,
			)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("coordinator handoff failed with predecessor retained: %w", err)
	}
	after := s.messageQueue.GetStatus(ctx, successor.ID)
	if queueDigest(before.Entries) != queueDigest(after.Entries) {
		return nil, s.rollbackCoordinatorHandoff(ctx, request, store, before, after)
	}
	return coordinatorHandoffResult(request, receipt, changed, after), nil
}

func validateCoordinatorHandoffRequest(request CoordinatorHandoffRequest) error {
	values := []string{request.TaskID, request.PredecessorSessionID, request.SuccessorSessionID,
		request.PredecessorQueueIncarnationID, request.SuccessorQueueIncarnationID,
		request.ExpectedAgentProfileID, request.ExpectedModel, request.OperationID}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return errors.New("all coordinator handoff identity fields are required")
		}
	}
	if request.PredecessorSessionID == request.SuccessorSessionID || len(request.OperationID) > 128 {
		return errors.New("coordinator handoff identities are invalid")
	}
	return nil
}

func (s *Service) coordinatorHandoffSessions(
	ctx context.Context, request CoordinatorHandoffRequest,
) (*models.TaskSession, *models.TaskSession, bool, error) {
	predecessor, err := s.repo.GetTaskSession(ctx, request.PredecessorSessionID)
	if err != nil {
		return nil, nil, false, err
	}
	successor, err := s.repo.GetTaskSession(ctx, request.SuccessorSessionID)
	if err != nil {
		return nil, nil, false, err
	}
	replay := coordinatorHandoffReplayMatches(predecessor, successor, request)
	if replay {
		return predecessor, successor, true, nil
	}
	if !coordinatorHandoffReady(predecessor, successor, request) {
		return nil, nil, false, ErrCoordinatorHandoffConflict
	}
	return predecessor, successor, false, nil
}

func coordinatorHandoffReplayMatches(
	predecessor, successor *models.TaskSession, request CoordinatorHandoffRequest,
) bool {
	return successor.TaskID == request.TaskID && successor.IsPrimary &&
		predecessor.TaskID == request.TaskID &&
		predecessor.QueueIncarnationID == request.PredecessorQueueIncarnationID &&
		successor.QueueIncarnationID == request.SuccessorQueueIncarnationID &&
		predecessor.RouteState == models.TaskSessionRouteStateCoordinatorHandoffFenced &&
		predecessor.RouteReason == request.OperationID
}

func coordinatorHandoffReady(
	predecessor, successor *models.TaskSession, request CoordinatorHandoffRequest,
) bool {
	return predecessor.TaskID == request.TaskID && successor.TaskID == request.TaskID &&
		predecessor.IsPrimary && !successor.IsPrimary &&
		predecessor.QueueIncarnationID == request.PredecessorQueueIncarnationID &&
		successor.QueueIncarnationID == request.SuccessorQueueIncarnationID &&
		predecessor.RouteState == ""
}

func (s *Service) verifiedCoordinatorSuccessor(
	ctx context.Context, request CoordinatorHandoffRequest, successor *models.TaskSession,
) (*models.ExactProfileLaunchReceipt, error) {
	receipt, err := s.ExactProfileLaunchReceipt(ctx, request.TaskID, successor.ID)
	if err != nil {
		return nil, err
	}
	verified := receipt != nil && receipt.AgentProfileID == request.ExpectedAgentProfileID &&
		receipt.Model == request.ExpectedModel && receipt.Outcome == models.ExactProfileLaunchOutcomeApplied &&
		receipt.InferenceStarted && !receipt.SubstitutionDone && successor.AgentProfileID == request.ExpectedAgentProfileID
	if !verified {
		return nil, ErrCoordinatorSuccessorModel
	}
	return receipt, nil
}

func (s *Service) rollbackCoordinatorHandoff(
	ctx context.Context, request CoordinatorHandoffRequest, store coordinatorHandoffStore,
	before, after *messagequeue.QueueStatus,
) error {
	reverseErr := s.messageQueue.TransferSessionIdentities(ctx,
		messagequeue.QueueSessionIdentity{TaskID: request.TaskID, SessionID: request.SuccessorSessionID, SessionIncarnationID: request.SuccessorQueueIncarnationID},
		messagequeue.QueueSessionIdentity{TaskID: request.TaskID, SessionID: request.PredecessorSessionID, SessionIncarnationID: request.PredecessorQueueIncarnationID},
	)
	rollbackErr := store.RollbackCoordinatorSuccessor(ctx, request.TaskID,
		request.PredecessorSessionID, request.SuccessorSessionID,
		request.PredecessorQueueIncarnationID, request.SuccessorQueueIncarnationID, request.OperationID)
	return fmt.Errorf("coordinator handoff verification failed (before=%s after=%s): %w",
		queueDigest(before.Entries), queueDigest(after.Entries),
		errors.Join(errors.New("queue identity or FIFO mismatch"), reverseErr, rollbackErr))
}

func coordinatorHandoffResult(
	request CoordinatorHandoffRequest, receipt *models.ExactProfileLaunchReceipt,
	changed bool, status *messagequeue.QueueStatus,
) *CoordinatorHandoffResult {
	ids := make([]string, 0, len(status.Entries))
	for _, entry := range status.Entries {
		ids = append(ids, entry.ID)
	}
	return &CoordinatorHandoffResult{
		OperationID: request.OperationID, TaskID: request.TaskID,
		PredecessorSessionID: request.PredecessorSessionID, PrimarySessionID: request.SuccessorSessionID,
		AgentProfileID: receipt.AgentProfileID, EffectiveModel: receipt.Model,
		AssignmentGeneration: receipt.Generation, Changed: changed, PredecessorFenced: true,
		QueueEntryIDs: ids, QueueDigest: queueDigest(status.Entries), QueueCount: len(ids),
		AutomationTarget: request.SuccessorSessionID,
	}
}

func queueDigest(entries []messagequeue.QueuedMessage) string {
	ordered := append([]messagequeue.QueuedMessage(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Position < ordered[j].Position })
	hash := sha256.New()
	for _, entry := range ordered {
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%s\x00%s\x00%t\x00", entry.ID, entry.Position, entry.Content, entry.Model, entry.PlanMode)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
