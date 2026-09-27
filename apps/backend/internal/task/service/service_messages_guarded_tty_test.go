package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestGuardedTTYAuditServicePersistsClaimBeforeOneFinalization(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	claim := models.GuardedTTYAuditClaim{
		AttestationID: "attestation-service-1",
		Execution: streams.MCPExecutionContext{
			ExecutionID: "execution-1", TaskID: "task-123", SessionID: sessionID,
		},
		WorkspaceID:      "workspace-1",
		AgentID:          streams.GuardedTTYAgentID,
		PrincipalSurface: "kanban-task",
		Argv:             []string{"stty", "-a"},
		RequestedAt:      time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
		Outcome:          models.GuardedTTYOutcomePending,
	}

	if err := svc.ClaimGuardedTTYExecution(ctx, claim); err != nil {
		t.Fatalf("claim: %v", err)
	}
	message, err := repo.GetMessage(ctx, claim.AttestationID)
	if err != nil {
		t.Fatalf("get claim: %v", err)
	}
	audit, ok := models.GuardedTTYAuditFromMetadata(message.Metadata)
	if !ok || audit.Execution != claim.Execution || audit.Outcome != models.GuardedTTYOutcomePending {
		t.Fatalf("claim audit = %+v, ok=%v", audit, ok)
	}
	if got := len(eventBus.GetPublishedEvents()); got == 0 {
		t.Fatal("claim did not publish its durable message")
	}
	beforeFinalizeEvents := len(eventBus.GetPublishedEvents())
	finalize := models.GuardedTTYAuditFinalize{
		AttestationID:   claim.AttestationID,
		Execution:       claim.Execution,
		Outcome:         models.GuardedTTYOutcomeSucceeded,
		ExitCode:        0,
		OutputBytes:     42,
		OutputSHA256:    "digest",
		CompletionCount: 1,
		CompletedAt:     claim.RequestedAt.Add(time.Second),
		ProviderMetadata: map[string]interface{}{
			"method": "command/exec", "requested_tty": true, "dispatched_tty": true,
		},
	}
	if err := svc.FinalizeGuardedTTYExecution(ctx, finalize); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if got := len(eventBus.GetPublishedEvents()); got != beforeFinalizeEvents+1 {
		t.Fatalf("events after finalize = %d, want %d", got, beforeFinalizeEvents+1)
	}
	if err := svc.FinalizeGuardedTTYExecution(ctx, finalize); err == nil {
		t.Fatal("duplicate finalization succeeded")
	}
}
