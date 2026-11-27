package changes

import (
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func (c *Coordinator) settleUnavailableTurnRepositoryEnd(changeSetID string, row models.TurnRepositoryChangeSet, reason models.TurnChangeReason) error {
	ctx, cancel := turnChangePersistenceContext()
	defer cancel()
	accepted, err := c.repository.SetTurnRepositoryEndUnavailable(ctx, changeSetID, row.ID, row.StartCommitOID, row.StartTreeOID, reason)
	if err != nil {
		return fmt.Errorf("persist unavailable end for checkout %q: %w", row.CheckoutID, err)
	}
	if !accepted {
		return fmt.Errorf("unavailable end for checkout %q lost ownership", row.CheckoutID)
	}
	return nil
}
