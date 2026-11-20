package lifecycle

import (
	"context"
	"time"
)

const defaultRuntimeStorageAdvisoryInterval = 30 * time.Second

// startRuntimeStorageAdvisory owns one advisory loop per execution. It starts
// from the workspace stream, so passthrough sessions receive capacity evidence
// without any delivery-health reconciliation.
func (sm *StreamManager) startRuntimeStorageAdvisory(execution *AgentExecution) {
	sm.startRuntimeStorageAdvisoryWithContext(sm.streamContext(execution), execution)
}

func (sm *StreamManager) startRuntimeStorageAdvisoryWithContext(
	parent context.Context,
	execution *AgentExecution,
) {
	if sm == nil || execution == nil || execution.ID == "" {
		return
	}
	if previous, loaded := sm.runtimeStorageAdvisories.LoadAndDelete(execution.ID); loaded {
		if cancel, ok := previous.(context.CancelFunc); ok {
			cancel()
		}
	}
	ctx, cancel := context.WithCancel(parent)
	sm.runtimeStorageAdvisories.Store(execution.ID, cancel)
	if !sm.start(func() {
		defer sm.releaseRuntimeStorageAdvisory(execution)
		defer cancel()
		sm.monitorRuntimeStorageAdvisory(ctx, execution)
	}) {
		cancel()
		sm.runtimeStorageAdvisories.Delete(execution.ID)
	}
}

func (sm *StreamManager) releaseRuntimeStorageAdvisory(execution *AgentExecution) {
	if current, loaded := sm.runtimeStorageAdvisories.Load(execution.ID); loaded {
		if cancel, ok := current.(context.CancelFunc); ok {
			cancel()
		}
		sm.runtimeStorageAdvisories.Delete(execution.ID)
	}
	attachment := sm.currentRuntimeStorageAttachment(execution.ID)
	if sm.onRuntimeStorageDetached != nil {
		sm.onRuntimeStorageDetached(execution, attachment)
	}
}

func (sm *StreamManager) monitorRuntimeStorageAdvisory(ctx context.Context, execution *AgentExecution) {
	interval := sm.runtimeStorageProbeInterval
	if interval <= 0 {
		interval = defaultRuntimeStorageAdvisoryInterval
	}
	for {
		if ctx.Err() != nil {
			return
		}
		if !waitRuntimeStorageAdvisory(ctx, interval) {
			return
		}
		if sm.isExecutionCurrent != nil && !sm.isExecutionCurrent(execution) {
			return
		}
		sm.probeRuntimeStorageAdvisory(ctx, execution)
	}
}

func waitRuntimeStorageAdvisory(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (sm *StreamManager) probeRuntimeStorageAdvisory(ctx context.Context, execution *AgentExecution) {
	if sm.onRuntimeStorageProbe == nil {
		return
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		return
	}
	defer releaseClient()
	if sm.runtimeStorageSlots != nil {
		select {
		case sm.runtimeStorageSlots <- struct{}{}:
			defer func() { <-sm.runtimeStorageSlots }()
		case <-ctx.Done():
			return
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	snapshot, err := client.RuntimeStorage(probeCtx)
	cancel()
	if ctx.Err() != nil {
		return
	}
	result := RuntimeStorageProbeResult{Available: false, Generation: execution.DeliveryHarnessGeneration,
		attachment: sm.currentRuntimeStorageAttachment(execution.ID)}
	if err == nil && snapshot != nil && !snapshot.Unavailable && snapshot.Filesystem != nil {
		total := snapshot.Filesystem.TotalBytes
		available := snapshot.Filesystem.AvailableBytes
		result.Available = true
		result.StorageKind = snapshot.StorageKind
		result.TotalBytes = &total
		result.AvailableBytes = &available
		result.ObservedAt = snapshot.Filesystem.ObservedAt
	}
	sm.onRuntimeStorageProbe(ctx, execution, result)
}

func (sm *StreamManager) currentRuntimeStorageAttachment(executionID string) *deliveryStreamAttachment {
	current, loaded := sm.deliveryAttachments.Load(executionID)
	if !loaded {
		return nil
	}
	attachment, ok := current.(*deliveryStreamAttachment)
	if !ok || attachment.isClosed() {
		return nil
	}
	return attachment
}
