package orchestrator

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
)

// TestProcessOnEnter_UnrecognizedActionType_WarnsAndContinuesDispatch covers
// AC-A6 — the bug report's own headline acceptance bullet: "An on_enter
// action the handler does not recognise emits a WARNING naming the step and
// action type. The silent discard is what made this take a full afternoon
// to find." Build round 1 (commit d4c3dd42a) added the default-branch
// warning in dispatchOnEnterActions but shipped with zero test coverage for
// it; Testing round 1 verified the behavior with an uncommitted throwaway
// probe and routed the card back for exactly this permanent test.
func TestProcessOnEnter_UnrecognizedActionType_WarnsAndContinuesDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-a6", "session-a6", "step-a6")

	session, err := repo.GetTaskSession(ctx, "session-a6")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)

	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build observed logger: %v", err)
	}
	svc.logger = log

	// A genuinely unrecognized action type followed by a real one: the
	// warning must not abort the loop, so set_session_mode must still run
	// and persist.
	step := &wfmodels.WorkflowStep{
		ID:         "step-a6",
		WorkflowID: "wf-a6",
		Name:       "AC-A6 Step",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterActionType("totally_bogus_action")},
			{Type: wfmodels.OnEnterSetSessionMode, Config: map[string]interface{}{"mode": "acceptEdits"}},
		}},
	}

	svc.processOnEnter(ctx, "task-a6", session, step, "", 0, nil)

	var warnings []observer.LoggedEntry
	for _, e := range logs.All() {
		if e.Message == "processOnEnter: unrecognized on_enter action type" {
			warnings = append(warnings, e)
		}
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d WARNING(s) for the unrecognized action, want exactly 1 (all entries: %+v)", len(warnings), logs.All())
	}
	fields := warnings[0].ContextMap()
	if fields["step_id"] != "step-a6" {
		t.Errorf("warning step_id = %v, want %q", fields["step_id"], "step-a6")
	}
	if fields["action_type"] != "totally_bogus_action" {
		t.Errorf("warning action_type = %v, want %q", fields["action_type"], "totally_bogus_action")
	}

	reloaded, err := repo.GetTaskSession(ctx, "session-a6")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if got := reloaded.Metadata[models.SessionMetaKeySessionMode]; got != "acceptEdits" {
		t.Errorf("set_session_mode after the unrecognized action = %v, want %q — the warning must not abort dispatch of subsequent actions", got, "acceptEdits")
	}
}

// TestProcessOnEnter_EnsureParticipantSeat_DoesNotWarn covers the regression
// found while investigating the on_enter dispatch divergence card:
// ensure_participant_seat is a recognized, ledger-owned action type (dispatched
// by engine.DispatchStepEntry, not by processOnEnter) but dispatchOnEnterActions
// had no case for it, so it fell into the AC-A6 default branch and logged the
// "unrecognized on_enter action type" warning on every step entry that declares
// it — including builtin Office review steps, which insertSeatAction seeds with
// this action. A genuinely unrecognized type alongside it must still warn
// exactly once, proving the fix narrows the default case rather than widening it.
func TestProcessOnEnter_EnsureParticipantSeat_DoesNotWarn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-seat", "session-seat", "step-seat")

	session, err := repo.GetTaskSession(ctx, "session-seat")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)

	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build observed logger: %v", err)
	}
	svc.logger = log

	step := &wfmodels.WorkflowStep{
		ID:         "step-seat",
		WorkflowID: "wf-seat",
		Name:       "Seat Step",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterEnsureParticipantSeat},
			{Type: wfmodels.OnEnterActionType("totally_bogus_action")},
		}},
	}

	svc.processOnEnter(ctx, "task-seat", session, step, "", 0, nil)

	var warnings []observer.LoggedEntry
	for _, e := range logs.All() {
		if e.Message == "processOnEnter: unrecognized on_enter action type" {
			warnings = append(warnings, e)
		}
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d WARNING(s), want exactly 1 (only the genuinely bogus type; ensure_participant_seat must not warn) (all entries: %+v)", len(warnings), logs.All())
	}
	if fields := warnings[0].ContextMap(); fields["action_type"] != "totally_bogus_action" {
		t.Errorf("warning action_type = %v, want %q — ensure_participant_seat must not be the one that warned", fields["action_type"], "totally_bogus_action")
	}
}

// TestProcessOnEnter_LedgerOwnedKinds_DoNotWarnAndWriteNoMarker covers
// AC-OFFICE-STEP-ENTRY-DISPATCH-002.3's full contract for the four kinds
// this diff moved into dispatchOnEnterActions's non-owner branch
// (clear_decisions, queue_run_for_each_participant, queue_run,
// run_code_review): a dispatcher that is not the owner of a kind skips it
// "without emitting a warning or an error record, and without writing a
// marker for it." Only the no-warning half had a test before this
// (TestProcessOnEnter_EnsureParticipantSeat_DoesNotWarn, for a different
// kind); this also asserts the no-marker-written half, which a
// processOnEnter that accidentally started claiming these kinds' markers
// would otherwise pass unnoticed.
func TestProcessOnEnter_LedgerOwnedKinds_DoNotWarnAndWriteNoMarker(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-ledger-owned", "session-ledger-owned", "step-a")

	session, err := repo.GetTaskSession(ctx, "session-ledger-owned")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)

	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build observed logger: %v", err)
	}
	svc.logger = log

	step := &wfmodels.WorkflowStep{
		ID:         "step-ledger-owned",
		WorkflowID: "wf-ledger-owned",
		Name:       "Ledger Owned Step",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterClearDecisions},
			{Type: wfmodels.OnEnterQueueRunForEachParticipant},
			{Type: wfmodels.OnEnterQueueRun},
			{Type: wfmodels.OnEnterRunCodeReview},
			{Type: wfmodels.OnEnterActionType("totally_bogus_action")},
		}},
	}

	// Allocate a real step-entry so GetStepEntryMarkerState has an entryID
	// to check markers against — processOnEnter never allocates one itself.
	task, err := repo.GetTask(ctx, "task-ledger-owned")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "step-ledger-owned"
	holder := &stepentry.AllocationResult{}
	allocCtx := stepentry.WithResultHolder(ctx, holder)
	allocCtx = stepentry.WithPendingAllocation(allocCtx, stepentry.PendingAllocation{
		StepID: "step-ledger-owned",
		Digest: "digest-ledger-owned",
		Positions: []stepentry.EnginePosition{
			{Position: 0, Kind: string(wfmodels.OnEnterClearDecisions)},
			{Position: 1, Kind: string(wfmodels.OnEnterQueueRunForEachParticipant)},
		},
	})
	if err := repo.UpdateTask(allocCtx, task); err != nil {
		t.Fatalf("UpdateTask (allocate entry): %v", err)
	}
	if holder.EntryID == 0 {
		t.Fatalf("expected a real entryID to be allocated")
	}

	svc.processOnEnter(ctx, "task-ledger-owned", session, step, "", holder.EntryID, nil)

	var warnings []observer.LoggedEntry
	for _, e := range logs.All() {
		if e.Message == "processOnEnter: unrecognized on_enter action type" {
			warnings = append(warnings, e)
		}
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d WARNING(s), want exactly 1 (only the bogus type; the four ledger-owned kinds must not warn) (all entries: %+v)", len(warnings), logs.All())
	}
	if fields := warnings[0].ContextMap(); fields["action_type"] != "totally_bogus_action" {
		t.Errorf("warning action_type = %v, want %q", fields["action_type"], "totally_bogus_action")
	}

	for position, kind := range []wfmodels.OnEnterActionType{
		wfmodels.OnEnterClearDecisions,
		wfmodels.OnEnterQueueRunForEachParticipant,
		wfmodels.OnEnterQueueRun,
		wfmodels.OnEnterRunCodeReview,
	} {
		_, _, found, err := repo.GetStepEntryMarkerState(ctx, holder.EntryID, position)
		if err != nil {
			t.Fatalf("GetStepEntryMarkerState(position=%d, kind=%s): %v", position, kind, err)
		}
		if found {
			t.Errorf("processOnEnter must not write a marker for ledger-owned kind %s at position %d — it is not this dispatcher's kind to claim", kind, position)
		}
	}
}

// TestProcessOnEnter_EveryLedgerOwnedKind_DoesNotWarn iterates
// stepentry.KnownKinds(stepentry.DispatcherLedger) directly instead of a
// hand-written literal, so a kind added to ownershipTable in the future is
// covered here automatically — mirroring
// entrydispatch_test.go's engine-side
// TestSessionShapedAndSessionIndependentKindsPartitionCompiledOnEnterKinds.
// TestProcessOnEnter_LedgerOwnedKinds_DoNotWarnAndWriteNoMarker (above) is a
// fixed-membership regression test for the four kinds that motivated this
// dispatch; this test is the drift guard that keeps a fifth or sixth
// ledger-owned kind from silently regressing to the AC-A6 warning path.
func TestProcessOnEnter_EveryLedgerOwnedKind_DoesNotWarn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-all-ledger", "session-all-ledger", "step-all-ledger")

	session, err := repo.GetTaskSession(ctx, "session-all-ledger")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)

	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build observed logger: %v", err)
	}
	svc.logger = log

	ledgerKinds := stepentry.KnownKinds(stepentry.DispatcherLedger)
	if len(ledgerKinds) == 0 {
		t.Fatalf("expected at least one ledger-owned kind to test against")
	}
	actions := make([]wfmodels.OnEnterAction, 0, len(ledgerKinds)+1)
	for _, kind := range ledgerKinds {
		actions = append(actions, wfmodels.OnEnterAction{Type: wfmodels.OnEnterActionType(kind)})
	}
	actions = append(actions, wfmodels.OnEnterAction{Type: wfmodels.OnEnterActionType("totally_bogus_action")})

	step := &wfmodels.WorkflowStep{
		ID:         "step-all-ledger",
		WorkflowID: "wf-all-ledger",
		Name:       "All Ledger-Owned Kinds Step",
		Events:     wfmodels.StepEvents{OnEnter: actions},
	}

	svc.processOnEnter(ctx, "task-all-ledger", session, step, "", 0, nil)

	var warnings []observer.LoggedEntry
	for _, e := range logs.All() {
		if e.Message == "processOnEnter: unrecognized on_enter action type" {
			warnings = append(warnings, e)
		}
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d WARNING(s), want exactly 1 (only the bogus type; every ledger-owned kind must not warn) (all entries: %+v)", len(warnings), logs.All())
	}
	if fields := warnings[0].ContextMap(); fields["action_type"] != "totally_bogus_action" {
		t.Errorf("warning action_type = %v, want %q", fields["action_type"], "totally_bogus_action")
	}
}
