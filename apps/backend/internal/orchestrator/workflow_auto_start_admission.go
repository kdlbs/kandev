package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type workflowAutoStartAdmissionKey struct{}
type workflowAutoStartRetryKey struct{}

type workflowAutoStartAdmission struct {
	identity          messagequeue.QueueSessionIdentity
	cancellationFence promptCancellationFence
}

// withWorkflowAutoStartAdmission keeps retries within the original session
// incarnation and cancellation revision.
func (s *Service) withWorkflowAutoStartAdmission(ctx context.Context, taskID, sessionID string) (context.Context, error) {
	if cancelInFlightGuardHeld(ctx) && !cancelInFlightGuardHeldForSession(ctx, sessionID) &&
		!workflowProfileSwitchGuardIsHeld(ctx, sessionID) {
		ctx = context.WithValue(ctx, cancelInFlightGuardHeldContextKey{}, false)
	}
	_, revision := s.CancellationPendingSnapshot(sessionID)
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return ctx, err
	}
	if session == nil || session.TaskID != taskID {
		return ctx, messagequeue.ErrSessionIdentityMismatch
	}
	identity := messagequeue.QueueSessionIdentity{TaskID: taskID, SessionID: sessionID, SessionIncarnationID: session.QueueIncarnationID}
	if s.messageQueue != nil {
		identity, err = s.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
		if err != nil {
			return ctx, err
		}
	}
	admission := &workflowAutoStartAdmission{identity: identity, cancellationFence: promptCancellationFence{revision: revision}}
	return context.WithValue(ctx, workflowAutoStartAdmissionKey{}, admission), nil
}

func workflowAutoStartAdmissionFromContext(ctx context.Context, taskID, sessionID string) *workflowAutoStartAdmission {
	admission, _ := ctx.Value(workflowAutoStartAdmissionKey{}).(*workflowAutoStartAdmission)
	if admission == nil || admission.identity.TaskID != taskID || admission.identity.SessionID != sessionID {
		return nil
	}
	return admission
}

func (s *Service) validateWorkflowAutoStartAdmission(ctx context.Context, session *models.TaskSession) error {
	admission, _ := ctx.Value(workflowAutoStartAdmissionKey{}).(*workflowAutoStartAdmission)
	if admission == nil {
		return nil
	}
	if session == nil || session.TaskID != admission.identity.TaskID || session.ID != admission.identity.SessionID ||
		session.QueueIncarnationID != admission.identity.SessionIncarnationID {
		return messagequeue.ErrSessionIdentityMismatch
	}
	if err := s.authorizeTaskPrompt(ctx, session.TaskID); err != nil {
		return err
	}
	if err := s.authorizeTaskSessionPair(ctx, session.TaskID, session.ID); err != nil {
		return err
	}
	if err := s.validatePromptAdmissionContext(ctx, session.TaskID, session.ID, &admission.cancellationFence, "workflow auto-start"); err != nil {
		return err
	}
	retry, _ := ctx.Value(workflowAutoStartRetryKey{}).(bool)
	if retry && isTerminalSessionState(session.State) {
		return newWorkflowAutoStartSessionTerminalizedError(session)
	}
	return nil
}

// withWorkflowAutoStartInputAdmission serializes one-shot input claims and queue
// insertion with session settlement. Provider I/O runs after this guard is released.
func (s *Service) withWorkflowAutoStartInputAdmission(
	ctx context.Context, session *models.TaskSession, step *wfmodels.WorkflowStep, requireNonterminal bool, admit func() error,
) error {
	admission, _ := ctx.Value(workflowAutoStartAdmissionKey{}).(*workflowAutoStartAdmission)
	if admission == nil {
		return admit()
	}
	if !workflowProfileSwitchGuardIsHeld(ctx, session.ID) && !cancelInFlightGuardHeldForSession(ctx, session.ID) {
		guard, err := s.lockCancelInFlightGuardWithContext(ctx, session.ID)
		if err != nil {
			return err
		}
		defer guard.release()
	}
	fresh, err := s.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		return err
	}
	if err := s.validateWorkflowAutoStartAdmission(ctx, fresh); err != nil {
		return err
	}
	if requireNonterminal && isTerminalSessionState(fresh.State) {
		return newWorkflowAutoStartSessionTerminalizedError(fresh)
	}
	if err := s.validateWorkflowStepPromptSession(ctx, session.TaskID, fresh, step); err != nil {
		return err
	}
	return admit()
}

// validateWorkflowAutoStartResumeAdmission runs under the resume-registration guard.
func (s *Service) validateWorkflowAutoStartResumeAdmission(ctx context.Context, taskID, sessionID string) error {
	admission, _ := ctx.Value(workflowAutoStartAdmissionKey{}).(*workflowAutoStartAdmission)
	if admission == nil {
		return nil
	}
	if admission.identity.TaskID != taskID || admission.identity.SessionID != sessionID {
		return messagequeue.ErrSessionIdentityMismatch
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.validateWorkflowAutoStartAdmission(ctx, session)
}

func (s *Service) validateWorkflowStepPromptSession(ctx context.Context, taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep) error {
	if session.TaskID != taskID {
		return fmt.Errorf("session does not belong to task")
	}
	effectiveProfile, err := s.resolveStepAgentProfileForTaskID(ctx, taskID, step)
	if err != nil {
		return err
	}
	if effectiveProfile != "" && effectiveProfile != session.AgentProfileID {
		return fmt.Errorf("workflow step profile mismatch: step %q resolves to profile %q but session %q uses profile %q; route the session before prompting", step.ID, effectiveProfile, session.ID, session.AgentProfileID)
	}
	if session.ReviewStatus == models.ReviewStatusPending {
		return fmt.Errorf("session is pending approval - use Approve button to proceed or send a message to request changes")
	}
	return nil
}
