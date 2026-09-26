package dto

import "github.com/kandev/kandev/internal/task/statussummary"

// EnrichTaskStatusSummary carries a summary or an explicit invalidation. A
// missing map entry remains ordinary omission for partial-response safety.
func EnrichTaskStatusSummary(
	task *TaskDTO,
	taskID string,
	summaries map[string]*statussummary.TaskStatusSummary,
) {
	if task == nil {
		return
	}
	summary, known := summaries[taskID]
	task.StatusSummary = summary
	task.StatusSummaryInvalidated = known && summary == nil
}
