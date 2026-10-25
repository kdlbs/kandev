package repository

import (
	"context"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
)

type TaskHierarchyReader = hierarchy.TaskHierarchyReader
type TaskParentValidator = hierarchy.TaskParentValidator
type TaskHierarchyAdmission = hierarchy.TaskHierarchyAdmission

func WithTaskCreationParentValidator(ctx context.Context, validate TaskParentValidator) context.Context {
	return hierarchy.WithTaskCreationParentValidator(ctx, validate)
}
