package backendapp

import "github.com/kandev/kandev/internal/user/models"

func mapThreadViews(views []models.ThreadView) []map[string]any {
	result := make([]map[string]any, 0, len(views))
	for _, view := range views {
		result = append(result, map[string]any{
			"id":         view.ID,
			"name":       view.Name,
			"taskScope":  mapThreadTaskScope(view.TaskScope),
			"filters":    mapThreadViewClauses(view.Filters),
			"sort":       mapThreadViewSort(view.Sort),
			"maxColumns": view.MaxColumns,
		})
	}
	return result
}

func mapThreadViewDraft(draft *models.ThreadViewDraft) map[string]any {
	if draft == nil {
		return nil
	}
	return map[string]any{
		"baseViewId": draft.BaseViewID,
		"taskScope":  mapThreadTaskScope(draft.TaskScope),
		"filters":    mapThreadViewClauses(draft.Filters),
		"sort":       mapThreadViewSort(draft.Sort),
		"maxColumns": draft.MaxColumns,
	}
}

func mapThreadTaskScope(scope models.ThreadTaskScope) map[string]any {
	taskIDs := scope.TaskIDs
	if taskIDs == nil {
		taskIDs = []string{}
	}
	return map[string]any{"mode": scope.Mode, "taskIds": taskIDs}
}

func mapThreadViewClauses(clauses []models.ThreadViewClause) []map[string]any {
	result := make([]map[string]any, 0, len(clauses))
	for _, clause := range clauses {
		result = append(result, map[string]any{
			"id":        clause.ID,
			"dimension": clause.Dimension,
			"op":        clause.Op,
			"value":     clause.Value,
		})
	}
	return result
}

func mapThreadViewSort(sort models.ThreadViewSort) map[string]any {
	return map[string]any{"key": sort.Key, "direction": sort.Direction}
}
