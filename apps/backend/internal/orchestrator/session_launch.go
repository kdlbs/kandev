package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// SessionIntent represents the type of session operation requested.
type SessionIntent string

const (
	IntentPrepare          SessionIntent = "prepare"           // Create session, optionally launch workspace, NO agent
	IntentStart            SessionIntent = "start"             // Create session + launch agent (new session)
	IntentStartCreated     SessionIntent = "start_created"     // Start agent on existing CREATED session
	IntentResume           SessionIntent = "resume"            // Restart stopped session with resume token
	IntentWorkflowStep     SessionIntent = "workflow_step"     // Start session with workflow step prompt config
	IntentRestoreWorkspace SessionIntent = "restore_workspace" // Restore workspace access for terminal-state session
)

// LaunchSessionRequest is the unified request for session.launch.
type LaunchSessionRequest struct {
	TaskID            string                 `json:"task_id"`
	Intent            SessionIntent          `json:"intent,omitempty"`
	SessionID         string                 `json:"session_id,omitempty"`
	AgentProfileID    string                 `json:"agent_profile_id,omitempty"`
	ExecutorID        string                 `json:"executor_id,omitempty"`
	ExecutorProfileID string                 `json:"executor_profile_id,omitempty"`
	Prompt            string                 `json:"prompt,omitempty"`
	PlanMode          bool                   `json:"plan_mode,omitempty"`
	WorkflowStepID    string                 `json:"workflow_step_id,omitempty"`
	Priority          int                    `json:"priority,omitempty"`
	LaunchWorkspace   bool                   `json:"launch_workspace,omitempty"`
	SkipMessageRecord bool                   `json:"skip_message_record,omitempty"`
	AutoStart         bool                   `json:"auto_start,omitempty"`
	Attachments       []v1.MessageAttachment `json:"attachments,omitempty"`
}

// LaunchSessionResponse is the unified response for session.launch.
type LaunchSessionResponse struct {
	Success          bool    `json:"success"`
	TaskID           string  `json:"task_id"`
	SessionID        string  `json:"session_id,omitempty"`
	AgentExecutionID string  `json:"agent_execution_id,omitempty"`
	State            string  `json:"state"`
	WorktreePath     *string `json:"worktree_path,omitempty"`
	WorktreeBranch   *string `json:"worktree_branch,omitempty"`
}

// ResolveIntent infers the session intent from request fields when Intent is empty.
func ResolveIntent(req *LaunchSessionRequest) SessionIntent {
	if req.Intent != "" {
		return req.Intent
	}
	if req.SessionID != "" && req.WorkflowStepID != "" {
		return IntentWorkflowStep
	}
	if req.SessionID != "" && req.Prompt == "" && req.AgentProfileID == "" {
		return IntentResume
	}
	if req.SessionID != "" {
		return IntentStartCreated
	}
	if req.LaunchWorkspace && req.Prompt == "" {
		return IntentPrepare
	}
	return IntentStart
}

// LaunchSession is the unified entry point for all session operations.
func (s *Service) LaunchSession(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	intent := ResolveIntent(req)
	req.Prompt = strings.TrimSpace(req.Prompt)

	s.logger.Debug("LaunchSession",
		zap.String("task_id", req.TaskID),
		zap.String("intent", string(intent)),
		zap.String("session_id", req.SessionID))

	switch intent {
	case IntentPrepare:
		return s.launchPrepare(ctx, req)
	case IntentStart:
		return s.launchStart(ctx, req)
	case IntentStartCreated:
		return s.launchStartCreated(ctx, req)
	case IntentResume:
		return s.launchResume(ctx, req)
	case IntentWorkflowStep:
		return s.launchWorkflowStep(ctx, req)
	case IntentRestoreWorkspace:
		return s.launchRestoreWorkspace(ctx, req)
	default:
		return nil, fmt.Errorf("unknown intent: %s", intent)
	}
}

// launchPrepare creates a session entry without launching the agent.
func (s *Service) launchPrepare(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	sessionID, err := s.PrepareTaskSession(
		ctx, req.TaskID, req.AgentProfileID, req.ExecutorID,
		req.ExecutorProfileID, req.WorkflowStepID, req.LaunchWorkspace,
	)
	if err != nil {
		return nil, err
	}
	return &LaunchSessionResponse{
		Success:   true,
		TaskID:    req.TaskID,
		SessionID: sessionID,
		State:     string(models.TaskSessionStateCreated),
	}, nil
}

// launchStart creates a new session and launches the agent.
// If the request is an auto-start and the task's current workflow step does not
// have auto_start_agent, the request is downgraded to a prepare (workspace-only,
// no agent) to prevent unwanted auto-starts from the frontend's useAutoStartSession hook.
func (s *Service) launchStart(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	if req.AutoStart && s.shouldBlockAutoStart(ctx, req) {
		req.LaunchWorkspace = true
		return s.launchPrepare(ctx, req)
	}

	execution, err := s.StartTask(
		ctx, req.TaskID, req.AgentProfileID, req.ExecutorID,
		req.ExecutorProfileID, req.Priority, req.Prompt,
		req.WorkflowStepID, req.PlanMode, req.Attachments,
	)
	if err != nil {
		return nil, err
	}
	return executionToLaunchResponse(req.TaskID, execution), nil
}

// shouldBlockAutoStart checks whether the task's workflow step allows auto-starting
// the agent. Returns true when the step exists but does not have auto_start_agent
// in its on_enter events. Tasks without a workflow step are never blocked.
func (s *Service) shouldBlockAutoStart(ctx context.Context, req *LaunchSessionRequest) bool {
	if s.workflowStepGetter == nil {
		return false
	}

	task, err := s.repo.GetTask(ctx, req.TaskID)
	if err != nil || task.WorkflowStepID == "" {
		return false
	}

	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || step == nil {
		return false
	}

	if step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent) {
		return false
	}

	s.logger.Info("auto-start downgraded to prepare: step lacks auto_start_agent",
		zap.String("task_id", req.TaskID),
		zap.String("workflow_step_id", task.WorkflowStepID),
		zap.String("step_name", step.Name))

	return true
}

// launchStartCreated starts agent execution on an existing CREATED session.
func (s *Service) launchStartCreated(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	execution, err := s.StartCreatedSession(
		ctx, req.TaskID, req.SessionID, req.AgentProfileID,
		req.Prompt, req.SkipMessageRecord, req.PlanMode, req.Attachments,
	)
	if err != nil {
		return nil, err
	}
	return executionToLaunchResponse(req.TaskID, execution), nil
}

// launchResume resumes a stopped session.
func (s *Service) launchResume(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	execution, err := s.ResumeTaskSession(ctx, req.TaskID, req.SessionID)
	if err != nil {
		return nil, err
	}
	return executionToLaunchResponse(req.TaskID, execution), nil
}

// launchWorkflowStep starts a session with workflow step prompt configuration.
func (s *Service) launchWorkflowStep(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	err := s.StartSessionForWorkflowStep(ctx, req.TaskID, req.SessionID, req.WorkflowStepID)
	if err != nil {
		return nil, err
	}
	return &LaunchSessionResponse{
		Success:   true,
		TaskID:    req.TaskID,
		SessionID: req.SessionID,
		State:     string(v1.TaskSessionStateRunning),
	}, nil
}

// launchRestoreWorkspace restores workspace access for a terminal-state session (COMPLETED, FAILED, CANCELLED).
// It creates a lightweight agentctl execution so the frontend can browse files, open terminals, and view git status.
func (s *Service) launchRestoreWorkspace(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required for workspace restore")
	}

	session, err := s.repo.GetTaskSession(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	if session.TaskID != req.TaskID {
		return nil, fmt.Errorf("session does not belong to task")
	}

	if err := s.agentManager.EnsureWorkspaceExecutionForSession(ctx, req.TaskID, req.SessionID); err != nil {
		return nil, fmt.Errorf("failed to restore workspace: %w", err)
	}

	resp := &LaunchSessionResponse{
		Success:   true,
		TaskID:    req.TaskID,
		SessionID: req.SessionID,
		State:     string(session.State),
	}
	if len(session.Worktrees) > 0 {
		wt := session.Worktrees[0]
		if wt.WorktreePath != "" {
			resp.WorktreePath = &wt.WorktreePath
		}
		if wt.WorktreeBranch != "" {
			resp.WorktreeBranch = &wt.WorktreeBranch
		}
	}
	return resp, nil
}

// RecoverSession handles user-initiated recovery after an agent CLI failure.
// action is "resume" (retry with existing ACP session) or "fresh_start" (clear token, start fresh).
func (s *Service) RecoverSession(ctx context.Context, taskID, sessionID, action string) (*LaunchSessionResponse, error) {
	switch action {
	case "fresh_start":
		s.clearResumeToken(ctx, sessionID)
	case "resume":
		// no-op — relaunch with existing resume token
	default:
		return nil, fmt.Errorf("invalid recovery action: %s", action)
	}

	return s.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:    taskID,
		SessionID: sessionID,
		Intent:    IntentResume,
	})
}

// executionToLaunchResponse converts a TaskExecution to a LaunchSessionResponse.
func executionToLaunchResponse(taskID string, exec *executor.TaskExecution) *LaunchSessionResponse {
	resp := &LaunchSessionResponse{
		Success:          true,
		TaskID:           taskID,
		SessionID:        exec.SessionID,
		AgentExecutionID: exec.AgentExecutionID,
		State:            string(exec.SessionState),
	}
	if exec.WorktreePath != "" {
		resp.WorktreePath = &exec.WorktreePath
	}
	if exec.WorktreeBranch != "" {
		resp.WorktreeBranch = &exec.WorktreeBranch
	}
	return resp
}
