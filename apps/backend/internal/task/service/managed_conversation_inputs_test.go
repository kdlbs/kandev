package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAgentConversationServiceExposesManagedInputOperations(t *testing.T) {
	serviceType := reflect.TypeOf((*AgentConversationService)(nil))
	for _, method := range []string{
		"EnqueueManagedInput",
		"GetManagedInput",
		"ListManagedInputs",
		"CancelManagedInput",
		"DispatchManagedInput",
	} {
		_, exists := serviceType.MethodByName(method)
		require.Truef(t, exists, "AgentConversationService must implement %s", method)
	}
}

type managedInputTestTarget struct {
	service    *AgentConversationService
	deps       acTestDeps
	storage    messagequeue.ManagedInputStorage
	queueRepo  messagequeue.Repository
	identity   messagequeue.QueueSessionIdentity
	descriptor pluginsdk.ManagedAgentConversationDescriptor
	notifies   chan struct{}
}

func newManagedInputTestTarget(t *testing.T) managedInputTestTarget {
	t.Helper()
	ctx := context.Background()
	svc, deps := newACTestService()
	descriptor, _, err := svc.EnsureManaged(ctx, "plugin-coordinator", "install-1", pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "ws-1", InstanceKey: "coordinator", AgentProfileID: "profile-1",
		ApprovalRevision: 3, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, "ensure-op", "ensure-digest")
	require.NoError(t, err)

	identity := messagequeue.QueueSessionIdentity{
		TaskID: descriptor.TaskID, SessionID: descriptor.SessionID, SessionIncarnationID: "incarnation-1",
	}
	deps.sess.mu.Lock()
	deps.sess.sessions[descriptor.SessionID].QueueIncarnationID = identity.SessionIncarnationID
	require.Equal(t, identity.TaskID, deps.sess.sessions[descriptor.SessionID].TaskID)
	require.Equal(t, identity.SessionIncarnationID, deps.sess.sessions[descriptor.SessionID].QueueIncarnationID)
	deps.sess.mu.Unlock()
	identityResolver := func(_ context.Context, taskID, sessionID string) (messagequeue.QueueSessionIdentity, error) {
		if taskID != identity.TaskID || sessionID != identity.SessionID {
			return messagequeue.QueueSessionIdentity{}, messagequeue.ErrSessionIdentityMismatch
		}
		return identity, nil
	}
	repository := messagequeue.NewMemoryRepositoryWithAuthority(identityResolver)
	storage, ok := repository.(messagequeue.ManagedInputStorage)
	require.True(t, ok)
	notifies := make(chan struct{}, 8)
	target := managedInputTestTarget{
		service: svc, deps: deps, storage: storage, queueRepo: repository,
		identity: identity, descriptor: descriptor, notifies: notifies,
	}
	svc.SetManagedInputStorage(storage, repository.ResolveSessionIdentity, func() int { return 10 })
	svc.SetManagedInputNotifier(func(_ context.Context, taskID, sessionID string) {
		if taskID == identity.TaskID && sessionID == identity.SessionID {
			notifies <- struct{}{}
		}
	})
	return target
}

func managedInputEnqueueRequest(payload, occurrence string) pluginsdk.ManagedAgentInputEnqueue {
	return pluginsdk.ManagedAgentInputEnqueue{
		RequestID: "request-" + occurrence, IdempotencyKey: "idem-" + occurrence,
		WorkspaceID: "ws-1", InstanceKey: "coordinator", ExpectedConversationRevision: 1,
		ApprovalRevision: 3, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		OccurrenceKey: occurrence, Origin: pluginsdk.ManagedAgentInputHuman, Payload: payload,
	}
}

func awaitManagedInputNotifications(t *testing.T, notifications <-chan struct{}, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-notifications:
		case <-time.After(time.Second):
			t.Fatalf("received fewer than %d managed input queue notifications", count)
		}
	}
}

func TestManagedInputEnqueueReplayConflictAndBoundedList(t *testing.T) {
	target := newManagedInputTestTarget(t)
	ctx := context.Background()
	request := managedInputEnqueueRequest("Review the latest change", "human-message-1")

	first, replayed, err := target.service.EnqueueManagedInput(ctx, "install-1", "input-1", request, "op-1", "digest-1")
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, pluginsdk.ManagedAgentInputAccepted, first.State)
	require.Equal(t, uint64(1), first.Sequence)
	require.Equal(t, "Review the latest change", first.Payload)
	require.Equal(t, "input-1", first.QueueEntryID)

	second, replayed, err := target.service.EnqueueManagedInput(ctx, "install-1", "input-1", request, "op-1", "digest-1")
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, first.HostInputID, second.HostInputID)
	require.Equal(t, first.Sequence, second.Sequence)
	awaitManagedInputNotifications(t, target.notifies, 2)

	secondRequest := managedInputEnqueueRequest("Check the next change", "human-message-2")
	secondReceipt, replayed, err := target.service.EnqueueManagedInput(ctx, "install-1", "input-2", secondRequest, "op-2", "digest-2")
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, first.Sequence+1, secondReceipt.Sequence)

	changed := request
	changed.Payload = "Run a different review"
	_, _, err = target.service.EnqueueManagedInput(ctx, "install-1", "input-1", changed, "op-2", "digest-2")
	require.Equal(t, codes.Aborted, status.Code(err), "one occurrence cannot identify different payloads")

	page, err := target.service.ListManagedInputs(ctx, "install-1", pluginsdk.ManagedAgentInputListQuery{
		WorkspaceID: "ws-1", InstanceKey: "coordinator", Limit: 1,
		ApprovalRevision: 3, ManifestDigest: request.ManifestDigest,
	})
	require.NoError(t, err)
	require.Len(t, page.Inputs, 1)
	require.True(t, page.HasMore)
	require.Equal(t, first.Sequence, page.NextSequenceCursor)
	page, err = target.service.ListManagedInputs(ctx, "install-1", pluginsdk.ManagedAgentInputListQuery{
		WorkspaceID: "ws-1", InstanceKey: "coordinator", SequenceCursor: first.Sequence, Limit: 1,
		ApprovalRevision: 3, ManifestDigest: request.ManifestDigest,
	})
	require.NoError(t, err)
	require.Len(t, page.Inputs, 1)
	require.False(t, page.HasMore)
	require.Equal(t, secondReceipt.HostInputID, page.Inputs[0].HostInputID)
	require.Equal(t, secondReceipt.Sequence, page.NextSequenceCursor)

	got, err := target.service.GetManagedInput(ctx, "install-1", pluginsdk.ManagedAgentInputQuery{
		WorkspaceID: "ws-1", InstanceKey: "coordinator", HostInputID: "input-1",
		ApprovalRevision: 3, ManifestDigest: request.ManifestDigest,
	})
	require.NoError(t, err)
	require.Equal(t, first.Payload, got.Payload)
}

func TestManagedInputEnqueueReturnsReceiptBeforeQueueWakeCompletes(t *testing.T) {
	target := newManagedInputTestTarget(t)
	notifierEntered := make(chan struct{})
	releaseNotifier := make(chan struct{})
	defer close(releaseNotifier)
	target.service.SetManagedInputNotifier(func(context.Context, string, string) {
		close(notifierEntered)
		<-releaseNotifier
	})

	type enqueueResult struct {
		receipt pluginsdk.ManagedAgentInputReceipt
		err     error
	}
	result := make(chan enqueueResult, 1)
	go func() {
		receipt, _, err := target.service.EnqueueManagedInput(
			context.Background(), "install-1", "input-fast-receipt",
			managedInputEnqueueRequest("run the daily check", "automation-fast-receipt"),
			"op-fast-receipt", "digest-fast-receipt",
		)
		result <- enqueueResult{receipt: receipt, err: err}
	}()
	select {
	case <-notifierEntered:
	case <-time.After(time.Second):
		t.Fatal("accepted input did not wake the queue")
	}
	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.Equal(t, pluginsdk.ManagedAgentInputAccepted, got.receipt.State)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("durable input receipt waited for queue wake completion")
	}
}

func TestManagedInputRejectsInvalidCoalescingAndStaleAuthority(t *testing.T) {
	target := newManagedInputTestTarget(t)
	ctx := context.Background()
	request := managedInputEnqueueRequest("hello", "human-coalescing")
	request.CoalesceKey = "daily"
	_, _, err := target.service.EnqueueManagedInput(ctx, "install-1", "input-human", request, "op", "digest")
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	request = managedInputEnqueueRequest("hello", "human-stale-revision")
	request.ExpectedConversationRevision++
	_, _, err = target.service.EnqueueManagedInput(ctx, "install-1", "input-stale", request, "op", "digest")
	require.Equal(t, codes.Aborted, status.Code(err))

	request = managedInputEnqueueRequest("hello", "human-stale-approval")
	request.ApprovalRevision++
	_, _, err = target.service.EnqueueManagedInput(ctx, "install-1", "input-approval", request, "op", "digest")
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestManagedInputAdmissionEnforcesConfiguredQueueLimit(t *testing.T) {
	target := newManagedInputTestTarget(t)
	target.service.SetManagedInputStorage(target.storage, target.queueRepo.ResolveSessionIdentity, func() int { return 1 })
	ctx := context.Background()
	first := managedInputEnqueueRequest("first", "capacity-1")
	_, _, err := target.service.EnqueueManagedInput(ctx, "install-1", "capacity-input-1", first, "op-1", "digest-1")
	require.NoError(t, err)
	second := managedInputEnqueueRequest("second", "capacity-2")
	_, _, err = target.service.EnqueueManagedInput(ctx, "install-1", "capacity-input-2", second, "op-2", "digest-2")
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	queued, err := target.queueRepo.ListBySession(ctx, target.identity.SessionID)
	require.NoError(t, err)
	require.Len(t, queued, 1)
}

func TestManagedPeriodicInputCoalescesOnlyPendingPeriodicWork(t *testing.T) {
	target := newManagedInputTestTarget(t)
	ctx := context.Background()
	firstRequest := managedInputEnqueueRequest("inspect current sprint", "periodic-1")
	firstRequest.Origin = pluginsdk.ManagedAgentInputPeriodic
	firstRequest.CoalesceKey = "sprint-inspection"
	first, _, err := target.service.EnqueueManagedInput(ctx, "install-1", "periodic-input-1", firstRequest, "op-periodic-1", "digest-periodic-1")
	require.NoError(t, err)
	require.Equal(t, pluginsdk.ManagedAgentInputAccepted, first.State)

	secondRequest := managedInputEnqueueRequest("inspect current sprint again", "periodic-2")
	secondRequest.Origin = pluginsdk.ManagedAgentInputPeriodic
	secondRequest.CoalesceKey = "sprint-inspection"
	second, _, err := target.service.EnqueueManagedInput(ctx, "install-1", "periodic-input-2", secondRequest, "op-periodic-2", "digest-periodic-2")
	require.NoError(t, err)
	require.Equal(t, pluginsdk.ManagedAgentInputAccepted, second.State)

	superseded, err := target.service.GetManagedInput(ctx, "install-1", pluginsdk.ManagedAgentInputQuery{
		WorkspaceID: "ws-1", InstanceKey: "coordinator", HostInputID: first.HostInputID,
		ApprovalRevision: 3, ManifestDigest: firstRequest.ManifestDigest,
	})
	require.NoError(t, err)
	require.Equal(t, pluginsdk.ManagedAgentInputCancelled, superseded.State)
	require.Equal(t, second.HostInputID, superseded.SupersededBy)

	queued, err := target.queueRepo.ListBySession(ctx, target.identity.SessionID)
	require.NoError(t, err)
	require.Len(t, queued, 1, "the shared FIFO contains only the newest pending periodic item")
	require.Equal(t, second.HostInputID, queued[0].ID)
}

func TestManagedInputPauseRetainsAcceptedWorkAndUnpauseWakesQueue(t *testing.T) {
	target := newManagedInputTestTarget(t)
	ctx := context.Background()
	paused, err := target.service.SetManagedPaused(ctx, "install-1", "ws-1", "coordinator",
		target.descriptor.Revision, true, "pause-op", "pause-digest")
	require.NoError(t, err)
	require.True(t, paused.DesiredPaused)

	request := managedInputEnqueueRequest("wait until resumed", "paused-input")
	request.ExpectedConversationRevision = paused.Revision
	accepted, _, err := target.service.EnqueueManagedInput(ctx, "install-1", "paused-input-id", request, "enqueue-op", "enqueue-digest")
	require.NoError(t, err)
	require.Equal(t, pluginsdk.ManagedAgentInputAccepted, accepted.State)
	awaitManagedInputNotifications(t, target.notifies, 1)

	unpaused, err := target.service.SetManagedPaused(ctx, "install-1", "ws-1", "coordinator",
		paused.Revision, false, "unpause-op", "unpause-digest")
	require.NoError(t, err)
	require.False(t, unpaused.DesiredPaused)
	require.Equal(t, paused.Revision+1, unpaused.Revision)
	awaitManagedInputNotifications(t, target.notifies, 1)
}

func TestManagedInputCancelAcceptedAndRequiresExactStopConfirmation(t *testing.T) {
	target := newManagedInputTestTarget(t)
	ctx := context.Background()
	request := managedInputEnqueueRequest("cancel before start", "cancel-accepted")
	accepted, _, err := target.service.EnqueueManagedInput(ctx, "install-1", "input-accepted", request, "enqueue-op", "enqueue-digest")
	require.NoError(t, err)
	acceptedCancel := managedInputCancelRequest(accepted.HostInputID, "")
	cancelled, changed, err := target.service.CancelManagedInput(ctx, "install-1", acceptedCancel, "cancel-op", "cancel-digest")
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, pluginsdk.ManagedAgentInputCancelled, cancelled.State)

	runningRequest := managedInputEnqueueRequest("cancel while running", "cancel-running")
	running, _, err := target.service.EnqueueManagedInput(ctx, "install-1", "input-running", runningRequest, "enqueue-op-2", "enqueue-digest-2")
	require.NoError(t, err)
	_, _, err = target.storage.MarkManagedInputRunning(ctx, target.identity,
		running.HostInputID, "turn-1", "execution-1")
	require.NoError(t, err)
	target.deps.sess.mu.Lock()
	target.deps.sess.sessions[target.descriptor.SessionID].State = models.TaskSessionStateRunning
	target.deps.sess.sessions[target.descriptor.SessionID].AgentExecutionID = "execution-1"
	target.deps.sess.mu.Unlock()

	var stopCalls int
	target.service.SetManagedInputExecutionStopper(func(_ context.Context, taskID, sessionID, executionID string) (bool, error) {
		stopCalls++
		require.Equal(t, target.identity.TaskID, taskID)
		require.Equal(t, target.identity.SessionID, sessionID)
		require.Equal(t, "execution-1", executionID)
		return false, nil
	})
	runningCancel := managedInputCancelRequest(running.HostInputID, "execution-1")
	stillRunning, changed, err := target.service.CancelManagedInput(ctx, "install-1", runningCancel, "cancel-op-2", "cancel-digest-2")
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, pluginsdk.ManagedAgentInputRunning, stillRunning.State)
	require.Equal(t, 1, stopCalls)

	target.service.SetManagedInputExecutionStopper(func(context.Context, string, string, string) (bool, error) {
		return true, nil
	})
	stopped, changed, err := target.service.CancelManagedInput(ctx, "install-1", runningCancel, "cancel-op-3", "cancel-digest-3")
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, pluginsdk.ManagedAgentInputCancelled, stopped.State)
	require.Equal(t, "execution-1", stopped.ExecutionID)

	_, _, err = target.service.CancelManagedInput(ctx, "install-1", managedInputCancelRequest("input-accepted", "execution-stale"), "cancel-op-4", "cancel-digest-4")
	require.Equal(t, codes.Aborted, status.Code(err))
}

func managedInputCancelRequest(inputID, executionID string) pluginsdk.ManagedAgentInputCancel {
	return pluginsdk.ManagedAgentInputCancel{
		RequestID: "cancel-request-" + inputID, IdempotencyKey: "cancel-idem-" + inputID,
		WorkspaceID: "ws-1", InstanceKey: "coordinator", HostInputID: inputID,
		ExpectedConversationRevision: 1, ExpectedExecutionID: executionID,
		ApprovalRevision: 3, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
}

type managedInputImmediateDispatcher struct {
	base   *acFakeDispatcher
	status pluginsdk.ManagedAgentDispatchStatus
	err    error
}

func (d *managedInputImmediateDispatcher) Deliver(ctx context.Context, taskID string, session *models.TaskSession, text, source, idempotencyID string) (string, error) {
	return d.base.Deliver(ctx, taskID, session, text, source, idempotencyID)
}

func (d *managedInputImmediateDispatcher) DispatchImmediate(ctx context.Context, taskID string, session *models.TaskSession, text, source, idempotencyID string) (pluginsdk.ManagedAgentDispatchStatus, error) {
	if d.err != nil {
		return "", d.err
	}
	if d.status != "" {
		return d.status, nil
	}
	result, err := d.base.Deliver(ctx, taskID, session, text, source, idempotencyID)
	return pluginsdk.ManagedAgentDispatchStatus(result), err
}

func TestManagedInputImmediateDispatchNeverQueuesAndReplaysStatus(t *testing.T) {
	target := newManagedInputTestTarget(t)
	dispatcher := &managedInputImmediateDispatcher{base: target.deps.dispatcher}
	target.service.SetDispatcher(dispatcher)
	request := pluginsdk.ManagedAgentConversationDispatch{
		RequestID: "dispatch-request", IdempotencyKey: "dispatch-idem",
		WorkspaceID: "ws-1", InstanceKey: "coordinator", ExpectedConversationRevision: 1,
		ApprovalRevision: 3, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		OccurrenceKey: "immediate-1", Origin: pluginsdk.ManagedAgentInputAutomation, Payload: "Run now",
	}

	dispatcher.status = pluginsdk.ManagedAgentDispatchBusy
	gotStatus, gotDescriptor, err := target.service.DispatchManagedInput(context.Background(), "install-1", request, "dispatch-op", "dispatch-digest")
	require.NoError(t, err)
	require.Equal(t, pluginsdk.ManagedAgentDispatchBusy, gotStatus)
	require.Equal(t, target.descriptor.TaskID, gotDescriptor.TaskID)
	require.Zero(t, target.deps.dispatcher.callCount(), "a busy dispatch must not reach the delivery path")

	dispatcher.status = ""
	gotStatus, _, err = target.service.DispatchManagedInput(context.Background(), "install-1", request, "dispatch-op", "dispatch-digest")
	require.NoError(t, err)
	require.Equal(t, pluginsdk.ManagedAgentDispatchStarted, gotStatus)
	require.Equal(t, 1, target.deps.dispatcher.callCount())
	call := target.deps.dispatcher.lastCall()
	require.Contains(t, call.text, "Run now")

	gotStatus, _, err = target.service.DispatchManagedInput(context.Background(), "install-1", request, "dispatch-op", "dispatch-digest")
	require.NoError(t, err)
	require.Equal(t, pluginsdk.ManagedAgentDispatchStarted, gotStatus)
	require.Equal(t, 1, target.deps.dispatcher.callCount(), "an occurrence replay must not dispatch a second turn")

	queued, err := target.storage.ListManagedInputs(context.Background(), target.identity)
	require.NoError(t, err)
	require.Empty(t, queued, "immediate dispatch must not create a queued receipt")
}

func TestManagedInputImmediateDispatchReturnsBusyForSessionAndQueuedInput(t *testing.T) {
	target := newManagedInputTestTarget(t)
	dispatcher := &managedInputImmediateDispatcher{base: target.deps.dispatcher}
	target.service.SetDispatcher(dispatcher)
	request := pluginsdk.ManagedAgentConversationDispatch{
		RequestID: "dispatch-request", IdempotencyKey: "dispatch-idem",
		WorkspaceID: "ws-1", InstanceKey: "coordinator", ExpectedConversationRevision: 1,
		ApprovalRevision: 3, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		OccurrenceKey: "immediate-2", Origin: pluginsdk.ManagedAgentInputHuman, Payload: "Run now",
	}

	target.deps.sess.mu.Lock()
	target.deps.sess.sessions[target.descriptor.SessionID].State = models.TaskSessionStateRunning
	target.deps.sess.sessions[target.descriptor.SessionID].AgentExecutionID = "active-execution"
	target.deps.sess.mu.Unlock()
	gotStatus, _, err := target.service.DispatchManagedInput(context.Background(), "install-1", request, "dispatch-op", "dispatch-digest")
	require.NoError(t, err)
	require.Equal(t, pluginsdk.ManagedAgentDispatchBusy, gotStatus)
	require.Zero(t, target.deps.dispatcher.callCount())

	target.deps.sess.mu.Lock()
	target.deps.sess.sessions[target.descriptor.SessionID].State = models.TaskSessionStateCreated
	target.deps.sess.sessions[target.descriptor.SessionID].AgentExecutionID = ""
	target.deps.sess.mu.Unlock()
	enqueue := managedInputEnqueueRequest("queued first", "queued-before-immediate")
	_, _, err = target.service.EnqueueManagedInput(context.Background(), "install-1", "input-queued", enqueue, "enqueue-op", "enqueue-digest")
	require.NoError(t, err)
	request.OccurrenceKey = "immediate-3"
	request.RequestID = "dispatch-request-3"
	request.IdempotencyKey = "dispatch-idem-3"
	gotStatus, _, err = target.service.DispatchManagedInput(context.Background(), "install-1", request, "dispatch-op-3", "dispatch-digest-3")
	require.NoError(t, err)
	require.Equal(t, pluginsdk.ManagedAgentDispatchBusy, gotStatus)
	require.Zero(t, target.deps.dispatcher.callCount(), "immediate dispatch cannot pass an already accepted FIFO input")
}
