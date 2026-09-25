package dashboard

import (
	"context"

	"github.com/kandev/kandev/internal/office/models"
)

// enrichActivityLabels resolves all names in bounded workspace-scoped reads.
// Activity rows are intentionally not joined one-by-one: the feed is a hot
// read path and historical identifiers must remain useful when a target was
// later removed.
func (s *DashboardService) enrichActivityLabels(
	ctx context.Context,
	workspaceID string,
	entries []*models.ActivityEntry,
	agents []*models.AgentInstance,
) {
	if len(entries) == 0 {
		return
	}
	agentNames := s.activityAgentNames(ctx, workspaceID, agents)
	tasksByID := s.activityTaskLabels(ctx, workspaceID, entries)
	s.applyActivityLabels(entries, agentNames, tasksByID)
}

func (s *DashboardService) activityAgentNames(
	ctx context.Context,
	workspaceID string,
	agents []*models.AgentInstance,
) map[string]string {
	if agents == nil && s.agents != nil {
		if listed, err := s.agents.ListAgentInstances(ctx, workspaceID); err == nil {
			agents = listed
		}
	}
	agentNames := make(map[string]string, len(agents))
	for _, agent := range agents {
		if agent != nil && agent.ID != "" {
			agentNames[agent.ID] = agent.Name
		}
	}
	return agentNames
}

func (s *DashboardService) activityTaskLabels(
	ctx context.Context,
	workspaceID string,
	entries []*models.ActivityEntry,
) map[string]taskLabel {
	for _, entry := range entries {
		if entry != nil && entry.TargetType == models.ActivityTargetType("task") && entry.TargetID != "" {
			return s.loadActivityTaskLabels(ctx, workspaceID)
		}
	}
	return map[string]taskLabel{}
}

func (s *DashboardService) loadActivityTaskLabels(
	ctx context.Context,
	workspaceID string,
) map[string]taskLabel {
	tasksByID := map[string]taskLabel{}
	tasks, err := s.repo.ListTasksByWorkspace(ctx, workspaceID, true)
	if err != nil {
		return tasksByID
	}
	for _, task := range tasks {
		if task != nil {
			tasksByID[task.ID] = taskLabel{Name: task.Title, Identifier: task.Identifier}
		}
	}
	return tasksByID
}

func (s *DashboardService) applyActivityLabels(
	entries []*models.ActivityEntry,
	agentNames map[string]string,
	tasksByID map[string]taskLabel,
) {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.ActorType == models.ActivityActorType("agent") {
			entry.ActorName = agentNames[entry.ActorID]
		}
		if label, ok := tasksByID[entry.TargetID]; ok {
			entry.TargetName = label.Name
			entry.TargetIdentifier = label.Identifier
		}
	}
}

type taskLabel struct {
	Name       string
	Identifier string
}
