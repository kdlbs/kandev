package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

type tasklessTestLauncher struct {
	svc   *service.Service
	calls []service.LaunchContext
	next  int
}

func (l *tasklessTestLauncher) StartRunSession(
	ctx context.Context, run *models.Run, agent *models.AgentInstance,
	launch service.LaunchContext, _ *service.RouteOverride,
) (service.RunSessionLaunch, error) {
	l.calls = append(l.calls, launch)
	l.next++
	id := fmt.Sprintf("run-session-%d", l.next)
	now := time.Now().UTC()
	session := &models.RunSession{
		ID: id, WorkspaceID: agent.WorkspaceID, AgentProfileID: agent.ID,
		RunID: run.ID, Attempt: l.next, State: models.RunSessionStatePreparing,
		CreatedAt: now, Version: 1,
	}
	reserved, err := l.svc.RepoForTest().ReserveRunSession(ctx, session)
	if err != nil || !reserved {
		return service.RunSessionLaunch{}, fmt.Errorf("reserve test run session: reserved=%v err=%w", reserved, err)
	}
	if _, err := l.svc.RepoForTest().BindRunSessionExecution(
		ctx, id, "execution-"+id, launch.ProfileID, "test-adapter", "test-model", "acp-"+id,
	); err != nil {
		return service.RunSessionLaunch{}, err
	}
	if _, err := l.svc.RepoForTest().MarkRunSessionStarted(
		ctx, id, "execution-"+id, launch.ProfileID, "test-adapter", "test-model", "acp-"+id,
	); err != nil {
		return service.RunSessionLaunch{}, err
	}
	return service.RunSessionLaunch{
		SessionID: id, ExecutionID: "execution-" + id,
		ExecutionProfileID: launch.ProfileID, Adapter: "test-adapter", Model: "test-model",
		ACPSessionID: "acp-" + id,
	}, nil
}

func TestTasklessRoutineRunLaunchesRunSessionAndCompletes(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	launcher := &tasklessTestLauncher{svc: svc}
	svc.SetRunSessionLauncher(launcher)
	agent := &models.AgentInstance{
		ID: "taskless-agent", WorkspaceID: "ws-1", Name: "taskless-agent",
		Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "taskless-test"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list queued run: runs=%#v err=%v", runs, err)
	}
	run := runs[0]
	service.RunSchedulerTick(svc, ctx)
	if len(launcher.calls) != 1 || launcher.calls[0].Prompt == "" {
		t.Fatalf("taskless launch = %#v, want one real prompt", launcher.calls)
	}
	session, err := svc.RepoForTest().GetRunSession(ctx, "run-session-1")
	if err != nil || session == nil || session.State != models.RunSessionStateRunning {
		t.Fatalf("run session after launch = %#v, err=%v", session, err)
	}
	if runs, err := svc.ListRuns(ctx, agent.WorkspaceID); err != nil || len(runs) != 1 || runs[0].SessionID != "run-session-1" {
		t.Fatalf("run session projection = runs=%#v err=%v, want run-session-1", runs, err)
	}
	if err := eb.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-run-session-1", AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: run.ID, RunSessionID: "run-session-1", RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "COMPLETED",
	})); err != nil {
		t.Fatalf("publish completion: %v", err)
	}
	finished, err := svc.RepoForTest().GetRunSession(ctx, "run-session-1")
	if err != nil || finished == nil || finished.State != models.RunSessionStateFinished {
		t.Fatalf("run session after completion = %#v, err=%v", finished, err)
	}
	runs, err = svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 || runs[0].Status != service.RunStatusFinished {
		t.Fatalf("runs after completion = %#v, err=%v", runs, err)
	}
	var tasks int
	if err := svc.RepoForTest().ReaderDB().Get(&tasks, `SELECT COUNT(*) FROM tasks WHERE workspace_id = ?`, agent.WorkspaceID); err != nil {
		t.Fatalf("count task rows: %v", err)
	}
	if tasks != 0 {
		t.Fatalf("taskless run created %d task rows", tasks)
	}
}

func TestTasklessLateCompletionDoesNotFinishSuccessor(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	launcher := &tasklessTestLauncher{svc: svc}
	svc.SetRunSessionLauncher(launcher)
	agent := &models.AgentInstance{
		ID: "taskless-successor-agent", WorkspaceID: "ws-1", Name: "taskless-successor-agent",
		Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "first"); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list first run: runs=%#v err=%v", runs, err)
	}
	first := runs[0]
	service.RunSchedulerTick(svc, ctx)
	if _, err := svc.FinishRun(ctx, first.ID, service.RunOutcomeProcessed); err != nil {
		t.Fatalf("finish first: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "second"); err != nil {
		t.Fatalf("queue second: %v", err)
	}
	runs, err = svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 2 {
		t.Fatalf("list successor run: runs=%#v err=%v", runs, err)
	}
	second := runs[0]
	if second.ID == first.ID {
		second = runs[1]
	}
	if _, err := svc.RepoForTest().MarkAgentWorking(ctx, agent.ID, second.ID); err != nil {
		t.Fatalf("mark successor working: %v", err)
	}
	if err := eb.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-run-session-1", AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: first.ID, RunSessionID: "run-session-1", RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "COMPLETED",
	})); err != nil {
		t.Fatalf("publish stale completion: %v", err)
	}
	var workingRunID string
	if err := svc.RepoForTest().ReaderDB().Get(&workingRunID, `SELECT working_run_id FROM agent_profiles WHERE id = ?`, agent.ID); err != nil {
		t.Fatalf("read working run: %v", err)
	}
	if workingRunID != second.ID {
		t.Fatalf("successor working run = %q, want %q", workingRunID, second.ID)
	}
}

func TestTasklessUsageDuplicate(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	launcher := &tasklessTestLauncher{svc: svc}
	svc.SetRunSessionLauncher(launcher)
	agent := &models.AgentInstance{
		ID: "taskless-usage-agent", WorkspaceID: "ws-1", Name: "taskless-usage-agent",
		Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "usage"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	service.RunSchedulerTick(svc, ctx)
	payload := lifecycle.AgentStreamEventPayload{
		ExecutionID: "execution-run-session-1", OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, RunID: "", RunSessionID: "run-session-1", RunAttempt: 1,
		AgentProfileID: agent.ID, AgentType: "claude-acp",
		Data: &lifecycle.AgentStreamEventData{PromptGeneration: 1, TurnID: "turn-1", CurrentModelID: "test-model", Usage: &streams.PromptUsage{InputTokens: 7, OutputTokens: 3, OutputTokensPresent: true, TotalTokens: 10, ProviderReportedCostSubcents: 123, ProviderReportedCostPresent: true}},
	}
	stale := payload
	stale.RunAttempt++
	if err := eb.Publish(ctx, events.BuildAgentStreamSubject("run-session-1"), bus.NewEvent(events.AgentStream, "test", stale)); err != nil {
		t.Fatal(err)
	}
	var staleCosts int
	if err := svc.RepoForTest().ReaderDB().Get(&staleCosts, `SELECT COUNT(*) FROM office_cost_events`); err != nil {
		t.Fatal(err)
	}
	if staleCosts != 0 {
		t.Fatalf("mismatched attempt recorded %d cost events", staleCosts)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var wirePayload map[string]interface{}
	if err := json.Unmarshal(encoded, &wirePayload); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := eb.Publish(ctx, events.BuildAgentStreamSubject("run-session-1"), bus.NewEvent(events.AgentStream, "test", wirePayload)); err != nil {
			t.Fatalf("publish usage %d: %v", i, err)
		}
	}
	var costCount int
	if err := svc.RepoForTest().ReaderDB().Get(&costCount,
		`SELECT COUNT(*) FROM office_cost_events WHERE session_id = ?`, "run-session-1"); err != nil {
		t.Fatalf("count costs: %v", err)
	}
	if costCount != 1 {
		t.Fatalf("cost events = %d, want one deduplicated row", costCount)
	}
	costEvents, err := svc.RepoForTest().ListCostEvents(ctx, agent.WorkspaceID)
	if err != nil || len(costEvents) != 1 {
		t.Fatalf("workspace costs = %v, err=%v", costEvents, err)
	}
	for name, read := range map[string]func(context.Context, string) ([]*models.CostBreakdown, error){
		"agent": svc.RepoForTest().GetCostsByAgent, "model": svc.RepoForTest().GetCostsByModel,
		"provider": svc.RepoForTest().GetCostsByProvider, "project": svc.RepoForTest().GetCostsByProject,
	} {
		got, err := read(ctx, agent.WorkspaceID)
		if err != nil || len(got) != 1 || got[0].Count != 1 {
			t.Errorf("%s costs = %#v, err=%v", name, got, err)
		}
	}
	total, err := svc.RepoForTest().SumCosts(ctx, agent.WorkspaceID)
	if err != nil || total != 123 {
		t.Errorf("total = %d, err=%v", total, err)
	}
	total, err = svc.RepoForTest().SumCostsSince(ctx, agent.WorkspaceID, time.Now().Add(-time.Hour))
	if err != nil || total != 123 {
		t.Errorf("period total = %d, err=%v", total, err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	_, rollup, err := svc.RepoForTest().GetRunWithCosts(ctx, runs[0].ID)
	if err != nil || rollup.CostSubcents != 123 || rollup.InputTokens != 7 {
		t.Errorf("rollup = %#v, err=%v", rollup, err)
	}

}

func TestTasklessRoutineReadyCompletesRun(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	stopper := &fakeRunExecutionStopper{}
	svc.SetRunExecutionStopper(stopper)
	launcher := &tasklessTestLauncher{svc: svc}
	svc.SetRunSessionLauncher(launcher)
	agent := &models.AgentInstance{
		ID: "taskless-agent", WorkspaceID: "ws-1", Name: "taskless-agent",
		Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "taskless-test"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list queued run: runs=%#v err=%v", runs, err)
	}
	run := runs[0]
	service.RunSchedulerTick(svc, ctx)
	if len(launcher.calls) != 1 || launcher.calls[0].Prompt == "" {
		t.Fatalf("taskless launch = %#v, want one real prompt", launcher.calls)
	}
	session, err := svc.RepoForTest().GetRunSession(ctx, "run-session-1")
	if err != nil || session == nil || session.State != models.RunSessionStateRunning {
		t.Fatalf("run session after launch = %#v, err=%v", session, err)
	}
	if runs, err := svc.ListRuns(ctx, agent.WorkspaceID); err != nil || len(runs) != 1 || runs[0].SessionID != "run-session-1" {
		t.Fatalf("run session projection = runs=%#v err=%v, want run-session-1", runs, err)
	}
	stopper.err = errors.New("stop failed")
	_ = eb.Publish(ctx, events.AgentReady, bus.NewEvent(events.AgentReady, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-run-session-1", AgentProfileID: agent.ID,
		RunID: run.ID, RunSessionID: "run-session-1", RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun, WorkspaceID: agent.WorkspaceID,
	}))
	stillRunning, err := svc.RepoForTest().GetRunSession(ctx, "run-session-1")
	if err != nil || stillRunning.State != models.RunSessionStateRunning {
		t.Fatalf("failed stop settled session: %#v, %v", stillRunning, err)
	}
	stopper.err = nil
	stopper.executionIDs = nil
	if err := eb.Publish(ctx, events.AgentReady, bus.NewEvent(events.AgentReady, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-run-session-1", AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: run.ID, RunSessionID: "run-session-1", RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "READY",
	})); err != nil {
		t.Fatalf("publish completion: %v", err)
	}
	finished, err := svc.RepoForTest().GetRunSession(ctx, "run-session-1")
	if err != nil || finished == nil || finished.State != models.RunSessionStateFinished {
		t.Fatalf("run session after completion = %#v, err=%v", finished, err)
	}
	runs, err = svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 || runs[0].Status != service.RunStatusFinished {
		t.Fatalf("runs after completion = %#v, err=%v", runs, err)
	}
	var tasks int
	if err := svc.RepoForTest().ReaderDB().Get(&tasks, `SELECT COUNT(*) FROM tasks WHERE workspace_id = ?`, agent.WorkspaceID); err != nil {
		t.Fatalf("count task rows: %v", err)
	}
	if tasks != 0 {
		t.Fatalf("taskless run created %d task rows", tasks)
	}
	if len(stopper.executionIDs) != 1 || stopper.executionIDs[0] != "execution-run-session-1" {
		t.Fatalf("stopped executions = %v", stopper.executionIDs)
	}
}
