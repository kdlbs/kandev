package backendapp

import (
	"context"
	"maps"

	agentmodels "github.com/kandev/kandev/internal/agent/settings/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func portableProfileMatches(p *agentmodels.AgentProfile, descriptor *wfmodels.AgentProfilePortable) bool {
	return p != nil && p.AgentDisplayName == descriptor.AgentName && p.Model == descriptor.Model &&
		p.Mode == descriptor.Mode && (descriptor.ConfigOptions == nil || maps.Equal(p.ConfigOptions, *descriptor.ConfigOptions))
}

type portableCandidates struct {
	best, current *agentmodels.AgentProfile
	count         int
}

func (c *portableCandidates) add(p *agentmodels.AgentProfile, descriptor *wfmodels.AgentProfilePortable, currentID string) {
	if !portableProfileMatches(p, descriptor) {
		return
	}
	if p.ID == currentID {
		c.current = p
	}
	if !p.Enabled || p.WorkspaceID != "" {
		return
	}
	c.count++
	if c.best == nil || isOlderAgentProfileMatch(p, c.best) {
		c.best = p
	}
}

func (c *portableCandidates) result() wfmodels.ProfileMatch {
	if c.current != nil && (!c.current.Enabled || c.current.WorkspaceID != "") {
		c.count++
	}
	if c.current != nil {
		c.best = c.current
	}
	if c.best == nil {
		return wfmodels.ProfileMatch{Reason: "no matching profile"}
	}
	if c.count > 1 {
		return wfmodels.ProfileMatch{ID: c.best.ID, Reason: "multiple matching profiles"}
	}
	return wfmodels.ProfileMatch{ID: c.best.ID}
}

func buildAgentProfileLookup(repos *Repositories) wfmodels.AgentProfileLookup {
	return func(descriptor *wfmodels.AgentProfilePortable, currentID string) wfmodels.ProfileMatch {
		ctx := context.Background()
		agents, err := repos.AgentSettings.ListAgents(ctx)
		if err != nil {
			return wfmodels.ProfileMatch{Reason: "profile lookup failed"}
		}
		var candidates portableCandidates
		for _, agent := range agents {
			profiles, err := repos.AgentSettings.ListAgentProfiles(ctx, agent.ID)
			if err != nil {
				return wfmodels.ProfileMatch{Reason: "profile lookup failed"}
			}
			for _, p := range profiles {
				candidates.add(p, descriptor, currentID)
			}
		}
		return candidates.result()
	}
}
