package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/office/shared"
)

// Limits on blocked_by match the task service's dependency-set limits, so an
// input the runtime accepts is never rejected later for its size.
const (
	maxTaskRelationBlockers        = 512
	maxTaskRelationBlockerIDLength = 128
)

const (
	taskRelationModeInheritParent = "inherit_parent"
	taskRelationModeNewWorkspace  = "new_workspace"
)

// TaskReadRelation answers whether the run's task may read another task under
// the shared Office read relation (self, ancestors, descendants, siblings, and
// blockers, within one workspace). A missing task is a plain false.
type TaskReadRelation interface {
	CanReadTask(ctx context.Context, currentTaskID, targetTaskID string) (bool, error)
}

// normalizeTaskRelations trims, validates, and de-duplicates the relation
// fields before any lookup, so malformed input never causes a read.
func normalizeTaskRelations(input CreateTaskInput) (shared.AgentTaskRelations, error) {
	blockedBy, err := normalizeBlockedBy(input.BlockedBy)
	if err != nil {
		return shared.AgentTaskRelations{}, err
	}
	mode := strings.TrimSpace(input.WorkspaceMode)
	switch mode {
	case "", taskRelationModeNewWorkspace:
	case taskRelationModeInheritParent:
		if input.ParentTaskID == "" {
			return shared.AgentTaskRelations{}, fmt.Errorf(
				"%w: workspace_mode=%s requires parent_id", ErrInvalidTaskRelations, taskRelationModeInheritParent)
		}
	default:
		return shared.AgentTaskRelations{}, fmt.Errorf(
			"%w: invalid workspace_mode %q (allowed: %s, %s)", ErrInvalidTaskRelations,
			mode, taskRelationModeInheritParent, taskRelationModeNewWorkspace)
	}
	return shared.AgentTaskRelations{BlockedBy: blockedBy, WorkspaceMode: mode}, nil
}

func normalizeBlockedBy(raw []string) ([]string, error) {
	if len(raw) > maxTaskRelationBlockers {
		return nil, fmt.Errorf("%w: blocked_by accepts at most %d task IDs", ErrInvalidTaskRelations, maxTaskRelationBlockers)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for index, entry := range raw {
		id := strings.TrimSpace(entry)
		if id == "" {
			return nil, fmt.Errorf("%w: blocked_by[%d] is empty", ErrInvalidTaskRelations, index)
		}
		if len(id) > maxTaskRelationBlockerIDLength {
			return nil, fmt.Errorf("%w: blocked_by[%d] exceeds %d characters",
				ErrInvalidTaskRelations, index, maxTaskRelationBlockerIDLength)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// validateBlockerScope admits a blocker only when it is in the run's workspace
// and either inside the run's mutation scope or readable from the run's task.
// Every denial returns the same error, so a missing, foreign, and unrelated
// task are indistinguishable to the caller.
func (a *Actions) validateBlockerScope(ctx context.Context, runCtx RunContext, blockedBy []string) error {
	for index, id := range blockedBy {
		ok, err := a.blockerInScope(ctx, runCtx, id)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: blocked_by[%d] is not a task this run may depend on", ErrBlockerOutOfScope, index)
		}
	}
	return nil
}

func (a *Actions) blockerInScope(ctx context.Context, runCtx RunContext, id string) (bool, error) {
	if id == WildcardTaskScope {
		return false, nil
	}
	if runCtx.CanMutateTask(id) {
		workspaceID, err := a.deps.Tasks.GetTaskWorkspaceID(ctx, id)
		if err != nil {
			return false, err
		}
		return workspaceID != "" && workspaceID == runCtx.WorkspaceID, nil
	}
	callerTaskID := strings.TrimSpace(runCtx.TaskID)
	if callerTaskID == "" {
		return false, nil
	}
	if a.deps.TaskReads == nil {
		return false, fmt.Errorf("%w: task read relation", ErrRuntimeDependencyMissing)
	}
	return a.deps.TaskReads.CanReadTask(ctx, callerTaskID, id)
}
