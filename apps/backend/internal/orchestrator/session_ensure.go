package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// EnsureSessionResponse describes the outcome of EnsureSession.
type EnsureSessionResponse struct {
	Success        bool   `json:"success"`
	TaskID         string `json:"task_id"`
	SessionID      string `json:"session_id,omitempty"`
	State          string `json:"state"`
	AgentProfileID string `json:"agent_profile_id,omitempty"`
	Source         string `json:"source"`        // existing_primary | existing_newest | created_prepare | created_start
	NewlyCreated   bool   `json:"newly_created"` // true when a new session was created by this call
}

// ensureLocks serializes EnsureSession calls per task id so concurrent callers
// observe the same session rather than racing to create duplicates. Entries are
// not deleted on release: deletion would race with a concurrent waiter
// (it could acquire the about-to-be-discarded mutex while a new caller LoadOrStores
// a fresh one, putting two goroutines in the critical section for the same task).
// Growth is bounded by the number of distinct task IDs (~160 B per entry).
var ensureLocks sync.Map // map[taskID]*sync.Mutex

func acquireEnsureLock(taskID string) func() {
	v, _ := ensureLocks.LoadOrStore(taskID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// EnsureSession is the server-authoritative idempotent entry point for opening
// a task: it returns the existing primary (or newest) session if any, otherwise
// resolves the agent profile from the task's full context and creates a session
// via prepare (workspace-only) or start (with agent), gated by the task's
// workflow step.
func (s *Service) EnsureSession(ctx context.Context, taskID string) (*EnsureSessionResponse, error) {
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	release := acquireEnsureLock(taskID)
	defer release()

	if existing := s.findExistingSession(ctx, taskID); existing != nil {
		return existing, nil
	}

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}

	agentProfileID, step := s.resolveTaskAgentProfile(ctx, task)
	autoStart := stepAllowsAutoStart(step)

	intent := IntentPrepare
	source := "created_prepare"
	if agentProfileID != "" && autoStart {
		intent = IntentStart
		source = "created_start"
	}

	launchResp, err := s.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:          taskID,
		Intent:          intent,
		AgentProfileID:  agentProfileID,
		WorkflowStepID:  task.WorkflowStepID,
		LaunchWorkspace: true,
		AutoStart:       intent == IntentStart,
	})
	if err != nil {
		return nil, err
	}

	return &EnsureSessionResponse{
		Success:        true,
		TaskID:         taskID,
		SessionID:      launchResp.SessionID,
		State:          launchResp.State,
		AgentProfileID: agentProfileID,
		Source:         source,
		NewlyCreated:   true,
	}, nil
}

// findExistingSession returns the task's existing primary session (or the
// newest if none is marked primary). Returns nil when the task has no sessions.
func (s *Service) findExistingSession(ctx context.Context, taskID string) *EnsureSessionResponse {
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil || len(sessions) == 0 {
		return nil
	}
	for _, sess := range sessions {
		if sess.IsPrimary {
			return existingResponse(taskID, sess, "existing_primary")
		}
	}
	// ListTaskSessions returns rows ordered by started_at DESC.
	return existingResponse(taskID, sessions[0], "existing_newest")
}

func existingResponse(taskID string, sess *models.TaskSession, source string) *EnsureSessionResponse {
	return &EnsureSessionResponse{
		Success:        true,
		TaskID:         taskID,
		SessionID:      sess.ID,
		State:          string(sess.State),
		AgentProfileID: sess.AgentProfileID,
		Source:         source,
		NewlyCreated:   false,
	}
}

// resolveTaskAgentProfile applies the 4-step resolution chain on the backend:
// 1) task.metadata.agent_profile_id, 2) workflow step override,
// 3) workflow default, 4) workspace default. Returns the resolved profile id
// (or "" when none resolve) along with the workflow step it loaded (or nil).
// Returning the step lets callers reuse it (e.g. to gate auto-start) without a
// second DB lookup.
func (s *Service) resolveTaskAgentProfile(ctx context.Context, task *models.Task) (string, *wfmodels.WorkflowStep) {
	step := s.lookupWorkflowStep(ctx, task.WorkflowStepID)
	if v, ok := task.Metadata["agent_profile_id"].(string); ok && v != "" {
		return v, step
	}
	if step != nil {
		if id := s.resolveStepAgentProfile(ctx, step); id != "" {
			return id, step
		}
	}
	ws, err := s.repo.GetWorkspace(ctx, task.WorkspaceID)
	if err == nil && ws != nil && ws.DefaultAgentProfileID != nil && *ws.DefaultAgentProfileID != "" {
		return *ws.DefaultAgentProfileID, step
	}
	return "", step
}

func (s *Service) lookupWorkflowStep(ctx context.Context, stepID string) *wfmodels.WorkflowStep {
	if stepID == "" || s.workflowStepGetter == nil {
		return nil
	}
	step, err := s.workflowStepGetter.GetStep(ctx, stepID)
	if err != nil {
		return nil
	}
	return step
}

// stepAllowsAutoStart reports whether the workflow step (if any) has the
// auto_start_agent on-enter action. Tasks without a workflow step default to
// allowing auto-start (mirrors shouldBlockAutoStart's behavior).
func stepAllowsAutoStart(step *wfmodels.WorkflowStep) bool {
	if step == nil {
		return true
	}
	return step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent)
}
