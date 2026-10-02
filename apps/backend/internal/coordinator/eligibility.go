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
	if StartsAgentOnEnter(steps, stepID) {
		return false
	}
	if !candidate.IsStart && !candidate.AllowManualMove {
		return false
	}
	return true
}

// EligibleStartingStep is EligibleStep with the agent-starting clauses
// relaxed: the step exists, is the start step or allows manual moves, and does
// not complete the task on enter. It is used only while start_agent needs
// approval, and the caller stores StartsAgentOnEnter for the disclosure.
func EligibleStartingStep(steps []StepNode, stepID string) bool {
	for _, step := range steps {
		if step.ID != stepID {
			continue
		}
		return !step.CompletesOnEnter && (step.IsStart || step.AllowManualMove)
	}
	return false
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

// StartsAgentOnEnter reports whether entering stepID starts an agent: the step
// auto-starts one on enter, or it feeds, directly or through pull_from_step_id
// links, a step that does. An unknown stepID does not start one.
func StartsAgentOnEnter(steps []StepNode, stepID string) bool {
	byID := make(map[string]StepNode, len(steps))
	for _, step := range steps {
		byID[step.ID] = step
	}
	if candidate, ok := byID[stepID]; ok && candidate.AutoStartOnEnter {
		return true
	}
	for _, step := range steps {
		if step.AutoStartOnEnter && feedsInto(byID, step.PullFromStepID, stepID) {
			return true
		}
	}
	return false
}

// feedsAutoStartStep reports whether entering stepID can promote queued work
// into a different step that auto-starts an agent. Undo can suppress the
// returned task's direct auto-start, but the shared move path also reconciles
// feeder work and does not carry that one-shot option to promoted tasks.
func feedsAutoStartStep(steps []StepNode, stepID string) bool {
	byID := make(map[string]StepNode, len(steps))
	for _, step := range steps {
		byID[step.ID] = step
	}
	for _, step := range steps {
		if step.ID == stepID || !step.AutoStartOnEnter {
			continue
		}
		if feedsInto(byID, step.PullFromStepID, stepID) {
			return true
		}
	}
	return false
}
