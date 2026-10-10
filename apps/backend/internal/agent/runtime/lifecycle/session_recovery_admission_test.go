package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type recoveryAdmissionStore struct {
	block *models.SessionRecoveryBlock
}

func (s *recoveryAdmissionStore) CreateHarnessSessionGeneration(context.Context, *models.HarnessSessionGeneration) error {
	return nil
}
func (s *recoveryAdmissionStore) GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error) {
	return nil, sql.ErrNoRows
}
func (s *recoveryAdmissionStore) CommitHarnessSessionGeneration(context.Context, *models.HarnessSessionGeneration, int64) (bool, error) {
	return true, nil
}
func (s *recoveryAdmissionStore) CreateRestoreAttempt(context.Context, *models.RestoreAttempt) error {
	return nil
}
func (s *recoveryAdmissionStore) CompleteRestoreAttempt(context.Context, string, string, time.Time) error {
	return nil
}
func (s *recoveryAdmissionStore) CreateContinuationSnapshot(context.Context, *models.ContinuationSnapshot) error {
	return nil
}
func (s *recoveryAdmissionStore) GetContinuationSnapshot(context.Context, string) (*models.ContinuationSnapshot, error) {
	return nil, sql.ErrNoRows
}
func (s *recoveryAdmissionStore) UpsertSessionRecoveryBlock(_ context.Context, block *models.SessionRecoveryBlock) error {
	if s.block == nil {
		block.ID = "block-1"
	}
	s.block = block
	return nil
}
func (s *recoveryAdmissionStore) GetOpenSessionRecoveryBlock(context.Context, string, string, int64) (*models.SessionRecoveryBlock, error) {
	if s.block == nil || s.block.State != models.RecoveryBlockOpen {
		return nil, sql.ErrNoRows
	}
	return s.block, nil
}
func (s *recoveryAdmissionStore) ResolveSessionRecoveryBlock(_ context.Context, id, action string, resolvedAt time.Time) (bool, error) {
	if s.block == nil || s.block.ID != id || s.block.State != models.RecoveryBlockOpen {
		return false, nil
	}
	s.block.State = models.RecoveryBlockResolved
	s.block.AuthorizedAction = action
	s.block.ResolvedAt = &resolvedAt
	return true, nil
}

func TestOfficeSessionRecoveryParksDurably(t *testing.T) {
	store := &recoveryAdmissionStore{}
	admission := &RecoveryAdmission{Store: store}
	block, err := admission.Block(context.Background(), RestoreIdentity{
		SessionID:         "session-office",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 4,
	}, string(RestoreReasonNativeStateMissing), "office-run-1")
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	if block.State != models.RecoveryBlockOpen || block.ConsumerReference != "office-run-1" {
		t.Fatalf("block = %+v, want open office reference", block)
	}
	err = admission.Check(context.Background(), "session-office", "incarnation-1", 4)
	var blocked *SessionRecoveryBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("Check error = %v, want durable recovery block", err)
	}
}

func TestOfficeRecoveryCannotBeClearedByBackgroundActors(t *testing.T) {
	store := &recoveryAdmissionStore{}
	admission := &RecoveryAdmission{Store: store}
	_, err := admission.Block(context.Background(), RestoreIdentity{SessionID: "session-office", IncarnationID: "incarnation-1"}, "native_state_missing", "office-run-1")
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	if err := admission.Check(context.Background(), "session-office", "incarnation-1", 0); err == nil {
		t.Fatal("background admission cleared the open block")
	}
	resolved, err := admission.Resolve(context.Background(), "block-1", string(RestoreActionContinueFromHistory))
	if err != nil || !resolved {
		t.Fatalf("Resolve = %v/%v, want explicit resolution", resolved, err)
	}
	if err := admission.Check(context.Background(), "session-office", "incarnation-1", 0); err != nil {
		t.Fatalf("resolved block still prevented admission: %v", err)
	}
}
