// Package agentprofile owns the order in which a task's agent profile is
// resolved, so the orchestrator and the coordinator cannot disagree about it.
package agentprofile

// Source names the input that supplied the resolved profile.
type Source string

const (
	SourceNone         Source = "none"
	SourceStep         Source = "step"
	SourceWorkflow     Source = "workflow"
	SourceTaskMetadata Source = "task_metadata"
	SourceAssignee     Source = "assignee"
	SourceWorkspace    Source = "workspace"
)

// Input carries every profile a task's resolution can consult. An empty string
// means the input names no profile.
type Input struct {
	// HasStep is true when the task sits on a workflow step. The workflow
	// default is consulted only then.
	HasStep bool
	// StepSessionTarget is true when the step has a session target, which
	// makes the step's own profile, its replacement and the workflow default
	// inapplicable.
	StepSessionTarget bool
	StepReplacement   string
	StepProfile       string
	WorkflowDefault   string
	TaskMetadata      string
	Assignee          string
	WorkspaceDefault  string
}

// Resolve returns the first profile in the order: step replacement, step
// profile, workflow default, task metadata, assignee, workspace default.
func Resolve(in Input) (string, Source) {
	if in.HasStep && !in.StepSessionTarget {
		if in.StepReplacement != "" {
			return in.StepReplacement, SourceStep
		}
		if in.StepProfile != "" {
			return in.StepProfile, SourceStep
		}
		if in.WorkflowDefault != "" {
			return in.WorkflowDefault, SourceWorkflow
		}
	}
	if in.TaskMetadata != "" {
		return in.TaskMetadata, SourceTaskMetadata
	}
	if in.Assignee != "" {
		return in.Assignee, SourceAssignee
	}
	if in.WorkspaceDefault != "" {
		return in.WorkspaceDefault, SourceWorkspace
	}
	return "", SourceNone
}
