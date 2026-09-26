package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"go.uber.org/zap"
)

const activationReasonSessionCapacity = "session_capacity"

// EnsureSessionResponse describes the outcome of EnsureSession.
type EnsureSessionResponse struct {
	Success               bool   `json:"success"`
	TaskID                string `json:"task_id"`
	SessionID             string `json:"session_id,omitempty"`
	State                 string `json:"state"`
	AgentProfileID        string `json:"agent_profile_id,omitempty"`
	Source                string `json:"source"`                   // existing_primary | existing_newest | created_prepare | created_start | skipped_terminal_pr
	NewlyCreated          bool   `json:"newly_created"`            // true when a new session was created by this call
	WorkspacePath         string `json:"workspace_path,omitempty"` // effective workspace path (for quick-chat sessions without worktrees)
	ActivationDisposition string `json:"activation_disposition,omitempty"`
	ActivationReason      string `json:"activation_reason,omitempty"`
}

// EnsureSessionOptions holds optional parameters for EnsureSession.
type EnsureSessionOptions struct {
	// EnsureExecution triggers an execution resume when the session exists
	// but the agent process (agentctl) is not running. Used by office advanced
	// mode to bring up file/terminal/changes panels.
	EnsureExecution bool
	// AutoStart overrides the workflow-step auto-start decision. When
	// explicitly false, the session is created workspace-only (prepare,
	// CREATED) even when the step's on-enter has auto_start_agent, and the
	// launch is marked NoAgentLaunch so passthrough profiles are never
	// upgraded into an agent start. Absent (nil) keeps the step-derived
	// decision.
	AutoStart *bool
	// ActivationSource distinguishes passive task opening from an explicit
	// launch action. A passive open never resumes an existing session.
	ActivationSource LaunchActivationSource
}

// ensureLocks serializes EnsureSession calls per task id so concurrent callers
// observe the same session rather than racing to create duplicates. Entries are
// not deleted on release: deletion would race with a concurrent waiter
// (it could acquire the about-to-be-discarded mutex while a new caller LoadOrStores
// a fresh one, putting two goroutines in the critical section for the same task).
// Growth is bounded by the number of distinct task IDs (~160 B per entry).
var ensureLocks sync.Map // map[taskID]*sync.Mutex

// acquireEnsureLock serializes concurrent EnsureSession calls per task,
// returning an unlock function.
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
	if err := s.authorizeTask(ctx, taskID); err != nil {
		return nil, err
	}

	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	release := acquireEnsureLock(taskID)
	defer release()

	var o EnsureSessionOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if err := validateLaunchActivationSource(o.ActivationSource); err != nil {
		return nil, err
	}

	// A passive task open must follow the session already selected by a
	// deferred launch. Returning the ordinary primary session here would make
	// the browser inspect the parked predecessor while the queued destination
	// remained hidden, and a later open could attempt a second launch.
	if o.ActivationSource == LaunchActivationSourceSessionOpen {
		if queued, handled := s.queuedEnsureResponse(ctx, taskID); handled {
			return queued, nil
		}
	}

	if existing := s.findExistingSession(ctx, taskID); existing != nil {
		if o.EnsureExecution && o.ActivationSource != LaunchActivationSourceSessionOpen {
			s.tryEnsureExecution(ctx, existing.SessionID, seam3CallShapeViewing, launchOriginManual, "")
		}
		return existing, nil
	}

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}

	agentProfileID, step := s.resolveTaskAgentProfile(ctx, task)
	autoStart := stepAllowsAutoStart(step)
	if o.AutoStart != nil {
		autoStart = *o.AutoStart
	}

	intent := IntentPrepare
	source := "created_prepare"
	if agentProfileID != "" && autoStart {
		intent = IntentStart
		source = "created_start"
	}

	launchResp, err := s.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:           taskID,
		Intent:           intent,
		AgentProfileID:   agentProfileID,
		WorkflowStepID:   task.WorkflowStepID,
		LaunchWorkspace:  true,
		AutoStart:        intent == IntentStart,
		NoAgentLaunch:    o.AutoStart != nil && !*o.AutoStart,
		ActivationSource: o.ActivationSource,
	})
	if err != nil {
		return nil, err
	}
	if launchResp == nil {
		return nil, fmt.Errorf("session launch returned no response")
	}
	if launchResp.SessionID == "" {
		// An automatic launch can be intentionally skipped after the task is
		// found to have a terminal pull request. Keep ensure idempotent and let
		// the task-owned launch error card render without creating a session.
		return &EnsureSessionResponse{
			Success:               true,
			TaskID:                taskID,
			State:                 launchResp.State,
			AgentProfileID:        agentProfileID,
			Source:                "skipped_terminal_pr",
			NewlyCreated:          false,
			ActivationDisposition: launchResp.ActivationDisposition,
			ActivationReason:      launchResp.ActivationReason,
		}, nil
	}

	return &EnsureSessionResponse{
		Success:               true,
		TaskID:                taskID,
		SessionID:             launchResp.SessionID,
		State:                 launchResp.State,
		AgentProfileID:        agentProfileID,
		Source:                source,
		NewlyCreated:          true,
		ActivationDisposition: launchResp.ActivationDisposition,
		ActivationReason:      launchResp.ActivationReason,
	}, nil
}

// queuedEnsureResponse returns the exact destination owned by a durable
// automatic launch deferral. handled is true whenever the task has a ceiling
// record, including malformed or orphaned records: passive inspection must
// remain read-only and must not fall back to creating or resuming another
// session when ownership cannot be established.
func (s *Service) queuedEnsureResponse(ctx context.Context, taskID string) (*EnsureSessionResponse, bool) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return queuedEnsureOwnershipUnavailableResponse(taskID, "", ""), true
	}
	if !models.HasCeilingDeferredIntent(task) {
		return nil, false
	}
	record, _ := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	deferral, err := models.ReadCeilingDeferral(record)
	if err != nil {
		return queuedEnsureOwnershipUnavailableResponse(taskID, "", ""), true
	}
	return s.queuedEnsureResponseForDeferral(ctx, taskID, task, deferral), true
}

func (s *Service) queuedEnsureResponseForDeferral(
	ctx context.Context,
	taskID string,
	task *models.Task,
	deferral models.CeilingDeferral,
) *EnsureSessionResponse {
	sessionID := models.CeilingDeferralSessionID(task, deferral)
	agentProfileID := stringField(deferral.Payload, metaKeyAgentProfileID)
	if queuedEnsureOwnershipUnavailable(task, deferral, sessionID) {
		return queuedEnsureOwnershipUnavailableResponse(taskID, sessionID, agentProfileID)
	}
	if sessionID == "" {
		return queuedEnsureCapacityResponse(taskID, agentProfileID)
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return queuedEnsureOwnershipUnavailableResponse(taskID, sessionID, agentProfileID)
	}

	response := s.existingResponse(ctx, taskID, session, "existing_queued")
	response.ActivationDisposition = activationDispositionQueued
	response.ActivationReason = activationReasonSessionCapacity
	return response
}

func queuedEnsureOwnershipUnavailable(
	task *models.Task,
	deferral models.CeilingDeferral,
	sessionID string,
) bool {
	_, bindingPresent, bindingErr := models.ReadCeilingWorkflowEntryBinding(deferral.Payload)
	workflowOrigin := bindingPresent || stringField(deferral.Payload, metaKeyWorkflowStepID) != "" ||
		int64Field(deferral.Payload, "workflow_entry_id") > 0
	if deferral.Kind == models.CeilingLaunchStart {
		if _, routePresent := models.LoadWorkflowSessionRoute(task.Metadata); routePresent {
			workflowOrigin = true
		}
	}
	return bindingErr != nil || (workflowOrigin && (sessionID == "" ||
		(bindingPresent && !models.CeilingDeferralTargetsSession(task, deferral, sessionID))))
}

func queuedEnsureOwnershipUnavailableResponse(
	taskID, sessionID, agentProfileID string,
) *EnsureSessionResponse {
	return &EnsureSessionResponse{
		Success:               true,
		TaskID:                taskID,
		SessionID:             sessionID,
		AgentProfileID:        agentProfileID,
		State:                 string(models.TaskSessionStateCreated),
		Source:                "queued",
		NewlyCreated:          false,
		ActivationDisposition: activationDispositionSuppressed,
		ActivationReason:      autoResumeBlockedOwnershipUnavailable,
	}
}

func queuedEnsureCapacityResponse(taskID, agentProfileID string) *EnsureSessionResponse {
	return &EnsureSessionResponse{
		Success:               true,
		TaskID:                taskID,
		State:                 string(models.TaskSessionStateCreated),
		AgentProfileID:        agentProfileID,
		Source:                "queued",
		NewlyCreated:          false,
		ActivationDisposition: activationDispositionQueued,
		ActivationReason:      activationReasonSessionCapacity,
	}
}

// findExistingSession returns the task's existing session for advanced-mode
// resume. For office tasks, it picks the (task, agent) row matching the
// authenticated agent context (or the task's assignee when the viewer is the
// singleton human user). For kanban tasks, it falls back to the existing
// is_primary-first lookup. Returns nil when no matching session exists.
func (s *Service) findExistingSession(ctx context.Context, taskID string) *EnsureSessionResponse {
	if office, isOffice := s.findOfficeSessionForResume(ctx, taskID); isOffice {
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
// resume. The second return value reports whether the task is Office-owned.
// Returns nil when no per-agent session has been created yet or when no
// relevant agent identity is available. An Office task must not fall through to
// the generic primary/newest lookup, because that lookup can return another
// participant's session. When the task has no projected runner (for example,
// a task created with only metadata.agent_profile_id), resolve the same profile
// EnsureSession would use for creation and look up that exact session. The
// "create on demand" branch is handled by EnsureSession after this pure lookup.
func (s *Service) findOfficeSessionForResume(ctx context.Context, taskID string) (*EnsureSessionResponse, bool) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || !task.IsFromOffice {
		return nil, false
	}
	agentID := s.agentForViewer(ctx, task)
	if agentID == "" {
		// A task can be Office-owned without a projected runner. This is
		// common for API-created review tasks whose concrete profile lives in
		// metadata. Match the profile resolution used by EnsureSession so an
		// already-created run session is reused while still avoiding a
		// participant-agnostic newest-session fallback.
		agentID, _ = s.resolveTaskAgentProfile(ctx, task)
	}
	if agentID == "" {
		return nil, true
	}
	sess, err := s.repo.GetTaskSessionByTaskAndAgent(ctx, taskID, agentID)
	if err != nil || sess == nil {
		return nil, true
	}
	return s.existingResponse(ctx, taskID, sess, "existing_office_agent"), true
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

// existingResponse builds an ensure response for a pre-existing session,
// attaching the task's workspace path when the session row lacks one.
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
//
// callShape distinguishes this call's two production shapes (AC-47e): a seam-3
// refusal is swallowed for the viewing shape (nothing to replay, nobody
// waiting) but deferred for the queue-drain shape, which supplies
// queuedMessageID so the deferred record can be matched back to its queued
// message at retry time. An unrecognized shape defaults to deferring
// (AC-47e1) — the unsafe direction here is silently dropping a launch, not
// refusing one.
func (s *Service) tryEnsureExecution(
	ctx context.Context, sessionID string, callShape seam3CallShape, origin launchOrigin, queuedMessageID string,
) {
	_ = s.tryEnsureExecutionWithBinding(
		ctx, sessionID, callShape, origin, queuedMessageID, ceilingEntryBindingFromContext(ctx),
	)
}

func (s *Service) tryEnsureExecutionWithBinding(
	ctx context.Context, sessionID string, callShape seam3CallShape, origin launchOrigin, queuedMessageID string,
	binding *models.CeilingWorkflowEntryBinding,
) error {
	if binding != nil {
		ctx = withCeilingEntryBinding(ctx, binding)
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return err
	}
	err = s.ensureSessionRunningWithBinding(ctx, sessionID, session, origin, binding)
	if err == nil {
		return nil
	}
	if refusal, ok := isSeam3Refusal(err); ok {
		switch callShape {
		case seam3CallShapeViewing:
			return nil
		case seam3CallShapeQueueDrain:
			s.deferSeam3QueueDrainRefusal(ctx, session.TaskID, sessionID, queuedMessageID, refusal, binding)
			return err
		default:
			s.logger.Zap().Warn("tryEnsureExecution saw an unrecognized seam-3 call shape; defaulting to deferring",
				zap.String("session_id", sessionID), zap.String("call_shape", string(callShape)))
			s.deferSeam3QueueDrainRefusal(ctx, session.TaskID, sessionID, queuedMessageID, refusal, binding)
			return err
		}
	}
	s.logger.Debug("ensure execution for existing session (non-fatal)",
		zap.String("session_id", sessionID),
		zap.Error(err))
	return err
}

// resolveTaskAgentProfile applies the 5-step resolution chain on the backend:
// 1) workflow step override, 2) task.metadata.agent_profile_id,
// 3) workflow default, 4) Office task assignee, 5) workspace default. Returns the resolved profile id
// (or "" when none resolve) along with the workflow step it loaded (or nil).
// Returning the step lets callers reuse it (e.g. to gate auto-start) without a
// second DB lookup.
func (s *Service) resolveTaskAgentProfile(ctx context.Context, task *models.Task) (string, *wfmodels.WorkflowStep) {
	step := s.lookupWorkflowStep(ctx, task.WorkflowStepID)
	if step != nil {
		if id := s.resolveStepAgentProfileForTask(ctx, task, step); id != "" {
			return id, step
		}
	}
	if v, ok := task.Metadata["agent_profile_id"].(string); ok && v != "" {
		return v, step
	}
	if task.AssigneeAgentProfileID != "" {
		return task.AssigneeAgentProfileID, step
	}
	ws, err := s.repo.GetWorkspace(ctx, task.WorkspaceID)
	if err == nil && ws != nil && ws.DefaultAgentProfileID != nil && *ws.DefaultAgentProfileID != "" {
		return *ws.DefaultAgentProfileID, step
	}
	return "", step
}

// lookupWorkflowStep loads a workflow step by id, returning nil when the id
// is empty, the getter is unavailable, or the lookup fails.
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
