package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"net/url"
	"strings"
)

type RuntimeUpdateNotifier interface {
	HandleAgentRuntimeUpdate(context.Context, agents.RuntimeUpdateNotice)
}

func (c *Controller) SetRuntimeUpdateNotifier(notifier RuntimeUpdateNotifier) {
	c.runtimeUpdateNotifier = notifier
}

func (c *Controller) publishRuntimeStatus(ctx context.Context, status dto.AgentUpdateStatusDTO) {
	if c.runtimeUpdateNotifier == nil {
		return
	}
	notice := agents.RuntimeUpdateNotice{AgentID: status.AgentName, RuntimeID: status.RuntimeID, DisplayName: status.DisplayName, URL: "/settings/agents#runtime-update-" + url.PathEscape(status.AgentName)}
	if status.Available && status.Enabled && status.CheckState == dto.AgentUpdateCheckStateUpdateAvailable {
		notice.PreviousVersion, notice.Version, notice.Status = status.EffectiveVersion, status.LatestVersion, "available"
		notice.OccurrenceID = runtimeNoticeKey(status.AgentName, status.RuntimeID, status.LatestVersion, "available")
		c.runtimeUpdateNotifier.HandleAgentRuntimeUpdate(ctx, notice)
	}
	if outcome := status.LastOutcome; outcome != nil && (outcome.Status == managedruntime.UpdateOutcomeSucceeded || outcome.Status == managedruntime.UpdateOutcomeFailed || outcome.Status == managedruntime.UpdateOutcomeInterrupted) {
		notice.PreviousVersion, notice.Version, notice.Status = outcome.PreviousVersion, outcome.TargetVersion, outcome.Status
		notice.OccurrenceID = runtimeNoticeKey(status.AgentName, status.RuntimeID, outcome.TargetVersion, outcome.Status, outcome.ID)
		c.runtimeUpdateNotifier.HandleAgentRuntimeUpdate(ctx, notice)
	}
}

func runtimeNoticeKey(parts ...string) string {
	return fmt.Sprintf("agent-runtime:%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}

func (c *Controller) ReplayRuntimeUpdateNotices(ctx context.Context) error {
	response, err := c.ListAgentUpdateStatuses(ctx)
	if err != nil {
		return err
	}
	for _, status := range response.Statuses {
		c.publishRuntimeStatus(ctx, status)
	}
	return nil
}
