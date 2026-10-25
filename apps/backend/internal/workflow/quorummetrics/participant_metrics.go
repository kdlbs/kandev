package quorummetrics

import "expvar"

// workflowParticipantAgentUnresolved counts one occurrence, per role, of a
// quorum guard's required slate dropping a seat because its agent profile no
// longer resolves (REQ-OFFICE-REVIEW-SEATS-004.3): the agent was deleted
// after the seat was cast. It is distinct from
// workflowQuorumGuardNotFired — a guard can still fire after dropping such a
// seat, so this counter must not fold into that one.
var workflowParticipantAgentUnresolved = expvar.NewMap("workflow_participant_agent_unresolved_total")

// RecordParticipantAgentUnresolved counts one seat dropped from role's
// required slate because its agent profile no longer resolves.
func RecordParticipantAgentUnresolved(role string) {
	workflowParticipantAgentUnresolved.Add(role, 1)
}
