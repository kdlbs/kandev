package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// standaloneLivenessScope is a single adopted-server enumeration snapshot
// (design 02 "Persistence": "liveness for a standalone record is judged by
// the presence of that record's instance in the adopted server's
// enumeration"). Reachable is false when the enumeration attempt itself
// failed -- nothing answered -- as distinct from a successful enumeration
// that simply found no matching instance. Produced by
// StandaloneExecutor.newLivenessScope and Manager.NewStandaloneLivenessScope;
// consumed only by classifyStandaloneLiveness. Never memoized across calls:
// a caller that wants one enumeration reused across many rows (a
// reconciliation pass) must build it once and thread it through itself; any
// other caller must build a fresh one for each classification (design 02
// "two kinds of caller, and only one of them is a pass").
type standaloneLivenessScope struct {
	reachable     bool
	liveBySession map[string]struct{}
}

// newLivenessScope takes one enumeration of the adopted server's live
// instances, keyed by session ID for O(1) row lookups. Reuses the same
// bounded per-attempt timeout/retry shape as listInstancesWithRetry's other
// callers (AC-EXECUTORS-SURVIVAL-002.13).
func (r *StandaloneExecutor) newLivenessScope(ctx context.Context) *standaloneLivenessScope {
	instances, err := r.listInstancesWithRetry(ctx)
	if err != nil {
		return &standaloneLivenessScope{reachable: false}
	}
	live := make(map[string]struct{}, len(instances))
	for _, inst := range instances {
		if inst.SessionID != "" {
			live[inst.SessionID] = struct{}{}
		}
	}
	return &standaloneLivenessScope{reachable: true, liveBySession: live}
}

// NewStandaloneLivenessScope takes one adopted-server enumeration for reuse
// across every row of a single reconciliation pass. Returns nil when no
// standalone backend is registered; classifyStandaloneLiveness treats a nil
// scope identically to an unreachable one (falls back to the
// process-identifier probe).
func (m *Manager) NewStandaloneLivenessScope(ctx context.Context) interface{} {
	backend, err := m.executorRegistry.GetBackend(executor.NameStandalone)
	if err != nil {
		return (*standaloneLivenessScope)(nil)
	}
	standalone, ok := backend.(*StandaloneExecutor)
	if !ok {
		return (*standaloneLivenessScope)(nil)
	}
	return standalone.newLivenessScope(ctx)
}

// classifyStandaloneLiveness implements design 02's "Persistence" liveness
// rule for a single row, given a (possibly nil/unreachable) enumeration
// scope:
//
//   - non-standalone runtime: unchanged, delegates to RowProcessLiveness
//     (Unknown -- never probed by a local process check).
//   - agent survival capability disabled: unchanged, delegates to
//     RowProcessLiveness -- an enumeration scope is never consulted when the
//     capability that makes a standalone control server outlive this
//     backend isn't itself enabled.
//   - scope nil or unreachable ("nothing answered"): presence is
//     undeterminable, so Unknown -- except where no control server answered
//     at the recorded endpoint at all, which falls back to today's
//     process-identifier probe so a genuinely dead row is still repaired on
//     the common case of a first start with no survivor.
//   - present in the enumeration, but this session's stop was still in
//     flight when the enumeration was taken: Unknown, not Alive -- the
//     snapshot predates the stop's actual completion.
//   - present, no stop in flight: Alive.
//   - absent, and this backend created the row during its own process
//     lifetime: Dead -- this exact server is the only authority for it, so
//     "not present" is determinate.
//   - absent, and the row was inherited from an earlier launch while this
//     backend adopted the recorded server: Unknown.
//
// A record inherited from an earlier launch is not judged against the
// enumeration at all unless this backend adopted the recorded server, since
// a server this backend started never launched that instance. What happens
// instead turns on what answered at the recorded endpoint: nothing answered
// means no server holds it, so the process-identifier probe applies and a
// genuinely dead record is still repaired; a server that answered and was
// left running means the instance may still be alive on a server this
// backend does not drive, so the record is Unknown and is left alone.
func (m *Manager) classifyStandaloneLiveness(row *models.ExecutorRunning, scope *standaloneLivenessScope) models.ProcessLiveness {
	if row == nil {
		return models.ProcessLivenessUnknown
	}
	if !isLocalRuntime(row.Runtime) {
		return RowProcessLiveness(row)
	}
	if !m.agentSurvivalEnabled {
		return RowProcessLiveness(row)
	}
	if scope == nil || !scope.reachable {
		// AC-EXECUTORS-SURVIVAL-003.6: presence could not be determined, so
		// this is unknown rather than live or dead. The process-identifier
		// probe cannot stand in for it here: a standalone row carries the
		// SHARED control server's identifier, so with a server still running
		// it answers live for an instance that is already gone -- reporting a
		// record live solely because the shared control server is running,
		// which that criterion forbids, and leaving the row to escape repair
		// indefinitely. The probe is only evidence where no control server
		// holds these instances at all, which is exactly the case the
		// criterion names: nothing answered at the recorded endpoint, or no
		// endpoint was recorded.
		if m.inheritedRecordScope == InheritedRecordScopeNoServer {
			return RowProcessLiveness(row)
		}
		return models.ProcessLivenessUnknown
	}
	sessionID := row.SessionID
	createdHere := m.wasCreatedThisLifetime(sessionID)
	if !createdHere && m.inheritedRecordScope != InheritedRecordScopeAdopted {
		// An inherited record is judged only against a server this backend
		// adopted. A freshly started server never launched this instance, so
		// its enumeration is not evidence either way and is not consulted.
		if m.inheritedRecordScope == InheritedRecordScopeNoServer {
			return RowProcessLiveness(row)
		}
		return models.ProcessLivenessUnknown
	}
	if _, present := scope.liveBySession[sessionID]; present {
		if m.recoveryGuard.IsStopInFlight(sessionID) {
			return models.ProcessLivenessUnknown
		}
		return models.ProcessLivenessAlive
	}
	if createdHere {
		return models.ProcessLivenessDead
	}
	return models.ProcessLivenessUnknown
}

// RowLivenessScoped classifies row's liveness using scope (from
// NewStandaloneLivenessScope), reused across every row of one reconciliation
// pass. scope must be the interface{} value NewStandaloneLivenessScope
// returned (or nil); any other type is treated as unreachable.
func (m *Manager) RowLivenessScoped(row *models.ExecutorRunning, scope interface{}) models.ProcessLiveness {
	s, _ := scope.(*standaloneLivenessScope)
	return m.classifyStandaloneLiveness(row, s)
}

// RowLiveness classifies row's liveness for a caller outside a reconciliation
// pass (e.g. the single-session idle reclaim path): it always takes its own
// fresh enumeration rather than reading one a pass cached (design 02 "two
// kinds of caller"), since such a caller can fire at any moment and a cached
// answer would age without bound.
//
// AC-EXECUTORS-SURVIVAL-003.6: such a caller firing before Start's recovery
// pass has finished for this process's lifetime must answer Unknown
// immediately -- neither enumerate nor wait -- since re-tracking has not yet
// reached an outcome for every record and a live enumeration taken now could
// race work recovery itself has not finished doing.
func (m *Manager) RowLiveness(row *models.ExecutorRunning) models.ProcessLiveness {
	if row == nil || !isLocalRuntime(row.Runtime) {
		return RowProcessLiveness(row)
	}
	if !m.recoveryComplete.Load() {
		return models.ProcessLivenessUnknown
	}
	scope, _ := m.NewStandaloneLivenessScope(context.Background()).(*standaloneLivenessScope)
	return m.classifyStandaloneLiveness(row, scope)
}
