package client

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

type stopObservationKey struct{}

// StopObservationCollector keeps one exact target result across native teardown.
type StopObservationCollector struct {
	mu     sync.Mutex
	target string
	result *utility.InferenceStopObservation
}

func NewStopObservationCollector() *StopObservationCollector {
	return &StopObservationCollector{}
}

func WithStopObservation(ctx context.Context, collector *StopObservationCollector) context.Context {
	return context.WithValue(ctx, stopObservationKey{}, collector)
}

// record keeps the observation for the first exact delete target seen in this
// teardown context. A later target cannot overwrite it, and the response must
// match the target the transport already accepted.
func (c *StopObservationCollector) record(targetInstanceID string, result *utility.InferenceStopObservation) {
	if result == nil || result.InstanceID == "" || result.InstanceID != targetInstanceID {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.target != "" && c.target != targetInstanceID {
		return
	}
	c.target = targetInstanceID
	c.result = result
}

func (c *StopObservationCollector) Result() *utility.InferenceStopObservation {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.result == nil {
		return nil
	}
	copy := *c.result
	return &copy
}

func validManagedStop(receipt *utility.ManagedProcessStopReceipt, snapshot *utility.AttemptSnapshot, instanceID string) bool {
	if receipt == nil || snapshot == nil {
		return false
	}
	if receipt.Version != 1 || receipt.InstanceID != instanceID || receipt.OwnerID == "" || receipt.OwnerID != snapshot.OwnerID {
		return false
	}
	return completeManagedStop(receipt, snapshot)
}

func completeManagedStop(receipt *utility.ManagedProcessStopReceipt, snapshot *utility.AttemptSnapshot) bool {
	return receipt.InferenceCensusKnown == snapshot.Known && receipt.RecordedChildrenTerminal == utility.RecordedAttemptsTerminal(snapshot) &&
		receipt.Coverage == utility.ManagedStopCoverage && !receipt.ObservedAt.IsZero() && receipt.AdmissionClosed && receipt.OperationsDrained && receipt.ManagedProcessesStopped
}
