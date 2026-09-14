package messagequeue

import (
	"context"
	"errors"
	"testing"
)

type generationReadFailureRepository struct {
	Repository
}

func (r *generationReadFailureRepository) SessionGeneration(context.Context, string) (int64, error) {
	return 0, errors.New("non-transactional session generation read")
}

func (r *generationReadFailureRepository) LifecycleGeneration(context.Context, string) (int64, error) {
	return 0, errors.New("non-transactional lifecycle generation read")
}

func TestReserveQueuedUsesRepositoryAtomicGenerationSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := &generationReadFailureRepository{Repository: NewMemoryRepository()}
	service := setupService(t)
	service.repo = repo
	source, err := service.QueueMessage(ctx, "session-atomic", "task-atomic", "prompt", "", QueuedByUser, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	reserved, exists, autoRun := service.ReserveQueuedWithAutoRun(ctx, source.SessionID)
	if !exists || !autoRun || reserved == nil || reserved.ID != source.ID {
		t.Fatalf("atomic reservation = %#v, exists=%t, autoRun=%t", reserved, exists, autoRun)
	}
	if !reserved.reservationGenerationsCaptured {
		t.Fatal("repository reservation did not capture fencing generations")
	}
}
