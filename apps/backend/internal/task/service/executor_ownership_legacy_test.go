package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/stretchr/testify/require"
)

type executorAliasReadErrorRepo struct {
	repository.ExecutorRepository
	err         error
	failHistory bool
}

func (r *executorAliasReadErrorRepo) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	if id == "local_pc" && !r.failHistory {
		return nil, r.err
	}
	return r.ExecutorRepository.GetExecutor(ctx, id)
}

func TestExecutorOwnershipLegacyLocalAliasUsesBuiltinPolicy(t *testing.T) {
	for _, scenario := range []string{"shared", "private-profile", "private-alias-row", "deleted-alias-row", "unknown-id", "read-error", "history-read-error"} {
		t.Run(scenario, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := t.Context()
			if _, err := repo.GetExecutor(ctx, models.ExecutorIDLocal); err != nil {
				require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{ID: models.ExecutorIDLocal, Name: "Local", Type: models.ExecutorTypeLocal, IsSystem: true}))
			}
			selected := "local_pc"
			profileID := ""
			switch scenario {
			case "private-profile":
				profileID = "foreign-local-profile"
				require.NoError(t, repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{ID: profileID, ExecutorID: models.ExecutorIDLocal, Name: "Bob", OwnerUserID: "bob"}))
			case "private-alias-row", "deleted-alias-row":
				require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{ID: selected, Name: "Real private resource", Type: models.ExecutorTypeLocal, OwnerUserID: "bob"}))
			case "unknown-id":
				selected = "missing-executor"
			}
			if scenario == "deleted-alias-row" {
				require.NoError(t, repo.DeleteExecutor(ctx, selected))
			}
			seedExecutorSession(t, repo, "legacy-local-alias", selected)
			readErr := errors.New("executor storage unavailable")
			if scenario == "read-error" || scenario == "history-read-error" {
				svc.executors = &executorAliasReadErrorRepo{ExecutorRepository: repo, err: readErr, failHistory: scenario == "history-read-error"}
			}
			caller := authn.WithIdentity(ctx, authn.Identity{UserID: "alice", Role: authn.RoleMember})
			_, err := svc.PrepareExecutorAdmission(caller, "legacy-local-alias", selected, profileID)
			if scenario == "read-error" || scenario == "history-read-error" {
				require.ErrorIs(t, err, readErr)
			}
			if scenario == "shared" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func (r *executorAliasReadErrorRepo) HasExecutorRecord(context.Context, string) (bool, error) {
	return false, r.err
}
