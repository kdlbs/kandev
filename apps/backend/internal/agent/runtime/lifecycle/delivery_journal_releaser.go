package lifecycle

import (
	"context"
	"errors"
	"fmt"
)

// errDeliveryDurabilityBarrier marks a journal release that was refused
// because the SQL sink could not prove its earlier commits durable. Each
// caller keeps its pending progress and retries through its existing owner.
var errDeliveryDurabilityBarrier = errors.New("delivery durability barrier failed")

// deliveryJournalReleaser is the single gate between backend state and any
// agentctl request that discards retained journal data. Every request first
// awaits the durability barrier that covers the SQL state it relies on, then
// issues the agentctl call. A structural test rejects direct client calls from
// any other production file in this package.
type deliveryJournalReleaser struct {
	durability func(context.Context) error
}

type deliveryAcknowledgerClient interface {
	AcknowledgeDelivery(ctx context.Context, streamID string, sequence uint64) error
}

type deliveryRetirerClient interface {
	RetireDeliverySubmission(ctx context.Context, id string) error
}

type deliveryInterruptedRetirerClient interface {
	RetireInterruptedDeliverySubmission(ctx context.Context, id, streamID string, generation uint64) error
}

func (sm *StreamManager) deliveryJournalReleaser() deliveryJournalReleaser {
	if sm == nil {
		return deliveryJournalReleaser{}
	}
	return deliveryJournalReleaser{durability: sm.awaitDeliveryDurability}
}

func (m *Manager) deliveryJournalReleaser() deliveryJournalReleaser {
	if m == nil || m.streamManager == nil {
		return deliveryJournalReleaser{}
	}
	return m.streamManager.deliveryJournalReleaser()
}

func (sm *SessionManager) deliveryJournalReleaser() deliveryJournalReleaser {
	if sm == nil || sm.streamManager == nil {
		return deliveryJournalReleaser{}
	}
	return sm.streamManager.deliveryJournalReleaser()
}

// awaitDeliveryDurability is a no-op for legacy or isolated runtimes without a
// delivery repository that can prove power-loss durability.
func (sm *StreamManager) awaitDeliveryDurability(ctx context.Context) error {
	delivery := sm.deliveryRepository()
	if delivery == nil {
		return nil
	}
	barrier, ok := delivery.(agentDeliveryDurabilityBarrier)
	if !ok {
		return nil
	}
	return barrier.AwaitAgentDeliveryDurability(ctx)
}

type agentDeliveryDurabilityBarrier interface {
	AwaitAgentDeliveryDurability(context.Context) error
}

func (r deliveryJournalReleaser) await(ctx context.Context) error {
	if r.durability == nil {
		return nil
	}
	if err := r.durability(ctx); err != nil {
		return fmt.Errorf("%w: %w", errDeliveryDurabilityBarrier, err)
	}
	return nil
}

func (r deliveryJournalReleaser) acknowledge(
	ctx context.Context,
	client deliveryAcknowledgerClient,
	streamID string,
	sequence uint64,
) error {
	if streamID == "" || sequence == 0 {
		return nil
	}
	if client == nil {
		return errors.New("agent delivery acknowledger is unavailable")
	}
	if err := r.await(ctx); err != nil {
		return err
	}
	return client.AcknowledgeDelivery(ctx, streamID, sequence)
}

func (r deliveryJournalReleaser) retire(
	ctx context.Context,
	client deliveryRetirerClient,
	id string,
) error {
	if client == nil {
		return errors.New("agent delivery retirer is unavailable")
	}
	if err := r.await(ctx); err != nil {
		return err
	}
	return client.RetireDeliverySubmission(ctx, id)
}

func (r deliveryJournalReleaser) retireInterrupted(
	ctx context.Context,
	client deliveryInterruptedRetirerClient,
	id, streamID string,
	generation uint64,
) error {
	if client == nil {
		return errors.New("agent delivery retirer is unavailable")
	}
	if err := r.await(ctx); err != nil {
		return err
	}
	return client.RetireInterruptedDeliverySubmission(ctx, id, streamID, generation)
}
