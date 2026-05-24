package orchestrator

import "context"

// serviceTaskStarter adapts Service.StartTask to the coordinator's
// taskStarter interface. Lives in its own file so the wiring stays close
// to the Service definition without polluting watcher_dispatch.go with
// orchestrator-internal types.
type serviceTaskStarter struct{ svc *Service }

func (s serviceTaskStarter) Start(ctx context.Context, taskID, workflowStepID string, p AutoStartParams) error {
	_, err := s.svc.StartTask(
		ctx, taskID, p.AgentProfileID, "", p.ExecutorProfileID,
		"", p.Prompt, workflowStepID, false, true, nil,
	)
	return err
}

// initWatcherCoordinator builds the coordinator (once) and (always) refreshes
// the mutable taskCreator dependency. Called from SetIssueTaskCreator, which
// can be invoked multiple times — tests in particular may swap creators
// between scenarios. Re-running the setter MUST update the coordinator,
// otherwise Dispatch silently keeps the original creator.
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
	// Always refresh: SetIssueTaskCreator may be called more than once.
	s.watcherCoordinator.taskCreator = s.issueTaskCreator
}
