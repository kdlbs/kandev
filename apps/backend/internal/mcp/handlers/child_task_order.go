package handlers

import (
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"sort"
)

// childReorderBandOrder returns the complete new order of the target band.
// stepTasks may hold tasks from other steps; only the target step's band is
// used, in its current step order.
func childReorderBandOrder(
	stepTasks []*models.Task, target childReorderTarget, orderedIDs []string, placement string,
) ([]string, *childReorderFailure) {
	band := childReorderRows(stepTasks, target)
	named := make(map[string]struct{}, len(orderedIDs))
	for _, id := range orderedIDs {
		named[id] = struct{}{}
	}
	current, failure := childReorderCurrentBand(band, target, named)
	if failure != nil {
		return nil, failure
	}
	return composeChildOrder(current, orderedIDs, named, placement), nil
}

func childReorderRows(stepTasks []*models.Task, target childReorderTarget) []*models.Task {
	band := make([]*models.Task, 0, len(stepTasks))
	for _, task := range stepTasks {
		if task != nil && task.WorkflowStepID == target.stepID && childReorderBand(task, target.stepID) == target.band {
			band = append(band, task)
		}
	}
	sort.SliceStable(band, func(i, j int) bool { return models.StepOrderLess(band[i], band[j]) })
	return band
}

func childReorderCurrentBand(band []*models.Task, target childReorderTarget, named map[string]struct{}) ([]string, *childReorderFailure) {
	current := make([]string, len(band))
	found := 0
	for i, task := range band {
		current[i] = task.ID
		if _, ok := named[task.ID]; ok {
			if !canDirectParentAccess(&models.Task{ID: target.parentID, WorkspaceID: target.workspaceID}, task) {
				return nil, &childReorderFailure{ws.ErrorCodeForbidden, "a named task is no longer your direct child: " + task.ID}
			}
			found++
		}
	}
	if found != len(named) {
		return nil, &childReorderFailure{ws.ErrorCodeConflict,
			"a named task left the step or band while reordering; read it again and retry"}
	}
	return current, nil
}

func composeChildOrder(current, orderedIDs []string, named map[string]struct{}, placement string) []string {
	if placement == childReorderPlacementFront {
		next := append([]string{}, orderedIDs...)
		for _, id := range current {
			if _, ok := named[id]; !ok {
				next = append(next, id)
			}
		}
		return next
	}
	next := make([]string, len(current))
	cursor := 0
	for i, id := range current {
		if _, ok := named[id]; ok {
			next[i] = orderedIDs[cursor]
			cursor++
			continue
		}
		next[i] = id
	}
	return next
}

// childReorderBand mirrors the step reorder contract's band split: the queued
// band is !wip_admitted && queued_for_step_id == stepID; everything else in
// the step is the admitted band.
func childReorderBand(task *models.Task, stepID string) string {
	if !task.WIPAdmitted && task.QueuedForStepID == stepID {
		return childReorderBandQueued
	}
	return childReorderBandAdmitted
}
