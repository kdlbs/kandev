// Agent "working" status lifecycle.
//
// The status is set at the launch boundary and cleared on every terminal
// path. Writing "working" gates nothing: isAgentActive
// (scheduler_integration.go) accepts idle AND working, and
// ClaimNextEligibleRun does not filter on agent status at all, so no run
// is blocked and no schedule is skipped by this transition.

package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
)

// Payload keys shared by the office agent status events. Declared here
// because this file's publisher pushed the repeated literals past the
// goconst threshold; the older publishers in event_subscribers.go,
// retry.go, and failure.go still inline them.
const (
	eventKeyAgentProfileID = "agent_profile_id"
	eventKeyWorkspaceID    = "workspace_id"
)

// markAgentWorking flips an idle agent to "working" as its run is handed to
// the adapter.
//
// Called BEFORE the launch rather than after it: the completion event for a
// fast run can be processed as soon as the adapter is invoked, and a
// mark-after-launch would race that reset and strand the agent showing
// "working" forever — the exact failure this feature must not introduce,
// since a stuck "working" reads as progress that is not happening.
// The caller clears the status when the launch turns out not to have
// happened.
func (s *Service) markAgentWorking(ctx context.Context, agent *models.AgentInstance) {
	if agent == nil || agent.ID == "" {
		return
	}
	changed, err := s.repo.MarkAgentWorking(ctx, agent.ID)
	if err != nil {
		// Status is a display signal, not a correctness gate: a failed
		// write must never abort a run that is otherwise ready to launch.
		s.logger.Warn("failed to mark agent working",
			zap.String("agent", agent.ID), zap.Error(err))
		return
	}
	if changed {
		s.publishAgentStatusChanged(ctx, agent.ID, agent.WorkspaceID, string(models.AgentStatusWorking))
	}
}

// clearAgentWorking returns an agent from "working" to "idle" once its run
// reaches any terminal state.
//
// Idempotent and safe to call from more than one path for the same run: the
// underlying CAS only fires while the agent is still "working". It is
// therefore also safe to call after autoPauseAgent has paused the agent --
// the pause wins and this becomes a no-op.
//
// The agent is looked up only when the status actually changed, so the
// common completion path pays for the extra read once per finished run
// rather than on every terminal transition (many of which never launched).
func (s *Service) clearAgentWorking(ctx context.Context, agentID string) {
	if agentID == "" {
		return
	}
	changed, err := s.repo.ClearAgentWorking(ctx, agentID)
	if err != nil {
		s.logger.Warn("failed to clear agent working status",
			zap.String("agent", agentID), zap.Error(err))
		return
	}
	if !changed {
		return
	}
	agent, err := s.repo.GetAgentInstance(ctx, agentID)
	if err != nil {
		s.logger.Debug("agent lookup for status broadcast failed",
			zap.String("agent", agentID), zap.Error(err))
		return
	}
	s.publishAgentStatusChanged(ctx, agentID, agent.WorkspaceID, string(models.AgentStatusIdle))
}

// publishAgentStatusChanged broadcasts an agent status flip so the chip
// updates live instead of waiting for the next poll.
//
// It publishes OfficeAgentUpdated, not OfficeAgentStatusChanged: the
// latter constant is declared in both internal/events and this package but
// has never had a publisher or a WS forwarding rule, so an event on it
// reaches no client. OfficeAgentUpdated is subscribed by the office
// broadcaster (gateway/websocket/office_notifications.go) and handled in
// apps/web/lib/ws/handlers/office.ts, which refetches the agent list.
func (s *Service) publishAgentStatusChanged(ctx context.Context, agentID, workspaceID, status string) {
	if s.eb == nil || workspaceID == "" {
		return
	}
	data := map[string]interface{}{
		eventKeyAgentProfileID: agentID,
		eventKeyWorkspaceID:    workspaceID,
		"status":               status,
	}
	_ = s.eb.Publish(ctx, events.OfficeAgentUpdated,
		bus.NewEvent(events.OfficeAgentUpdated, "office-agent-status", data))
}
