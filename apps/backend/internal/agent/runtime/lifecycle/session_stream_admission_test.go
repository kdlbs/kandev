package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestSessionManager_StreamBoundaryPrecedesAdmission(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	sm := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	if err := client.StreamUpdates(context.Background(), func(event agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("connect agent stream: %v", err)
	}
	waitForWSConnected(t, mock)

	var sessionGuard sync.Mutex
	var guardHeld atomic.Bool
	lockSessionGuard := func() {
		sessionGuard.Lock()
		guardHeld.Store(true)
	}
	unlockSessionGuard := func() {
		if guardHeld.Swap(false) {
			sessionGuard.Unlock()
		}
	}
	defer unlockSessionGuard()
	firstPublishEntered := make(chan struct{})
	published := make(chan coalescedStreamChunk, 2)
	memoryBus := eventbus.NewMemoryEventBus(newSessionTestLogger())
	t.Cleanup(memoryBus.Close)
	eventPublisher := NewEventPublisher(memoryBus, newSessionTestLogger())
	subscription, err := memoryBus.Subscribe(
		events.BuildAgentStreamSubject("stream-admission-session"),
		func(_ context.Context, event *eventbus.Event) error {
			payload, ok := event.Data.(AgentStreamEventPayload)
			if !ok || payload.Data == nil {
				return errors.New("agent stream event payload is missing")
			}
			chunk := coalescedStreamChunk{
				eventType: payload.Data.Type, messageID: payload.Data.MessageID,
				content: payload.Data.Text, isAppend: payload.Data.IsAppend,
				promptGeneration: payload.Data.PromptGeneration,
			}
			if chunk.content == "old-first" {
				close(firstPublishEntered)
			}
			lockSessionGuard()
			published <- chunk
			unlockSessionGuard()
			return nil
		},
	)
	if err != nil {
		t.Fatalf("subscribe to agent stream events: %v", err)
	}
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	sm.SetDependencies(eventPublisher, nil, nil, nil)
	manager := &Manager{eventPublisher: eventPublisher, logger: newSessionTestLogger()}

	execution := &AgentExecution{
		ID:               "stream-admission-exec",
		TaskID:           "stream-admission-task",
		SessionID:        "stream-admission-session",
		Status:           v1.AgentStatusRunning,
		agentctl:         client,
		promptDoneCh:     make(chan PromptCompletionSignal, 1),
		promptGeneration: 7,
	}
	lockSessionGuard()
	stream := manager.streamCoalescer(execution)

	firstAddDone := make(chan struct{})
	go func() {
		stream.add(coalescedStreamChunk{
			eventType: "message_streaming", messageID: "old-message", content: "old-first",
			promptGeneration: 7,
		})
		close(firstAddDone)
	}()
	select {
	case <-firstPublishEntered:
	case <-time.After(2 * time.Second):
		unlockSessionGuard()
		select {
		case <-firstAddDone:
		case <-time.After(2 * time.Second):
		}
		t.Fatal("stream publication did not reach the session guard")
	}
	// The orchestrator releases this physical guard before entering lifecycle
	// prompt preparation. The in-flight synchronous publication can then finish.
	unlockSessionGuard()
	select {
	case <-firstAddDone:
	case <-time.After(2 * time.Second):
		t.Fatal("stream publication remained blocked after releasing the session guard")
	}
	if first := <-published; first.content != "old-first" {
		t.Fatalf("first published chunk = %#v, want old-first", first)
	}

	stream.add(coalescedStreamChunk{
		eventType: "message_streaming", messageID: "old-message", content: "old-tail",
		isAppend: true, promptGeneration: 7,
	})
	stream.add(coalescedStreamChunk{
		eventType: "message_streaming", messageID: "old-message", content: "-segment",
		isAppend: true, promptGeneration: 7,
	})
	var publishedBeforeAdmission []coalescedStreamChunk
	dispatched := make(chan struct{})
	var dispatchCount atomic.Int32
	result := make(chan error, 1)
	go func() {
		_, err := sm.SendPromptWithAdmissionCallback(
			context.Background(), execution, "next prompt", true, nil, true,
			func() error {
				lockSessionGuard()
				if execution.promptGeneration != 7 {
					unlockSessionGuard()
					return errPromptAdmissionGenerationAdvanced
				}
				for {
					select {
					case chunk := <-published:
						publishedBeforeAdmission = append(publishedBeforeAdmission, chunk)
					default:
						return nil
					}
				}
			},
			func() {
				dispatchCount.Add(1)
				unlockSessionGuard()
				close(dispatched)
			},
		)
		result <- err
	}()

	select {
	case <-dispatched:
	case <-time.After(2 * time.Second):
		select {
		case err := <-result:
			t.Fatalf("prompt returned before dispatch acceptance: %v", err)
		default:
			t.Fatal("prompt was not accepted after stream-boundary admission")
		}
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("SendPromptWithAdmissionCallback: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch-only prompt did not return after acceptance")
	}
	if len(publishedBeforeAdmission) != 1 || publishedBeforeAdmission[0].content != "old-tail-segment" {
		t.Fatalf("published before admission = %#v, want exact buffered old tail", publishedBeforeAdmission)
	}
	if publishedBeforeAdmission[0].messageID != "old-message" || publishedBeforeAdmission[0].promptGeneration != 7 {
		t.Fatalf("old tail correlation changed before admission: %#v", publishedBeforeAdmission[0])
	}
	if execution.promptGeneration != 8 {
		t.Fatalf("prompt generation = %d, want 8 after admission", execution.promptGeneration)
	}
	if got := dispatchCount.Load(); got != 1 {
		t.Fatalf("dispatch callback count = %d, want 1", got)
	}
	if execution.ID != "stream-admission-exec" || execution.Status != v1.AgentStatusRunning {
		t.Fatalf("accepted execution changed: id=%s status=%s", execution.ID, execution.Status)
	}
}

var errPromptAdmissionGenerationAdvanced = errors.New("prompt generation advanced before admission")
