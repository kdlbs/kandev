package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
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

const sessionRecoveryActionContinueFromHistory = "continue_from_history"

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

// sessionContinuityStore is optional so lightweight test repositories and
// older remote repository adapters can keep the existing launch contract. The
// production SQL repository implements the complete persistence boundary.
type sessionContinuityStore interface {
	CreateHarnessSessionGeneration(context.Context, *models.HarnessSessionGeneration) error
	GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error)
	CommitHarnessSessionGeneration(context.Context, *models.HarnessSessionGeneration, int64) (bool, error)
	CreateRestoreAttempt(context.Context, *models.RestoreAttempt) error
	CompleteRestoreAttempt(context.Context, string, string, time.Time) error
	CreateContinuationSnapshot(context.Context, *models.ContinuationSnapshot) error
	CompleteContinuationSnapshot(context.Context, string, string, time.Time) error
}

type sessionRecoveryBlockStore interface {
	UpsertSessionRecoveryBlock(context.Context, *models.SessionRecoveryBlock) error
	GetOpenSessionRecoveryBlock(context.Context, string, string, int64) (*models.SessionRecoveryBlock, error)
	ResolveSessionRecoveryBlock(context.Context, string, string, time.Time) (bool, error)
}

type sessionRecoveryBlockLookup interface {
	GetSessionRecoveryBlock(context.Context, string) (*models.SessionRecoveryBlock, error)
}

type recoveryRequiredSignal interface {
	RecoveryReason() string
}

// ErrSessionRecoveryRequired prevents ordinary and autonomous launch callers
// from dispatching work while the native harness outcome is unsettled.
var ErrSessionRecoveryRequired = errors.New("session recovery required")

type sessionRecoveryRequiredError struct {
	Block *models.SessionRecoveryBlock
}

func (e *sessionRecoveryRequiredError) Error() string {
	if e == nil || e.Block == nil {
		return ErrSessionRecoveryRequired.Error()
	}
	return fmt.Sprintf("%s: %s", ErrSessionRecoveryRequired, e.Block.Reason)
}

func (e *sessionRecoveryRequiredError) Unwrap() error { return ErrSessionRecoveryRequired }

// RecoveryReason exposes the persisted classification to autonomous callers
// that must park their own work instead of entering a generic retry loop.
func (e *sessionRecoveryRequiredError) RecoveryReason() string {
	if e == nil || e.Block == nil || e.Block.Reason == "" {
		return "unknown_failure"
	}
	return e.Block.Reason
}

// RecoveryGeneration identifies the generation the operator is settling. It
// is optional browser context and is omitted when no persisted block carries
// one.
func (e *sessionRecoveryRequiredError) RecoveryGeneration() int64 {
	if e == nil || e.Block == nil {
		return 0
	}
	return e.Block.ExpectedGeneration
}

// DispatchDeferred leaves an automation run admitted but open until the
// operator resolves the persisted recovery block. The method satisfies the
// automation package's generic deferred-dispatch contract without coupling
// that package to orchestrator error types.
func (e *sessionRecoveryRequiredError) DispatchDeferred() string {
	return e.RecoveryReason()
}

type continuationCheckpoint struct {
	store         sessionContinuityStore
	attemptID     string
	snapshotID    string
	submissionID  string
	sessionID     string
	incarnationID string
	expectedGen   int64
	workspace     string
	nativeID      string
	// The candidate execution is deliberately tracked separately from the
	// committed harness generation. A candidate must be torn down, and the
	// pre-recovery session state restored, if the generation CAS loses after
	// candidate initialization.
	candidateExecutionID string
	previousState        models.TaskSessionState
	previousErrorMessage string
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
	DeferredStart bool                   `json:"-"`
	Attachments   []v1.MessageAttachment `json:"attachments,omitempty"`
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
	// ForceContextContinuation is set only by RecoverSession for the explicit
	// continue_from_history action. Clients cannot bypass native resume through
	// the general session.launch request.
	ForceContextContinuation bool   `json:"-"`
	ContinuationPrompt       string `json:"-"`
	// RecoveryAction authorizes one explicit settlement of an existing
	// recovery block. It is populated only by RecoverSession and is consumed
	// after a successful launch, so a failed recovery remains blocked.
	RecoveryAction string `json:"-"`
	// DeferRecoveryResolution keeps the existing recovery block and parked
	// work in place until an explicit continuation prompt has crossed the
	// durable admission boundary. It is set only by RecoverSession.
	DeferRecoveryResolution bool `json:"-"`
	// AllowCompletedSessionResume is set only by explicit recovery or a pinned
	// follow-up dispatcher. It is intentionally not part of the wire request:
	// ordinary launch, ensure, and startup recovery paths must keep completed
	// sessions terminal.
	AllowCompletedSessionResume bool `json:"-"`
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
	Success          bool    `json:"success"`
	TaskID           string  `json:"task_id"`
	SessionID        string  `json:"session_id,omitempty"`
	AgentExecutionID string  `json:"agent_execution_id,omitempty"`
	AgentProfileID   string  `json:"agent_profile_id,omitempty"`
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
	// Every intent funnels through here. SessionID is empty when creating, so
	// that case is carried by the task check alone.
	// Launching a session starts an agent turn: session.prompt.
	if err := s.authorizeTaskPrompt(ctx, req.TaskID); err != nil {
		return nil, err
	}
	// Existing-session launches must also prove that the supplied session and
	// task belong together; independent reach checks do not establish that
	// binding when a caller can access more than one task.
	if err := s.authorizeTaskSessionPair(ctx, req.TaskID, req.SessionID); err != nil {
		return nil, err
	}
	if req.RecoveryAction == "" {
		if err := s.checkSessionRecoveryBlock(ctx, req.SessionID); err != nil {
			return nil, err
		}
	}
	if err := s.claimLaunchAttachments(ctx, req); err != nil {
		return nil, fmt.Errorf("claim launch attachments: %w", err)
	}
	intent := ResolveIntent(req)
	req.Prompt = strings.TrimSpace(req.Prompt)

	var (
		response *LaunchSessionResponse
		err      error
	)
	switch intent {
	case IntentPrepare:
		response, err = s.launchPrepare(ctx, req)
	case IntentStart:
		response, err = s.launchStart(ctx, req)
	case IntentStartCreated:
		response, err = s.launchStartCreated(ctx, req)
	case IntentResume:
		response, err = s.launchResume(ctx, req)
	case IntentWorkflowStep:
		response, err = s.launchWorkflowStep(ctx, req)
	case IntentRestoreWorkspace:
		response, err = s.launchRestoreWorkspace(ctx, req)
	default:
		err = fmt.Errorf("unknown intent: %s", intent)
	}
	if err != nil {
		if req.RecoveryAction == "" {
			if blockErr := s.recordRecoveryBlockForError(ctx, req, err); blockErr != nil {
				err = errors.Join(err, blockErr)
			}
		}
		return nil, err
	}
	if req.RecoveryAction != "" && !req.DeferRecoveryResolution {
		if err := s.resolveSessionRecoveryBlock(ctx, req.SessionID, req.RecoveryAction); err != nil {
			return nil, err
		}
	}
	return response, nil
}

func (s *Service) checkSessionRecoveryBlock(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	store, ok := s.repo.(sessionRecoveryBlockStore)
	if !ok {
		return nil
	}
	incarnationID, generation, err := s.recoverySessionIdentity(ctx, sessionID)
	if err != nil {
		return err
	}
	block, err := store.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, generation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check session recovery block: %w", err)
	}
	return &sessionRecoveryRequiredError{Block: block}
}

// GetOpenSessionRecoveryBlock returns the canonical block for an autonomous
// consumer that needs to retain its own pending work reference. A missing
// block is represented by (nil, nil), matching the ordinary admission check.
func (s *Service) GetOpenSessionRecoveryBlock(ctx context.Context, sessionID string) (*models.SessionRecoveryBlock, error) {
	if sessionID == "" {
		return nil, nil
	}
	store, ok := s.repo.(sessionRecoveryBlockStore)
	if !ok {
		return nil, nil
	}
	incarnationID, generation, err := s.recoverySessionIdentity(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	block, err := store.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, generation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load session recovery block: %w", err)
	}
	return block, nil
}

// GetSessionRecoveryBlock returns a block by identity for autonomous owners
// that release parked work only after the canonical action is resolved.
func (s *Service) GetSessionRecoveryBlock(ctx context.Context, blockID string) (*models.SessionRecoveryBlock, error) {
	if blockID == "" {
		return nil, nil
	}
	lookup, ok := s.repo.(sessionRecoveryBlockLookup)
	if !ok {
		return nil, nil
	}
	block, err := lookup.GetSessionRecoveryBlock(ctx, blockID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load session recovery block %s: %w", blockID, err)
	}
	return block, nil
}

func (s *Service) resolveSessionRecoveryBlock(ctx context.Context, sessionID, action string) error {
	if sessionID == "" || action == "" {
		return nil
	}
	store, ok := s.repo.(sessionRecoveryBlockStore)
	if !ok {
		return nil
	}
	incarnationID, generation, err := s.recoverySessionIdentity(ctx, sessionID)
	if err != nil {
		return err
	}
	block, err := store.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, generation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load session recovery block for resolution: %w", err)
	}
	resolved, err := store.ResolveSessionRecoveryBlock(ctx, block.ID, action, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("resolve session recovery block: %w", err)
	}
	if !resolved {
		return fmt.Errorf("resolve session recovery block: block is no longer open")
	}
	if err := s.restorePendingQueueDispatchesForRecovery(ctx, sessionID); err != nil {
		return fmt.Errorf("release parked queue work after session recovery: %w", err)
	}
	if err := s.resumeAutomationRunsAfterRecovery(ctx, block); err != nil {
		return fmt.Errorf("resume parked automation work after session recovery: %w", err)
	}
	return nil
}

func (s *Service) recoverySessionIdentity(ctx context.Context, sessionID string) (string, int64, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", 0, fmt.Errorf("load session recovery identity: %w", err)
	}
	if session == nil {
		return "", 0, models.ErrTaskSessionNotFound
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	generation := int64(0)
	if continuity, ok := s.repo.(sessionContinuityStore); ok {
		current, generationErr := continuity.GetCurrentHarnessSessionGeneration(ctx, session.ID, incarnationID)
		switch {
		case generationErr == nil && current != nil:
			generation = current.Generation
		case generationErr == nil:
		case errors.Is(generationErr, models.ErrTaskSessionNotFound), errors.Is(generationErr, sql.ErrNoRows):
		default:
			return "", 0, fmt.Errorf("load current harness generation: %w", generationErr)
		}
	}
	return incarnationID, generation, nil
}

func (s *Service) recordRecoveryBlockForError(ctx context.Context, req *LaunchSessionRequest, launchErr error) error {
	if req == nil || req.SessionID == "" || launchErr == nil {
		return nil
	}
	consumer := "interactive"
	if req.AutoStart {
		consumer = "queue"
	}
	return s.recordSessionRecoveryBlock(ctx, req.SessionID, consumer, launchErr)
}

func (s *Service) recordSessionRecoveryBlock(
	ctx context.Context, sessionID, consumer string, launchErr error,
) error {
	if sessionID == "" || launchErr == nil {
		return nil
	}
	var signal recoveryRequiredSignal
	if !errors.As(launchErr, &signal) {
		return nil
	}
	store, ok := s.repo.(sessionRecoveryBlockStore)
	if !ok {
		return nil
	}
	incarnationID, generation, err := s.recoverySessionIdentity(ctx, sessionID)
	if err != nil {
		return err
	}
	reason := signal.RecoveryReason()
	if reason == "" {
		reason = "unknown_failure"
	}
	if consumer == "" {
		consumer = "interactive"
	}
	block := &models.SessionRecoveryBlock{
		SessionID:          sessionID,
		IncarnationID:      incarnationID,
		ExpectedGeneration: generation,
		Reason:             reason,
		State:              models.RecoveryBlockOpen,
		ConsumerReference:  consumer,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	if existing, getErr := store.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, generation); getErr == nil && existing != nil {
		var typed *sessionRecoveryRequiredError
		if errors.As(launchErr, &typed) {
			typed.Block = existing
		}
		return nil
	} else if getErr != nil && !errors.Is(getErr, sql.ErrNoRows) {
		return getErr
	}
	if err := store.UpsertSessionRecoveryBlock(ctx, block); err != nil {
		return fmt.Errorf("persist session recovery block: %w", err)
	}
	var typed *sessionRecoveryRequiredError
	if errors.As(launchErr, &typed) {
		typed.Block = block
	}
	agentruntime.RecordRecoveryRequired(consumer, reason)
	return nil
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
	if s.shouldUpgradePassthroughPrepare(ctx, req) {
		return s.launchStart(ctx, req)
	}
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

// shouldUpgradePassthroughPrepare reports whether a prepare request for a
// passthrough profile should be eagerly upgraded to a full launch so a PTY
// exists for the terminal to attach to. It is the single decision point for the
// upgrade documented on launchPrepare: only genuine prepare-only callers (no
// imminent prompt-bearing start) get the eager launch. See launchPrepare for
// why AutoStart and DeferredStart each suppress it.
func (s *Service) shouldUpgradePassthroughPrepare(ctx context.Context, req *LaunchSessionRequest) bool {
	if req.NoAgentLaunch {
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
	if req.AutoStart && s.blocksAutoStartLaunch(ctx, req) {
		req.LaunchWorkspace = true
		return s.launchPrepare(ctx, req)
	}

	execution, err := s.startTask(
		ctx, req.TaskID, req.AgentProfileID, req.ExecutorID,
		req.ExecutorProfileID, req.Priority, req.Prompt,
		req.WorkflowStepID, req.PlanMode, req.AutoStart, req.Attachments,
		startTaskOptions{ProfileExplicit: req.ProfileExplicit, SpawnOrigin: req.SpawnOrigin},
	)
	if err != nil {
		return nil, err
	}
	if execution == nil {
		// The automatic terminal-PR gate intentionally skips session creation.
		// Return a successful no-op response so session.ensure and WS callers do
		// not dereference a nil execution while the task-owned error card remains
		// the recovery surface.
		return &LaunchSessionResponse{
			Success: true,
			TaskID:  req.TaskID,
		}, nil
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
		req.Prompt, req.SkipMessageRecord, req.PlanMode, req.AutoStart, req.Attachments, nil,
	)
	if err != nil {
		return nil, err
	}
	return executionToLaunchResponse(req.TaskID, execution), nil
}

// launchResume resumes a stopped session.
func (s *Service) launchResume(ctx context.Context, req *LaunchSessionRequest) (*LaunchSessionResponse, error) {
	execution, err := s.ResumeTaskSessionWithOptions(ctx, req.TaskID, req.SessionID, executor.ResumeOptions{
		AllowBranchReplacement:      req.AllowBranchReplacement,
		AllowCompletedSessionResume: req.AllowCompletedSessionResume,
		ForceContextContinuation:    req.ForceContextContinuation,
		ContinuationPrompt:          req.ContinuationPrompt,
		DeferInitialPrompt:          req.DeferRecoveryResolution,
		RecoveryAction:              req.RecoveryAction,
		StartAgentSynchronously:     req.DeferRecoveryResolution,
	})
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
	if err := s.ensureTaskNotArchived(ctx, req.TaskID); err != nil {
		return nil, err
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
// action is "resume" (retry with existing ACP session), "resume_new_branch"
// (retry after replacing a confirmed missing branch), "continue_from_history"
// (start a new native session with a bounded context snapshot), or
// "fresh_start" (clear token, start fresh).
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
	// Office owns autonomous run admission. An operator recovery action must
	// settle the canonical block and let the Office scheduler perform the next
	// launch, so budget, provenance, approval, checkout, and agent-status gates
	// remain authoritative. Calling ResumeTaskSessionWithOptions here would
	// bypass those gates and would also create a direct chat-style launch for a
	// run whose identity belongs to the scheduler.
	if action == "resume" || action == sessionRecoveryActionContinueFromHistory || action == "fresh_start" {
		isOfficeTask, officeErr := s.lookupOfficeTask(ctx, taskID)
		if officeErr != nil {
			return nil, fmt.Errorf("failed to determine office task status: %w", officeErr)
		}
		if isOfficeTask {
			return s.recoverOfficeSessionThroughScheduler(ctx, taskID, sessionID, action)
		}
	}
	resumeOptions := executor.ResumeOptions{}
	var checkpoint *continuationCheckpoint
	var continuation executor.ContextContinuation
	var err error
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
	case sessionRecoveryActionContinueFromHistory:
		continuation, err = s.buildContextContinuationPrompt(ctx, taskID, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to build context continuation: %w", err)
		}
		checkpoint, err = s.persistContinuationCheckpoint(ctx, taskID, sessionID, continuation)
		if err != nil {
			return nil, fmt.Errorf("failed to persist context continuation: %w", err)
		}
		resumeOptions = executor.ResumeOptions{
			ForceContextContinuation: true,
			ContinuationPrompt:       continuation.Prompt,
			DeferInitialPrompt:       true,
			RecoveryAction:           action,
			StartAgentSynchronously:  true,
		}
	default:
		return nil, fmt.Errorf("invalid recovery action: %s", action)
	}

	resp, err := s.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:                      taskID,
		SessionID:                   sessionID,
		Intent:                      IntentResume,
		AllowBranchReplacement:      action == "resume_new_branch",
		AllowCompletedSessionResume: action == "resume",
		ForceContextContinuation:    resumeOptions.ForceContextContinuation,
		ContinuationPrompt:          resumeOptions.ContinuationPrompt,
		RecoveryAction:              action,
		DeferRecoveryResolution:     checkpoint != nil,
	})
	if err != nil {
		return s.handleContinuationLaunchError(ctx, checkpoint, err)
	}
	if checkpoint == nil {
		return resp, nil
	}
	checkpoint.candidateExecutionID = resp.AgentExecutionID
	if err := s.commitContinuationGeneration(ctx, checkpoint); err != nil {
		rollbackErr := s.rollbackContinuationCandidate(ctx, checkpoint)
		settleErr := s.settleContinuationCheckpoint(ctx, checkpoint, models.ContinuitySnapshotUncertain)
		if rollbackErr != nil {
			err = errors.Join(err, rollbackErr)
		}
		if settleErr != nil {
			err = errors.Join(err, settleErr)
		}
		return nil, normalizeRecoverSessionError(err)
	}
	if _, err := s.promptTask(
		context.WithoutCancel(ctx), taskID, sessionID, continuation.Prompt, "", false, nil, true,
		promptTaskOptions{
			recoveryAction:             action,
			preservePromptContext:      true,
			deliverySubmissionID:       checkpoint.submissionID,
			expectedDeliveryGeneration: checkpoint.expectedGen + 1,
		},
	); err != nil {
		settleErr := s.settleContinuationCheckpoint(ctx, checkpoint, models.ContinuitySnapshotUncertain)
		if settleErr != nil {
			err = errors.Join(err, settleErr)
		}
		return nil, normalizeRecoverSessionError(err)
	}
	if err := s.settleContinuationCheckpoint(ctx, checkpoint, models.ContinuitySnapshotConsumed); err != nil {
		return nil, normalizeRecoverSessionError(err)
	}
	if err := s.resolveSessionRecoveryBlock(ctx, sessionID, action); err != nil {
		return nil, normalizeRecoverSessionError(err)
	}
	return resp, nil
}

func (s *Service) handleContinuationLaunchError(
	ctx context.Context,
	checkpoint *continuationCheckpoint,
	err error,
) (*LaunchSessionResponse, error) {
	if checkpoint == nil {
		return nil, normalizeRecoverSessionError(err)
	}
	settleErr := s.finishContinuationCheckpoint(ctx, checkpoint, models.ContinuitySnapshotUncertain)
	if settleErr != nil {
		err = errors.Join(err, settleErr)
	}
	return nil, normalizeRecoverSessionError(err)
}

// recoverOfficeSessionThroughScheduler resolves an operator-owned recovery
// block without launching the session in the chat/orchestrator path. The
// resolved block is observed by Office's next scheduler tick, which releases
// the original run and repeats its normal admission gates before calling the
// Office task starter. Missing blocks remain an error: a scheduler-owned
// session must never be turned into an untracked manual launch.
func (s *Service) recoverOfficeSessionThroughScheduler(
	ctx context.Context,
	taskID, sessionID, action string,
) (*LaunchSessionResponse, error) {
	// Validate the task/session binding before mutating the recovery block. A
	// scheduler handoff must never authorize a block and then fail while
	// loading the session that owns the run.
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load Office session for recovery authorization: %w", err)
	}
	if session == nil || session.TaskID != taskID {
		return nil, ErrTaskSessionPairMismatch
	}
	if err := s.checkSessionRecoveryBlock(ctx, sessionID); err == nil {
		return nil, errOfficeTaskResumeRequiresScheduler
	} else {
		var required *sessionRecoveryRequiredError
		if !errors.As(err, &required) {
			return nil, err
		}
	}
	if action == "fresh_start" {
		if err := s.clearResumeToken(ctx, sessionID); err != nil {
			return nil, fmt.Errorf("failed to clear resume token for Office fresh start: %w", err)
		}
	}
	if err := s.resolveSessionRecoveryBlock(ctx, sessionID, action); err != nil {
		return nil, normalizeRecoverSessionError(err)
	}
	return &LaunchSessionResponse{
		Success:   true,
		TaskID:    taskID,
		SessionID: sessionID,
		State:     string(session.State),
	}, nil
}

// buildContextContinuationPrompt composes canonical Kandev data for the
// explicit recovery path that starts a new native conversation. Native resume
// remains the default; this prompt is only created after the operator selects
// continue_from_history.
func (s *Service) buildContextContinuationPrompt(ctx context.Context, taskID, sessionID string) (executor.ContextContinuation, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return executor.ContextContinuation{}, fmt.Errorf("load task: %w", err)
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return executor.ContextContinuation{}, fmt.Errorf("load session: %w", err)
	}
	if session == nil || session.TaskID != taskID {
		return executor.ContextContinuation{}, ErrTaskSessionPairMismatch
	}
	messages, err := s.repo.ListMessages(ctx, sessionID)
	if err != nil {
		return executor.ContextContinuation{}, fmt.Errorf("load session history: %w", err)
	}
	plan, err := s.repo.GetTaskPlan(ctx, taskID)
	if err != nil {
		return executor.ContextContinuation{}, fmt.Errorf("load task plan: %w", err)
	}
	planText := ""
	if plan != nil {
		planText = strings.TrimSpace(strings.Join([]string{plan.Title, plan.Content}, "\n"))
	}
	return executor.BuildContextContinuation(
		task.Description,
		planText,
		session.WorkspacePath,
		messages,
	), nil
}

func (s *Service) persistContinuationCheckpoint(
	ctx context.Context,
	taskID, sessionID string,
	continuation executor.ContextContinuation,
) (*continuationCheckpoint, error) {
	store, ok := s.repo.(sessionContinuityStore)
	if !ok {
		return nil, errors.New("continuation checkpoint store is unavailable")
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session for checkpoint: %w", err)
	}
	if session == nil || session.TaskID != taskID {
		return nil, ErrTaskSessionPairMismatch
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	var expectedGeneration int64
	current, err := store.GetCurrentHarnessSessionGeneration(ctx, session.ID, incarnationID)
	if err == nil && current != nil {
		expectedGeneration = current.Generation
	} else if err != nil && !errors.Is(err, models.ErrTaskSessionNotFound) {
		return nil, fmt.Errorf("load current harness generation: %w", err)
	}
	now := time.Now().UTC()
	attemptID := uuid.NewString()
	if err := store.CreateRestoreAttempt(ctx, &models.RestoreAttempt{
		ID:                 attemptID,
		SessionID:          session.ID,
		IncarnationID:      incarnationID,
		ExpectedGeneration: expectedGeneration,
		Action:             sessionRecoveryActionContinueFromHistory,
		Outcome:            "blocked",
		Reason:             "native_state_missing",
		TargetWorkspace:    session.WorkspacePath,
		Authorized:         true,
		CreatedAt:          now,
	}); err != nil {
		return nil, err
	}
	snapshotID := uuid.NewString()
	submissionID := normalizeDeliverySubmissionID(uuid.NewString())
	if err := store.CreateContinuationSnapshot(ctx, &models.ContinuationSnapshot{
		ID:               snapshotID,
		AttemptID:        attemptID,
		SessionID:        session.ID,
		TargetGeneration: expectedGeneration + 1,
		SubmissionID:     submissionID,
		SourceMessageID:  continuation.SourceMessageID,
		Content:          continuation.Prompt,
		ByteCount:        continuation.ByteCount,
		OmittedMessages:  continuation.OmittedMessages,
		Truncated:        continuation.Truncated,
		ContentHash:      continuation.ContentHash,
		Status:           models.ContinuitySnapshotPrepared,
		CreatedAt:        now,
	}); err != nil {
		return nil, err
	}
	if continuation.Truncated {
		agentruntime.RecordRestoreContextTruncated(session.AgentProfileID)
	}
	return &continuationCheckpoint{
		store:                store,
		attemptID:            attemptID,
		snapshotID:           snapshotID,
		submissionID:         submissionID,
		sessionID:            session.ID,
		incarnationID:        incarnationID,
		expectedGen:          expectedGeneration,
		workspace:            session.WorkspacePath,
		previousState:        session.State,
		previousErrorMessage: session.ErrorMessage,
	}, nil
}

// rollbackContinuationCandidate releases the promptless replacement created
// while preparing an explicit context continuation. The candidate execution
// is not yet a committed harness generation, so a failed generation CAS must
// not leave it live or overwrite a concurrent successor's state.
func (s *Service) rollbackContinuationCandidate(ctx context.Context, checkpoint *continuationCheckpoint) error {
	if checkpoint == nil || checkpoint.candidateExecutionID == "" {
		return nil
	}
	running, err := s.loadContinuationCandidate(ctx, checkpoint)
	if err != nil {
		return err
	}
	if running == nil || running.AgentExecutionID != checkpoint.candidateExecutionID {
		// A different execution already owns the session. Never clean it up or
		// roll its state back while settling a stale continuation candidate.
		return nil
	}
	if err := s.cleanupContinuationCandidate(ctx, checkpoint, running); err != nil {
		return err
	}
	return s.restoreContinuationSessionState(ctx, checkpoint)
}

func (s *Service) loadContinuationCandidate(ctx context.Context, checkpoint *continuationCheckpoint) (*models.ExecutorRunning, error) {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, checkpoint.sessionID)
	if err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, fmt.Errorf("load continuation candidate execution: %w", err)
	}
	return running, nil
}

func (s *Service) cleanupContinuationCandidate(ctx context.Context, checkpoint *continuationCheckpoint, running *models.ExecutorRunning) error {
	if cleaner, ok := s.agentManager.(executionIdentityCleaner); ok {
		if err := cleaner.CleanupStaleExecutionBySessionIDIfCurrent(
			ctx,
			checkpoint.sessionID,
			checkpoint.candidateExecutionID,
			running.UpdatedAt,
		); err != nil {
			return fmt.Errorf("cleanup continuation candidate: %w", err)
		}
	} else if err := s.agentManager.CleanupStaleExecutionBySessionID(ctx, checkpoint.sessionID); err != nil {
		return fmt.Errorf("cleanup continuation candidate: %w", err)
	}
	return nil
}

func (s *Service) restoreContinuationSessionState(ctx context.Context, checkpoint *continuationCheckpoint) error {
	// Cleanup is identity-fenced above. Re-read before restoring state so a
	// successor that won the session after cleanup is never overwritten.
	currentRunning, runningErr := s.repo.GetExecutorRunningBySessionID(ctx, checkpoint.sessionID)
	if runningErr == nil && currentRunning != nil && currentRunning.AgentExecutionID != checkpoint.candidateExecutionID {
		return nil
	}
	if runningErr != nil && !errors.Is(runningErr, models.ErrExecutorRunningNotFound) {
		return fmt.Errorf("re-read continuation execution after cleanup: %w", runningErr)
	}
	currentSession, err := s.repo.GetTaskSession(ctx, checkpoint.sessionID)
	if err != nil {
		return fmt.Errorf("load session after continuation cleanup: %w", err)
	}
	if currentSession == nil || isTerminalSessionState(currentSession.State) {
		return nil
	}
	if currentSession.State == checkpoint.previousState {
		return nil
	}
	changed, _, err := s.repo.UpdateTaskSessionStateIfCurrent(
		ctx,
		checkpoint.sessionID,
		currentSession.State,
		checkpoint.previousState,
		checkpoint.previousErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("restore session state after continuation cleanup: %w", err)
	}
	if !changed {
		return nil
	}
	return nil
}

func (s *Service) commitContinuationGeneration(ctx context.Context, checkpoint *continuationCheckpoint) error {
	if checkpoint == nil || checkpoint.store == nil {
		return nil
	}
	now := time.Now().UTC()
	acpSessionID := s.currentACPSessionID(checkpoint.sessionID)
	if acpSessionID == "" {
		return errors.New("continuation candidate has no native session identity")
	}
	committed, err := checkpoint.store.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID:             checkpoint.sessionID,
		IncarnationID:         checkpoint.incarnationID,
		Generation:            checkpoint.expectedGen + 1,
		PredecessorGeneration: checkpoint.expectedGen,
		NativeSessionID:       acpSessionID,
		OriginalWorkspace:     checkpoint.workspace,
		CurrentWorkspace:      checkpoint.workspace,
		CreationReason:        "context_continued",
		CreatedAt:             now,
		CommittedAt:           now,
	}, checkpoint.expectedGen)
	if err == nil && !committed {
		// The ACP session-created event may have filled the absent initial row
		// before this continuation checkpoint reaches its CAS. Treat that exact
		// native identity as success, but never accept an unrelated generation.
		current, currentErr := checkpoint.store.GetCurrentHarnessSessionGeneration(
			ctx, checkpoint.sessionID, checkpoint.incarnationID,
		)
		if currentErr == nil && current != nil &&
			current.Generation == checkpoint.expectedGen+1 && current.NativeSessionID == acpSessionID {
			committed = true
		}
	}
	if err != nil || !committed {
		if err != nil {
			return fmt.Errorf("commit continuation generation: %w", err)
		}
		return errors.New("commit continuation generation: stale generation")
	}
	checkpoint.nativeID = acpSessionID
	return nil
}

func (s *Service) finishContinuationCheckpoint(ctx context.Context, checkpoint *continuationCheckpoint, status string) error {
	if checkpoint == nil || checkpoint.store == nil {
		return nil
	}
	now := time.Now().UTC()
	var resultErr error
	if err := checkpoint.store.CompleteContinuationSnapshot(ctx, checkpoint.snapshotID, status, now); err != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("settle continuation snapshot: %w", err))
	}
	attemptOutcome := "blocked"
	metricOutcome := "blocked"
	if status == models.ContinuitySnapshotConsumed {
		attemptOutcome = "context_continued"
		metricOutcome = "context_continued"
	}
	if err := checkpoint.store.CompleteRestoreAttempt(ctx, checkpoint.attemptID, attemptOutcome, now); err != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("settle restore attempt: %w", err))
	}
	if resultErr == nil {
		agentruntime.RecordRestoreAttempt(metricOutcome, "native_state_missing", "")
	}
	return resultErr
}

// settleContinuationCheckpoint never reports a consumed snapshot after a
// persistence failure. If the success settlement is ambiguous, the
// conservative outcome is uncertain, which keeps the recovery block closed
// and prevents a retry from replaying the same context automatically.
func (s *Service) settleContinuationCheckpoint(
	ctx context.Context,
	checkpoint *continuationCheckpoint,
	status string,
) error {
	err := s.finishContinuationCheckpoint(ctx, checkpoint, status)
	if err == nil || status != models.ContinuitySnapshotConsumed {
		return err
	}
	uncertainErr := s.finishContinuationCheckpoint(ctx, checkpoint, models.ContinuitySnapshotUncertain)
	if uncertainErr != nil {
		return errors.Join(err, uncertainErr)
	}
	return err
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
