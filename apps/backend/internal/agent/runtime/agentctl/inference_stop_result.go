package client

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

type stopObservationKey struct{}

// StopObservationCollector keeps one exact target result across native teardown.
type StopObservationCollector struct {
	mu         sync.Mutex
	instanceID string
	result     *utility.InferenceStopObservation
}

func NewStopObservationCollector(instanceID string) *StopObservationCollector {
	return &StopObservationCollector{instanceID: instanceID}
}

func WithStopObservation(ctx context.Context, collector *StopObservationCollector) context.Context {
	return context.WithValue(ctx, stopObservationKey{}, collector)
}

func (c *StopObservationCollector) record(result *utility.InferenceStopObservation) {
	if result == nil || result.InstanceID != c.instanceID {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
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
