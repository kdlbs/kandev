package service

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Task dependencies ("task B is blocked by task A") are peer-to-peer edges,
// distinct from the parent/child subtask hierarchy: a subtask means "part of",
// a dependency means "not until". Edges live in task_blockers and are read
// through BlockerRepository.
//
// Blocked state is DERIVED on every read, never stored. A denormalized
// is_blocked column would be read by the auto-start gate, and a stale value
// there would launch work whose predecessor never ran — the one failure this
// feature exists to prevent.

// Dependency resolution verdicts for a single predecessor.
const (
	// DependencyResolved means the predecessor finished successfully.
	DependencyResolved = "resolved"
	// DependencyFailed means the predecessor reached a terminal state that is
	// not success. It never resolves the edge.
	DependencyFailed = "failed"
	// DependencyPending means the predecessor has not finished. Archived
	// predecessors are pending: archival is neither success nor failure.
	DependencyPending = "pending"
)

// Blocked reasons reported on the task payload.
const (
	// BlockedReasonPending — at least one predecessor is unfinished.
	BlockedReasonPending = "pending"
	// BlockedReasonFailed — no predecessor is unfinished and at least one
	// failed, so the chain has halted and needs human action.
	BlockedReasonFailed = "failed"
	// BlockedReasonUnknown — the dependency store could not be read. The gate
	// fails closed: unknown counts as blocked.
	BlockedReasonUnknown = "unknown"
)

// dependencyCycleWalkLimit bounds the BFS in checkDependencyCycle. A walk that
// exceeds it rejects the edge rather than accepting an unverified one.
const dependencyCycleWalkLimit = 1000

// DependencyRef is one end of a dependency edge, carrying enough detail for the
// dependency chip to render without fetching each related task.
type DependencyRef struct {
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	State  v1.TaskState `json:"state"`
	Status string       `json:"status,omitempty"`
}

// DependencyView is the derived dependency state for one task.
type DependencyView struct {
	Blocked       bool
	BlockedReason string
	DependsOn     []DependencyRef
	Blocks        []DependencyRef
}

// CycleError is returned when a proposed edge would close a cycle. Path lists
// the task IDs in traversal order (A, B, C, A) so callers can render
// "A → B → C → A".
type CycleError struct {
	Path []string
}

// Error implements the error interface.
func (e *CycleError) Error() string {
	if len(e.Path) == 0 {
		return "would create a dependency cycle"
	}
	return "would create a dependency cycle: " + strings.Join(e.Path, " → ")
}

// ErrDependencyRepositoryUnavailable is returned when the dependency store is
// not wired. Callers must treat it as "cannot determine", not "not blocked".
var ErrDependencyRepositoryUnavailable = fmt.Errorf("dependency repository not configured")

// ResolveStartWhenUnblocked decides whether a create request's agent start
// should become a start-when-unblocked intent instead of an immediate launch.
//
// A create with no dependencies is never deferred by this rule. A create WITH
// dependencies defaults to deferring, because `start_agent` defaults to true and
// every automated caller passes it: launching immediately would start every step
// of an agent-built chain at once, which is precisely the collision dependencies
// exist to prevent. An explicit `start_when_unblocked: false` opts out and
// creates the edges with no launch intent at all.
func ResolveStartWhenUnblocked(req *CreateTaskRequest) bool {
	if req == nil || len(req.BlockedBy) == 0 {
		return false
	}
	if req.StartWhenUnblocked == nil {
		return true
	}
	return *req.StartWhenUnblocked
}

// AddDependency records "taskID depends on dependsOnTaskID".
//
// This is the single validator for dependency edges. Self-edges,
// cross-workspace edges, and cycles of any length are rejected here, and both
// the task-scoped routes and the Office blocker routes go through it — a second
// validator would let a cycle in through whichever path was weakest.
func (s *Service) AddDependency(ctx context.Context, taskID, dependsOnTaskID string) error {
	if s.blockers == nil {
		return ErrDependencyRepositoryUnavailable
	}
	if taskID == "" || dependsOnTaskID == "" {
		return fmt.Errorf("both task_id and depends_on_task_id are required")
	}
	if taskID == dependsOnTaskID {
		return fmt.Errorf("a task cannot depend on itself")
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("resolve task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("%w: %s", taskrepo.ErrTaskNotFound, taskID)
	}
	dep, err := s.tasks.GetTask(ctx, dependsOnTaskID)
	if err != nil {
		return fmt.Errorf("resolve dependency task: %w", err)
	}
	if dep == nil {
		return fmt.Errorf("%w: %s", taskrepo.ErrTaskNotFound, dependsOnTaskID)
	}
	if task.WorkspaceID != "" && dep.WorkspaceID != "" && task.WorkspaceID != dep.WorkspaceID {
		return fmt.Errorf("task %s belongs to a different workspace", dependsOnTaskID)
	}
	if cycle, err := s.checkDependencyCycle(ctx, taskID, dependsOnTaskID); err != nil {
		return fmt.Errorf("check dependency cycle: %w", err)
	} else if cycle != nil {
		return cycle
	}
	if err := s.createBlockerEdge(ctx, taskID, dependsOnTaskID); err != nil {
		return err
	}
	s.publishDependencyChange(ctx, taskID, dependsOnTaskID)
	return nil
}

// RemoveDependency deletes a dependency edge. Removing an absent edge is a
// success no-op.
//
// Removing the last edge unblocks the task but deliberately does NOT consume
// its start-when-unblocked intent: that intent is consumed by dependency
// resolution, not by the absence of dependencies. A user who removes the edge
// is taking manual control.
func (s *Service) RemoveDependency(ctx context.Context, taskID, dependsOnTaskID string) error {
	if s.blockers == nil {
		return ErrDependencyRepositoryUnavailable
	}
	if err := s.blockers.DeleteTaskBlocker(ctx, taskID, dependsOnTaskID); err != nil {
		return err
	}
	s.publishDependencyChange(ctx, taskID, dependsOnTaskID)
	return nil
}

// publishDependencyChange emits task.updated for both ends of a mutated edge so
// every client refreshes its badge, chip and graph. Task mutations must go
// through the event publisher; walking the repository alone breaks WS-driven UI.
func (s *Service) publishDependencyChange(ctx context.Context, taskIDs ...string) {
	for _, id := range taskIDs {
		if id == "" {
			continue
		}
		task, err := s.tasks.GetTask(ctx, id)
		if err != nil || task == nil {
			continue
		}
		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	}
}

// checkDependencyCycle walks forward from dependsOnTaskID through existing
// edges. If any path reaches taskID, adding the edge would close a cycle, and
// the returned *CycleError carries the path for the caller to surface.
func (s *Service) checkDependencyCycle(ctx context.Context, taskID, dependsOnTaskID string) (*CycleError, error) {
	// parent[x] is the node we reached x from, so a hit can be walked back
	// into a readable path instead of only reporting "there is a cycle".
	parent := map[string]string{dependsOnTaskID: ""}
	queue := []string{dependsOnTaskID}
	walked := 0
	for len(queue) > 0 {
		if walked > dependencyCycleWalkLimit {
			return &CycleError{}, nil
		}
		current := queue[0]
		queue = queue[1:]
		walked++
		if current == taskID {
			return &CycleError{Path: buildCyclePath(parent, taskID, dependsOnTaskID)}, nil
		}
		blockers, err := s.blockers.ListTaskBlockers(ctx, current)
		if err != nil {
			return nil, err
		}
		for _, b := range blockers {
			if _, seen := parent[b.BlockerTaskID]; seen {
				continue
			}
			parent[b.BlockerTaskID] = current
			queue = append(queue, b.BlockerTaskID)
		}
	}
	return nil, nil
}

// buildCyclePath renders the discovered cycle in depends-on order, starting and
// ending at the task the new edge would block: taskID → dependsOn → … → taskID.
//
// parent[x] is the node whose blocker list contained x, so walking parent back
// from taskID yields the chain from taskID to dependsOn reversed. The proposed
// edge (taskID depends on dependsOn) closes the loop, so taskID is PREPENDED —
// the reversed chain already ends at taskID, and appending it again produced a
// duplicated final hop like "A → B → C → C" instead of "C → A → B → C".
func buildCyclePath(parent map[string]string, taskID, dependsOnTaskID string) []string {
	reverse := []string{taskID}
	for node := parent[taskID]; node != ""; node = parent[node] {
		reverse = append(reverse, node)
		if node == dependsOnTaskID {
			break
		}
	}
	path := make([]string, 0, len(reverse)+1)
	path = append(path, taskID)
	for i := len(reverse) - 1; i >= 0; i-- {
		path = append(path, reverse[i])
	}
	return path
}

// BuildDependencyViews returns derived dependency state for a batch of tasks.
//
// Batched deliberately: the Kanban board reads a whole workflow at once, so a
// per-task query would add one round trip per card to every board load.
func (s *Service) BuildDependencyViews(ctx context.Context, tasks []*models.Task) map[string]DependencyView {
	out := make(map[string]DependencyView, len(tasks))
	if s.blockers == nil || len(tasks) == 0 {
		return out
	}
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t != nil {
			ids = append(ids, t.ID)
		}
	}
	predecessors, err := s.blockers.ListBlockersForTasks(ctx, ids)
	if err != nil {
		s.logger.Warn("failed to load task dependencies", zap.Error(err))
		// Fail closed: a task whose edges cannot be read reports blocked with
		// an unknown reason rather than silently reporting "not blocked".
		for _, id := range ids {
			out[id] = DependencyView{Blocked: true, BlockedReason: BlockedReasonUnknown}
		}
		return out
	}
	dependents, err := s.blockers.ListDependentsForTasks(ctx, ids)
	if err != nil {
		s.logger.Warn("failed to load task dependents", zap.Error(err))
		dependents = map[string][]string{}
	}
	refs := s.resolveDependencyRefs(ctx, predecessors, dependents)
	for _, id := range ids {
		out[id] = buildDependencyView(refs, predecessors[id], dependents[id])
	}
	return out
}

// resolveDependencyRefs loads title/state for every task named on either side
// of any edge in the batch, in one query.
func (s *Service) resolveDependencyRefs(
	ctx context.Context, predecessors, dependents map[string][]string,
) map[string]DependencyRef {
	seen := map[string]struct{}{}
	for _, group := range []map[string][]string{predecessors, dependents} {
		for _, ids := range group {
			for _, id := range ids {
				seen[id] = struct{}{}
			}
		}
	}
	refs := make(map[string]DependencyRef, len(seen))
	for id := range seen {
		task, err := s.tasks.GetTask(ctx, id)
		if err != nil || task == nil {
			// A referenced task we cannot read counts as pending, never as
			// resolved — the gate must not open on a failed lookup.
			refs[id] = DependencyRef{ID: id, Status: DependencyPending}
			continue
		}
		refs[id] = DependencyRef{
			ID:     id,
			Title:  task.Title,
			State:  task.State,
			Status: DependencyStatusForTask(task),
		}
	}
	return refs
}

// buildDependencyView derives blocked/reason plus both edge lists for one task.
func buildDependencyView(
	refs map[string]DependencyRef, predecessorIDs, dependentIDs []string,
) DependencyView {
	view := DependencyView{
		DependsOn: make([]DependencyRef, 0, len(predecessorIDs)),
		Blocks:    make([]DependencyRef, 0, len(dependentIDs)),
	}
	anyPending, anyFailed := false, false
	for _, id := range predecessorIDs {
		ref := refs[id]
		if ref.ID == "" {
			ref = DependencyRef{ID: id, Status: DependencyPending}
		}
		view.DependsOn = append(view.DependsOn, ref)
		switch ref.Status {
		case DependencyFailed:
			anyFailed = true
		case DependencyResolved:
		default:
			anyPending = true
		}
	}
	for _, id := range dependentIDs {
		ref := refs[id]
		if ref.ID == "" {
			ref = DependencyRef{ID: id, Status: DependencyPending}
		}
		view.Blocks = append(view.Blocks, ref)
	}
	switch {
	case anyPending:
		view.Blocked = true
		view.BlockedReason = BlockedReasonPending
	case anyFailed:
		view.Blocked = true
		view.BlockedReason = BlockedReasonFailed
	}
	return view
}

// DependencyStatusForTask classifies one predecessor.
//
// Resolution requires SUCCESS. This is deliberately stricter than the
// on_children_completed trigger, which counts FAILED as terminal: a chain must
// never proceed on a failed step. Archived tasks are pending, because archival
// is neither success nor failure.
func DependencyStatusForTask(task *models.Task) string {
	if task == nil {
		return DependencyPending
	}
	switch task.State {
	case v1.TaskStateCompleted:
		if task.ArchivedAt != nil {
			return DependencyPending
		}
		return DependencyResolved
	case v1.TaskStateFailed, v1.TaskStateCancelled:
		return DependencyFailed
	default:
		return DependencyPending
	}
}

// DependencyGate reports whether taskID may be started by an automated path.
//
// Returns blocked=true on ANY read error. The gate fails closed on purpose:
// failing open would launch work whose predecessor may not have run.
func (s *Service) DependencyGate(ctx context.Context, taskID string) (blocked bool, reason string, err error) {
	if s.blockers == nil {
		// No dependency store wired means no edges can exist, so nothing is
		// blocked. This is not a read failure.
		return false, "", nil
	}
	blockers, listErr := s.blockers.ListTaskBlockers(ctx, taskID)
	if listErr != nil {
		return true, BlockedReasonUnknown, listErr
	}
	if len(blockers) == 0 {
		return false, "", nil
	}
	anyPending, anyFailed := false, false
	for _, b := range blockers {
		predecessor, getErr := s.tasks.GetTask(ctx, b.BlockerTaskID)
		if getErr != nil {
			return true, BlockedReasonUnknown, getErr
		}
		if predecessor == nil {
			// Edge to a task that no longer exists: treat as pending rather
			// than resolved, and let edge cleanup remove it.
			anyPending = true
			continue
		}
		switch DependencyStatusForTask(predecessor) {
		case DependencyFailed:
			anyFailed = true
		case DependencyResolved:
		default:
			anyPending = true
		}
	}
	switch {
	case anyPending:
		return true, BlockedReasonPending, nil
	case anyFailed:
		return true, BlockedReasonFailed, nil
	}
	return false, "", nil
}

// PendingDependencyLaunch is a task that will start on its own once its
// dependencies resolve.
type PendingDependencyLaunch struct {
	TaskID         string
	WorkflowStepID string
}

// ListPendingDependencyLaunches returns every non-archived task that has
// dependency edges AND a start-when-unblocked intent.
//
// Backs the orchestrator's startup sweep: task.dependencies_resolved is
// in-memory and is not replayed, so a restart between a predecessor completing
// and its dependent launching would otherwise stall the chain silently. Scoped
// by "has edges" so the sweep is bounded by pending chain steps, not the board.
func (s *Service) ListPendingDependencyLaunches(ctx context.Context) ([]PendingDependencyLaunch, error) {
	if s.blockers == nil {
		return nil, nil
	}
	lister, ok := s.blockers.(dependentTaskLister)
	if !ok {
		return nil, nil
	}
	ids, err := lister.ListTasksWithDependencies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PendingDependencyLaunch, 0, len(ids))
	for _, id := range ids {
		task, err := s.tasks.GetTask(ctx, id)
		if err != nil || task == nil || task.ArchivedAt != nil || task.IsEphemeral {
			continue
		}
		if !HasStartWhenUnblockedIntent(task) {
			continue
		}
		out = append(out, PendingDependencyLaunch{TaskID: task.ID, WorkflowStepID: task.WorkflowStepID})
	}
	return out, nil
}

// dependentTaskLister is the optional "which tasks have edges" read, kept off
// BlockerRepository so existing test doubles keep compiling.
type dependentTaskLister interface {
	ListTasksWithDependencies(ctx context.Context) ([]string, error)
}

// HasStartWhenUnblockedIntent reports whether the task's deferred launch intent
// belongs to a dependency chain rather than to WIP overflow. The two share one
// record; the flag is what tells them apart.
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

// ListDependentTaskIDs returns the tasks directly blocked by taskID.
func (s *Service) ListDependentTaskIDs(ctx context.Context, taskID string) ([]string, error) {
	if s.blockers == nil {
		return nil, ErrDependencyRepositoryUnavailable
	}
	return s.blockers.ListTasksBlockedBy(ctx, taskID)
}

// deleteDependencyEdgesForTask removes both directions of every edge touching
// taskID. task_blockers predates the tasks foreign key, so nothing cascades.
func (s *Service) deleteDependencyEdgesForTask(ctx context.Context, taskID string) {
	if s.blockers == nil {
		return
	}
	cleaner, ok := s.blockers.(taskDependencyCleaner)
	if !ok {
		return
	}
	dependents, err := s.blockers.ListTasksBlockedBy(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to list dependents before edge cleanup",
			zap.String("task_id", taskID), zap.Error(err))
	}
	if err := cleaner.DeleteTaskBlockersForTask(ctx, taskID); err != nil {
		s.logger.Warn("failed to clean up dependency edges for deleted task",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	// Dependents may now be unblocked, so refresh them. Deliberately no
	// auto-start: deletion is not success, and a chain must not advance
	// because a predecessor was removed.
	s.publishDependencyChange(ctx, dependents...)
}
