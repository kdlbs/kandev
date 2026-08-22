package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/task/service"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// userDirectoryAdapter exposes the minimum account information workspace
// membership needs, and nothing more.
//
// It lives in backendapp rather than internal/user because the contract it
// satisfies belongs to the task service: putting it in internal/user would
// make the account package depend on the task package.
type userDirectoryAdapter struct {
	accounts userstore.AccountRepository
}

func newUserDirectoryAdapter(accounts userstore.AccountRepository) *userDirectoryAdapter {
	return &userDirectoryAdapter{accounts: accounts}
}

// ListDirectory returns active users as ID plus display name. Email, role and
// status are deliberately dropped: a member picker needs a name, and anything
// more would hand every authenticated user a full account dump.
func (a *userDirectoryAdapter) ListDirectory(ctx context.Context) ([]service.DirectoryUser, error) {
	users, err := a.accounts.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.DirectoryUser, 0, len(users))
	for _, user := range users {
		if user == nil || user.Status == "disabled" {
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
