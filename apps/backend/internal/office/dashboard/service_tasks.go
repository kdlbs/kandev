package dashboard

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	"github.com/kandev/kandev/internal/office/shared"

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

// UpdateTaskProjectID sets the project_id field. Empty string clears the
// project. When non-empty, validates that the project belongs to the same
// workspace as the task.
func (s *DashboardService) UpdateTaskProjectID(ctx context.Context, taskID, projectID string) error {
	if projectID != "" {
		taskWS, err := s.repo.GetTaskWorkspaceID(ctx, taskID)
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
	if err := s.repo.UpdateTaskProjectID(ctx, taskID, projectID); err != nil {
		return err
	}
	s.publishTaskUpdated(ctx, taskID, []string{"project_id"})
	return nil
}

// UpdateTaskParentID sets the parent_id field. Empty string clears the
// parent. Rejects only direct self-reference (parentID == taskID); deeper
// cycle detection is intentionally not enforced.
func (s *DashboardService) UpdateTaskParentID(ctx context.Context, taskID, parentID string) error {
	if parentID != "" && parentID == taskID {
		return fmt.Errorf("task cannot be its own parent")
	}
	if err := s.repo.UpdateTaskParentID(ctx, taskID, parentID); err != nil {
		return err
	}
	s.publishTaskUpdated(ctx, taskID, []string{"parent_id"})
	return nil
}

// fieldBlockers is the canonical "fields" entry on OfficeTaskUpdated for a
// blocker mutation. Used in two callsites (add/remove); pulled out as a
// constant per CLAUDE.md ≥3-occurrence rule.
const fieldBlockers = "blockers"

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
	if err := s.validateBlockerPair(ctx, taskID, blockerTaskID); err != nil {
		return err
	}
	blocker := &models.TaskBlocker{TaskID: taskID, BlockerTaskID: blockerTaskID}
	if err := s.repo.CreateTaskBlocker(ctx, blocker); err != nil {
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
	if err := s.repo.DeleteTaskBlocker(ctx, taskID, blockerTaskID); err != nil {
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
// agent is supplied), writes to the DB, and publishes the matching
// OfficeTaskUpdated + activity entry.
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
	var dbErr error
	action := "task_participant_removed"
	if add {
		dbErr = s.repo.AddTaskParticipant(ctx, taskID, agentID, role)
		action = "task_participant_added"
	} else {
		dbErr = s.repo.RemoveTaskParticipant(ctx, taskID, agentID, role)
	}
	if dbErr != nil {
		return dbErr
	}
	// On removal, flip the participant's office session row to COMPLETED so
	// it leaves the live indicators and the next add would create a fresh
	// row (preserving historical conversation separation).
	if !add && s.sessionTerm != nil {
		if err := s.sessionTerm.TerminateOfficeSession(ctx, taskID, agentID, sessionTermReasonRoleRemoved); err != nil {
			s.logger.Warn("terminate office session on participant removal failed",
				zap.String("task_id", taskID),
				zap.String("agent_profile_id", agentID),
				zap.String("role", role),
				zap.Error(err))
		}
	}
	s.publishTaskUpdated(ctx, taskID, []string{field})
	s.logParticipantActivity(ctx, taskID, agentID, role, action)
	return nil
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
		return fmt.Errorf("unknown status: %s", req.NewStatus)
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
	// the running execution above; this is the persistent-row counterpart.
	if prevAssigneeID != "" && s.sessionTerm != nil {
		if err := s.sessionTerm.TerminateOfficeSession(ctx, taskID, prevAssigneeID, sessionTermReasonReassigned); err != nil {
			s.logger.Warn("terminate prev-assignee office session failed",
				zap.String("task_id", taskID),
				zap.String("agent_profile_id", prevAssigneeID),
				zap.Error(err))
		}
	}
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
)

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
	ctx context.Context, comment *models.TaskComment,
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
	}
	if _, err := s.reactivity.ApplyTaskMutation(ctx, comment.TaskID, "", change); err != nil {
		s.logger.Warn("reactivity pipeline failed (comment)",
			zap.String("task_id", comment.TaskID), zap.Error(err))
	}
}

func (s *DashboardService) publishCommentCreated(ctx context.Context, comment *models.TaskComment) {
	if s.eb == nil {
		return
	}
	data := map[string]string{
		"task_id":     comment.TaskID,
		"comment_id":  comment.ID,
		"author_type": comment.AuthorType,
		"author_id":   comment.AuthorID,
	}
	event := bus.NewEvent(events.OfficeCommentCreated, "office-dashboard", data)
	if err := s.eb.Publish(ctx, events.OfficeCommentCreated, event); err != nil {
		s.logger.Error("publish comment created event failed",
			zap.String("task_id", comment.TaskID),
			zap.String("comment_id", comment.ID),
			zap.Error(err))
	}
}
