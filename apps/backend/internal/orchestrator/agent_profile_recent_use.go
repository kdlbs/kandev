package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

const deferredAgentProfileRecentUseTimeout = 5 * time.Second

// AgentProfileRecentUseRecorder persists the bounded operational profile
// history. It is optional so orchestrator-only tests and installations without
// the user service retain their existing behavior.
type AgentProfileRecentUseRecorder interface {
	RecordAgentProfileRecentUse(
		ctx context.Context,
		contextValue usermodels.AgentProfileRecentUseContext,
		profileID string,
	) (*usermodels.AgentProfileRecentUse, error)
}

// SetAgentProfileRecentUseRecorder wires the optional recorder used by
// deferred task-create launches.
func (s *Service) SetAgentProfileRecentUseRecorder(recorder AgentProfileRecentUseRecorder) {
	s.agentProfileRecentUseRecorder = recorder
}

// recordSuccessfulDeferredTaskProfileAsync updates task_create recency after
// a deferred launch has succeeded. The creator identity is restored from the
// persisted intent because workflow events do not carry the original request
// context. The write is best effort and bounded so launch lifecycle work never
// waits on preference persistence.
func (s *Service) recordSuccessfulDeferredTaskProfileAsync(
	ctx context.Context, userID, profileID string,
) {
	if s.agentProfileRecentUseRecorder == nil || userID == "" || profileID == "" {
		return
	}
	go func() {
		baseCtx := context.WithoutCancel(ctx)
		recordCtx, cancel := context.WithTimeout(
			authn.WithIdentity(baseCtx, authn.Identity{UserID: userID}),
			deferredAgentProfileRecentUseTimeout,
		)
		defer cancel()
		if _, err := s.agentProfileRecentUseRecorder.RecordAgentProfileRecentUse(
			recordCtx, usermodels.AgentProfileRecentUseTaskCreate, profileID,
		); err != nil {
			s.logger.Warn("deferred task-create profile recency recording failed",
				zap.String("user_id", userID),
				zap.String("agent_profile_id", profileID),
				zap.Error(err),
			)
		}
	}()
}
