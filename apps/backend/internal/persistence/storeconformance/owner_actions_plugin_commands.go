package storeconformance

import (
	"errors"
	"fmt"

	pluginstate "github.com/kandev/kandev/internal/plugins/state"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

const (
	conformanceInstallation = "conformance-installation"
	conformanceProvider     = "conformance-provider"
)

func pluginCommandStoreFactory(s testconformance.ScenarioContext) (*pluginstate.CommandStore, error) {
	return pluginstate.NewCommandStore(ownerPool(s))
}

// pluginIdentityLinkAction drives the command store's identity-link API. A
// subject identifies the record: linking the same subject again replaces the
// row, which is how the owning API updates a link.
func pluginIdentityLinkAction() apiAction {
	link := func(id, displayName string) pluginstate.IdentityLink {
		return pluginstate.IdentityLink{
			InstallationID: conformanceInstallation, Provider: conformanceProvider,
			Subject: id, UserID: "user-" + id, DisplayName: displayName,
		}
	}
	action := apiAction{name: "plugin_identity_links"}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := pluginCommandStoreFactory(s)
		if err != nil {
			return nil, err
		}
		created, err := store.LinkIdentity(s.Context, link(id, "created"))
		if err != nil {
			return nil, err
		}
		return &created, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := pluginCommandStoreFactory(s)
		if err != nil {
			return nil, err
		}
		found, err := store.FindIdentityLink(s.Context, conformanceInstallation, conformanceProvider, id)
		if err != nil {
			return nil, err
		}
		return &found, nil
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginCommandStoreFactory(s)
		if err != nil {
			return err
		}
		_, err = store.LinkIdentity(s.Context, link(id, "updated"))
		return err
	}
	action.assertUpdated = func(_, after any, id string) error {
		if got, ok := after.(*pluginstate.IdentityLink); !ok || got.DisplayName != "updated" {
			return fmt.Errorf("plugin identity link %q was not updated: %+v", id, after)
		}
		return nil
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := pluginCommandStoreFactory(s)
		if err != nil {
			return err
		}
		found, err := store.FindIdentityLink(s.Context, conformanceInstallation, conformanceProvider, id)
		if err != nil {
			return err
		}
		return store.DeleteIdentityLink(s.Context, conformanceInstallation, found.ID)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginCommandStoreFactory(s)
		if err != nil {
			return err
		}
		if _, err := store.FindIdentityLink(s.Context, conformanceInstallation, conformanceProvider, id); !errors.Is(err, pluginstate.ErrIdentityLinkNotFound) {
			return fmt.Errorf("plugin identity link %q after delete: %v", id, err)
		}
		return nil
	}
	return action
}
