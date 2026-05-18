package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// Activity action labels for decision lifecycle. Pulled out as constants
// so the values stay consistent across record + supersede + tests.
const (
	activityActionDecisionRecorded   = "task_decision_recorded"
	activityActionDecisionsCleared   = "task_decisions_superseded"
	activityActorTypeAgent           = "agent"
	activityActorTypeUser            = "user"
	activityTaskTargetType           = "task"
	approvalCallerErrEmpty           = "caller type and id are required"
	approvalCommentRequiredOnRequest = "comment is required for request_changes"
	decisionStoreNotWiredErr         = "decision store not wired"
)

// userParticipantSentinel is the participant_id used for decisions
// recorded by the singleton human user. workflow_step_decisions.participant_id
// is NOT NULL with no FK; the office decisions flow recorded by the
// human has no real participant row, so we project this stable sentinel.
const userParticipantSentinel = "user"

// ApprovalsPendingError is returned by UpdateTaskStatus when a task is
// being transitioned to "done" but one or more approvers have no
// current approved decision recorded. The handler maps it to HTTP 409
// and surfaces the redirected status in the response body.
type ApprovalsPendingError struct {
	// Pending is the list of approver agent IDs without a current
	// approved decision.
	Pending []string
}

// Error implements the error interface.
func (e *ApprovalsPendingError) Error() string {
	return fmt.Sprintf(
		"approvals pending from %d approver(s): %s",
		len(e.Pending), strings.Join(e.Pending, ","),
	)
}

// PendingApproverDTO is the per-approver shape rendered in the 409 body
// when UpdateTaskStatus is gated by missing approvals. The frontend
// status-picker reads {agent_profile_id, name} to format the toast text
// "Cannot mark done: awaiting approval from <names>".
type PendingApproverDTO struct {
	AgentProfileID string `json:"agent_profile_id"`
	Name           string `json:"name,omitempty"`
}

// resolvePendingApprovers turns a list of approver agent profile IDs into
// PendingApproverDTOs with display names looked up via the agents reader.
// Lookup failures fall back to the bare ID so the toast still has
// something to render.
func (s *DashboardService) resolvePendingApprovers(
	ctx context.Context, ids []string,
) []PendingApproverDTO {
	out := make([]PendingApproverDTO, len(ids))
	for i, id := range ids {
		out[i] = PendingApproverDTO{AgentProfileID: id, Name: id}
		if s.agents == nil || id == "" {
			continue
		}
		if agent, err := s.agents.GetAgentInstance(ctx, id); err == nil && agent != nil {
			if name := strings.TrimSpace(agent.Name); name != "" {
				out[i].Name = name
			}
		}
	}
	return out
}

// DecisionRecord is the dashboard-tier view of a recorded approval
// decision. ADR 0005 Wave E switched the storage from
// office_task_approval_decisions to workflow_step_decisions; this struct
// preserves the previous office-flavour shape so call sites and the
// HTTP DTO mapper stay stable.
type DecisionRecord struct {
	ID           string
	TaskID       string
	StepID       string
	DeciderType  string
	DeciderID    string
	Role         string
	Decision     string
	Comment      string
	CreatedAt    time.Time
	SupersededAt *time.Time
}

// fromWorkflowDecision projects a workflow_step_decisions row into the
// dashboard's office-flavour view.
func fromWorkflowDecision(d *workflowmodels.WorkflowStepDecision) *DecisionRecord {
	if d == nil {
		return nil
	}
	return &DecisionRecord{
		ID:           d.ID,
		TaskID:       d.TaskID,
		StepID:       d.StepID,
		DeciderType:  d.DeciderType,
		DeciderID:    d.DeciderID,
		Role:         d.Role,
		Decision:     d.Decision,
		Comment:      d.Comment,
		CreatedAt:    d.DecidedAt,
		SupersededAt: d.SupersededAt,
	}
}

// ApproveTask records an "approved" decision by the caller for the
// given task. The caller must appear as a reviewer or approver on the
// task; otherwise shared.ErrForbidden is returned. When the caller
// holds both roles, the decision is recorded under the approver role
// (it's the stricter signal).
//
// Side-effects: emits OfficeTaskDecisionRecorded, logs an activity
// entry, and (when an ApprovalReactivityQueuer is wired) queues a
// task_ready_to_close run for the assignee if all approvers have
// now approved AND the task is currently in_review.
func (s *DashboardService) ApproveTask(
	ctx context.Context, callerType, callerID, taskID, comment string,
) (*DecisionRecord, error) {
	return s.recordTaskDecision(ctx, decisionInput{
		callerType: callerType,
		callerID:   callerID,
		taskID:     taskID,
		comment:    comment,
		decision:   models.DecisionApproved,
	})
}

// RequestTaskChanges records a "changes_requested" decision by the
// caller for the given task. A non-empty comment is required (the
// agent receiving the changes run needs the context). The caller
// must appear in the task's reviewers or approvers list; otherwise
// shared.ErrForbidden is returned.
//
// Side-effects: emits OfficeTaskDecisionRecorded, logs an activity
// entry, and (when wired) queues task_changes_requested for the
// assignee.
func (s *DashboardService) RequestTaskChanges(
	ctx context.Context, callerType, callerID, taskID, comment string,
) (*DecisionRecord, error) {
	if strings.TrimSpace(comment) == "" {
		return nil, fmt.Errorf("%s", approvalCommentRequiredOnRequest)
	}
	return s.recordTaskDecision(ctx, decisionInput{
		callerType: callerType,
		callerID:   callerID,
		taskID:     taskID,
		comment:    comment,
		decision:   models.DecisionChangesRequested,
	})
}

// ListTaskDecisions returns the active (non-superseded) decisions for
// the task in created_at ascending order.
func (s *DashboardService) ListTaskDecisions(
	ctx context.Context, taskID string,
) ([]DecisionRecord, error) {
	if s.decisions == nil {
		return nil, fmt.Errorf("%s", decisionStoreNotWiredErr)
	}
	rows, err := s.decisions.ListActiveTaskDecisions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]DecisionRecord, 0, len(rows))
	for _, r := range rows {
		if rec := fromWorkflowDecision(r); rec != nil {
			out = append(out, *rec)
		}
	}
	return out, nil
}

// decisionInput is the bundle of fields recordTaskDecision needs.
type decisionInput struct {
	callerType string
	callerID   string
	taskID     string
	comment    string
	decision   string
}

// recordTaskDecision performs the shared body of ApproveTask /
// RequestTaskChanges: validates inputs, resolves the caller's role,
// persists the decision, fires events + activity, and queues
// reactivity runs via the configured queuer.
func (s *DashboardService) recordTaskDecision(
	ctx context.Context, in decisionInput,
) (*DecisionRecord, error) {
	if in.callerType == "" || in.callerID == "" {
		return nil, fmt.Errorf("%s", approvalCallerErrEmpty)
	}
	if s.decisions == nil {
		return nil, fmt.Errorf("%s", decisionStoreNotWiredErr)
	}
	role, err := s.resolveDeciderRole(ctx, in.callerType, in.callerID, in.taskID)
	if err != nil {
		return nil, err
	}
	stepID, err := s.repo.GetTaskWorkflowStepID(ctx, in.taskID)
	if err != nil {
		return nil, fmt.Errorf("resolve task workflow_step_id: %w", err)
	}
	if stepID == "" {
		return nil, fmt.Errorf("task %s has no workflow step bound", in.taskID)
	}
	participantID := s.resolveParticipantID(ctx, stepID, in.taskID, role, in.callerType, in.callerID)

	row := &workflowmodels.WorkflowStepDecision{
		ID:            uuid.New().String(),
		TaskID:        in.taskID,
		StepID:        stepID,
		ParticipantID: participantID,
		Decision:      in.decision,
		DecidedAt:     time.Now().UTC(),
		DeciderType:   in.callerType,
		DeciderID:     in.callerID,
		Role:          role,
		Comment:       in.comment,
	}
	if err := s.decisions.RecordStepDecision(ctx, row); err != nil {
		return nil, fmt.Errorf("record decision: %w", err)
	}

	rec := fromWorkflowDecision(row)
	s.publishDecisionRecorded(ctx, rec)
	s.logDecisionActivity(ctx, rec)
	s.runReactivityForDecision(ctx, rec)
	return rec, nil
}

// resolveParticipantID looks up the workflow_step_participants row for
// (step, task, role, agent) and returns its id. Singleton-user callers
// project to a stable sentinel because the user has no participant row.
// A miss for an agent caller falls back to the sentinel as well — the
// office user is treated as the implicit fallback identity per Wave-E
// spec, and RecordStepDecision tolerates a non-empty arbitrary
// participant_id (it has no FK).
func (s *DashboardService) resolveParticipantID(
	ctx context.Context, stepID, taskID, role, callerType, callerID string,
) string {
	if callerType == models.DeciderTypeUser {
		return userParticipantSentinel
	}
	pid, err := s.decisions.FindParticipantID(ctx, stepID, taskID, role, callerID)
	if err != nil || pid == "" {
		// Fall back to a stable sentinel keyed on the caller so the
		// supersede semantics (which key on (decider_id, role)) still hold.
		return callerID
	}
	return pid
}

// resolveDeciderRole returns the role under which the caller's
// decision is recorded. The caller must appear in the task's
// participants for any role; otherwise ErrForbidden. When the caller
// holds both roles, approver wins (it's the stricter signal that
// gates completion).
//
// Note: the human user has no agent_profile_id, so if callerType is
// user the only way they participate is via a sentinel decider_id
// configured upstream. For v1 this is intentionally simple — agents
// are matched by ID; the singleton user is matched by callerType.
func (s *DashboardService) resolveDeciderRole(
	ctx context.Context, callerType, callerID, taskID string,
) (string, error) {
	parts, err := s.repo.ListAllTaskParticipants(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("list participants: %w", err)
	}
	hasReviewer, hasApprover := false, false
	for _, p := range parts {
		if callerType == models.DeciderTypeAgent && p.AgentProfileID == callerID {
			if p.Role == models.ParticipantRoleApprover {
				hasApprover = true
			}
			if p.Role == models.ParticipantRoleReviewer {
				hasReviewer = true
			}
		}
	}
	// User caller — for v1 the singleton user is treated as an
	// implicit approver on every task. This matches the inbox
	// "human user sees every review request" rule from the spec.
	if callerType == models.DeciderTypeUser {
		hasApprover = true
	}
	if !hasReviewer && !hasApprover {
		return "", shared.ErrForbidden
	}
	if hasApprover {
		return models.ParticipantRoleApprover, nil
	}
	return models.ParticipantRoleReviewer, nil
}

// publishDecisionRecorded emits OfficeTaskDecisionRecorded so frontend
// subscribers can refresh the task DTO timeline.
func (s *DashboardService) publishDecisionRecorded(ctx context.Context, d *DecisionRecord) {
	if s.eb == nil || d == nil {
		return
	}
	wsID, _ := s.repo.GetTaskWorkspaceID(ctx, d.TaskID)
	data := map[string]string{
		"task_id":      d.TaskID,
		"workspace_id": wsID,
		"decision_id":  d.ID,
		"role":         d.Role,
		"decider_type": d.DeciderType,
		"decider_id":   d.DeciderID,
		"decision":     d.Decision,
		"created_at":   d.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	event := bus.NewEvent(events.OfficeTaskDecisionRecorded, "office-dashboard", data)
	if err := s.eb.Publish(ctx, events.OfficeTaskDecisionRecorded, event); err != nil {
		s.logger.Error("publish task decision recorded failed",
			zap.String("task_id", d.TaskID), zap.Error(err))
	}
}

// logDecisionActivity records an audit log entry for a decision.
// Best-effort.
func (s *DashboardService) logDecisionActivity(ctx context.Context, d *DecisionRecord) {
	if s.activity == nil || d == nil {
		return
	}
	wsID, _ := s.repo.GetTaskWorkspaceID(ctx, d.TaskID)
	details, _ := json.Marshal(map[string]string{
		"task_id":      d.TaskID,
		"decider_type": d.DeciderType,
		"decider_id":   d.DeciderID,
		"role":         d.Role,
		"decision":     d.Decision,
	})
	actorType := activityActorTypeAgent
	actorID := d.DeciderID
	if d.DeciderType == models.DeciderTypeUser {
		actorType = activityActorTypeUser
		actorID = ""
	}
	s.activity.LogActivity(ctx, wsID, actorType, actorID,
		activityActionDecisionRecorded, activityTaskTargetType, d.TaskID, string(details))
}

// runReactivityForDecision queues the appropriate run after a
// decision lands. For changes_requested, the assignee is woken with
// the comment passed through. For approved, when all approvers now
// have current approved decisions AND the task is in_review, the
// assignee is woken with task_ready_to_close.
//
// Best-effort — failures are logged, never propagated.
func (s *DashboardService) runReactivityForDecision(ctx context.Context, d *DecisionRecord) {
	if s.approvalQueuer == nil || d == nil {
		return
	}
	exec, err := s.repo.GetTaskExecutionFields(ctx, d.TaskID)
	if err != nil || exec == nil {
		return
	}
	if exec.AssigneeAgentProfileID == "" {
		return
	}

	runs := s.buildDecisionRuns(ctx, d, exec)
	if len(runs) == 0 {
		return
	}
	if err := s.approvalQueuer.QueueApprovalRuns(ctx, runs); err != nil {
		s.logger.Warn("queue approval runs failed",
			zap.String("task_id", d.TaskID), zap.Error(err))
	}
}

// buildDecisionRuns constructs the run list for a single
// decision. Returns nil when no run is appropriate.
func (s *DashboardService) buildDecisionRuns(
	ctx context.Context, d *DecisionRecord, exec *sqlite.TaskExecutionFields,
) []ApprovalRun {
	switch d.Decision {
	case models.DecisionChangesRequested:
		return []ApprovalRun{{
			AgentID:         exec.AssigneeAgentProfileID,
			Reason:          runTaskChangesRequested,
			TaskID:          d.TaskID,
			WorkspaceID:     exec.WorkspaceID,
			ActorID:         d.DeciderID,
			ActorType:       d.DeciderType,
			DecisionComment: d.Comment,
		}}
	case models.DecisionApproved:
		if !s.allApproversApproved(ctx, d.TaskID) {
			return nil
		}
		if !isReviewState(exec.State) {
			return nil
		}
		return []ApprovalRun{{
			AgentID:     exec.AssigneeAgentProfileID,
			Reason:      runTaskReadyToClose,
			TaskID:      d.TaskID,
			WorkspaceID: exec.WorkspaceID,
			ActorID:     d.DeciderID,
			ActorType:   d.DeciderType,
		}}
	}
	return nil
}

// isReviewState returns true when a stored task state represents the
// in_review status. Accepts both the canonical DB value "REVIEW" and
// the lowercase form "in_review" (legacy fixtures and some test
// inserts use the latter directly).
func isReviewState(s string) bool {
	switch s {
	case stateInReview, statusInReviewLowercase, "review":
		return true
	}
	return false
}

// allApproversApproved returns true when every approver participant
// for the task has a current (non-superseded) approved decision. An
// empty approver list returns true (no gate to clear).
func (s *DashboardService) allApproversApproved(ctx context.Context, taskID string) bool {
	approvers, err := s.repo.ListTaskParticipants(ctx, taskID, models.ParticipantRoleApprover)
	if err != nil {
		return false
	}
	if len(approvers) == 0 {
		return true
	}
	if s.decisions == nil {
		return false
	}
	decisions, err := s.decisions.ListActiveTaskDecisions(ctx, taskID)
	if err != nil {
		return false
	}
	approvedBy := make(map[string]struct{}, len(decisions))
	for _, dec := range decisions {
		if dec.Decision == models.DecisionApproved {
			approvedBy[dec.DeciderID] = struct{}{}
		}
	}
	for _, ap := range approvers {
		if _, ok := approvedBy[ap.AgentProfileID]; !ok {
			return false
		}
	}
	return true
}

// pendingApprovers returns the agent IDs of approvers without a
// current approved decision. An empty result means the gate is clear.
func (s *DashboardService) pendingApprovers(ctx context.Context, taskID string) ([]string, error) {
	approvers, err := s.repo.ListTaskParticipants(ctx, taskID, models.ParticipantRoleApprover)
	if err != nil {
		return nil, err
	}
	if len(approvers) == 0 {
		return nil, nil
	}
	if s.decisions == nil {
		return nil, fmt.Errorf("%s", decisionStoreNotWiredErr)
	}
	decisions, err := s.decisions.ListActiveTaskDecisions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	approvedBy := make(map[string]struct{}, len(decisions))
	for _, dec := range decisions {
		if dec.Decision == models.DecisionApproved {
			approvedBy[dec.DeciderID] = struct{}{}
		}
	}
	pending := make([]string, 0, len(approvers))
	for _, ap := range approvers {
		if _, ok := approvedBy[ap.AgentProfileID]; !ok {
			pending = append(pending, ap.AgentProfileID)
		}
	}
	return pending, nil
}

// supersedeAndLog clears all active decisions for a task and emits an
// activity entry. Used by the rework / reopen paths in
// UpdateTaskStatus.
func (s *DashboardService) supersedeAndLog(ctx context.Context, taskID string) {
	if s.decisions == nil {
		return
	}
	if err := s.decisions.SupersedeTaskDecisions(ctx, taskID); err != nil {
		s.logger.Warn("supersede decisions failed",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if s.activity == nil {
		return
	}
	wsID, _ := s.repo.GetTaskWorkspaceID(ctx, taskID)
	details, _ := json.Marshal(map[string]string{"task_id": taskID})
	s.activity.LogActivity(ctx, wsID, activityActorTypeUser, "",
		activityActionDecisionsCleared, activityTaskTargetType, taskID, string(details))
}

// Run reason constants that mirror the scheduler package values.
// Duplicated here so the dashboard package doesn't import scheduler;
// the adapter passes these strings straight through to QueueRun.
const (
	runTaskReviewRequested  = "task_review_requested"
	runTaskChangesRequested = "task_changes_requested"
	runTaskReadyToClose     = "task_ready_to_close"
)

// resolveDeciderName returns a friendly display name for a decision's
// decider. For agents it goes through AgentReader; for users it
// returns "User" as a stable label. Empty string when no name can be
// resolved (the DTO renderer falls back to the raw ID).
func (s *DashboardService) resolveDeciderName(
	ctx context.Context, d *DecisionRecord,
) string {
	if d == nil {
		return ""
	}
	if d.DeciderType == models.DeciderTypeUser {
		return "User"
	}
	if s.agents == nil || d.DeciderID == "" {
		return ""
	}
	agent, err := s.agents.GetAgentInstance(ctx, d.DeciderID)
	if err != nil || agent == nil {
		return ""
	}
	return agent.Name
}
