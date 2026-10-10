package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	agentapi "github.com/kandev/kandev/internal/agent/runtime"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type idleParkingTestAgent struct {
	*mockAgentManager
	mu           sync.Mutex
	suspendCalls []agentapi.IdleSuspensionIdentity
	suspendFunc  func(context.Context, agentapi.IdleSuspensionIdentity) error
}

func (m *idleParkingTestAgent) SuspendIdle(ctx context.Context, identity agentapi.IdleSuspensionIdentity) error {
	m.mu.Lock()
	m.suspendCalls = append(m.suspendCalls, identity)
	suspendFunc := m.suspendFunc
	m.mu.Unlock()
	if suspendFunc != nil {
		return suspendFunc(ctx, identity)
	}
	return nil
}

func (m *idleParkingTestAgent) suspensionCalls() []agentapi.IdleSuspensionIdentity {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agentapi.IdleSuspensionIdentity(nil), m.suspendCalls...)
}

func newIdleParkingFixture(t *testing.T, enabled bool, timeout int, provider string) (*Service, *idleParkingTestAgent, *models.ExecutorRunning) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	old := time.Now().UTC().Add(-3 * time.Hour)
	workspace := &models.Workspace{
		ID: "workspace-idle-parking", Name: "Workspace idle parking",
		ACPIdleSuspensionEnabled: enabled, ACPIdleTimeoutMinutes: timeout,
	}
	if err := repo.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE workspaces SET updated_at = ? WHERE id = ?`, old, workspace.ID); err != nil {
		t.Fatalf("age workspace policy: %v", err)
	}
	task := &models.Task{
		ID: "task-idle-parking", WorkspaceID: workspace.ID, Title: "Idle parking",
		State: v1.TaskStateInProgress, CreatedAt: old, UpdatedAt: old,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	session := &models.TaskSession{
		ID: "session-idle-parking", TaskID: task.ID, AgentProfileID: provider,
		State: models.TaskSessionStateWaitingForInput, StartedAt: old, UpdatedAt: old,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	running := &models.ExecutorRunning{
		ID: session.ID, SessionID: session.ID, TaskID: task.ID,
		AgentExecutionID: "execution-idle-parking", ExecutorID: "executor-local",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		Resumable: true, ResumeToken: "conversation-token", CreatedAt: old, UpdatedAt: old,
	}
	if err := repo.UpsertExecutorRunning(ctx, running); err != nil {
		t.Fatalf("create runtime inventory: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE executors_running SET updated_at = ? WHERE session_id = ?`, old, session.ID); err != nil {
		t.Fatalf("age runtime settlement: %v", err)
	}
	agent := &idleParkingTestAgent{mockAgentManager: &mockAgentManager{isAgentRunning: true}}
	agent.currentPromptExecutionID = running.AgentExecutionID
	agent.currentPromptLastActivityAt = old
	agent.currentPromptGeneration.Store(4)
	agent.currentPromptActivityEpoch.Store(9)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	svc.turnService = &inactiveTurnService{}
	return svc, agent, running
}

func TestIdleParkingPolicyDisabledByDefaultAndScopedByWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	for _, id := range []string{"workspace-disabled-a", "workspace-disabled-b"} {
		if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: id, Name: id}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	first, err := repo.GetWorkspace(ctx, "workspace-disabled-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.GetWorkspace(ctx, "workspace-disabled-b")
	if err != nil {
		t.Fatal(err)
	}
	if first.ACPIdleSuspensionEnabled || second.ACPIdleSuspensionEnabled || first.ACPIdleTimeoutMinutes != 120 || second.ACPIdleTimeoutMinutes != 120 {
		t.Fatalf("workspace defaults = (%v,%d), (%v,%d); want disabled/120 for both",
			first.ACPIdleSuspensionEnabled, first.ACPIdleTimeoutMinutes,
			second.ACPIdleSuspensionEnabled, second.ACPIdleTimeoutMinutes)
	}
	first.ACPIdleSuspensionEnabled = true
	first.ACPIdleTimeoutMinutes = 45
	if err := repo.UpdateWorkspace(ctx, first); err != nil {
		t.Fatal(err)
	}
	second, err = repo.GetWorkspace(ctx, "workspace-disabled-b")
	if err != nil {
		t.Fatal(err)
	}
	if second.ACPIdleSuspensionEnabled || second.ACPIdleTimeoutMinutes != 120 {
		t.Fatalf("updating workspace A changed workspace B: enabled=%v timeout=%d", second.ACPIdleSuspensionEnabled, second.ACPIdleTimeoutMinutes)
	}
}

func TestIdleParkingSuspendsSettledSessionsAcrossACPProviders(t *testing.T) {
	for _, provider := range []string{"claude-acp", "opencode", "codex-acp", "gemini-acp"} {
		t.Run(provider, func(t *testing.T) {
			svc, agent, _ := newIdleParkingFixture(t, true, 1, provider)
			svc.suspendWorkspaceIdleSessionsOnce(context.Background())
			calls := agent.suspensionCalls()
			if len(calls) != 1 {
				t.Fatalf("suspension calls = %d, want 1", len(calls))
			}
			if calls[0].ExecutionID != "execution-idle-parking" || calls[0].SessionID != "session-idle-parking" ||
				calls[0].WorkspaceID != "workspace-idle-parking" || calls[0].PolicyUpdatedAt.IsZero() ||
				calls[0].PromptGeneration != 4 || calls[0].ActivityEpoch != 9 {
				t.Fatalf("suspension identity = %+v", calls[0])
			}
		})
	}
}

func TestIdleParkingKeepsDisabledAndKnownWorkSessionsRunning(t *testing.T) {
	t.Run("disabled workspace", func(t *testing.T) {
		svc, agent, _ := newIdleParkingFixture(t, false, 1, "claude-acp")
		svc.suspendWorkspaceIdleSessionsOnce(context.Background())
		if got := len(agent.suspensionCalls()); got != 0 {
			t.Fatalf("suspension calls = %d, want 0", got)
		}
	})
	t.Run("known background workload", func(t *testing.T) {
		svc, agent, _ := newIdleParkingFixture(t, true, 1, "opencode")
		svc.registerBackgroundTask("session-idle-parking", "background-work")
		svc.suspendWorkspaceIdleSessionsOnce(context.Background())
		if got := len(agent.suspensionCalls()); got != 0 {
			t.Fatalf("suspension calls = %d, want 0 for known active work", got)
		}
	})
}

func TestIdleParkingDoesNotDependOnProcessDescendantProbe(t *testing.T) {
	tests := []struct {
		name   string
		result client.ProbeResult
		err    error
	}{
		{name: "live", result: client.ProbeResultLive},
		{name: "unknown", result: client.ProbeResultUnknown},
		{name: "probe error", result: client.ProbeResultUnknown, err: errors.New("probe failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, agent, _ := newIdleParkingFixture(t, true, 1, "claude-acp")
			probeCalls := 0
			agent.probeBackgroundWorkloadsFunc = func(context.Context, string) (client.ProbeResult, error) {
				probeCalls++
				return tt.result, tt.err
			}
			svc.suspendWorkspaceIdleSessionsOnce(context.Background())
			if probeCalls != 0 {
				t.Fatalf("process-descendant probe calls = %d, want 0", probeCalls)
			}
			if got := len(agent.suspensionCalls()); got != 1 {
				t.Fatalf("suspension calls = %d, want 1 when process probe is %q (err=%v)", got, tt.result, tt.err)
			}
		})
	}
}

func TestIdleParkingDoesNotReleaseItsActiveSuspensionClaim(t *testing.T) {
	svc, agent, running := newIdleParkingFixture(t, true, 1, "claude-acp")
	ctx := context.Background()
	entered := make(chan struct{})
	finishStop := make(chan struct{})
	var finishStopOnce sync.Once
	releaseStop := func() { finishStopOnce.Do(func() { close(finishStop) }) }
	defer releaseStop()
	agent.suspendFunc = func(ctx context.Context, identity agentapi.IdleSuspensionIdentity) error {
		current, err := svc.repo.GetExecutorRunningBySessionID(ctx, identity.SessionID)
		if err != nil {
			return err
		}
		claimer := svc.repo.(interface {
			ClaimExecutorRunningIdleSuspension(context.Context, string, string, time.Time, string, time.Time) (bool, error)
		})
		claimed, err := claimer.ClaimExecutorRunningIdleSuspension(ctx, identity.SessionID, identity.ExecutionID,
			current.UpdatedAt, identity.WorkspaceID, identity.PolicyUpdatedAt)
		if err != nil || !claimed {
			return errors.Join(err, errors.New("test suspension claim was rejected"))
		}
		close(entered)
		select {
		case <-finishStop:
		case <-ctx.Done():
			return ctx.Err()
		}
		cas := svc.repo.(idleSuspensionStateCAS)
		if err := cas.CompareAndSetExecutorRunningIdleSuspension(ctx, identity.SessionID, identity.ExecutionID,
			time.Time{}, models.ExecutorIdleSuspensionInProgress, models.ExecutorIdleSuspensionAgentStopped); err != nil {
			return err
		}
		return cas.CompareAndSetExecutorRunningIdleSuspension(ctx, identity.SessionID, identity.ExecutionID,
			time.Time{}, models.ExecutorIdleSuspensionAgentStopped, models.ExecutorIdleSuspensionSuspended)
	}
	finished := make(chan struct{})
	go func() {
		svc.suspendWorkspaceIdleSessionsOnce(ctx)
		close(finished)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("idle suspension did not acquire its durable claim")
	}
	current, err := svc.repo.GetExecutorRunningBySessionID(ctx, running.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	summary := idleParkingScanSummary{observed: make(map[idleParkingCandidateKey]struct{}), skipped: make(map[idleParkingSkipReason]int)}
	svc.processWorkspaceIdleRow(ctx, current, time.Now().UTC(), agent, &summary)
	current, err = svc.repo.GetExecutorRunningBySessionID(ctx, running.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if current.IdleSuspensionState != models.ExecutorIdleSuspensionInProgress {
		releaseStop()
		t.Fatalf("concurrent reaper changed active claim to %q", current.IdleSuspensionState)
	}
	releaseStop()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("active suspension did not finish after its stop completed")
	}
	current, err = svc.repo.GetExecutorRunningBySessionID(ctx, running.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if current.IdleSuspensionState != models.ExecutorIdleSuspensionSuspended {
		t.Fatalf("completed suspension state = %q, want suspended", current.IdleSuspensionState)
	}
}

func TestIdleParkingRetriesOnlyClaimsWithTheCurrentWorkspacePolicy(t *testing.T) {
	for _, changed := range []bool{false, true} {
		name := "current_policy"
		if changed {
			name = "changed_policy"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			svc, agent, seed := newIdleParkingFixture(t, true, 1, "claude-acp")
			workspace, err := svc.repo.GetWorkspace(ctx, "workspace-idle-parking")
			if err != nil {
				t.Fatal(err)
			}
			running, err := svc.repo.GetExecutorRunningBySessionID(ctx, seed.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			claimer := svc.repo.(interface {
				ClaimExecutorRunningIdleSuspension(context.Context, string, string, time.Time, string, time.Time) (bool, error)
			})
			claimed, err := claimer.ClaimExecutorRunningIdleSuspension(ctx, running.SessionID, running.AgentExecutionID,
				running.UpdatedAt, workspace.ID, workspace.UpdatedAt)
			if err != nil || !claimed {
				t.Fatalf("claim = %v, error = %v", claimed, err)
			}
			if changed {
				workspace.ACPIdleTimeoutMinutes = 121
				updater := svc.repo.(interface {
					UpdateWorkspace(context.Context, *models.Workspace) error
				})
				if err := updater.UpdateWorkspace(ctx, workspace); err != nil {
					t.Fatal(err)
				}
			}
			running, err = svc.repo.GetExecutorRunningBySessionID(ctx, seed.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if recovered := svc.reconcileIdleSuspensionAfterRestart(ctx, running); !recovered {
				t.Fatal("reconciliation did not resolve the provisional claim")
			}
			got, err := svc.repo.GetExecutorRunningBySessionID(ctx, seed.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			calls := agent.suspensionCalls()
			if changed {
				if len(calls) != 0 || got.IdleSuspensionState != models.ExecutorIdleSuspensionNone {
					t.Fatalf("changed policy claim recovery = calls:%d state:%q, want released without resume", len(calls), got.IdleSuspensionState)
				}
			} else if len(calls) != 1 || got.IdleSuspensionState != models.ExecutorIdleSuspensionInProgress {
				t.Fatalf("current policy claim recovery = calls:%d state:%q, want one retry retaining claim", len(calls), got.IdleSuspensionState)
			}
		})
	}
}

func TestIdleParkingReleasesPromptLocksBeforeBoundedSuspensionIO(t *testing.T) {
	svc, agent, _ := newIdleParkingFixture(t, true, 1, "claude-acp")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	entered := make(chan struct{})
	deadlineSeen := make(chan bool, 1)
	agent.suspendFunc = func(ctx context.Context, _ agentapi.IdleSuspensionIdentity) error {
		_, ok := ctx.Deadline()
		deadlineSeen <- ok
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}

	finished := make(chan struct{})
	go func() {
		svc.suspendWorkspaceIdleSessionsOnce(ctx)
		close(finished)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("suspension did not reach runtime IO")
	}

	locksAvailable := make(chan error, 1)
	go func() {
		locksAvailable <- svc.withSessionPromptAdmission(context.Background(), "session-idle-parking", func(admitted context.Context) error {
			release := svc.acquireSessionLifecycleLock("session-idle-parking")
			defer release()
			return nil
		})
	}()
	select {
	case err := <-locksAvailable:
		if err != nil {
			t.Fatalf("acquire session operations during suspension: %v", err)
		}
	case <-time.After(time.Second):
		cancel()
		<-finished
		t.Fatal("session admission/lifecycle locks remained held across runtime IO")
	}
	if hasDeadline := <-deadlineSeen; !hasDeadline {
		cancel()
		<-finished
		t.Fatal("suspension runtime IO has no operation deadline")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("suspension did not stop after its parent context was cancelled")
	}
}

func TestIdleParkingReconcilesStoppedAgentWithCurrentIdentity(t *testing.T) {
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	cas, ok := svc.repo.(idleSuspensionStateCAS)
	if !ok {
		t.Fatal("repository does not support idle suspension state updates")
	}
	if err := cas.CompareAndSetExecutorRunningIdleSuspension(
		context.Background(), row.SessionID, row.AgentExecutionID, time.Time{},
		models.ExecutorIdleSuspensionNone, models.ExecutorIdleSuspensionInProgress,
	); err != nil {
		t.Fatalf("claim idle suspension: %v", err)
	}
	if err := cas.CompareAndSetExecutorRunningIdleSuspension(
		context.Background(), row.SessionID, row.AgentExecutionID, time.Time{},
		models.ExecutorIdleSuspensionInProgress, models.ExecutorIdleSuspensionAgentStopped,
	); err != nil {
		t.Fatalf("mark agent stopped: %v", err)
	}
	agent.isAgentRunning = false

	svc.suspendWorkspaceIdleSessionsOnce(context.Background())
	calls := agent.suspensionCalls()
	if len(calls) != 1 {
		t.Fatalf("suspension calls = %d, want one retry", len(calls))
	}
	if calls[0].ExecutionID != row.AgentExecutionID || calls[0].SessionID != row.SessionID ||
		calls[0].PromptGeneration != 4 || calls[0].ActivityEpoch != 9 {
		t.Fatalf("reconciled suspension identity = %+v", calls[0])
	}
}

func TestIdleParkingClockHonorsPolicyChangeAndFocus(t *testing.T) {
	svc := &Service{}
	now := time.Now().UTC()
	key := idleParkingCandidateKey{
		workspaceID: "workspace", sessionID: "session", executionID: "execution",
		idleSince: now.Add(-time.Hour).UnixNano(), policyUpdated: now.Add(-2 * time.Minute).UnixNano(),
	}
	if svc.idleParkingCandidateDue(key, now, 5*time.Minute) {
		t.Fatal("policy change should start a fresh five-minute interval")
	}
	if !svc.idleParkingCandidateDue(key, now.Add(4*time.Minute), 5*time.Minute) {
		t.Fatal("candidate should be due after the fresh policy interval")
	}
	key.idleSince = now.Add(-time.Hour).UnixNano()
	key.policyUpdated = now.Add(-time.Hour).UnixNano()
	svc.idleParkingMu.Lock()
	svc.idleParkingCandidates = nil
	svc.idleParkingFocusAt = map[string]time.Time{key.sessionID: now.Add(-time.Minute)}
	svc.idleParkingMu.Unlock()
	if svc.idleParkingCandidateDue(key, now, 5*time.Minute) {
		t.Fatal("explicit focus should restart the idle interval")
	}
}

var _ idleSuspender = (*idleParkingTestAgent)(nil)
var _ idlePromptActivityReader = (*idleParkingTestAgent)(nil)
var _ executor.AgentManagerClient = (*idleParkingTestAgent)(nil)
