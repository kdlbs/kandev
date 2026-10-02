package coordinator

// EligibleStep reports whether stepID is an eligible placement for a
// coordinator-approved task: the workflow's start step or a step that allows
// manual moves, the step itself does not auto-start an agent on enter, and
// it is not a feeder — directly or through a chain of pull_from_step_id
// links — of any step that does. See
// docs/specs/coordinator/system-design/proposals.md#no-agent-starts.
//
// steps is the workflow's full step graph, not just the candidate step:
// eligibility depends on reachability through other steps. An unknown
// stepID is ineligible.
func EligibleStep(steps []StepNode, stepID string) bool {
	byID := make(map[string]StepNode, len(steps))
	for _, step := range steps {
		byID[step.ID] = step
	}
	candidate, ok := byID[stepID]
	if !ok {
		return false
	}
	if candidate.AutoStartOnEnter {
		return false
	}
	if !candidate.IsStart && !candidate.AllowManualMove {
		return false
	}
	for _, step := range steps {
		if step.AutoStartOnEnter && feedsInto(byID, step.PullFromStepID, stepID) {
			return false
		}
	}
	return true
}

// feedsInto reports whether target is reachable from fromStepID by walking
// pull_from_step_id links. A visited set guards against a cycle in the
// feeder graph.
func feedsInto(byID map[string]StepNode, fromStepID, target string) bool {
	visited := make(map[string]bool)
	for fromStepID != "" {
		if fromStepID == target {
			return true
		}
		if visited[fromStepID] {
			return false
		}
		visited[fromStepID] = true
		step, ok := byID[fromStepID]
		if !ok {
			return false
		}
		fromStepID = step.PullFromStepID
	}
	return false
}
