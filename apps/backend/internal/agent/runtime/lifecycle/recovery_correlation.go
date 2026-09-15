package lifecycle

import (
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

// RecoveryCorrelationResult is the outcome of correlating adopted-server
// instances to recovery-inventory records by session identity
// (AC-EXECUTORS-SURVIVAL-002.1), including the AC-EXECUTORS-SURVIVAL-002.10
// duplicate tiebreak.
type RecoveryCorrelationResult struct {
	// Winners maps a session identity to the single live instance to
	// re-track for it.
	Winners map[string]*agentctl.InstanceInfo
	// ToStop lists every instance that must be stopped: duplicate losers,
	// ambiguous-session instances (AC-EXECUTORS-SURVIVAL-002.10), and
	// orphans with no matching record (AC-EXECUTORS-SURVIVAL-002.6).
	ToStop []*agentctl.InstanceInfo
}

// CorrelateRecoveryInstances joins enumerated instances to recovery-inventory
// records by the session identity the control server itself reports
// (AC-EXECUTORS-SURVIVAL-002.1):
//
//   - Exactly one live instance for a session with a record re-tracks that
//     instance whether or not its instance identifier equals the record's
//     agent execution identifier (AC-EXECUTORS-SURVIVAL-002.2).
//   - More than one live instance for the same session re-tracks only the
//     one whose instance identifier equals that session's record's agent
//     execution identifier; no match or more than one match re-tracks none
//     of them and stops every one (AC-EXECUTORS-SURVIVAL-002.10).
//   - A live instance whose session has no record is stopped, because it has
//     no session identity to attach it to (AC-EXECUTORS-SURVIVAL-002.6).
//   - A record with no live instance is left alone entirely: it is not an
//     input this function stops or wins, it is left to the existing
//     stale-execution repair path (AC-EXECUTORS-SURVIVAL-002.7).
//
// Records are keyed by session identity, so this function has no dependency
// on iteration or arrival order between sessions (AC-EXECUTORS-SURVIVAL-002.11).
func CorrelateRecoveryInstances(
	records []*models.ExecutorRunning,
	instances []*agentctl.InstanceInfo,
) *RecoveryCorrelationResult {
	recordBySession := make(map[string]*models.ExecutorRunning, len(records))
	for _, rec := range records {
		if rec == nil || rec.SessionID == "" {
			continue
		}
		recordBySession[rec.SessionID] = rec
	}

	instancesBySession := make(map[string][]*agentctl.InstanceInfo)
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		instancesBySession[inst.SessionID] = append(instancesBySession[inst.SessionID], inst)
	}

	result := &RecoveryCorrelationResult{Winners: make(map[string]*agentctl.InstanceInfo)}
	for sessionID, group := range instancesBySession {
		record, hasRecord := recordBySession[sessionID]
		if !hasRecord {
			result.ToStop = append(result.ToStop, group...)
			continue
		}
		if len(group) == 1 {
			result.Winners[sessionID] = group[0]
			continue
		}
		result.resolveDuplicates(sessionID, record, group)
	}

	return result
}

// resolveDuplicates applies the AC-EXECUTORS-SURVIVAL-002.10 tiebreak for a
// session with more than one live instance.
func (r *RecoveryCorrelationResult) resolveDuplicates(
	sessionID string,
	record *models.ExecutorRunning,
	group []*agentctl.InstanceInfo,
) {
	var matches []*agentctl.InstanceInfo
	for _, inst := range group {
		if inst.ID == record.AgentExecutionID {
			matches = append(matches, inst)
		}
	}
	if len(matches) != 1 {
		// No match or an ambiguous match: re-track none, stop every candidate.
		r.ToStop = append(r.ToStop, group...)
		return
	}
	winner := matches[0]
	r.Winners[sessionID] = winner
	for _, inst := range group {
		if inst != winner {
			r.ToStop = append(r.ToStop, inst)
		}
	}
}
