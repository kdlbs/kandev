package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPermissionResolutionServicePublishesOnlySuccessfulWrites(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-permission-service")
	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "permission-service-message",
		TaskID:        "task-123",
		TaskSessionID: sessionID,
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypePermissionRequest,
		Metadata: map[string]any{
			"request_id": "request-service",
			"pending_id": "pending-service",
		},
	}); err != nil {
		t.Fatal(err)
	}
	eventBus.ClearEvents()
	selectedAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	claim := models.PermissionResolutionClaimRequest{
		TaskID:    "task-123",
		SessionID: sessionID,
		Audit: models.PermissionResolutionAudit{
			ClaimID:    "claim-service",
			ActorKind:  models.PermissionActorSynthetic,
			Source:     models.PermissionSourceExternalMCP,
			RequestID:  "request-service",
			PendingID:  "pending-service",
			OptionID:   "allow-once",
			OptionKind: "allow_once",
			SelectedAt: selectedAt,
		},
	}

	result, err := svc.ClaimPermissionResolution(ctx, claim)
	if err != nil || result.Outcome != models.PermissionClaimed {
		t.Fatalf("claim = %+v, err=%v", result, err)
	}
	if got := len(eventBus.GetPublishedEvents()); got != 1 {
		t.Fatalf("events after claim = %d, want 1", got)
	}
	competing := claim
	competing.Audit.ClaimID = "claim-other"
	result, err = svc.ClaimPermissionResolution(ctx, competing)
	if err != nil || result.Outcome != models.PermissionClaimInProgress {
		t.Fatalf("competing claim = %+v, err=%v", result, err)
	}
	if got := len(eventBus.GetPublishedEvents()); got != 1 {
		t.Fatalf("events after no-op claim = %d, want 1", got)
	}

	finalized, err := svc.FinalizePermissionResolution(ctx, models.PermissionResolutionFinalizeRequest{
		TaskID:      "task-123",
		SessionID:   sessionID,
		RequestID:   "request-service",
		PendingID:   "pending-service",
		ClaimID:     "claim-service",
		Result:      models.PermissionResolutionAccepted,
		Status:      models.PermissionStatusApproved,
		FinalizedAt: selectedAt.Add(time.Second),
	})
	if err != nil || finalized.Outcome != models.PermissionFinalized {
		t.Fatalf("finalize = %+v, err=%v", finalized, err)
	}
	if got := len(eventBus.GetPublishedEvents()); got != 2 {
		t.Fatalf("events after finalization = %d, want 2", got)
	}
}
