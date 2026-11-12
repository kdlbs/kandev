package orchestrator

import (
	"context"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type admittedGradeWriter interface {
	SetAdmittedExecutionGrade(context.Context, string, string) error
}

func (s *Service) wireAdmissionGrade(resolver *agentruntime.ProfileExecutionResolver) {
	if s.sessionCeiling == nil {
		return
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resolveGrade = nil
	c.persistGrade = nil
	if resolver == nil {
		return
	}
	c.resolveGrade = func(ctx context.Context, req admissionRequest) (string, error) {
		return s.actualExecutionGrade(ctx, req, resolver)
	}
	if writer, ok := s.repo.(admittedGradeWriter); ok {
		c.persistGrade = writer.SetAdmittedExecutionGrade
	}
}

func (s *Service) actualExecutionGrade(ctx context.Context, req admissionRequest, resolver *agentruntime.ProfileExecutionResolver) (string, error) {
	session, err := s.sessionForExecutionGrade(ctx, req)
	if err != nil {
		return "", err
	}
	if session != nil {
		if models.IsAdmittedSessionState(session.State) {
			if !session.AdmittedExecutionGradeKnown {
				return "", errors.New("active execution grade is unknown")
			}
			return session.AdmittedExecutionGrade, nil
		}
		req.taskID = session.TaskID
		if req.profileID == "" {
			req.profileID = session.AgentProfileID
		}
	}
	if req.taskID != "" {
		return s.startTaskExecutionGrade(ctx, req, resolver)
	}
	if req.profileID == "" {
		return "", nil
	}
	return resolver.LogicalExecutionGrade(ctx, req.profileID)
}

func (s *Service) startTaskExecutionGrade(ctx context.Context, req admissionRequest, resolver *agentruntime.ProfileExecutionResolver) (string, error) {
	task, err := s.repo.GetTask(ctx, req.taskID)
	if err != nil {
		return "", err
	}
	if task == nil {
		return "", errors.New("execution grade task unavailable")
	}
	if req.startSelection != nil && !req.profileExplicit {
		if err := s.captureExecutionGradeStartSelection(ctx, task, req); err != nil {
			return "", err
		}
		req.profileID = req.startSelection.profileID
		selected := req.startSelection.session
		if selected != nil && models.IsAdmittedSessionState(selected.State) {
			if !selected.AdmittedExecutionGradeKnown {
				return "", errors.New("active execution grade is unknown")
			}
			return selected.AdmittedExecutionGrade, nil
		}
	}
	if task.ExecutionGrade != "" {
		return task.ExecutionGrade, nil
	}
	if req.profileID == "" {
		return "", nil
	}
	return resolver.LogicalExecutionGrade(ctx, req.profileID)
}

func (s *Service) sessionForExecutionGrade(ctx context.Context, req admissionRequest) (*models.TaskSession, error) {
	if req.sessionID == "" {
		return nil, nil
	}
	session, err := s.repo.GetTaskSession(ctx, req.sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || (req.taskID != "" && session.TaskID != req.taskID) {
		return nil, errors.New("execution grade session does not belong to task")
	}
	return session, nil
}

func (c *sessionCeilingController) resolveAdmissionGradeLocked(ctx context.Context, req admissionRequest) admissionRequest {
	if c.policy != sessioncapacity.PolicyAll {
		req.startSelection = nil
	}
	if held := c.reservations[req.sessionID]; req.sessionID != "" && held != nil {
		req.grade, req.gradeKnown, req.gradeHeld = held.grade, held.gradeKnown, true
		return req
	}
	if c.resolveGrade != nil {
		grade, err := c.resolveGrade(ctx, req)
		if err == nil {
			req.grade, req.gradeKnown = grade, true
		}
	}
	return req
}

func (c *sessionCeilingController) persistAdmissionGradeLocked(ctx context.Context, req admissionRequest, decision admissionDecision) admissionDecision {
	if !decision.admitted || !req.gradeKnown || req.sessionID == "" || c.persistGrade == nil {
		return decision
	}
	if err := c.persistGrade(ctx, req.sessionID, req.grade); err != nil {
		if !req.gradeHeld {
			delete(c.reservations, decision.reservationKey)
		}
		c.logger.Error("could not persist admitted execution grade", zap.String("session_id", req.sessionID), zap.Error(err))
		return admissionDecision{ceiling: c.ceiling, reasonCode: ceilingReasonUnknownPopulation}
	}
	if held := c.reservations[decision.reservationKey]; held != nil {
		held.grade, held.gradeKnown = req.grade, true
	}
	return decision
}

// bindAdmissionGradeLocked runs with the existing reservation mutex held, before
// ownership moves. A colliding reservation is never modified by the losing caller.
func (c *sessionCeilingController) bindAdmissionGradeLocked(ctx context.Context, reservation *ceilingReservation, sessionID string) error {
	if c.resolveGrade == nil || c.persistGrade == nil {
		return nil
	}
	grade, err := c.resolveGrade(ctx, admissionRequest{sessionID: sessionID})
	known := err == nil
	if c.policy == sessioncapacity.PolicyAll {
		if err != nil {
			return err
		}
		if !reservation.gradeKnown || reservation.grade != grade {
			return errors.New("admitted execution grade changed before session binding")
		}
	}
	if !known {
		reservation.grade, reservation.gradeKnown = "", false
		return nil
	}
	if err := c.persistGrade(ctx, sessionID, grade); err != nil {
		return err
	}
	reservation.grade, reservation.gradeKnown = grade, true
	return nil
}
