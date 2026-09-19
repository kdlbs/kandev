package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
)

type agentPlanRaceRepository struct {
	repository.MessageRepository
	receiptWriter conversationMessageReceiptWriter

	admissionMu   sync.Mutex
	stateMu       sync.Mutex
	admissionHeld bool

	olderUpdateReady chan struct{}
	releaseOlder     chan struct{}
	newerUpdateDone  chan struct{}
}

func newAgentPlanRaceRepository(base repository.MessageRepository) *agentPlanRaceRepository {
	return &agentPlanRaceRepository{
		MessageRepository: base,
		receiptWriter:     base.(conversationMessageReceiptWriter),
		olderUpdateReady:  make(chan struct{}),
		releaseOlder:      make(chan struct{}),
		newerUpdateDone:   make(chan struct{}),
	}
}

func (r *agentPlanRaceRepository) AcquireMessageAdmission(
	ctx context.Context,
	messageID string,
) (context.Context, func(), error) {
	key := "message:" + messageID
	if plancommenttx.AdmissionLeaseHeld(ctx, key) {
		return ctx, func() {}, nil
	}
	r.admissionMu.Lock()
	r.stateMu.Lock()
	r.admissionHeld = true
	r.stateMu.Unlock()
	return plancommenttx.WithAdmissionLease(ctx, key), func() {
		r.stateMu.Lock()
		r.admissionHeld = false
		r.stateMu.Unlock()
		r.admissionMu.Unlock()
	}, nil
}

func (r *agentPlanRaceRepository) CreateMessageWithConversationReceipt(
	ctx context.Context,
	message *models.Message,
) (*models.ConversationMutationReceipt, error) {
	return r.receiptWriter.CreateMessageWithConversationReceipt(ctx, message)
}

func (r *agentPlanRaceRepository) UpdateMessageWithConversationReceipt(
	ctx context.Context,
	message *models.Message,
) (*models.ConversationMutationReceipt, error) {
	if message.Content == "older snapshot" {
		close(r.olderUpdateReady)
		<-r.releaseOlder
	}
	receipt, err := r.receiptWriter.UpdateMessageWithConversationReceipt(ctx, message)
	if message.Content == "newer snapshot" {
		close(r.newerUpdateDone)
	}
	return receipt, err
}

func (r *agentPlanRaceRepository) DeleteMessageWithConversationReceipt(
	ctx context.Context,
	messageID string,
) (*models.ConversationMutationReceipt, error) {
	return r.receiptWriter.DeleteMessageWithConversationReceipt(ctx, messageID)
}

func (r *agentPlanRaceRepository) isAdmissionHeld() bool {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.admissionHeld
}

// @covers AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.1
// @covers AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.2
func TestUpsertAgentPlanMessageCoalescesSnapshotsAndSeparatesToolCalls(t *testing.T) {
	service, eventBus, repo := newMessageTestService(t)
	ctx := context.Background()

	for _, snapshot := range []string{"# Plan\n\n1. Read", "# Plan\n\n1. Read\n2. Write"} {
		err := service.UpsertAgentPlanMessage(
			ctx, "task-msg", "source-call-1", "sess-msg", snapshot, "",
		)
		if err != nil {
			t.Fatalf("upsert correlated plan: %v", err)
		}
	}
	publishedBeforeReplay := len(eventBus.GetPublishedEvents())
	if err := service.UpsertAgentPlanMessage(
		ctx, "task-msg", "source-call-1", "sess-msg", "# Plan\n\n1. Read\n2. Write", "",
	); err != nil {
		t.Fatalf("replay identical plan: %v", err)
	}
	if published := len(eventBus.GetPublishedEvents()); published != publishedBeforeReplay {
		t.Fatalf("identical replay published %d new events, want 0", published-publishedBeforeReplay)
	}

	if err := service.UpsertAgentPlanMessage(
		ctx, "task-msg", "source-call-2", "sess-msg", "# Other plan", "",
	); err != nil {
		t.Fatalf("upsert separate plan: %v", err)
	}

	messages, err := repo.ListMessages(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[0].Content != "# Plan\n\n1. Read\n2. Write" {
		t.Fatalf("first plan content = %q", messages[0].Content)
	}
	if messages[1].Content != "# Other plan" {
		t.Fatalf("second plan content = %q", messages[1].Content)
	}
	if correlation, _ := messages[0].Metadata["tool_call_id"].(string); !strings.HasPrefix(correlation, "agent-plan:") {
		t.Fatalf("plan correlation = %q, want agent-plan namespace", correlation)
	}
	if source := messages[0].Metadata["agent_plan_tool_call_id"]; source != "source-call-1" {
		t.Fatalf("source tool call id = %v, want source-call-1", source)
	}
}

// @covers AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.2
func TestUpsertAgentPlanMessageSerializesReadThroughUpdate(t *testing.T) {
	service, eventBus, baseRepo := newMessageTestService(t)
	raceRepo := newAgentPlanRaceRepository(baseRepo)
	service.messages = raceRepo
	ctx := context.Background()

	if err := service.UpsertAgentPlanMessage(
		ctx, "task-msg", "source-call-race", "sess-msg", "initial snapshot", "turn-msg",
	); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	eventBus.ClearEvents()

	olderDone := make(chan error, 1)
	go func() {
		olderDone <- service.UpsertAgentPlanMessage(
			ctx, "task-msg", "source-call-race", "sess-msg", "older snapshot", "turn-msg",
		)
	}()
	waitForAgentPlanRace(t, raceRepo.olderUpdateReady, "older snapshot update")

	newerDone := make(chan error, 1)
	go func() {
		newerDone <- service.UpsertAgentPlanMessage(
			ctx, "task-msg", "source-call-race", "sess-msg", "newer snapshot", "turn-msg",
		)
	}()
	if raceRepo.isAdmissionHeld() {
		close(raceRepo.releaseOlder)
	} else {
		waitForAgentPlanRace(t, raceRepo.newerUpdateDone, "newer snapshot update")
		close(raceRepo.releaseOlder)
	}
	if err := waitForAgentPlanResult(t, olderDone, "older upsert"); err != nil {
		t.Fatalf("older upsert: %v", err)
	}
	if err := waitForAgentPlanResult(t, newerDone, "newer upsert"); err != nil {
		t.Fatalf("newer upsert: %v", err)
	}

	messages, err := baseRepo.ListMessages(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if messages[0].Content != "newer snapshot" {
		t.Fatalf("plan content = %q, want newer snapshot", messages[0].Content)
	}
}

func waitForAgentPlanRace(t *testing.T, ready <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", operation)
	}
}

func waitForAgentPlanResult(t *testing.T, result <-chan error, operation string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", operation)
		return nil
	}
}
