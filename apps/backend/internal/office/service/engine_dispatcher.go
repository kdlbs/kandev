package service

import (
	"github.com/kandev/kandev/internal/office/shared"
)

// SetWorkflowEngineDispatcher wires a dispatcher onto the office service.
// After Phase 4, when a dispatcher is set, the four event subscribers
// (comment_created, blockers_resolved, children_completed,
// approval_resolved) route through the engine unconditionally — there is
// no legacy fallback path.
//
// Calling SetWorkflowEngineDispatcher with nil disables engine routing
// (useful in tests that don't need a workflow engine wired).
func (s *Service) SetWorkflowEngineDispatcher(d shared.WorkflowEngineDispatcher) {
	s.engineDispatcher = d
}
