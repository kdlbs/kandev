package orchestrator

import (
	"context"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// exhaustWithRateLimit fails candidate a with a rate limit whose reset is
// five minutes away while b's circuit is open for four, leaving the route
// waiting without an eligible candidate.
func (f *failedRouteFixture) exhaustWithRateLimit() int64 {
	f.t.Helper()
	f.openCircuit(failedRouteB, f.now.Add(4*time.Minute))
	f.failPreResult(rateLimitedUntil(f.now.Add(5 * time.Minute)))
	state := f.routeState()
	if state.Status != "waiting" || state.ExecutionProfileID != "" {
		f.t.Fatalf("route = %s %q, want waiting without a candidate", state.Status, state.ExecutionProfileID)
	}
	return state.Generation
}

func TestFailedRouteAutoRetry_DisabledWorkspaceKeepsManualRecovery(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-off", failedRouteSettings{max: 1})
	generation := f.exhaustWithRateLimit()
	if _, scheduled := f.pending(); scheduled || f.armed() {
		t.Fatalf("pending=%v armed=%v, want nothing scheduled while the setting is off", scheduled, f.armed())
	}
	f.advance(f.now.Add(10 * time.Minute))
	f.fire(generation)
	if got := f.requestCount(); got != 0 {
		t.Fatalf("route actions = %d, want 0", got)
	}
}

func TestFailedRouteAutoRetry_RetriesOnceAfterEarliestCircuitExpiry(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-on", failedRouteSettings{autoRetry: true, max: 1})
	start := f.now
	generation := f.exhaustWithRateLimit()
	pending, scheduled := f.pending()
	if !scheduled || !f.armed() || !pending.Due.Equal(start.Add(4*time.Minute)) {
		t.Fatalf("pending = %#v scheduled=%v armed=%v, want due at b's circuit expiry", pending, scheduled, f.armed())
	}
	f.advance(start.Add(3 * time.Minute))
	f.fire(generation)
	if got := f.requestCount(); got != 0 || !f.armed() {
		t.Fatalf("early fire: route actions = %d armed=%v, want none and the timer re-armed", got, f.armed())
	}
	f.advance(pending.Due)
	f.fire(generation)
	f.fire(generation)
	if got := f.requestCount(); got != 1 {
		t.Fatalf("route actions = %d, want exactly 1", got)
	}
	request := f.requests[0]
	if request.Action != RouteActionRetry || !request.Automatic || request.ExpectedGeneration != generation {
		t.Fatalf("request = %#v, want an automatic retry of generation %d", request, generation)
	}
	session := f.currentSession()
	if session.RouteGeneration != generation+1 || session.ExecutionProfileID != failedRouteB ||
		session.RouteReason != agentruntime.RouteReasonAutoFailedRouteRetry {
		t.Fatalf("session route = gen %d %s %s, want b at the next generation with the automatic reason",
			session.RouteGeneration, session.ExecutionProfileID, session.RouteReason)
	}
}

func TestFailedRouteAutoRetry_LimitAndRepeatedFailureStop(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-limit", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	pending, _ := f.pending()
	f.advance(pending.Due)
	f.fire(generation)
	// The retried b fails the same way; a is still limited.
	f.activate(failedRouteB, generation+1)
	f.failPreResult(rateLimitedUntil(f.now.Add(5 * time.Minute)))
	if _, scheduled := f.pending(); scheduled {
		t.Fatal("a second automatic retry was scheduled past the limit of 1")
	}
	if state := f.routeState(); state.Status != "waiting" {
		t.Fatalf("route = %s, want the waiting recovery state", state.Status)
	}

	r := newFailedRouteFixture(t, "fr-repeat", failedRouteSettings{autoRetry: true, max: 3})
	generation = r.exhaustWithRateLimit()
	pending, _ = r.pending()
	r.advance(pending.Due)
	r.fire(generation)
	// b fails: a different candidate, so the chain retries once more.
	r.activate(failedRouteB, generation+1)
	r.openCircuit(failedRouteA, r.now.Add(2*time.Minute))
	r.failPreResult(rateLimitedUntil(r.now.Add(time.Minute)))
	second, scheduled := r.pending()
	if !scheduled {
		t.Fatal("a failure of another candidate should schedule the second retry")
	}
	r.advance(second.Due)
	r.fire(second.Generation)
	retried := r.currentSession()
	// The same candidate fails with the same code again: the chain stops.
	r.activate(retried.ExecutionProfileID, retried.RouteGeneration)
	r.openCircuit(otherFailedRouteCandidate(retried.ExecutionProfileID), r.now.Add(2*time.Minute))
	r.failPreResult(rateLimitedUntil(r.now.Add(time.Minute)))
	if _, scheduled := r.pending(); scheduled {
		t.Fatal("a repeated failure of a retried candidate scheduled another retry")
	}
	if got := r.requestCount(); got != 2 {
		t.Fatalf("route actions = %d, want 2", got)
	}
}

func otherFailedRouteCandidate(candidate string) string {
	if candidate == failedRouteA {
		return failedRouteB
	}
	return failedRouteA
}

func TestFailedRouteAutoRetry_DuplicateFailureSchedulesOnce(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-dup", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	first, _ := f.pending()
	f.advance(f.now.Add(30 * time.Second))
	failure := rateLimitedUntil(f.now)
	if f.scheduleFailedRouteAutoRetryAgain(failure) {
		t.Fatal("a redelivered failure recorded a second schedule")
	}
	if again, _ := f.pending(); again != first {
		t.Fatalf("pending = %#v, want the original schedule %#v", again, first)
	}
	f.advance(first.Due)
	f.fire(generation)
	f.fire(generation)
	if got := f.requestCount(); got != 1 {
		t.Fatalf("route actions = %d, want 1", got)
	}
}

func (f *failedRouteFixture) scheduleFailedRouteAutoRetryAgain(failure *routingerr.Error) bool {
	return f.svc.scheduleFailedRouteAutoRetry(f.ctx, f.session, dynamicruntime.FailedRouteFailure{
		ExecutionProfileID: failedRouteA, Code: failure.Code,
	})
}

func TestFailedRouteAutoRetry_StaleGenerationDoesNothing(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-stale", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	pending, _ := f.pending()
	f.advance(pending.Due)
	// A person retries first.
	if _, err := f.resolver.ResolveRouteAction(f.ctx, f.session, failedRouteProfile, "", generation, "retry"); err != nil {
		t.Fatalf("manual retry: %v", err)
	}
	f.fire(generation)
	if got := f.requestCount(); got != 0 {
		t.Fatalf("route actions = %d, want 0 after the route moved", got)
	}
}

func TestFailedRouteAutoRetry_SupersededAndIneligibleFailuresDoNotSchedule(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-superseded", failedRouteSettings{autoRetry: true, max: 1})
	f.openCircuit(failedRouteB, f.now.Add(4*time.Minute))
	f.svc.routeDynamicAgentFailure(f.ctx, watcher.AgentEventData{
		TaskID: f.taskID, SessionID: f.session, AgentExecutionID: "execution-old", ExecutionProfileID: failedRouteB,
	}, rateLimitedUntil(f.now.Add(5*time.Minute)))
	if _, scheduled := f.pending(); scheduled || f.armed() {
		t.Fatal("a failure of a superseded candidate scheduled a retry")
	}

	g := newFailedRouteFixture(t, "fr-auth", failedRouteSettings{autoRetry: true, max: 1})
	g.svc.scheduleFailedRouteAutoRetryAfterFailure(g.ctx, g.session, failedRouteA, &routingerr.Error{
		Code: routingerr.CodeAuthRequired, Class: routingerr.ClassHard, FallbackAllowed: true,
	})
	if g.armed() {
		t.Fatal("a credential failure scheduled an automatic retry")
	}
}

func TestFailedRouteAutoRetry_ScheduleSurvivesRestart(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-restart", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	pending, _ := f.pending()
	f.svc.stopDynamicPolicyRecovery()
	f.svc = f.newService()
	if !f.armed() {
		t.Fatal("restart did not re-arm the recorded automatic retry")
	}
	f.advance(pending.Due)
	f.fire(generation)
	if got := f.requestCount(); got != 1 {
		t.Fatalf("route actions = %d, want 1 after restart", got)
	}
	f.svc.stopDynamicPolicyRecovery()
	f.svc = f.newService()
	if f.armed() {
		t.Fatal("a consumed retry was re-armed after a second restart")
	}
}

func TestFailedRouteAutoRetry_SettingTurnedOffBeforeDueDoesNothing(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-turned-off", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	pending, _ := f.pending()
	workspace, err := f.repo.GetWorkspace(f.ctx, f.workspace)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	workspace.DynamicFailedRouteAutoRetryEnabled = false
	if err := f.repo.UpdateWorkspace(f.ctx, workspace); err != nil {
		t.Fatalf("UpdateWorkspace: %v", err)
	}
	f.advance(pending.Due)
	f.fire(generation)
	if got := f.requestCount(); got != 0 {
		t.Fatalf("route actions = %d, want 0 once the workspace turned the setting off", got)
	}
}

func TestFailedRouteAutoRetry_CompletedTurnResetsChain(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-reset", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	pending, _ := f.pending()
	f.advance(pending.Due)
	f.fire(generation)
	f.activate(failedRouteB, generation+1)
	f.svc.resetFailedRouteAutoRetryChain(f.ctx, f.currentSession())
	f.openCircuit(failedRouteA, f.now.Add(2*time.Minute))
	f.failPreResult(rateLimitedUntil(f.now.Add(time.Minute)))
	if _, scheduled := f.pending(); !scheduled {
		t.Fatal("after a completed turn the next exhausted failure should schedule a new retry")
	}
}

func TestFailedRouteAutoRetry_MidTurnFailoverActsFirst(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-midturn", failedRouteSettings{autoRetry: true, max: 1, midTurn: true})
	f.openCircuit(failedRouteB, f.now.Add(4*time.Minute))
	execution := "execution-" + failedRouteA
	f.svc.beginDynamicAttempt(f.session)
	f.svc.bindDynamicAttemptExecution(f.session, execution)
	f.svc.observePromptAttempt(f.session, execution, 1, true, false)
	f.svc.routeDynamicAgentFailure(f.ctx, watcher.AgentEventData{
		TaskID: f.taskID, SessionID: f.session, AgentExecutionID: execution, PromptGeneration: 1,
	}, rateLimitedUntil(f.now.Add(5*time.Minute)))
	f.svc.dynamicSuccessorWorkers.Wait()
	if got := f.requestCount(); got != 1 || f.requests[0].Action != RouteActionTryNext {
		t.Fatalf("route actions = %#v, want the mid-turn automatic try_next first", f.requests)
	}
	pending, scheduled := f.pending()
	if !scheduled {
		t.Fatal("the exhausted mid-turn handoff should schedule the automatic retry")
	}
	f.advance(pending.Due)
	f.fire(pending.Generation)
	if got := f.requestCount(); got != 2 || f.requests[1].Action != RouteActionRetry {
		t.Fatalf("route actions = %#v, want the automatic retry second", f.requests)
	}
}

// failedRouteActiveTurnService reports one active turn for the fixture. All
// other TurnService methods are unused by this path.
type failedRouteActiveTurnService struct {
	TurnService
	active *models.Turn
}

func (s *failedRouteActiveTurnService) GetActiveTurn(context.Context, string) (*models.Turn, error) {
	return s.active, nil
}

func TestFailedRouteAutoRetry_DefersWhileTurnIsActive(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-active-turn", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	pending, scheduled := f.pending()
	if !scheduled {
		t.Fatal("the exhausted route did not schedule an automatic retry")
	}
	turns := &failedRouteActiveTurnService{active: &models.Turn{ID: "turn-active"}}
	f.svc.SetTurnService(turns)
	f.advance(pending.Due)
	f.fire(generation)
	if got := f.requestCount(); got != 0 {
		t.Fatalf("route actions = %d, want none while the turn is still active", got)
	}
	if !f.armed() {
		t.Fatal("the automatic retry was dropped instead of staying armed while the turn is active")
	}
	turns.active = nil
	f.fire(generation)
	if got := f.requestCount(); got != 1 {
		t.Fatalf("route actions = %d, want the deferred automatic retry once the turn settles", got)
	}
}

func TestDynamicRecoveryTimer_StaleGenerationKeepsNewerTimer(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-timer-generation", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	if got := f.armedGeneration(); got != generation {
		t.Fatalf("armed generation = %d, want %d", got, generation)
	}
	f.svc.armFailedRouteAutoRetry(f.session, dynamicruntime.PendingFailedRouteRetry{
		Generation: generation - 1,
		Due:        f.now.Add(time.Minute),
	})
	if got := f.armedGeneration(); got != generation {
		t.Fatalf("a stale generation replaced the armed timer: got %d, want %d", got, generation)
	}
}

func TestFailedRouteAutoRetry_TerminalSessionIsNotRearmed(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-terminal", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	pending, scheduled := f.pending()
	if !scheduled {
		t.Fatal("the exhausted route did not schedule an automatic retry")
	}
	f.updateSession(func(session *models.TaskSession) { session.State = models.TaskSessionStateCompleted })
	turns := &failedRouteActiveTurnService{active: &models.Turn{ID: "turn-terminal"}}
	f.svc.SetTurnService(turns)
	f.advance(pending.Due)
	f.fire(generation)
	if f.armed() || f.requestCount() != 0 {
		t.Fatalf("terminal session with an active turn: armed=%v actions=%d, want neither", f.armed(), f.requestCount())
	}
	turns.active = nil
	f.fire(generation)
	if f.armed() || f.requestCount() != 0 {
		t.Fatalf("terminal session: armed=%v actions=%d, want neither", f.armed(), f.requestCount())
	}
	if _, stillPending := f.pending(); !stillPending {
		t.Fatal("the durable schedule should remain for a later explicit recovery")
	}
}

func TestFailedRouteAutoRetry_StaleGenerationIsNotRearmed(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-stale-defer", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	f.svc.removeDynamicPolicyRecoveryTimer(f.session)
	if f.svc.deferFailedRouteAutoRetry(f.ctx, f.session, generation+1) {
		t.Fatal("a deferral for another generation re-armed the retry")
	}
	if f.armed() {
		t.Fatal("a stale generation armed a timer")
	}
	if !f.svc.deferFailedRouteAutoRetry(f.ctx, f.session, generation) {
		t.Fatal("the matching generation did not re-arm the retry")
	}
	if got := f.armedGeneration(); got != generation {
		t.Fatalf("armed generation = %d, want %d", got, generation)
	}
}

func TestFailedRouteAutoRetry_DefersWhileSessionNotSettled(t *testing.T) {
	f := newFailedRouteFixture(t, "fr-not-settled", failedRouteSettings{autoRetry: true, max: 1})
	generation := f.exhaustWithRateLimit()
	pending, scheduled := f.pending()
	if !scheduled {
		t.Fatal("the exhausted route did not schedule an automatic retry")
	}
	f.updateSession(func(session *models.TaskSession) { session.State = models.TaskSessionStateRunning })
	f.advance(pending.Due)
	f.fire(generation)
	if got := f.requestCount(); got != 0 {
		t.Fatalf("route actions = %d, want none before the session settles", got)
	}
	if !f.armed() {
		t.Fatal("the automatic retry was dropped instead of staying armed until the session settles")
	}
	f.updateSession(func(session *models.TaskSession) { session.State = models.TaskSessionStateWaitingForInput })
	f.fire(generation)
	if got := f.requestCount(); got != 1 {
		t.Fatalf("route actions = %d, want the deferred automatic retry after settlement", got)
	}
}
