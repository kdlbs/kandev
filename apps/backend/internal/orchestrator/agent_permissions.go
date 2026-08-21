package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

var (
	ErrPermissionTaskOrSessionNotFound = errors.New("task_or_session_not_found")
	ErrPermissionNotFound              = errors.New("permission_not_found")
	ErrPermissionStale                 = errors.New("permission_stale")
	ErrPermissionAlreadyResolved       = errors.New("permission_already_resolved")
	ErrPermissionResolutionInProgress  = errors.New("permission_resolution_in_progress")
	ErrPermissionOptionNotOffered      = errors.New("permission_option_not_offered")
	ErrPermissionAuditFailed           = errors.New("permission_audit_failed")
	ErrPermissionDeliveryFailed        = errors.New("permission_delivery_failed")
)

type ResolveAgentPermissionRequest struct {
	TaskID    string
	SessionID string
	RequestID string
	PendingID string
	OptionID  string
	Source    models.PermissionResolutionSource
}

type ResolveAgentPermissionResult struct {
	TaskID     string                            `json:"task_id"`
	SessionID  string                            `json:"session_id"`
	RequestID  string                            `json:"request_id"`
	PendingID  string                            `json:"pending_id"`
	OptionID   string                            `json:"option_id"`
	OptionKind streams.PermissionOptionKind      `json:"option_kind"`
	Source     models.PermissionResolutionSource `json:"source"`
	Status     string                            `json:"status"`
}

// ListPendingAgentPermissions returns only live agentctl snapshots after the
// server-owned task/session relationship has been authorized.
func (s *Service) ListPendingAgentPermissions(ctx context.Context, taskID, sessionID string) ([]streams.PendingAgentPermission, error) {
	sessions, err := s.authorizedPermissionSessions(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	permissions := make([]streams.PendingAgentPermission, 0)
	for _, session := range sessions {
		live, listErr := s.executor.ListPendingPermissions(ctx, session.ID)
		if errors.Is(listErr, executor.ErrExecutionNotFound) {
			continue
		}
		if listErr != nil {
			return nil, fmt.Errorf("list live permissions: %w", listErr)
		}
		for _, permission := range live {
			permission.TaskID = taskID
			permission.SessionID = session.ID
			permissions = append(permissions, permission)
		}
	}
	sort.Slice(permissions, func(i, j int) bool {
		if permissions[i].CreatedAt.Equal(permissions[j].CreatedAt) {
			return permissions[i].RequestID < permissions[j].RequestID
		}
		return permissions[i].CreatedAt.Before(permissions[j].CreatedAt)
	})
	return permissions, nil
}

// ResolveAgentPermission claims the durable audit record before delivering one
// exact provider-offered option to the current live request generation.
func (s *Service) ResolveAgentPermission(ctx context.Context, request ResolveAgentPermissionRequest) (*ResolveAgentPermissionResult, error) {
	if !completePermissionResolutionIdentity(request) {
		return nil, ErrPermissionNotFound
	}
	permissions, err := s.ListPendingAgentPermissions(ctx, request.TaskID, request.SessionID)
	if err != nil {
		return nil, err
	}
	_, option, lookupErr := findPermissionOption(permissions, request)
	if lookupErr != nil {
		return nil, s.permissionLookupError(ctx, request, lookupErr)
	}
	if s.messageCreator == nil {
		return nil, ErrPermissionAuditFailed
	}
	if request.Source == "" {
		request.Source = models.PermissionSourceAutomation
	}
	claimID, err := s.claimAgentPermission(ctx, request, option)
	if err != nil {
		return nil, err
	}

	delivered, deliveryErr := s.executor.ResolvePermission(ctx, request.SessionID, request.RequestID, request.PendingID, request.OptionID)
	if deliveryErr != nil {
		return nil, s.finalizePermissionDeliveryFailure(ctx, request, claimID, deliveryErr)
	}
	if delivered == nil {
		return nil, s.finalizePermissionDeliveryFailure(ctx, request, claimID, errors.New("empty permission delivery response"))
	}
	status := models.PermissionStatusApproved
	if strings.HasPrefix(string(delivered.OptionKind), "reject_") {
		status = models.PermissionStatusRejected
	}
	finalized, err := s.messageCreator.FinalizePermissionResolution(ctx, models.PermissionResolutionFinalizeRequest{
		TaskID:      request.TaskID,
		SessionID:   request.SessionID,
		RequestID:   request.RequestID,
		PendingID:   request.PendingID,
		ClaimID:     claimID,
		Result:      models.PermissionResolutionAccepted,
		Status:      status,
		FinalizedAt: time.Now().UTC(),
	})
	if err != nil || finalized.Outcome != models.PermissionFinalized {
		return nil, ErrPermissionAuditFailed
	}
	s.markSessionRunningAfterPermission(ctx, request.SessionID)
	return &ResolveAgentPermissionResult{
		TaskID:     request.TaskID,
		SessionID:  request.SessionID,
		RequestID:  request.RequestID,
		PendingID:  request.PendingID,
		OptionID:   delivered.OptionID,
		OptionKind: delivered.OptionKind,
		Source:     request.Source,
		Status:     "resolved",
	}, nil
}

func completePermissionResolutionIdentity(request ResolveAgentPermissionRequest) bool {
	return request.TaskID != "" && request.SessionID != "" && request.RequestID != "" &&
		request.PendingID != "" && request.OptionID != ""
}

func (s *Service) claimAgentPermission(ctx context.Context, request ResolveAgentPermissionRequest, option *streams.PermissionChoice) (string, error) {
	actorUserID, actorKind := permissionAuditActor(ctx)
	claimID := uuid.NewString()
	claim, err := s.claimPermissionWithRetry(ctx, models.PermissionResolutionClaimRequest{
		TaskID:    request.TaskID,
		SessionID: request.SessionID,
		Audit: models.PermissionResolutionAudit{
			ClaimID:     claimID,
			ActorUserID: actorUserID,
			ActorKind:   actorKind,
			Source:      request.Source,
			RequestID:   request.RequestID,
			PendingID:   request.PendingID,
			OptionID:    option.OptionID,
			OptionKind:  string(option.Kind),
			SelectedAt:  time.Now().UTC(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPermissionAuditFailed, err)
	}
	if claim == nil {
		return "", ErrPermissionAuditFailed
	}
	switch claim.Outcome {
	case models.PermissionClaimed:
		return claimID, nil
	case models.PermissionClaimInProgress:
		return "", ErrPermissionResolutionInProgress
	case models.PermissionClaimAlreadyFinal:
		return "", ErrPermissionAlreadyResolved
	default:
		return "", ErrPermissionAuditFailed
	}
}

func (s *Service) cancelAgentPermission(ctx context.Context, request ResolveAgentPermissionRequest) error {
	permissions, err := s.ListPendingAgentPermissions(ctx, request.TaskID, request.SessionID)
	if err != nil {
		return err
	}
	if lookupErr := findPermissionForCancellation(permissions, request); lookupErr != nil {
		if errors.Is(lookupErr, ErrPermissionStale) {
			return lookupErr
		}
		return s.permissionLookupError(ctx, request, lookupErr)
	}
	if s.messageCreator == nil {
		return ErrPermissionAuditFailed
	}
	claimID, err := s.claimAgentPermission(ctx, request, &streams.PermissionChoice{
		OptionID: stopReasonCancelled,
		Kind:     streams.PermissionOptionKind(stopReasonCancelled),
	})
	if err != nil {
		return err
	}
	if _, err := s.executor.CancelPermission(ctx, request.SessionID, request.RequestID, request.PendingID); err != nil {
		return s.finalizePermissionDeliveryFailure(ctx, request, claimID, err)
	}
	finalized, err := s.messageCreator.FinalizePermissionResolution(ctx, models.PermissionResolutionFinalizeRequest{
		TaskID:      request.TaskID,
		SessionID:   request.SessionID,
		RequestID:   request.RequestID,
		PendingID:   request.PendingID,
		ClaimID:     claimID,
		Result:      models.PermissionResolutionAccepted,
		Status:      models.PermissionStatusRejected,
		FinalizedAt: time.Now().UTC(),
	})
	if err != nil || finalized.Outcome != models.PermissionFinalized {
		return ErrPermissionAuditFailed
	}
	return nil
}

func findPermissionForCancellation(permissions []streams.PendingAgentPermission, request ResolveAgentPermissionRequest) error {
	for _, permission := range permissions {
		if permission.PendingID != request.PendingID {
			continue
		}
		if permission.RequestID != request.RequestID {
			return ErrPermissionStale
		}
		return nil
	}
	return ErrPermissionNotFound
}

func (s *Service) authorizedPermissionSessions(ctx context.Context, taskID, sessionID string) ([]*models.TaskSession, error) {
	if taskID == "" {
		return nil, ErrPermissionTaskOrSessionNotFound
	}
	if sessionID != "" {
		if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
			return nil, ErrPermissionTaskOrSessionNotFound
		}
		session, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil || session == nil || session.TaskID != taskID {
			return nil, ErrPermissionTaskOrSessionNotFound
		}
		return []*models.TaskSession{session}, nil
	}
	if err := s.authorizeTask(ctx, taskID); err != nil {
		return nil, ErrPermissionTaskOrSessionNotFound
	}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, ErrPermissionTaskOrSessionNotFound
	}
	return sessions, nil
}

func findPermissionOption(permissions []streams.PendingAgentPermission, request ResolveAgentPermissionRequest) (*streams.PendingAgentPermission, *streams.PermissionChoice, error) {
	for i := range permissions {
		permission := &permissions[i]
		if permission.PendingID != request.PendingID {
			continue
		}
		if permission.RequestID != request.RequestID {
			return nil, nil, ErrPermissionStale
		}
		for j := range permission.Options {
			if permission.Options[j].OptionID == request.OptionID {
				return permission, &permission.Options[j], nil
			}
		}
		return permission, nil, ErrPermissionOptionNotOffered
	}
	return nil, nil, ErrPermissionNotFound
}

func (s *Service) permissionLookupError(ctx context.Context, request ResolveAgentPermissionRequest, lookupErr error) error {
	if errors.Is(lookupErr, ErrPermissionStale) || errors.Is(lookupErr, ErrPermissionOptionNotOffered) {
		return lookupErr
	}
	if s.messageCreator == nil {
		return lookupErr
	}
	audit, err := s.messageCreator.GetPermissionResolutionAudit(ctx, request.TaskID, request.SessionID, request.RequestID, request.PendingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ErrPermissionAuditFailed
	}
	if audit == nil {
		if updateErr := s.messageCreator.UpdatePermissionMessage(ctx, request.TaskID, request.SessionID, request.RequestID, request.PendingID, models.PermissionStatusExpired); updateErr != nil {
			s.logger.Warn("failed to expire stale permission message",
				zap.String("session_id", request.SessionID),
				zap.String("pending_id", request.PendingID),
				zap.Error(updateErr))
		}
		return lookupErr
	}
	if audit.Result == models.PermissionResolutionDispatching {
		return ErrPermissionResolutionInProgress
	}
	return ErrPermissionAlreadyResolved
}

func (s *Service) claimPermissionWithRetry(ctx context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
	const attempts = 5
	for attempt := 0; attempt < attempts; attempt++ {
		claim, err := s.messageCreator.ClaimPermissionResolution(ctx, request)
		if err != nil || claim == nil || claim.Outcome != models.PermissionClaimNotFound || attempt == attempts-1 {
			return claim, err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, nil
}

func (s *Service) finalizePermissionDeliveryFailure(ctx context.Context, request ResolveAgentPermissionRequest, claimID string, deliveryErr error) error {
	result := models.PermissionResolutionFailed
	domainErr := ErrPermissionDeliveryFailed
	if code := permissionErrorCode(deliveryErr); code == streams.PermissionErrorStale || code == streams.PermissionErrorNotFound || code == streams.PermissionErrorAlreadyResolved {
		result = models.PermissionResolutionStale
		domainErr = ErrPermissionStale
	}
	finalized, finalizeErr := s.messageCreator.FinalizePermissionResolution(ctx, models.PermissionResolutionFinalizeRequest{
		TaskID:      request.TaskID,
		SessionID:   request.SessionID,
		RequestID:   request.RequestID,
		PendingID:   request.PendingID,
		ClaimID:     claimID,
		Result:      result,
		Status:      models.PermissionStatusExpired,
		FinalizedAt: time.Now().UTC(),
	})
	if finalizeErr != nil || finalized.Outcome != models.PermissionFinalized {
		return ErrPermissionAuditFailed
	}
	return domainErr
}

func permissionAuditActor(ctx context.Context) (string, models.PermissionResolutionActorKind) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		return "", models.PermissionActorAutomation
	}
	switch {
	case identity.Synthetic:
		return identity.UserID, models.PermissionActorSynthetic
	case identity.TokenID != "":
		return identity.UserID, models.PermissionActorPersonalAccessToken
	case identity.SessionID != "":
		return identity.UserID, models.PermissionActorBrowser
	default:
		return identity.UserID, models.PermissionActorAutomation
	}
}

func permissionErrorCode(err error) string {
	type coded interface{ PermissionCode() string }
	var permissionErr coded
	if errors.As(err, &permissionErr) {
		return permissionErr.PermissionCode()
	}
	return ""
}

func (s *Service) markSessionRunningAfterPermission(ctx context.Context, sessionID string) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return
	}
	s.setSessionRunning(ctx, session.TaskID, sessionID, session)
}
