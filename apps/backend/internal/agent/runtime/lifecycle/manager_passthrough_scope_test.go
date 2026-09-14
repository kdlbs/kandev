package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// TestStopAgentWithReasonTerminatesPassthroughOnBackendShutdownWhenSurvivalEnabled
// pins AC-EXECUTORS-SURVIVAL-005.3's "never detached" clause. A passthrough
// session's agent runs on a terminal owned by this backend process, so it dies
// with the backend whatever the control server does. StopAllAgents calls
// StopAgentWithReason with StopReasonBackendShutdown for every tracked
// execution, including passthrough ones on the standalone runtime, so without
// the exclusion such a session takes the survivable-detach branch: no
// StopInstance, no agent.stopped, and an executors_running row left claiming a
// live agent that no later pass repairs (AC-EXECUTORS-SURVIVAL-004.5).
func TestStopAgentWithReasonTerminatesPassthroughOnBackendShutdownWhenSurvivalEnabled(t *testing.T) {
	mgr, mockExecutor := newDetachTestManager(t, true)
	execution := newDetachTestExecution("exec-passthrough", "session-passthrough")
	execution.IsPassthrough = true
	require.NoError(t, mgr.executionStore.Add(execution))

	err := mgr.StopAgentWithReason(context.Background(), execution.ID, StopReasonBackendShutdown, false)
	require.NoError(t, err)

	require.Len(t, mockExecutor.stopInstanceCalls, 1,
		"a passthrough session must behave as if the capability were disabled: the runtime instance is still stopped")

	eventBus := mgr.eventBus.(*MockEventBus)
	require.True(t, hasAgentStoppedEvent(eventBus.PublishedEvents),
		"a passthrough session must still publish agent.stopped: its PTY agent is genuinely gone")
}

// TestStopAgentWithReasonTerminatesPassthroughIdentifiedByProcessID covers the
// second half of the in-memory passthrough idiom this package already uses
// (IsAgentReadyForPrompt, RecoverAgentPromptStream): an execution whose
// terminal process exists is passthrough even if the flag was not carried.
func TestStopAgentWithReasonTerminatesPassthroughIdentifiedByProcessID(t *testing.T) {
	mgr, mockExecutor := newDetachTestManager(t, true)
	execution := newDetachTestExecution("exec-passthrough-pid", "session-passthrough-pid")
	execution.PassthroughProcessID = "pty-1"
	require.NoError(t, mgr.executionStore.Add(execution))

	err := mgr.StopAgentWithReason(context.Background(), execution.ID, StopReasonBackendShutdown, false)
	require.NoError(t, err)

	require.Len(t, mockExecutor.stopInstanceCalls, 1,
		"an execution carrying a terminal process is passthrough and must take the terminating path")
	eventBus := mgr.eventBus.(*MockEventBus)
	require.True(t, hasAgentStoppedEvent(eventBus.PublishedEvents), "a terminating stop must publish agent.stopped")
}

// TestManagerStartExcludesPassthroughRecordFromRecovery pins
// AC-EXECUTORS-SURVIVAL-005.3's "never re-tracked" clause at the seam that
// decides it. A passthrough session owns a real agentctl instance -- created
// by CreateInstance like any other execution, or promoted from a
// workspace-only one -- so that instance really does outlive this backend and
// would correlate to the session's recovery-inventory record. Handing the
// record to RecoverInstances is therefore all it takes to re-track a session
// whose agent is gone and report it as having survived.
func TestManagerStartExcludesPassthroughRecordFromRecovery(t *testing.T) {
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)

	var recovered []*models.ExecutorRunning
	mock := &guardObservingExecutor{
		name: executor.NameStandalone,
		onRecoverInstances: func(_ context.Context, records []*models.ExecutorRunning) {
			recovered = records
		},
	}
	execRegistry.Register(mock)

	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{SessionID: "session-normal"},
		{SessionID: "session-passthrough"},
		{SessionID: "session-unreadable"},
	}})
	mgr.SetPassthroughLookup(func(_ context.Context, sessionID string) (bool, bool) {
		switch sessionID {
		case "session-passthrough":
			return true, true
		case "session-unreadable":
			return false, false
		default:
			return false, true
		}
	})

	require.NoError(t, mgr.Start(context.Background()))

	sessions := make([]string, 0, len(recovered))
	for _, record := range recovered {
		sessions = append(sessions, record.SessionID)
	}
	require.NotContains(t, sessions, "session-passthrough",
		"a confirmed-passthrough record must never reach re-tracking")
	require.Contains(t, sessions, "session-normal", "an ordinary standalone record must still be recovered")
	require.Contains(t, sessions, "session-unreadable",
		"a session whose passthrough mode could not be read is guarded rather than excluded, so it stays recoverable")
}

// TestRecoverableRecordsKeepsEverythingButConfirmedPassthrough pins the
// filter's exact scope: it removes only records the pass classified as
// passthrough. A record with no session identity is not something the pass
// could classify either way, and correlation already skips it, so dropping it
// here would narrow recovery for a reason the AC does not give.
func TestRecoverableRecordsKeepsEverythingButConfirmedPassthrough(t *testing.T) {
	records := []*models.ExecutorRunning{
		{SessionID: "guarded-a"},
		{SessionID: "passthrough"},
		{SessionID: ""},
		nil,
		{SessionID: "guarded-b"},
	}

	got := recoverableRecords(records, []string{"guarded-a", "guarded-b"})

	sessions := make([]string, 0, len(got))
	for _, record := range got {
		require.NotNil(t, record)
		sessions = append(sessions, record.SessionID)
	}
	require.Equal(t, []string{"guarded-a", "", "guarded-b"}, sessions)
}

// TestRecoverableRecordsKeepsEveryRecordWhenNothingIsExcluded guards the
// no-passthrough case, which is every ordinary installation: the filter must
// be a pass-through when SessionsToGuard excluded nothing.
func TestRecoverableRecordsKeepsEveryRecordWhenNothingIsExcluded(t *testing.T) {
	records := []*models.ExecutorRunning{{SessionID: "a"}, {SessionID: "b"}}
	got := recoverableRecords(records, []string{"a", "b"})
	require.Len(t, got, 2)
	require.Equal(t, records[0], got[0])
	require.Equal(t, records[1], got[1])
}
