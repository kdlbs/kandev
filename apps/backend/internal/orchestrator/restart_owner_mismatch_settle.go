package orchestrator

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"go.uber.org/zap"
)

// errDetachedDeliveryOwnerMismatch marks a silent-restore failure whose
// delivery owner was proven to be different from the one pinned by recovery.
var errDetachedDeliveryOwnerMismatch = errors.New("detached delivery owner mismatch")

// isDetachedDeliveryOwnerMismatch reports an error proving the agentctl owner
// rejected the delivery as belonging to a different owner (HTTP 409 with the
// OWNER_MISMATCH code), or the lifecycle's own owner-mismatch sentinel.
// Retrying cannot succeed. A bare stale-evidence error is not enough: it can
// also be a transient snapshot change.
func isDetachedDeliveryOwnerMismatch(err error) bool {
	if errors.Is(err, agentruntime.ErrDeliveryOwnerMismatch) {
		return true
	}
	var httpErr *agentctl.DeliveryHTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict &&
		strings.Contains(httpErr.Body, "OWNER_MISMATCH")
}

// settleDetachedTurnAfterOwnerMismatch settles a detached turn whose delivery
// can no longer be reconciled: still-dispatching submissions become
// interrupted_unknown and open turns are abandoned, which frees the session
// for new input instead of retrying the same mismatching delivery.
func (s *Service) settleDetachedTurnAfterOwnerMismatch(ctx context.Context, sessionID string) error {
	lister, ok := s.repo.(nativeResumeDeliverySubmissionReader)
	store, storeOK := s.repo.(taskrepo.AgentDeliveryRepository)
	if !ok || !storeOK {
		return errors.New("delivery submission store is unavailable")
	}
	rows, err := lister.ListAgentDeliverySubmissions(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.State != models.DeliverySubmissionDispatching {
			continue
		}
		if _, err := store.TransitionAgentDeliverySubmission(ctx, row.ID, row.State,
			models.DeliverySubmissionInterruptedUnknown, unknownPromptOutcome, time.Now().UTC()); err != nil {
			return err
		}
	}
	s.abandonOpenTurnsOnStartup(ctx, sessionID, "delivery owner mismatch on detached turn")
	s.logger.Info("settled detached turn as interrupted after delivery owner mismatch",
		zap.String("session_id", sessionID))
	return nil
}
