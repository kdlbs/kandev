package lifecycle

import (
	"math"
	"sync/atomic"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// streamProjectionProgress tracks the second, durable projection queue, whose
// handler can remain blocked after the transport queue has handed events off.
type streamProjectionProgress struct {
	sequence uint64
	enqueued atomic.Uint64
	done     atomic.Uint64
	lastDone atomic.Int64
}

func (p *streamProjectionProgress) completed(count uint64) {
	p.done.Add(count)
	p.lastDone.Store(time.Now().UnixNano())
}

func (execution *AgentExecution) installStreamProgress(next *streamProjectionProgress) {
	for {
		current := execution.streamProgress.Load()
		if current != nil && current.sequence >= next.sequence {
			return
		}
		if execution.streamProgress.CompareAndSwap(current, next) {
			return
		}
	}
}

func tryUpdateStreamStageSnapshot(execution *AgentExecution) (client.UpdateStreamStageSnapshot, bool) {
	if execution == nil || !execution.agentctlLifecycleMu.TryRLock() {
		return client.UpdateStreamStageSnapshot{}, false
	}
	defer execution.agentctlLifecycleMu.RUnlock()
	streamClient := execution.currentAgentCtlClient()
	if streamClient == nil {
		return client.UpdateStreamStageSnapshot{}, false
	}
	return streamClient.UpdateStreamStage()
}

func freshUpstreamHeartbeat(snapshot client.UpdateStreamStageSnapshot) bool {
	return snapshot.HeartbeatAgeMillis >= 0 && snapshot.HeartbeatAgeMillis <= 30_000
}

func upstreamStageAge(snapshot client.UpdateStreamStageSnapshot, ageAtHeartbeat int64) int64 {
	if !freshUpstreamHeartbeat(snapshot) || ageAtHeartbeat < 0 {
		return -1
	}
	return snapshot.HeartbeatAgeMillis + ageAtHeartbeat
}

// upstreamStageCount reports a heartbeat counter, or -1 without a fresh heartbeat.
func upstreamStageCount(snapshot client.UpdateStreamStageSnapshot, count uint64) int64 {
	if !freshUpstreamHeartbeat(snapshot) {
		return -1
	}
	return int64(min(count, uint64(math.MaxInt64)))
}

type streamStageDiagnostic struct {
	Available                bool   `json:"available"`
	Connected                bool   `json:"connected"`
	AdapterSeenCount         int64  `json:"adapter_seen_count"`
	AdapterAgeMillis         int64  `json:"adapter_age_ms"`
	WriterSentCount          int64  `json:"writer_sent_count"`
	WriterAgeMillis          int64  `json:"writer_age_ms"`
	ReadAgeMillis            int64  `json:"read_age_ms"`
	EnqueueAgeMillis         int64  `json:"projection_enqueue_age_ms"`
	HeartbeatAgeMillis       int64  `json:"heartbeat_age_ms"`
	ReadCount                uint64 `json:"read_count"`
	EnqueueCount             uint64 `json:"projection_enqueue_count"`
	ReaderPendingCount       uint64 `json:"reader_pending_count"`
	ProjectionAvailable      bool   `json:"projection_available"`
	ProjectionAgeMillis      int64  `json:"projection_completed_age_ms"`
	ProjectionCompletedCount uint64 `json:"projection_completed_count"`
	ProjectionPendingCount   uint64 `json:"projection_pending_count"`
}

func captureStreamStages(execution *AgentExecution) *streamStageDiagnostic {
	snapshot, available := tryUpdateStreamStageSnapshot(execution)
	adapterAge, writerAge := int64(-1), int64(-1)
	adapterSeen, writerSent := int64(-1), int64(-1)
	readAge, handledAge, heartbeatAge := int64(-1), int64(-1), int64(-1)
	if available {
		adapterAge = upstreamStageAge(snapshot, snapshot.AgentctlStage.AdapterAgeMillis)
		writerAge = upstreamStageAge(snapshot, snapshot.AgentctlStage.WriterAgeMillis)
		adapterSeen = upstreamStageCount(snapshot, snapshot.AgentctlStage.AdapterSeen)
		writerSent = upstreamStageCount(snapshot, snapshot.AgentctlStage.WriterSent)
		readAge, handledAge, heartbeatAge = snapshot.LastReadAgeMillis, snapshot.LastHandledAgeMillis, snapshot.HeartbeatAgeMillis
	}
	projectionPending, projectionDone := uint64(0), uint64(0)
	projectionAge := int64(-1)
	projectionAvailable := false
	if progress := execution.streamProgress.Load(); progress != nil {
		projectionAvailable = true
		enqueued := progress.enqueued.Load()
		projectionDone = progress.done.Load()
		projectionPending = enqueued - min(enqueued, projectionDone)
		if last := progress.lastDone.Load(); last != 0 {
			projectionAge = max(0, time.Since(time.Unix(0, last)).Milliseconds())
		}
	}
	return &streamStageDiagnostic{
		Available: available, Connected: available && snapshot.Connected,
		AdapterSeenCount: adapterSeen, AdapterAgeMillis: adapterAge,
		WriterSentCount: writerSent, WriterAgeMillis: writerAge, ReadAgeMillis: readAge,
		EnqueueAgeMillis: handledAge, HeartbeatAgeMillis: heartbeatAge,
		ReadCount: snapshot.ReadCount, EnqueueCount: snapshot.HandledCount, ReaderPendingCount: snapshot.Pending,
		ProjectionAvailable: projectionAvailable, ProjectionAgeMillis: projectionAge,
		ProjectionCompletedCount: projectionDone, ProjectionPendingCount: projectionPending,
	}
}
