package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// TestInheritedRecordIsProbedWhenNothingAnsweredAtTheRecordedEndpoint is the
// case that made every inherited record permanently unrepairable. On a start
// where nothing answered at the recorded endpoint, no control server holds
// those instances at all, so the process-identifier probe is the right
// authority and a genuinely dead record must still be repaired. Judging the
// record against the enumeration of a server this launch just started
// answers Unknown forever, and Unknown means "leave it alone".
func TestInheritedRecordIsProbedWhenNothingAnsweredAtTheRecordedEndpoint(t *testing.T) {
	mgr := newLivenessTestManager(t, newStandaloneControlServer(t, true))
	mgr.SetInheritedRecordScope(InheritedRecordScopeNoServer)
	row := &models.ExecutorRunning{
		SessionID: "session-inherited-noserver",
		Runtime:   agentruntime.RuntimeStandalone,
		LocalPID:  spawnAndReapPID(t),
	}
	// Reachable and empty: this is the freshly started server's enumeration,
	// which never launched the inherited instance and is therefore not
	// evidence about it either way.
	scope := &standaloneLivenessScope{reachable: true, liveBySession: map[string]struct{}{}}

	if got := mgr.classifyStandaloneLiveness(row, scope); got != models.ProcessLivenessDead {
		t.Fatalf("classifyStandaloneLiveness = %v, want Dead via the process-identifier probe", got)
	}
}

// TestInheritedRecordIsUnknownWhenAForeignServerAnswered is the opposite
// direction and the reason the probe cannot simply always apply. A server
// answered at the recorded endpoint and was left running, so the instance
// may still be alive on it. Probing a recycled or reused local PID there
// could report the record dead and repair a record whose agent is still
// working.
func TestInheritedRecordIsUnknownWhenAForeignServerAnswered(t *testing.T) {
	mgr := newLivenessTestManager(t, newStandaloneControlServer(t, true))
	mgr.SetInheritedRecordScope(InheritedRecordScopeForeignServer)
	row := &models.ExecutorRunning{
		SessionID: "session-inherited-foreign",
		Runtime:   agentruntime.RuntimeStandalone,
		LocalPID:  spawnAndReapPID(t),
	}
	scope := &standaloneLivenessScope{reachable: true, liveBySession: map[string]struct{}{}}

	if got := mgr.classifyStandaloneLiveness(row, scope); got != models.ProcessLivenessUnknown {
		t.Fatalf("classifyStandaloneLiveness = %v, want Unknown while a server this backend did not adopt is running", got)
	}
}

// TestRecordCreatedThisLifetimeIgnoresTheInheritedScope pins the split the
// rule turns on. A record this backend created is judged against whichever
// server it is using, adopted or freshly started, because that server is the
// one that launched the instance. Only records inherited from an earlier
// launch depend on what answered at the recorded endpoint.
func TestRecordCreatedThisLifetimeIgnoresTheInheritedScope(t *testing.T) {
	for _, scopeKind := range []InheritedRecordScope{
		InheritedRecordScopeNoServer,
		InheritedRecordScopeForeignServer,
		InheritedRecordScopeAdopted,
	} {
		mgr := newLivenessTestManager(t, newStandaloneControlServer(t, true))
		mgr.SetInheritedRecordScope(scopeKind)
		mgr.markSessionCreatedThisLifetime("session-own")

		alive := &models.ExecutorRunning{SessionID: "session-own", Runtime: agentruntime.RuntimeStandalone}
		present := &standaloneLivenessScope{reachable: true, liveBySession: map[string]struct{}{"session-own": {}}}
		if got := mgr.classifyStandaloneLiveness(alive, present); got != models.ProcessLivenessAlive {
			t.Fatalf("scope %v: present own record = %v, want Alive", scopeKind, got)
		}

		absent := &standaloneLivenessScope{reachable: true, liveBySession: map[string]struct{}{}}
		if got := mgr.classifyStandaloneLiveness(alive, absent); got != models.ProcessLivenessDead {
			t.Fatalf("scope %v: absent own record = %v, want Dead", scopeKind, got)
		}
	}
}
