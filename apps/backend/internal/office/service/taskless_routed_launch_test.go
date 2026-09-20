package service_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
	"github.com/kandev/kandev/internal/office/service"
)

// routedTestLauncher is service.RunSessionLauncher wired directly onto a
// real scheduler.SchedulerService (not the fake captureDispatcher used by
// the task-bound routing tests), so DispatchWithRouting / tryCandidates /
// HandlePostStartFailure run unmodified and persist real
// office_run_route_attempts / office_run_sessions rows. Session ids and
// attempts are keyed off run.CurrentRouteAttemptSeq, which
// dispatch_routing.go's recordAttemptStart sets on the run pointer before
// launchCandidate calls StartRunSession — so a failed candidate's seq is
// never reused by the candidate that actually launches.
type routedTestLauncher struct {
	svc *service.Service

	mu      sync.Mutex
	calls   []service.LaunchContext
	failFor map[string]error
}

func (l *routedTestLauncher) StartRunSession(
	ctx context.Context, run *models.Run, agent *models.AgentInstance,
	launch service.LaunchContext, route *service.RouteOverride,
) (service.RunSessionLaunch, error) {
	l.mu.Lock()
	l.calls = append(l.calls, launch)
	l.mu.Unlock()
	seq := run.CurrentRouteAttemptSeq
	id := fmt.Sprintf("route-session-%d", seq)
	now := time.Now().UTC()
	session := &models.RunSession{
		ID: id, WorkspaceID: agent.WorkspaceID, AgentProfileID: agent.ID,
		RunID: run.ID, Attempt: seq, State: models.RunSessionStatePreparing,
		CreatedAt: now, Version: 1,
	}
	reserved, err := l.svc.RepoForTest().ReserveRunSession(ctx, session)
	if err != nil || !reserved {
		return service.RunSessionLaunch{}, fmt.Errorf("reserve test run session: reserved=%v err=%w", reserved, err)
	}
	// Mirrors officeRunSessionLauncher.StartRunSession's real order: the
	// session is always reserved before the runtime launch can fail, so a
	// pre-launch-fallback candidate still leaves its own terminal session
	// row rather than none at all.
	if failErr, ok := l.failFor[route.ProviderID]; ok {
		if _, err := l.svc.RepoForTest().FinishRunSession(ctx, id, models.RunSessionStateFailed, failErr.Error()); err != nil {
			return service.RunSessionLaunch{}, err
		}
		return service.RunSessionLaunch{}, failErr
	}
	execID := "execution-" + id
	acpID := "acp-" + id
	if _, err := l.svc.RepoForTest().BindRunSessionExecution(
		ctx, id, execID, route.ExecutionProfileID, route.ProviderID, route.Model, acpID,
	); err != nil {
		return service.RunSessionLaunch{}, err
	}
	if _, err := l.svc.RepoForTest().MarkRunSessionStarted(
		ctx, id, execID, route.ExecutionProfileID, route.ProviderID, route.Model, acpID,
	); err != nil {
		return service.RunSessionLaunch{}, err
	}
	return service.RunSessionLaunch{
		SessionID: id, ExecutionID: execID,
		ExecutionProfileID: route.ExecutionProfileID, Adapter: route.ProviderID, Model: route.Model,
		ACPSessionID: acpID,
	}, nil
}

func (l *routedTestLauncher) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.calls)
}

// seedRoutedWorkspaceConfig mirrors scheduler_test's seedRoutingConfig
// (internal/office/scheduler/dispatch_routing_test.go), reimplemented here
// because that helper lives in an external test package this file cannot
// import. Provider ids must be real rules-catalogue keys ("claude-acp",
// "codex-acp", ...) for routingerr.Classify's per-provider rules (used by
// the proven "anthropic_quota_exceeded" fallback string) to match.
func seedRoutedWorkspaceConfig(
	t *testing.T, repo *officesqlite.Repository, workspaceID string, providers []routing.ProviderID,
) {
	t.Helper()
	profiles := map[routing.ProviderID]routing.ProviderProfile{}
	for _, p := range providers {
		profiles[p] = routing.ProviderProfile{
			TierMap:             routing.TierMap{Balanced: string(p) + "-bal"},
			ExecutionProfileIDs: routing.ExecutionProfileIDs{Balanced: string(p) + "-profile"},
			Mode:                "default",
		}
	}
	cfg := &routing.WorkspaceConfig{
		Enabled: true, DefaultTier: routing.TierBalanced,
		ProviderOrder: providers, ProviderProfiles: profiles,
	}
	if err := repo.UpsertWorkspaceRouting(context.Background(), workspaceID, cfg); err != nil {
		t.Fatalf("upsert routing: %v", err)
	}
}

// buildRoutedScheduler wires a real scheduler.SchedulerService the same
// way internal/backendapp/main.go wires production: repo + resolver +
// run-session launcher, then handed to Service.SetRoutingDispatcher so
// launchAgent's taskless branch reaches DispatchWithRouting instead of
// the direct RunSessionLauncher path.
func buildRoutedScheduler(t *testing.T, svc *service.Service) *scheduler.SchedulerService {
	t.Helper()
	sched := scheduler.NewSchedulerService(svc.RepoForTest(), logger.Default(), svc)
	sched.SetResolver(routing.NewResolver(svc.RepoForTest(), nil))
	return sched
}

func createRoutedTestAgent(t *testing.T, svc *service.Service, id string) *models.AgentInstance {
	t.Helper()
	agent := &models.AgentInstance{
		ID: id, WorkspaceID: "ws-1", Name: id,
		Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := svc.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func countTaskRows(t *testing.T, svc *service.Service, workspaceID string) (tasks, taskSessions int) {
	t.Helper()
	if err := svc.RepoForTest().ReaderDB().Get(&tasks,
		`SELECT COUNT(*) FROM tasks WHERE workspace_id = ?`, workspaceID); err != nil {
		t.Fatalf("count task rows: %v", err)
	}
	if err := svc.RepoForTest().ReaderDB().Get(&taskSessions,
		`SELECT COUNT(*) FROM task_sessions`); err != nil {
		t.Fatalf("count task_session rows: %v", err)
	}
	return tasks, taskSessions
}

// TestRoutedTaskless_FirstCandidateSucceeds covers the one-attempt-succeeds
// half of AC-OFFICE-TASKLESS-001.3's routed-profile requirement: exactly
// one attempt row carrying the candidate's provider/model/execution
// profile, the Office-built prompt reaching the launcher, runs.session_id
// bound to the single session, and no task/task_session rows created.
func TestRoutedTaskless_FirstCandidateSucceeds(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()
	sched := buildRoutedScheduler(t, svc)
	launcher := &routedTestLauncher{svc: svc, failFor: map[string]error{}}
	sched.SetRunSessionLauncher(launcher)
	svc.SetRoutingDispatcher(sched)
	seedRoutedWorkspaceConfig(t, svc.RepoForTest(), "ws-1",
		[]routing.ProviderID{"claude-acp", "codex-acp"})

	agent := createRoutedTestAgent(t, svc, "routed-agent-solo")
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger,
		`{"one_time_instructions":"ROUTED_TASKLESS_PROMPT_SENTINEL"}`, "routed-solo"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list queued run: runs=%#v err=%v", runs, err)
	}
	run := runs[0]

	service.RunSchedulerTick(svc, ctx)

	if launcher.callCount() != 1 || launcher.calls[0].Prompt == "" {
		t.Fatalf("routed launch = %#v, want exactly one real-prompt call", launcher.calls)
	}
	persistedRun, err := svc.GetRun(ctx, run.ID)
	if err != nil || persistedRun == nil {
		t.Fatalf("reload dispatched run: %v (run=%v)", err, persistedRun)
	}
	if launcher.calls[0].Prompt != persistedRun.AssembledPrompt {
		t.Fatalf("routed prompt = %q, want persisted Office prompt %q", launcher.calls[0].Prompt, persistedRun.AssembledPrompt)
	}
	if !strings.Contains(launcher.calls[0].Prompt, "ROUTED_TASKLESS_PROMPT_SENTINEL") {
		t.Fatalf("routed prompt lost the known Office instruction: %q", launcher.calls[0].Prompt)
	}
	if len(launcher.calls[0].AdditionalSkillSlugs) != 0 {
		t.Fatalf("taskless launch received task-only skills: %v", launcher.calls[0].AdditionalSkillSlugs)
	}

	attempts, err := svc.RepoForTest().ListRouteAttempts(ctx, run.ID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("route attempts = %#v, err=%v, want exactly one", attempts, err)
	}
	if attempts[0].ProviderID != "claude-acp" || attempts[0].ExecutionProfileID != "claude-acp-profile" {
		t.Fatalf("attempt candidate = %#v, want claude-acp/claude-acp-profile", attempts[0])
	}

	sessions, err := svc.RepoForTest().ListRunSessions(ctx, run.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("run sessions = %#v, err=%v, want exactly one", sessions, err)
	}
	session := sessions[0]
	if session.State != models.RunSessionStateRunning {
		t.Fatalf("session state = %q, want running", session.State)
	}
	if session.Adapter != "claude-acp" || session.Model != "claude-acp-bal" ||
		session.ExecutionProfileID != "claude-acp-profile" {
		t.Fatalf("session route fields = %#v, want claude-acp/claude-acp-bal/claude-acp-profile", session)
	}

	runs, err = svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 || runs[0].SessionID != session.ID {
		t.Fatalf("run projection = %#v, err=%v, want session_id=%q", runs, err, session.ID)
	}

	tasks, taskSessions := countTaskRows(t, svc, agent.WorkspaceID)
	if tasks != 0 || taskSessions != 0 {
		t.Fatalf("routed taskless launch created tasks=%d task_sessions=%d, want 0/0", tasks, taskSessions)
	}
}

// TestRoutedTaskless_PreLaunchFallback_TwoAttemptsTwoSessions covers a
// routed profile whose first candidate fails during the launch call itself
// (a provider error returned from StartRunSession, not a post-start event):
// two office_run_route_attempts rows (seq 1 failed, seq 2 launched) and two
// office_run_sessions rows, since StartRunSession always reserves its
// session before it can fail — the failed candidate's row lands Failed, the
// winning candidate's row lands Running. Also covers usage attribution
// landing on the launched attempt's own session, not the failed candidate's
// (AC-OFFICE-TASKLESS-001.4).
func TestRoutedTaskless_PreLaunchFallback_TwoAttemptsTwoSessions(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	sched := buildRoutedScheduler(t, svc)
	launcher := &routedTestLauncher{svc: svc, failFor: map[string]error{
		"claude-acp": fmt.Errorf("anthropic_quota_exceeded: please try again"),
	}}
	sched.SetRunSessionLauncher(launcher)
	svc.SetRoutingDispatcher(sched)
	seedRoutedWorkspaceConfig(t, svc.RepoForTest(), "ws-1",
		[]routing.ProviderID{"claude-acp", "codex-acp"})

	agent := createRoutedTestAgent(t, svc, "routed-agent-prelaunch")
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "routed-prelaunch"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list queued run: runs=%#v err=%v", runs, err)
	}
	run := runs[0]

	service.RunSchedulerTick(svc, ctx)

	// Ordering: both candidates were tried in the same synchronous
	// dispatch (not just a final count), so the launcher recorded the
	// failing claude-acp call before the succeeding codex-acp call.
	if launcher.callCount() != 2 {
		t.Fatalf("launcher calls = %d, want 2 (one failed, one succeeded)", launcher.callCount())
	}

	attempts, err := svc.RepoForTest().ListRouteAttempts(ctx, run.ID)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("route attempts = %#v, err=%v, want exactly two", attempts, err)
	}
	if attempts[0].Seq != 1 || attempts[0].ProviderID != "claude-acp" ||
		attempts[0].Outcome != scheduler.RouteAttemptOutcomeFailedProviderUnavail ||
		attempts[0].FinishedAt == nil {
		t.Fatalf("first attempt = %#v, want seq=1 claude-acp failed_provider_unavailable with FinishedAt set", attempts[0])
	}
	if attempts[1].Seq != 2 || attempts[1].ProviderID != "codex-acp" ||
		attempts[1].Outcome != scheduler.RouteAttemptOutcomeLaunched {
		t.Fatalf("second attempt = %#v, want seq=2 codex-acp launched", attempts[1])
	}

	// A session was reserved for BOTH candidates: StartRunSession always
	// reserves before it can fail, so the failed claude-acp candidate still
	// leaves its own terminal row rather than none at all.
	sessions, err := svc.RepoForTest().ListRunSessions(ctx, run.ID)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("run sessions = %#v, err=%v, want exactly two (one per candidate)", sessions, err)
	}
	var failedSession, session *models.RunSession
	for i := range sessions {
		switch sessions[i].Attempt {
		case 1:
			failedSession = &sessions[i]
		case 2:
			session = &sessions[i]
		}
	}
	// Production's BindRunSessionExecution (which sets Adapter/Model) only
	// runs after a successful runtime.Launch, so a candidate that fails
	// inside StartRunSession is reserved but never adapter-bound — its
	// identity is the run/attempt pair and the recorded error, not Adapter.
	if failedSession == nil || failedSession.State != models.RunSessionStateFailed ||
		failedSession.ErrorMessage == "" {
		t.Fatalf("failed candidate's session = %#v, want attempt=1 failed with an error message", failedSession)
	}
	if session == nil || session.Adapter != "codex-acp" || session.Attempt != 2 ||
		session.State != models.RunSessionStateRunning {
		t.Fatalf("launched session = %#v, want codex-acp attempt=2 running", session)
	}

	runs, err = svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 || runs[0].SessionID != session.ID {
		t.Fatalf("run projection = %#v, err=%v, want session_id=%q", runs, err, session.ID)
	}

	tasks, taskSessions := countTaskRows(t, svc, agent.WorkspaceID)
	if tasks != 0 || taskSessions != 0 {
		t.Fatalf("routed fallback launch created tasks=%d task_sessions=%d, want 0/0", tasks, taskSessions)
	}

	// Usage attribution reaches the cost ledger joined to the attempt
	// that actually launched (codex-acp / session), not a first-attempt
	// session that never existed.
	usagePayload := lifecycle.AgentStreamEventPayload{
		ExecutionID: session.ExecutionID, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, RunSessionID: session.ID, RunAttempt: 2,
		AgentProfileID: agent.ID, AgentType: "claude-acp",
		Data: &lifecycle.AgentStreamEventData{
			PromptGeneration: 1, TurnID: "turn-1", CurrentModelID: session.Model,
			Usage: &streams.PromptUsage{
				InputTokens: 5, OutputTokens: 2, OutputTokensPresent: true, TotalTokens: 7,
				ProviderReportedCostSubcents: 42, ProviderReportedCostPresent: true,
			},
		},
	}
	if err := eb.Publish(ctx, events.BuildAgentStreamSubject(session.ID),
		bus.NewEvent(events.AgentStream, "test", usagePayload)); err != nil {
		t.Fatalf("publish usage: %v", err)
	}
	var costCount int
	if err := svc.RepoForTest().ReaderDB().Get(&costCount,
		`SELECT COUNT(*) FROM office_cost_events WHERE session_id = ?`, session.ID); err != nil {
		t.Fatalf("count costs for launched session: %v", err)
	}
	if costCount != 1 {
		t.Fatalf("cost events joined to launched session = %d, want 1", costCount)
	}
	var totalCosts int
	if err := svc.RepoForTest().ReaderDB().Get(&totalCosts, `SELECT COUNT(*) FROM office_cost_events`); err != nil {
		t.Fatalf("count total costs: %v", err)
	}
	if totalCosts != 1 {
		t.Fatalf("total cost events = %d, want 1 (none misattributed to the failed first candidate)", totalCosts)
	}
}

// TestRoutedTaskless_PostStartFallback_RetryAndDelayedDuplicateAreSafe
// covers the retry half of AC-OFFICE-TASKLESS-001.2 (a post-start failure
// produces a genuinely distinct second session/ACP session on the same
// run, driven through the real HandlePostStartFailure -> RequeueForNextCandidate
// -> next scheduler tick chain, not a synthetic two-session setup) and the
// same-run half of AC-OFFICE-TASKLESS-001.4: once attempt 2 is live, a
// delayed/duplicate terminal event naming the now-stale attempt 1 session
// must be a safe no-op — no run completion, no second cost-ledger row, no
// cleared agent-working state — because resolveLifecycleRun's session-state
// gate excludes an already-terminal session from an exact-id lookup.
func TestRoutedTaskless_PostStartFallback_RetryAndDelayedDuplicateAreSafe(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	sched := buildRoutedScheduler(t, svc)
	launcher := &routedTestLauncher{svc: svc, failFor: map[string]error{}}
	sched.SetRunSessionLauncher(launcher)
	svc.SetRoutingDispatcher(sched)
	seedRoutedWorkspaceConfig(t, svc.RepoForTest(), "ws-1",
		[]routing.ProviderID{"claude-acp", "codex-acp"})

	agent := createRoutedTestAgent(t, svc, "routed-agent-poststart")
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "routed-poststart"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list queued run: runs=%#v err=%v", runs, err)
	}
	run := runs[0]

	service.RunSchedulerTick(svc, ctx)
	if launcher.callCount() != 1 {
		t.Fatalf("launcher calls after first tick = %d, want 1", launcher.callCount())
	}
	session1ID := "route-session-1"
	session1, err := svc.RepoForTest().GetRunSession(ctx, session1ID)
	if err != nil || session1 == nil || session1.State != models.RunSessionStateRunning {
		t.Fatalf("session1 after launch = %#v, err=%v, want running", session1, err)
	}

	// Real post-start failure event for attempt 1: this is the same
	// production event a live agent process publishes, not a bypass of
	// the event-subscriber machinery.
	if err := eb.Publish(ctx, events.AgentFailed, bus.NewEvent(events.AgentFailed, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: session1.ExecutionID, AgentID: "claude-acp", AgentProfileID: agent.ID,
		RunID: run.ID, RunSessionID: session1ID, RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "FAILED",
		ErrorMessage: "anthropic_quota_exceeded: please try again",
	})); err != nil {
		t.Fatalf("publish attempt1 failure: %v", err)
	}

	session1, err = svc.RepoForTest().GetRunSession(ctx, session1ID)
	if err != nil || session1 == nil || session1.State != models.RunSessionStateFailed {
		t.Fatalf("session1 after post-start failure = %#v, err=%v, want failed", session1, err)
	}
	// Ordering: attempt 1's session reached its terminal state before
	// attempt 2 was ever reserved — assert this directly, not just the
	// eventual row count, by checking no second session exists yet.
	sessionsAfterFailure, err := svc.RepoForTest().ListRunSessions(ctx, run.ID)
	if err != nil || len(sessionsAfterFailure) != 1 {
		t.Fatalf("run sessions right after attempt1 failure = %#v, err=%v, want exactly one (attempt2 not reserved yet)",
			sessionsAfterFailure, err)
	}
	queuedRun, err := svc.RepoForTest().GetRun(ctx, run.ID)
	if err != nil || queuedRun.Status != models.RunStatusQueued {
		t.Fatalf("run status after post-start fallback = %#v, err=%v, want queued", queuedRun, err)
	}

	service.RunSchedulerTick(svc, ctx)
	if launcher.callCount() != 2 {
		t.Fatalf("launcher calls after second tick = %d, want 2 (retry to the next candidate)", launcher.callCount())
	}
	if got := launcher.calls[1]; got.Prompt == "" {
		t.Fatalf("retry launch context missing prompt: %#v", got)
	}

	session2ID := "route-session-2"
	session2, err := svc.RepoForTest().GetRunSession(ctx, session2ID)
	if err != nil || session2 == nil || session2.State != models.RunSessionStateRunning {
		t.Fatalf("session2 after retry = %#v, err=%v, want running", session2, err)
	}
	if session2.ID == session1.ID || session2.ExecutionID == session1.ExecutionID ||
		session2.ACPSessionID == session1.ACPSessionID {
		t.Fatalf("retry reused attempt1's session/execution/ACP identity: session1=%#v session2=%#v", session1, session2)
	}
	if session2.Adapter != "codex-acp" {
		t.Fatalf("retry candidate = %q, want codex-acp (claude-acp excluded after its failure)", session2.Adapter)
	}

	sessions, err := svc.RepoForTest().ListRunSessions(ctx, run.ID)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("run sessions after retry = %#v, err=%v, want exactly two", sessions, err)
	}
	runsAfterRetry, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runsAfterRetry) != 1 || runsAfterRetry[0].SessionID != session2.ID {
		t.Fatalf("run projection after retry = %#v, err=%v, want session_id=%q", runsAfterRetry, err, session2.ID)
	}

	var workingRunID string
	if err := svc.RepoForTest().ReaderDB().Get(&workingRunID,
		`SELECT working_run_id FROM agent_profiles WHERE id = ?`, agent.ID); err != nil {
		t.Fatalf("read working run: %v", err)
	}
	if workingRunID != run.ID {
		t.Fatalf("agent working_run_id after retry = %q, want %q", workingRunID, run.ID)
	}

	// Attempt 2's own usage lands on its own session.
	if err := eb.Publish(ctx, events.BuildAgentStreamSubject(session2ID),
		bus.NewEvent(events.AgentStream, "test", lifecycle.AgentStreamEventPayload{
			ExecutionID: session2.ExecutionID, OwnerKind: lifecycle.ExecutionOwnerRun,
			WorkspaceID: agent.WorkspaceID, RunSessionID: session2ID, RunAttempt: 2,
			AgentProfileID: agent.ID, AgentType: "codex-acp",
			Data: &lifecycle.AgentStreamEventData{
				PromptGeneration: 1, TurnID: "turn-1", CurrentModelID: session2.Model,
				Usage: &streams.PromptUsage{
					InputTokens: 4, OutputTokens: 1, OutputTokensPresent: true, TotalTokens: 5,
					ProviderReportedCostSubcents: 77, ProviderReportedCostPresent: true,
				},
			},
		})); err != nil {
		t.Fatalf("publish attempt2 usage: %v", err)
	}
	var costCountBefore int
	if err := svc.RepoForTest().ReaderDB().Get(&costCountBefore, `SELECT COUNT(*) FROM office_cost_events`); err != nil {
		t.Fatalf("count costs before duplicate: %v", err)
	}
	if costCountBefore != 1 {
		t.Fatalf("cost events after attempt2 usage = %d, want 1", costCountBefore)
	}

	// Delayed/duplicate terminal event for the now-stale attempt 1,
	// arriving after attempt 2 is live on the same run. Must be a
	// no-op: no completion, no second ledger row, no cleared
	// agent-working state, attempt2 and the run untouched.
	if err := eb.Publish(ctx, events.AgentFailed, bus.NewEvent(events.AgentFailed, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: session1.ExecutionID, AgentID: "claude-acp", AgentProfileID: agent.ID,
		RunID: run.ID, RunSessionID: session1ID, RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "FAILED",
		ErrorMessage: "anthropic_quota_exceeded: please try again",
	})); err != nil {
		t.Fatalf("publish delayed duplicate attempt1 failure: %v", err)
	}

	runAfterDuplicate, err := svc.RepoForTest().GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run after duplicate: %v", err)
	}
	if runAfterDuplicate.Status == models.RunStatusFinished || runAfterDuplicate.Status == models.RunStatusFailed {
		t.Fatalf("run status after delayed duplicate = %q, want no completion", runAfterDuplicate.Status)
	}
	session2Unchanged, err := svc.RepoForTest().GetRunSession(ctx, session2ID)
	if err != nil || session2Unchanged.State != models.RunSessionStateRunning || session2Unchanged.Version != session2.Version {
		t.Fatalf("session2 after delayed duplicate = %#v, err=%v, want unchanged running version=%d",
			session2Unchanged, err, session2.Version)
	}
	session1Unchanged, err := svc.RepoForTest().GetRunSession(ctx, session1ID)
	if err != nil || session1Unchanged.State != models.RunSessionStateFailed {
		t.Fatalf("session1 after delayed duplicate = %#v, err=%v, want unchanged failed", session1Unchanged, err)
	}
	attemptsAfterDuplicate, err := svc.RepoForTest().ListRouteAttempts(ctx, run.ID)
	if err != nil || len(attemptsAfterDuplicate) != 2 {
		t.Fatalf("route attempts after delayed duplicate = %#v, err=%v, want still exactly two", attemptsAfterDuplicate, err)
	}
	var costCountAfter int
	if err := svc.RepoForTest().ReaderDB().Get(&costCountAfter, `SELECT COUNT(*) FROM office_cost_events`); err != nil {
		t.Fatalf("count costs after duplicate: %v", err)
	}
	if costCountAfter != costCountBefore {
		t.Fatalf("cost events after delayed duplicate = %d, want unchanged %d", costCountAfter, costCountBefore)
	}
	var workingRunIDAfter string
	if err := svc.RepoForTest().ReaderDB().Get(&workingRunIDAfter,
		`SELECT working_run_id FROM agent_profiles WHERE id = ?`, agent.ID); err != nil {
		t.Fatalf("read working run after duplicate: %v", err)
	}
	if workingRunIDAfter != run.ID {
		t.Fatalf("agent working_run_id after delayed duplicate = %q, want unchanged %q", workingRunIDAfter, run.ID)
	}

	tasks, taskSessions := countTaskRows(t, svc, agent.WorkspaceID)
	if tasks != 0 || taskSessions != 0 {
		t.Fatalf("routed post-start fallback created tasks=%d task_sessions=%d, want 0/0", tasks, taskSessions)
	}
}
