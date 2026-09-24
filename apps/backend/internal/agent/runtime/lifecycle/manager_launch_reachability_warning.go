package lifecycle

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// ReachabilityReader is the narrow read accessor launchBuildExecutorRequest
// uses to look up an ssh executor's stored reachability record immediately
// before calling CreateInstance. It is declared here — not imported from
// executors/reachability — because that package already imports lifecycle
// for SSH target resolution and dial classification; importing it back here
// would cycle. In production this is satisfied directly by the task
// repository, which already implements this exact method for the
// reachability HTTP routes (see internal/ssh.ReachabilityLister).
type ReachabilityReader interface {
	GetExecutorReachability(ctx context.Context, executorID string) (*models.ExecutorReachability, error)
}

// SetSSHReachabilityWarningPolicy wires the launch-time session.launch.warning
// producer (see task-05-launch-non-gating.md). reader resolves an ssh
// executor's stored record; probingEnabled mirrors whether the reachability
// poller's periodic sweep is on; warningWindowSeconds is 3x the reachability
// package's own default interval, evaluated even with probing disabled.
// Leaving this unset (reader nil) disables the producer entirely.
func (m *Manager) SetSSHReachabilityWarningPolicy(reader ReachabilityReader, probingEnabled bool, warningWindowSeconds int) {
	m.reachabilityReader = reader
	m.reachabilityProbingEnabled = probingEnabled
	m.reachabilityWarningWindowSeconds = warningWindowSeconds
}

// LaunchWarningEventPayload is the session.launch.warning payload. It is
// published once, immediately before an SSH launch's CreateInstance call,
// when the target executor's stored reachability record is unreachable and
// still within the probing window.
type LaunchWarningEventPayload struct {
	TaskID        string     `json:"task_id"`
	SessionID     string     `json:"session_id"`
	ExecutorID    string     `json:"executor_id"`
	Host          string     `json:"host"`
	State         string     `json:"state"`
	Reason        string     `json:"reason"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	Timestamp     string     `json:"timestamp"`
}

// sshLaunchWarningReadTimeout bounds the best-effort record lookup started by
// a launch. The lookup must not hold up CreateInstance, but it also must not
// leave a database call running forever after a launch has finished.
const sshLaunchWarningReadTimeout = 2 * time.Second

// GetSessionID satisfies the session-routing accessor
// internal/gateway/websocket.extractSessionID uses to broadcast this event
// to the correct session's subscribers.
func (p LaunchWarningEventPayload) GetSessionID() string {
	return p.SessionID
}

// PublishLaunchWarning publishes a session.launch.warning event to the
// launched session's own event stream.
func (p *EventPublisher) PublishLaunchWarning(sessionID string, payload *LaunchWarningEventPayload) {
	if p.eventBus == nil {
		return
	}
	if payload.Timestamp == "" {
		payload.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	subject := events.BuildSessionLaunchWarningSubject(sessionID)
	event := bus.NewEvent(events.SessionLaunchWarning, "lifecycle", payload)
	if err := p.eventBus.Publish(context.Background(), subject, event); err != nil {
		p.logger.Error("failed to publish session launch warning event",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// scheduleSSHLaunchWarning starts the best-effort record lookup without
// delaying CreateInstance. The request and metadata values used by the
// goroutine are copied because the launch path may reuse or mutate its input
// maps after it returns.
func (m *Manager) scheduleSSHLaunchWarning(ctx context.Context, req *LaunchRequest, metadata map[string]interface{}, sessionID string) {
	if req == nil || req.ExecutorType != string(models.ExecutorTypeSSH) ||
		m.reachabilityReader == nil || m.eventPublisher == nil {
		return
	}
	warningReq := *req
	warningMetadata := map[string]interface{}{
		"executor_id": getMetadataString(metadata, "executor_id"),
	}
	warningCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sshLaunchWarningReadTimeout)
	go func() {
		defer cancel()
		m.maybePublishSSHLaunchWarning(warningCtx, &warningReq, warningMetadata, sessionID)
	}()
}

// maybePublishSSHLaunchWarning reads the target executor's reachability
// record and, when it is unreachable and still warning-eligible, publishes
// exactly one session.launch.warning. It is a strict read-only accessor call,
// made from launchBuildExecutorRequest alongside its single CreateInstance
// call site, never from inside CreateInstance itself. A read failure or a
// missing record produces no warning.
func (m *Manager) maybePublishSSHLaunchWarning(ctx context.Context, req *LaunchRequest, metadata map[string]interface{}, sessionID string) {
	if req == nil || req.ExecutorType != string(models.ExecutorTypeSSH) ||
		m.reachabilityReader == nil || m.eventPublisher == nil {
		return
	}
	executorID := strings.TrimSpace(getMetadataString(metadata, "executor_id"))
	if executorID == "" {
		return
	}
	record, err := m.reachabilityReader.GetExecutorReachability(ctx, executorID)
	if err != nil || record == nil || record.State != models.ExecutorReachabilityStateUnreachable {
		return
	}
	if !m.reachabilityProbingEnabled && !withinReachabilityWarningWindow(record.CheckedAt, m.reachabilityWarningWindowSeconds) {
		return
	}
	m.eventPublisher.PublishLaunchWarning(sessionID, &LaunchWarningEventPayload{
		TaskID:        req.TaskID,
		SessionID:     sessionID,
		ExecutorID:    executorID,
		Host:          record.Host,
		State:         string(record.State),
		Reason:        string(record.Reason),
		LastSuccessAt: record.LastSuccessAt,
	})
}

// withinReachabilityWarningWindow reports whether checkedAt is recent enough
// to still warn even with periodic probing disabled. A nil checkedAt (never
// probed) or a non-positive window never qualifies.
func withinReachabilityWarningWindow(checkedAt *time.Time, windowSeconds int) bool {
	if checkedAt == nil || windowSeconds <= 0 {
		return false
	}
	return time.Since(*checkedAt) <= time.Duration(windowSeconds)*time.Second
}
