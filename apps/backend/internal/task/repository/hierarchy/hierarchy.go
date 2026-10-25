package hierarchy

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
)

// TaskHierarchyReader reads current graph state from the admitted transaction.
type TaskHierarchyReader interface {
	GetTask(context.Context, string) (*models.Task, error)
	ListChildren(context.Context, string) ([]*models.Task, error)
}

type TaskParentValidator func(context.Context, TaskHierarchyReader, *models.Task, string) error

// TaskHierarchyAdmission keeps explicit parent intent separate from snapshots.
type TaskHierarchyAdmission interface {
	ValidateTaskParent(context.Context, string, string, TaskParentValidator) error
	UpdateTaskWithParentAdmission(context.Context, *models.Task, *string, bool, TaskParentValidator) (bool, error)
	ValidateTaskCreationParent(context.Context, string, string, TaskParentValidator) error
}

type creationParentValidatorKey struct{}

// WithTaskCreationParentValidator carries only the create-parent predicate to
// existing insertion variants, which revalidate after reserving the graph.
func WithTaskCreationParentValidator(ctx context.Context, validate TaskParentValidator) context.Context {
	return context.WithValue(ctx, creationParentValidatorKey{}, validate)
}
func TaskCreationParentValidator(ctx context.Context) TaskParentValidator {
	validator, _ := ctx.Value(creationParentValidatorKey{}).(TaskParentValidator)
	return validator
}
