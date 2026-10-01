package replay

import "strings"

// Normalise lower-cases a title with every whitespace run collapsed to one
// space and the ends trimmed.
func Normalise(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(title), " "))
}

// Key identifies a proposal for comparison: the workflow and normalised title
// for a create_task, the kind and the target task for every other kind.
func Key(kind, targetTaskID, workflowID, title string) string {
	if kind == ProposalCreateTask {
		return workflowID + "|" + Normalise(title)
	}
	return kind + "|" + targetTaskID
}
