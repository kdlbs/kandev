package dto

// TaskRunnerMutabilityProjection is the shape a caller supplies to stamp the
// runner-mutability verdict onto a DTO. It mirrors
// service.RunnerMutabilityView without importing the service package (dto
// must stay importable from it).
type TaskRunnerMutabilityProjection struct {
	Editable bool
	Reason   string
}

// EnrichTaskRunnerMutability stamps the derived runner-mutability verdict
// onto a task DTO, following the same read-time-enrichment pattern as
// EnrichTaskDependencies. Both fields are always set — never left at their
// zero value on the fail-closed path — because the empty string is outside
// the closed reason vocabulary and a caller that skips this call keeps
// FromTask's own fail-closed default instead of a silent "editable" zero
// value.
func EnrichTaskRunnerMutability(dto *TaskDTO, projection TaskRunnerMutabilityProjection) {
	if dto == nil {
		return
	}
	dto.RunnerEditable = projection.Editable
	dto.RunnerIneligibleReason = projection.Reason
}
