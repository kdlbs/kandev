package dto

import (
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TaskDependencyRefDTO is one end of a dependency edge, carrying enough detail
// for the dependency chip and the blocked badge to render without fetching each
// related task individually.
type TaskDependencyRefDTO struct {
	ID    string       `json:"id"`
	Title string       `json:"title,omitempty"`
	State v1.TaskState `json:"state,omitempty"`
	// Status is the resolution verdict for this edge: "resolved", "failed", or
	// "pending". Only meaningful on depends_on entries.
	Status string `json:"status,omitempty"`
}

// TaskDependencyProjection is the shape a caller supplies to stamp dependency
// state onto a DTO. It mirrors service.DependencyView without importing the
// service package (dto must stay importable from it).
type TaskDependencyProjection struct {
	Blocked       bool
	BlockedReason string
	DependsOn     []TaskDependencyRefDTO
	Blocks        []TaskDependencyRefDTO
}

// EnrichTaskDependencies stamps the derived dependency projection onto a task
// DTO, following the same read-time-enrichment pattern as
// EnrichTaskForegroundActivity.
//
// StartWhenUnblocked is read from the task's deferred-launch metadata rather
// than passed in: the intent and the edges are stored separately, and only the
// combination means "this will start on its own".
func EnrichTaskDependencies(dto *TaskDTO, projection TaskDependencyProjection, task *models.Task) {
	if dto == nil {
		return
	}
	dto.Blocked = projection.Blocked
	dto.BlockedReason = projection.BlockedReason
	dto.DependsOn = projection.DependsOn
	dto.Blocks = projection.Blocks
	dto.StartWhenUnblocked = HasStartWhenUnblockedIntent(task)
}

// HasStartWhenUnblockedIntent reports whether the task carries a deferred launch
// intent that dependency resolution should consume.
//
// The intent is the same metadata record WIP overflow persists, reused so the
// "launch exactly once" and restart-survival properties are inherited rather
// than re-derived. The dependency flavour is marked so a WIP-only intent is not
// mistaken for a chain step.
func HasStartWhenUnblockedIntent(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	raw, ok := task.Metadata[models.MetaKeyDeferredLaunch]
	if !ok {
		return false
	}
	launch, ok := raw.(map[string]interface{})
	if !ok {
		return false
	}
	flag, ok := launch[models.DeferredLaunchStartWhenUnblockedKey].(bool)
	return ok && flag
}
