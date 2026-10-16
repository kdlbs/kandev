package backendapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/orchestrator"
)

// SetExecutionCapacityOwner is installed before Office schedulers/events start.
func (l *officeRunSessionLauncher) SetExecutionCapacityOwner(owner *orchestrator.Service) {
	l.capacity = owner
	if owner != nil {
		owner.SetRunSessionCapacitySource(l.repo)
		l.repo.SetRunSessionCapacityReleased(owner.NotifyRunSessionCapacityReleased)
	}
}

type officeCapacityAttempt struct {
	launcher    *officeRunSessionLauncher
	session     *officemodels.RunSession
	reservation *orchestrator.RunSessionCapacityReservation
	persisted   bool
}

func (l *officeRunSessionLauncher) reserveOfficeCapacity(ctx context.Context, session *officemodels.RunSession) (*officeCapacityAttempt, error) {
	attempt := &officeCapacityAttempt{launcher: l, session: session}
	if l.capacity == nil {
		return attempt, nil
	}
	grade := ""
	if l.capacity.RunSessionCapacityEnabled() {
		if l.resolver == nil || l.recovery == nil {
			return nil, l.deferOfficeCapacity(ctx, session.RunID, "run grade or owned runtime proof unavailable")
		}
		var err error
		grade, err = l.resolver.LogicalExecutionGrade(ctx, session.AgentProfileID)
		if err != nil {
			return nil, l.deferOfficeCapacity(ctx, session.RunID, "logical execution grade unknown")
		}
	}
	reservation, reason, err := l.capacity.ReserveRunSessionCapacity(ctx, session.ID, grade)
	if err != nil || reason != "" {
		return nil, errors.Join(l.deferOfficeCapacity(ctx, session.RunID, reason), err)
	}
	attempt.reservation = reservation
	if reservation != nil {
		session.AdmittedExecutionGrade = reservation.Grade
		session.AdmittedExecutionGradeKnown = true
	}
	return attempt, nil
}

func (l *officeRunSessionLauncher) deferOfficeCapacity(ctx context.Context, runID, reason string) error {
	_, err := l.repo.RequeueUnstartedRunForCapacity(context.WithoutCancel(ctx), runID)
	return errors.Join(fmt.Errorf("%w: %s", officeservice.ErrRunSessionAdmissionDeferred, reason), err)
}

func (l *officeRunSessionLauncher) existingOfficeCapacityAttempt(ctx context.Context, sessions []officemodels.RunSession) (officeservice.RunSessionLaunch, bool, error) {
	for _, session := range sessions {
		live := session.State == officemodels.RunSessionStatePreparing || session.State == officemodels.RunSessionStateRunning
		if !live || l.capacity == nil || (!session.AdmittedExecutionGradeKnown && !l.capacity.RunSessionCapacityEnabled()) {
			continue
		}
		if session.State != officemodels.RunSessionStateRunning || session.ExecutionID == "" {
			return officeservice.RunSessionLaunch{}, true, fmt.Errorf("%w: predecessor registration pending", officeservice.ErrRunSessionAdmissionDeferred)
		}
		execution, err := l.runtime.GetExecution(ctx, session.ExecutionID)
		if err != nil || execution == nil || !sameRunOwner(execution.Owner, session) || !isLiveExecution(execution.Status) {
			return officeservice.RunSessionLaunch{}, true, errors.Join(fmt.Errorf("%w: predecessor proof pending", officeservice.ErrRunSessionAdmissionDeferred), err)
		}
		return officeservice.RunSessionLaunch{SessionID: session.ID, ExecutionID: session.ExecutionID, ExecutionProfileID: session.ExecutionProfileID, Adapter: session.Adapter, Model: session.Model, ACPSessionID: session.ACPSessionID}, true, nil
	}
	return officeservice.RunSessionLaunch{}, false, nil
}

func (a *officeCapacityAttempt) tracked() bool { return a != nil && a.reservation != nil }

func (a *officeCapacityAttempt) confirm(ctx context.Context) error {
	a.persisted = true
	if a.tracked() {
		return a.reservation.Confirm(ctx)
	}
	return nil
}

func (a *officeCapacityAttempt) prepareCandidate(ctx context.Context, request *runtimeapi.LaunchRequest) error {
	if !a.tracked() {
		return nil
	}
	candidate := uuid.NewString()
	wrote, err := a.launcher.repo.BindRunSessionCandidate(ctx, a.session.ID, candidate)
	if err != nil {
		return err
	}
	if !wrote {
		return errors.New("run candidate owner no longer preparing")
	}
	a.session.ExecutionID = candidate
	request.CandidateExecutionID = candidate
	request.OnExecutionAllocated = func(ctx context.Context, id string) error {
		if id != candidate {
			return errors.New("run candidate execution identity changed")
		}
		return a.launcher.AdmitExecution(ctx, request.Owner)
	}
	return nil
}

func (a *officeCapacityAttempt) abort(ctx context.Context, cause error) error {
	if !a.tracked() || !a.persisted {
		return nil
	}
	if a.session.ExecutionID != "" {
		// Preserve abort intent before cleanup so restart cannot mistake the
		// uncertain allocation for an execution that is still allowed to start.
		_, cancelErr := a.launcher.repo.RequestRunSessionCancellation(ctx, a.session.ID)
		if err := a.launcher.stopRecoveredRunSession(ctx, *a.session); err != nil {
			return errors.Join(officeservice.ErrRunSessionAdmissionDeferred, fmt.Errorf("retain uncertain run execution: %w", err), cancelErr)
		}
	}
	_, err := a.launcher.repo.FinishRunSession(ctx, a.session.ID, officemodels.RunSessionStateFailed, cause.Error())
	return err
}

func (a *officeCapacityAttempt) release() {
	if a.tracked() {
		a.reservation.Release()
	}
}

func (a *officeCapacityAttempt) finishLegacy(ctx context.Context, id string, state officemodels.RunSessionState, message string) (bool, error) {
	if a.tracked() {
		return false, nil
	}
	return a.launcher.repo.FinishRunSession(ctx, id, state, message)
}

func (a *officeCapacityAttempt) stopLegacy(ctx context.Context, id, reason string) error {
	if a.tracked() {
		return nil
	}
	return a.launcher.runtime.Stop(ctx, id, reason)
}

// A fast exact-owner completion may win the terminal CAS before the launcher's
// started-state write. That already-completed run must not enter launch retry.
func (a *officeCapacityAttempt) completedBeforeStartRecorded(ctx context.Context, owner runtimeapi.ExecutionOwner, executionID string) bool {
	if !a.tracked() {
		return false
	}
	session, err := a.launcher.repo.GetRunSession(ctx, owner.RunSessionID)
	if err != nil || session == nil || session.ExecutionID != executionID {
		return false
	}
	return session.State == officemodels.RunSessionStateFinished && session.CancelRequestedAt == nil && sameRunOwner(owner, *session)
}
