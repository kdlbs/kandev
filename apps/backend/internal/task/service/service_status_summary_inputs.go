package service

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	"reflect"
	"strings"
)

func taskLaunchErrorSummary(task *models.Task) *statussummary.ActiveErrorSummary {
	if task == nil {
		return nil
	}
	errorValue, ok := models.LoadTaskLaunchError(task.Metadata)
	if !ok {
		return nil
	}
	return &statussummary.ActiveErrorSummary{
		Scope:            models.ErrorScopeTask,
		SessionID:        errorValue.SessionID,
		TaskRepositoryID: errorValue.TaskRepositoryID,
		Stamp:            errorValue.Stamp(),
		OccurredAt:       errorValue.OccurredAt,
		Preview:          errorValue.Message,
		Details:          errorValue.Details,
		Category:         errorValue.Code,
		RecoveryActions:  errorValue.RecoveryActions,
	}
}

func snapshotRepositoryKey(snapshot *models.GitSnapshot, fallback string) string {
	if snapshot != nil && snapshot.Metadata != nil {
		if repository, ok := snapshot.Metadata["repository_name"].(string); ok && strings.TrimSpace(repository) != "" {
			return repository
		}
	}
	return fallback
}

func gitSummaryFromSnapshot(snapshot *models.GitSnapshot) statussummary.GitSummary {
	if snapshot == nil {
		return statussummary.GitSummary{}
	}
	return statussummary.GitSummary{
		Additions:             nonNegative(snapshot.Metadata, "branch_additions"),
		Deletions:             nonNegative(snapshot.Metadata, "branch_deletions"),
		ChangedFiles:          changedFilesFromSnapshot(snapshot),
		Ahead:                 maxInt(snapshot.Ahead, 0),
		Behind:                maxInt(snapshot.Behind, 0),
		ComparisonUnavailable: snapshotComparisonUnavailable(snapshot),
	}
}

func snapshotComparisonUnavailable(snapshot *models.GitSnapshot) bool {
	if snapshot == nil || snapshot.Metadata == nil {
		return false
	}
	value, _ := snapshot.Metadata["comparison_status"].(string)
	return value == "unavailable"
}

func changedFilesFromSnapshot(snapshot *models.GitSnapshot) int {
	if snapshot == nil {
		return 0
	}
	if _, ok := snapshot.Metadata["changed_files"]; ok {
		return nonNegative(snapshot.Metadata, "changed_files")
	}
	count := 0
	for _, key := range []string{"modified", "added", "deleted", "untracked", "renamed"} {
		count += collectionLength(snapshot.Metadata[key])
	}
	if count == 0 {
		count = len(snapshot.Files)
	}
	return count
}

func nonNegative(values map[string]interface{}, key string) int {
	if values == nil {
		return 0
	}
	value := values[key]
	switch number := value.(type) {
	case int:
		return maxInt(number, 0)
	case int64:
		return maxInt(int(number), 0)
	case float64:
		return maxInt(int(number), 0)
	default:
		return 0
	}
}

func collectionLength(value interface{}) int {
	if value == nil {
		return 0
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return reflected.Len()
	default:
		return 0
	}
}

func maxInt(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}
