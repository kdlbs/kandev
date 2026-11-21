package handlers

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/task/models"
)

// rejectionSessionRepo records session metadata writes for the step_complete
// refusal helpers.
type rejectionSessionRepo struct {
	SessionRepository
	writes map[string]interface{}
}

func (r *rejectionSessionRepo) SetSessionMetadataKey(_ context.Context, sessionID, key string, value interface{}) error {
	r.writes[sessionID+"/"+key] = value
	return nil
}

func TestStepCompleteRejectionIsRecordedAndCleared(t *testing.T) {
	repo := &rejectionSessionRepo{writes: map[string]interface{}{}}
	h := &Handlers{sessionRepo: repo, logger: testLogger(t)}
	ctx := context.Background()
	key := "session-1/" + models.SessionMetaKeyStepCompleteRejection

	h.recordStepCompleteRejection(ctx, "session-1", "step-1", "  profile mismatch  ")
	recorded, ok := models.LoadStepCompleteRejection(map[string]interface{}{
		models.SessionMetaKeyStepCompleteRejection: repo.writes[key],
	})
	if !ok || recorded.StepID != "step-1" || recorded.Reason != "profile mismatch" || recorded.At.IsZero() {
		t.Fatalf("recorded rejection = %+v (ok=%v)", recorded, ok)
	}

	h.clearStepCompleteRejection(ctx, "session-1")
	if value, present := repo.writes[key]; !present || value != nil {
		t.Fatalf("clear wrote %v (present=%v), want nil", value, present)
	}
}

func TestStepCompleteRejectionIgnoresIncompleteInput(t *testing.T) {
	repo := &rejectionSessionRepo{writes: map[string]interface{}{}}
	h := &Handlers{sessionRepo: repo, logger: testLogger(t)}
	ctx := context.Background()

	h.recordStepCompleteRejection(ctx, "", "step-1", "reason")
	h.recordStepCompleteRejection(ctx, "session-1", "", "reason")
	h.recordStepCompleteRejection(ctx, "session-1", "step-1", "   ")
	if len(repo.writes) != 0 {
		t.Fatalf("incomplete refusals must not be recorded, got %v", repo.writes)
	}
}

func TestBoundStepCompleteRejectionReasonKeepsWholeRunes(t *testing.T) {
	reason := strings.Repeat("단", stepCompleteRejectionReasonLimit+10)
	bounded := boundStepCompleteRejectionReason(reason)
	if !utf8.ValidString(bounded) {
		t.Fatalf("bounded reason is not valid UTF-8: %q", bounded)
	}
	if got := utf8.RuneCountInString(bounded); got != stepCompleteRejectionReasonLimit {
		t.Fatalf("bounded reason has %d runes, want %d", got, stepCompleteRejectionReasonLimit)
	}
}
