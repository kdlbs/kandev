package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestNativeResumePassthroughDoesNotRequireACPDelivery(t *testing.T) {
	for _, unresolved := range []bool{false, true} {
		t.Run(map[bool]string{false: "no ACP history", true: "unresolved ACP history"}[unresolved], func(t *testing.T) {
			manager := newTestManager(t)
			repository := &adoptionDeliveryRepository{submissions: map[string]*models.AgentDeliverySubmission{}}
			if unresolved {
				repository.submissions["prior"] = &models.AgentDeliverySubmission{
					ID: "prior", SessionID: "session", State: models.DeliverySubmissionInterruptedUnknown,
				}
			}
			manager.streamManager.setAgentDeliveryRepository(repository)
			execution := &AgentExecution{ID: "execution", SessionID: "session", IsPassthrough: true}
			require.NoError(t, manager.executionStore.Add(execution))
			err := manager.AcknowledgeNativeResumeDelivery(context.Background(), execution.SessionID)
			if unresolved {
				require.ErrorIs(t, err, ErrDurableAdoptionBlocked)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
