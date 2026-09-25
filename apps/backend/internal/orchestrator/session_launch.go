package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// sessionTerminalErrText mirrors lifecycle.ErrSessionTerminal's message. It is
// duplicated as a string rather than imported so this higher-level orchestrator
// file does not take a direct dependency on internal/agent/runtime/lifecycle
// (ARCH-RUNTIME-IMPORT): the launch failures reach here only as wrapped-error
// strings or a stringified persisted session error, so a string match is both
// sufficient and required (see IsBenignLaunchTeardownErr).
const sessionTerminalErrText = "session is terminal"

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

type LaunchActivationSource string

const (
	LaunchActivationSourceUserAction  LaunchActivationSource = "user_action"
	LaunchActivationSourceSessionOpen LaunchActivationSource = "session_open"

	activationDispositionSuppressed = "suppressed"
	activationDispositionQueued     = "queued"
)

type sessionOpenRecoveryContextKey struct{}

// sessionOpenRecoveryBlockedError carries a guarded ownership decision back to
// LaunchSession. Passive inspection treats that decision as a successful
// waiting response so the browser does not enter workspace recovery.
type sessionOpenRecoveryBlockedError struct {
	reason string
}

func (e *sessionOpenRecoveryBlockedError) Error() string {
	return "session_open recovery blocked: " + e.reason
}

func withSessionOpenRecoveryContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, sessionOpenRecoveryContextKey{}, true)
}

func isSessionOpenRecoveryContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	active, _ := ctx.Value(sessionOpenRecoveryContextKey{}).(bool)
	return active
}

// LaunchSessionRequest is the unified request for session.launch.
type LaunchSessionRequest struct {
	TaskID         string        `json:"task_id"`
	Intent         SessionIntent `json:"intent,omitempty"`
	SessionID      string        `json:"session_id,omitempty"`
	AgentProfileID string        `json:"agent_profile_id,omitempty"`
	// ProfileExplicit marks a non-empty profile selected through an explicit
	// selector-backed choice. It bypasses workflow-step profile resolution for
	// IntentStart only; IntentStartCreated keeps its existing profile resolution
	// behavior.
	ProfileExplicit   bool   `json:"profile_explicit,omitempty"`
	ExecutorID        string `json:"executor_id,omitempty"`
	ExecutorProfileID string `json:"executor_profile_id,omitempty"`
	Prompt            string `json:"prompt,omitempty"`
	PlanMode          bool   `json:"plan_mode,omitempty"`
	WorkflowStepID    string `json:"workflow_step_id,omitempty"`
	Priority          string `json:"priority,omitempty"`
	LaunchWorkspace   bool   `json:"launch_workspace,omitempty"`
	SkipMessageRecord bool   `json:"skip_message_record,omitempty"`
	AutoStart         bool   `json:"auto_start,omitempty"`
	// NoAgentLaunch marks a prepare request that must NEVER be upgraded into an
	// agent launch, even for passthrough profiles (whose prepare would normally
	// be eagerly upgraded so the PTY exists). It backs the session.ensure
	// auto_start=false override used by the prevent-auto-start-on-open
	// preference: the session is created workspace-only (CREATED) and the
	// Start agent button launches it later. It is an internal server-side flag
	// set from EnsureSessionOptions, kept off the wire protocol (`json:"-"`).
	NoAgentLaunch bool `json:"-"`
	// DeferredStart marks a prepare whose caller will follow up with an explicit
	// IntentStartCreated that carries the prompt (the two-phase create flow:
	// cheap sync prepare + async start). It suppresses the passthrough
	// launchPrepare→launchStart upgrade so the eager launch doesn't spawn a
	// promptless PTY and pre-empt the prompt-bearing start. It is an internal
	// server-side coordination flag set by the deferred-start handlers, so it is
	// kept off the wire protocol (`json:"-"`) — a client must not be able to
	// suppress the upgrade and strand a passthrough session without a PTY.
	DeferredStart bool `json:"-"`
	// InitialCreatePrompt marks the one eligible, prompt-bearing explicit-step
	// create flow. It is server-side provenance, so clients cannot turn the
	// generic launch path into a workflow turn-start admission.
	InitialCreatePrompt bool `json:"-"`
	// InitialPromptPreview is supplied only by task creation after attachment claim.
	InitialPromptPreview *models.InitialPromptPreview `json:"-"`
	Attachments          []v1.MessageAttachment       `json:"attachments,omitempty"`
	// SpawnOrigin identifies the agent session that requested this launch via
	// spawn_session_kandev, so the new session's first turn can carry spawner
	// attribution and reply instructions. Like DeferredStart it is kept off the
	// wire protocol (`json:"-"`): the launch site turns it into a *trusted*
	// <kandev-system> block that survives first-turn canonicalization, so a WS
	// client must not be able to forge one and fabricate server authority.
	SpawnOrigin *SpawnOrigin `json:"-"`
	// AllowBranchReplacement is set only by RecoverSession for the explicit
	// resume_new_branch action. Clients cannot grant this permission directly.
	AllowBranchReplacement bool `json:"-"`
	// AllowCompletedSessionResume is set only by explicit recovery or a pinned
	// follow-up dispatcher. It is intentionally not part of the wire request:
	// ordinary launch, ensure, and startup recovery paths must keep completed
	// sessions terminal.
	AllowCompletedSessionResume bool `json:"-"`
	// ActivationSource distinguishes passive task opening from an explicit
	// launch action. An omitted value preserves the existing behavior.
	ActivationSource LaunchActivationSource `json:"activation_source,omitempty"`
}

// SpawnOrigin describes the agent session that spawned a new sibling session.
// The identifiers are resolved server-side by the MCP layer from the calling
// agent's own session, never read from the tool arguments.
type SpawnOrigin struct {
	TaskID      string
	SessionID   string
	SessionName string
}

// LaunchSessionResponse is the unified response for session.launch.
type LaunchSessionResponse struct {
	Success                   bool                              `json:"success"`
	TaskID                    string                            `json:"task_id"`
	SessionID                 string                            `json:"session_id,omitempty"`
	AgentExecutionID          string                            `json:"agent_execution_id,omitempty"`
	AgentProfileID            string                            `json:"agent_profile_id,omitempty"`
	State                     string                            `json:"state"`
	WorktreePath              *string                           `json:"worktree_path,omitempty"`
	WorktreeBranch            *string                           `json:"worktree_branch,omitempty"`
	ActivationDisposition     string                            `json:"activation_disposition,omitempty"`
	ActivationReason          string                            `json:"activation_reason,omitempty"`
	ExactProfileLaunchReceipt *models.ExactProfileLaunchReceipt `json:"exact_profile_launch_receipt,omitempty"`
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

// IsBenignLaunchTeardownErr reports whether a session-launch failure is an
// expected graceful-shutdown teardown race rather than a genuine fault. During
// shutdown the root context is cancelled and terminal sessions reject launches,
// so an in-flight session.launch fails predictably; those should log WARN
// without a stack trace, not ERROR.
//
// Two shapes reach the launch handler on shutdown:
//   - restore_workspace wraps lifecycle.ErrSessionTerminal with %w and the
//     cancelled root context surfaces context.Canceled, so errors.Is matches
//     the context sentinel.
//   - resume stringifies a persisted session error (task_operations.go uses %s
//     on sess.ErrorMessage), destroying any sentinel, so a bounded string
//     fallback for "context canceled"/"session is terminal" is required. The
//     terminal-session text is matched as a string (not errors.Is) to avoid a
//     higher-level import of the runtime/lifecycle seam.
func IsBenignLaunchTeardownErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, context.Canceled.Error()) ||
		strings.Contains(msg, sessionTerminalErrText)
}

// LaunchSession is the unified entry point for all session operations.
func (s *Service) LaunchSession(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	if req == nil {
		return nil, errors.New("launch request is required")
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if err := validateLaunchActivationSource(req.ActivationSource); err != nil {
		return nil, err
	}
	if req.ActivationSource == LaunchActivationSourceSessionOpen && req.Prompt != "" {
		return nil, errors.New("session_open activation cannot include a prompt")
	}
	intent := ResolveIntent(req)
	// Every intent funnels through here. SessionID is empty when creating, so
	// that case is carried by the task check alone.
	// Launching a session starts an agent turn: session.prompt.
	// Workspace restoration only opens retained infrastructure. It must not
	// require permission to start or resume an agent; lifecycle applies the
	// session.exec check at the execution boundary.
	if intent != IntentRestoreWorkspace {
		if err := s.authorizeTaskPrompt(ctx, req.TaskID); err != nil {
			return nil, err
		}
	}
	// Existing-session launches must also prove that the supplied session and
	// task belong together; independent reach checks do not establish that
	// binding when a caller can access more than one task.
	if err := s.authorizeTaskSessionPair(ctx, req.TaskID, req.SessionID); err != nil {
		return nil, err
	}
	if response := s.passiveLaunchResponse(ctx, req, intent); response != nil {
		return response, nil
	}
	if err := s.claimLaunchAttachments(ctx, req); err != nil {
		return nil, fmt.Errorf("claim launch attachments: %w", err)
	}
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

func validateLaunchActivationSource(source LaunchActivationSource) error {
	if source != "" && source != LaunchActivationSourceUserAction && source != LaunchActivationSourceSessionOpen {
		return fmt.Errorf("unknown launch activation source %q", source)
	}
	return nil
}

// passiveLaunchResponse is the server-side guard for browser inspection. The
// status endpoint is the normal fast path, but a launch request must repeat
// the ownership check because the status can be stale by the time the browser
// sends it. It returns a successful no-execution disposition so callers do not
// fall back to a fresh session or restore attempt.
func (s *Service) passiveLaunchResponse(
	ctx context.Context, req *LaunchSessionRequest, intent SessionIntent,
) *LaunchSessionResponse {
	if req == nil || req.ActivationSource != LaunchActivationSourceSessionOpen || s.repo == nil {
		return nil
	}
	switch intent {
	case IntentPrepare, IntentRestoreWorkspace, IntentWorkflowStep:
		return nil
	}
	if req.SessionID == "" {
		return s.deferredLaunchResponseIfPresent(ctx, req, "")
	}

	task, taskErr := s.repo.GetTask(ctx, req.TaskID)
	session, sessionErr := s.repo.GetTaskSession(ctx, req.SessionID)
	if taskErr != nil || sessionErr != nil || task == nil || session == nil || session.TaskID != req.TaskID {
		return &LaunchSessionResponse{
			Success:               true,
			TaskID:                req.TaskID,
			SessionID:             req.SessionID,
			State:                 sessionStateOrEmpty(session),
			AgentProfileID:        sessionProfileOrEmpty(session),
			ActivationDisposition: activationDispositionSuppressed,
			ActivationReason:      "ownership_unavailable",
		}
	}
	allowed, reason := s.autoResumeEligibility(ctx, task, session)
	if allowed {
		return nil
	}
	disposition := activationDispositionSuppressed
	if reason == autoResumeBlockedLaunchQueued {
		disposition = activationDispositionQueued
	}
	return &LaunchSessionResponse{
		Success:               true,
		TaskID:                req.TaskID,
		SessionID:             session.ID,
		State:                 string(session.State),
		AgentProfileID:        session.AgentProfileID,
		ActivationDisposition: disposition,
		ActivationReason:      reason,
	}
}

func sessionStateOrEmpty(session *models.TaskSession) string {
	if session == nil {
		return ""
	}
	return string(session.State)
}

func sessionProfileOrEmpty(session *models.TaskSession) string {
	if session == nil {
		return ""
	}
	return session.AgentProfileID
}

func (s *Service) claimLaunchAttachments(ctx context.Context, req *LaunchSessionRequest) error {
	hasDescriptor := false
	for _, attachment := range req.Attachments {
		if attachment.AttachmentID != "" {
			hasDescriptor = true
			break
		}
	}
	if !hasDescriptor {
		return nil
	}
	if s.launchAttachmentClaimer == nil {
		return errors.New("launch attachment claimer is not configured")
	}
	return s.launchAttachmentClaimer.ClaimMessageAttachments(
		ctx,
		req.TaskID,
		req.SessionID,
		req.Attachments,
	)
}

// launchPrepare creates a session entry without launching the agent.
// Passthrough profiles can't be "prepared" without a running PTY — the terminal
// has nothing to attach to until the agent process exists. Upgrade those calls
// to a full start so the PTY is ready by the time the user sees the terminal.
//
// AutoStart=true means we arrived here from launchStart's blocked-auto-start
// downgrade path; skipping the upgrade in that case avoids a launchStart ↔
// launchPrepare bounce.
//
// DeferredStart=true means a prompt-bearing IntentStartCreated will follow this
// prepare (the two-phase create flow); skipping the upgrade there leaves the
// session CREATED so that follow-up start launches the passthrough agent WITH
// the prompt — eagerly launching here would spawn a promptless PTY and the
// later start would be rejected against the now-running session.
func (s *Service) launchPrepare(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	prepareCtx := withInitialPromptPreview(ctx, req.InitialPromptPreview)
	if s.shouldUpgradePassthroughPrepare(ctx, req) {
		return s.launchStart(prepareCtx, req)
	}
	sessionID, err := s.PrepareTaskSession(
		prepareCtx, req.TaskID, req.AgentProfileID, req.ExecutorID,
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

// shouldUpgradePassthroughPrepare reports whether a prepare request for a
// passthrough profile should be eagerly upgraded to a full launch so a PTY
// exists for the terminal to attach to. It is the single decision point for the
// upgrade documented on launchPrepare: only genuine prepare-only callers (no
// imminent prompt-bearing start) get the eager launch. See launchPrepare for
// why AutoStart and DeferredStart each suppress it.
func (s *Service) shouldUpgradePassthroughPrepare(ctx context.Context, req *LaunchSessionRequest) bool {
	if req.NoAgentLaunch || req.ActivationSource == LaunchActivationSourceSessionOpen {
		return false
	}
	return !req.AutoStart && !req.DeferredStart && s.isPassthroughProfile(ctx, req.AgentProfileID)
}

// isPassthroughProfile reports whether the agent profile is a CLI
// passthrough provider.
func (s *Service) isPassthroughProfile(ctx context.Context, profileID string) bool {
	if profileID == "" || s.agentManager == nil {
		return false
	}
	info, err := s.agentManager.ResolveAgentProfile(ctx, profileID)
	if err != nil || info == nil {
		return false
	}
	return info.CLIPassthrough
}

// blocksAutoStartLaunch reports whether an auto-start request must be
// downgraded to a prepare, either because the task's current step does not
// allow it or because it has an unresolved dependency. The dependency gate's
// launch-token restore concern does not apply here: this path owns no
// lifecycle token to restore.
func (s *Service) blocksAutoStartLaunch(ctx context.Context, req *LaunchSessionRequest) bool {
	if s.shouldBlockAutoStart(ctx, req) {
		return true
	}
	blocked, _ := s.dependencyBlocksAutoStart(ctx, req.TaskID, "session.launch")
	return blocked
}

// launchStart creates a new session and launches the agent.
// If the request is an auto-start and the task's current workflow step does not
// have auto_start_agent, or the task has unresolved dependencies, the request
// is downgraded to a prepare (workspace-only, no agent) to prevent unwanted
// auto-starts from the frontend's useAutoStartSession hook.
func (s *Service) launchStart(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	autoStart := req.AutoStart || req.ActivationSource == LaunchActivationSourceSessionOpen
	if autoStart && s.blocksAutoStartLaunch(ctx, req) {
		req.LaunchWorkspace = true
		return s.launchPrepare(ctx, req)
	}

	execution, err := s.startTask(
		ctx, req.TaskID, req.AgentProfileID, req.ExecutorID,
		req.ExecutorProfileID, req.Priority, req.Prompt,
		req.WorkflowStepID, req.PlanMode, autoStart, req.Attachments,
		startTaskOptions{ProfileExplicit: req.ProfileExplicit, SpawnOrigin: req.SpawnOrigin},
	)
	if errors.Is(err, ErrCeilingLaunchDeferred) {
		return s.deferredLaunchResponse(ctx, req, "")
	}
	if err != nil {
		return nil, err
	}
	if execution == nil {
		if response := s.deferredLaunchResponseIfPresent(ctx, req, ""); response != nil {
			return response, nil
		}
		// The automatic terminal-PR gate intentionally skips session creation.
		// Return a successful no-op response so session.ensure and WS callers do
		// not dereference a nil execution while the task-owned error card remains
		// the recovery surface.
		return &LaunchSessionResponse{
			Success: true,
			TaskID:  req.TaskID,
		}, nil
	}
	return s.withExactProfileLaunchReceipt(ctx, executionToLaunchResponse(req.TaskID, execution)), nil
}

func (s *Service) withExactProfileLaunchReceipt(
	ctx context.Context,
	response *LaunchSessionResponse,
) *LaunchSessionResponse {
	if response == nil || response.SessionID == "" {
		return response
	}
	receipts, ok := s.repo.(interface {
		GetExactProfileLaunchReceipt(context.Context, string, string) (*models.ExactProfileLaunchReceipt, error)
	})
	if !ok {
		return response
	}
	receipt, err := receipts.GetExactProfileLaunchReceipt(ctx, response.TaskID, response.SessionID)
	if err != nil {
		s.logger.Warn("failed to load exact-profile launch receipt",
			zap.String("task_id", response.TaskID),
			zap.String("session_id", response.SessionID),
			zap.Error(err))
		return response
	}
	response.ExactProfileLaunchReceipt = receipt
	return response
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
	if req.InitialCreatePrompt {
		return s.launchInitialCreatePrompt(ctx, req)
	}
	autoStart := req.AutoStart || req.ActivationSource == LaunchActivationSourceSessionOpen
	parkingStamp := s.captureWorkflowParkingStamp(ctx, req.SessionID)
	execution, err := s.StartCreatedSession(
		ctx, req.TaskID, req.SessionID, req.AgentProfileID,
		req.Prompt, req.SkipMessageRecord, req.PlanMode, autoStart, req.Attachments, nil,
	)
	if err != nil {
		return nil, err
	}
	if execution == nil {
		if response := s.deferredLaunchResponseIfPresent(ctx, req, req.SessionID); response != nil {
			return response, nil
		}
	}
	if execution != nil {
		s.clearWorkflowParkingForSession(ctx, req.SessionID, parkingStamp)
	}
	return s.withExactProfileLaunchReceipt(ctx, executionToLaunchResponse(req.TaskID, execution)), nil
}

// launchResume resumes a stopped session.
func (s *Service) launchResume(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	parkingStamp := s.captureWorkflowParkingStamp(ctx, req.SessionID)
	resumeCtx := ctx
	if req.ActivationSource == LaunchActivationSourceSessionOpen {
		resumeCtx = withSessionOpenRecoveryContext(ctx)
	}
	execution, err := s.ResumeTaskSessionWithOptions(resumeCtx, req.TaskID, req.SessionID, executor.ResumeOptions{
		AllowBranchReplacement:      req.AllowBranchReplacement,
		AllowCompletedSessionResume: req.AllowCompletedSessionResume,
		Origin:                      string(launchOriginForActivation(req)),
	})
	if err != nil {
		var blocked *sessionOpenRecoveryBlockedError
		if errors.As(err, &blocked) {
			return s.sessionOpenRecoveryWaitingResponse(ctx, req, blocked.reason), nil
		}
		if req.ActivationSource == LaunchActivationSourceSessionOpen && errors.Is(err, ErrCeilingLaunchConflict) {
			return s.sessionOpenRecoveryWaitingResponse(ctx, req, "session_capacity"), nil
		}
		return nil, err
	}
	if execution == nil {
		if response := s.deferredLaunchResponseIfPresent(ctx, req, req.SessionID); response != nil {
			return response, nil
		}
	}
	if execution != nil {
		s.clearWorkflowParkingForSession(ctx, req.SessionID, parkingStamp)
	}
	return s.withExactProfileLaunchReceipt(ctx, executionToLaunchResponse(req.TaskID, execution)), nil
}

func (s *Service) sessionOpenRecoveryWaitingResponse(
	ctx context.Context,
	req *LaunchSessionRequest,
	reason string,
) *LaunchSessionResponse {
	var session *models.TaskSession
	if s != nil && s.repo != nil && req != nil {
		session, _ = s.repo.GetTaskSession(ctx, req.SessionID)
	}
	disposition := activationDispositionSuppressed
	if reason == autoResumeBlockedLaunchQueued {
		disposition = activationDispositionQueued
	}
	return &LaunchSessionResponse{
		Success:               true,
		TaskID:                req.TaskID,
		SessionID:             req.SessionID,
		State:                 sessionStateOrEmpty(session),
		AgentProfileID:        sessionProfileOrEmpty(session),
		ActivationDisposition: disposition,
		ActivationReason:      reason,
	}
}

func launchOriginForActivation(req *LaunchSessionRequest) launchOrigin {
	if req != nil && req.ActivationSource == LaunchActivationSourceSessionOpen {
		return launchOriginAutomatic
	}
	return originFromAutoStart(req != nil && req.AutoStart)
}

func (s *Service) deferredLaunchResponse(
	ctx context.Context,
	req *LaunchSessionRequest,
	sessionID string,
) (*LaunchSessionResponse, error) {
	response := s.deferredLaunchResponseIfPresent(ctx, req, sessionID)
	if response == nil {
		return nil, ErrCeilingLaunchDeferred
	}
	return response, nil
}

func (s *Service) deferredLaunchResponseIfPresent(
	ctx context.Context,
	req *LaunchSessionRequest,
	sessionID string,
) *LaunchSessionResponse {
	if req == nil {
		return nil
	}
	task, err := s.repo.GetTask(ctx, req.TaskID)
	if err != nil || task == nil {
		return deferredLaunchSuppressedForInspection(req, sessionID, autoResumeBlockedOwnershipUnavailable)
	}
	if !models.HasCeilingDeferredIntent(task) {
		return nil
	}
	record, _ := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	deferral, err := models.ReadCeilingDeferral(record)
	if err != nil {
		return deferredLaunchOwnershipUnavailableResponse(req, sessionID)
	}
	return s.deferredLaunchResponseForDeferral(req, task, deferral, sessionID)
}

func (s *Service) deferredLaunchResponseForDeferral(
	req *LaunchSessionRequest,
	task *models.Task,
	deferral models.CeilingDeferral,
	sessionID string,
) *LaunchSessionResponse {
	// Seam-1 workflow starts do not have a session id in their payload. Resolve
	// that destination from the task-owned route so opening the task reports the
	// exact queued session instead of falling back to the request's empty id.
	deferredSessionID := models.CeilingDeferralSessionID(task, deferral)
	if sessionID != "" && deferredSessionID != "" && sessionID != deferredSessionID {
		return deferredLaunchSuppressedForInspection(req, sessionID, autoResumeBlockedOwnershipUnavailable)
	}
	_, bindingPresent, bindingErr := models.ReadCeilingWorkflowEntryBinding(deferral.Payload)
	workflowOrigin := deferredLaunchIsWorkflowOrigin(task, deferral, bindingPresent)
	if req.ActivationSource == LaunchActivationSourceSessionOpen &&
		(bindingErr != nil || (workflowOrigin && (deferredSessionID == "" ||
			(bindingPresent && !models.CeilingDeferralTargetsSession(task, deferral, deferredSessionID))))) {
		return deferredLaunchSuppressedForInspection(req, sessionID, autoResumeBlockedOwnershipUnavailable)
	}
	if sessionID == "" {
		sessionID = deferredSessionID
		if sessionID == "" {
			sessionID = req.SessionID
		}
	}
	agentProfileID := req.AgentProfileID
	if agentProfileID == "" {
		agentProfileID = stringField(deferral.Payload, metaKeyAgentProfileID)
	}
	return &LaunchSessionResponse{
		Success:               true,
		TaskID:                req.TaskID,
		SessionID:             sessionID,
		AgentProfileID:        agentProfileID,
		State:                 string(models.TaskSessionStateCreated),
		ActivationDisposition: activationDispositionQueued,
		ActivationReason:      "session_capacity",
	}
}

func deferredLaunchSuppressedForInspection(
	req *LaunchSessionRequest,
	sessionID, reason string,
) *LaunchSessionResponse {
	if req == nil || req.ActivationSource != LaunchActivationSourceSessionOpen {
		return nil
	}
	return &LaunchSessionResponse{
		Success:               true,
		TaskID:                req.TaskID,
		SessionID:             sessionID,
		ActivationDisposition: activationDispositionSuppressed,
		ActivationReason:      reason,
	}
}

func deferredLaunchOwnershipUnavailableResponse(
	req *LaunchSessionRequest,
	sessionID string,
) *LaunchSessionResponse {
	return &LaunchSessionResponse{
		Success:               true,
		TaskID:                req.TaskID,
		SessionID:             sessionID,
		ActivationDisposition: activationDispositionSuppressed,
		ActivationReason:      "ownership_unavailable",
	}
}

func deferredLaunchIsWorkflowOrigin(
	task *models.Task,
	deferral models.CeilingDeferral,
	bindingPresent bool,
) bool {
	workflowOrigin := bindingPresent || stringField(deferral.Payload, metaKeyWorkflowStepID) != "" ||
		int64Field(deferral.Payload, "workflow_entry_id") > 0
	if deferral.Kind != models.CeilingLaunchStart {
		return workflowOrigin
	}
	_, routePresent := models.LoadWorkflowSessionRoute(task.Metadata)
	return workflowOrigin || routePresent
}

func (s *Service) captureWorkflowParkingStamp(ctx context.Context, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return ""
	}
	parking, ok := models.LoadWorkflowParking(session.Metadata)
	if !ok {
		return ""
	}
	return parking.Stamp
}

// clearWorkflowParkingForSession removes only the stamped parking marker that
// was authorized before the launch. A later park writes a new stamp and is
// therefore preserved when an older launch completes.
func (s *Service) clearWorkflowParkingForSession(ctx context.Context, sessionID, authorizedStamp string) {
	if sessionID == "" || strings.TrimSpace(authorizedStamp) == "" {
		return
	}
	remover, ok := s.repo.(workflowProfileSwitchStopIntentRemover)
	if !ok {
		return
	}
	if _, err := remover.RemoveSessionMetadataKeyIfStamp(
		ctx, sessionID, models.SessionMetaKeyWorkflowParking, authorizedStamp,
	); err != nil {
		s.logger.Warn("failed to clear workflow parking marker after launch",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

// launchWorkflowStep starts a session with workflow step prompt configuration.
func (s *Service) launchWorkflowStep(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	err := s.StartSessionForWorkflowStep(ctx, req.TaskID, req.SessionID, req.WorkflowStepID)
	if err != nil {
		return nil, err
	}
	return s.withExactProfileLaunchReceipt(ctx, &LaunchSessionResponse{
		Success:   true,
		TaskID:    req.TaskID,
		SessionID: req.SessionID,
		State:     string(v1.TaskSessionStateRunning),
	}), nil
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
	if err := s.ensureTaskNotArchived(ctx, req.TaskID); err != nil {
		return nil, err
	}

	if err := s.agentManager.EnsureWorkspaceExecutionForSession(ctx, req.TaskID, req.SessionID); err != nil {
		return nil, fmt.Errorf("failed to restore workspace: %w", err)
	}
	agentExecutionID, _ := s.agentManager.GetExecutionIDForSession(ctx, req.SessionID)

	resp := &LaunchSessionResponse{
		Success:          true,
		TaskID:           req.TaskID,
		SessionID:        req.SessionID,
		AgentExecutionID: agentExecutionID,
		State:            string(session.State),
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
// action is "resume" (retry with existing ACP session), "resume_new_branch"
// (retry after replacing a confirmed missing branch), or "fresh_start" (clear
// token, start fresh).
func (s *Service) RecoverSession(ctx context.Context, taskID, sessionID, action string) (*LaunchSessionResponse, error) {
	// Guard before the switch: "fresh_start" clears the resume token, so an
	// unauthorized call would mutate the session even if the launch failed.
	// Recovering a session resumes an agent turn: session.prompt.
	if err := s.authorizeSessionPrompt(ctx, sessionID); err != nil {
		return nil, err
	}
	if err := s.authorizeTask(ctx, taskID); err != nil {
		return nil, err
	}
	if err := s.ensureTaskNotArchived(ctx, taskID); err != nil {
		return nil, err
	}
	if action == "runtime_retry" {
		if s.wasResumeAttempt(ctx, sessionID) {
			action = "resume"
		} else {
			action = "fresh_start"
		}
	}
	switch action {
	case "fresh_start":
		if err := s.clearResumeToken(ctx, sessionID); err != nil {
			return nil, fmt.Errorf("failed to clear resume token for fresh start: %w", err)
		}
	case "resume":
		// no-op — relaunch with existing resume token
	case "resume_new_branch":
		// The launch carries the explicit permission. It is not persisted on the
		// session and cannot be inferred from a previous failed attempt.
	default:
		return nil, fmt.Errorf("invalid recovery action: %s", action)
	}

	resp, err := s.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:                      taskID,
		SessionID:                   sessionID,
		Intent:                      IntentResume,
		AllowBranchReplacement:      action == "resume_new_branch",
		AllowCompletedSessionResume: action == "resume",
	})
	if err != nil {
		return nil, normalizeRecoverSessionError(err)
	}
	return resp, nil
}

// normalizeRecoverSessionError maps a missing-profile resume failure to a
// user-actionable message.
func normalizeRecoverSessionError(err error) error {
	if err == nil {
		return nil
	}
	if isMissingProfileResumeError(err) {
		return fmt.Errorf("the agent profile used by this session was deleted; start a new session and choose an available agent profile: %w", err)
	}
	return err
}

// isMissingProfileResumeError reports whether the error indicates the
// session's agent profile no longer exists.
func isMissingProfileResumeError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "failed to resolve agent profile") ||
		strings.Contains(msg, "agent profile not found")
}

// executionToLaunchResponse converts a TaskExecution to a LaunchSessionResponse.
func executionToLaunchResponse(taskID string, exec *executor.TaskExecution) *LaunchSessionResponse {
	if exec == nil {
		return &LaunchSessionResponse{
			Success: true,
			TaskID:  taskID,
		}
	}
	resp := &LaunchSessionResponse{
		Success:          true,
		TaskID:           taskID,
		SessionID:        exec.SessionID,
		AgentExecutionID: exec.AgentExecutionID,
		AgentProfileID:   exec.AgentProfileID,
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
