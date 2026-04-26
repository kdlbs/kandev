package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Execution state status constants.
const (
	execStatusPending  = "pending"
	execStatusApproved = "approved"
	execStatusRejected = "rejected"
)

// participantTypeAgent is the participant type for agent reviewers/approvers.
const participantTypeAgent = "agent"

// ExecutionPolicy defines ordered review/approval stages for a task.
type ExecutionPolicy struct {
	Stages []ExecutionStage `json:"stages"`
}

// ExecutionStage is a single stage of review or approval.
type ExecutionStage struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"` // "review" or "approval"
	Participants    []ExecutionParticipant `json:"participants"`
	ApprovalsNeeded int                    `json:"approvals_needed"`
}

// ExecutionParticipant identifies a reviewer or approver.
type ExecutionParticipant struct {
	Type    string `json:"type"` // "agent" or "user"
	AgentID string `json:"agent_id,omitempty"`
	UserID  string `json:"user_id,omitempty"`
}

// ExecutionState tracks the progress through execution policy stages.
type ExecutionState struct {
	CurrentStageIndex int                       `json:"current_stage_index"`
	Responses         map[string]*StageResponse `json:"responses"`
	Status            string                    `json:"status"` // "pending", "approved", "rejected"
}

// StageResponse records one participant's verdict.
type StageResponse struct {
	ParticipantID string `json:"participant_id"`
	Verdict       string `json:"verdict"` // "approve" or "reject"
	Comments      string `json:"comments"`
	RespondedAt   string `json:"responded_at"`
}

// ParseExecutionPolicy parses an execution policy from a JSON string.
func ParseExecutionPolicy(jsonStr string) (*ExecutionPolicy, error) {
	if jsonStr == "" || jsonStr == "{}" {
		return nil, nil
	}
	var policy ExecutionPolicy
	if err := json.Unmarshal([]byte(jsonStr), &policy); err != nil {
		return nil, fmt.Errorf("parse execution policy: %w", err)
	}
	return &policy, nil
}

// SerializeExecutionState serializes execution state to a JSON string.
func SerializeExecutionState(state *ExecutionState) (string, error) {
	b, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("serialize execution state: %w", err)
	}
	return string(b), nil
}

// parseExecutionState parses an execution state from a JSON string.
func parseExecutionState(jsonStr string) (*ExecutionState, error) {
	if jsonStr == "" || jsonStr == "{}" {
		return nil, nil
	}
	var state ExecutionState
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		return nil, fmt.Errorf("parse execution state: %w", err)
	}
	return &state, nil
}

// EnterReviewStage initialises execution state at the first stage and
// wakes all reviewer participants in parallel.
func (s *Service) EnterReviewStage(ctx context.Context, taskID string, policy ExecutionPolicy) error {
	if len(policy.Stages) == 0 {
		return fmt.Errorf("execution policy has no stages")
	}

	state := &ExecutionState{
		CurrentStageIndex: 0,
		Responses:         make(map[string]*StageResponse),
		Status:            execStatusPending,
	}
	serialized, err := SerializeExecutionState(state)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateTaskExecutionState(ctx, taskID, serialized); err != nil {
		return fmt.Errorf("set initial execution state: %w", err)
	}

	return s.wakeStageParticipants(ctx, taskID, &policy.Stages[0])
}

// wakeStageParticipants queues wakeups for all participants in a stage.
func (s *Service) wakeStageParticipants(ctx context.Context, taskID string, stage *ExecutionStage) error {
	for _, p := range stage.Participants {
		agentID := participantAgentID(&p)
		if agentID == "" {
			continue // user participants use inbox items, not wakeups
		}
		payload := mustJSON(map[string]string{
			"task_id":    taskID,
			"stage_id":   stage.ID,
			"stage_type": stage.Type,
		})
		key := fmt.Sprintf("review_request:%s:%s", taskID, agentID)
		if err := s.QueueWakeup(ctx, agentID, WakeupReasonTaskAssigned, payload, key); err != nil {
			s.logger.Error("failed to wake stage participant",
				zap.String("agent", agentID), zap.Error(err))
		}
	}
	return nil
}

// RecordParticipantResponse records a participant's verdict and, once all
// responses are in, either advances to the next stage or returns the task
// to in_progress with aggregated feedback.
func (s *Service) RecordParticipantResponse(
	ctx context.Context, taskID, participantID, verdict, comments string,
) error {
	fields, err := s.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task fields: %w", err)
	}

	policy, err := ParseExecutionPolicy(fields.ExecutionPolicy)
	if err != nil || policy == nil {
		return fmt.Errorf("task has no execution policy")
	}
	state, err := parseExecutionState(fields.ExecutionState)
	if err != nil || state == nil {
		return fmt.Errorf("task has no execution state")
	}
	if state.CurrentStageIndex >= len(policy.Stages) {
		return fmt.Errorf("no active stage")
	}

	state.Responses[participantID] = &StageResponse{
		ParticipantID: participantID,
		Verdict:       verdict,
		Comments:      comments,
		RespondedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	stage := &policy.Stages[state.CurrentStageIndex]
	if err := s.persistState(ctx, taskID, state); err != nil {
		return err
	}

	responseCount := countStageResponses(state, stage)
	if responseCount < len(stage.Participants) {
		return nil // still waiting for more responses
	}

	return s.evaluateStageCompletion(ctx, taskID, policy, state, stage)
}

// evaluateStageCompletion processes a completed stage -- either advancing
// or returning the task to in_progress with feedback.
func (s *Service) evaluateStageCompletion(
	ctx context.Context, taskID string,
	policy *ExecutionPolicy, state *ExecutionState, stage *ExecutionStage,
) error {
	approvals := countApprovals(state, stage)
	if approvals >= stage.ApprovalsNeeded {
		state.Status = execStatusApproved
		return s.AdvanceStage(ctx, taskID)
	}

	// Rejection path: aggregate all feedback, return task to in_progress.
	state.Status = execStatusRejected
	if err := s.persistState(ctx, taskID, state); err != nil {
		return err
	}

	return s.returnTaskWithFeedback(ctx, taskID, state, stage)
}

// returnTaskWithFeedback moves the task back to in_progress and wakes
// the assignee with aggregated reviewer feedback.
func (s *Service) returnTaskWithFeedback(
	ctx context.Context, taskID string,
	state *ExecutionState, stage *ExecutionStage,
) error {
	if err := s.repo.UpdateTaskState(ctx, taskID, "IN_PROGRESS"); err != nil {
		return fmt.Errorf("return task to in_progress: %w", err)
	}

	feedback := aggregateFeedback(state, stage)
	fields, err := s.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task assignee: %w", err)
	}
	if fields.AssigneeAgentInstanceID == "" {
		return nil
	}

	payload := mustJSON(map[string]string{
		"task_id":  taskID,
		"feedback": feedback,
		"reason":   "review_rejected",
	})
	key := fmt.Sprintf("review_feedback:%s:%d", taskID, state.CurrentStageIndex)
	return s.QueueWakeup(ctx, fields.AssigneeAgentInstanceID,
		WakeupReasonTaskAssigned, payload, key)
}

// AdvanceStage moves to the next execution policy stage, or completes the task.
func (s *Service) AdvanceStage(ctx context.Context, taskID string) error {
	fields, err := s.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task fields: %w", err)
	}

	policy, err := ParseExecutionPolicy(fields.ExecutionPolicy)
	if err != nil || policy == nil {
		return fmt.Errorf("task has no execution policy")
	}
	state, err := parseExecutionState(fields.ExecutionState)
	if err != nil || state == nil {
		return fmt.Errorf("task has no execution state")
	}

	nextIndex := state.CurrentStageIndex + 1
	if nextIndex >= len(policy.Stages) {
		return s.completeTask(ctx, taskID, fields.WorkspaceID)
	}

	state.CurrentStageIndex = nextIndex
	state.Responses = make(map[string]*StageResponse)
	state.Status = execStatusPending
	if err := s.persistState(ctx, taskID, state); err != nil {
		return err
	}

	nextStage := &policy.Stages[nextIndex]
	if nextStage.Type == "approval" {
		return s.createApprovalInboxItems(ctx, taskID, fields.WorkspaceID, nextStage)
	}
	return s.wakeStageParticipants(ctx, taskID, nextStage)
}

// completeTask moves a task to done and resolves blockers.
func (s *Service) completeTask(ctx context.Context, taskID, workspaceID string) error {
	if err := s.repo.UpdateTaskState(ctx, taskID, "COMPLETED"); err != nil {
		return fmt.Errorf("complete task: %w", err)
	}
	s.LogActivity(ctx, workspaceID, "system", "", "task.status_changed", "task", taskID,
		`{"new_status":"done","via":"execution_policy"}`)

	if err := s.queueBlockersResolvedWakeups(ctx, taskID); err != nil {
		s.logger.Error("blocker resolution after policy completion failed", zap.Error(err))
	}
	return nil
}

// createApprovalInboxItems creates inbox-style approval wakeups for approval stage participants.
func (s *Service) createApprovalInboxItems(
	ctx context.Context, taskID, workspaceID string, stage *ExecutionStage,
) error {
	for _, p := range stage.Participants {
		if p.Type == "user" {
			s.LogActivity(ctx, workspaceID, "system", "", "approval.created", "task", taskID,
				fmt.Sprintf(`{"user_id":%q,"stage_id":%q}`, p.UserID, stage.ID))
			continue
		}
		agentID := participantAgentID(&p)
		if agentID == "" {
			continue
		}
		payload := mustJSON(map[string]string{
			"task_id":    taskID,
			"stage_id":   stage.ID,
			"stage_type": "approval",
		})
		key := fmt.Sprintf("approval_request:%s:%s", taskID, agentID)
		if err := s.QueueWakeup(ctx, agentID, WakeupReasonTaskAssigned, payload, key); err != nil {
			s.logger.Error("failed to wake approver",
				zap.String("agent", agentID), zap.Error(err))
		}
	}
	return nil
}

// persistState serialises and writes execution state to the task.
func (s *Service) persistState(ctx context.Context, taskID string, state *ExecutionState) error {
	serialized, err := SerializeExecutionState(state)
	if err != nil {
		return err
	}
	return s.repo.UpdateTaskExecutionState(ctx, taskID, serialized)
}

// participantAgentID returns the agent ID for a participant, or "" for users.
func participantAgentID(p *ExecutionParticipant) string {
	if p.Type == participantTypeAgent {
		return p.AgentID
	}
	return ""
}

// countStageResponses counts how many of the stage's participants have responded.
func countStageResponses(state *ExecutionState, stage *ExecutionStage) int {
	count := 0
	for _, p := range stage.Participants {
		id := participantKey(&p)
		if _, ok := state.Responses[id]; ok {
			count++
		}
	}
	return count
}

// countApprovals counts how many participants approved in the current stage.
func countApprovals(state *ExecutionState, stage *ExecutionStage) int {
	count := 0
	for _, p := range stage.Participants {
		id := participantKey(&p)
		if resp, ok := state.Responses[id]; ok && resp.Verdict == "approve" {
			count++
		}
	}
	return count
}

// participantKey returns a unique key for a participant.
func participantKey(p *ExecutionParticipant) string {
	if p.Type == participantTypeAgent {
		return p.AgentID
	}
	return p.UserID
}

// SetTaskExecutionPolicy stores an execution policy JSON on a task.
func (s *Service) SetTaskExecutionPolicy(ctx context.Context, taskID, policyJSON string) error {
	return s.repo.UpdateTaskExecutionPolicy(ctx, taskID, policyJSON)
}

// SetTaskAssignee updates the assignee agent instance on a task.
func (s *Service) SetTaskAssignee(ctx context.Context, taskID, assigneeID string) error {
	return s.repo.UpdateTaskAssignee(ctx, taskID, assigneeID)
}

// aggregateFeedback collects all review comments into a single string.
func aggregateFeedback(state *ExecutionState, stage *ExecutionStage) string {
	var parts []string
	for _, p := range stage.Participants {
		id := participantKey(&p)
		resp, ok := state.Responses[id]
		if !ok || resp.Comments == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("[%s] %s: %s", resp.Verdict, id, resp.Comments))
	}
	return strings.Join(parts, "\n")
}
