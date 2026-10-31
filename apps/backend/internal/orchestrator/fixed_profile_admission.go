package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// FixedProfileSuspendedError refuses a launch or prompt for a fixed (concrete)
// execution profile whose own resource circuit is open. The profile keeps its
// configured model: the refusal never selects or implies another one.
type FixedProfileSuspendedError struct {
	ExecutionProfileID string
	Suspension         dynamicruntime.Suspension
}

func (e *FixedProfileSuspendedError) Error() string {
	return fmt.Sprintf("fixed profile %s suspended: %s", e.ExecutionProfileID, fixedProfileSuspensionMessage(e.Suspension))
}

// DispatchDeferred keeps an admitted automation run open before provider I/O.
func (e *FixedProfileSuspendedError) DispatchDeferred() string { return "fixed_profile_suspended" }

func isFixedProfileSuspendedError(err error) bool {
	var suspended *FixedProfileSuspendedError
	return errors.As(err, &suspended)
}

// fixedProfileSuspensionMessage names the suspended scope, the reason code and
// when the resource may be tried again. A hold has no end instant: only the
// owner release lifts it.
func fixedProfileSuspensionMessage(suspension dynamicruntime.Suspension) string {
	if suspension.Until.IsZero() {
		return fmt.Sprintf("resource suspended: scope=%s code=%s (held until owner release)",
			suspension.Scope, suspension.Code)
	}
	return fmt.Sprintf("resource suspended: scope=%s code=%s reset=%s",
		suspension.Scope, suspension.Code, suspension.Until.UTC().Format(time.RFC3339))
}

// fixedSessionProfileID names the concrete profile a fixed session runs. A
// fixed session usually leaves ExecutionProfileID empty, so its agent profile
// is the profile, the same fallback the executor uses to launch it.
func fixedSessionProfileID(session *models.TaskSession) string {
	if session == nil {
		return ""
	}
	if session.ExecutionProfileID != "" {
		return session.ExecutionProfileID
	}
	return session.AgentProfileID
}

// fixedLaunchProfileID names the profile a concrete launch will run: the
// launch option when given, otherwise the session's profile, as the executor
// resolves it.
func (s *Service) fixedLaunchProfileID(ctx context.Context, sessionID, optionProfileID string) string {
	if optionProfileID != "" || s.profileExecutionResolver == nil || s.repo == nil || sessionID == "" {
		return optionProfileID
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return ""
	}
	return fixedSessionProfileID(session)
}

// admitFixedProfileDispatch refuses a new execution or a new prompt for a fixed
// profile while that profile's resource circuit is open. Dynamic sessions are
// gated by selection and never reach this check.
//
// An unknown verdict means the profile itself cannot be read, and the launch or
// prompt resolves the same profile, so admitting it here cannot send to a
// resource known to be suspended.
//
// The refusal leaves the session waiting for input with the suspension named
// in its error message; it never stops a running turn or replays past output.
func (s *Service) admitFixedProfileDispatch(ctx context.Context, sessionID, executionProfileID string) error {
	if s.profileExecutionResolver == nil || executionProfileID == "" {
		return nil
	}
	suspension, known := s.profileExecutionResolver.FixedProfileSuspension(ctx, executionProfileID, time.Now())
	return s.refuseFixedProfileDispatch(ctx, sessionID, executionProfileID, suspension, known)
}

func (s *Service) refuseFixedProfileDispatch(ctx context.Context, sessionID, executionProfileID string, suspension dynamicruntime.Suspension, known bool) error {
	if !known || !suspension.Blocked() {
		return nil
	}
	refusal := &FixedProfileSuspendedError{ExecutionProfileID: executionProfileID, Suspension: suspension}
	s.surfaceFixedProfileSuspension(ctx, sessionID, suspension)
	return refusal
}

func (s *Service) fixedProfileSuspensionForSession(ctx context.Context, session *models.TaskSession) (dynamicruntime.Suspension, bool) {
	if session == nil || s.profileExecutionResolver == nil || s.isDynamicPromptSession(session) {
		return dynamicruntime.Suspension{}, false
	}
	profileID := fixedSessionProfileID(session)
	if session.AgentExecutionID != "" {
		running, err := s.repo.GetExecutorRunningBySessionID(ctx, session.ID)
		if err != nil || running == nil || running.AgentExecutionID != session.AgentExecutionID {
			return dynamicruntime.Suspension{}, false
		}
		key := agentruntime.CapturedResourceCircuitKey(running.Metadata, running.AgentExecutionID, profileID)
		return s.profileExecutionResolver.CapturedFixedProfileSuspension(profileID, key, time.Now())
	}
	return s.profileExecutionResolver.FixedProfileSuspension(ctx, profileID, time.Now())
}

func (s *Service) admitFixedSessionPrompt(ctx context.Context, session *models.TaskSession) error {
	suspension, known := s.fixedProfileSuspensionForSession(ctx, session)
	return s.refuseFixedProfileDispatch(ctx, session.ID, fixedSessionProfileID(session), suspension, known)
}

// surfaceFixedProfileSuspension records the refusal on the session so the
// existing session state transport shows why nothing was sent. Failing to
// persist it does not admit the dispatch.
func (s *Service) surfaceFixedProfileSuspension(
	ctx context.Context,
	sessionID string,
	suspension dynamicruntime.Suspension,
) {
	if s.repo == nil || sessionID == "" {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		s.logger.Warn("failed to load suspended fixed-profile session",
			zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	previous := session.State
	// Only a session that has not started a turn moves to waiting; a running
	// turn keeps its state and its output.
	if previous == models.TaskSessionStateCreated || previous == models.TaskSessionStateStarting {
		session.State = models.TaskSessionStateWaitingForInput
	}
	session.ErrorMessage = fixedProfileSuspensionMessage(suspension)
	if err := s.repo.UpdateTaskSession(ctx, session); err != nil {
		s.logger.Warn("failed to persist suspended fixed-profile session state",
			zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	s.publishTaskSessionStateChanged(ctx, session.TaskID, session.ID, previous, session.State,
		session.ErrorMessage, &session.UpdatedAt, session)
	s.markFixedProfileWait(ctx, session)
}

// recordFixedProfileFailure records the resource circuit of a fixed profile
// whose attempt failed with a classified provider error. Only the profile the
// failed execution actually ran is charged; a failure whose execution cannot be
// tied to the session is ignored. The engine applies its own circuit policy:
// account blocks are held until owner release, other qualifying codes open the
// existing credential or model circuit, and anything else records nothing.
func (s *Service) recordFixedProfileFailure(
	ctx context.Context,
	data watcher.AgentEventData,
	classified *routingerr.Error,
) {
	if classified == nil || s.profileExecutionResolver == nil || s.repo == nil || data.SessionID == "" {
		return
	}
	if !routingerr.HoldsUntilRelease(classified.Code) && classified.Confidence == routingerr.ConfLow {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil || s.isDynamicPromptSession(session) {
		return
	}
	profileID := fixedProfileFailureOwner(data, session)
	if profileID == "" {
		return
	}
	s.profileExecutionResolver.RecordFixedProfileFailure(ctx, profileID, data.ResourceCircuitKey, classified)
	s.logger.Info("recorded fixed profile resource failure",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID),
		zap.String("execution_profile_id", profileID),
		zap.String("routing_code", string(classified.Code)))
}

func fixedProfileFailureOwner(data watcher.AgentEventData, session *models.TaskSession) string {
	if data.AgentExecutionID == "" || session.AgentExecutionID != data.AgentExecutionID || session.TaskID != data.TaskID {
		return ""
	}
	profileID := fixedSessionProfileID(session)
	if profileID == "" || (data.ExecutionProfileID != "" && data.ExecutionProfileID != profileID) ||
		(data.AgentProfileID != "" && data.AgentProfileID != session.AgentProfileID) {
		return ""
	}
	return profileID
}
