package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"go.uber.org/zap"
)

// EnsureSessionResponse describes the outcome of EnsureSession.
type EnsureSessionResponse struct {
	Success        bool   `json:"success"`
	TaskID         string `json:"task_id"`
	SessionID      string `json:"session_id,omitempty"`
	State          string `json:"state"`
	AgentProfileID string `json:"agent_profile_id,omitempty"`
	Source         string `json:"source"`                   // existing_primary | existing_newest | created_prepare | created_start
	NewlyCreated   bool   `json:"newly_created"`            // true when a new session was created by this call
	WorkspacePath  string `json:"workspace_path,omitempty"` // effective workspace path (for quick-chat sessions without worktrees)
}

// EnsureSessionOptions holds optional parameters for EnsureSession.
type EnsureSessionOptions struct {
	// EnsureExecution triggers an execution resume when the session exists
	// but the agent process (agentctl) is not running. Used by office advanced
	// mode to bring up file/terminal/changes panels.
	EnsureExecution bool
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
//
// When opts.EnsureExecution is true and the session already exists, the method
// also verifies that the agent process (agentctl) is running and resumes it if
// not. This is used by the office advanced mode where one-off tasks may have
// their execution torn down after completion.
func (s *Service) EnsureSession(ctx context.Context, taskID string, opts ...EnsureSessionOptions) (*EnsureSessionResponse, error) {
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	release := acquireEnsureLock(taskID)
	defer release()

	var o EnsureSessionOptions
	if len(opts) > 0 {
		o = opts[0]
	}

	if existing := s.findExistingSession(ctx, taskID); existing != nil {
		if o.EnsureExecution {
			s.tryEnsureExecution(ctx, existing.SessionID)
		}
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

// findExistingSession returns the task's existing session for advanced-mode
// resume. For office tasks, it picks the (task, agent) row matching the
// authenticated agent context (or the task's assignee when the viewer is the
// singleton human user). For kanban tasks, it falls back to the existing
// is_primary-first lookup. Returns nil when no matching session exists.
func (s *Service) findExistingSession(ctx context.Context, taskID string) *EnsureSessionResponse {
	if office := s.findOfficeSessionForResume(ctx, taskID); office != nil {
		return office
	}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil || len(sessions) == 0 {
		return nil
	}
	for _, sess := range sessions {
		if sess.IsPrimary {
			return s.existingResponse(ctx, taskID, sess, "existing_primary")
		}
	}
	// ListTaskSessions returns rows ordered by started_at DESC.
	return s.existingResponse(ctx, taskID, sessions[0], "existing_newest")
}

// findOfficeSessionForResume implements the office-only branch of advanced-mode
// resume. Returns nil when the task isn't office, when no per-agent session
// has been created yet, or when no relevant agent identity is available.
// (The "create on demand" branch is handled by EnsureSession's fallthrough,
// not here — keeping findExistingSession a pure lookup.)
func (s *Service) findOfficeSessionForResume(ctx context.Context, taskID string) *EnsureSessionResponse {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.AssigneeAgentProfileID == "" {
		return nil
	}
	agentID := s.agentForViewer(ctx, task)
	if agentID == "" {
		return nil
	}
	sess, err := s.repo.GetTaskSessionByTaskAndAgent(ctx, taskID, agentID)
	if err != nil || sess == nil {
		return nil
	}
	return s.existingResponse(ctx, taskID, sess, "existing_office_agent")
}

// agentForViewer resolves the agent_profile_id whose session should be
// surfaced in advanced mode. Order:
//  1. Authenticated agent context (when an agent opens advanced mode itself).
//  2. Task assignee (the singleton-human-user case — singleton users see the
//     assignee's session by default).
func (s *Service) agentForViewer(ctx context.Context, task *models.Task) string {
	if v, ok := ctx.Value(viewerAgentContextKey).(string); ok && v != "" {
		return v
	}
	return task.AssigneeAgentProfileID
}

// viewerAgentContextKey is the context key used to thread an authenticated
// agent identity through advanced-mode resume. The HTTP layer sets it when
// an agent opens advanced mode under their own credentials. The singleton
// human user leaves it unset; agentForViewer then falls back to the task
// assignee.
type viewerAgentCtxKey struct{}

var viewerAgentContextKey = viewerAgentCtxKey{}

// WithViewerAgent returns a context that surfaces agentInstanceID to
// findOfficeSessionForResume's lookup. Exported so HTTP handlers (and tests)
// can attach the viewer's identity without reaching into the orchestrator's
// internals.
func WithViewerAgent(ctx context.Context, agentInstanceID string) context.Context {
	if agentInstanceID == "" {
		return ctx
	}
	return context.WithValue(ctx, viewerAgentContextKey, agentInstanceID)
}

func (s *Service) existingResponse(ctx context.Context, taskID string, sess *models.TaskSession, source string) *EnsureSessionResponse {
	resp := &EnsureSessionResponse{
		Success:        true,
		TaskID:         taskID,
		SessionID:      sess.ID,
		State:          string(sess.State),
		AgentProfileID: sess.AgentProfileID,
		Source:         source,
		NewlyCreated:   false,
	}
	// Include workspace path from the task environment (needed by quick-chat
	// sessions that have no worktree_path on the session record).
	if env, err := s.repo.GetTaskEnvironmentByTaskID(ctx, taskID); err == nil && env != nil {
		resp.WorkspacePath = env.WorkspacePath
	}
	return resp
}

// tryEnsureExecution attempts to resume the execution for an existing session.
// Failures are logged but not propagated — the session is still usable for chat
// even if the execution can't be resumed (file/terminal panels will show
// appropriate "not available" states).
func (s *Service) tryEnsureExecution(ctx context.Context, sessionID string) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return
	}
	if err := s.ensureSessionRunning(ctx, sessionID, session); err != nil {
		s.logger.Debug("ensure execution for existing session (non-fatal)",
			zap.String("session_id", sessionID),
			zap.Error(err))
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
