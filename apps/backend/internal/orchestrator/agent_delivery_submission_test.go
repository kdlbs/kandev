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
	capability agentruntime.DurableDeliveryCapability
	advertised bool
}

func (m *durableDeliveryTestAgentManager) DurableDeliveryCapabilityForExecution(
	context.Context,
	string,
) (agentruntime.DurableDeliveryCapability, bool) {
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
