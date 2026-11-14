package orchestrator

import (
	"strings"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

// formatChildStallDescriptorsText rebuilds the base prompt text of a folded
// alert from the candidates that still qualify. Folding concatenates the
// candidate texts in descriptor order, so the surviving candidates rebuild the
// same prompt without the stale ones.
func formatChildStallDescriptorsText(descriptors []messagequeue.ChildStallAlertDescriptor) string {
	parts := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		parts = append(parts, formatChildStallPromptText(
			descriptor.ChildTaskTitle,
			descriptor.ChildTaskID,
			descriptor.TurnID,
			descriptor.StepName,
			descriptor.Cause,
			descriptor.QuestionID,
		))
	}
	return strings.Join(parts, "\n\n")
}
