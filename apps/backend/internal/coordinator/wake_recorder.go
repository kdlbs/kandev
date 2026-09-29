package coordinator

import (
	"context"
	"errors"
	"expvar"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// StallWakeHook records the stall wakes of a task whose stall row was just
// upserted.
type StallWakeHook func(ctx context.Context, workspaceID, taskID string)

// Drop reasons of coordinator_wake_dropped_total; the set is closed.
const (
	dropReasonCap         = "cap"
	dropReasonAutonomyOff = "autonomy_off"
	dropReasonNotOwn      = "not_own"
	dropReasonNotFound    = "not_found"
	dropReasonReadError   = "read_error"
	dropReasonWriteError  = "write_error"
)

var (
	wakeRecordedTotal    = expvar.NewMap("coordinator_wake_recorded_total")
	wakeDroppedTotal     = expvar.NewMap("coordinator_wake_dropped_total")
	backstopSkippedTotal = expvar.NewInt("coordinator_backstop_skipped_total")
)

func expvarMapValue(m *expvar.Map, key string) int64 {
	if v, ok := m.Get(key).(*expvar.Int); ok {
		return v.Value()
	}
	return 0
}

func wakeRecordedCounter(kind WakeKind) int64 { return expvarMapValue(wakeRecordedTotal, string(kind)) }

func wakeDroppedCounter(reason string) int64 { return expvarMapValue(wakeDroppedTotal, reason) }

// SetStallWakeHook registers the hook the stall subscriber calls after a stall
// row changed; nil clears it.
func (s *Service) SetStallWakeHook(hook StallWakeHook) {
	s.wakeMu.Lock()
	s.stallWakeHook = hook
	s.wakeMu.Unlock()
}

// enterWake registers one recorder handler or hook in flight and returns the
// wake sources. It refuses once StopWakeRecorder ran or no sources are set.
func (s *Service) enterWake() (WakeSources, func(), bool) {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	if s.recorderStopped || s.wakeSources == nil {
		return nil, nil, false
	}
	s.wakeInFlight.Add(1)
	return s.wakeSources, s.wakeInFlight.Done, true
}

// runStallWakeHook calls the stall hook, if set, while registered in flight. A
// panic is recovered and logged.
func (s *Service) runStallWakeHook(ctx context.Context, workspaceID, taskID string) {
	s.wakeMu.Lock()
	hook := s.stallWakeHook
	if hook == nil || s.recorderStopped {
		s.wakeMu.Unlock()
		return
	}
	s.wakeInFlight.Add(1)
	s.wakeMu.Unlock()
	defer s.wakeInFlight.Done()
	defer s.recoverWake("stall hook")
	hook(ctx, workspaceID, taskID)
}

func (s *Service) recoverWake(what string) {
	if r := recover(); r != nil {
		s.logger.Error("coordinator wake recorder panicked", zap.String("where", what), zap.Any("panic", r))
	}
}

// StartWakeRecorder subscribes the recorder to its three event sources and
// installs the stall hook. The Service owns the subscriptions; a subscribe
// failure is logged and the backstop covers the missing source.
func (s *Service) StartWakeRecorder(eventBus bus.EventBus, src WakeSources) {
	s.wakeMu.Lock()
	s.wakeSources = src
	s.wakeMu.Unlock()
	subs := []struct {
		subject string
		handle  func(context.Context, *bus.Event)
	}{
		{events.SessionPendingActionChanged, s.onPendingActionChanged},
		{events.TaskSessionErrorChanged, s.onSessionErrorChanged},
		{events.TaskStateChanged, s.onTaskStateChanged},
	}
	for _, sub := range subs {
		subscription, err := eventBus.Subscribe(sub.subject, s.wakeEventHandler(sub.handle))
		if err != nil {
			s.logger.Warn("wake recorder subscription failed", zap.String("subject", sub.subject), zap.Error(err))
			continue
		}
		s.keepRecorderSubscription(subscription)
	}
	s.SetStallWakeHook(s.recordStallWake)
}

func (s *Service) keepRecorderSubscription(sub bus.Subscription) {
	s.wakeMu.Lock()
	stopped := s.recorderStopped
	if !stopped {
		s.recorderSubs = append(s.recorderSubs, sub)
	}
	s.wakeMu.Unlock()
	if stopped {
		_ = sub.Unsubscribe()
	}
}

// StopWakeRecorder unsubscribes the recorder, clears the stall hook and waits
// for handlers and hook calls in flight. It is idempotent; a handler or hook
// that starts afterwards is a no-op. It never waits while holding the mutex.
func (s *Service) StopWakeRecorder() {
	s.wakeMu.Lock()
	s.recorderStopped = true
	subs := s.recorderSubs
	s.recorderSubs = nil
	s.stallWakeHook = nil
	s.wakeMu.Unlock()
	for _, sub := range subs {
		if err := sub.Unsubscribe(); err != nil {
			s.logger.Debug("wake recorder unsubscribe failed", zap.Error(err))
		}
	}
	s.wakeInFlight.Wait()
}

func (s *Service) wakeEventHandler(fn func(context.Context, *bus.Event)) bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		_, release, ok := s.enterWake()
		if !ok {
			return nil
		}
		defer release()
		defer s.recoverWake("event handler")
		fn(ctx, event)
		return nil
	}
}

func eventString(data map[string]any, key string) string {
	v, _ := data[key].(string)
	return strings.TrimSpace(v)
}

func eventData(event *bus.Event) (map[string]any, bool) {
	if event == nil {
		return nil, false
	}
	data, ok := event.Data.(map[string]any)
	return data, ok
}

func (s *Service) onPendingActionChanged(ctx context.Context, event *bus.Event) {
	data, ok := eventData(event)
	if !ok {
		return
	}
	var kind WakeKind
	switch eventString(data, "pending_action") {
	case "clarification":
		kind = WakeKindQuestion
	case "permission":
		kind = WakeKindPermission
	default:
		return
	}
	s.recordEpisodeEvent(ctx, eventString(data, "task_id"), eventString(data, "session_id"), kind, true)
}

func (s *Service) onSessionErrorChanged(ctx context.Context, event *bus.Event) {
	data, ok := eventData(event)
	if !ok {
		return
	}
	if active, _ := data["active"].(bool); !active {
		return
	}
	s.recordEpisodeEvent(ctx, eventString(data, "task_id"), eventString(data, "session_id"), WakeKindError, true)
}

func (s *Service) onTaskStateChanged(ctx context.Context, event *bus.Event) {
	data, ok := eventData(event)
	if !ok || eventString(data, "state") != taskStateCompleted {
		return
	}
	s.recordEpisodeEvent(ctx, eventString(data, "task_id"), "", WakeKindCompleted, false)
}

func (s *Service) recordStallWake(ctx context.Context, _, taskID string) {
	if taskID == "" {
		return
	}
	s.recordEpisodeEvent(ctx, taskID, "", WakeKindStall, false)
}

// recordEpisodeEvent records the episode of one kind on taskID for every
// autonomous coordinator that owns it and watches its workflow. The event
// only says where to look: the key comes from stored state. A question,
// permission or error event must name a session, and it is ignored unless
// that session is the task's primary session.
func (s *Service) recordEpisodeEvent(ctx context.Context, taskID, sessionID string, kind WakeKind, needSession bool) {
	if taskID == "" || (needSession && sessionID == "") {
		s.logger.Debug("wake event without task or session id dropped", zap.String("kind", string(kind)))
		return
	}
	s.wakeMu.Lock()
	src := s.wakeSources
	s.wakeMu.Unlock()
	if src == nil {
		return
	}
	owners, err := s.store.OwnersOfTask(ctx, taskID)
	if err != nil {
		s.dropWakeRead(ctx, "", taskID, err)
		return
	}
	if len(owners) == 0 {
		return
	}
	if needSession && !s.isPrimarySession(ctx, src, taskID, sessionID) {
		return
	}
	s.recordForOwners(ctx, src, owners, taskID, kind)
}

// isPrimarySession reports whether sessionID is the task's primary session; a
// read failure drops the event.
func (s *Service) isPrimarySession(ctx context.Context, src WakeSources, taskID, sessionID string) bool {
	primary, err := src.PrimarySessionID(ctx, taskID)
	if err != nil {
		s.dropWakeRead(ctx, "", taskID, err)
		return false
	}
	return primary == sessionID
}

// recordForOwners reads the kind's episodes once and records them for every
// owner that watches the task's workflow.
func (s *Service) recordForOwners(ctx context.Context, src WakeSources, owners []OwningCoordinator, taskID string, kind WakeKind) {
	var episodes []wakeEpisode
	read := false
	for _, owner := range owners {
		set, err := s.EffectiveWatchSet(ctx, owner.CoordinatorID)
		if err != nil {
			s.dropWakeRead(ctx, owner.CoordinatorID, taskID, err)
			continue
		}
		if !set.Contains(owner.WorkflowID) {
			continue
		}
		if !read {
			episodes, err = s.readEpisodes(ctx, src, owner.WorkspaceID, taskID, []WakeKind{kind})
			if err != nil {
				s.dropWakeRead(ctx, owner.CoordinatorID, taskID, err)
				return
			}
			read = true
		}
		for _, ep := range episodes {
			_, _ = s.RecordWake(ctx, owner.CoordinatorID, taskID, ep.Kind, ep.Key)
		}
	}
}

// dropWakeRead counts a read failure that dropped an event; a deleted
// coordinator counts as not_found and a cancelled ctx (shutdown) counts
// nothing.
func (s *Service) dropWakeRead(ctx context.Context, coordinatorID, taskID string, err error) {
	if ctx.Err() != nil {
		s.logger.Debug("wake read cancelled", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if errors.Is(err, ErrNotFound) {
		wakeDroppedTotal.Add(dropReasonNotFound, 1)
		return
	}
	wakeDroppedTotal.Add(dropReasonReadError, 1)
	s.logger.Warn("wake read failed",
		zap.String("coordinator_id", coordinatorID), zap.String("task_id", taskID), zap.Error(err))
}

// RecordWake stores one wake through Store.RecordWake and, after the
// transaction, counts the outcome; an inserted wake also publishes
// coordinator.updated and calls Kick, in that order. A refusal is not an
// error: the caller logs it and the backstop recovers it.
func (s *Service) RecordWake(ctx context.Context, coordinatorID, taskID string, kind WakeKind, episodeKey string) (WakeOutcome, error) {
	res, err := s.store.RecordWake(ctx, coordinatorID, taskID, kind, episodeKey)
	if err != nil {
		s.countRecordError(ctx, coordinatorID, taskID, err)
		return "", err
	}
	switch res.Outcome {
	case WakeInserted:
		wakeRecordedTotal.Add(string(kind), 1)
		s.publishCoordinatorUpdatedWith(ctx, res.WorkspaceID, coordinatorID, true)
		s.callKick(ctx, coordinatorID)
	case WakeCapped:
		wakeDroppedTotal.Add(dropReasonCap, 1)
	case WakeAutonomyOff:
		wakeDroppedTotal.Add(dropReasonAutonomyOff, 1)
	case WakeNotOwn:
		wakeDroppedTotal.Add(dropReasonNotOwn, 1)
	}
	return res.Outcome, nil
}

func (s *Service) countRecordError(ctx context.Context, coordinatorID, taskID string, err error) {
	if ctx.Err() != nil {
		s.logger.Debug("wake record cancelled", zap.String("coordinator_id", coordinatorID), zap.Error(err))
		return
	}
	if errors.Is(err, ErrNotFound) {
		wakeDroppedTotal.Add(dropReasonNotFound, 1)
		return
	}
	wakeDroppedTotal.Add(dropReasonWriteError, 1)
	s.logger.Warn("wake record failed",
		zap.String("coordinator_id", coordinatorID), zap.String("task_id", taskID), zap.Error(err))
}
