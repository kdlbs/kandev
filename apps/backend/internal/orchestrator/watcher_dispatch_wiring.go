package orchestrator

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// serviceTaskStarter adapts Service.StartTask to the coordinator's
// taskStarter interface. Lives in its own file so the wiring stays close
// to the Service definition without polluting watcher_dispatch.go with
// orchestrator-internal types.
type serviceTaskStarter struct{ svc *Service }

func (s serviceTaskStarter) Start(ctx context.Context, taskID, workflowStepID, prompt string, p AutoStartParams) error {
	_, err := s.svc.StartTask(
		ctx, taskID, p.AgentProfileID, "", p.ExecutorProfileID,
		"", prompt, workflowStepID, false, true, nil,
	)
	return err
}

// initWatcherCoordinator builds the coordinator (once) and (always) refreshes
// the mutable taskCreator dependency via SetTaskCreator. Called from
// SetIssueTaskCreator, which can be invoked multiple times — tests in
// particular may swap creators between scenarios. Re-running the setter MUST
// update the coordinator, otherwise Dispatch silently keeps the original
// creator.
func (s *Service) initWatcherCoordinator() {
	if s.watcherCoordinator == nil {
		s.watcherCoordinator = &WatcherDispatchCoordinator{
			startTask: serviceTaskStarter{svc: s},
			shouldAutoStart: func(ctx context.Context, stepID string) bool {
				return s.shouldAutoStartStep(ctx, stepID)
			},
			logger: s.logger,
		}
	}
	s.watcherCoordinator.SetTaskCreator(s.issueTaskCreator)
}

// dispatchWatcherEvent runs the wiring guards every per-integration bus
// handler shares — issueTaskCreator check and the final goroutine dispatch
// with cancellation detached. integration is used in log message templating
// ("new linear issue ...", "skipping jira task ..."). fields are the
// structured log fields that identify the event in operator logs; pass at
// least the issue_watch_id and an integration-specific identifier field so
// a deferred / dropped event is diagnosable.
//
// Lives in its own helper so per-integration handlers stay below dupl's
// duplicate-block threshold without copy-pasting the same guards.
func (s *Service) dispatchWatcherEvent(ctx context.Context, integration string, src WatcherSource, evt any, fields ...zap.Field) {
	s.logger.Info(fmt.Sprintf("new %s issue detected from watch", integration), fields...)
	if s.issueTaskCreator == nil {
		s.logger.Warn(fmt.Sprintf("issue task creator not configured, skipping %s task creation", integration))
		return
	}
	// Capture the coordinator pointer locally. issueTaskCreator is set before
	// initWatcherCoordinator in SetIssueTaskCreator, so a concurrent bus event
	// could otherwise see issueTaskCreator non-nil while watcherCoordinator is
	// still nil and crash on the goroutine dispatch below.
	coordinator := s.watcherCoordinator
	if coordinator == nil {
		s.logger.Warn(fmt.Sprintf("watcher coordinator not configured, skipping %s task creation", integration))
		return
	}

	// Per-watcher throttle. Synchronous: we MUST acquire the slot before
	// spawning the goroutine, otherwise a tight burst of bus events all
	// read the same pre-creation DB count and overshoot the cap. See
	// docs/specs/throttle-watcher-fanout/spec.md.
	release, ok := s.acquireWatcherSlot(ctx, integration, src.WatchID(evt), src.MaxInflightTasks(evt))
	if !ok {
		return
	}

	// Detach from cancellation but keep request-scoped values (tracing, etc.):
	// the bus delivery context may be cancelled before task creation finishes.
	go func() {
		defer release()
		coordinator.Dispatch(context.WithoutCancel(ctx), src, evt)
	}()
}
