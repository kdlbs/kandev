package orchestrator

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type durableDeliveryTestAgentManager struct {
	*mockAgentManager
	capability   agentruntime.DurableDeliveryCapability
	advertised   bool
	capabilityFn func(context.Context, string) (agentruntime.DurableDeliveryCapability, bool)
}

func (m *durableDeliveryTestAgentManager) DurableDeliveryCapabilityForExecution(
	ctx context.Context,
	executionID string,
) (agentruntime.DurableDeliveryCapability, bool) {
	if m.capabilityFn != nil {
		return m.capabilityFn(ctx, executionID)
	}
	return m.capability, m.advertised
}

func TestPrepareAgentDeliverySubmissionUsesStableBackendIdentity(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-admission", "session-delivery-admission", "step-1")
	session, err := repo.GetTaskSession(ctx, "session-delivery-admission")
	if err != nil {
		t.Fatal(err)
	}
	agentManager := &durableDeliveryTestAgentManager{
		mockAgentManager: &mockAgentManager{},
		advertised:       true,
		capability: agentruntime.DurableDeliveryCapability{
			Version: journal.CurrentVersion,
			Durable: true,
		},
	}
	service := createTestServiceWithAgent(
		repo, newMockStepGetter(), newMockTaskRepo(), agentManager,
	)
	options := promptTaskOptions{
		deliveryProtocol:     messagequeue.DeliveryProtocolPending,
		deliverySubmissionID: "queue:attempt-1",
	}
	runtime, err := service.prepareAgentDeliverySubmission(
		ctx, session, "execution-1", "hello", nil, options,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runtime == nil || runtime.id != "prompt:queue:attempt-1" {
		t.Fatalf("runtime = %#v, want prompt:queue:attempt-1", runtime)
	}
	stored, err := repo.GetAgentDeliverySubmission(ctx, runtime.id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.DeliverySubmissionDispatching {
		t.Fatalf("submission state = %q, want dispatching", stored.State)
	}
	if err := runtime.markCompleted(ctx); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.GetAgentDeliverySubmission(ctx, runtime.id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.DeliverySubmissionCompleted {
		t.Fatalf("completed submission state = %q, want completed", stored.State)
	}
}

func TestPrepareAgentDeliverySubmissionBlocksUnresolvedPeer(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-unresolved", "session-delivery-unresolved", "step-1")
	session, err := repo.GetTaskSession(ctx, "session-delivery-unresolved")
	if err != nil {
		t.Fatal(err)
	}
	agentManager := &durableDeliveryTestAgentManager{
		mockAgentManager: &mockAgentManager{},
		advertised:       true,
		capability: agentruntime.DurableDeliveryCapability{
			Version:    journal.CurrentVersion,
			Durable:    true,
			Unresolved: true,
		},
	}
	service := createTestServiceWithAgent(
		repo, newMockStepGetter(), newMockTaskRepo(), agentManager,
	)
	_, err = service.prepareAgentDeliverySubmission(
		ctx,
		session,
		"execution-1",
		"hello",
		nil,
		promptTaskOptions{
			deliveryProtocol:     messagequeue.DeliveryProtocolPending,
			deliverySubmissionID: "queue:attempt-2",
		},
	)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("error = %v, want session recovery required", err)
	}
	block, err := service.GetOpenSessionRecoveryBlock(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if block == nil || block.Reason != durableDeliveryUnresolvedReason {
		t.Fatalf("recovery block = %#v, want %s", block, durableDeliveryUnresolvedReason)
	}
}

func TestPrepareAgentDeliverySubmissionRefreshesRecoveryBeforeAdmittingNextPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-refresh", "session-delivery-refresh", "step-1")
	session, err := repo.GetTaskSession(ctx, "session-delivery-refresh")
	if err != nil {
		t.Fatal(err)
	}
	capability := agentruntime.DurableDeliveryCapability{
		Version:    journal.CurrentVersion,
		Durable:    true,
		Unresolved: true,
		Reason:     durableDeliveryUnresolvedReason,
	}
	agentManager := &durableDeliveryTestAgentManager{
		mockAgentManager: &mockAgentManager{},
		capabilityFn: func(context.Context, string) (agentruntime.DurableDeliveryCapability, bool) {
			return capability, true
		},
	}
	service := createTestServiceWithAgent(
		repo, newMockStepGetter(), newMockTaskRepo(), agentManager,
	)

	_, err = service.prepareAgentDeliverySubmission(
		ctx, session, "execution-1", "blocked", nil,
		promptTaskOptions{
			deliveryProtocol:     messagequeue.DeliveryProtocolPending,
			deliverySubmissionID: "queue:blocked",
		},
	)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("unresolved admission error = %v, want session recovery required", err)
	}
	if _, err := repo.GetAgentDeliverySubmission(ctx, "prompt:queue:blocked"); err == nil {
		t.Fatal("unresolved prompt unexpectedly created a durable submission")
	}

	capability.Unresolved = false
	capability.Reason = ""
	runtime, err := service.prepareAgentDeliverySubmission(
		ctx, session, "execution-1", "first", nil,
		promptTaskOptions{
			deliveryProtocol:     messagequeue.DeliveryProtocolPending,
			deliverySubmissionID: "queue:first",
		},
	)
	if err != nil {
		t.Fatalf("refreshed admission error = %v", err)
	}
	if runtime == nil {
		t.Fatal("refreshed admission returned no durable runtime")
	}
	if err := runtime.markCompleted(ctx); err != nil {
		t.Fatalf("mark first prompt completed: %v", err)
	}
	first, err := repo.GetAgentDeliverySubmission(ctx, "prompt:queue:first")
	if err != nil {
		t.Fatal(err)
	}
	if first.State != models.DeliverySubmissionCompleted {
		t.Fatalf("first prompt state = %q, want completed", first.State)
	}

	next, err := service.prepareAgentDeliverySubmission(
		ctx, session, "execution-1", "next", nil,
		promptTaskOptions{
			deliveryProtocol:     messagequeue.DeliveryProtocolPending,
			deliverySubmissionID: "queue:next",
		},
	)
	if err != nil {
		t.Fatalf("next admission error = %v", err)
	}
	if next == nil || next.id != "prompt:queue:next" {
		t.Fatalf("next runtime = %#v, want prompt:queue:next", next)
	}
	storedNext, err := repo.GetAgentDeliverySubmission(ctx, next.id)
	if err != nil {
		t.Fatal(err)
	}
	if storedNext.State != models.DeliverySubmissionDispatching {
		t.Fatalf("next prompt state = %q, want dispatching", storedNext.State)
	}

	if _, err := service.prepareAgentDeliverySubmission(
		ctx, session, "execution-1", "next", nil,
		promptTaskOptions{
			deliveryProtocol:     messagequeue.DeliveryProtocolPending,
			deliverySubmissionID: "queue:next",
		},
	); !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("duplicate next admission error = %v, want session recovery required", err)
	}
}

func TestPrepareAgentDeliverySubmissionRejectsRotatedContinuationGeneration(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-generation", "session-delivery-generation", "step-1")
	session, err := repo.GetTaskSession(ctx, "session-delivery-generation")
	if err != nil {
		t.Fatal(err)
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	if err := repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID:         session.ID,
		IncarnationID:     incarnationID,
		Generation:        1,
		NativeSessionID:   "native-generation-1",
		CreationReason:    "test",
		OriginalWorkspace: session.WorkspacePath,
		CurrentWorkspace:  session.WorkspacePath,
	}); err != nil {
		t.Fatal(err)
	}
	service := createTestServiceWithAgent(
		repo, newMockStepGetter(), newMockTaskRepo(), &durableDeliveryTestAgentManager{
			mockAgentManager: &mockAgentManager{},
			advertised:       true,
			capability: agentruntime.DurableDeliveryCapability{
				Version: journal.CurrentVersion,
				Durable: true,
			},
		},
	)
	_, err = service.prepareAgentDeliverySubmission(
		ctx, session, "execution-1", "hello", nil,
		promptTaskOptions{
			deliveryProtocol:           messagequeue.DeliveryProtocolPending,
			deliverySubmissionID:       "prompt:continuation",
			expectedDeliveryGeneration: 2,
		},
	)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("error = %v, want session recovery required", err)
	}
	if _, err := repo.GetAgentDeliverySubmission(ctx, "prompt:continuation"); err == nil {
		t.Fatal("rotated continuation unexpectedly created a durable submission")
	}
}
