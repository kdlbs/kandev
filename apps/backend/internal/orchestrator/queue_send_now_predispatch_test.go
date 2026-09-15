package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// preDispatchGateAgentManager pauses FIFO A before the provider call and keeps
// its completion pending after the acceptance callback. This models a real
// provider admission boundary followed by a late predecessor completion.
type preDispatchGateAgentManager struct {
	*mockAgentManager
	firstDispatchEntered chan struct{}
	allowFirstDispatch   chan struct{}
	firstAccepted        chan struct{}
	allowFirstReturn     chan struct{}
	firstReturned        chan struct{}
	secondAccepted       chan struct{}
	allowSecondReturn    chan struct{}
	secondReturned       chan struct{}
	promptCount          atomic.Int32
	firstDispatchOnce    sync.Once
	firstAcceptedOnce    sync.Once
	firstReturnedOnce    sync.Once
	secondAcceptedOnce   sync.Once
	secondReturnedOnce   sync.Once
}

func (m *preDispatchGateAgentManager) PromptAgentWithDispatchCallback(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	onDispatched func(),
) (*executor.PromptResult, error) {
	call := m.promptCount.Add(1)
	if call == 1 {
		m.firstDispatchOnce.Do(func() { close(m.firstDispatchEntered) })
		<-m.allowFirstDispatch
	}
	result, err := m.PromptAgent(ctx, executionID, prompt, attachments, dispatchOnly)
	if err != nil || onDispatched == nil {
		return result, err
	}
	onDispatched()
	switch call {
	case 1:
		m.firstAcceptedOnce.Do(func() { close(m.firstAccepted) })
		<-m.allowFirstReturn
		m.firstReturnedOnce.Do(func() { close(m.firstReturned) })
	case 2:
		m.secondAcceptedOnce.Do(func() { close(m.secondAccepted) })
		<-m.allowSecondReturn
		m.secondReturnedOnce.Do(func() { close(m.secondReturned) })
	default:
		panic(fmt.Sprintf("preDispatchGateAgentManager: unexpected prompt call %d", call))
	}
	return result, nil
}

func (m *preDispatchGateAgentManager) releaseFirstDispatch() {
	select {
	case <-m.allowFirstDispatch:
	default:
		close(m.allowFirstDispatch)
	}
}

func (m *preDispatchGateAgentManager) releaseFirstReturn() {
	select {
	case <-m.allowFirstReturn:
	default:
		close(m.allowFirstReturn)
	}
}

func (m *preDispatchGateAgentManager) releaseSecondReturn() {
	select {
	case <-m.allowSecondReturn:
	default:
		close(m.allowSecondReturn)
	}
}

type armedTurnCaptureService struct {
	*repoTurnService
	armed    atomic.Bool
	captured chan struct{}
	once     sync.Once
}

func (s *armedTurnCaptureService) arm() {
	s.armed.Store(true)
}

func (s *armedTurnCaptureService) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	if s.armed.Load() {
		s.once.Do(func() { close(s.captured) })
	}
	return s.repoTurnService.GetActiveTurn(ctx, sessionID)
}

func hasCancelInFlightGuard(s *Service, sessionID string) bool {
	s.cancelInFlightMu.Lock()
	defer s.cancelInFlightMu.Unlock()
	_, ok := s.cancelInFlight[sessionID]
	return ok
}

// @covers AC-UI-MESSAGE-QUEUE-SEND-NOW-001.9
// @covers AC-UI-MESSAGE-QUEUE-SEND-NOW-001.10
func TestSendQueuedNowSerializesFIFOPredispatchAdmission(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	agentMgr := &preDispatchGateAgentManager{
		mockAgentManager:     &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo},
		firstDispatchEntered: make(chan struct{}),
		allowFirstDispatch:   make(chan struct{}),
		firstAccepted:        make(chan struct{}),
		allowFirstReturn:     make(chan struct{}),
		firstReturned:        make(chan struct{}),
		secondAccepted:       make(chan struct{}),
		allowSecondReturn:    make(chan struct{}),
		secondReturned:       make(chan struct{}),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.messageQueue.SetAutoMergeEnabled(false)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	turns := &armedTurnCaptureService{
		repoTurnService: &repoTurnService{repo: repo},
		captured:        make(chan struct{}),
	}
	svc.turnService = turns
	t.Cleanup(func() {
		agentMgr.releaseFirstDispatch()
		agentMgr.releaseFirstReturn()
		agentMgr.releaseSecondReturn()
		svc.stopSendNowWorkers()
	})

	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "running A", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue FIFO A: %v", err)
	}
	if !svc.drainQueuedMessageForPromptableSession(ctx, "session-1") {
		t.Fatal("ordinary FIFO drain did not start A")
	}
	select {
	case <-agentMgr.firstDispatchEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out before FIFO A provider admission")
	}
	if !hasCancelInFlightGuard(svc, "session-1") {
		t.Fatal("FIFO A released cancellation admission before provider acceptance")
	}

	b, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "urgent B", "", messagequeue.QueuedByUser, false, nil, nil,
	)
	if err != nil {
		t.Fatalf("queue Send Now B: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "later C", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue FIFO C: %v", err)
	}

	turns.arm()
	sendNowDone := make(chan error, 1)
	go func() {
		_, sendErr := svc.SendQueuedNow(ctx, "session-1", QueueSendNowScopeEntry, b.ID)
		sendNowDone <- sendErr
	}()
	select {
	case <-turns.captured:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Send Now turn capture")
	}

	agentMgr.releaseFirstDispatch()
	select {
	case <-agentMgr.firstAccepted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for FIFO A provider acceptance")
	}
	select {
	case err := <-sendNowDone:
		if err != nil {
			t.Fatalf("Send Now B: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Send Now B request")
	}
	select {
	case <-agentMgr.secondAccepted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for replacement B provider acceptance")
	}
	if got := agentMgr.promptCount.Load(); got != 2 {
		t.Fatalf("provider prompt count after B acceptance = %d, want exactly A and B", got)
	}
	agentMgr.mu.Lock()
	prompts := append([]string(nil), agentMgr.capturedPrompts...)
	agentMgr.mu.Unlock()
	if len(prompts) != 2 || prompts[1] != "urgent B" {
		t.Fatalf("provider prompts after B acceptance = %#v, want A and one B", prompts)
	}

	if got := agentMgr.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("cancel calls before late A completion = %d, want 1", got)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 || status.Entries[0].Content != "later C" {
		t.Fatalf("queue after B acceptance = %#v, want C only", status.Entries)
	}
	activeBefore, err := turns.GetActiveTurn(ctx, "session-1")
	if err != nil || activeBefore == nil {
		t.Fatalf("active replacement turn before late A completion = %#v, err=%v", activeBefore, err)
	}
	replacement := svc.acceptedQueuedDispatchForSession("session-1")
	if replacement == nil || replacement.currentPhase() != queuedDispatchLive {
		t.Fatalf("replacement reservation before late A completion = %#v, want live", replacement)
	}
	attemptBefore, ok := svc.promptAttemptForSession("session-1")
	if !ok || attemptBefore == nil {
		t.Fatal("replacement prompt attempt was not recorded")
	}

	agentMgr.releaseFirstReturn()
	select {
	case <-agentMgr.firstReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for late FIFO A completion")
	}
	activeAfter, err := turns.GetActiveTurn(ctx, "session-1")
	if err != nil || activeAfter == nil || activeAfter.ID != activeBefore.ID {
		t.Fatalf("late FIFO A completion changed active replacement: before=%#v after=%#v err=%v", activeBefore, activeAfter, err)
	}
	if got := svc.acceptedQueuedDispatchForSession("session-1"); got != replacement {
		t.Fatal("late FIFO A completion cleared or replaced B ownership")
	}
	attemptAfter, ok := svc.promptAttemptForSession("session-1")
	if !ok || attemptAfter != attemptBefore {
		t.Fatal("late FIFO A completion cleared or replaced B prompt-attempt ownership")
	}

	agentMgr.releaseSecondReturn()
	select {
	case <-agentMgr.secondReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for replacement B completion")
	}
}

var _ executor.AgentManagerClient = (*preDispatchGateAgentManager)(nil)
