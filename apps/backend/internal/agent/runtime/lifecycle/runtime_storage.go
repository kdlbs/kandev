package lifecycle

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// Advisory storage states. They describe a source observation and grant no
// process, prompt, or recovery authority.
const (
	RuntimeStorageStateWarning     = "warning"
	RuntimeStorageStateNormal      = "normal"
	RuntimeStorageStateUnavailable = "unavailable"
)

// Advisory thresholds. Between the warning and clearance thresholds the
// previous state is retained to prevent flicker.
const (
	RuntimeStorageWarningPercent   = 90.0
	RuntimeStorageClearPercent     = 85.0
	runtimeStorageExecutorLocation = "executor_runtime"
	runtimeStorageLocalLocation    = "local_runtime"
	// runtimeStorageMaximumWireEpoch keeps the browser epoch a JavaScript safe
	// integer. Unix nanoseconds exceed Number.MAX_SAFE_INTEGER in production.
	runtimeStorageMaximumWireEpoch = uint64(1<<53 - 1)
)

// RuntimeStorageObservation is the bounded, runtime-only advisory projection
// for one session. Measurements remain nil when unknown.
type RuntimeStorageObservation struct {
	ExecutionID    string                    `json:"execution_id,omitempty"`
	TaskID         string                    `json:"task_id"`
	SessionID      string                    `json:"session_id"`
	State          string                    `json:"state"`
	StorageKind    string                    `json:"storage_kind,omitempty"`
	Location       string                    `json:"location"`
	ExecutorLabel  string                    `json:"executor_label,omitempty"`
	TotalBytes     *int64                    `json:"total_bytes,omitempty"`
	AvailableBytes *int64                    `json:"available_bytes,omitempty"`
	ObservedAt     *time.Time                `json:"observed_at,omitempty"`
	AgeSeconds     int64                     `json:"age_seconds"`
	WritePressure  bool                      `json:"write_pressure,omitempty"`
	ProducerPaused bool                      `json:"producer_paused,omitempty"`
	DeliveryState  string                    `json:"delivery_state,omitempty"`
	FailureStorage *models.AgentErrorStorage `json:"failure_storage,omitempty"`
	Revision       uint64                    `json:"revision"`
	Epoch          uint64                    `json:"epoch"`

	// ReceivedAt is the backend receipt time. It is never serialized; freshness
	// travels as AgeSeconds so browser and executor clocks cannot decide it.
	ReceivedAt time.Time `json:"-"`
	// execution and attachment are the immutable owners that produced this
	// observation.
	execution  *AgentExecution           `json:"-"`
	attachment *deliveryStreamAttachment `json:"-"`
}

// RuntimeStorageProbeResult is one source sample, valid or unavailable.
type RuntimeStorageProbeResult struct {
	StorageKind    string
	TotalBytes     *int64
	AvailableBytes *int64
	ObservedAt     time.Time
	Generation     uint64
	Available      bool
	attachment     *deliveryStreamAttachment
}

// RuntimeStoragePressure is the owner-correlated write-pressure projection.
// It is independent of the percentage advisory and never establishes prompt
// outcome or replay authority.
type RuntimeStoragePressure struct {
	WritePressure  bool
	ProducerPaused bool
	DeliveryState  string
	FailureStorage *models.AgentErrorStorage
	Generation     uint64
	attachment     *deliveryStreamAttachment
}

type runtimeStorageProbeState struct {
	mu           sync.Mutex
	observations map[string]RuntimeStorageObservation
	runtimeEpoch uint64
	nextRevision uint64
}

func newRuntimeStorageProbeState() *runtimeStorageProbeState {
	epoch := uint64(time.Now().UnixMilli())
	if epoch > runtimeStorageMaximumWireEpoch {
		epoch = runtimeStorageMaximumWireEpoch
	}
	return &runtimeStorageProbeState{
		observations: make(map[string]RuntimeStorageObservation),
		runtimeEpoch: epoch,
	}
}

// RuntimeStorageObservation returns a copy of the latest accepted advisory for
// one session. A replacement execution invalidates the cached entry rather
// than exposing its predecessor's warning as current.
func (m *Manager) RuntimeStorageObservation(sessionID string) (RuntimeStorageObservation, bool) {
	if m == nil || m.runtimeStorage == nil || sessionID == "" {
		return RuntimeStorageObservation{}, false
	}
	m.runtimeStorage.mu.Lock()
	defer m.runtimeStorage.mu.Unlock()
	observation, ok := m.runtimeStorage.observations[sessionID]
	if !ok {
		return RuntimeStorageObservation{}, false
	}
	if !m.runtimeStorageExecutionCurrent(observation.execution) ||
		!m.runtimeStorageAttachmentCurrent(observation.execution, observation.attachment) {
		delete(m.runtimeStorage.observations, sessionID)
		return RuntimeStorageObservation{}, false
	}
	return copyRuntimeStorageObservation(observation), true
}

// applyRuntimeStorageProbe fuses one source sample with the previous advisory
// using the documented hysteresis. It publishes only accepted changes and a
// bounded warning refresh, never a healthy poll.
func (m *Manager) applyRuntimeStorageProbe(
	ctx context.Context,
	execution *AgentExecution,
	probe RuntimeStorageProbeResult,
) {
	if m == nil || m.runtimeStorage == nil || execution == nil || execution.SessionID == "" {
		return
	}
	state := m.runtimeStorage
	state.mu.Lock()
	previous, hadPrevious := state.observations[execution.SessionID]
	if hadPrevious && previous.execution != execution {
		previous = RuntimeStorageObservation{}
		hadPrevious = false
	}
	probe.attachment = firstAttachment(probe.attachment, previous.attachment)
	if !m.runtimeStorageProbeOwnerCurrent(execution, probe) {
		state.mu.Unlock()
		return
	}
	observation := buildRuntimeStorageObservation(execution, probe, previous, hadPrevious, state)
	changed := !hadPrevious || previous.State != observation.State
	refresh := observation.State == RuntimeStorageStateWarning || observation.WritePressure
	if changed || refresh {
		state.nextRevision++
		observation.Revision = state.nextRevision
		state.observations[execution.SessionID] = observation
	} else {
		// Preserve the published revision while keeping the latest non-warning
		// measurement available to a fresh page load.
		observation.Revision = previous.Revision
		state.observations[execution.SessionID] = observation
	}
	state.mu.Unlock()
	if !changed && !refresh {
		return
	}
	m.publishRuntimeStorageObservation(ctx, observation)
}

// applyRuntimeStoragePressure updates the live write-pressure projection. It
// is separate from the percentage sample so a blocked writer below 90% is
// still visible and never claims that work continues.
func (m *Manager) applyRuntimeStoragePressure(
	ctx context.Context,
	execution *AgentExecution,
	pressure RuntimeStoragePressure,
) {
	if m == nil || m.runtimeStorage == nil || execution == nil || execution.SessionID == "" {
		return
	}
	pressure.FailureStorage = stampStorageEvidence(pressure.FailureStorage, execution)
	state := m.runtimeStorage
	state.mu.Lock()
	if !m.runtimeStorageExecutionCurrentLocked(execution) {
		state.mu.Unlock()
		return
	}
	previous, hadPrevious := state.observations[execution.SessionID]
	if hadPrevious && previous.execution != execution {
		previous = RuntimeStorageObservation{}
		hadPrevious = false
	}
	pressure.attachment = firstAttachment(pressure.attachment, previous.attachment)
	if !m.runtimeStorageAttachmentCurrent(execution, pressure.attachment) ||
		(pressure.Generation != 0 && pressure.Generation != execution.DeliveryHarnessGeneration) {
		state.mu.Unlock()
		return
	}
	observation := runtimeStoragePressureObservation(execution, previous, hadPrevious, state, pressure)
	changed := !hadPrevious || runtimeStoragePressureChanged(previous, observation)
	if !changed {
		state.mu.Unlock()
		return
	}
	state.nextRevision++
	observation.Revision = state.nextRevision
	state.observations[execution.SessionID] = observation
	state.mu.Unlock()
	m.publishRuntimeStorageObservation(ctx, observation)
}

func runtimeStoragePressureObservation(
	execution *AgentExecution,
	previous RuntimeStorageObservation,
	hadPrevious bool,
	state *runtimeStorageProbeState,
	pressure RuntimeStoragePressure,
) RuntimeStorageObservation {
	observation := previous
	if !hadPrevious {
		observation = RuntimeStorageObservation{
			ExecutionID:   execution.ID,
			TaskID:        execution.TaskID,
			SessionID:     execution.SessionID,
			State:         RuntimeStorageStateUnavailable,
			Location:      runtimeStorageLocation(execution),
			ExecutorLabel: runtimeStorageExecutorLabel(execution),
			Epoch:         state.runtimeEpoch,
		}
	}
	observation.execution = execution
	observation.attachment = pressure.attachment
	observation.ExecutionID = execution.ID
	observation.TaskID = execution.TaskID
	observation.SessionID = execution.SessionID
	observation.Location = runtimeStorageLocation(execution)
	observation.ExecutorLabel = runtimeStorageExecutorLabel(execution)
	observation.WritePressure = pressure.WritePressure
	observation.ProducerPaused = pressure.ProducerPaused
	observation.DeliveryState = pressure.DeliveryState
	observation.FailureStorage = pressure.FailureStorage
	return observation
}

func runtimeStoragePressureChanged(previous, next RuntimeStorageObservation) bool {
	return previous.WritePressure != next.WritePressure ||
		previous.ProducerPaused != next.ProducerPaused ||
		previous.DeliveryState != next.DeliveryState ||
		!models.AgentErrorStorageEqual(previous.FailureStorage, next.FailureStorage)
}

func buildRuntimeStorageObservation(
	execution *AgentExecution,
	probe RuntimeStorageProbeResult,
	previous RuntimeStorageObservation,
	hadPrevious bool,
	state *runtimeStorageProbeState,
) RuntimeStorageObservation {
	observation := RuntimeStorageObservation{
		ExecutionID:   execution.ID,
		TaskID:        execution.TaskID,
		SessionID:     execution.SessionID,
		State:         RuntimeStorageStateUnavailable,
		Location:      runtimeStorageLocation(execution),
		ExecutorLabel: runtimeStorageExecutorLabel(execution),
		Epoch:         state.runtimeEpoch,
		execution:     execution,
		attachment:    probe.attachment,
	}
	// Pressure evidence survives a capacity sample.
	observation.WritePressure = previous.WritePressure
	observation.ProducerPaused = previous.ProducerPaused
	observation.DeliveryState = previous.DeliveryState
	observation.FailureStorage = previous.FailureStorage
	if !probe.Available {
		// A failed sample never invents capacity or clears the last cause.
		observation.State = previous.State
		if observation.State == "" {
			observation.State = RuntimeStorageStateUnavailable
		}
		observation.StorageKind = previous.StorageKind
		observation.TotalBytes = copyInt64(previous.TotalBytes)
		observation.AvailableBytes = copyInt64(previous.AvailableBytes)
		observation.ObservedAt = copyTime(previous.ObservedAt)
		observation.ReceivedAt = previous.ReceivedAt
		return observation
	}
	observation.StorageKind = probe.StorageKind
	observation.TotalBytes = copyInt64(probe.TotalBytes)
	observation.AvailableBytes = copyInt64(probe.AvailableBytes)
	observedAt := probe.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	observation.ObservedAt = &observedAt
	observation.ReceivedAt = time.Now().UTC()
	observation.State = runtimeStorageStateAfterProbe(observation, previous, hadPrevious)
	return observation
}

func runtimeStorageStateAfterProbe(
	observation RuntimeStorageObservation,
	previous RuntimeStorageObservation,
	hadPrevious bool,
) string {
	usage, known := runtimeStorageUsagePercent(observation.TotalBytes, observation.AvailableBytes)
	if !known {
		if hadPrevious && previous.State != "" {
			return previous.State
		}
		return RuntimeStorageStateUnavailable
	}
	switch {
	case usage >= RuntimeStorageWarningPercent:
		return RuntimeStorageStateWarning
	case usage < RuntimeStorageClearPercent:
		return RuntimeStorageStateNormal
	case hadPrevious && previous.State != "":
		return previous.State
	default:
		return RuntimeStorageStateNormal
	}
}

func runtimeStorageUsagePercent(total, available *int64) (float64, bool) {
	if total == nil || available == nil || *total <= 0 || *available < 0 || *available > *total {
		return 0, false
	}
	used := *total - *available
	return float64(used) / float64(*total) * 100, true
}

// runtimeStorageAgeSeconds is the backend receipt age used by the browser
// instead of comparing source and browser clocks.
func runtimeStorageAgeSeconds(receivedAt, now time.Time) int64 {
	if receivedAt.IsZero() || now.Before(receivedAt) {
		return 0
	}
	return int64(now.Sub(receivedAt).Seconds())
}

func (m *Manager) runtimeStorageExecutionCurrent(execution *AgentExecution) bool {
	if execution == nil {
		return false
	}
	if m.executionStore == nil {
		return true
	}
	current, ok := m.executionStore.Get(execution.ID)
	return ok && current == execution
}

func (m *Manager) runtimeStorageExecutionCurrentLocked(execution *AgentExecution) bool {
	return m.runtimeStorageExecutionCurrent(execution)
}

func (m *Manager) runtimeStorageProbeOwnerCurrent(
	execution *AgentExecution,
	probe RuntimeStorageProbeResult,
) bool {
	return m.runtimeStorageExecutionCurrentLocked(execution) &&
		m.runtimeStorageAttachmentCurrent(execution, probe.attachment) &&
		(probe.Generation == 0 || probe.Generation == execution.DeliveryHarnessGeneration)
}

func (m *Manager) runtimeStorageAttachmentCurrent(
	execution *AgentExecution,
	attachment *deliveryStreamAttachment,
) bool {
	if attachment == nil || m.streamManager == nil || execution == nil {
		return true
	}
	current, ok := m.streamManager.deliveryAttachments.Load(execution.ID)
	return ok && current == attachment && !attachment.isClosed()
}

func firstAttachment(values ...*deliveryStreamAttachment) *deliveryStreamAttachment {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func runtimeStorageLocation(execution *AgentExecution) string {
	if execution == nil {
		return "unknown"
	}
	switch execution.ExecutorType {
	case "", string(models.ExecutorTypeLocal), string(models.ExecutorTypeWorktree), "local_pc":
		return runtimeStorageLocalLocation
	default:
		return runtimeStorageExecutorLocation
	}
}

func runtimeStorageExecutorLabel(execution *AgentExecution) string {
	if execution == nil {
		return ""
	}
	label := execution.ExecutorType
	if label == "" {
		label = string(execution.RuntimeName)
	}
	return label
}

func copyRuntimeStorageObservation(observation RuntimeStorageObservation) RuntimeStorageObservation {
	observation.TotalBytes = copyInt64(observation.TotalBytes)
	observation.AvailableBytes = copyInt64(observation.AvailableBytes)
	observation.ObservedAt = copyTime(observation.ObservedAt)
	if observation.FailureStorage != nil {
		copied := *observation.FailureStorage
		observation.FailureStorage = &copied
	}
	return observation
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

// GetSessionID satisfies the gateway's session routing contract without
// widening the payload into a map.
func (o RuntimeStorageObservation) GetSessionID() string { return o.SessionID }

// publishRuntimeStorageObservation stamps receipt age and publishes.
func (m *Manager) publishRuntimeStorageObservation(ctx context.Context, observation RuntimeStorageObservation) {
	observation.AgeSeconds = runtimeStorageAgeSeconds(observation.ReceivedAt, time.Now().UTC())
	m.eventPublisher.PublishRuntimeStorage(ctx, observation)
}

// modelStorageDiagnostic converts validated source evidence into the bounded
// persisted cause shape.
func modelStorageDiagnostic(diagnostic *streams.DeliveryStorageDiagnostic) *models.AgentErrorStorage {
	if diagnostic == nil {
		return nil
	}
	normalized := diagnostic.Normalize()
	if normalized == nil {
		return nil
	}
	storage := &models.AgentErrorStorage{
		Reason:             normalized.Reason,
		DiskAvailableBytes: normalized.DiskAvailableBytes,
		DiskRequiredBytes:  normalized.DiskRequiredBytes,
		RetainedBytes:      normalized.RetainedBytes,
		LimitBytes:         normalized.LimitBytes,
		EventBytes:         normalized.EventBytes,
	}
	if !normalized.ObservedAt.IsZero() {
		observedAt := normalized.ObservedAt.UTC()
		storage.ObservedAt = &observedAt
	}
	return models.NormalizeAgentErrorStorage(storage)
}

// retireRuntimeStorageObservation removes a cached advisory when its owning
// execution or stream attachment ends. A nil attachment retires only entries
// for that execution; a non-nil attachment must match as well.
func (m *Manager) retireRuntimeStorageObservation(
	execution *AgentExecution,
	attachment *deliveryStreamAttachment,
) {
	if m == nil || m.runtimeStorage == nil || execution == nil {
		return
	}
	state := m.runtimeStorage
	state.mu.Lock()
	defer state.mu.Unlock()
	observation, ok := state.observations[execution.SessionID]
	if !ok || observation.execution != execution {
		return
	}
	if attachment != nil && observation.attachment != attachment {
		return
	}
	delete(state.observations, execution.SessionID)
}
