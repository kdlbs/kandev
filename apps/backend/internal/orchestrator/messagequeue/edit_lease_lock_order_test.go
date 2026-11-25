package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// fenceLockProbeRepository records whether the service edit-lease mutex is
// free when the repository session fence begins. A lease-guarded save holds
// that mutex across its repository write, so a fence entered without it can
// deadlock against the save on a single SQLite writer connection.
type fenceLockProbeRepository struct {
	*sqliteRepository
	service        *Service
	enteredUnfreed int
}

func (r *fenceLockProbeRepository) withSessionTransferFence(
	ctx context.Context, sessionID string, fn func(context.Context) error,
) error {
	if r.service.editLeaseMu.TryLock() {
		r.service.editLeaseMu.Unlock()
		r.enteredUnfreed++
	}
	return r.sqliteRepository.withSessionTransferFence(ctx, sessionID, fn)
}

func TestEditLeaseFencedOperationsHoldLeaseMutexBeforeRepositoryFence(t *testing.T) {
	operations := map[string]func(*Service, *QueueEditLease) error{
		"renew": func(s *Service, lease *QueueEditLease) error {
			_, err := s.RenewEdit(context.Background(), lease.SessionID, lease.EntryID, lease.LeaseID, "connection")
			return err
		},
		"end": func(s *Service, lease *QueueEditLease) error {
			return s.EndEdit(context.Background(), lease.SessionID, lease.EntryID, lease.LeaseID, "connection")
		},
		"end after save": func(s *Service, lease *QueueEditLease) error {
			_, err := s.EndEditAfterSave(context.Background(), lease.SessionID, lease.EntryID, lease.LeaseID, "connection")
			return err
		},
	}
	for name, operate := range operations {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := &fenceLockProbeRepository{sqliteRepository: newTestSQLiteRepo(t).(*sqliteRepository)}
			service := NewService(repo, 10, logger.Default())
			repo.service = service
			entry, err := service.QueueMessage(ctx, "session", "task", "body", "", QueuedByUser, false, nil)
			require.NoError(t, err)
			lease, err := service.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
			require.NoError(t, err)
			repo.enteredUnfreed = 0

			require.NoError(t, operate(service, lease))
			require.Zero(t, repo.enteredUnfreed, "repository fence entered while the edit-lease mutex was free")
		})
	}
}
