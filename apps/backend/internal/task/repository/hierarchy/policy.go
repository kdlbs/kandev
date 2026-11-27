package hierarchy

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

var ErrInvalidParent = errors.New("invalid parent")
var ErrSubtaskDepthExceeded = errors.New("cannot create a subtask of a subtask — maximum nesting depth is 1 for kanban tasks. Create a sibling task under the same parent or a top-level task instead")

const parentChainWalkLimit = 1000

func ValidateParent(ctx context.Context, reader TaskHierarchyReader, task *models.Task, parentID string) error {
	if parentID == "" || parentID == task.ParentID {
		return nil
	}
	if parentID == task.ID {
		return fmt.Errorf("%w: a task cannot be its own parent", ErrInvalidParent)
	}
	parent, err := reader.GetTask(ctx, parentID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return fmt.Errorf("%w: parent task not found: %s", ErrInvalidParent, parentID)
		}
		return err
	}
	if parent.WorkspaceID != task.WorkspaceID {
		return fmt.Errorf("%w: parent task must belong to the same workspace", ErrInvalidParent)
	}
	if parent.ArchivedAt != nil {
		return fmt.Errorf("%w: parent task is archived", ErrInvalidParent)
	}
	// Cycle detection runs before the depth guard so a self-referential
	// re-parent reports the more specific "cycle" error rather than a depth
	// violation.
	if err := checkParentCycle(ctx, reader, task, parent); err != nil {
		return err
	}
	return validateReparentDepth(ctx, reader, task, parent)
}

// checkParentCycle walks up the parent's ancestor chain. Reaching task.ID means
// the new edge would close a cycle (task -> ... -> parent -> task).
func checkParentCycle(ctx context.Context, reader TaskHierarchyReader, task, parent *models.Task) error {
	current := parent
	visited := make(map[string]bool)
	for i := 0; i < parentChainWalkLimit; i++ {
		if current.ID == task.ID || visited[current.ID] {
			return fmt.Errorf("%w: nesting would create a cycle", ErrInvalidParent)
		}
		visited[current.ID] = true
		if current.ParentID == "" {
			return nil
		}
		ancestor, err := reader.GetTask(ctx, current.ParentID)
		if err != nil {
			if errors.Is(err, repoerrors.ErrTaskNotFound) {
				return nil
			}
			return err
		}
		current = ancestor
	}
	return fmt.Errorf("%w: parent chain too deep", ErrInvalidParent)
}

// validateReparentDepth enforces the one-level subtask limit for kanban
// (non-office) tasks on the re-parent path, mirroring validateSubtaskDepth on
// the create path. Office task trees intentionally allow arbitrary depth, so
// the guard is skipped when either endpoint is an Office task. The returned
// error wraps both ErrInvalidParent (so handlers map it to HTTP 400) and
// ErrSubtaskDepthExceeded (so callers can still classify the depth violation).
func validateReparentDepth(ctx context.Context, reader TaskHierarchyReader, task, parent *models.Task) error {
	if task.IsFromOffice || parent.IsFromOffice {
		return nil
	}
	// Nesting under a task that is itself a subtask would create a grandchild.
	if parent.ParentID != "" {
		return fmt.Errorf("%w: %w", ErrInvalidParent, ErrSubtaskDepthExceeded)
	}
	// Moving a task that already has children would push those children to
	// depth 2 under the new parent.
	children, err := reader.ListChildren(ctx, task.ID)
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidParent, ErrSubtaskDepthExceeded)
	}
	return nil
}

// ValidateCreationParent retains creation's parent-only Office depth exemption.
func ValidateCreationParent(ctx context.Context, reader TaskHierarchyReader, _ *models.Task, parentID string) error {
	if parentID == "" {
		return nil
	}
	parent, err := reader.GetTask(ctx, parentID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return fmt.Errorf("invalid parent_id: %w", err)
		}
		return err
	}
	if parent.ParentID != "" && !parent.IsFromOffice {
		return ErrSubtaskDepthExceeded
	}
	return nil
}
