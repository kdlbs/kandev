package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/org"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// orgAccountAdapter exposes the account operations the organization service
// needs without making internal/user depend on internal/org.
type orgAccountAdapter struct {
	accounts userstore.AccountRepository
}

func (a orgAccountAdapter) AssignUsersWithoutOrg(ctx context.Context, orgID string) (int64, error) {
	return a.tenancy().AssignUsersWithoutOrg(ctx, orgID)
}

func (a orgAccountAdapter) SetOperator(ctx context.Context, id string, operator bool) error {
	return a.tenancy().SetOperator(ctx, id, operator)
}

func (a orgAccountAdapter) CountOperators(ctx context.Context) (int, error) {
	return a.tenancy().CountOperators(ctx)
}

func (a orgAccountAdapter) FirstAdminID(ctx context.Context) (string, error) {
	return a.tenancy().FirstAdminID(ctx)
}

func (a orgAccountAdapter) DeleteUsersByOrg(ctx context.Context, orgID string) error {
	return a.tenancy().DeleteUsersByOrg(ctx, orgID)
}

// tenancy narrows the account repository to its tenancy operations. The
// concrete SQLite repository implements them; the interface assertion keeps
// existing AccountRepository fakes unaffected.
func (a orgAccountAdapter) tenancy() userstore.TenancyRepository {
	if t, ok := a.accounts.(userstore.TenancyRepository); ok {
		return t
	}
	return userstore.NoopTenancy{}
}

var _ org.AccountMigrator = orgAccountAdapter{}

// buildOrgService constructs the organization service. It is built even when
// organizations are off so callers never branch on nil; the service itself
// reports Enabled() false and every lifecycle operation is a no-op.
func buildOrgService(
	cfg *config.Config,
	pool *db.Pool,
	repos *Repositories,
	log *logger.Logger,
) (*org.Service, error) {
	store, err := org.NewStore(pool)
	if err != nil {
		return nil, err
	}
	return org.NewService(
		store,
		orgAccountAdapter{accounts: repos.UserAccounts},
		repos.Task,
		cfg.Features.MultiTenancy,
		log,
	), nil
}
