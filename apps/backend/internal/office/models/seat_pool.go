package models

import wfmodels "github.com/kandev/kandev/internal/workflow/models"

// seatRolePools maps a participant role to the agent roles eligible for its
// seat: approver draws from `ceo`; reviewer
// draws from `ceo` and `specialist`, and only those two. Any other
// participant role (watcher, collaborator, ...) keeps ceo-only behavior; the
// pool must not be widened without an amended AC.
var seatRolePools = map[string]map[AgentRole]bool{
	string(wfmodels.ParticipantRoleReviewer): {
		AgentRoleCEO:        true,
		AgentRoleSpecialist: true,
	},
	string(wfmodels.ParticipantRoleApprover): {
		AgentRoleCEO: true,
	},
}

// SeatPoolAgentRoles returns participantRole's agent-role allowlist. It is
// always a positive match, never a denylist, so worker, assistant and
// empty-role agents are never eligible.
func SeatPoolAgentRoles(participantRole string) map[AgentRole]bool {
	if pool, ok := seatRolePools[participantRole]; ok {
		return pool
	}
	return map[AgentRole]bool{AgentRoleCEO: true}
}

// IsSeatPoolEligible reports whether agent can fill participantRole's seat:
// its role is in the pool and its status is neither `stopped` nor
// `pending_approval`. Seat casting and
// the author-exclusion setup check share it so they cannot disagree.
func IsSeatPoolEligible(participantRole string, agent *AgentInstance) bool {
	if agent == nil || !SeatPoolAgentRoles(participantRole)[agent.Role] {
		return false
	}
	return agent.Status != AgentStatusStopped && agent.Status != AgentStatusPendingApproval
}
