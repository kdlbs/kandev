package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// PeerMessageStartAdmission reserves one session incarnation across workflow
// preparation and its first peer-message launch. Release must be called when
// the handler no longer owns the launch path.
type PeerMessageStartAdmission interface {
	SessionIdentity() messagequeue.QueueSessionIdentity
	ProcessOnTurnStart(ctx context.Context, taskID, sessionID string) (ProcessOnTurnStartResult, error)
	StartCreatedSession(
		ctx context.Context,
		agentProfileID, prompt string,
		skipMessageRecord, planMode, autoStart bool,
		attachments []v1.MessageAttachment,
		references []v1.EntityReference,
	) (*executor.TaskExecution, error)
	IsActive() bool
	Release()
}

type peerMessageStartAdmission struct {
	service         *Service
	identity        messagequeue.QueueSessionIdentity
	lifecycleUnlock func()
	cancelGuard     *sync.Mutex
	releaseGuardRef func()

	releaseGuardOnce sync.Once
	releaseOnce      sync.Once
	stateMu          sync.Mutex
	released         bool
}

func (a *peerMessageStartAdmission) SessionIdentity() messagequeue.QueueSessionIdentity {
	if a == nil {
		return messagequeue.QueueSessionIdentity{}
	}
	return a.identity
}

func (a *peerMessageStartAdmission) ProcessOnTurnStart(
	ctx context.Context,
	taskID, sessionID string,
) (ProcessOnTurnStartResult, error) {
	if a == nil || a.identity.TaskID != taskID || a.identity.SessionID != sessionID {
		return ProcessOnTurnStartResult{}, messagequeue.ErrSessionIdentityMismatch
	}
	if err := a.validateContext(); err != nil {
		return ProcessOnTurnStartResult{}, err
	}
	defer a.releaseCancelGuard()
	return a.service.processOnTurnStartAdmissionWithGuard(ctx, taskID, sessionID, false, a.cancelGuard, false)
}

func (a *peerMessageStartAdmission) StartCreatedSession(
	ctx context.Context,
	agentProfileID, prompt string,
	skipMessageRecord, planMode, autoStart bool,
	attachments []v1.MessageAttachment,
	references []v1.EntityReference,
) (*executor.TaskExecution, error) {
	if a == nil {
		return nil, executor.ErrExecutionAlreadyRunning
	}
	a.releaseCancelGuard()
	if err := a.validateContext(); err != nil {
		return nil, err
	}
	session, err := a.service.repo.GetTaskSession(ctx, a.identity.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil || session.TaskID != a.identity.TaskID || session.QueueIncarnationID != a.identity.SessionIncarnationID {
		return nil, messagequeue.ErrSessionIdentityMismatch
	}
	if session.State == models.TaskSessionStateStarting || session.State == models.TaskSessionStateRunning {
		return nil, executor.ErrExecutionAlreadyRunning
	}
	active, err := a.service.hasActiveExecutionForPeerMessage(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("check existing session execution: %w", err)
	}
	if active {
		return nil, executor.ErrExecutionAlreadyRunning
	}
	return a.service.startCreatedSession(
		ctx, a.identity.TaskID, a.identity.SessionID, agentProfileID, prompt,
		skipMessageRecord, planMode, autoStart, attachments, references, "",
		startCreatedSessionOptions{lifecycleLockHeld: true, refuseIfAgentRunning: true},
	)
}

func (a *peerMessageStartAdmission) validateContext() error {
	if a == nil || a.service == nil {
		return executor.ErrExecutionAlreadyRunning
	}
	if a.service.currentCancellation(a.identity.SessionID) != nil {
		return executor.ErrExecutionAlreadyRunning
	}
	return nil
}

func (a *peerMessageStartAdmission) releaseCancelGuard() {
	if a == nil {
		return
	}
	a.releaseGuardOnce.Do(func() {
		if a.cancelGuard != nil {
			a.cancelGuard.Unlock()
		}
		if a.releaseGuardRef != nil {
			a.releaseGuardRef()
		}
	})
}

func (a *peerMessageStartAdmission) Release() {
	if a == nil {
		return
	}
	a.releaseOnce.Do(func() {
		a.releaseCancelGuard()
		if a.lifecycleUnlock != nil {
			a.lifecycleUnlock()
		}
		a.stateMu.Lock()
		a.released = true
		a.stateMu.Unlock()
	})
}

func (a *peerMessageStartAdmission) IsActive() bool {
	if a == nil {
		return false
	}
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	return !a.released
}

// BeginPeerMessageStart acquires lifecycle ownership before a peer message can
// run on_turn_start. The cancel guard is only acquired non-blockingly because
// other established paths still enter lifecycle admission while holding it.
func (s *Service) BeginPeerMessageStart(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
) (PeerMessageStartAdmission, error) {
	if identity.TaskID == "" || identity.SessionID == "" || identity.SessionIncarnationID == "" {
		return nil, messagequeue.ErrSessionIdentityMismatch
	}
	lifecycleUnlock, acquired := s.tryAcquireSessionLifecycleLock(identity.SessionID)
	if !acquired {
		return nil, executor.ErrExecutionAlreadyRunning
	}
	guard, releaseGuardRef := s.acquireCancelInFlightGuard(identity.SessionID)
	if !guard.TryLock() {
		releaseGuardRef()
		lifecycleUnlock()
		return nil, executor.ErrExecutionAlreadyRunning
	}
	admission := &peerMessageStartAdmission{
		service:         s,
		identity:        identity,
		lifecycleUnlock: lifecycleUnlock,
		cancelGuard:     guard,
		releaseGuardRef: releaseGuardRef,
	}
	if s.currentCancellation(identity.SessionID) != nil {
		admission.Release()
		return nil, executor.ErrExecutionAlreadyRunning
	}
	session, err := s.repo.GetTaskSession(ctx, identity.SessionID)
	if err != nil {
		admission.Release()
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil || session.TaskID != identity.TaskID || session.QueueIncarnationID != identity.SessionIncarnationID {
		admission.Release()
		return nil, messagequeue.ErrSessionIdentityMismatch
	}
	if session.State == models.TaskSessionStateStarting || session.State == models.TaskSessionStateRunning {
		admission.Release()
		return nil, executor.ErrExecutionAlreadyRunning
	}
	active, err := s.hasActiveExecutionForPeerMessage(ctx, identity.SessionID)
	if err != nil {
		admission.Release()
		return nil, fmt.Errorf("check existing session execution: %w", err)
	}
	if active {
		admission.Release()
		return nil, executor.ErrExecutionAlreadyRunning
	}
	return admission, nil
}

// StartCreatedSessionForPeerMessage admits a peer message only when it wins
// ownership of the session's initial launch. A concurrent launch keeps the
// lifecycle lock, so the losing message can be queued without waiting for the
// winner to finish bootstrapping.
func (s *Service) StartCreatedSessionForPeerMessage(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	agentProfileID, prompt string,
	skipMessageRecord, planMode, autoStart bool,
	attachments []v1.MessageAttachment,
	references []v1.EntityReference,
) (*executor.TaskExecution, error) {
	admission, err := s.BeginPeerMessageStart(ctx, identity)
	if err != nil {
		return nil, err
	}
	defer admission.Release()
	return admission.StartCreatedSession(
		ctx, agentProfileID, prompt, skipMessageRecord, planMode, autoStart, attachments, references,
	)
}

func (s *Service) hasActiveExecutionForPeerMessage(ctx context.Context, sessionID string) (bool, error) {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return false, err
	}
	if running != nil {
		switch running.Status {
		case models.ExecutorRunningStatusRunning:
			return true, nil
		case models.ExecutorRunningStatusStarting:
			// Workspace-only preparation uses starting until it publishes the
			// prepared status. The process probe below distinguishes it from an
			// agent startup that does not hold this service's lifecycle lock.
		case models.ExecutorRunningStatusFailed,
			models.ExecutorRunningStatusStopped,
			models.ExecutorRunningStatusComplete,
			models.ExecutorRunningStatusPrepared,
			models.ExecutorRunningStatusReady:
			// Failed, stopped, completed, and prepared workspaces do not by
			// themselves own an active agent process. Ready is the workspace
			// readiness state; the process probe below identifies live agents.
		default:
			// Unknown durable runtime state is not safe evidence for a new start.
			return true, nil
		}
	}
	return s.agentManager != nil && s.agentManager.IsAgentRunningForSession(ctx, sessionID), nil
}
