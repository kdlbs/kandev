package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	runsmodels "github.com/kandev/kandev/internal/runs/models"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/service"
)

const (
	officeClaudeUsageNotice  = "You've hit your usage limit"
	officeCodexCreditsNotice = "Your workspace is out of credits. Add credits to continue."
)

// futureClaudeFiveHourNotice renders a Berlin reset clock that is always in
// the future, so the Office backoff assertion does not depend on wall time.
func futureClaudeFiveHourNotice(t *testing.T) string {
	t.Helper()
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("load Berlin zone: %v", err)
	}
	reset := time.Now().In(location).Add(2 * time.Hour)
	return fmt.Sprintf("Internal error: You've hit your 5-hour limit · resets %s (%s)",
		reset.Format("3:04pm"), location)
}

type officeQuotaCase struct {
	name     string
	provider routing.ProviderID
	next     routing.ProviderID
	source   string
	notice   string
	rule     string
}

func officeQuotaCases() []officeQuotaCase {
	return []officeQuotaCase{
		{
			name: "claude to codex", provider: "claude-acp", next: "codex-acp",
			source: streams.ProviderErrorSourceACPPrompt, notice: officeClaudeUsageNotice,
			rule: "claude.stderr.usage_limit.v1",
		},
		{
			name: "codex to claude", provider: "codex-acp", next: "claude-acp",
			source: streams.ProviderErrorSourceCodexACP, notice: officeCodexCreditsNotice,
			rule: "codex.stderr.out_of_credits.v1",
		},
	}
}

func officeQuotaProviderError(tc officeQuotaCase) *streams.ProviderError {
	return &streams.ProviderError{
		Source: tc.source, ProviderID: string(tc.provider),
		Message: tc.notice, OccurredAt: time.Now().UTC(),
	}
}

func publishOfficeAgentFailed(
	t *testing.T, eb bus.EventBus, payload lifecycle.AgentEventPayload,
) {
	t.Helper()
	if err := eb.Publish(context.Background(), events.AgentFailed,
		bus.NewEvent(events.AgentFailed, "test", payload)); err != nil {
		t.Fatalf("publish agent failed: %v", err)
	}
}

func launchTasklessQuotaCandidate(
	t *testing.T, svc *service.Service, launcher *routedTestLauncher,
	agent *models.AgentInstance, order []routing.ProviderID, key string,
) (*runsmodels.Run, *models.RunSession) {
	t.Helper()
	ctx := context.Background()
	seedRoutedWorkspaceConfig(t, svc.RepoForTest(), agent.WorkspaceID, order)
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, key); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list queued run: runs=%#v err=%v", runs, err)
	}
	run := runs[0]
	service.RunSchedulerTick(svc, ctx)
	if launcher.callCount() != 1 {
		t.Fatalf("launcher calls = %d, want 1", launcher.callCount())
	}
	session, err := svc.RepoForTest().GetRunSession(ctx, "route-session-1")
	if err != nil || session == nil || session.State != models.RunSessionStateRunning {
		t.Fatalf("initial run session = %#v, err=%v, want running", session, err)
	}
	return run, session
}

func assertOfficeProviderQuotaFallback(
	t *testing.T, svc *service.Service, run *runsmodels.Run, agent *models.AgentInstance,
	provider routing.ProviderID, rule string, before time.Time, wantAttempts, wantFailures int,
) {
	t.Helper()
	ctx := context.Background()
	persisted, err := svc.RepoForTest().GetRun(ctx, run.ID)
	if err != nil || persisted == nil || persisted.Status != runsmodels.RunStatusQueued {
		t.Fatalf("run after quota fallback = %#v, err=%v, want queued", persisted, err)
	}
	attempts, err := svc.RepoForTest().ListRouteAttempts(ctx, run.ID)
	if err != nil || len(attempts) != wantAttempts {
		t.Fatalf("route attempts = %#v, err=%v, want %d failed attempts", attempts, err, wantAttempts)
	}
	attempt := attempts[len(attempts)-1]
	if attempt.Outcome != "failed_provider_unavailable" || attempt.ErrorCode != string(routingerr.CodeQuotaLimited) ||
		attempt.ClassifierRule != rule || attempt.FinishedAt == nil {
		t.Fatalf("attempt = %#v, want failed_provider_unavailable quota via %s", attempt, rule)
	}
	health, err := svc.RepoForTest().ListProviderHealth(ctx, agent.WorkspaceID)
	if err != nil || len(health) == 0 {
		t.Fatalf("provider health = %#v, err=%v, want a degraded provider", health, err)
	}
	var degraded *models.ProviderHealth
	for i := range health {
		if health[i].ProviderID == string(provider) {
			degraded = &health[i]
			break
		}
	}
	if degraded == nil || degraded.ErrorCode != string(routingerr.CodeQuotaLimited) ||
		degraded.RetryAt == nil || !degraded.RetryAt.After(before) {
		t.Fatalf("degraded provider health = %#v, want quota with a future retry deadline", degraded)
	}
	persistedAgent, err := svc.GetAgentInstance(ctx, agent.ID)
	if err != nil || persistedAgent == nil {
		t.Fatalf("get agent: %#v, err=%v", persistedAgent, err)
	}
	if persistedAgent.ConsecutiveFailures != wantFailures {
		t.Fatalf("agent consecutive failures = %d, want %d after handled routing",
			persistedAgent.ConsecutiveFailures, wantFailures)
	}
	if persistedAgent.Status == models.AgentStatusPaused || persistedAgent.PauseReason != "" {
		t.Fatalf("agent was paused by a handled quota failure: %#v", persistedAgent)
	}
}

func TestOfficeProviderQuotaPostStartFallback(t *testing.T) {
	for _, tc := range officeQuotaCases() {
		t.Run("taskless "+tc.name, func(t *testing.T) {
			svc, eb := newTestServiceWithBus(t)
			ctx := context.Background()
			sched := buildRoutedScheduler(t, svc)
			launcher := &routedTestLauncher{svc: svc, failFor: map[string]error{}}
			sched.SetRunSessionLauncher(launcher)
			svc.SetRoutingDispatcher(sched)

			agent := createRoutedTestAgent(t, svc, "quota-taskless-"+string(tc.provider))
			run, session := launchTasklessQuotaCandidate(
				t, svc, launcher, agent, []routing.ProviderID{tc.provider, tc.next}, "quota-"+tc.name,
			)
			before := time.Now().UTC()
			publishOfficeAgentFailed(t, eb, lifecycle.AgentEventPayload{
				AgentExecutionID: session.ExecutionID, AgentID: string(tc.provider), AgentProfileID: agent.ID,
				RunID: run.ID, RunSessionID: session.ID, RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun,
				WorkspaceID: agent.WorkspaceID, Status: "FAILED", ErrorMessage: tc.notice,
				ProviderError: officeQuotaProviderError(tc),
			})
			assertOfficeProviderQuotaFallback(t, svc, run, agent, tc.provider, tc.rule, before, 1, 0)

			service.RunSchedulerTick(svc, ctx)
			if launcher.callCount() != 2 {
				t.Fatalf("launcher calls after fallback = %d, want 2", launcher.callCount())
			}
			nextSession, err := svc.RepoForTest().GetRunSession(ctx, "route-session-2")
			if err != nil || nextSession == nil || nextSession.Adapter != string(tc.next) ||
				nextSession.State != models.RunSessionStateRunning {
				t.Fatalf("next run session = %#v, err=%v, want running %s", nextSession, err, tc.next)
			}
			runsAfterRetry, err := svc.ListRuns(ctx, agent.WorkspaceID)
			if err != nil || len(runsAfterRetry) != 1 || runsAfterRetry[0].SessionID != nextSession.ID {
				t.Fatalf("run projection after fallback = %#v, err=%v, want session %s", runsAfterRetry, err, nextSession.ID)
			}
		})

		t.Run("task-backed "+tc.name, func(t *testing.T) {
			svc, eb := newTestServiceWithBus(t)
			ctx := context.Background()
			sched := buildRoutedScheduler(t, svc)
			svc.SetRoutingDispatcher(sched)
			seedRoutedWorkspaceConfig(t, svc.RepoForTest(), "ws-1",
				[]routing.ProviderID{tc.provider, tc.next})

			agentID := "quota-task-agent-" + string(tc.provider)
			createTestAgent(t, svc, "ws-1", agentID)
			taskID := createOfficeTask(t, svc, "ws-1", agentID)
			if _, err := svc.QueueRun(ctx, agentID, service.RunReasonTaskAssigned,
				`{"task_id":"`+taskID+`"}`, "quota-task-"+tc.name); err != nil {
				t.Fatalf("queue task run: %v", err)
			}
			run, err := svc.ClaimNextRun(ctx)
			if err != nil || run == nil {
				t.Fatalf("claim task run: %v (run=%v)", err, run)
			}
			seq, err := svc.RepoForTest().IncrementRouteAttemptSeq(ctx, run.ID)
			if err != nil {
				t.Fatalf("increment route seq: %v", err)
			}
			if err := svc.RepoForTest().SetRunResolvedRoute(ctx, run.ID,
				string(tc.provider)+"-profile", string(tc.provider), string(tc.provider)+"-model"); err != nil {
				t.Fatalf("set resolved route: %v", err)
			}
			if err := svc.RepoForTest().AppendRouteAttempt(ctx, &models.RouteAttempt{
				RunID: run.ID, Seq: seq, ExecutionProfileID: string(tc.provider) + "-profile",
				ProviderID: string(tc.provider), Model: string(tc.provider) + "-model",
				Tier: "balanced", Outcome: "launched", StartedAt: time.Now().UTC(),
			}); err != nil {
				t.Fatalf("append route attempt: %v", err)
			}

			before := time.Now().UTC()
			publishOfficeAgentFailed(t, eb, lifecycle.AgentEventPayload{
				AgentID: string(tc.provider), AgentProfileID: agentID,
				RunID: run.ID, RunAttempt: seq, OwnerKind: lifecycle.ExecutionOwnerRun,
				TaskID: taskID, WorkspaceID: "ws-1", Status: "FAILED", ErrorMessage: tc.notice,
				ProviderError: officeQuotaProviderError(tc),
			})
			assertOfficeProviderQuotaFallback(t, svc, run, &models.AgentInstance{
				ID: agentID, WorkspaceID: "ws-1",
			}, tc.provider, tc.rule, before, 1, 0)
		})
	}
}

// resetOfficeProviderHealth clears the provider's degraded rows so the next
// scheduler tick deterministically selects the same candidate again.
func resetOfficeProviderHealth(t *testing.T, svc *service.Service, provider routing.ProviderID) {
	t.Helper()
	ctx := context.Background()
	rows, err := svc.RepoForTest().ListProviderHealth(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list provider health: %v", err)
	}
	for _, row := range rows {
		if row.ProviderID != string(provider) {
			continue
		}
		if err := svc.RepoForTest().MarkProviderHealthy(
			ctx, "ws-1", row.ProviderID, string(row.Scope), row.ScopeValue,
		); err != nil {
			t.Fatalf("reset provider health: %v", err)
		}
	}
}

// TestOfficeProviderQuotaDoesNotAutoPause starts each case one below the
// effective pause threshold and drives three recognized failures through the
// same agent. Route health is restored between attempts so each failure is a
// fresh handled quota event, and the original counter and unpaused state are
// asserted after every one.
func TestOfficeProviderQuotaDoesNotAutoPause(t *testing.T) {
	cases := officeQuotaCases()
	cases = append(cases, officeQuotaCase{
		name: "claude five-hour", provider: "claude-acp", next: "codex-acp",
		source: streams.ProviderErrorSourceACPPrompt, notice: futureClaudeFiveHourNotice(t),
		rule: "claude.stderr.five_hour_limit.v1",
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, eb := newTestServiceWithBus(t)
			sched := buildRoutedScheduler(t, svc)
			launcher := &routedTestLauncher{svc: svc, failFor: map[string]error{}}
			sched.SetRunSessionLauncher(launcher)
			svc.SetRoutingDispatcher(sched)

			agent := createRoutedTestAgent(t, svc, "quota-threshold-"+string(tc.provider)+"-"+strings.ReplaceAll(tc.name, " ", "-"))
			threshold, err := svc.RepoForTest().GetEffectiveFailureThreshold(ctx, agent.ID)
			if err != nil {
				t.Fatalf("read effective threshold: %v", err)
			}
			if threshold < 2 {
				t.Fatalf("effective threshold = %d, want at least 2", threshold)
			}
			startFailures := threshold - 1
			svc.ExecSQL(t, `UPDATE agent_profiles SET consecutive_failures = ? WHERE id = ?`, startFailures, agent.ID)
			run, _ := launchTasklessQuotaCandidate(
				t, svc, launcher, agent, []routing.ProviderID{tc.provider, tc.next}, "threshold-"+tc.name,
			)
			for attempt := 1; attempt <= 3; attempt++ {
				if attempt > 1 {
					resetOfficeProviderHealth(t, svc, tc.provider)
					if err := svc.RepoForTest().BumpRouteCycleBaseline(ctx, run.ID); err != nil {
						t.Fatalf("reset route cycle baseline: %v", err)
					}
					service.RunSchedulerTick(svc, ctx)
					if launcher.callCount() != attempt {
						t.Fatalf("launcher calls before attempt %d = %d, want %d",
							attempt, launcher.callCount(), attempt)
					}
				}
				session, err := svc.RepoForTest().GetRunSession(ctx, fmt.Sprintf("route-session-%d", attempt))
				if err != nil || session == nil || session.State != models.RunSessionStateRunning {
					t.Fatalf("attempt %d session = %#v, err=%v, want running", attempt, session, err)
				}
				before := time.Now().UTC()
				publishOfficeAgentFailed(t, eb, lifecycle.AgentEventPayload{
					AgentExecutionID: session.ExecutionID, AgentID: string(tc.provider), AgentProfileID: agent.ID,
					RunID: run.ID, RunSessionID: session.ID, RunAttempt: attempt, OwnerKind: lifecycle.ExecutionOwnerRun,
					WorkspaceID: agent.WorkspaceID, Status: "FAILED", ErrorMessage: tc.notice,
					ProviderError: officeQuotaProviderError(tc),
				})
				assertOfficeProviderQuotaFallback(t, svc, run, agent, tc.provider, tc.rule, before, attempt, startFailures)
				persisted, err := svc.GetAgentInstance(ctx, agent.ID)
				if err != nil {
					t.Fatalf("get agent after attempt %d: %v", attempt, err)
				}
				if persisted.ConsecutiveFailures != startFailures {
					t.Fatalf("consecutive failures after attempt %d = %d, want the original %d",
						attempt, persisted.ConsecutiveFailures, startFailures)
				}
				if persisted.Status == models.AgentStatusPaused || persisted.PauseReason != "" {
					t.Fatalf("attempt %d paused the agent: %#v", attempt, persisted)
				}
			}
		})
	}
}
