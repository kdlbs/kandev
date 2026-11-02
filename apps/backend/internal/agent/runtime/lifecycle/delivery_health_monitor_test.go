package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func newDeliveryHealthTestManager() *StreamManager {
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	sm.deliveryHealthClock = func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) }
	sm.deliveryHealthProbeInterval = time.Second
	sm.deliveryHealthProbeTimeout = time.Second
	sm.deliveryHealthRecoveryCooldown = 10 * time.Second
	return sm
}

func deliveryFailureStatus(failure *types.DeliveryFailureSnapshot) *agentctl.StatusResponse {
	status := &agentctl.StatusResponse{AgentStatus: "running"}
	status.DeliveryHealth.State = deliveryFailedHealthState
	status.DeliveryHealth.Failure = failure
	return status
}

func correlatedFailure(execution *AgentExecution) *types.DeliveryFailureSnapshot {
	return &types.DeliveryFailureSnapshot{
		Reason: types.DeliveryFailureEventTooLarge, Revision: 1,
		SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
		HarnessGeneration: execution.DeliveryHarnessGeneration, StreamID: execution.DeliveryStreamID,
		SubmissionID: "submission-1", PromptGeneration: 1,
	}
}

func healthySilenceStatus() *agentctl.StatusResponse {
	status := &agentctl.StatusResponse{AgentStatus: "running"}
	status.DeliveryHealth.State = "healthy"
	return status
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
func TestDeliveryHealthFailureWithoutOutput(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	sm := newDeliveryHealthTestManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := sm.deliveryHealthNow()
	reconciledAt := time.Time{}
	probes, reconciles := 0, 0
	owner := deliveryHealthOwner{
		execution: execution,
		probe: func(context.Context) (*agentctl.StatusResponse, error) {
			probes++
			return deliveryFailureStatus(correlatedFailure(execution)), nil
		},
		current: func() bool { return true },
		reconcile: func(context.Context) DeliveryReconciliationResult {
			reconciles++
			reconciledAt = sm.deliveryHealthNow()
			cancel()
			return DeliveryReconciliationResult{Outcome: DeliveryReconciliationBlocked}
		},
		submission:       func() string { return "submission-1" },
		promptGeneration: func() uint64 { return 1 },
	}
	sm.deliveryHealthWait = func(context.Context, time.Duration) error {
		if probes > 5 {
			cancel()
		}
		return nil
	}
	sm.monitorDeliveryHealth(ctx, owner)
	if probes == 0 || reconciles != 1 {
		t.Fatalf("probes=%d reconciles=%d, want at least one probe and one reconciliation", probes, reconciles)
	}
	if reconciledAt.Sub(started) > deliveryReconciliationWindow {
		t.Fatalf("recovery started after %s, want within %s", reconciledAt.Sub(started), deliveryReconciliationWindow)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
func TestDeliveryHealthHealthySilence(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	sm := newDeliveryHealthTestManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	probes, reconciles := 0, 0
	owner := deliveryHealthOwner{
		execution: execution,
		probe: func(context.Context) (*agentctl.StatusResponse, error) {
			return healthySilenceStatus(), nil
		},
		current: func() bool { return true },
		reconcile: func(context.Context) DeliveryReconciliationResult {
			reconciles++
			return DeliveryReconciliationResult{}
		},
		submission:       func() string { return "submission-1" },
		promptGeneration: func() uint64 { return 1 },
	}
	sm.deliveryHealthWait = func(context.Context, time.Duration) error {
		probes++
		if probes >= 4 {
			cancel()
		}
		return nil
	}
	sm.monitorDeliveryHealth(ctx, owner)
	if probes < 4 || reconciles != 0 {
		t.Fatalf("probes=%d reconciles=%d; healthy silence must stay running", probes, reconciles)
	}
	if execution.promptGenerationSnapshot() != 1 {
		t.Fatal("healthy silence changed the prompt generation")
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.26
func TestDeliveryHealthStaleOwnerAndSiblingIsolation(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	sibling := newDeliveryReconciliationTestExecution()
	sibling.ID = "execution-2"
	sibling.DeliveryStreamID = "stream-2"
	sibling.setDeliverySubmissionID("submission-2")
	stale := []*types.DeliveryFailureSnapshot{
		{Reason: "event_too_large", SubmissionID: "old-submission", SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: execution.DeliveryHarnessGeneration, StreamID: execution.DeliveryStreamID, PromptGeneration: 1},
		{Reason: "event_too_large", SubmissionID: "submission-1", SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: execution.DeliveryHarnessGeneration, StreamID: "old-stream", PromptGeneration: 1},
		{Reason: "event_too_large", SubmissionID: "submission-1", SessionID: execution.SessionID, IncarnationID: "old-incarnation", HarnessGeneration: execution.DeliveryHarnessGeneration, StreamID: execution.DeliveryStreamID, PromptGeneration: 1},
		{Reason: "event_too_large", SubmissionID: "submission-1", SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: 99, StreamID: execution.DeliveryStreamID, PromptGeneration: 1},
		{Reason: "event_too_large", SubmissionID: "submission-1", SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: execution.DeliveryHarnessGeneration, StreamID: execution.DeliveryStreamID, PromptGeneration: 7},
	}
	for index, failure := range stale {
		reconciled := false
		owner := deliveryHealthOwner{
			execution: execution,
			probe: func(context.Context) (*agentctl.StatusResponse, error) {
				return deliveryFailureStatus(failure), nil
			},
			current: func() bool { return true },
			reconcile: func(context.Context) DeliveryReconciliationResult {
				reconciled = true
				return DeliveryReconciliationResult{}
			},
			submission:       func() string { return "submission-1" },
			promptGeneration: func() uint64 { return 1 },
		}
		if deliveryFailureCorrelates(owner, failure, "submission-1", 1) {
			t.Fatalf("stale failure %d correlated with the current owner", index)
		}
		if reconciled {
			t.Fatalf("stale failure %d changed state", index)
		}
	}

	siblingFailure := correlatedFailure(sibling)
	siblingFailure.SubmissionID = "submission-2"
	siblingOwner := deliveryHealthOwner{
		execution: sibling,
		probe: func(context.Context) (*agentctl.StatusResponse, error) {
			return deliveryFailureStatus(siblingFailure), nil
		},
		current: func() bool { return true },
		reconcile: func(context.Context) DeliveryReconciliationResult {
			return DeliveryReconciliationResult{Outcome: DeliveryReconciliationRunningAttached}
		},
		submission:       func() string { return "submission-2" },
		promptGeneration: func() uint64 { return 1 },
	}
	if !deliveryFailureCorrelates(siblingOwner, siblingFailure, "submission-2", 1) {
		t.Fatal("sibling failure did not correlate with its own owner")
	}
	if deliveryFailureCorrelates(siblingOwner, correlatedFailure(execution), "submission-2", 1) {
		t.Fatal("one execution's failure correlated with a sibling")
	}

	stopped := false
	owner := deliveryHealthOwner{
		execution: execution,
		probe: func(context.Context) (*agentctl.StatusResponse, error) {
			return deliveryFailureStatus(correlatedFailure(execution)), nil
		},
		current: func() bool { return false },
		reconcile: func(context.Context) DeliveryReconciliationResult {
			stopped = true
			return DeliveryReconciliationResult{}
		},
		submission:       func() string { return "submission-1" },
		promptGeneration: func() uint64 { return 1 },
	}
	sm := newDeliveryHealthTestManager()
	sm.monitorDeliveryHealth(context.Background(), owner)
	if stopped {
		t.Fatal("a replaced owner changed delivery state")
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.25
func TestDeliveryHealthRecoveryDoesNotResend(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	sm := newDeliveryHealthTestManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	sm.deliveryHealthClock = func() time.Time { return now }
	failure := correlatedFailure(execution)
	reconciles, probes := 0, 0
	owner := deliveryHealthOwner{
		execution: execution,
		probe: func(context.Context) (*agentctl.StatusResponse, error) {
			return deliveryFailureStatus(failure), nil
		},
		current: func() bool { return true },
		reconcile: func(context.Context) DeliveryReconciliationResult {
			reconciles++
			if currentDeliverySubmissionID(execution) != "submission-1" {
				t.Fatal("recovery changed the original submission identity")
			}
			if execution.promptGenerationSnapshot() != 1 {
				t.Fatal("recovery changed the original prompt generation")
			}
			return DeliveryReconciliationResult{Outcome: DeliveryReconciliationRunningAttached}
		},
		submission:       func() string { return "submission-1" },
		promptGeneration: func() uint64 { return 1 },
	}
	sm.deliveryHealthWait = func(context.Context, time.Duration) error {
		probes++
		switch probes {
		case 3:
			now = now.Add(5 * time.Second)
		case 4:
			failure = correlatedFailure(execution)
			failure.Revision = 2
		case 6:
			cancel()
		}
		return nil
	}
	sm.monitorDeliveryHealth(ctx, owner)
	if reconciles != 2 {
		t.Fatalf("reconciles=%d, want one per failure revision", reconciles)
	}
	if currentDeliverySubmissionID(execution) != "submission-1" || execution.promptGenerationSnapshot() != 1 {
		t.Fatal("recovery resent or replaced the original prompt")
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.26
func TestDeliveryHealthRecoveryKeepsIndependentFence(t *testing.T) {
	clock := newReconciliationTestClock()
	sm := newDeliveryReconciliationTestManager(clock)
	peer := newDeliveryReconciliationTestPeer()
	peer.status = func(context.Context, int) (*agentctl.StatusResponse, error) {
		status := &agentctl.StatusResponse{AgentStatus: "running"}
		status.DeliveryHealth.State = "guarded"
		return status, nil
	}
	execution := newDeliveryReconciliationTestExecution()
	result := sm.reconcileDeliveryWithPeer(context.Background(), execution, peer)
	if result.Outcome != DeliveryReconciliationBlocked {
		t.Fatalf("outcome = %q, want blocked", result.Outcome)
	}
	sibling := newDeliveryReconciliationTestExecution()
	sibling.ID = "execution-2"
	sibling.SessionID = "session-2"
	sibling.DeliveryStreamID = "stream-2"
	sibling.DeliveryIncarnationID = "incarnation-2"
	sibling.setDeliverySubmissionID("submission-2")
	if currentDeliverySubmissionID(sibling) != "submission-2" || sibling.promptGenerationSnapshot() != 1 {
		t.Fatal("one blocked recovery changed an unrelated session fence")
	}
	sm.Wait()
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
func TestDeliveryHealthMonitorSurvivesSuccessfulStreamSetup(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	execution.ID = "execution-monitor"
	execution.DeliveryStreamID = "stream-monitor"
	execution.DeliveryIncarnationID = "incarnation-monitor"
	execution.DeliveryHarnessGeneration = 4
	execution.setDeliverySubmissionID("submission-monitor")
	execution.DeliveryMode = DurableDeliveryV1
	failure := &types.DeliveryFailureSnapshot{
		Reason: types.DeliveryFailureEventTooLarge, Revision: 1,
		SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
		HarnessGeneration: execution.DeliveryHarnessGeneration, StreamID: execution.DeliveryStreamID,
		SubmissionID: "submission-monitor", PromptGeneration: 1,
	}
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			probes.Add(1)
			status := &agentctl.StatusResponse{AgentStatus: "running"}
			status.DeliveryHealth.State = deliveryFailedHealthState
			status.DeliveryHealth.Failure = failure
			_ = json.NewEncoder(w).Encode(status)
		case "/api/v1/runtime/storage":
			_ = json.NewEncoder(w).Encode(streams.RuntimeStorageSnapshot{
				Version: streams.RuntimeStorageSnapshotVersion, StorageKind: streams.RuntimeStorageKindWorkspace,
				Filesystem: &streams.RuntimeStorageFilesystem{TotalBytes: 1000, AvailableBytes: 900},
			})
		case "/api/v1/agent/delivery":
			_ = json.NewEncoder(w).Encode(agentctl.DeliveryStatus{
				StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
				SessionID:         execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
				HarnessGeneration: execution.DeliveryHarnessGeneration, StreamID: execution.DeliveryStreamID,
			})
		case "/api/v1/agent/submissions/submission-monitor":
			_ = json.NewEncoder(w).Encode(journal.Submission{
				ID: "submission-monitor", SessionID: execution.SessionID,
				IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: execution.DeliveryHarnessGeneration,
				StreamID: execution.DeliveryStreamID, State: journal.SubmissionDispatching,
			})
		case "/api/v1/agent/stream":
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	client := agentctl.NewClient(u.Hostname(), port, newTestLogger())
	t.Cleanup(client.Close)
	execution.agentctl = client
	phases := make(chan DeliveryReconciliationPhase, 4)
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	manager.onDeliveryReconciliationPhase = func(_ *AgentExecution, _ DeliveryReconciliationIdentity, phase DeliveryReconciliationPhase) {
		phases <- phase
	}
	manager.deliveryHealthProbeInterval = 10 * time.Millisecond
	manager.deliveryHealthProbeTimeout = 100 * time.Millisecond
	manager.deliveryHealthRecoveryCooldown = time.Hour
	t.Cleanup(manager.Wait)
	manager.connectUpdatesStream(execution, nil)

	seeReconnecting, seeUncertain := false, false
	deadline := time.After(5 * time.Second)
	for !seeReconnecting || !seeUncertain {
		select {
		case phase := <-phases:
			seeReconnecting = seeReconnecting || phase == DeliveryReconciliationPhaseReconnecting
			seeUncertain = seeUncertain || phase == DeliveryReconciliationPhaseUncertain
		case <-deadline:
			t.Fatalf("phases after successful setup: reconnecting=%t uncertain=%t probes=%d", seeReconnecting, seeUncertain, probes.Load())
		}
	}
	if probes.Load() == 0 {
		t.Fatal("monitor never probed after successful stream setup")
	}
	if client.HasAgentStream() == false {
		t.Fatal("the test did not establish a live stream")
	}
	firstCount := probes.Load()
	deadline = time.After(2 * time.Second)
	for probes.Load() == firstCount {
		select {
		case <-deadline:
			t.Fatal("monitor stopped probing after successful stream setup")
		case <-time.After(10 * time.Millisecond):
		}
	}

	manager.Wait()
	time.Sleep(100 * time.Millisecond)
	afterWait := probes.Load()
	time.Sleep(100 * time.Millisecond)
	if probes.Load() != afterWait {
		t.Fatalf("monitor kept probing after shutdown: %d -> %d", afterWait, probes.Load())
	}
	if _, loaded := manager.deliveryAttachments.Load(execution.ID); loaded {
		t.Fatal("monitor attachment survived shutdown")
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
func TestDeliveryHealthMonitorPublishesRecoveryAfterRealTerminal(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	sm := newDeliveryReconciliationTestManager(newReconciliationTestClock())
	peer := newDeliveryReconciliationTestPeer()
	peer.streamOpen = true
	failed := true
	peer.status = func(context.Context, int) (*agentctl.StatusResponse, error) {
		status := &agentctl.StatusResponse{AgentStatus: "running"}
		if failed {
			status.DeliveryHealth.State = deliveryFailedHealthState
			status.DeliveryHealth.Failure = correlatedFailure(execution)
		} else {
			status.DeliveryHealth.State = "healthy"
		}
		return status, nil
	}
	phases := make(chan DeliveryReconciliationPhase, 8)
	sm.onDeliveryReconciliationPhase = func(_ *AgentExecution, _ DeliveryReconciliationIdentity, phase DeliveryReconciliationPhase) {
		phases <- phase
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reconciles := 0
	owner := deliveryHealthOwner{
		execution: execution,
		probe:     func(ctx context.Context) (*agentctl.StatusResponse, error) { return peer.GetStatus(ctx) },
		current:   func() bool { return true },
		reconcile: func(ctx context.Context) DeliveryReconciliationResult {
			reconciles++
			result := sm.reconcileDeliveryWithPeer(ctx, execution, peer)
			if failed {
				failed = false
			}
			return result
		},
		submission:       func() string { return "submission-1" },
		promptGeneration: func() uint64 { return 1 },
	}
	calls := 0
	sm.deliveryHealthWait = func(context.Context, time.Duration) error {
		calls++
		if calls > 6 {
			cancel()
		}
		return nil
	}
	sm.deliveryHealthProbeInterval = time.Millisecond
	sm.monitorDeliveryHealth(ctx, owner)
	if reconciles != 2 {
		t.Fatalf("reconciles = %d, want one failure and one recovery reconciliation", reconciles)
	}
	seen := map[DeliveryReconciliationPhase]bool{}
	for len(phases) > 0 {
		seen[<-phases] = true
	}
	if !seen[DeliveryReconciliationPhaseReconnecting] || !seen[DeliveryReconciliationPhaseUncertain] ||
		!seen[DeliveryReconciliationPhaseRecovered] {
		t.Fatalf("published phases = %v, want reconnecting, uncertain, recovered", seen)
	}
	sm.Wait()
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
func TestDeliveryHealthRecoveryWithinTenSeconds(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	sm := newDeliveryHealthTestManager()
	start := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	now := start
	sm.deliveryHealthClock = func() time.Time { return now }
	sm.deliveryHealthProbeInterval = 2 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	probes := 0
	var failureAt, reconciledAt time.Time
	owner := deliveryHealthOwner{
		execution: execution,
		probe: func(context.Context) (*agentctl.StatusResponse, error) {
			probes++
			if probes < 3 {
				return healthySilenceStatus(), nil
			}
			failureAt = now
			return deliveryFailureStatus(correlatedFailure(execution)), nil
		},
		current: func() bool { return true },
		reconcile: func(context.Context) DeliveryReconciliationResult {
			reconciledAt = now
			cancel()
			return DeliveryReconciliationResult{Outcome: DeliveryReconciliationBlocked}
		},
		submission:       func() string { return "submission-1" },
		promptGeneration: func() uint64 { return 1 },
	}
	sm.deliveryHealthWait = func(_ context.Context, interval time.Duration) error {
		now = now.Add(interval)
		return nil
	}
	sm.monitorDeliveryHealth(ctx, owner)
	if reconciledAt.IsZero() || failureAt.IsZero() {
		t.Fatal("the failure was never reconciled")
	}
	if reconciledAt.Sub(failureAt) > deliveryReconciliationWindow {
		t.Fatalf("recovery started after %s, want within %s", reconciledAt.Sub(failureAt), deliveryReconciliationWindow)
	}
	if reconciledAt.Sub(start) > deliveryReconciliationWindow+time.Second {
		t.Fatalf("recovery took %s from monitor start", reconciledAt.Sub(start))
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
func TestDeliveryHealthStateOnlyFailureUsesReconciler(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	sm := newDeliveryReconciliationTestManager(newReconciliationTestClock())
	peer := newDeliveryReconciliationTestPeer()
	peer.streamOpen = true
	peer.status = func(context.Context, int) (*agentctl.StatusResponse, error) {
		status := &agentctl.StatusResponse{AgentStatus: "running"}
		status.DeliveryHealth.State = "storage_error"
		return status, nil
	}
	phases := make(chan DeliveryReconciliationPhase, 4)
	sm.onDeliveryReconciliationPhase = func(_ *AgentExecution, _ DeliveryReconciliationIdentity, phase DeliveryReconciliationPhase) {
		phases <- phase
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reconciles := 0
	owner := deliveryHealthOwner{
		execution: execution,
		probe:     func(ctx context.Context) (*agentctl.StatusResponse, error) { return peer.GetStatus(ctx) },
		current:   func() bool { return true },
		reconcile: func(ctx context.Context) DeliveryReconciliationResult {
			reconciles++
			return sm.reconcileDeliveryWithPeer(ctx, execution, peer)
		},
		submission:       func() string { return "submission-1" },
		promptGeneration: func() uint64 { return 1 },
	}
	waits := 0
	sm.deliveryHealthWait = func(context.Context, time.Duration) error {
		waits++
		if waits >= 2 {
			cancel()
		}
		return nil
	}
	sm.monitorDeliveryHealth(ctx, owner)
	if reconciles != 1 {
		t.Fatalf("reconciles = %d, want one state-only recovery probe", reconciles)
	}
	seen := map[DeliveryReconciliationPhase]bool{}
	for len(phases) > 0 {
		seen[<-phases] = true
	}
	if !seen[DeliveryReconciliationPhaseReconnecting] || !seen[DeliveryReconciliationPhaseUncertain] ||
		seen[DeliveryReconciliationPhaseRecovered] {
		t.Fatalf("published phases = %v, want reconnecting and uncertain", seen)
	}
	sm.Wait()
}

func TestDeliveryStoragePressureStatusProjectsQuotaBelowUsageThreshold(t *testing.T) {
	execution := &AgentExecution{
		ID: "execution", TaskID: "task", SessionID: "session",
		DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 3,
	}
	failure := &types.DeliveryFailureSnapshot{
		Reason: types.DeliveryFailureStorageFull, Revision: 1,
		SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
		HarnessGeneration: execution.DeliveryHarnessGeneration, StreamID: execution.DeliveryStreamID,
		SubmissionID: "submission", PromptGeneration: 2,
		Storage: &streams.DeliveryStorageDiagnostic{
			Version: streams.DeliveryStorageDiagnosticVersion,
			Reason:  streams.DeliveryStorageReasonDiskQuotaExceeded,
		},
	}
	status := &agentctl.StatusResponse{AgentStatus: "running"}
	status.DeliveryHealth.State = "delivery_failed"
	status.DeliveryHealth.Failure = failure
	status.DeliveryHealth.Capacity.DiskAvailableBytes = 10
	status.DeliveryHealth.Capacity.DiskCapacityKnown = true
	owner := deliveryHealthOwner{
		execution:        execution,
		submission:       func() string { return "submission" },
		promptGeneration: func() uint64 { return 2 },
	}
	pressure := runtimeStoragePressureFromStatus(owner, status, "submission", 2)
	if !pressure.WritePressure || pressure.DeliveryState != "delivery_failed" {
		t.Fatalf("pressure = %+v", pressure)
	}
	if pressure.FailureStorage == nil || pressure.FailureStorage.Reason != models.AgentStorageReasonDiskQuotaExceeded {
		t.Fatalf("failure storage = %+v", pressure.FailureStorage)
	}
	uncorrelated := runtimeStoragePressureFromStatus(owner, status, "other-submission", 2)
	if uncorrelated.FailureStorage != nil {
		t.Fatalf("uncorrelated failure attached storage: %+v", uncorrelated.FailureStorage)
	}
}
