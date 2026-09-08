package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/common/taskdependencies"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/runs/commentkeys"
	"github.com/kandev/kandev/internal/workflow/engine"

	"go.uber.org/zap"
)

// validPriorities is the canonical set of TEXT priority values stored on
// office tasks.
var validPriorities = map[string]struct{}{
	"critical": {},
	"high":     {},
	"medium":   {},
	"low":      {},
}

// UpdateTaskPriority sets the priority field. Validates the value against
// the four-value enum; rejects anything else.
func (s *DashboardService) UpdateTaskPriority(ctx context.Context, taskID, priority string) error {
	if _, ok := validPriorities[priority]; !ok {
		return fmt.Errorf("invalid priority: %q (must be critical|high|medium|low)", priority)
	}
	if err := s.repo.UpdateTaskPriority(ctx, taskID, priority); err != nil {
		return err
	}
	s.publishTaskUpdated(ctx, taskID, []string{"priority"})
	return nil
}

// SetTaskAssigneeUser sets (or clears, on empty string) the human assignee.
//
// The human assignee is advisory and independent of the agent assignee: it
// records who on the team owns the task, gates nothing, and setting it never
// touches the runner participant. Taking a task over is this write plus a
// prompt, not a lock.
func (s *DashboardService) SetTaskAssigneeUser(ctx context.Context, taskID, userID string) error {
	if s.assigneeWriter == nil {
		return errors.New("human assignee is not available: no assignee writer configured")
	}
	// The task service authorizes the caller and validates the assignee, then
	// persists. Office only mirrors the result onto its own event stream so the
	// board and the open task detail refresh.
	if err := s.assigneeWriter.SetHumanAssignee(ctx, taskID, userID); err != nil {
		return err
	}
	s.publishTaskUpdated(ctx, taskID, []string{"assignee_user_id"})
	return nil
}

// UpdateTaskProjectID sets the project_id field. Empty string clears the
// project. When non-empty, validates that the project belongs to the same
// workspace as the task.
func (s *DashboardService) UpdateTaskProjectID(ctx context.Context, taskID, projectID string) error {
	var taskWS string
	if projectID != "" {
		var err error
		taskWS, err = s.repo.GetTaskWorkspaceID(ctx, taskID)
		if err != nil {
			return fmt.Errorf("resolve task workspace: %w", err)
		}
		projectWS, err := s.repo.GetProjectWorkspaceID(ctx, projectID)
		if err != nil {
			return fmt.Errorf("resolve project workspace: %w", err)
		}
		if projectWS != taskWS {
			return fmt.Errorf("project %s belongs to a different workspace", projectID)
		}
	}
	// Read the pre-write project so a no-op PATCH (destination == current)
	// doesn't trigger a budget re-evaluation below: nothing crossed a
	// threshold, so evaluatePolicy's unconditional alert/exceeded logging
	// would otherwise write a fresh activity row on every retry. A read
	// error is treated as "changed" (fail open to evaluating, the prior
	// behavior) rather than silently skipping. Only read when a
	// reassignment could actually trigger an evaluation.
	needsChangeCheck := projectID != "" && s.projectBudget != nil
	var prevProjectID string
	var prevErr error
	if needsChangeCheck {
		prevProjectID, prevErr = s.repo.GetTaskProjectID(ctx, taskID)
	}

	if err := s.repo.UpdateTaskProjectID(ctx, taskID, projectID); err != nil {
		return err
	}
	// Best-effort: the write above has already committed, so an evaluation
	// error here is logged, not returned. Mirrors event_subscribers.go's
	// post-cost-event budget check. Evaluate before publishing the task event
	// so a client's refetch can observe any activity row created by the check.
	if needsChangeCheck && (prevErr != nil || prevProjectID != projectID) {
		if err := s.projectBudget.EvaluateProjectBudget(ctx, taskWS, projectID); err != nil {
			s.logger.Warn("project budget evaluation failed on reassignment",
				zap.String("task_id", taskID),
				zap.String("project_id", projectID),
				zap.Error(err))
		}
	}
	s.publishTaskUpdated(ctx, taskID, []string{"project_id"})
	return nil
}

// UpdateTaskParentID sets the parent_id field. Empty string clears the
// parent (via the canonical detacher). A non-empty value must reference an
// existing task and also applies the canonical re-parent workspace policy:
// an inherit_parent subtask keeps its materialized workspace as shared_group
// instead of inheriting the new parent's. Rejects only direct self-reference
// (parentID == taskID); deeper cycle detection is intentionally not enforced
// on this surface.
func (s *DashboardService) UpdateTaskParentID(ctx context.Context, taskID, parentID string) error {
	if parentID != "" && parentID == taskID {
		return fmt.Errorf("task cannot be its own parent")
	}
	if parentID == "" {
		if s.taskDetacher == nil {
			return errors.New("task detacher is not configured")
		}
		if _, err := s.taskDetacher.DetachTask(ctx, taskID); err != nil {
			return err
		}
		return nil
	}
	// Preflight existence so a concurrent parent deletion (or a direct hit on
	// the endpoint) cannot write a dangling parent_id — mirroring the
	// canonical resolveParentID guard.
	parent, err := s.repo.GetTaskByID(ctx, parentID)
	if err != nil {
		return fmt.Errorf("resolve parent task: %w", err)
	}
	if parent == nil {
		return fmt.Errorf("parent task not found: %s", parentID)
	}
	if err := s.repo.UpdateTaskParentID(ctx, taskID, parentID); err != nil {
		return err
	}
	s.publishTaskUpdated(ctx, taskID, []string{"parent_id", "metadata"})
	return nil
}

// fieldBlockers is the canonical "fields" entry on OfficeTaskUpdated for a
// blocker mutation. Used in two callsites (add/remove); pulled out as a
// constant per CLAUDE.md ≥3-occurrence rule.
const fieldBlockers = "blockers"

// roleLogKey is the "role" log-field / activity-detail key name, factored
// out because it recurs across the participant claim/removal log lines.
const roleLogKey = "role"

// blockerCycleWalkLimit caps the BFS in detectBlockerCycle as a safety
// bound. Real workspaces are nowhere near this; if we hit it we have
// other problems.
const blockerCycleWalkLimit = 1000

// BlockerCycleError is returned by AddTaskBlocker when the proposed
// blocker would create a cycle. Path lists the task IDs in the cycle in
// traversal order (e.g. ["A","B","C","A"]) so callers can render a
// human-readable error message such as "A → B → C → A".
type BlockerCycleError struct {
	Path []string
}

// Error implements the error interface.
func (e *BlockerCycleError) Error() string {
	if len(e.Path) == 0 {
		return "would create blocker cycle"
	}
	return "would create blocker cycle: " + joinPath(e.Path)
}

// joinPath renders a cycle path as "A → B → C → A".
func joinPath(path []string) string {
	out := ""
	for i, id := range path {
		if i > 0 {
			out += " → "
		}
		out += id
	}
	return out
}

// AddTaskBlocker creates a blocker relationship: taskID is blocked by
// blockerTaskID. Validates self-reference, cross-workspace, and runs a
// BFS forward walk through the existing blocker chain to detect cycles
// of any length. On cycle detection returns a *BlockerCycleError whose
// Path lists the cycle for the caller to surface.
func (s *DashboardService) AddTaskBlocker(ctx context.Context, taskID, blockerTaskID string) error {
	var err error
	func() {
		unlock := taskdependencies.AcquireMutationLock()
		defer unlock()
		if err = s.validateBlockerPair(ctx, taskID, blockerTaskID); err != nil {
			return
		}
		blocker := &models.TaskBlocker{TaskID: taskID, BlockerTaskID: blockerTaskID}
		err = s.repo.CreateTaskBlocker(ctx, blocker)
	}()
	if err != nil {
		return err
	}
	s.publishTaskUpdated(ctx, taskID, []string{fieldBlockers})
	s.logBlockerActivity(ctx, taskID, blockerTaskID, "task_blocker_added")
	return nil
}

// RemoveTaskBlocker deletes a blocker relationship. A delete of an absent
// row is a no-op at the DB level; the event/activity entry are still
// emitted so the UI re-fetches.
func (s *DashboardService) RemoveTaskBlocker(ctx context.Context, taskID, blockerTaskID string) error {
	var err error
	func() {
		unlock := taskdependencies.AcquireMutationLock()
		defer unlock()
		err = s.repo.DeleteTaskBlocker(ctx, taskID, blockerTaskID)
	}()
	if err != nil {
		return err
	}
	s.publishTaskUpdated(ctx, taskID, []string{fieldBlockers})
	s.logBlockerActivity(ctx, taskID, blockerTaskID, "task_blocker_removed")
	return nil
}

// validateBlockerPair runs the validation rules for AddTaskBlocker:
// self-reference, cross-workspace, and a BFS walk of the existing
// blocker graph that returns *BlockerCycleError if adding the proposed
// edge would close a cycle.
func (s *DashboardService) validateBlockerPair(ctx context.Context, taskID, blockerTaskID string) error {
	if blockerTaskID == taskID {
		return fmt.Errorf("task cannot block itself")
	}
	taskWS, err := s.repo.GetTaskWorkspaceID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("resolve task workspace: %w", err)
	}
	blockerWS, err := s.repo.GetTaskWorkspaceID(ctx, blockerTaskID)
	if err != nil {
		return fmt.Errorf("resolve blocker workspace: %w", err)
	}
	if taskWS != "" && blockerWS != "" && taskWS != blockerWS {
		return fmt.Errorf("blocker task %s belongs to a different workspace", blockerTaskID)
	}
	cycle, err := s.detectBlockerCycle(ctx, taskID, blockerTaskID)
	if err != nil {
		return fmt.Errorf("check cycle: %w", err)
	}
	if cycle != nil {
		return cycle
	}
	return nil
}

// detectBlockerCycle walks the blocker graph forward from blockerTaskID
// (BFS) checking whether taskID is reachable. If so, the proposed edge
// (taskID → blockerTaskID, "taskID is blocked by blockerTaskID") would
// close a cycle. Returns a *BlockerCycleError with the proven path or
// nil when no cycle is reachable. Bounded by blockerCycleWalkLimit.
//
// Edge semantics: ListTaskBlockers(t) returns blockers OF t — the tasks
// that block t. Walking those edges follows the same direction the
// proposed edge introduces, so a path back to taskID proves a cycle.
func (s *DashboardService) detectBlockerCycle(
	ctx context.Context, taskID, blockerTaskID string,
) (*BlockerCycleError, error) {
	parent := map[string]string{blockerTaskID: ""}
	queue := []string{blockerTaskID}
	for len(queue) > 0 && len(parent) <= blockerCycleWalkLimit {
		node := queue[0]
		queue = queue[1:]
		blockers, err := s.repo.ListTaskBlockers(ctx, node)
		if err != nil {
			return nil, err
		}
		for _, b := range blockers {
			next := b.BlockerTaskID
			if next == taskID {
				return &BlockerCycleError{
					Path: s.buildCyclePath(ctx, taskID, blockerTaskID, node, parent),
				}, nil
			}
			if _, seen := parent[next]; seen {
				continue
			}
			parent[next] = node
			queue = append(queue, next)
		}
	}
	return nil, nil
}

// buildCyclePath reconstructs the cycle path proving the BFS hit.
// The proposed edge is taskID → blockerTaskID. The BFS reached `node`
// from `blockerTaskID`, and `node` has taskID as one of its blockers
// (closing the cycle). The returned slice is the cycle in traversal
// order: [taskID, blockerTaskID, …, node, taskID]. Identifiers are
// substituted for IDs when cheap (one DB call); falls back to raw IDs.
func (s *DashboardService) buildCyclePath(
	ctx context.Context, taskID, blockerTaskID, node string, parent map[string]string,
) []string {
	// Reconstruct blockerTaskID → … → node by walking parent chain.
	chain := []string{node}
	for cur := parent[node]; cur != ""; cur = parent[cur] {
		chain = append(chain, cur)
	}
	// chain currently ends at blockerTaskID; reverse so it starts there.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	path := append([]string{taskID}, chain...)
	path = append(path, taskID)
	return s.resolveIdentifiers(ctx, path)
}

// resolveIdentifiers replaces task IDs with their human identifiers
// (e.g. "TASK-12") when cheap. Falls back to raw IDs on error or for
// any task missing an identifier. Best-effort — never returns an error.
func (s *DashboardService) resolveIdentifiers(ctx context.Context, ids []string) []string {
	uniq := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	rows, err := s.repo.GetTasksByIDs(ctx, uniq)
	if err != nil {
		return ids
	}
	identByID := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.Identifier != "" {
			identByID[r.ID] = r.Identifier
		}
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		if ident, ok := identByID[id]; ok {
			out[i] = ident
		} else {
			out[i] = id
		}
	}
	return out
}

// logBlockerActivity records an audit entry for an add/remove blocker action.
// Best-effort.
func (s *DashboardService) logBlockerActivity(ctx context.Context, taskID, blockerTaskID, action string) {
	if s.activity == nil {
		return
	}
	wsID, _ := s.repo.GetTaskWorkspaceID(ctx, taskID)
	details, _ := json.Marshal(map[string]string{
		"task_id":         taskID,
		"blocker_task_id": blockerTaskID,
	})
	s.activity.LogActivity(ctx, wsID, userSentinel, "", action, "task", taskID, string(details))
}

// participantFields maps a role to the OfficeTaskUpdated "fields" entry the
// frontend listens for. Kept as a private map so the role→field mapping
// stays in one place.
var participantFields = map[string]string{
	models.ParticipantRoleReviewer: "reviewers",
	models.ParticipantRoleApprover: "approvers",
}

// AddTaskReviewer adds an agent as a reviewer of a task.
func (s *DashboardService) AddTaskReviewer(ctx context.Context, callerAgentID, taskID, agentID string) error {
	return s.addOrRemoveParticipant(ctx, callerAgentID, taskID, agentID, models.ParticipantRoleReviewer, true)
}

// RemoveTaskReviewer removes an agent from the reviewer list.
func (s *DashboardService) RemoveTaskReviewer(ctx context.Context, callerAgentID, taskID, agentID string) error {
	return s.addOrRemoveParticipant(ctx, callerAgentID, taskID, agentID, models.ParticipantRoleReviewer, false)
}

// AddTaskApprover adds an agent as an approver of a task.
func (s *DashboardService) AddTaskApprover(ctx context.Context, callerAgentID, taskID, agentID string) error {
	return s.addOrRemoveParticipant(ctx, callerAgentID, taskID, agentID, models.ParticipantRoleApprover, true)
}

// RemoveTaskApprover removes an agent from the approver list.
func (s *DashboardService) RemoveTaskApprover(ctx context.Context, callerAgentID, taskID, agentID string) error {
	return s.addOrRemoveParticipant(ctx, callerAgentID, taskID, agentID, models.ParticipantRoleApprover, false)
}

// addOrRemoveParticipant is the shared body of the four reviewer/approver
// mutators. It enforces the can_approve permission gate (when a caller
// agent is supplied), writes to the DB, and drives the matching
// OfficeTaskUpdated + activity entry off what the write actually did.
func (s *DashboardService) addOrRemoveParticipant(
	ctx context.Context,
	callerAgentID, taskID, agentID, role string,
	add bool,
) error {
	if err := s.requireApprovePermission(ctx, callerAgentID); err != nil {
		return err
	}
	field, ok := participantFields[role]
	if !ok {
		return fmt.Errorf("invalid participant role: %q", role)
	}
	if add {
		result, err := s.repo.AddTaskParticipant(ctx, taskID, agentID, role)
		if err != nil {
			return err
		}
		s.applyParticipantAddOutcome(ctx, taskID, agentID, role, field, result)
		return nil
	}

	if err := s.repo.RemoveTaskParticipant(ctx, taskID, agentID, role); err != nil {
		return err
	}
	// Flip the participant's office session row to COMPLETED so it leaves
	// the live indicators and the next add would create a fresh row
	// (preserving historical conversation separation). Skipped when the
	// agent still holds another capacity on this task: the removed role was
	// not the only thing addressing it here.
	s.terminateSessionUnlessRetained(ctx, taskID, agentID, sessionTermReasonRoleRemoved,
		zap.String(roleLogKey, role))
	s.publishTaskUpdated(ctx, taskID, []string{field})
	s.logParticipantActivity(ctx, taskID, agentID, role, "task_participant_removed")
	return nil
}

// applyParticipantAddOutcome drives the post-commit side effects an add
// outcome earns. Unchanged earns none — an identity-probe hit, promotion
// included, raises no activity entry and publishes no notification.
// Claimed additionally records the takeover,
// ends the displaced agent's live session, and cancels the run already
// queued for it. Inserted is the plain-registration path, unchanged from
// before this write reported an outcome. No order among these effects is
// contracted.
func (s *DashboardService) applyParticipantAddOutcome(
	ctx context.Context, taskID, agentID, role, field string, result sqlite.ParticipantWriteResult,
) {
	switch result.Outcome {
	case sqlite.ParticipantWriteOutcomeUnchanged:
		return
	case sqlite.ParticipantWriteOutcomeClaimed:
		s.logParticipantClaimActivity(ctx, taskID, result.StepID, role, result.DisplacedAgentProfileID, agentID)
		// Detached from ctx: these run after the claim has already
		// committed, so a caller (HTTP request, WS handler) that cancels
		// after that point must not also cancel the cleanup it earned.
		detachedCtx := context.WithoutCancel(ctx)
		s.terminateDisplacedSession(detachedCtx, taskID, result.DisplacedAgentProfileID, role)
		s.cancelDisplacedRun(detachedCtx, taskID, result.StepID, result.DisplacedAgentProfileID)
	case sqlite.ParticipantWriteOutcomeInserted:
		s.logParticipantActivity(ctx, taskID, agentID, role, "task_participant_added")
	}
	s.publishTaskUpdated(ctx, taskID, []string{field})
}

// terminateDisplacedSession ends the displaced agent's live office session
// for the role a claim just took the seat away from. Best-effort,
// mirroring the removal branch's own termination call: a failure is
// logged, not surfaced. An agent that still runs the task, or is seated in
// another role, keeps the session it is still using.
func (s *DashboardService) terminateDisplacedSession(ctx context.Context, taskID, displacedAgentID, role string) {
	s.terminateSessionUnlessRetained(ctx, taskID, displacedAgentID, sessionTermReasonSeatClaimed,
		zap.String(roleLogKey, role))
}

// cancelDisplacedRun cancels the run the step-entry fan-out queued for the
// displaced agent profile, which the claim's seat reassignment does not
// itself redirect. Best-effort: logged and swallowed on failure — the
// registration has already committed and must still return success,
// leaving at most one run runnable for an agent no longer seated in the
// role.
func (s *DashboardService) cancelDisplacedRun(ctx context.Context, taskID, stepID, displacedAgentID string) {
	if displacedAgentID == "" {
		return
	}
	if _, err := s.repo.CancelDisplacedParticipantRun(ctx, taskID, stepID, displacedAgentID); err != nil {
		s.logger.Warn("cancel displaced participant run failed",
			zap.String("task_id", taskID),
			zap.String("step_id", stepID),
			zap.String("agent_profile_id", displacedAgentID),
			zap.Error(err))
	}
}

// logParticipantClaimActivity records a claim as an activity entry
// distinct from a plain registration's, naming the task, step, role, the
// displaced agent profile and the claiming agent profile. Best-effort.
func (s *DashboardService) logParticipantClaimActivity(
	ctx context.Context, taskID, stepID, role, displacedAgentID, claimingAgentID string,
) {
	if s.activity == nil {
		return
	}
	wsID, _ := s.repo.GetTaskWorkspaceID(ctx, taskID)
	details, _ := json.Marshal(map[string]string{
		"task_id":                    taskID,
		"step_id":                    stepID,
		roleLogKey:                   role,
		"displaced_agent_profile_id": displacedAgentID,
		"claiming_agent_profile_id":  claimingAgentID,
	})
	s.activity.LogActivity(ctx, wsID, userSentinel, "", "task_participant_claimed", "task", taskID, string(details))
}

// requireApprovePermission returns ErrForbidden when the caller agent
// lacks PermCanApprove. callerAgentID="" skips the check (admin/internal
// callers).
func (s *DashboardService) requireApprovePermission(ctx context.Context, callerAgentID string) error {
	if callerAgentID == "" {
		return nil
	}
	agent, err := s.agents.GetAgentInstance(ctx, callerAgentID)
	if err != nil {
		return fmt.Errorf("resolve caller: %w", err)
	}
	perms := shared.ResolvePermissions(shared.AgentRole(agent.Role), agent.Permissions)
	if !shared.HasPermission(perms, shared.PermCanApprove) {
		return shared.ErrForbidden
	}
	return nil
}

// logParticipantActivity records an audit entry for a reviewer/approver
// add or remove. Best-effort.
func (s *DashboardService) logParticipantActivity(
	ctx context.Context, taskID, agentID, role, action string,
) {
	if s.activity == nil {
		return
	}
	wsID, _ := s.repo.GetTaskWorkspaceID(ctx, taskID)
	details, _ := json.Marshal(map[string]string{
		"task_id":          taskID,
		"agent_profile_id": agentID,
		"role":             role,
	})
	s.activity.LogActivity(ctx, wsID, userSentinel, "", action, "task", taskID, string(details))
}

// ListTaskParticipants returns participants for a task filtered by role.
func (s *DashboardService) ListTaskParticipants(ctx context.Context, taskID, role string) ([]sqlite.Participant, error) {
	return s.repo.ListTaskParticipants(ctx, taskID, role)
}

// ListAllTaskParticipants returns every participant for a task. Used by
// the task DTO to surface reviewers + approvers in one query.
func (s *DashboardService) ListAllTaskParticipants(ctx context.Context, taskID string) ([]sqlite.Participant, error) {
	return s.repo.ListAllTaskParticipants(ctx, taskID)
}

// TaskStatusUpdateRequest carries the fields for a status-based task update.
type TaskStatusUpdateRequest struct {
	TaskID       string
	NewStatus    string
	Comment      string
	ActorAgentID string
	// ReopenIntent=true marks this status change as a reopen — used by the
	// reactivity pipeline to prefer the task_reopened* run reasons.
	ReopenIntent bool
	// ResumeIntent=true is an explicit "pick this up again" with required
	// follow-up comment. Forces task_reopened_via_comment.
	ResumeIntent bool
}

// UpdateTaskStatus persists a new task status, optionally creates a comment,
// and publishes an office.task.status_changed event so execution policy
// subscribers can apply stage transitions.
//
// When a ReactivityApplier is set, the function also runs the office
// reactivity pipeline AFTER the DB write to queue downstream runs
// (dependents unblocked, parent children-completed, reopen intent,
// session interrupt for cancellation, etc.).
//
// Approval gate: when the requested status is "done" and the task has
// approvers without a current approved decision, the persisted state
// is redirected to in_review and the function returns a typed
// *ApprovalsPendingError (the handler maps it to HTTP 409). The
// req.NewStatus is updated in place so downstream side-effects see
// the redirected status.
//
// Rework / reopen: transitions leaving in_review for todo|in_progress,
// and transitions leaving done for any non-terminal state, supersede
// every active decision so the next review round starts fresh.
func (s *DashboardService) UpdateTaskStatus(ctx context.Context, req TaskStatusUpdateRequest) error {
	dbState := normaliseStatus(req.NewStatus)
	if dbState == "" {
		return &InvalidTaskStatusError{Status: req.NewStatus}
	}

	preStatus := ""
	if exec, err := s.repo.GetTaskExecutionFields(ctx, req.TaskID); err == nil && exec != nil {
		preStatus = exec.State
	}

	gateErr := s.applyApprovalGate(ctx, req.TaskID, &dbState, &req.NewStatus)

	if err := s.repo.UpdateTaskState(ctx, req.TaskID, dbState); err != nil {
		return fmt.Errorf("update task state: %w", err)
	}

	commentID := s.maybeCreateStatusComment(ctx, req)
	s.logTaskStatusChangeActivity(ctx, req)
	s.publishTaskStatusChanged(ctx, req)
	s.runReactivityForStatus(ctx, req, commentID, preStatus)
	s.maybeSupersedeOnRework(ctx, req.TaskID, preStatus, dbState)

	return gateErr
}

// UpdateTaskStatusAsAgent adapts the dashboard status mutation to the Office
// runtime action surface.
func (s *DashboardService) UpdateTaskStatusAsAgent(
	ctx context.Context,
	update officeruntime.TaskStatusUpdate,
) error {
	return s.UpdateTaskStatus(ctx, TaskStatusUpdateRequest{
		TaskID:       update.TaskID,
		NewStatus:    update.NewStatus,
		Comment:      update.Comment,
		ActorAgentID: update.ActorAgentID,
	})
}

// logTaskStatusChangeActivity records the task_status_changed activity row
// synchronously, in-process, before the OfficeTaskStatusChanged event is
// published. This used to be an async event-bus subscriber
// (office/service.Service.handleTaskStatusChanged); under a NATS-backed
// bus, that subscriber and the WS broadcast subscriber that triggers the
// frontend's task-detail refetch raced independently, so a browser GET
// could land before the activity row existed and see stale Started/
// Completed sidebar values with nothing left to trigger a corrective
// refetch. Writing the row here, ahead of publishTaskStatusChanged,
// makes the row durable before the event (and therefore the broadcast)
// can ever fire, regardless of bus implementation. Best-effort: mirrors
// logBlockerActivity's activity==nil guard.
func (s *DashboardService) logTaskStatusChangeActivity(ctx context.Context, req TaskStatusUpdateRequest) {
	if s.activity == nil {
		return
	}
	wsID, _ := s.repo.GetTaskWorkspaceID(ctx, req.TaskID)
	actorType := "system"
	actorID := "office-scheduler"
	if req.ActorAgentID != "" {
		actorType = activityActorTypeAgent
		actorID = req.ActorAgentID
	}
	runID := ""
	if s.runResolver != nil {
		runID = s.runResolver.ResolveRunForTask(ctx, req.TaskID)
	}
	s.activity.LogActivityWithRun(ctx, wsID, actorType, actorID,
		"task_status_changed", "task", req.TaskID,
		fmt.Sprintf(`{"new_status":%q}`, req.NewStatus), runID, "")
}

// applyApprovalGate redirects a "done" transition to in_review when
// approvals are pending. Mutates dbState and apiStatus in place so the
// rest of UpdateTaskStatus persists and emits the redirected status.
// Returns a *ApprovalsPendingError when the gate fires; nil otherwise.
func (s *DashboardService) applyApprovalGate(
	ctx context.Context, taskID string, dbState, apiStatus *string,
) error {
	if *dbState != stateCompleted {
		return nil
	}
	pending, err := s.pendingApprovers(ctx, taskID)
	if err != nil || len(pending) == 0 {
		return nil
	}
	*dbState = stateInReview
	*apiStatus = statusInReviewLowercase
	return &ApprovalsPendingError{Pending: pending}
}

// maybeCreateStatusComment creates the optional status-change comment
// and returns its ID. Best-effort.
func (s *DashboardService) maybeCreateStatusComment(
	ctx context.Context, req TaskStatusUpdateRequest,
) string {
	if req.Comment == "" {
		return ""
	}
	comment := &models.TaskComment{
		TaskID:     req.TaskID,
		AuthorType: "agent",
		AuthorID:   req.ActorAgentID,
		Body:       req.Comment,
		Source:     "agent",
	}
	if err := s.repo.CreateTaskComment(ctx, comment); err != nil {
		s.logger.Warn("failed to create comment on status update",
			zap.String("task_id", req.TaskID), zap.Error(err))
		return ""
	}
	return comment.ID
}

// maybeSupersedeOnRework clears active decisions when a task is
// moving out of in_review back to todo|in_progress (rework) or out
// of done back to any non-terminal state (reopen). Best-effort.
func (s *DashboardService) maybeSupersedeOnRework(
	ctx context.Context, taskID, preDB, nextDB string,
) {
	if preDB == "" || preDB == nextDB {
		return
	}
	switch preDB {
	case stateInReview:
		if nextDB == stateTODO || nextDB == stateInProgress {
			s.supersedeAndLog(ctx, taskID)
		}
	case stateCompleted:
		if nextDB != stateCompleted && nextDB != stateCancelled {
			s.supersedeAndLog(ctx, taskID)
		}
	}
}

// runReactivityForStatus invokes the reactivity pipeline for a status
// mutation and triggers any post-pipeline side effects (hard cancel).
// Best-effort — failures are logged, never propagated.
func (s *DashboardService) runReactivityForStatus(
	ctx context.Context, req TaskStatusUpdateRequest, commentID, preStatus string,
) {
	if s.reactivity == nil {
		return
	}
	actorType := userSentinel
	if req.ActorAgentID != "" {
		actorType = "agent"
	}
	change := TaskReactivityChange{
		NewStatus:    &req.NewStatus,
		ActorID:      req.ActorAgentID,
		ActorType:    actorType,
		ReopenIntent: req.ReopenIntent,
		ResumeIntent: req.ResumeIntent,
	}
	if commentID != "" {
		change.Comment = &TaskReactivityComment{
			ID:         commentID,
			Body:       req.Comment,
			AuthorType: actorType,
			AuthorID:   req.ActorAgentID,
		}
	}
	result, err := s.reactivity.ApplyTaskMutation(ctx, req.TaskID, preStatus, change)
	if err != nil {
		s.logger.Warn("reactivity pipeline failed",
			zap.String("task_id", req.TaskID), zap.Error(err))
		return
	}
	if result != nil && result.InterruptSessionID != "" && s.taskCanceller != nil {
		// Hard-cancel the active session for status→cancelled.
		go s.hardCancelTaskAsync(result.InterruptSessionID)
	}
}

func (s *DashboardService) hardCancelTaskAsync(taskID string) {
	bg := context.Background()
	if err := s.taskCanceller.CancelTaskExecution(bg, taskID, "status_changed_to_cancelled", true); err != nil {
		s.logger.Warn("hard-cancel after status→cancelled failed",
			zap.String("task_id", taskID), zap.Error(err))
	}
}

// normaliseStatus maps CLI/API status values to the DB state column value.
func normaliseStatus(status string) string {
	switch status {
	case statusDoneLowercase, stateCompleted:
		return stateCompleted
	case statusInProgressLowercase, stateInProgress:
		return stateInProgress
	case statusTODOLowercase, stateTODO:
		return stateTODO
	case statusInReviewLowercase, stateInReview, statusReviewLowercase:
		return stateInReview
	case statusBlockedLowercase, stateBlocked:
		return stateBlocked
	case statusCancelledLowercase, stateCancelled:
		return stateCancelled
	case statusBacklogLowercase, stateBacklog:
		return stateBacklog
	default:
		return ""
	}
}

// dbStateToOfficeStatus maps the persisted tasks.state value (uppercase
// kanban enum, e.g. "COMPLETED") to the office canonical lowercase
// vocabulary returned to clients (e.g. "done"). Pass-through for already
// lowercased values; empty string falls back to "backlog" so the
// frontend's status-picker has a defined value to render.
func dbStateToOfficeStatus(state string) string {
	switch state {
	case stateCompleted, statusDoneLowercase:
		return statusDoneLowercase
	case stateInProgress, statusInProgressLowercase:
		return statusInProgressLowercase
	case stateInReview, statusInReviewLowercase, statusReviewLowercase:
		return statusInReviewLowercase
	case stateTODO, statusTODOLowercase:
		return statusTODOLowercase
	case stateBlocked, statusBlockedLowercase:
		return statusBlockedLowercase
	case stateCancelled, statusCancelledLowercase:
		return statusCancelledLowercase
	case stateBacklog, statusBacklogLowercase, "":
		return statusBacklogLowercase
	default:
		return state
	}
}

// SetTaskAssigneeAsAgent checks can_assign_tasks for the given caller before
// updating the task's assignee. Passing callerAgentID="" skips the check
// (for internal/admin callers).
func (s *DashboardService) SetTaskAssigneeAsAgent(ctx context.Context, callerAgentID, taskID, assigneeID string) error {
	if callerAgentID != "" {
		agent, err := s.agents.GetAgentInstance(ctx, callerAgentID)
		if err != nil {
			return fmt.Errorf("resolve caller: %w", err)
		}
		perms := shared.ResolvePermissions(shared.AgentRole(agent.Role), agent.Permissions)
		if !shared.HasPermission(perms, shared.PermCanAssignTasks) {
			return shared.ErrForbidden
		}
	}
	// Capture the pre-update assignee so the reactivity pipeline can
	// detect a real change and hard-cancel the previous session.
	prevAssignee := ""
	if exec, err := s.repo.GetTaskExecutionFields(ctx, taskID); err == nil && exec != nil {
		prevAssignee = exec.AssigneeAgentProfileID
	}

	if s.retryCanceller != nil {
		if err := s.retryCanceller.CancelPendingRetriesForTask(ctx, taskID); err != nil {
			s.logger.Warn("failed to cancel pending retries on reassign",
				zap.String("task_id", taskID), zap.Error(err))
		}
	}
	if err := s.repo.UpdateTaskAssignee(ctx, taskID, assigneeID); err != nil {
		return err
	}

	s.publishTaskUpdated(ctx, taskID, []string{"assignee_agent_profile_id"})

	// Reactivity pipeline — wakes the new assignee with task_assigned
	// and hard-cancels the previous assignee's active session.
	s.runReactivityForAssigneeChange(ctx, taskID, prevAssignee, assigneeID, callerAgentID)
	return nil
}

// runReactivityForAssigneeChange invokes the reactivity pipeline for an
// assignee change. Best-effort — failures are logged, never propagated.
func (s *DashboardService) runReactivityForAssigneeChange(
	ctx context.Context, taskID, prevAssigneeID, newAssigneeID, callerAgentID string,
) {
	if s.reactivity == nil {
		return
	}
	actorType := userSentinel
	if callerAgentID != "" {
		actorType = "agent"
	}
	change := TaskReactivityChange{
		NewAssigneeID:  &newAssigneeID,
		PrevAssigneeID: prevAssigneeID,
		ActorID:        callerAgentID,
		ActorType:      actorType,
	}
	// preStatus="" — assignee changes don't depend on the prev status.
	result, err := s.reactivity.ApplyTaskMutation(ctx, taskID, "", change)
	if err != nil {
		s.logger.Warn("reactivity pipeline failed (assignee change)",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if result != nil && result.InterruptSessionID != "" && s.taskCanceller != nil {
		go s.hardCancelTaskAsync(result.InterruptSessionID)
	}
	// Flip the prev assignee's office session row to COMPLETED so it leaves
	// the active sessions list. The reactivity pipeline already hard-cancels
	// the running execution above, unconditionally: an agent reassigned away
	// but still seated as a reviewer stops the turn it no longer owns and
	// keeps the row it still reviews through. Those are two different objects
	// and only this one is guarded.
	s.terminateSessionUnlessRetained(ctx, taskID, prevAssigneeID, sessionTermReasonReassigned)
	// Auto-dismiss any inbox entry tied to the prior (task, agent) so
	// the user isn't asked to triage a failure they already worked
	// around by reassigning. Counter is intentionally not reset — the
	// root cause may still be unfixed for the old agent.
	if prevAssigneeID != "" && s.failureNotifier != nil {
		s.failureNotifier.OnAssigneeChanged(ctx, taskID, prevAssigneeID)
	}
}

// sessionTermReason* are the reasons stored on the session row when it goes
// COMPLETED via SessionTerminator. Mostly informational, surfaced in logs.
const (
	sessionTermReasonReassigned   = "task_reassigned"
	sessionTermReasonRoleRemoved  = "participant_removed"
	sessionTermReasonAgentDeleted = "agent_instance_deleted"
	sessionTermReasonSeatClaimed  = "participant_seat_claimed"
)

// retainedCapacity names a standing that keeps an agent addressed on a task.
// Two exist, and either one alone keeps the pair's office session live.
type retainedCapacity string

const (
	capacityNone   retainedCapacity = ""
	capacityRunner retainedCapacity = "runner"
	capacitySeat   retainedCapacity = "seat"
)

// retainsTaskCapacity reports which capacity, if any, agentProfileID still
// holds on taskID.
//
// The runner half reads the task's execution fields, whose assignee value is
// the shared runner projection. Resolving the runner any other way — a direct
// read of runner seats, say — answers a different question, because the
// projection's last tier deliberately reads runner rows across every step.
//
// The seat half reads the effective slate at the task's current step, which
// already merges template-level rows under per-task precedence and already
// spans every role. The step scope is load-bearing: seats are keyed
// (step, task, role, agent) and survive a step change, so an unscoped read
// would find a seat naming a previous occupant and suppress every future
// termination for the pair.
//
// Both reads run on the read pool and take no lock. The determination is not
// atomic with the mutation that preceded it and does not need to be: each
// guarded path reads strictly after its own commit, so two concurrent paths
// removing different capacities end with the session terminated by whichever
// reads second.
func (s *DashboardService) retainsTaskCapacity(
	ctx context.Context, taskID, agentProfileID string,
) (retainedCapacity, error) {
	exec, err := s.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		return capacityNone, fmt.Errorf("read task runner: %w", err)
	}
	if exec != nil && exec.AssigneeAgentProfileID == agentProfileID {
		return capacityRunner, nil
	}
	seats, err := s.repo.ListAllTaskParticipants(ctx, taskID)
	if err != nil {
		return capacityNone, fmt.Errorf("read task participant slate: %w", err)
	}
	for _, seat := range seats {
		if seat.AgentProfileID == agentProfileID {
			return capacitySeat, nil
		}
	}
	return capacityNone, nil
}

// terminateSessionUnlessRetained is the single guarded termination step,
// shared by the three paths that end a session on the loss of one capacity:
// role removal, seat claim displacement, and reassignment. It ends the pair's
// session only when the agent has lost its last capacity on the task.
//
// It fails closed. A capacity read that errors suppresses the termination,
// because a session left live is recovered by the next guarded path that runs
// on the pair, and a session wrongly ended is not.
//
// Suppression is not an error: every caller here already treats termination as
// best-effort, and a suppressed termination is a normal outcome.
func (s *DashboardService) terminateSessionUnlessRetained(
	ctx context.Context, taskID, agentProfileID, reason string, extra ...zap.Field,
) {
	if s.sessionTerm == nil || taskID == "" || agentProfileID == "" {
		return
	}
	fields := append([]zap.Field{
		zap.String("task_id", taskID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("reason", reason),
	}, extra...)

	capacity, err := s.retainsTaskCapacity(ctx, taskID, agentProfileID)
	if err != nil {
		s.logger.Warn("office session termination suppressed: capacity read failed",
			append(fields, zap.Error(err))...)
		return
	}
	if capacity != capacityNone {
		s.logger.Info("office session termination suppressed: agent retains a capacity",
			append(fields, zap.String("retained_capacity", string(capacity)))...)
		return
	}
	if err := s.sessionTerm.TerminateOfficeSession(ctx, taskID, agentProfileID, reason); err != nil {
		s.logger.Warn("terminate office session failed", append(fields, zap.Error(err))...)
	}
}

// publishTaskUpdated emits an OfficeTaskUpdated event listing the fields
// that changed. Frontend subscribers re-fetch the task DTO. Silently
// skipped when no event bus is configured.
func (s *DashboardService) publishTaskUpdated(ctx context.Context, taskID string, fields []string) {
	if s.eb == nil || len(fields) == 0 {
		return
	}
	wsID, err := s.repo.GetTaskWorkspaceID(ctx, taskID)
	if err != nil {
		s.logger.Warn("publish task updated: resolve workspace failed",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	data := map[string]any{
		"task_id":      taskID,
		"workspace_id": wsID,
		"fields":       fields,
	}
	event := bus.NewEvent(events.OfficeTaskUpdated, "office-dashboard", data)
	if err := s.eb.Publish(ctx, events.OfficeTaskUpdated, event); err != nil {
		s.logger.Error("publish task updated event failed",
			zap.String("task_id", taskID), zap.Error(err))
	}
}

// publishTaskStatusChanged emits an OfficeTaskStatusChanged event so
// the office service event bus subscribers can drive activity logging
// for human/agent status updates. The legacy ExecutionPolicy gate was
// dropped in Phase 4 of task-model-unification — every status change
// now publishes an event regardless of policy.
func (s *DashboardService) publishTaskStatusChanged(ctx context.Context, req TaskStatusUpdateRequest) {
	if s.eb == nil {
		return
	}
	wsID, _ := s.repo.GetTaskWorkspaceID(ctx, req.TaskID)
	data := map[string]string{
		"task_id":        req.TaskID,
		"workspace_id":   wsID,
		"new_status":     req.NewStatus,
		"comment":        req.Comment,
		"actor_agent_id": req.ActorAgentID,
	}
	event := bus.NewEvent(events.OfficeTaskStatusChanged, "office-dashboard", data)
	if err := s.eb.Publish(ctx, events.OfficeTaskStatusChanged, event); err != nil {
		s.logger.Error("publish task status changed event failed",
			zap.String("task_id", req.TaskID), zap.Error(err))
	}
}

// runReactivityForComment fires the pipeline for a standalone comment
// (one not attached to a status update). Wakes the assignee unless the
// comment is a self-comment or the task is closed; resolves @mentions.
// Best-effort.
func (s *DashboardService) runReactivityForComment(
	ctx context.Context, comment *models.TaskComment, engineHandled bool,
) {
	if s.reactivity == nil || comment == nil || comment.ID == "" {
		return
	}
	change := TaskReactivityChange{
		ActorID:   comment.AuthorID,
		ActorType: comment.AuthorType,
		Comment: &TaskReactivityComment{
			ID:         comment.ID,
			Body:       comment.Body,
			AuthorType: comment.AuthorType,
			AuthorID:   comment.AuthorID,
		},
		SkipAssigneeCommentWake: engineHandled,
	}
	if _, err := s.reactivity.ApplyTaskMutation(ctx, comment.TaskID, "", change); err != nil {
		s.logger.Warn("reactivity pipeline failed (comment)",
			zap.String("task_id", comment.TaskID), zap.Error(err))
	}
}

func (s *DashboardService) dispatchCommentEngineTrigger(ctx context.Context, comment *models.TaskComment) bool {
	if s.engineDispatcher == nil || comment == nil || comment.TaskID == "" || comment.ID == "" {
		return false
	}
	if s.isSelfComment(ctx, comment) {
		return false
	}
	handled, err := s.dispatchCommentEngineTriggerOnce(ctx, comment)
	if err == nil {
		return handled
	}
	if errors.Is(err, shared.ErrEngineNoSession) {
		return false
	}
	s.logger.Warn("engine comment trigger failed",
		zap.String("task_id", comment.TaskID),
		zap.String("comment_id", comment.ID),
		zap.Error(err))
	return false
}

type handledWorkflowEngineDispatcher interface {
	HandleTriggerHandled(ctx context.Context, taskID string, trigger engine.Trigger, payload any, operationID string) (bool, error)
}

func (s *DashboardService) dispatchCommentEngineTriggerOnce(ctx context.Context, comment *models.TaskComment) (bool, error) {
	payload := engine.OnCommentPayload{
		CommentID: comment.ID,
		AuthorID:  comment.AuthorID,
	}
	opID := commentkeys.TaskComment(comment.ID)
	if dispatcher, ok := s.engineDispatcher.(handledWorkflowEngineDispatcher); ok {
		return dispatcher.HandleTriggerHandled(ctx, comment.TaskID, engine.TriggerOnComment, payload, opID)
	}
	err := s.engineDispatcher.HandleTrigger(ctx, comment.TaskID, engine.TriggerOnComment,
		payload, opID)
	return err == nil, err
}

func (s *DashboardService) isSelfComment(ctx context.Context, comment *models.TaskComment) bool {
	if comment.AuthorType != "agent" || comment.AuthorID == "" {
		return false
	}
	fields, err := s.repo.GetTaskExecutionFields(ctx, comment.TaskID)
	return err == nil && fields != nil && fields.AssigneeAgentProfileID == comment.AuthorID
}

func (s *DashboardService) publishCommentCreated(ctx context.Context, comment *models.TaskComment, engineHandled bool) {
	if s.eb == nil {
		return
	}
	data := map[string]string{
		"task_id":     comment.TaskID,
		"comment_id":  comment.ID,
		"author_type": comment.AuthorType,
		"author_id":   comment.AuthorID,
	}
	if engineHandled {
		data["engine_dispatched"] = commentkeys.EngineDispatchedValue
	}
	event := bus.NewEvent(events.OfficeCommentCreated, "office-dashboard", data)
	if err := s.eb.Publish(ctx, events.OfficeCommentCreated, event); err != nil {
		s.logger.Error("publish comment created event failed",
			zap.String("task_id", comment.TaskID),
			zap.String("comment_id", comment.ID),
			zap.Error(err))
	}
}
