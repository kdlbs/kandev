package clarification

import (
	"context"
	"errors"
	"maps"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type resolverDeliveryMessageCreator struct {
	repo             *stubMessageStore
	finalizeErr      error
	refuseFinalize   bool
	restoreErr       error
	refuseRestore    bool
	publishErr       error
	claimHasDeadline bool
	claimContextErr  error
	restoreCalls     int
	published        [][]*taskmodels.Message
}

func (s *resolverDeliveryMessageCreator) CreateClarificationRequestMessages(
	context.Context, string, string, string, []Question, string,
) ([]string, error) {
	return nil, nil
}

func (s *resolverDeliveryMessageCreator) UpdateClarificationMessage(
	context.Context, string, string, string, string, *Answer,
) error {
	return nil
}

func (s *resolverDeliveryMessageCreator) CompleteActiveClarificationBundle(
	ctx context.Context,
	pendingID, status string,
	responses map[string]interface{},
) ([]*taskmodels.Message, bool, error) {
	_, s.claimHasDeadline = ctx.Deadline()
	s.claimContextErr = ctx.Err()
	stored := s.repo.messages[pendingID]
	claimed := make([]*taskmodels.Message, 0, len(stored))
	for _, message := range stored {
		currentStatus := stringFromMetadata(message.Metadata, metaStatusKey)
		if currentStatus != "" && currentStatus != string(StatusPending) {
			continue
		}
		message.Metadata[metaStatusKey] = status
		message.Metadata["response_delivery_pending"] = true
		if response, ok := responses[questionIDFromMetadata(message.Metadata)]; ok {
			if answer, ok := response.(Answer); ok {
				selected := make([]any, len(answer.SelectedOptions))
				for i, option := range answer.SelectedOptions {
					selected[i] = option
				}
				message.Metadata["response"] = map[string]any{
					"selected_options": selected,
					"custom_text":      answer.CustomText,
				}
			}
		}
		claimed = append(claimed, cloneResolverMessage(message))
	}
	return claimed, len(claimed) > 0, nil
}

func (s *resolverDeliveryMessageCreator) FinalizeClarificationResponseDelivery(
	_ context.Context,
	pendingID, _ string,
	claimedMessages []*taskmodels.Message,
) ([]*taskmodels.Message, bool, error) {
	if s.finalizeErr != nil {
		return nil, false, s.finalizeErr
	}
	if s.refuseFinalize {
		return nil, false, nil
	}
	finalized := make([]*taskmodels.Message, 0, len(claimedMessages))
	for _, claimed := range claimedMessages {
		stored := resolverMessageByID(s.repo.messages[pendingID], claimed.ID)
		if stored == nil {
			return nil, false, nil
		}
		delete(stored.Metadata, "response_delivery_pending")
		finalized = append(finalized, cloneResolverMessage(stored))
	}
	return finalized, true, nil
}

func (s *resolverDeliveryMessageCreator) RestoreActiveClarificationBundle(
	_ context.Context,
	pendingID, _ string,
	claimedMessages []*taskmodels.Message,
) ([]*taskmodels.Message, bool, error) {
	s.restoreCalls++
	if s.restoreErr != nil {
		return nil, false, s.restoreErr
	}
	if s.refuseRestore {
		return nil, false, nil
	}
	restored := make([]*taskmodels.Message, 0, len(claimedMessages))
	for _, claimed := range claimedMessages {
		stored := resolverMessageByID(s.repo.messages[pendingID], claimed.ID)
		if stored == nil {
			return nil, false, nil
		}
		stored.Metadata[metaStatusKey] = string(StatusPending)
		delete(stored.Metadata, "response")
		delete(stored.Metadata, "response_delivery_pending")
		restored = append(restored, cloneResolverMessage(stored))
	}
	return restored, true, nil
}

func (s *resolverDeliveryMessageCreator) PublishClarificationBundleUpdates(
	_ context.Context, messages []*taskmodels.Message,
) error {
	if s.publishErr != nil {
		return s.publishErr
	}
	published := make([]*taskmodels.Message, 0, len(messages))
	for _, message := range messages {
		published = append(published, cloneResolverMessage(message))
	}
	s.published = append(s.published, published)
	return nil
}

func cloneResolverMessage(message *taskmodels.Message) *taskmodels.Message {
	copyMessage := *message
	copyMessage.Metadata = maps.Clone(message.Metadata)
	return &copyMessage
}

func resolverMessageByID(messages []*taskmodels.Message, id string) *taskmodels.Message {
	for _, message := range messages {
		if message.ID == id {
			return message
		}
	}
	return nil
}

func resolverDeliveryMessage(pendingID, messageID, turnID string) *taskmodels.Message {
	return &taskmodels.Message{
		ID:            messageID,
		TaskID:        "task-1",
		TaskSessionID: "session-1",
		TurnID:        turnID,
		Metadata: map[string]any{
			metaPendingIDKey:  pendingID,
			metaQuestionIDKey: "q1",
			metaStatusKey:     string(StatusPending),
			metaQuestionKey: map[string]any{
				"id":     "q1",
				"prompt": "Continue?",
				"options": []any{
					map[string]any{"option_id": "yes", "label": "Yes"},
					map[string]any{"option_id": "no", "label": "No"},
				},
			},
		},
	}
}

func newResolverDeliveryFixture(
	t *testing.T,
	pendingID string,
	messages []*taskmodels.Message,
) (*Resolver, *Store, *stubMessageStore, *resolverDeliveryMessageCreator, *stubEventBus) {
	t.Helper()
	store := NewStore(time.Minute)
	repo := &stubMessageStore{messages: map[string][]*taskmodels.Message{pendingID: messages}}
	creator := &resolverDeliveryMessageCreator{repo: repo}
	eventBus := &stubEventBus{}
	resolver := NewResolver(
		store,
		repo,
		creator,
		&stubAuthorizer{},
		eventBus,
		eventBus,
		nil,
		logger.Default(),
	)
	return resolver, store, repo, creator, eventBus
}

type resolverOrderingEventBus struct {
	*stubEventBus
	onPrimaryPublish func()
}

func (b *resolverOrderingEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if subject == events.ClarificationPrimaryAnswered && b.onPrimaryPublish != nil {
		b.onPrimaryPublish()
	}
	return b.stubEventBus.Publish(ctx, subject, event)
}

type resolverAsyncFanoutEventBus struct {
	*stubEventBus
	primaryPublishReturned chan struct{}
	releaseFanout          chan struct{}
}

func (b *resolverAsyncFanoutEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if subject == events.ClarificationPrimaryAnswered {
		close(b.primaryPublishReturned)
		go func() {
			<-b.releaseFanout
		}()
	}
	return b.stubEventBus.Publish(ctx, subject, event)
}

func resolverAnswer() Outcome {
	return Outcome{Answers: []Answer{{
		QuestionID:      "q1",
		SelectedOptions: []string{"yes"},
	}}}
}

func TestResolverLiveDeliveryRequiresDurableConfirmation(t *testing.T) {
	const pendingID = "pending-live-confirmation"
	message := resolverDeliveryMessage(pendingID, "message-live-confirmation", "turn-1")
	resolver, store, repo, creator, _ := newResolverDeliveryFixture(t, pendingID, []*taskmodels.Message{message})
	creator.finalizeErr = errors.New("database unavailable")
	store.CreateRequest(&Request{
		PendingID: pendingID,
		SessionID: "session-1",
		TaskID:    "task-1",
		Questions: []Question{{
			ID: "q1", Prompt: "Continue?",
			Options: []Option{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}},
		}},
	})

	waitEntered := make(chan struct{}, 1)
	store.SetOnWaitEntered(func(string) { waitEntered <- struct{}{} })
	waitDone := make(chan error, 1)
	go func() {
		_, err := store.WaitForResponse(context.Background(), pendingID)
		waitDone <- err
	}()
	<-waitEntered
	store.SetOnWaitEntered(nil)

	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, resolverAnswer())
	if !claimed {
		t.Fatal("live response did not win the durable claim")
	}
	if err == nil {
		t.Fatal("unconfirmed live delivery returned success")
	}
	if waitErr := <-waitDone; waitErr == nil {
		t.Fatal("live waiter returned an unconfirmed response")
	}
	if got := stringFromMetadata(repo.messages[pendingID][0].Metadata, metaStatusKey); got != string(StatusPending) {
		t.Fatalf("restored status = %q, want pending", got)
	}
	if creator.restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1", creator.restoreCalls)
	}
}

func TestResolverLiveDeliveryPublishesPrimaryAnswerBeforeWaiterReturns(t *testing.T) {
	const pendingID = "pending-live-ordering"
	message := resolverDeliveryMessage(pendingID, "message-live-ordering", "turn-1")
	resolver, store, _, _, eventBus := newResolverDeliveryFixture(
		t, pendingID, []*taskmodels.Message{message},
	)
	orderingBus := &resolverOrderingEventBus{stubEventBus: eventBus}
	resolver.eventBus = orderingBus
	store.CreateRequest(&Request{
		PendingID: pendingID,
		SessionID: "session-1",
		TaskID:    "task-1",
		Questions: []Question{{
			ID: "q1", Prompt: "Continue?",
			Options: []Option{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}},
		}},
	})

	waitEntered := make(chan struct{}, 1)
	store.SetOnWaitEntered(func(string) { waitEntered <- struct{}{} })
	waitReturned := make(chan struct{})
	waitDone := make(chan error, 1)
	go func() {
		_, err := store.WaitForResponse(context.Background(), pendingID)
		close(waitReturned)
		waitDone <- err
	}()
	<-waitEntered
	store.SetOnWaitEntered(nil)

	eventBeforeWaiterReturn := true
	eventPublished := make(chan struct{})
	orderingBus.onPrimaryPublish = func() {
		select {
		case <-waitReturned:
			eventBeforeWaiterReturn = false
		default:
		}
		close(eventPublished)
	}

	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, resolverAnswer())
	if err != nil || !claimed {
		t.Fatalf("ResolveBundle = claimed %v, err %v; want live delivery", claimed, err)
	}
	<-eventPublished
	if err := <-waitDone; err != nil {
		t.Fatalf("WaitForResponse: %v", err)
	}
	if !eventBeforeWaiterReturn {
		t.Fatal("primary-answer event was published after the live waiter returned")
	}
	if len(eventBus.events) != 1 || eventBus.events[0].Type != events.ClarificationPrimaryAnswered {
		t.Fatalf("primary-answer events = %d, want exactly 1", len(eventBus.events))
	}
}

func TestResolverLiveDeliveryNotifiesWatchdogBeforeWaiterReturns(t *testing.T) {
	const pendingID = "pending-live-watchdog-notifier"
	message := resolverDeliveryMessage(pendingID, "message-live-watchdog-notifier", "turn-1")
	resolver, store, _, _, _ := newResolverDeliveryFixture(t, pendingID, []*taskmodels.Message{message})

	notified := make(chan struct{})
	resolver.primaryAnsweredNotifier = func(context.Context, PrimaryAnswered) {
		close(notified)
	}

	store.CreateRequest(&Request{
		PendingID: pendingID,
		SessionID: "session-1",
		TaskID:    "task-1",
		Questions: []Question{{
			ID: "q1", Prompt: "Continue?",
			Options: []Option{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}},
		}},
	})
	waitEntered := make(chan struct{}, 1)
	store.SetOnWaitEntered(func(string) { waitEntered <- struct{}{} })
	waitDone := make(chan error, 1)
	go func() {
		_, err := store.WaitForResponse(context.Background(), pendingID)
		waitDone <- err
	}()
	<-waitEntered
	store.SetOnWaitEntered(nil)

	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, resolverAnswer())
	if err != nil || !claimed {
		t.Fatalf("ResolveBundle = claimed %v, err %v; want live delivery", claimed, err)
	}
	var waitErr error
	select {
	case waitErr = <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("WaitForResponse did not return")
	}
	select {
	case <-notified:
	default:
		t.Fatal("live waiter returned before the primary-answer watchdog was notified")
	}
	if waitErr != nil {
		t.Fatalf("WaitForResponse: %v", waitErr)
	}
}

// This is reviewer-requested contract coverage for transports whose Publish
// method returns before subscribers run, such as NATS. The local notifier is
// the ordering acknowledgement; fan-out delivery is not.
func TestResolverLiveDeliveryUsesSynchronousNotifierBeforeAsyncFanout(t *testing.T) {
	const pendingID = "pending-live-watchdog-async-fanout"
	message := resolverDeliveryMessage(pendingID, "message-live-watchdog-async-fanout", "turn-1")
	resolver, store, _, _, eventBus := newResolverDeliveryFixture(t, pendingID, []*taskmodels.Message{message})
	asyncBus := &resolverAsyncFanoutEventBus{
		stubEventBus:           eventBus,
		primaryPublishReturned: make(chan struct{}),
		releaseFanout:          make(chan struct{}),
	}
	resolver.eventBus = asyncBus
	t.Cleanup(func() { close(asyncBus.releaseFanout) })

	notifierEntered := make(chan struct{})
	releaseNotifier := make(chan struct{})
	var notifierReturned atomic.Bool
	resolver.primaryAnsweredNotifier = func(context.Context, PrimaryAnswered) {
		close(notifierEntered)
		<-releaseNotifier
		notifierReturned.Store(true)
	}

	store.CreateRequest(&Request{
		PendingID: pendingID,
		SessionID: "session-1",
		TaskID:    "task-1",
		Questions: []Question{{
			ID: "q1", Prompt: "Continue?",
			Options: []Option{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}},
		}},
	})
	waitEntered := make(chan struct{}, 1)
	store.SetOnWaitEntered(func(string) { waitEntered <- struct{}{} })
	waitDone := make(chan error, 1)
	go func() {
		_, err := store.WaitForResponse(context.Background(), pendingID)
		if !notifierReturned.Load() {
			t.Errorf("live waiter returned before notifier acknowledgement")
		}
		waitDone <- err
	}()
	<-waitEntered
	store.SetOnWaitEntered(nil)

	resolveDone := make(chan error, 1)
	go func() {
		_, _, err := resolver.ResolveBundle(context.Background(), pendingID, resolverAnswer())
		if !notifierReturned.Load() {
			t.Errorf("resolver returned before notifier acknowledgement")
		}
		resolveDone <- err
	}()

	select {
	case <-notifierEntered:
	case <-time.After(time.Second):
		t.Fatal("synchronous primary-answer notifier was not reached")
	}
	select {
	case <-asyncBus.primaryPublishReturned:
		t.Fatal("event-bus fan-out started before the synchronous notifier returned")
	default:
	}

	close(releaseNotifier)
	if err := <-resolveDone; err != nil {
		t.Fatalf("ResolveBundle: %v", err)
	}
	if err := <-waitDone; err != nil {
		t.Fatalf("WaitForResponse: %v", err)
	}
	select {
	case <-asyncBus.primaryPublishReturned:
	default:
		t.Fatal("primary-answer fan-out was not published after notifier acknowledgement")
	}
}

func TestResolverDetachedResumeFailureRestoresRetryableBundle(t *testing.T) {
	const pendingID = "pending-detached-retry"
	message := resolverDeliveryMessage(pendingID, "message-detached-retry", "turn-1")
	resolver, _, repo, creator, eventBus := newResolverDeliveryFixture(t, pendingID, []*taskmodels.Message{message})
	eventBus.resumeErr = errors.New("orchestrator rejected resume")

	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, resolverAnswer())
	if !claimed || err == nil {
		t.Fatalf("first detached response = claimed %v, err %v; want claimed true and error", claimed, err)
	}
	if got := stringFromMetadata(repo.messages[pendingID][0].Metadata, metaStatusKey); got != string(StatusPending) {
		t.Fatalf("status after failed resume = %q, want pending", got)
	}
	if creator.restoreCalls != 1 || len(creator.published) != 1 {
		t.Fatalf("restore calls=%d published=%d, want 1/1", creator.restoreCalls, len(creator.published))
	}

	eventBus.resumeErr = nil
	resolution, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, resolverAnswer())
	if err != nil || !claimed || resolution.Status != string(StatusAnswered) {
		t.Fatalf("retry response = resolution %+v, claimed %v, err %v", resolution, claimed, err)
	}
	if len(eventBus.resumeRequests) != 2 {
		t.Fatalf("resume attempts = %d, want 2", len(eventBus.resumeRequests))
	}
}

type acceptedResolverResumeError struct{ err error }

func (e acceptedResolverResumeError) Error() string              { return e.err.Error() }
func (e acceptedResolverResumeError) Unwrap() error              { return e.err }
func (acceptedResolverResumeError) DetachedResumeAccepted() bool { return true }

func TestResolverAcceptedResumeFailureRemainsTerminal(t *testing.T) {
	const pendingID = "pending-accepted-resume"
	message := resolverDeliveryMessage(pendingID, "message-accepted-resume", "turn-1")
	resolver, _, repo, creator, eventBus := newResolverDeliveryFixture(t, pendingID, []*taskmodels.Message{message})
	eventBus.resumeErr = acceptedResolverResumeError{err: errors.New("turn publication failed")}

	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, resolverAnswer())
	if !claimed || err == nil {
		t.Fatalf("accepted resume response = claimed %v, err %v; want claimed true and error", claimed, err)
	}
	if creator.restoreCalls != 0 {
		t.Fatalf("restore calls = %d, want 0 for an accepted resume", creator.restoreCalls)
	}
	if got := stringFromMetadata(repo.messages[pendingID][0].Metadata, metaStatusKey); got != string(StatusAnswered) {
		t.Fatalf("accepted resume status = %q, want answered", got)
	}
	if _, ok := repo.messages[pendingID][0].Metadata["response_delivery_pending"]; ok {
		t.Fatal("accepted resume retained the delivery recovery marker")
	}

	resolution, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, resolverAnswer())
	if err != nil || claimed || resolution.Status != string(StatusAnswered) {
		t.Fatalf("replayed accepted response = resolution %+v, claimed %v, err %v", resolution, claimed, err)
	}
	if len(eventBus.resumeRequests) != 1 {
		t.Fatalf("resume attempts after replay = %d, want 1", len(eventBus.resumeRequests))
	}
}

func TestResolverDetachedResumeFailureReportsRestoreFailure(t *testing.T) {
	for _, test := range []struct {
		name          string
		restoreErr    error
		refuseRestore bool
	}{
		{name: "restore error", restoreErr: errors.New("restore failed")},
		{name: "restore refused", refuseRestore: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			const pendingID = "pending-restore-failure"
			message := resolverDeliveryMessage(pendingID, "message-restore-failure", "turn-1")
			resolver, _, repo, creator, eventBus := newResolverDeliveryFixture(t, pendingID, []*taskmodels.Message{message})
			creator.restoreErr = test.restoreErr
			creator.refuseRestore = test.refuseRestore
			eventBus.resumeErr = errors.New("orchestrator rejected resume")

			_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, resolverAnswer())
			if !claimed || err == nil {
				t.Fatalf("response = claimed %v, err %v; want claimed true and error", claimed, err)
			}
			if got := stringFromMetadata(repo.messages[pendingID][0].Metadata, metaStatusKey); got != string(StatusAnswered) {
				t.Fatalf("status after failed restore = %q, want answered", got)
			}
		})
	}
}

func TestResolverDetachedResumeRestorePublicationFailureKeepsRetryableState(t *testing.T) {
	const pendingID = "pending-restore-publication"
	message := resolverDeliveryMessage(pendingID, "message-restore-publication", "turn-1")
	resolver, _, repo, creator, eventBus := newResolverDeliveryFixture(t, pendingID, []*taskmodels.Message{message})
	creator.publishErr = errors.New("websocket unavailable")
	eventBus.resumeErr = errors.New("orchestrator rejected resume")

	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, resolverAnswer())
	if !claimed || err == nil {
		t.Fatalf("response = claimed %v, err %v; want claimed true and error", claimed, err)
	}
	if got := stringFromMetadata(repo.messages[pendingID][0].Metadata, metaStatusKey); got != string(StatusPending) {
		t.Fatalf("status after publication failure = %q, want pending", got)
	}
	if creator.restoreCalls != 1 || len(creator.published) != 0 {
		t.Fatalf("restore calls=%d published=%d, want 1/0", creator.restoreCalls, len(creator.published))
	}
}

func TestResolverDetachedResumeUsesFreshBoundedContextAfterCallerCancellation(t *testing.T) {
	const pendingID = "pending-cancelled-context"
	message := resolverDeliveryMessage(pendingID, "message-cancelled-context", "turn-1")
	resolver, _, _, creator, eventBus := newResolverDeliveryFixture(t, pendingID, []*taskmodels.Message{message})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	questions := []Question{{
		ID: "q1", Prompt: "Continue?",
		Options: []Option{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}},
	}}
	_, claimed, err := resolver.claimAndDeliver(ctx, pendingID, "session-1", "task-1", questions, resolverAnswer())
	if err != nil || !claimed {
		t.Fatalf("cancelled-context response = claimed %v, err %v", claimed, err)
	}
	if !creator.claimHasDeadline || creator.claimContextErr != nil {
		t.Fatalf("claim context deadline=%v err=%v, want fresh bounded context", creator.claimHasDeadline, creator.claimContextErr)
	}
	if len(eventBus.resumeHasDeadline) != 1 || !eventBus.resumeHasDeadline[0] || eventBus.resumeContextErrs[0] != nil {
		t.Fatalf("resume contexts = deadlines %v errors %v, want fresh bounded context", eventBus.resumeHasDeadline, eventBus.resumeContextErrs)
	}
}

func TestClarificationClaimRecoveryRejectsMixedTurns(t *testing.T) {
	messages := []*taskmodels.Message{
		{ID: "message-one", TurnID: "turn-one"},
		{ID: "message-two", TurnID: "turn-two"},
	}
	if _, _, err := clarificationClaimRecovery(messages); err == nil {
		t.Fatal("clarificationClaimRecovery accepted messages from different turns")
	}
}
