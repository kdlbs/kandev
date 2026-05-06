// Package service holds office service helpers. This file contains the
// small surviving slivers of the legacy execution_policy.go — the
// task-assignee mutators surfaced via the dashboard handler — plus a
// few shared constants the prompt builder and event subscribers still
// reference.
//
// The full execution_policy state machine (EnterStage / AdvanceStage /
// RecordParticipantResponse / ApplyExecutionPolicyTransition) was
// removed in Phase 4 of task-model-unification; stage progression is
// now owned by the workflow engine. See ADR 0004 for the design.
package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/office/shared"
)

// Stage type tags carried in run payloads / prompt context. Originally
// derived from ExecutionStage.Type in the legacy policy schema; kept
// here as bare strings because the prompt builder still branches on
// them when rendering reviewer/ship-stage prompts. The engine emits
// the same string values from workflow_steps.stage_type.
const (
	stageTypeWork = "work"
	stageTypeShip = "ship"
)

// participantTypeAgent identifies an agent (vs user) actor in event
// payloads. Used to short-circuit self-comment runs and to label
// activity log entries.
const participantTypeAgent = "agent"

// SetTaskAssignee updates the assignee agent instance on a task. Used
// by tests and a couple of internal callers; the dashboard's
// permissioned variant is SetTaskAssigneeAsAgent.
func (s *Service) SetTaskAssignee(ctx context.Context, taskID, assigneeID string) error {
	return s.repo.UpdateTaskAssignee(ctx, taskID, assigneeID)
}

// SetTaskAssigneeAsAgent checks can_assign_tasks for the given caller
// before delegating to UpdateTaskAssignee. Passing callerAgentID=""
// skips the check (for internal/admin callers).
func (s *Service) SetTaskAssigneeAsAgent(ctx context.Context, callerAgentID, taskID, assigneeID string) error {
	if callerAgentID != "" {
		agent, err := s.repo.GetAgentInstance(ctx, callerAgentID)
		if err != nil {
			return fmt.Errorf("resolve caller: %w", err)
		}
		perms := shared.ResolvePermissions(shared.AgentRole(agent.Role), agent.Permissions)
		if !shared.HasPermission(perms, shared.PermCanAssignTasks) {
			return shared.ErrForbidden
		}
	}
	return s.repo.UpdateTaskAssignee(ctx, taskID, assigneeID)
}
