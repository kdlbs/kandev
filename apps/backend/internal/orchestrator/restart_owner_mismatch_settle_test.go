package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

func TestSettleDetachedTurnAfterOwnerMismatchFreesSession(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1", "session1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	ctx := context.Background()
	if _, err := svc.turnService.StartTurn(ctx, "session1"); err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	now := time.Now().UTC()
	submission := &models.AgentDeliverySubmission{
		ID: "prompt:detached", SessionID: "session1", IncarnationID: "session1", HarnessGeneration: 1,
		OwnerGeneration: 1, DispatchAttemptID: "detached", PayloadHash: "hash", Payload: []byte("prompt"),
		State: models.DeliverySubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, submission); err != nil {
		t.Fatal(err)
	}

	if err := svc.settleDetachedTurnAfterOwnerMismatch(ctx, "session1"); err != nil {
		t.Fatalf("settle: %v", err)
	}

	stored, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.DeliverySubmissionInterruptedUnknown {
		t.Fatalf("submission state = %s, want interrupted_unknown", stored.State)
	}
	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("open turns = %d, want 0", open)
	}
}

func TestIsDetachedDeliveryOwnerMismatch(t *testing.T) {
	if !isDetachedDeliveryOwnerMismatch(fmt.Errorf("x: %w", agentruntime.ErrDeliveryOwnerMismatch)) {
		t.Fatal("owner mismatch must be recognized")
	}
	if !isDetachedDeliveryOwnerMismatch(evidencedOwnerMismatchError()) {
		t.Fatal("the evidenced 409 OWNER_MISMATCH chain must be recognized")
	}
	if isDetachedDeliveryOwnerMismatch(transientStaleEvidenceError()) {
		t.Fatal("stale evidence without the OWNER_MISMATCH code can be transient and keeps retrying")
	}
	if isDetachedDeliveryOwnerMismatch(errors.Join(agentruntime.ErrDeliveryRecordEvidenceConflict,
		&agentctl.DeliveryHTTPError{StatusCode: http.StatusRequestEntityTooLarge, Body: "SUBMISSION_TOO_LARGE"})) {
		t.Fatal("a 413 evidence conflict is not an owner mismatch")
	}
	if isDetachedDeliveryOwnerMismatch(errors.New("transient")) {
		t.Fatal("transient errors keep retrying")
	}
}

// evidencedOwnerMismatchError is the error chain lifecycle builds for the
// card's HTTP 409 OWNER_MISMATCH ("delivery record evidence changed").
func evidencedOwnerMismatchError() error {
	httpErr := &agentctl.DeliveryHTTPError{
		StatusCode: http.StatusConflict, Body: `{"code":"OWNER_MISMATCH","message":"OWNER_MISMATCH"}`,
	}
	return errors.Join(agentruntime.ErrDeliveryRecordEvidenceStale, agentruntime.ErrDeliveryRecoveryBlocked, httpErr)
}

func transientStaleEvidenceError() error {
	httpErr := &agentctl.DeliveryHTTPError{
		StatusCode: http.StatusConflict, Body: `{"code":"SEQUENCE_CONFLICT","message":"SEQUENCE_CONFLICT"}`,
	}
	return errors.Join(agentruntime.ErrDeliveryRecordEvidenceStale, agentruntime.ErrDeliveryRecoveryBlocked, httpErr)
}
