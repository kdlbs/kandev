package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// ErrMaterializedRouteProfileChange refuses a start that would relabel a
// session whose durable dynamic route already belongs to another logical
// profile. Selecting a different dynamic profile is a session handoff; an
// in-place relabel would leave the session label and the authoritative route
// disagreeing and resume the previous family's concrete candidate.
var ErrMaterializedRouteProfileChange = errors.New(
	"agent profile change is not supported on a session with a materialized dynamic route; start a new session instead")

// refuseMaterializedRouteProfileChange runs before any session write. A session
// without a materialized route keeps its first profile selection, and a route
// that already matches the requested profile resumes normally.
func (s *Service) refuseMaterializedRouteProfileChange(
	ctx context.Context,
	session *models.TaskSession,
	effectiveProfileID string,
) error {
	if session == nil || effectiveProfileID == "" || effectiveProfileID == session.AgentProfileID {
		return nil
	}
	loader, ok := s.repo.(dynamicRouteStateLoader)
	if !ok {
		return nil
	}
	state, err := loader.LoadRouteState(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("load durable dynamic route state: %w", err)
	}
	if state == nil || state.Generation <= 0 || state.LogicalProfileID == "" ||
		state.LogicalProfileID == effectiveProfileID {
		return nil
	}
	s.logger.Warn("refusing agent profile change on a session with a materialized dynamic route",
		zap.String("session_id", session.ID),
		zap.String("current_profile", session.AgentProfileID),
		zap.String("requested_profile", effectiveProfileID),
		zap.String("route_logical_profile", state.LogicalProfileID),
		zap.Int64("route_generation", state.Generation))
	return fmt.Errorf("%w: session %s route %s requested %s",
		ErrMaterializedRouteProfileChange, session.ID, state.LogicalProfileID, effectiveProfileID)
}
