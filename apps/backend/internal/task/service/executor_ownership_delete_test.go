package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/stretchr/testify/require"
)

type ownershipDeleteRaceRepo struct {
	repository.ExecutorRepository
	afterExecutorRead func(*models.Executor)
	afterProfileRead  func(*models.ExecutorProfile)
}

func (r *ownershipDeleteRaceRepo) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	row, err := r.ExecutorRepository.GetExecutor(ctx, id)
	if err == nil && r.afterExecutorRead != nil {
		r.afterExecutorRead(row)
	}
	return row, err
}
func (r *ownershipDeleteRaceRepo) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error) {
	row, err := r.ExecutorRepository.GetExecutorProfile(ctx, id)
	if err == nil && r.afterProfileRead != nil {
		r.afterProfileRead(row)
	}
	return row, err
}

// @covers AC-EXECUTORS-OWNERSHIP-002.3
func TestExecutorOwnershipDeleteRejectsConcurrentTransfer(t *testing.T) {
	for _, profileDelete := range []bool{false, true} {
		for _, transfer := range []bool{false, true} {
			name := "executor"
			if profileDelete {
				name = "profile"
			}
			if transfer {
				name += "-transferred"
			}
			t.Run(name, func(t *testing.T) {
				svc, bus, repo := createTestService(t)
				ctx := t.Context()
				executor := &models.Executor{ID: "delete-race", Name: "Personal", Type: models.ExecutorTypeLocal, OwnerUserID: "alice"}
				if profileDelete {
					executor.OwnerUserID = ""
				}
				require.NoError(t, repo.CreateExecutor(ctx, executor))
				profile := &models.ExecutorProfile{ID: "delete-race-profile", ExecutorID: executor.ID, Name: "Personal", OwnerUserID: "alice"}
				if profileDelete {
					require.NoError(t, repo.CreateExecutorProfile(ctx, profile))
				}
				wrapped := &ownershipDeleteRaceRepo{ExecutorRepository: repo}
				if transfer {
					bob := "bob"
					if profileDelete {
						wrapped.afterProfileRead = func(row *models.ExecutorProfile) {
							replacement := *row
							replacement.OwnershipIntent = &models.ExecutorOwnershipIntent{ExpectedOwnerUserID: "alice", OwnerUserID: &bob}
							require.NoError(t, repo.UpdateExecutorProfile(ctx, &replacement))
						}
					} else {
						wrapped.afterExecutorRead = func(row *models.Executor) {
							replacement := *row
							replacement.OwnershipIntent = &models.ExecutorOwnershipIntent{ExpectedOwnerUserID: "alice", OwnerUserID: &bob}
							require.NoError(t, repo.UpdateExecutor(ctx, &replacement))
						}
					}
				}
				svc.executors = wrapped
				bus.ClearEvents()
				caller := authn.WithIdentity(ctx, authn.Identity{UserID: "alice", Role: authn.RoleMember})
				var err error
				if profileDelete {
					err = svc.DeleteExecutorProfile(caller, profile.ID)
				} else {
					err = svc.DeleteExecutor(caller, executor.ID)
				}
				if !transfer {
					require.NoError(t, err)
					return
				}
				require.ErrorIs(t, err, models.ErrExecutorOwnershipChanged)
				require.Empty(t, bus.GetPublishedEvents())
				if profileDelete {
					row, loadErr := repo.GetExecutorProfile(ctx, profile.ID)
					require.NoError(t, loadErr)
					require.Equal(t, "bob", row.OwnerUserID)
				} else {
					row, loadErr := repo.GetExecutor(ctx, executor.ID)
					require.NoError(t, loadErr)
					require.Equal(t, "bob", row.OwnerUserID)
				}
			})
		}
	}
}
