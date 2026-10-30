package sqlite

import (
	"context"
	"errors"
	"time"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// AwaitAgentDeliveryDurability returns after every earlier commit on the
// SQLite writer survives a host power loss. Lifecycle journal releases call it
// before asking agentctl to discard retained data. Callers must not hold a
// writer transaction while awaiting, because the barrier needs the single
// writer connection. PostgreSQL commits are already synchronous, so the
// barrier is a no-op there.
func (r *Repository) AwaitAgentDeliveryDurability(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("task repository durability barrier is unavailable")
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	r.durabilityBarrierOnce.Do(func() {
		r.durabilityBarrier = internaldb.NewDurabilityBarrier(r.db.DB)
	})
	started := time.Now()
	err := r.durabilityBarrier.Await(ctx)
	observeAgentDeliveryDurabilityBarrier(err, time.Since(started))
	return err
}
