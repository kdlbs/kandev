package lifecycle

import (
	"math"
	"sync"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// streamProjectionProgress tracks the durable projection queue for one
// accepted update-stream connection.
type streamProjectionProgress struct {
	ownerID uint64
	tracker *streamProjectionTracker
}

type streamProjectionTracker struct {
	mu                 sync.Mutex
	currentOwnerID     uint64
	pending            uint64
	predecessorPending uint64
	enqueued           uint64
	completed          uint64
	lastEnqueued       int64
	lastCompleted      int64
}

type streamProjectionSnapshot struct {
	available    bool
	enqueued     uint64
	completed    uint64
	pending      uint64
	enqueueAge   int64
	completedAge int64
}

func (execution *AgentExecution) streamProjectionTracker() *streamProjectionTracker {
	if tracker := execution.streamProjection.Load(); tracker != nil {
		return tracker
	}
	tracker := &streamProjectionTracker{}
	if execution.streamProjection.CompareAndSwap(nil, tracker) {
		return tracker
	}
	return execution.streamProjection.Load()
}

func (tracker *streamProjectionTracker) install(ownerID uint64) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.installLocked(ownerID)
}

func (tracker *streamProjectionTracker) installLocked(ownerID uint64) {
	if ownerID == 0 || ownerID <= tracker.currentOwnerID {
		return
	}
	tracker.currentOwnerID = ownerID
	tracker.predecessorPending = tracker.pending
	tracker.enqueued = 0
	tracker.completed = 0
	tracker.lastEnqueued = 0
	tracker.lastCompleted = 0
}

func (progress *streamProjectionProgress) enqueued() {
	if progress == nil || progress.tracker == nil {
		return
	}
	tracker := progress.tracker
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if progress.ownerID > tracker.currentOwnerID {
		tracker.installLocked(progress.ownerID)
	}
	tracker.pending++
	if progress.ownerID == tracker.currentOwnerID {
		tracker.enqueued++
		tracker.lastEnqueued = time.Now().UnixNano()
	} else if progress.ownerID < tracker.currentOwnerID {
		tracker.predecessorPending++
	}
}

func (progress *streamProjectionProgress) completed(count uint64) {
	if progress == nil || progress.tracker == nil || count == 0 {
		return
	}
	tracker := progress.tracker
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	settled := min(count, tracker.pending)
	tracker.pending -= settled
	if progress.ownerID == tracker.currentOwnerID {
		tracker.completed += settled
		tracker.lastCompleted = time.Now().UnixNano()
	} else if progress.ownerID < tracker.currentOwnerID {
		tracker.predecessorPending -= min(settled, tracker.predecessorPending)
	}
}

func (progress *streamProjectionProgress) snapshot() streamProjectionSnapshot {
	if progress == nil || progress.tracker == nil {
		return streamProjectionSnapshot{enqueueAge: -1, completedAge: -1}
	}
	tracker := progress.tracker
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	snapshot := streamProjectionSnapshot{
		available:    progress.ownerID != 0 && progress.ownerID == tracker.currentOwnerID && tracker.predecessorPending == 0,
		enqueued:     tracker.enqueued,
		completed:    tracker.completed,
		pending:      tracker.enqueued - min(tracker.enqueued, tracker.completed),
		enqueueAge:   stageAgeMillis(tracker.lastEnqueued),
		completedAge: stageAgeMillis(tracker.lastCompleted),
	}
	return snapshot
}

func stageAgeMillis(at int64) int64 {
	if at == 0 {
		return -1
	}
	return max(0, time.Since(time.Unix(0, at)).Milliseconds())
}

func (execution *AgentExecution) installStreamProgress(next *streamProjectionProgress) {
	if execution == nil || next == nil || next.ownerID == 0 {
		return
	}
	if next.tracker == nil {
		next.tracker = execution.streamProjectionTracker()
	}
	for {
		current := execution.streamProgress.Load()
		if current != nil && current.ownerID >= next.ownerID {
			return
		}
		if execution.streamProgress.CompareAndSwap(current, next) {
			next.tracker.install(next.ownerID)
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
	readAge, heartbeatAge := int64(-1), int64(-1)
	if available {
		adapterAge = upstreamStageAge(snapshot, snapshot.AgentctlStage.AdapterAgeMillis)
		writerAge = upstreamStageAge(snapshot, snapshot.AgentctlStage.WriterAgeMillis)
		adapterSeen = upstreamStageCount(snapshot, snapshot.AgentctlStage.AdapterSeen)
		writerSent = upstreamStageCount(snapshot, snapshot.AgentctlStage.WriterSent)
		readAge, heartbeatAge = snapshot.LastReadAgeMillis, snapshot.HeartbeatAgeMillis
	}
	projection := streamProjectionSnapshot{enqueueAge: -1, completedAge: -1}
	if progress := execution.streamProgress.Load(); progress != nil && (!available || progress.ownerID == snapshot.OwnerID) {
		projection = progress.snapshot()
	}
	return &streamStageDiagnostic{
		Available: available, Connected: available && snapshot.Connected,
		AdapterSeenCount: adapterSeen, AdapterAgeMillis: adapterAge,
		WriterSentCount: writerSent, WriterAgeMillis: writerAge, ReadAgeMillis: readAge,
		EnqueueAgeMillis: projection.enqueueAge, HeartbeatAgeMillis: heartbeatAge,
		ReadCount: snapshot.ReadCount, EnqueueCount: projection.enqueued, ReaderPendingCount: snapshot.Pending,
		ProjectionAvailable: projection.available, ProjectionAgeMillis: projection.completedAge,
		ProjectionCompletedCount: projection.completed, ProjectionPendingCount: projection.pending,
	}
}
