package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/authz"
	settingsstore "github.com/kandev/kandev/internal/system/settings"
	"github.com/kandev/kandev/internal/task/service"
)

// SettingKeyDefaultWorkspaceVisibility is the install-wide default applied to
// newly created workspaces. A team sets it to "org" once and never invites
// anyone; an install that is several individuals sharing a box leaves it
// "private" and behaves exactly as it does today.
const SettingKeyDefaultWorkspaceVisibility = "workspace.default_visibility"

type orgSettingsAdapter struct {
	store *settingsstore.Store
}

func newOrgSettingsAdapter(store *settingsstore.Store) *orgSettingsAdapter {
	return &orgSettingsAdapter{store: store}
}

// DefaultWorkspaceVisibility reads the stored default, falling back to private.
// A read failure also falls back to private: the safe direction for an unknown
// answer is the narrower one.
func (a *orgSettingsAdapter) DefaultWorkspaceVisibility(ctx context.Context) authz.Visibility {
	if a.store == nil {
		return authz.VisibilityPrivate
	}
	value, ok, err := a.store.Get(ctx, SettingKeyDefaultWorkspaceVisibility)
	if err != nil || !ok {
		return authz.VisibilityPrivate
	}
	return authz.NormalizeVisibility(string(value))
}

// SetDefaultWorkspaceVisibility persists the install-wide default.
func (a *orgSettingsAdapter) SetDefaultWorkspaceVisibility(ctx context.Context, visibility authz.Visibility) error {
	return a.store.Save(ctx, SettingKeyDefaultWorkspaceVisibility, []byte(visibility))
}

var _ service.OrgSettings = (*orgSettingsAdapter)(nil)
