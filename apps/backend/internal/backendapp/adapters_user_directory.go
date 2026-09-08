package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/task/service"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// directoryAccounts is the one method this adapter needs. Narrowing it from
// the full account repository is what makes the org filter testable without
// standing up an account store.
type directoryAccounts interface {
	ListUsers(ctx context.Context) ([]*usermodels.User, error)
	GetUser(ctx context.Context, id string) (*usermodels.User, error)
}

// userDirectoryAdapter exposes the minimum account information workspace
// membership needs, and nothing more.
//
// It lives in backendapp rather than internal/user because the contract it
// satisfies belongs to the task service: putting it in internal/user would
// make the account package depend on the task package.
type userDirectoryAdapter struct {
	accounts directoryAccounts
}

func newUserDirectoryAdapter(accounts userstore.AccountRepository) *userDirectoryAdapter {
	return &userDirectoryAdapter{accounts: accounts}
}

// ListDirectory returns active users as ID plus display name. Email, role and
// status are deliberately dropped: a member picker needs a name, and anything
// more would hand every authenticated user a full account dump.
func (a *userDirectoryAdapter) ListDirectory(ctx context.Context, orgID string) ([]service.DirectoryUser, error) {
	users, err := a.accounts.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.DirectoryUser, 0, len(users))
	for _, user := range users {
		if user == nil || user.Status == "disabled" {
			continue
		}
		// Filter here rather than in the store: with organizations off every
		// account carries an empty org and the comparison is a no-op, so one
		// code path covers both shapes.
		if orgID != "" && user.OrgID != orgID {
			continue
		}
		name := user.DisplayName
		if name == "" {
			name = user.Email
		}
		out = append(out, service.DirectoryUser{ID: user.ID, DisplayName: name})
	}
	return out, nil
}

// LookupStatus reports whether a user exists and, if so, their status and role.
func (a *userDirectoryAdapter) LookupStatus(ctx context.Context, userID string) (string, string, bool, error) {
	user, err := a.accounts.GetUser(ctx, userID)
	if err != nil || user == nil {
		// A missing account is an expected outcome here, not a failure: the
		// caller turns it into a "user not found" validation error.
		return "", "", false, nil //nolint:nilerr // absence is not an error on this path
	}
	return user.Status, user.Role, true, nil
}

var _ service.UserDirectory = (*userDirectoryAdapter)(nil)
