package service

import (
	"context"
	"errors"
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
	// dependencyMissing marks an edge whose predecessor row is gone. Internal
	// only: such edges are dropped from the projection rather than reported.
	dependencyMissing = "missing"
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

// validateDependencyPair resolves both ends and rejects a cross-workspace edge.
// Both tasks must exist: an edge to a missing task would leave the dependent
// blocked on something that can never complete.
func (s *Service) validateDependencyPair(ctx context.Context, taskID, dependsOnTaskID string) error {
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
	return nil
}

// authorizeDependencyPair guards BOTH ends of an edge before any read or write.
//
// Both IDs are caller-supplied, so guarding only the dependent would let a
// caller link (or unlink) another user's task and infer its existence from the
// validation error. Denials surface as ErrTaskNotFound, so there is no
// existence leak.
func (s *Service) authorizeDependencyPair(ctx context.Context, taskID, dependsOnTaskID string) error {
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return err
	}
	return s.authorizeTaskID(ctx, dependsOnTaskID)
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
	if err := s.authorizeDependencyPair(ctx, taskID, dependsOnTaskID); err != nil {
		return err
	}
	// Serialize validate-then-insert. The cycle walk and the insert are two
	// statements, so two concurrent adds can each pass a walk that cannot yet
	// see the other's edge and commit a cycle between them, leaving both tasks
	// blocked forever. Kandev runs one backend process (see the run-scheduling
	// spec, which rules out multiple active processes), so a process-wide lock
	// is sufficient and keeps the check and the write atomic with respect to
	// each other.
	if err := s.insertDependencyEdge(ctx, taskID, dependsOnTaskID); err != nil {
		return err
	}
	// Publish OUTSIDE the lock: event delivery is synchronous, so holding the
	// lock across it would serialize every dependency mutation behind fan-out,
	// and a subscriber that called back into this method would deadlock on a
	// non-reentrant mutex.
	s.publishDependencyChange(ctx, taskID, dependsOnTaskID)
	return nil
}

// insertDependencyEdge holds dependencyEdgeMu across exactly the validate,
// cycle-walk and insert, and nothing else.
func (s *Service) insertDependencyEdge(ctx context.Context, taskID, dependsOnTaskID string) error {
	s.dependencyEdgeMu.Lock()
	defer s.dependencyEdgeMu.Unlock()
	if err := s.validateDependencyPair(ctx, taskID, dependsOnTaskID); err != nil {
		return err
	}
	if cycle, err := s.checkDependencyCycle(ctx, taskID, dependsOnTaskID); err != nil {
		return fmt.Errorf("check dependency cycle: %w", err)
	} else if cycle != nil {
		return cycle
	}
	return s.createBlockerEdge(ctx, taskID, dependsOnTaskID)
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
	if err := s.authorizeDependencyPair(ctx, taskID, dependsOnTaskID); err != nil {
		return err
	}
	if err := s.deleteDependencyEdge(ctx, taskID, dependsOnTaskID); err != nil {
		return err
	}
	// Published outside the lock, for the same reason as AddDependency.
	s.publishDependencyChange(ctx, taskID, dependsOnTaskID)
	return nil
}

// deleteDependencyEdge holds the edge lock across the delete only.
func (s *Service) deleteDependencyEdge(ctx context.Context, taskID, dependsOnTaskID string) error {
	s.dependencyEdgeMu.Lock()
	defer s.dependencyEdgeMu.Unlock()
	return s.blockers.DeleteTaskBlocker(ctx, taskID, dependsOnTaskID)
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
		// Carry the recomputed projection on the event. A bare task.updated
		// omits these keys, and the client reads an omitted projection as
		// "unchanged" (it must, because most task.updated publishers are
		// lightweight) — so without them every other open board would keep a
		// stale chip and badge until a full refetch.
		s.publishTaskEventWithExtra(ctx, events.TaskUpdated, task, nil, s.dependencyEventFields(ctx, task))
	}
}

// dependencyEventFields renders one task's derived projection in the wire shape
// the client's task.updated mapper reads.
func (s *Service) dependencyEventFields(ctx context.Context, task *models.Task) map[string]interface{} {
	views := s.BuildDependencyViews(ctx, []*models.Task{task})
	view := views[task.ID]
	return map[string]interface{}{
		"blocked":        view.Blocked,
		"blocked_reason": view.BlockedReason,
		"depends_on":     view.DependsOn,
		"blocks":         view.Blocks,
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
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	// One batched read: a board payload references every edge on every card, so
	// a per-edge query turned one board load into N round trips.
	found, batchErr := s.tasks.GetTasksByIDs(ctx, ids)
	byID := make(map[string]*models.Task, len(found))
	for _, task := range found {
		if task != nil {
			byID[task.ID] = task
		}
	}
	for _, id := range ids {
		task := byID[id]
		if task == nil {
			// Absent row (deleted predecessor, dangling edge) versus a failed
			// read: the former must not block forever, the latter must not open
			// the gate. Mark them differently so buildDependencyView can tell.
			status := dependencyMissing
			if batchErr != nil {
				status = DependencyPending
			}
			refs[id] = DependencyRef{ID: id, Status: status}
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
	tally := dependencyTally{}
	for _, id := range predecessorIDs {
		ref := refs[id]
		if ref.ID == "" {
			ref = DependencyRef{ID: id, Status: DependencyPending}
		}
		if ref.Status == dependencyMissing {
			// Dangling edge to a deleted task: not shown and not counted, so a
			// failed cleanup cannot block a dependent forever.
			continue
		}
		view.DependsOn = append(view.DependsOn, ref)
		tally.add(ref.Status)
	}
	for _, id := range dependentIDs {
		ref := refs[id]
		if ref.ID == "" {
			ref = DependencyRef{ID: id, Status: DependencyPending}
		}
		view.Blocks = append(view.Blocks, ref)
	}
	view.Blocked, view.BlockedReason = tally.verdict()
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
	tally := dependencyTally{}
	for _, b := range blockers {
		predecessor, getErr := s.tasks.GetTask(ctx, b.BlockerTaskID)
		if getErr != nil && !errors.Is(getErr, taskrepo.ErrTaskNotFound) {
			return true, BlockedReasonUnknown, getErr
		}
		if predecessor == nil {
			// The predecessor row is genuinely gone: this is a dangling edge
			// left behind when delete-time cleanup failed. Counting it as
			// pending would block the dependent forever, since a deleted task
			// can never reach a terminal state. A deleted predecessor is
			// specified to unblock (without firing a launch intent), so ignore
			// the edge and prune it opportunistically.
			//
			// This is NOT the fail-open case: a read ERROR above still fails
			// closed. Only a confirmed-absent row is ignored.
			s.pruneDanglingEdge(ctx, taskID, b.BlockerTaskID)
			continue
		}
		tally.add(DependencyStatusForTask(predecessor))
	}
	blocked, reason = tally.verdict()
	return blocked, reason, nil
}

// dependencyTally accumulates predecessor verdicts and applies the single
// precedence rule. The gate and the derived projection both use it: two copies
// of "pending wins over failed" could drift, and the fail-closed behaviour is
// exactly the thing that must not.
type dependencyTally struct {
	anyPending bool
	anyFailed  bool
}

func (d *dependencyTally) add(status string) {
	switch status {
	case DependencyFailed:
		d.anyFailed = true
	case DependencyResolved, dependencyMissing:
		// Resolved satisfies the edge; missing is a dangling edge that is
		// dropped entirely, so neither blocks.
	default:
		d.anyPending = true
	}
}

func (d *dependencyTally) verdict() (bool, string) {
	switch {
	case d.anyPending:
		return true, BlockedReasonPending
	case d.anyFailed:
		return true, BlockedReasonFailed
	}
	return false, ""
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
// belongs to a dependency chain rather than to WIP overflow.
//
// Thin re-export of the models helper so the orchestrator, the DTO layer and
// this service cannot drift on what counts as a chain intent.
func HasStartWhenUnblockedIntent(task *models.Task) bool {
	return models.HasStartWhenUnblockedIntent(task)
}

// pruneDanglingEdge removes an edge whose predecessor row no longer exists.
//
// Best-effort and non-fatal: the caller has already decided to ignore the edge,
// so a failed prune only means the next read prunes it again.
func (s *Service) pruneDanglingEdge(ctx context.Context, taskID, missingTaskID string) {
	if s.blockers == nil {
		return
	}
	if err := s.blockers.DeleteTaskBlocker(ctx, taskID, missingTaskID); err != nil {
		s.logger.Warn("failed to prune dangling dependency edge",
			zap.String("task_id", taskID),
			zap.String("missing_task_id", missingTaskID),
			zap.Error(err))
	}
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
