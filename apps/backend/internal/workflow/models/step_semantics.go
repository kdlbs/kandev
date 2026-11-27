package models

// StepRunsOnEntry reports whether entering step starts work by itself: an
// on_enter action dispatches an agent or a run, or the step pulls work forward
// from another step. It reads only the step's own configuration, never its
// name. A nil step is not treated as one that starts work.
func StepRunsOnEntry(step *WorkflowStep) bool {
	if step == nil {
		return false
	}
	if step.PullFromStepID != "" {
		return true
	}
	for _, action := range step.Events.OnEnter {
		switch action.Type {
		case OnEnterAutoStartAgent, OnEnterQueueRun, OnEnterQueueRunForEachParticipant, OnEnterRunCodeReview:
			return true
		}
	}
	return false
}
