package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orgunit"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/user/store"
)

// buildOrgUnitService creates the unit tree, wires its occupancy seam, and runs
// the one-shot placement so no workspace is left without a home.
//
// It runs after the tenancy migration, which is what assigns organization ids:
// placing a workspace before it knows its organization would put it under the
// wrong root.
func buildOrgUnitService(
	pool *db.Pool,
	taskRepo *tasksqlite.Repository,
	accounts store.AccountRepository,
	log *logger.Logger,
) (*orgunit.Service, error) {
	unitStore, err := orgunit.NewStore(pool)
	if err != nil {
		return nil, err
	}
	svc := orgunit.NewService(unitStore, log)
	svc.SetWorkspaceCounter(taskRepo)

	if _, err := svc.Backfill(
		context.Background(),
		unitAccountAdapter{accounts: accounts},
		unitPlacementAdapter{repo: taskRepo},
	); err != nil {
		return nil, fmt.Errorf("organization unit backfill: %w", err)
	}
	return svc, nil
}

// unitAccountAdapter narrows the user repository to what a personal unit needs.
type unitAccountAdapter struct{ accounts store.AccountRepository }

func (a unitAccountAdapter) ListUnitUsers(ctx context.Context) ([]orgunit.UserRef, error) {
	users, err := a.accounts.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]orgunit.UserRef, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		out = append(out, orgunit.UserRef{ID: u.ID, OrgID: u.OrgID, DisplayName: u.DisplayName})
	}
	return out, nil
}

// unitPlacementAdapter narrows the task repository to workspace placement.
type unitPlacementAdapter struct{ repo *tasksqlite.Repository }

func (a unitPlacementAdapter) UnplacedWorkspaces(ctx context.Context) ([]orgunit.WorkspaceRef, error) {
	rows, err := a.repo.UnplacedWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]orgunit.WorkspaceRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, orgunit.WorkspaceRef{ID: r.ID, OrgID: r.OrgID, OwnerID: r.OwnerID})
	}
	return out, nil
}

func (a unitPlacementAdapter) PlaceWorkspace(ctx context.Context, workspaceID, unitID string) error {
	return a.repo.PlaceWorkspace(ctx, workspaceID, unitID)
}
