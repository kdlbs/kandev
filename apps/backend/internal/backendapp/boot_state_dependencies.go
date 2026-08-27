package backendapp

import (
	taskdto "github.com/kandev/kandev/internal/task/dto"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// bootDependencyProjection adapts the task service's derived DependencyView to
// the DTO projection type. Mirrors the same adapter in the task handlers; both
// exist because dto must stay importable from service without a cycle.
func bootDependencyProjection(view taskservice.DependencyView) taskdto.TaskDependencyProjection {
	return taskdto.TaskDependencyProjection{
		Blocked:       view.Blocked,
		BlockedReason: view.BlockedReason,
		DependsOn:     bootDependencyRefs(view.DependsOn),
		Blocks:        bootDependencyRefs(view.Blocks),
	}
}

func bootDependencyRefs(refs []taskservice.DependencyRef) []taskdto.TaskDependencyRefDTO {
	if len(refs) == 0 {
		return nil
	}
	out := make([]taskdto.TaskDependencyRefDTO, 0, len(refs))
	for _, r := range refs {
		out = append(out, taskdto.TaskDependencyRefDTO{
			ID: r.ID, Title: r.Title, State: r.State, Status: r.Status,
		})
	}
	return out
}
