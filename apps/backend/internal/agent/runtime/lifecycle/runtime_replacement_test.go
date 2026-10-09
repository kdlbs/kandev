package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestRuntimeReplacementUncertainSubmission(t *testing.T) {
	owner, oldBinding, _ := replaceTestRuntime(t)
	manager := newTestManager(t)
	manager.SetRuntimeOwner(owner)
	execution := &AgentExecution{
		ID: "old-execution", SessionID: "session-1", RuntimeName: executor.NameStandalone,
		Status: v1.AgentStatusRunning, runtimeEpoch: oldBinding.Epoch(), ACPSessionID: "native-session-1",
	}
	execution.setDeliverySubmissionID("submission-1")
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("add old execution: %v", err)
	}
	dispatched := false
	_, err := manager.PromptAgentWithDispatchCallbackAndSubmissionID(
		context.Background(), execution.ID, "new prompt", nil, true,
		func() { dispatched = true }, "submission-2",
	)
	var restoreRequired *RestoreRequiredError
	if !errors.As(err, &restoreRequired) {
		t.Fatalf("prompt error = %v, want RestoreRequiredError", err)
	}
	if !errors.Is(err, agentctl.ErrRuntimeLeaseRetired) {
		t.Fatalf("prompt error = %v, want retired-runtime evidence", err)
	}
	if dispatched {
		t.Fatal("prompt dispatch callback ran for a retired runtime")
	}
	if got := execution.deliverySubmissionIDSnapshot(); got != "submission-1" {
		t.Fatalf("uncertain submission identity = %q, want original submission-1", got)
	}
}

func TestRuntimeReplacementRetiredDisconnectSurfacesUncertainOutcome(t *testing.T) {
	owner, oldBinding, _ := replaceTestRuntime(t)
	manager, eventBus := createTestManagerWithTracking()
	manager.SetRuntimeOwner(owner)
	execution := createTestExecution("old-execution", "task-1", "session-1")
	execution.RuntimeName = executor.NameStandalone
	execution.runtimeEpoch = oldBinding.Epoch()
	execution.promptGeneration = 1
	execution.setDeliverySubmissionID("submission-1")
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("add old execution: %v", err)
	}

	manager.handleStreamDisconnectWithAttempt(
		execution,
		errors.Join(ErrUncertainPromptDelivery, agentctl.ErrRuntimeLeaseRetired),
		1,
		"",
		nil,
	)

	if execution.Status != v1.AgentStatusRunning {
		t.Fatalf("execution status = %q, want running until the uncertain submission is settled", execution.Status)
	}
	if execution.FailureCode != durableDeliveryUncertainFailureCode || execution.FailureDetails != "submission-1" {
		t.Fatalf("failure = (%q, %q), want durable delivery uncertainty for submission-1", execution.FailureCode, execution.FailureDetails)
	}
	if len(eventBus.PublishedEvents) == 0 || eventBus.PublishedEvents[len(eventBus.PublishedEvents)-1].Event.Type != events.AgentctlError {
		t.Fatalf("published events = %+v, want a terminal agentctl error", eventBus.PublishedEvents)
	}
	failedEventPublished := false
	for _, published := range eventBus.PublishedEvents {
		if published.Event.Type == events.AgentFailed {
			failedEventPublished = true
			break
		}
	}
	if failedEventPublished {
		t.Fatalf("published events = %+v, uncertain transport loss must not publish a terminal AgentFailed notification", eventBus.PublishedEvents)
	}
}

func TestRuntimeReplacementRetiredStreamDisconnectPersistsSubmissionRecovery(t *testing.T) {
	owner, oldBinding, _ := replaceTestRuntime(t)
	manager, eventBus := createTestManagerWithTracking()
	manager.SetRuntimeOwner(owner)
	execution := createTestExecution("old-execution", "task-1", "session-1")
	execution.RuntimeName = executor.NameStandalone
	execution.runtimeEpoch = oldBinding.Epoch()
	execution.promptGeneration = 1
	execution.DeliveryMode = DurableDeliveryV1
	execution.DeliveryStreamID = "stream-1"
	execution.DeliveryIncarnationID = "incarnation-1"
	execution.DeliveryHarnessGeneration = 3
	execution.setDeliverySubmissionID("submission-1")
	startupGeneration := execution.beginStartupAttempt()
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	manager.handleStreamDisconnectWithStartupGeneration(
		execution,
		errors.Join(ErrUncertainPromptDelivery, agentctl.ErrRuntimeLeaseRetired),
		1,
		startupGeneration,
	)

	for _, published := range eventBus.PublishedEvents {
		if published.Event == nil || published.Event.Type != events.AgentctlError {
			continue
		}
		payload, ok := published.Event.Data.(AgentctlEventPayload)
		if ok && payload.DeliveryRecoveryPhase == string(DeliveryReconciliationPhaseUncertain) {
			if payload.SessionID != execution.SessionID || payload.AgentExecutionID != execution.ID ||
				payload.DeliverySubmissionID != "submission-1" || payload.DeliveryStreamID != "stream-1" ||
				payload.DeliveryIncarnationID != "incarnation-1" || payload.DeliveryHarnessGeneration != 3 ||
				payload.PromptGeneration != 1 {
				t.Fatalf("retired stream recovery payload = %+v, want exact submission identity", payload)
			}
			return
		}
	}
	t.Fatalf("published events = %+v, want identity-bound uncertain delivery recovery", eventBus.PublishedEvents)
}

func TestRuntimeReplacementRecordsUncertaintyBeforePromptCompletionSignal(t *testing.T) {
	manager := newTestManager(t)
	execution := createTestExecution("old-execution", "task-1", "session-1")
	execution.RuntimeName = executor.NameStandalone
	execution.promptGeneration = 1
	execution.promptDoneCh = make(chan PromptCompletionSignal, 1)
	execution.setDeliverySubmissionID("submission-1")
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	observed := make(chan string, 1)
	execution.promptLifecycleMu.Lock()
	locked := true
	defer func() {
		if locked {
			execution.promptLifecycleMu.Unlock()
		}
	}()
	go func() {
		<-execution.promptDoneCh
		observed <- execution.FailureCode
	}()
	finished := make(chan struct{})
	go func() {
		manager.handleStreamDisconnectWithStartupGeneration(
			execution,
			errors.Join(ErrUncertainPromptDelivery, agentctl.ErrRuntimeLeaseRetired),
			1,
			execution.startupAttemptSnapshot(),
		)
		close(finished)
	}()

	select {
	case failureCode := <-observed:
		if failureCode != durableDeliveryUncertainFailureCode {
			t.Fatalf("failure code at prompt completion = %q, want uncertain delivery recorded before signal", failureCode)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime disconnect did not signal prompt completion")
	}
	execution.promptLifecycleMu.Unlock()
	locked = false
	<-finished
}

func TestRuntimeReplacementReconcilesActiveSessionsOnAvailabilityLoss(t *testing.T) {
	log := newTestLogger()
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	owner, _, _ := replaceTestRuntime(t, eventBus)
	runtimeSnapshot, ok := owner.Snapshot()
	if !ok {
		t.Fatal("runtime owner has no published snapshot")
	}
	manager := NewManager(newTestRegistry(), eventBus, nil, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	t.Cleanup(func() { _ = manager.Stop() })

	availabilityChanged := make(chan struct{}, 1)
	_, err := eventBus.Subscribe(events.AgentRuntimeAvailabilityChanged, func(_ context.Context, event *bus.Event) error {
		if snapshot, ok := event.Data.(agentctl.AvailabilitySnapshot); ok &&
			snapshot.Status == agentctl.AvailabilityStatusUnavailable && snapshot.RuntimeEpoch == runtimeSnapshot.RuntimeEpoch {
			availabilityChanged <- struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe to availability events: %v", err)
	}
	agentError := make(chan AgentctlEventPayload, 4)
	_, err = eventBus.Subscribe(events.AgentctlError, func(_ context.Context, event *bus.Event) error {
		if payload, ok := event.Data.(AgentctlEventPayload); ok {
			agentError <- payload
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe to agent error events: %v", err)
	}
	manager.SetRuntimeOwner(owner)

	active := createTestExecution("active-execution", "task-active", "session-active")
	active.RuntimeName = executor.NameStandalone
	active.runtimeEpoch = runtimeSnapshot.RuntimeEpoch
	active.promptGeneration = 1
	active.DeliveryMode = DurableDeliveryV1
	active.DeliveryIncarnationID = "incarnation-active"
	active.DeliveryHarnessGeneration = 1
	active.DeliveryStreamID = "stream-active"
	active.setDeliverySubmissionID("submission-active")
	if err := manager.executionStore.Add(active); err != nil {
		t.Fatalf("add active execution: %v", err)
	}
	idle := &AgentExecution{
		ID: "idle-execution", TaskID: "task-idle", SessionID: "session-idle",
		RuntimeName: executor.NameStandalone, Status: v1.AgentStatusReady,
		AgentCommand: "agent --acp", ACPSessionID: "native-idle-session", runtimeEpoch: runtimeSnapshot.RuntimeEpoch,
	}
	if err := manager.executionStore.Add(idle); err != nil {
		t.Fatalf("add idle execution: %v", err)
	}
	starting := createTestExecution("starting-execution", "task-starting", "session-starting")
	starting.RuntimeName = executor.NameStandalone
	starting.Status = v1.AgentStatusStarting
	starting.runtimeEpoch = runtimeSnapshot.RuntimeEpoch
	if err := manager.executionStore.Add(starting); err != nil {
		t.Fatalf("add starting execution: %v", err)
	}
	unresolved := createTestExecution("unresolved-execution", "task-unresolved", "session-unresolved")
	unresolved.RuntimeName = executor.NameStandalone
	unresolved.Status = v1.AgentStatusReady
	unresolved.promptGeneration = 2
	unresolved.runtimeEpoch = runtimeSnapshot.RuntimeEpoch
	unresolved.setDeliverySubmissionID("submission-unresolved")
	if err := manager.executionStore.Add(unresolved); err != nil {
		t.Fatalf("add unresolved execution: %v", err)
	}
	remote := createTestExecution("remote-execution", "task-remote", "session-remote")
	remote.RuntimeName = executor.NameDocker
	remote.runtimeEpoch = runtimeSnapshot.RuntimeEpoch
	if err := manager.executionStore.Add(remote); err != nil {
		t.Fatalf("add remote execution: %v", err)
	}
	if !owner.MarkUnavailableEpoch(runtimeSnapshot.RuntimeEpoch, agentctl.AvailabilityReasonAgentctlExited) {
		t.Fatal("failed to retire active runtime")
	}

	select {
	case <-availabilityChanged:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runtime-unavailable publication")
	}
	pendingErrors := map[string]struct{}{
		active.ID:     {},
		starting.ID:   {},
		unresolved.ID: {},
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for len(pendingErrors) > 0 {
		select {
		case payload := <-agentError:
			if payload.AgentExecutionID == active.ID {
				if payload.DeliveryRecoveryPhase != string(DeliveryReconciliationPhaseUncertain) {
					continue
				}
				if payload.SessionID != active.SessionID || payload.DeliverySubmissionID != "submission-active" ||
					payload.DeliveryStreamID != "stream-active" || payload.DeliveryIncarnationID != "incarnation-active" ||
					payload.DeliveryHarnessGeneration != 1 {
					t.Fatalf("runtime-loss recovery payload = %+v, want exact active delivery identity", payload)
				}
			}
			delete(pendingErrors, payload.AgentExecutionID)
		case <-deadline.C:
			t.Fatalf("runtime loss did not publish errors for executions %v", pendingErrors)
		}
	}
	type executionSnapshot struct {
		status        v1.AgentStatus
		failureCode   string
		failureDetail string
		acpSessionID  string
	}
	readExecution := func(execution *AgentExecution) executionSnapshot {
		t.Helper()
		var snapshot executionSnapshot
		if err := manager.executionStore.WithRLock(execution.ID, func(current *AgentExecution) {
			snapshot = executionSnapshot{
				status:        current.Status,
				failureCode:   current.FailureCode,
				failureDetail: current.FailureDetails,
				acpSessionID:  current.ACPSessionID,
			}
		}); err != nil {
			t.Fatalf("read execution %q: %v", execution.ID, err)
		}
		return snapshot
	}
	activeState := readExecution(active)
	if activeState.status != v1.AgentStatusRunning || activeState.failureCode != durableDeliveryUncertainFailureCode ||
		activeState.failureDetail != "submission-active" {
		t.Fatalf("active execution recovery = status %q, code %q, details %q; unresolved work must remain nonterminal", activeState.status, activeState.failureCode, activeState.failureDetail)
	}
	idleState := readExecution(idle)
	if idleState.status != v1.AgentStatusReady || idleState.acpSessionID != "native-idle-session" {
		t.Fatalf("idle native session changed during runtime loss: status=%q native_id=%q", idleState.status, idleState.acpSessionID)
	}
	startingState := readExecution(starting)
	if startingState.status != v1.AgentStatusFailed {
		t.Fatalf("starting execution status = %q, want failed after runtime loss", startingState.status)
	}
	unresolvedState := readExecution(unresolved)
	if unresolvedState.status != v1.AgentStatusReady || unresolvedState.failureCode != durableDeliveryUncertainFailureCode ||
		unresolvedState.failureDetail != "submission-unresolved" {
		t.Fatalf("unresolved execution recovery = status %q, code %q, details %q; delivery recovery must carry the unresolved admission state", unresolvedState.status, unresolvedState.failureCode, unresolvedState.failureDetail)
	}
	remoteState := readExecution(remote)
	if remoteState.status != v1.AgentStatusRunning {
		t.Fatalf("remote execution status = %q, want unchanged", remoteState.status)
	}
}

func TestRuntimeReplacementAdmissionEntrypoints(t *testing.T) {
	owner, oldBinding, _ := replaceTestRuntime(t)
	manager := newTestManager(t)
	manager.SetRuntimeOwner(owner)
	execution := &AgentExecution{
		ID: "old-execution", SessionID: "session-1", RuntimeName: executor.NameStandalone,
		Status: v1.AgentStatusReady, AgentCommand: "agent --acp", ACPSessionID: "native-session-1",
		runtimeEpoch: oldBinding.Epoch(),
	}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("add old execution: %v", err)
	}
	if !manager.isIdleSettledRetiredLocalExecution(execution) {
		t.Fatal("settled idle execution was not eligible for native restore")
	}
	dispatched := false
	_, err := manager.PromptAgentWithDispatchCallbackAndSubmissionID(
		context.Background(), execution.ID, "user prompt", nil, true,
		func() { dispatched = true }, "new-submission",
	)
	if !errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("idle stale prompt error = %v, want native-restore handoff", err)
	}
	if dispatched {
		t.Fatal("native restore handoff dispatched against the retired runtime")
	}
	if _, exists := manager.executionStore.GetBySessionID(execution.SessionID); exists {
		t.Fatal("native restore handoff retained the stale in-memory execution")
	}
}

func TestRuntimeReplacementIdleExecutionWithUnresolvedSubmissionStaysBlocked(t *testing.T) {
	owner, oldBinding, _ := replaceTestRuntime(t)
	manager := newTestManager(t)
	manager.SetRuntimeOwner(owner)
	execution := &AgentExecution{
		ID: "old-execution", SessionID: "session-1", RuntimeName: executor.NameStandalone,
		Status: v1.AgentStatusReady, AgentCommand: "agent --acp", runtimeEpoch: oldBinding.Epoch(),
	}
	execution.setDeliverySubmissionID("submission-uncertain")
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("add old execution: %v", err)
	}
	_, err := manager.PromptAgentWithDispatchCallbackAndSubmissionID(
		context.Background(), execution.ID, "user prompt", nil, true, nil, "new-submission",
	)
	var restoreRequired *RestoreRequiredError
	if !errors.As(err, &restoreRequired) {
		t.Fatalf("prompt error = %v, want blocked uncertain recovery", err)
	}
	if _, exists := manager.executionStore.GetBySessionID(execution.SessionID); !exists {
		t.Fatal("uncertain stale execution disappeared before session recovery")
	}
}

func TestRuntimeReplacementAllowsIndependentSession(t *testing.T) {
	owner, _, successor := replaceTestRuntime(t)
	standalone := NewStandaloneExecutor(nil, "", 0, newTestLogger())
	standalone.SetRuntimeOwner(owner)
	control, controlCtx, release, _, err := standalone.acquireControl(context.Background())
	if err != nil {
		t.Fatalf("acquire successor control: %v", err)
	}
	defer release()
	if _, err := control.CreateInstance(controlCtx, &agentctl.CreateInstanceRequest{ID: "independent-session"}); err != nil {
		t.Fatalf("create independent session after replacement: %v", err)
	}
	successor.mu.Lock()
	defer successor.mu.Unlock()
	if len(successor.createRequests) != 1 || successor.createRequests[0].ID != "independent-session" {
		t.Fatalf("successor create requests = %+v, want one independent session", successor.createRequests)
	}
}

func TestRuntimeReplacementStopUnknownOwner(t *testing.T) {
	oldServer := newStandaloneControlServer(t, true)
	owner := agentctl.NewRuntimeOwner(nil, newTestLogger(), "runtime-replacement-boot")
	t.Cleanup(owner.Stop)
	oldHost, oldPort := splitTestServerHostPort(t, oldServer.server)
	oldBinding, err := owner.PrepareBinding()
	if err != nil {
		t.Fatal(err)
	}
	if err := oldBinding.Configure(oldHost, oldPort, "old-secret", 10, nil); err != nil {
		t.Fatal(err)
	}
	if err := oldBinding.Commit(); err != nil {
		t.Fatal(err)
	}
	oldLease, err := owner.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	oldClient := oldLease.NewBoundInstanceClient(oldPort, newTestLogger())
	oldLease.Close()
	newServer := newStandaloneControlServer(t, true)
	newHost, newPort := splitTestServerHostPort(t, newServer.server)
	if !owner.MarkUnavailableEpoch(oldBinding.Epoch(), agentctl.AvailabilityReasonAgentctlExited) {
		t.Fatal("retire prior runtime")
	}
	successor, err := owner.PrepareBinding()
	if err != nil {
		t.Fatal(err)
	}
	if err := successor.Configure(newHost, newPort, "new-secret", 11, nil); err != nil {
		t.Fatal(err)
	}
	if err := successor.Commit(); err != nil {
		t.Fatal(err)
	}
	standalone := NewStandaloneExecutor(nil, "", 0, newTestLogger())
	standalone.SetRuntimeOwner(owner)
	err = standalone.StopInstance(context.Background(), &ExecutorInstance{
		StandaloneInstanceID: "old-instance", Client: oldClient,
	}, true)
	if !errors.Is(err, agentctl.ErrRuntimeStopUnconfirmed) || !errors.Is(err, agentctl.ErrRuntimeLeaseRetired) {
		t.Fatalf("stop error = %v, want unconfirmed stale-owner error", err)
	}
	newServer.mu.Lock()
	defer newServer.mu.Unlock()
	if len(newServer.deleted) != 0 {
		t.Fatalf("successor received stop for unknown old instance: %v", newServer.deleted)
	}
}

func TestRuntimeReplacementLaunchPreservesIdleNativeSession(t *testing.T) {
	owner, oldBinding, _ := replaceTestRuntime(t)
	manager := newTestManager(t)
	manager.SetRuntimeOwner(owner)
	runtimeRegistry := NewExecutorRegistry(manager.logger)
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		client:       newReadyAgentctlClient(t, manager.logger),
	}
	runtimeRegistry.Register(backend)
	manager.executorRegistry = runtimeRegistry
	manager.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{
		ProfileID: "profile-1", AgentName: "auggie",
	}}
	stale := &AgentExecution{
		ID: "old-execution", SessionID: "session-native-restore", TaskID: "task-1",
		AgentProfileID: "profile-1", RuntimeName: executor.NameStandalone,
		Status: v1.AgentStatusReady, AgentCommand: "agent --acp", ACPSessionID: "native-session-1",
		runtimeEpoch: oldBinding.Epoch(),
	}
	if err := manager.executionStore.Add(stale); err != nil {
		t.Fatalf("add stale execution: %v", err)
	}

	got, err := manager.Launch(context.Background(), &LaunchRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: stale.SessionID,
		AgentProfileID: "profile-1", ACPSessionID: stale.ACPSessionID,
		PreviousExecutionID: stale.ID, ExecutorType: string(models.ExecutorTypeLocal),
		WorkspacePath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("launch idle native restore: %v", err)
	}
	if got == stale || got.ID == stale.ID {
		t.Fatalf("launch returned retired execution %#v", got)
	}
	if got.ACPSessionID != "native-session-1" {
		t.Fatalf("native session identity = %q, want native-session-1", got.ACPSessionID)
	}
	if !got.isResumedSession {
		t.Fatal("replacement launch did not preserve native-resume intent")
	}
}

func replaceTestRuntime(
	t *testing.T,
	eventBuses ...bus.EventBus,
) (*agentctl.RuntimeOwner, *agentctl.RuntimeBindingCandidate, *standaloneControlServer) {
	t.Helper()
	oldServer := newStandaloneControlServer(t, true)
	newServer := newStandaloneControlServer(t, true)
	var eventBus bus.EventBus
	if len(eventBuses) > 0 {
		eventBus = eventBuses[0]
	}
	owner := agentctl.NewRuntimeOwner(eventBus, newTestLogger(), "runtime-replacement-boot")
	t.Cleanup(owner.Stop)
	oldHost, oldPort := splitTestServerHostPort(t, oldServer.server)
	oldBinding, err := owner.PrepareBinding()
	if err != nil {
		t.Fatal(err)
	}
	if err := oldBinding.Configure(oldHost, oldPort, "old-secret", 10, nil); err != nil {
		t.Fatal(err)
	}
	if err := oldBinding.Commit(); err != nil {
		t.Fatal(err)
	}
	newHost, newPort := splitTestServerHostPort(t, newServer.server)
	if !owner.MarkUnavailableEpoch(oldBinding.Epoch(), agentctl.AvailabilityReasonAgentctlExited) {
		t.Fatal("retire prior runtime")
	}
	successor, err := owner.PrepareBinding()
	if err != nil {
		t.Fatal(err)
	}
	if err := successor.Configure(newHost, newPort, "new-secret", 11, nil); err != nil {
		t.Fatal(err)
	}
	if err := successor.Commit(); err != nil {
		t.Fatal(err)
	}
	return owner, oldBinding, newServer
}
